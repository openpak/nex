package nexhost

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"google.golang.org/grpc"

	"github.com/openpak/nex/internal/coreid"
	"github.com/openpak/nex/internal/coremirror"
	mm "github.com/openpak/nex/internal/matchmakingpb"
)

type xpCore struct {
	mm.MatchmakingClient
	found []*mm.Session
}

func (f *xpCore) SearchSessions(context.Context, *mm.SearchSessionsRequest, ...grpc.CallOption) (*mm.SearchSessionsResponse, error) {
	return &mm.SearchSessionsResponse{Sessions: f.found, Hits: uint32(len(f.found))}, nil
}

type xpIDs map[string]uint64

func (x xpIDs) Account(context.Context, uint64) (string, error) { return "", coreid.ErrNoLink }
func (x xpIDs) PID(_ context.Context, a string) (uint64, error) {
	if p, ok := x[a]; ok {
		return p, nil
	}
	return 0, coreid.ErrNoLink
}

// The NEX tables as nex-protocols-common-go creates them (the columns the shadow code and the search read).
const xpTables = `DROP SCHEMA IF EXISTS matchmaking CASCADE; DROP SCHEMA IF EXISTS openpak_crossplay CASCADE; CREATE SCHEMA matchmaking;
CREATE TABLE matchmaking.gatherings (id bigserial PRIMARY KEY, owner_pid numeric(20), host_pid numeric(20), min_participants integer,
	max_participants integer, participation_policy bigint, policy_argument bigint, flags bigint, state bigint, description text,
	registered boolean NOT NULL DEFAULT true, type text NOT NULL DEFAULT '', started_time timestamp,
	participants numeric(20)[] NOT NULL DEFAULT array[]::numeric(20)[]);
CREATE TABLE matchmaking.matchmake_sessions (id bigserial PRIMARY KEY, game_mode bigint, attribs bigint[], open_participation boolean,
	matchmake_system_type bigint, application_buffer bytea, flags bigint, state bigint, progress_score smallint, session_key bytea,
	option_zero bigint, matchmake_param bytea, user_password text, refer_gid bigint, user_password_enabled boolean,
	system_password_enabled boolean, codeword text, system_password text NOT NULL DEFAULT '')`

