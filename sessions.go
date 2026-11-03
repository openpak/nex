package nexhost

// Online sessions for rewards.
//
// A title's own matchmaking tables say who is in which gathering. A player counts as having
// played an online session once they have shared a gathering with at least one other player for
// SessionMin of unbroken time (default 3 minutes; REWARDS_SESSION_MIN). A lobby alone, or one
// that ends sooner, counts nothing. That proves presence in an online match or race lobby, not a
// finished race: the title servers cannot see a result.
//
// One event per (gathering, player). The event id and the distinct key are the gathering id,
// and rewards keeps one count per key per account, so restarts and retries never double count.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/lib/pq"
	emit "github.com/openpak/rewards-emit"
)

// SessionMin is how long two players must share a gathering before it counts.
func SessionMin() time.Duration {
	if d, err := time.ParseDuration(env("REWARDS_SESSION_MIN", "3m")); err == nil && d > 0 {
		return d
	}
	return 3 * time.Minute
}

// sessionClock tracks how long each (gathering, player) pair has been together.
type sessionClock struct {
	min     time.Duration
	since   map[string]time.Time // key "gid/pid" -> first tick seen with company
	emitted map[string]bool
}

func newSessionClock(min time.Duration) *sessionClock {
	return &sessionClock{min: min, since: map[string]time.Time{}, emitted: map[string]bool{}}
}

// state and restore carry the clock through the rewards-emit durable state (Postgres), so a
// restart does not restart the counting windows. The shape is emit.State.
func (c *sessionClock) state() emit.State {
	st := emit.State{Since: make(map[string]time.Time, len(c.since))}
	for k, v := range c.since {
		st.Since[k] = v
	}
	for k := range c.emitted {
		st.Emitted = append(st.Emitted, k)
	}
	sort.Strings(st.Emitted)
	return st
}

func (c *sessionClock) restore(st emit.State) {
	for k, v := range st.Since {
		c.since[k] = v
	}
	for _, k := range st.Emitted {
		c.emitted[k] = true
	}
}

// tick takes the gatherings present now (gid -> participant pids) and returns the pairs that
// just crossed the minimum. Gatherings with fewer than two players reset their players.
func (c *sessionClock) tick(now time.Time, gatherings map[string][]string) (done [][2]string) {
	seen := map[string]bool{}
	for gid, pids := range gatherings {
		if len(pids) < 2 {
			continue
		}
		for _, pid := range pids {
			k := gid + "/" + pid
			seen[k] = true
			if _, ok := c.since[k]; !ok {
				c.since[k] = now
			}
			if !c.emitted[k] && now.Sub(c.since[k]) >= c.min {
				c.emitted[k] = true
				done = append(done, [2]string{gid, pid})
			}
		}
	}
	for k := range c.since {
		if !seen[k] {
			delete(c.since, k)
		}
	}
	// Forget emitted pairs once their gathering is gone, so the sets stay small.
	for k := range c.emitted {
		if !seen[k] {
			delete(c.emitted, k)
		}
	}
	return done
}

// WatchSessions starts reporting online sessions of a title that keeps its matchmaking in
// Postgres (nex-protocols-common-go's matchmaking.gatherings). metric is the rewards metric
// (for example "kart_sessions.mk7") and namespace the account-core link namespace of the
// title's console family ("wiiu" or "3ds"). It opens its own read-only use of cfg.DatabaseURL
// and returns at once; with rewards unconfigured it does nothing.
func WatchSessions(cfg Config, metric, namespace string) {
	rw := Rewards()
	if !rw.Enabled() || cfg.DatabaseURL == "" {
		return
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Printf("rewards sessions: %v", err)
		return
	}
	// The poll runs every 20 s: one short connection, none held in between. Postgres is shared by
	// every title and has a hard connection cap.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	go watchSessions(db, rw, metric, namespace, SessionMin(), 20*time.Second, nil)
}

func watchSessions(db *sql.DB, rw *emit.Client, metric, namespace string, min, every time.Duration, stop <-chan struct{}) {
	clock := newSessionClock(min)
	stateName := "sessions." + metric
	if b, ok := rw.LoadState(stateName); ok {
		var st emit.State
		if json.Unmarshal(b, &st) == nil {
			clock.restore(st)
		}
	}
	var last []byte
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		g, err := readGatherings(db)
		if err != nil {
			// The table appears when the title first creates it; until then, and across
			// database restarts, there is simply nothing to count.
			log.Printf("rewards sessions (%s): %v", metric, err)
		} else {
			for _, pair := range clock.tick(time.Now(), g) {
				rw.Event(emit.Event{
					ID: fmt.Sprintf("%s:%s:%s", metric, pair[0], pair[1]), Metric: metric,
					Player: PlayerOf(namespace, parseUint(pair[1])), Keys: []string{pair[0]},
				})
			}
		}
		if b, err := json.Marshal(clock.state()); err == nil && !bytes.Equal(b, last) {
			rw.SaveState(stateName, b)
			last = b
		}
		select {
		case <-t.C:
		case <-stop:
			return
		}
	}
}

func readGatherings(db *sql.DB) (map[string][]string, error) {
	rows, err := db.Query(`SELECT id::text, participants::text[] FROM matchmaking.gatherings WHERE registered AND cardinality(participants) >= 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id string
		var ps pq.StringArray
		if err := rows.Scan(&id, &ps); err != nil {
			return nil, err
		}
		out[id] = ps
	}
	return out, rows.Err()
}

func parseUint(s string) uint64 {
	var n uint64
	fmt.Sscan(s, &n)
	return n
}
