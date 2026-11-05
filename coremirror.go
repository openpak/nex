package nexhost

import (
	"context"
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
}

// StartCoreMirror starts the write-through mirror of a title's gatherings (the common
// matchmaking tables in its own database) into the matchmaking core. It needs
// NEX_MATCHMAKING_ADDR with NEX_MATCHMAKING_KEY (or MATCHMAKING_INTERNAL_KEY) and nn-account for
// PID to account; missing settings are an error so a title whose flag is on never quietly
// skips the core. The NEX protocol layer is untouched: the console is still answered from the
// title's own tables. It returns after starting the loop.
func StartCoreMirror(cfg Config, db *sql.DB, spec MirrorSpec) error {
	conn, err := coreConn("NEX_MATCHMAKING_ADDR", "NEX_MATCHMAKING_KEY", "MATCHMAKING_INTERNAL_KEY")
	if err != nil {
		return err
	}
	if cfg.AccountGRPCAddr == "" {
		return fmt.Errorf("core mirror: NEX_ACCOUNT_GRPC_ADDR is required (PID to account)")
	}
	res, err := coreid.DialResolution(cfg.AccountGRPCAddr, cfg.AccountGRPCKey, spec.Network)
	if err != nil {
		return err
	}
	m := &coremirror.Mirror{MM: mm.NewMatchmakingClient(conn), IDs: &coreid.Cache{L: res},
		Title: spec.Title, Network: spec.Network, Compat: spec.Compat, Console: spec.Console}
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
			cancel()
		}
	}()
	log.Printf("[%s] core mirror on: sessions to the matchmaking core, network %s, group %s", spec.Title, spec.Network, spec.Compat)
	return nil
}

// readMirrorGatherings reads the registered MatchmakeSessions of the common tables.
func readMirrorGatherings(db *sql.DB) ([]coremirror.Gathering, error) {
	rows, err := db.Query(`SELECT g.id, g.owner_pid::bigint, g.host_pid::bigint, g.participants::text[], g.max_participants,
		m.game_mode, m.attribs::text[], m.application_buffer, m.open_participation
		FROM matchmaking.gatherings g JOIN matchmaking.matchmake_sessions m ON m.id = g.id
		WHERE g.registered`)
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
