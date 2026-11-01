package nex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"github.com/openpak/nex/titles/mario-kart-7/database"
)

type fakeStore struct {
	err                   error
	course, limit, offset int
}

func (f *fakeStore) Object(_ context.Context, id uint64) (apiObject, error) {
	if id != 7 {
		return apiObject{}, errAPINotFound
	}
	return apiObject{ID: 7}, f.err
}
func (f *fakeStore) Ghosts(_ context.Context, course int, _ string, limit, offset int) ([]apiObject, int, error) {
	f.course, f.limit, f.offset = course, limit, offset
	return []apiObject{{ID: 7, Owner: "1800000005", Tags: []string{}}}, 31, f.err
}
func (f *fakeStore) Rankings(_ context.Context, course, limit, offset int) ([]apiRank, int, error) {
	f.course, f.limit, f.offset = course, limit, offset
	return []apiRank{{Rank: 1, Player: "5", Score: 83456, TimeMS: 83456, Time: formatTime(83456)}}, 1, f.err
}
func (f *fakeStore) Courses(context.Context) ([]apiCourse, error) { return nil, f.err }
func (f *fakeStore) PlayerRankings(context.Context, string) ([]apiPlayerRank, error) {
	return nil, f.err
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
	w := get(t, h, "/api/v1/courses/3/ghosts?limit=500&offset=40")
	if w.Code != 200 || f.limit != apiMaxLimit || f.offset != 40 || f.course != 3 {
		t.Fatalf("code %d limit %d offset %d", w.Code, f.limit, f.offset)
	}
	var p apiPage[apiObject]
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.Total != 31 || len(p.Items) != 1 || p.Items[0].Owner != "1800000005" {
		t.Fatalf("page %+v err %v", p, err)
	}
	get(t, h, "/api/v1/courses/3/ghosts")
	if f.limit != apiDefaultLimit {
		t.Fatalf("default limit %d", f.limit)
	}
	for _, bad := range []string{"limit=x", "offset=-1", "limit=-3"} {
		if c := get(t, h, "/api/v1/courses/3/ghosts?"+bad).Code; c != 400 {
			t.Errorf("%s: code %d", bad, c)
		}
	}
}

func TestAPIEmptyListIsArray(t *testing.T) {
	w := get(t, newAPI(&fakeStore{}, nil), "/api/v1/courses")
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("empty courses should be [], got %s", w.Body)
	}
}

func TestAPIRankingsAndCourse(t *testing.T) {
	f := &fakeStore{}
	w := get(t, newAPI(f, nil), "/api/v1/courses/12/rankings?limit=5")
	if w.Code != 200 || f.course != 12 || f.limit != 5 || !strings.Contains(w.Body.String(), `"time":"1:23.456"`) || !strings.Contains(w.Body.String(), `"time_ms":83456`) {
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
	if c := get(t, h, "/api/v1/tournaments").Code; c != 404 {
		t.Errorf("MK7 has no tournaments, got %d", c)
	}
	down := newAPI(&fakeStore{err: errors.New("pq: boom")}, nil)
	w := get(t, down, "/api/v1/courses/1/ghosts")
	if w.Code != 503 || strings.Contains(w.Body.String(), "boom") {
		t.Errorf("a store error must be 503 without leaking detail: %d %s", w.Code, w.Body)
	}
}

func TestAPIOwnerAndPlayer(t *testing.T) {
	h := newAPI(&fakeStore{}, nil)
	for _, bad := range []string{"/api/v1/courses/1/ghosts?owner=x", "/api/v1/courses/1/ghosts?owner=1%3Bdrop", "/api/v1/players/abc/rankings"} {
		if c := get(t, h, bad).Code; c != 400 {
			t.Errorf("%s: %d", bad, c)
		}
	}
	if w := get(t, h, "/api/v1/players/1800000005/rankings"); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Errorf("player rankings: %d %s", w.Code, w.Body)
	}
	if c := get(t, h, "/api/v1/courses/1/ghosts?owner=1800000005").Code; c != 200 {
		t.Errorf("owner filter: %d", c)
	}
}

