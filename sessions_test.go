package nexhost

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	emit "github.com/openpak/rewards-emit"
)

func TestSessionClock(t *testing.T) {
	c := newSessionClock(3 * time.Minute)
	t0 := time.Now()
	two := map[string][]string{"7": {"1", "2"}}
	if d := c.tick(t0, two); len(d) != 0 {
		t.Fatalf("just joined: %v", d)
	}
	if d := c.tick(t0.Add(2*time.Minute), two); len(d) != 0 {
		t.Fatalf("2 minutes: %v", d)
	}
	if d := c.tick(t0.Add(3*time.Minute), two); len(d) != 2 {
		t.Fatalf("3 minutes should count both: %v", d)
	}
	if d := c.tick(t0.Add(4*time.Minute), two); len(d) != 0 {
		t.Fatalf("counts once: %v", d)
	}
	// Left alone: the clock resets, a lobby of one earns nothing.
	c.tick(t0.Add(5*time.Minute), map[string][]string{"7": {"1"}})
	if d := c.tick(t0.Add(9*time.Minute), map[string][]string{"7": {"1"}}); len(d) != 0 {
		t.Fatalf("a lobby of one counted: %v", d)
	}
	// A short session never counts, however many there are.
	c2 := newSessionClock(3 * time.Minute)
	for i := 0; i < 20; i++ {
		gid := string(rune('a' + i))
		c2.tick(t0.Add(time.Duration(i)*time.Minute), map[string][]string{gid: {"1", "2"}})
		if d := c2.tick(t0.Add(time.Duration(i)*time.Minute+90*time.Second), map[string][]string{}); len(d) != 0 {
			t.Fatalf("90 s session counted: %v", d)
		}
	}
}

// Needs a throwaway Postgres: NEX_SESSIONS_TEST_DATABASE_URL.
func TestWatcherReadsGatheringsAndEmits(t *testing.T) {
	url := os.Getenv("NEX_SESSIONS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("NEX_SESSIONS_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{`DROP SCHEMA IF EXISTS matchmaking CASCADE`, `CREATE SCHEMA matchmaking`,
		`CREATE TABLE matchmaking.gatherings (id bigserial PRIMARY KEY, registered boolean NOT NULL DEFAULT true, participants numeric(20)[] NOT NULL DEFAULT array[]::numeric(20)[])`,
		`INSERT INTO matchmaking.gatherings (registered, participants) VALUES (true, '{1001,1002}'), (true, '{1003}'), (false, '{1004,1005}')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var got []map[string]any
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var in struct{ Events []map[string]any }
		json.Unmarshal(b, &in)
		mu.Lock()
		got = append(got, in.Events...)
		mu.Unlock()
		w.Write([]byte(`{"results":[]}`))
	}))
	defer sink.Close()
	rw := emit.New(emit.Config{URL: sink.URL, Key: "k", Source: "t", Flush: 10 * time.Millisecond})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { watchSessions(db, rw, "kart_sessions.mk7", "3ds", 150*time.Millisecond, 40*time.Millisecond, stop); close(done) }()
	time.Sleep(600 * time.Millisecond)
	close(stop)
	<-done
	rw.Close(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("only the registered two-player gathering counts, both players once: %v", got)
	}
	for _, e := range got {
		id := e["identity"].(map[string]any)
		if e["metric"] != "kart_sessions.mk7" || id["namespace"] != "3ds" || (id["subject"] != "1001" && id["subject"] != "1002") {
			t.Fatalf("%v", e)
		}
		if k := e["keys"].([]any); len(k) != 1 || k[0] == "" {
			t.Fatalf("keys %v", e)
		}
	}
}
