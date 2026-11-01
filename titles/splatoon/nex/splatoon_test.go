package nex

import (
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	nexgo "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	_ "github.com/lib/pq"
)

// DB tests need a throwaway Postgres:
//
//	podman run --rm -d -e POSTGRES_PASSWORD=check -p 23197:5432 postgres:16-alpine
//	SPLATOON_TEST_DATABASE_URL='postgres://postgres:check@127.0.0.1:23197/postgres?sslmode=disable' go test ./...
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("SPLATOON_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SPLATOON_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`DROP SCHEMA IF EXISTS ranking_splatoon CASCADE`,
		`DROP SCHEMA IF EXISTS matchmaking CASCADE`,
		`CREATE SCHEMA matchmaking`,
		// the shapes common-go creates (match-making/protocol.go, matchmake-extension/protocol.go)
		`CREATE TABLE matchmaking.gatherings (id bigserial PRIMARY KEY, registered boolean NOT NULL DEFAULT true,
			type text NOT NULL DEFAULT '', started_time timestamp, participants numeric(20)[] NOT NULL DEFAULT array[]::numeric(20)[])`,
		`CREATE TABLE matchmaking.matchmake_sessions (id bigserial PRIMARY KEY, game_mode bigint, open_participation boolean)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func streams() (*nexgo.LibraryVersions, *nexgo.ByteStreamSettings) {
	lv := nexgo.NewLibraryVersions()
	lv.SetDefault(nexgo.NewLibraryVersion(3, 8, 15))
	s := nexgo.NewByteStreamSettings()
	s.UseStructureHeader = true
	return lv, s
}

func TestUploadParamRoundTrip(t *testing.T) {
	lv, st := streams()
	in := NewCompetitionRankingUploadScoreParam()
	in.FestID, in.Score, in.TeamID, in.TeamScore, in.IsFirstUpload = 77, 1234, 1, 3, true
	in.AppData = types.NewQBuffer([]byte{1, 2, 3})
	out := nexgo.NewByteStreamOut(lv, st)
	in.WriteTo(out)

	got := NewCompetitionRankingUploadScoreParam()
	if err := got.ExtractFrom(nexgo.NewByteStreamIn(out.Bytes(), lv, st)); err != nil {
		t.Fatal(err)
	}
	if !got.Equals(in) {
		t.Fatalf("round trip changed the param: %+v vs %+v", got, in)
	}
}

func TestGetParamAndInfoRoundTrip(t *testing.T) {
	lv, st := streams()
	p := NewCompetitionRankingGetParam()
	p.FestivalIDs = types.List[types.UInt32]{4, 5}
	p.ResultRange.Offset, p.ResultRange.Length = 10, 20
	out := nexgo.NewByteStreamOut(lv, st)
	p.WriteTo(out)
	got := NewCompetitionRankingGetParam()
	if err := got.ExtractFrom(nexgo.NewByteStreamIn(out.Bytes(), lv, st)); err != nil {
		t.Fatal(err)
	}
	if len(got.FestivalIDs) != 2 || got.FestivalIDs[1] != 5 || got.ResultRange.Length != 20 {
		t.Fatalf("param lost data: %+v", got)
	}

	info := NewCompetitionRankingScoreInfo()
	info.FestID = 9
	d := NewCompetitionRankingScoreData()
	d.PID, d.Score = 1800000001, 50
	info.ScoreData = append(info.ScoreData, d)
	info.TeamWins = types.List[types.UInt32]{3, 4}
	info.TeamVotes = types.List[types.UInt32]{5, 6}
	out = nexgo.NewByteStreamOut(lv, st)
	info.WriteTo(out)
	back := NewCompetitionRankingScoreInfo()
	if err := back.ExtractFrom(nexgo.NewByteStreamIn(out.Bytes(), lv, st)); err != nil {
		t.Fatal(err)
	}
	if !back.Equals(info) {
		t.Fatalf("info round trip changed: %+v", back)
	}
}

func TestFestIDsFallsBackToOffset(t *testing.T) {
	p := NewCompetitionRankingGetParam()
	p.FestivalIDs = types.List[types.UInt32]{7}
	if ids, paged := festIDs(p); len(ids) != 1 || ids[0] != 7 || !paged {
		t.Fatalf("listed id: %v %v", ids, paged)
	}
	p = NewCompetitionRankingGetParam()
	p.ResultRange.Offset = 12
	if ids, paged := festIDs(p); len(ids) != 1 || ids[0] != 12 || paged {
		t.Fatalf("offset id: %v %v", ids, paged)
	}
}

func TestFestTallyAndScoreList(t *testing.T) {
	db := testDB(t)
	s := festStore{db: db}
	if err := s.setup(); err != nil {
		t.Fatal(err)
	}
	if err := s.setup(); err != nil { // idempotent
		t.Fatal(err)
	}
	up := func(pid uint64, team uint8, score, teamScore uint32, first bool) {
		p := NewCompetitionRankingUploadScoreParam()
		p.FestID, p.TeamID, p.Score, p.TeamScore, p.IsFirstUpload = 31, types.UInt8(team), types.UInt32(score), types.UInt32(teamScore), types.Bool(first)
		if err := s.record(pid, p); err != nil {
			t.Fatal(err)
		}
	}
	up(1, 0, 100, 1, true)
	up(2, 0, 300, 0, true)
	up(3, 1, 200, 1, true)
	up(1, 0, 90, 1, false) // lower score: best kept
	up(1, 0, 150, 1, false)

	tal, err := s.tally(31)
	if err != nil {
		t.Fatal(err)
	}
	if tal.Votes != [2]uint32{2, 1} || tal.Wins != [2]uint32{3, 1} || tal.Players != 3 {
		t.Fatalf("tally = %+v", tal)
	}

	info, err := s.info(31, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.ScoreData) != 3 || info.ScoreData[0].PID != 2 || info.ScoreData[0].Score != 300 ||
		info.ScoreData[1].Score != 200 || info.ScoreData[2].Score != 150 || info.ScoreData[2].Unknown1 != 3 {
		t.Fatalf("score list = %+v", info.ScoreData)
	}
	if info.TeamVotes[0] != 2 || info.TeamWins[1] != 1 {
		t.Fatalf("team lists = %v %v", info.TeamVotes, info.TeamWins)
	}
	page, err := s.info(31, 1, 1)
	if err != nil || len(page.ScoreData) != 1 || page.ScoreData[0].Score != 200 {
		t.Fatalf("page = %+v %v", page.ScoreData, err)
	}
	if empty, err := s.info(99, 0, 0); err != nil || len(empty.ScoreData) != 0 || empty.TeamVotes[0] != 0 {
		t.Fatalf("empty fest = %+v %v", empty, err)
	}

	bad := NewCompetitionRankingUploadScoreParam()
	bad.TeamID = 5
	if err := s.record(9, bad); err == nil {
		t.Fatal("team 5 accepted")
	}
}

const sampleSchedule = `{"DateTime":"2026-11-01T00:00:00Z","Phases":[
 {"Time":4,"RegularRule":"cPnt","RegularStages":[{"MapID":1},{"MapID":2}],"GachiRule":"cVar","GachiStages":[{"MapID":3},{"MapID":4}]},
 {"Time":4,"RegularRule":"cPnt","RegularStages":[{"MapID":5},{"MapID":6}],"GachiRule":"cVgl","GachiStages":[{"MapID":7},{"MapID":8}]}]}`

func TestScheduleAt(t *testing.T) {
	s, err := ParseSchedule([]byte(sampleSchedule))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	p, i, ok := s.At(base.Add(5 * time.Hour))
	if !ok || i != 1 || p.GachiRule != RuleRainmaker {
		t.Fatalf("at 05:00 = %+v %d %v", p, i, ok)
	}
	if _, _, ok := s.At(base.Add(-time.Minute)); ok {
		t.Fatal("before the schedule matched")
	}
	if _, _, ok := s.At(base.Add(8 * time.Hour)); ok {
		t.Fatal("after the schedule matched")
	}
	rule, stages, ok := p.Stages(gameModeRanked)
	if !ok || rule != RuleRainmaker || len(stages) != 2 || stages[0] != 7 {
		t.Fatalf("ranked stages = %s %v", rule, stages)
	}
	if _, _, ok := p.Stages(gameModeSplatfest); ok {
		t.Fatal("Splatfest has no rotation")
	}
	for _, bad := range []string{`{}`, `{"DateTime":"x","Phases":[{"Time":4}]}`, `{"DateTime":"2026-11-01T00:00:00Z","Phases":[{"Time":0}]}`} {
		if _, err := ParseSchedule([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestRotationSourceReloadsAndKeepsLastGood(t *testing.T) {
	path := t.TempDir() + "/schedule.json"
	src := newRotationSource(path)
	if src.current() != nil {
		t.Fatal("schedule without a file")
	}
	os.WriteFile(path, []byte(sampleSchedule), 0o644)
	if s := src.current(); s == nil || len(s.Phases) != 2 {
		t.Fatal("did not load")
	}
	os.WriteFile(path, []byte(`not json`), 0o644)
	os.Chtimes(path, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	if s := src.current(); s == nil || len(s.Phases) != 2 {
		t.Fatal("a bad file dropped the good schedule")
	}
}

func TestPhaseWatcherClosesOnBoundary(t *testing.T) {
	path := t.TempDir() + "/schedule.json"
	os.WriteFile(path, []byte(sampleSchedule), 0o644)
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	now := base.Add(time.Hour)
	var cutoffs []time.Time
	w := &phaseWatcher{
		src:     newRotationSource(path),
		closeFn: func(c time.Time) (int64, error) { cutoffs = append(cutoffs, c); return 0, nil },
		now:     func() time.Time { return now },
		logf:    func(string, ...any) {},
	}
	w.tick()
	w.tick()
	if len(cutoffs) != 1 || !cutoffs[0].Equal(base) {
		t.Fatalf("first phase: %v", cutoffs)
	}
	now = base.Add(4*time.Hour + time.Second)
	w.tick()
	if len(cutoffs) != 2 || !cutoffs[1].Equal(base.Add(4*time.Hour)) {
		t.Fatalf("boundary: %v", cutoffs)
	}
}

func TestStaleLobbiesAndFestLobbies(t *testing.T) {
	db := testDB(t)
	var mu sync.Mutex
	base := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
	add := func(mode int, started time.Time, open bool, participants string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO matchmaking.gatherings (type, started_time, participants)
			VALUES ('MatchmakeSession', $1, $2::numeric[]) RETURNING id`, started, participants).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO matchmaking.matchmake_sessions (id, game_mode, open_participation) VALUES ($1,$2,$3)`, id, mode, open); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oldRegular := add(0, base.Add(-time.Hour), true, "{1}")
	oldRanked := add(1, base.Add(-time.Minute), true, "{2}")
	newRegular := add(0, base.Add(time.Minute), true, "{3}")
	oldPrivate := add(4, base.Add(-time.Hour), true, "{4}")
	oldFest := add(12, base.Add(-time.Hour), true, "{5}")
	closedFest := add(12, base.Add(time.Minute), false, "{6,7}")
	otherFest := add(12, base.Add(time.Minute), false, "{8}")

	n, err := closeStaleLobbies(db, &mu, base)
	if err != nil || n != 2 {
		t.Fatalf("closed %d, %v", n, err)
	}
	open := func(id int64) bool {
		var o bool
		db.QueryRow(`SELECT open_participation FROM matchmaking.matchmake_sessions WHERE id=$1`, id).Scan(&o)
		return o
	}
	if open(oldRegular) || open(oldRanked) || !open(newRegular) || !open(oldPrivate) || !open(oldFest) {
		t.Fatal("wrong lobbies closed")
	}

	n, err = openFestLobby(db, &mu, 7)
	if err != nil || n != 1 || !open(closedFest) || open(otherFest) {
		t.Fatalf("opened %d, %v (fest open=%v, other fest open=%v)", n, err, open(closedFest), open(otherFest))
	}
}
