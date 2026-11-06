package nexhost

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/lib/pq"

	"github.com/openpak/nex/internal/coreid"
	"github.com/openpak/nex/internal/coremirror"
	mm "github.com/openpak/nex/internal/matchmakingpb"
)

// MirrorSpec names a title's mirror into the shared matchmaking core.
type MirrorSpec struct {
	Title   string // neutral core title key, "terraria"
	Network string // account-link namespace and core network, "wiiu"
	Compat  string // compatibility group assigned by the server, "old-gen-1.2.4"
	Console string // console tag, "wiiu"
	// Cross are the other networks whose rooms of the same group this title's consoles see in their own search, as shadow
	// gatherings built from Shadow (prds#145 phase 1, listing only; joins cannot connect until the relay exists). Empty = off.
	Cross  []string
	Shadow ShadowTemplate
}

// ShadowTemplate is the MatchmakeSession a title's own consoles create, copied into every shadow gathering so the console's
// search criteria match it (values observed from a real host of the title). The owner is the foreign owner's PID in this
// namespace; max participants and openness come from the core.
type ShadowTemplate struct {
	GameMode      uint32
	Attribs       []uint32
	SystemType    uint32
	AppBuffer     []byte
	Policy        uint32 // participation policy (98 = friends of the owner only)
	Flags         uint32
	ProgressScore int16
}