// Shadows appear for PS3 rooms, match the Wii U's own friends-only search, stay out of the mirror, follow the core and go away.
func TestShadowGatherings(t *testing.T) {
	url := os.Getenv("NEX_TEST_DSN")
	if url == "" {
		t.Skip("NEX_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.Exec(xpTables); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(shadowSchema); err != nil {
		t.Fatal(err)
	}
	// a real Wii U host, for the mirror
	if _, err := db.Exec(`INSERT INTO matchmaking.gatherings (owner_pid, host_pid, min_participants, max_participants, participation_policy, flags, type, participants)
		VALUES (100, 100, 1, 8, 98, 512, 'MatchmakeSession', ARRAY[100]::numeric(20)[]);
		INSERT INTO matchmaking.matchmake_sessions (id, game_mode, attribs, open_participation, matchmake_system_type, application_buffer)
		VALUES (1, 1, ARRAY[0,1,0,0,0,0], true, 2, '\x010203')`); err != nil {
		t.Fatal(err)
	}
	core := &xpCore{found: []*mm.Session{{Id: "ps3-room", Title: &mm.TitleRef{Network: "np", Title: "terraria"}, OwnerId: "acc-ps3", MaxMembers: 8, Open: true}}}
	m := &coremirror.Mirror{MM: core, IDs: xpIDs{"acc-ps3": 4242}, Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4", Cross: []string{"np"}}
	spec := MirrorSpec{Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4", Cross: []string{"np"}, Shadow: ShadowTemplate{GameMode: 1, Attribs: []uint32{0, 1, 0, 0, 0, 0}, SystemType: 2, AppBuffer: []byte{1, 2, 3}, Policy: 98, Flags: 512, ProgressScore: 100}}

	syncShadows(ctx, db, m, spec)
	syncShadows(ctx, db, m, spec) // idempotent
	var gid int64
	var owner, policy int64
	var open bool
	err = db.QueryRow(`SELECT g.id, g.owner_pid, g.participation_policy, ms.open_participation FROM matchmaking.gatherings g
		JOIN matchmaking.matchmake_sessions ms ON ms.id=g.id JOIN openpak_crossplay.shadow_gatherings s ON s.gathering_id=g.id
		WHERE s.core_session='ps3-room'`).Scan(&gid, &owner, &policy, &open)
	if err != nil || owner != 4242 || policy != 98 || !open {
		t.Fatalf("shadow: %v owner=%d policy=%d open=%v", err, owner, policy, open)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM openpak_crossplay.shadow_gatherings`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d shadows after two passes", n)
	}
	// the core-restricted search lets the shadow through
	m.MM = &xpCore{found: []*mm.Session{{Id: "ps3-room"}}}
	if c := m.Candidates(ctx); len(c) != 1 || int64(c[0]) != gid {
		t.Fatalf("candidates %v", c)
	}
	m.MM = core
	// the Wii U's own search (friends-only: the owner must be a friend) finds it with the observed criteria
	err = db.QueryRow(`SELECT count(*) FROM matchmaking.gatherings g JOIN matchmaking.matchmake_sessions ms ON ms.id=g.id
		WHERE g.registered AND g.type='MatchmakeSession' AND ms.refer_gid=0 AND ms.codeword='' AND array_length(ms.attribs,1)=6
		AND (CASE WHEN g.participation_policy=98 THEN g.owner_pid=ANY($1) ELSE true END) AND ms.game_mode=1 AND ms.matchmake_system_type=2`,
		"{4242}").Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("search by a friend of the owner: %v %d", err, n)
	}
	// the mirror never sends a shadow back to the core
	gs, err := readMirrorGatherings(db)
	if err != nil || len(gs) != 1 || gs[0].ID != 1 {
		t.Fatalf("mirror reads %+v %v", gs, err)
	}
	// the title unregisters every MatchmakeSession at start: the next pass registers the shadow again
	db.Exec(`UPDATE matchmaking.gatherings SET registered=false WHERE type='MatchmakeSession'`)
	syncShadows(ctx, db, m, spec)
	var reg bool
	db.QueryRow(`SELECT registered FROM matchmaking.gatherings WHERE id=$1`, gid).Scan(&reg)
	if !reg {
		t.Fatal("shadow not registered again after a title restart")
	}
	// a deleted gathering row is recreated
	db.Exec(`DELETE FROM matchmaking.gatherings WHERE id=$1`, gid)
	syncShadows(ctx, db, m, spec)
	if err := db.QueryRow(`SELECT gathering_id FROM openpak_crossplay.shadow_gatherings WHERE core_session='ps3-room'`).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT count(*) FROM matchmaking.gatherings WHERE id=$1 AND registered`, gid).Scan(&n)
	if n != 1 {
		t.Fatal("deleted shadow gathering not recreated")
	}
	// a console that joined the shadow is dropped again on the next pass; openness follows the core
	db.Exec(`UPDATE matchmaking.gatherings SET participants=ARRAY[4242,100]::numeric(20)[] WHERE id=$1`, gid)
	core.found[0].Open = false
	syncShadows(ctx, db, m, spec)
	var parts int
	db.QueryRow(`SELECT cardinality(g.participants), ms.open_participation FROM matchmaking.gatherings g JOIN matchmaking.matchmake_sessions ms ON ms.id=g.id WHERE g.id=$1`, gid).Scan(&parts, &open)
	if parts != 1 || open {
		t.Fatalf("after update: participants=%d open=%v", parts, open)
	}
	// the room is gone: shadow dropped; cross off: every shadow dropped
	core.found = nil
	syncShadows(ctx, db, m, spec)
	db.QueryRow(`SELECT count(*) FROM matchmaking.gatherings`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d gatherings after the room ended", n)
	}
	core.found = []*mm.Session{{Id: "ps3-room-2", Title: &mm.TitleRef{Network: "np"}, OwnerId: "acc-ps3", MaxMembers: 8, Open: true}}
	syncShadows(ctx, db, m, spec)
	spec.Cross = nil
	syncShadows(ctx, db, m, spec)
	db.QueryRow(`SELECT count(*) FROM openpak_crossplay.shadow_gatherings`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d shadows with cross off", n)
	}
}