func TestAPIReadOnly(t *testing.T) {
	w := httptest.NewRecorder()
	newAPI(&fakeStore{}, nil).ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/courses", nil))
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
	w = get(t, newAPI(&fakeStore{}, body([]byte("ghost\x00bytes"))), "/api/v1/objects/7/body")
	if w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Disposition") != "attachment" {
		t.Errorf("a ghost must be an attachment: %v", w.Header())
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

// TestPostgres runs the real queries (ranking store, DataStore schema, read API) against a
// throwaway database; set MK7_TEST_DATABASE_URL.
func TestPostgres(t *testing.T) {
	url := os.Getenv("MK7_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MK7_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS mk7 CASCADE`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{database.RankingSchema, database.DatastoreSchema, database.RankingSchema} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	rk := database.Ranking{DB: db}
	if ok, err := rk.UploadScore(1800000006, 6, 4, []uint32{90000, 1}, 0, 0); err != nil || !ok {
		t.Fatalf("first upload: %v %v", ok, err)
	}
	rk.UploadScore(1800000005, 5, 4, []uint32{80000, 1}, 0, 0)
	rk.UploadScore(1800000007, 7, 4, []uint32{85000, 1}, 0, 0)
	rk.UploadScore(1800000005, 5, 5, []uint32{70000, 1}, 0, 0)
	if ok, _ := rk.UploadScore(1800000006, 6, 4, []uint32{95000, 2}, 0, 0); ok {
		t.Fatal("a slower time replaced the best one")
	}
	if ok, _ := rk.UploadScore(1800000006, 6, 4, []uint32{88000, 3}, 0, 0); !ok {
		t.Fatal("a faster time was ignored")
	}
	if err := rk.UploadCommonData(1800000005, 5, []byte("mii")); err != nil {
		t.Fatal(err)
	}
	page, err := rk.Page(4, 0, false, 0, 10)
	if err != nil || len(page) != 3 || page[0].PID != 1800000005 || page[0].Rank != 1 || string(page[0].CommonData) != "mii" ||
		page[1].PID != 1800000007 || page[2].Scores[0] != 88000 || page[2].Rank != 3 {
		t.Fatalf("page: %+v %v", page, err)
	}
	if page, _ = rk.Page(4, 0, false, 1, 1); len(page) != 1 || page[0].Rank != 2 {
		t.Fatalf("offset page: %+v", page)
	}
	if pos, ok, err := rk.Position(1800000007, 4, 0, false); err != nil || !ok || pos != 1 {
		t.Fatalf("position: %d %v %v", pos, ok, err)
	}
	if _, ok, _ := rk.Position(1800000099, 4, 0, false); ok {
		t.Fatal("absent player has a position")
	}
	if n, _ := rk.Total(4); n != 3 {
		t.Fatalf("total %d", n)
	}
	if data, _ := rk.CommonData(5); string(data) != "mii" {
		t.Fatalf("common data %q", data)
	}
	if err := rk.DeleteScore(1800000005, 5); err != nil {
		t.Fatal(err)
	}
	if n, _ := rk.Total(5); n != 0 {
		t.Fatalf("delete left %d", n)
	}

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// ghost on course 4 (slot 2), unfinished ghost, private object, deleted object, slotless object.
	ins := `INSERT INTO mk7.datastore_objects (owner_pid, size, name, data_type, permission, persistence_slot, uploaded, deleted, tags) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'{a}')`
	mustExec(ins, 1800000006, 10, "gh", 4, 0, 2, true, false)
	mustExec(ins, 1800000006, 10, "gh2", 4, 0, 3, false, false)
	mustExec(ins, 1800000007, 10, "pv", 4, 3, 1, true, false)
	mustExec(ins, 1800000007, 10, "dl", 4, 0, 1, true, true)
	mustExec(ins, 1800000007, 10, "nos", 4, 0, nil, true, false)

	p := &pgAPI{db: db}
	ctx := context.Background()
	gh, total, err := p.Ghosts(ctx, 4, "", 10, 0)
	if err != nil || total != 1 || gh[0].Name != "gh" || gh[0].Slot == nil || *gh[0].Slot != 2 {
		t.Fatalf("ghosts: %+v %d %v", gh, total, err)
	}
	if gh[0].Course == nil || *gh[0].Course != 4 || gh[0].TimeMS == nil || *gh[0].TimeMS != 88000 || gh[0].Time != "1:28.000" {
		t.Fatalf("ghost course/time: %+v", gh[0])
	}
	if _, err := p.Object(ctx, 3); !errors.Is(err, errAPINotFound) {
		t.Fatalf("private object must be hidden: %v", err)
	}
	r, total, err := p.Rankings(ctx, 4, 10, 0)
	if err != nil || total != 3 || r[0].Player != "1800000005" || r[0].Rank != 1 || r[2].Time != "1:28.000" {
		t.Fatalf("rankings: %+v %d %v", r, total, err)
	}
	courses, err := p.Courses(ctx)
	if err != nil || len(courses) != 1 || courses[0] != (apiCourse{Course: 4, Ranked: 3, Ghosts: 1}) {
		t.Fatalf("courses: %+v %v", courses, err)
	}
	pr, err := p.PlayerRankings(ctx, "1800000006")
	if err != nil || len(pr) != 1 || pr[0].Rank != 3 || pr[0].Of != 3 || pr[0].Time != "1:28.000" {
		t.Fatalf("player rankings: %+v %v", pr, err)
	}
	if _, total, _ := p.Ghosts(ctx, 4, "1800000099", 10, 0); total != 0 {
		t.Fatalf("owner filter: %d", total)
	}
}