// StartCoreMirror starts the write-through mirror of a title's gatherings (the common
// matchmaking tables in its own database) into the matchmaking core. It needs
// NEX_MATCHMAKING_ADDR with NEX_MATCHMAKING_KEY (or MATCHMAKING_INTERNAL_KEY) and nn-account for
// PID to account; missing settings are an error so a title whose flag is on never quietly
// skips the core. The NEX protocol layer is untouched: the console is still answered from the
// title's own tables. It returns after starting the loop.
func StartCoreMirror(cfg Config, db *sql.DB, spec MirrorSpec) (*coremirror.Mirror, error) {
	conn, err := coreConn("NEX_MATCHMAKING_ADDR", "NEX_MATCHMAKING_KEY", "MATCHMAKING_INTERNAL_KEY")
	if err != nil {
		return nil, err
	}
	if cfg.AccountGRPCAddr == "" {
		return nil, fmt.Errorf("core mirror: NEX_ACCOUNT_GRPC_ADDR is required (PID to account)")
	}
	res, err := coreid.DialResolution(cfg.AccountGRPCAddr, cfg.AccountGRPCKey, spec.Network)
	if err != nil {
		return nil, err
	}
	m := &coremirror.Mirror{MM: mm.NewMatchmakingClient(conn), IDs: &coreid.Cache{L: res},
		Title: spec.Title, Network: spec.Network, Compat: spec.Compat, Console: spec.Console, Cross: spec.Cross}
	if _, err := db.Exec(shadowSchema); err != nil {
		return nil, fmt.Errorf("core mirror: shadow table: %w", err)
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for ; ; <-t.C {
			gs, err := readMirrorGatherings(db)
			if err != nil {
				// The tables appear when the title first creates them.
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			m.Tick(ctx, gs)
			syncShadows(ctx, db, m, spec)
			cancel()
		}
	}()
	log.Printf("[%s] core mirror on: sessions to the matchmaking core, network %s, group %s", spec.Title, spec.Network, spec.Compat)
	if len(spec.Cross) > 0 {
		log.Printf("[%s] cross-network listing on: rooms of %v in group %s appear as shadow gatherings (listing only)", spec.Title, spec.Cross, spec.Compat)
	}
	return m, nil
}

// readMirrorGatherings reads the registered MatchmakeSessions of the common tables.
func readMirrorGatherings(db *sql.DB) ([]coremirror.Gathering, error) {
	rows, err := db.Query(`SELECT g.id, g.owner_pid::bigint, g.host_pid::bigint, g.participants::text[], g.max_participants,
		m.game_mode, m.attribs::text[], m.application_buffer, m.open_participation
		FROM matchmaking.gatherings g JOIN matchmaking.matchmake_sessions m ON m.id = g.id
		WHERE g.registered AND g.id NOT IN (SELECT gathering_id FROM openpak_crossplay.shadow_gatherings)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []coremirror.Gathering
	for rows.Next() {
		var g coremirror.Gathering
		var owner, host, mode int64
		var id int64
		var parts, attribs pq.StringArray
		var max sql.NullInt64
		var open sql.NullBool
		if err := rows.Scan(&id, &owner, &host, &parts, &max, &mode, &attribs, &g.AppBuffer, &open); err != nil {
			return nil, err
		}
		g.ID, g.Owner, g.Host, g.GameMode, g.Open = uint64(id), uint64(owner), uint64(host), uint32(mode), open.Bool
		g.Max = uint32(max.Int64)
		for _, p := range parts {
			if n, err := strconv.ParseUint(p, 10, 64); err == nil {
				g.Participants = append(g.Participants, n)
			}
		}
		for _, a := range attribs {
			if n, err := strconv.ParseUint(a, 10, 32); err == nil {
				g.Attribs = append(g.Attribs, uint32(n))
			}
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// shadowSchema maps a foreign core session to the gathering that stands for it in this title's tables. It holds ids only: the
// owner PID on the gathering row is resolved from the account core on every pass, names never appear.
const shadowSchema = `CREATE SCHEMA IF NOT EXISTS openpak_crossplay;
CREATE TABLE IF NOT EXISTS openpak_crossplay.shadow_gatherings (
	core_session text PRIMARY KEY,
	gathering_id bigint NOT NULL UNIQUE,
	network text NOT NULL,
	created timestamptz NOT NULL DEFAULT now()
)`

// syncShadows makes the title's own tables hold one registered MatchmakeSession per open foreign room of the group (so the
// console's own search and its friends-only policy apply unchanged), and removes the ones whose room is gone. With Cross off
// it removes every shadow. Errors are logged; the next pass retries.
func syncShadows(ctx context.Context, db *sql.DB, m *coremirror.Mirror, spec MirrorSpec) {
	var rooms []coremirror.ForeignRoom
	if len(spec.Cross) > 0 {
		var err error
		if rooms, err = m.Foreign(ctx); err != nil {
			log.Printf("[%s] cross-network listing: %v", spec.Title, err)
			return // keep the shadows as they are until the core answers
		}
	}
	have := map[string]uint32{}
	rows, err := db.QueryContext(ctx, `SELECT core_session, gathering_id FROM openpak_crossplay.shadow_gatherings`)
	if err != nil {
		log.Printf("[%s] cross-network listing: read shadows: %v", spec.Title, err)
		return
	}
	for rows.Next() {
		var sid string
		var gid int64
		if rows.Scan(&sid, &gid) == nil {
			have[sid] = uint32(gid)
		}
	}
	rows.Close()
	want := map[string]bool{}
	for _, r := range rooms {
		want[r.Session] = true
		if gid, ok := have[r.Session]; ok {
			found, err := updateShadow(ctx, db, gid, r)
			if err != nil {
				log.Printf("[%s] cross-network listing: update shadow %d: %v", spec.Title, gid, err)
			}
			if found || err != nil {
				continue
			}
			// The gathering row is gone (deleted under us): recreate the shadow.
			if err := deleteShadow(ctx, db, r.Session, gid); err != nil {
				continue
			}
			delete(have, r.Session)
		}
		gid, err := insertShadow(ctx, db, r, spec.Shadow)
		if err != nil {
			log.Printf("[%s] cross-network listing: shadow for %s room %s: %v", spec.Title, r.Network, r.Session, err)
			continue
		}
		have[r.Session] = gid
		log.Printf("[%s] cross-network listing: %s room %s -> shadow gathering %d", spec.Title, r.Network, r.Session, gid)
	}
	for sid, gid := range have {
		if want[sid] {
			continue
		}
		if err := deleteShadow(ctx, db, sid, gid); err != nil {
			log.Printf("[%s] cross-network listing: drop shadow %d: %v", spec.Title, gid, err)
			continue
		}
		delete(have, sid)
		log.Printf("[%s] cross-network listing: room %s gone, shadow gathering %d dropped", spec.Title, sid, gid)
	}
	m.SetShadows(have)
}

func insertShadow(ctx context.Context, db *sql.DB, r coremirror.ForeignRoom, t ShadowTemplate) (uint32, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var gid int64
	err = tx.QueryRowContext(ctx, `INSERT INTO matchmaking.gatherings (owner_pid, host_pid, min_participants, max_participants,
		participation_policy, policy_argument, flags, state, description, registered, type, started_time, participants)
		VALUES ($1, $1, 1, $2, $3, 0, $4, 0, '', true, 'MatchmakeSession', now(), ARRAY[$1]::numeric(20)[]) RETURNING id`,
		r.OwnerPID, r.Max, t.Policy, t.Flags).Scan(&gid)
	if err != nil {
		return 0, err
	}
	attribs := make([]int64, len(t.Attribs))
	for i, a := range t.Attribs {
		attribs[i] = int64(a)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO matchmaking.matchmake_sessions (id, game_mode, attribs, open_participation,
		matchmake_system_type, application_buffer, progress_score, session_key, option_zero, matchmake_param, user_password,
		refer_gid, user_password_enabled, system_password_enabled, codeword)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, ''::bytea, '', 0, false, false, '')`,
		gid, t.GameMode, pq.Array(attribs), r.Open, t.SystemType, t.AppBuffer, t.ProgressScore, key); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO openpak_crossplay.shadow_gatherings (core_session, gathering_id, network) VALUES ($1, $2, $3)`,
		r.Session, gid, r.Network); err != nil {
		return 0, err
	}
	return uint32(gid), tx.Commit()
}

// updateShadow follows the core: owner (after a migration), size and openness, and registers the row again (the title
// unregisters every MatchmakeSession when it starts). Participants go back to the owner alone, so a console that joined a
// shadow (it cannot connect yet) does not stay in it. found is false when the gathering row no longer exists.
func updateShadow(ctx context.Context, db *sql.DB, gid uint32, r coremirror.ForeignRoom) (found bool, err error) {
	res, err := db.ExecContext(ctx, `UPDATE matchmaking.gatherings SET owner_pid=$2, host_pid=$2, max_participants=$3,
		participants=ARRAY[$2]::numeric(20)[], registered=true WHERE id=$1`, gid, r.OwnerPID, r.Max)
	if err != nil {
		return true, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	_, err = db.ExecContext(ctx, `UPDATE matchmaking.matchmake_sessions SET open_participation=$2 WHERE id=$1`, gid, r.Open)
	return true, err
}

func deleteShadow(ctx context.Context, db *sql.DB, sid string, gid uint32) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM matchmaking.matchmake_sessions WHERE id=$1`,
		`DELETE FROM matchmaking.gatherings WHERE id=$1`,
	} {
		if _, err := tx.ExecContext(ctx, q, gid); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM openpak_crossplay.shadow_gatherings WHERE core_session=$1`, sid); err != nil {
		return err
	}
	return tx.Commit()
}
