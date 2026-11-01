package mk8wiiu

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

type fakeStore struct {
	err    error
	course int
	limit  int
	offset int
}

func (f *fakeStore) Highlights(_ context.Context, _ string, limit, offset int) ([]apiObject, int, error) {
	f.limit, f.offset = limit, offset
	return []apiObject{{ID: 7, Owner: "1800000005", Tags: []string{}}}, 31, f.err
}
func (f *fakeStore) Object(_ context.Context, id uint64) (apiObject, error) {
	if id != 7 {
		return apiObject{}, errAPINotFound
	}
	return apiObject{ID: 7}, f.err
}
func (f *fakeStore) Ghosts(_ context.Context, course int, _ string, limit, offset int) ([]apiObject, int, error) {
	f.course, f.limit, f.offset = course, limit, offset
	return nil, 0, f.err
}
func (f *fakeStore) Rankings(_ context.Context, course, limit, offset int) ([]apiRank, int, error) {
	f.course, f.limit, f.offset = course, limit, offset
	return []apiRank{{Rank: 1, Player: "5", Score: 83456, Time: formatTime(83456)}}, 1, f.err
}
func (f *fakeStore) Courses(context.Context) ([]apiCourse, error) { return nil, f.err }
func (f *fakeStore) PlayerRankings(context.Context, string) ([]apiPlayerRank, error) {
	return nil, f.err
}
func (f *fakeStore) Tournaments(_ context.Context, limit, offset int) ([]apiTournament, int, error) {
	return nil, 0, f.err
}
func (f *fakeStore) Tournament(_ context.Context, id uint32, season int64, _ int) (apiTournament, error) {
	if id != 20001 {
		return apiTournament{}, errAPINotFound
	}
	return apiTournament{ID: id, Season: season}, f.err
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestAPIPaging(t *testing.T) {
	f := &fakeStore{}
	h := newAPI(f, nil)
	w := get(t, h, "/api/v1/mktv?limit=500&offset=40")
	if w.Code != 200 || f.limit != apiMaxLimit || f.offset != 40 {
		t.Fatalf("code %d limit %d offset %d", w.Code, f.limit, f.offset)
	}
	var p apiPage[apiObject]
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.Total != 31 || len(p.Items) != 1 || p.Items[0].Owner != "1800000005" {
		t.Fatalf("page %+v err %v", p, err)
	}
	get(t, h, "/api/v1/mktv")
	if f.limit != apiDefaultLimit {
		t.Fatalf("default limit %d", f.limit)
	}
	for _, bad := range []string{"limit=x", "offset=-1", "limit=-3"} {
		if c := get(t, h, "/api/v1/mktv?"+bad).Code; c != 400 {
			t.Errorf("%s: code %d", bad, c)
		}
	}
}

func TestAPIEmptyListIsArray(t *testing.T) {
	w := get(t, newAPI(&fakeStore{}, nil), "/api/v1/courses/3/ghosts")
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("empty ghosts should be [], got %s", w.Body)
	}
}

func TestAPIRankingsAndCourse(t *testing.T) {
	f := &fakeStore{}
	w := get(t, newAPI(f, nil), "/api/v1/courses/12/rankings?limit=5")
	if w.Code != 200 || f.course != 12 || f.limit != 5 || !strings.Contains(w.Body.String(), `"time":"1:23.456"`) {
		t.Fatalf("code %d course %d body %s", w.Code, f.course, w.Body)
	}
	if c := get(t, newAPI(f, nil), "/api/v1/courses/abc/rankings").Code; c != 400 {
		t.Fatalf("bad course: %d", c)
	}
}

func TestAPINotFoundAndStoreDown(t *testing.T) {
	h := newAPI(&fakeStore{}, nil)
	if c := get(t, h, "/api/v1/objects/9").Code; c != 404 {
		t.Errorf("missing object: %d", c)
	}
	if c := get(t, h, "/api/v1/tournaments/5").Code; c != 404 {
		t.Errorf("missing tournament: %d", c)
	}
	if c := get(t, h, "/api/v1/tournaments/20001?season=0").Code; c != 400 {
		t.Errorf("season 0: %d", c)
	}
	down := newAPI(&fakeStore{err: errors.New("pq: boom")}, nil)
	w := get(t, down, "/api/v1/mktv")
	if w.Code != 503 || strings.Contains(w.Body.String(), "boom") {
		t.Errorf("a store error must be 503 without leaking detail: %d %s", w.Code, w.Body)
	}
}

func TestAPIOwnerAndPlayer(t *testing.T) {
	h := newAPI(&fakeStore{}, nil)
	for _, bad := range []string{"/api/v1/mktv?owner=x", "/api/v1/courses/1/ghosts?owner=1%3Bdrop", "/api/v1/players/abc/rankings"} {
		if c := get(t, h, bad).Code; c != 400 {
			t.Errorf("%s: %d", bad, c)
		}
	}
	if w := get(t, h, "/api/v1/players/1800000005/rankings"); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Errorf("player rankings: %d %s", w.Code, w.Body)
	}
	if c := get(t, h, "/api/v1/mktv?owner=1800000005").Code; c != 200 {
		t.Errorf("owner filter: %d", c)
	}
}

func TestAPIReadOnly(t *testing.T) {
	w := httptest.NewRecorder()
	newAPI(&fakeStore{}, nil).ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/mktv", nil))
	if w.Code != 405 {
		t.Fatalf("POST: %d", w.Code)
	}
}

func TestAPIBody(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 16))
	body := func(content []byte) apiBodies {
		return func(context.Context, uint64) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(string(content))), nil
		}
	}
	if c := get(t, newAPI(&fakeStore{}, nil), "/api/v1/objects/7/body").Code; c != 501 {
		t.Errorf("no S3: %d", c)
	}
	w := get(t, newAPI(&fakeStore{}, body(png)), "/api/v1/objects/7/body")
	if w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("png served as %q", w.Header().Get("Content-Type"))
	}
	w = get(t, newAPI(&fakeStore{}, body([]byte("<html><script>x</script>"))), "/api/v1/objects/7/body")
	if w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Disposition") != "attachment" {
		t.Errorf("html must never be served as html: %v", w.Header())
	}
	if c := get(t, newAPI(&fakeStore{}, body(png)), "/api/v1/objects/8/body").Code; c != 404 {
		t.Errorf("unlisted object body: %d", c)
	}
}

func TestFormatTime(t *testing.T) {
	for in, want := range map[int64]string{0: "0:00.000", 59999: "0:59.999", 83456: "1:23.456", 600001: "10:00.001", -5: "0:00.000"} {
		if got := formatTime(in); got != want {
			t.Errorf("%d: %q want %q", in, got, want)
		}
	}
}

func TestNexTime(t *testing.T) {
	if nexTime("0") != nil || nexTime("junk") != nil {
		t.Fatal("zero and junk are unset")
	}
	// 2026-10-30 12:34:56 packed: second | minute<<6 | hour<<12 | day<<17 | month<<22 | year<<26
	packed := uint64(56) | 34<<6 | 12<<12 | 30<<17 | 10<<22 | 2026<<26
	got := nexTime(strconv.FormatUint(packed, 10))
	if got == nil || got.Year() != 2026 || got.Month() != 10 || got.Day() != 30 || got.Hour() != 12 {
		t.Fatalf("got %v", got)
	}
}

// TestPostgresAPI runs the real queries against a throwaway database; set MK8_TEST_DATABASE_URL.
func TestPostgresAPI(t *testing.T) {
	url := os.Getenv("MK8_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MK8_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS mk8 CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := newRankingStore(db); err != nil {
		t.Fatal(err)
	}
	if _, err := newTournamentStore(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(datastoreSchema); err != nil {
		t.Fatal(err)
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// 1 highlight, 2 its thumbnail, 3 ghost on course 4, 4 unfinished ghost, 5 private, 6 deleted.
	// Columns: owner, size, name, data_type, refer_data_id, permission, slot, uploaded, deleted.
	ins := `INSERT INTO mk8.datastore_objects (owner_pid, size, name, data_type, permission, refer_data_id, persistence_slot, uploaded, deleted, tags) VALUES ($1,$2,$3,$4,$6,$5,$7,$8,$9,'{a}')`
	mustExec(ins, 1800000005, 100, "hl", 1, 0, 0, 0xFFFF, true, false)
	mustExec(ins, 1800000005, 10, "th", 1, 1, 0, nil, true, false)
	mustExec(ins, 1800000006, 10, "gh", 4, 0, 0, 2, true, false)
	mustExec(ins, 1800000006, 10, "gh2", 4, 0, 0, 3, false, false)
	mustExec(ins, 1800000007, 10, "pv", 1, 0, 3, nil, true, false)
	mustExec(ins, 1800000007, 10, "dl", 1, 0, 0, nil, true, true)
	mustExec(`INSERT INTO mk8.ranking_scores (category, pid, unique_id, score, order_by, param) VALUES (4, 1800000006, 1, 90000, 0, 0), (4, 1800000005, 2, 80000, 0, 0), (5, 1800000005, 3, 70000, 0, 0)`)
	mustExec(`INSERT INTO mk8.tournaments (owner_pid, attributes, community_id, community_code, name, start_datetime, end_datetime) VALUES (1800000005, '{1,2}', 9, '111122223333', 'Cup', 0, 0)`)
	mustExec(`INSERT INTO mk8.tournament_scores (tournament_id, season_id, pid, score, team_id) VALUES (20000, 1, 1800000005, 10, 0), (20000, 1, 1800000006, 30, 0)`)

	p := &pgAPI{db: db, mktvType: -1}
	ctx := context.Background()
	hl, total, err := p.Highlights(ctx, "", 10, 0)
	if err != nil || total != 1 || hl[0].Name != "hl" {
		t.Fatalf("highlights should be the one public finished unreferring non-slot object: %d %+v %v", total, hl, err)
	}
	if hl[0].Thumbnail == nil || hl[0].Slot != nil {
		t.Fatalf("highlight lost its thumbnail or grew a slot: %+v", hl[0])
	}
	if hl[0].YouTubeID != "" {
		t.Fatalf("no video table yet, got %q", hl[0].YouTubeID)
	}
	mustExec(`CREATE TABLE mk8.mktv_videos (data_id bigint PRIMARY KEY, owner_pid numeric(20) NOT NULL, status text NOT NULL, youtube_id text)`)
	mustExec(`INSERT INTO mk8.mktv_videos VALUES (1, 1800000005, 'uploaded', 'dQw4w9WgXcQ'), (2, 1800000005, 'failed', 'zzzzzzzzzzz')`)
	if hl, _, _ = p.Highlights(ctx, "", 10, 0); hl[0].YouTubeID != "dQw4w9WgXcQ" {
		t.Fatalf("uploaded video id missing: %+v", hl[0])
	}
	if o, _ := p.Object(ctx, 2); o.YouTubeID != "" {
		t.Fatalf("a failed upload must not expose an id: %+v", o)
	}
	if _, err := p.Object(ctx, 5); !errors.Is(err, errAPINotFound) {
		t.Fatalf("private object must be hidden: %v", err)
	}
	gh, total, err := p.Ghosts(ctx, 4, "", 10, 0)
	if err != nil || total != 1 || gh[0].Name != "gh" || gh[0].Slot == nil || *gh[0].Slot != 2 {
		t.Fatalf("ghosts: %+v %d %v", gh, total, err)
	}
	rk, total, err := p.Rankings(ctx, 4, 10, 0)
	if err != nil || total != 2 || rk[0].Player != "1800000005" || rk[0].Rank != 1 || rk[1].Time != "1:30.000" {
		t.Fatalf("rankings: %+v %d %v", rk, total, err)
	}
	courses, err := p.Courses(ctx)
	if err != nil || len(courses) != 2 || courses[0] != (apiCourse{Course: 4, Ranked: 2, Ghosts: 1}) || courses[1].Course != 5 {
		t.Fatalf("courses: %+v %v", courses, err)
	}
	if _, total, _ := p.Highlights(ctx, "1800000006", 10, 0); total != 0 {
		t.Fatalf("owner filter: %d", total)
	}
	if _, total, _ := p.Ghosts(ctx, 4, "1800000006", 10, 0); total != 1 {
		t.Fatalf("ghost owner filter: %d", total)
	}
	pr, err := p.PlayerRankings(ctx, "1800000006")
	if err != nil || len(pr) != 1 || pr[0].Rank != 2 || pr[0].Of != 2 || pr[0].Time != "1:30.000" {
		t.Fatalf("player rankings: %+v %v", pr, err)
	}
	tl, total, err := p.Tournaments(ctx, 10, 0)
	if err != nil || total != 1 || tl[0].Name != "Cup" || tl[0].Start != nil {
		t.Fatalf("tournaments: %+v %v", tl, err)
	}
	tt, err := p.Tournament(ctx, tl[0].ID, 0, 10)
	if err != nil || len(tt.Scores) != 2 || tt.Scores[0].Score != 30 || tt.Scores[0].Rank != 1 {
		t.Fatalf("tournament: %+v %v", tt, err)
	}
	if _, err := p.Tournament(ctx, 1, 0, 10); !errors.Is(err, errAPINotFound) {
		t.Fatalf("missing tournament: %v", err)
	}
}
