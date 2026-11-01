package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu        sync.Mutex
	courses   map[uint64]Course
	bookmarks map[int32][]uint64
	nnids     map[string]int32
	lastQ     CourseQuery
}

func newFake() *fakeStore {
	return &fakeStore{
		courses: map[uint64]Course{
			943270: {DataID: 943270, Name: "Easy one", OwnerPID: 7, MakerName: "mari0", Stars: 3, Plays: 10, Clears: 9, Attempts: 10, HasThumbnail: true, Created: time.Unix(1700000000, 0), Updated: time.Unix(1700000100, 0)},
			950491: {DataID: 950491, Name: "Hard one", OwnerPID: 8, Stars: 60, Clears: 1, Attempts: 100},
		},
		bookmarks: map[int32][]uint64{},
		nnids:     map[string]int32{"luigi": 9},
	}
}

func (f *fakeStore) Courses(_ context.Context, q CourseQuery) ([]Course, int, error) {
	f.lastQ = q
	var out []Course
	for _, c := range f.courses {
		out = append(out, c)
	}
	return out, len(out), nil
}
func (f *fakeStore) Course(_ context.Context, id uint64) (Course, error) {
	if c, ok := f.courses[id]; ok {
		return c, nil
	}
	return Course{}, ErrNotFound
}
func (f *fakeStore) Thumbnail(_ context.Context, id uint64) ([]byte, error) {
	if c, ok := f.courses[id]; ok && c.HasThumbnail {
		return []byte("\xff\xd8jpeg"), nil
	}
	return nil, ErrNotFound
}
func (f *fakeStore) Makers(context.Context, int, int) ([]Maker, int, error) {
	return []Maker{{PID: 7, Name: "mari0", Courses: 1, Stars: 60, Plays: 10, Created: time.Unix(1700000000, 0)}}, 1, nil
}
func (f *fakeStore) Maker(_ context.Context, pid int32) (Maker, error) {
	if pid == 7 {
		return Maker{PID: 7, Name: "mari0", Courses: 1, Stars: 60}, nil
	}
	return Maker{}, ErrNotFound
}
func (f *fakeStore) Pickup(_ context.Context, lo, hi, limit int) ([]uint64, error) {
	if lo == 96 {
		return []uint64{950491, 943270}, nil
	}
	return nil, nil
}
func (f *fakeStore) AddBookmark(_ context.Context, pid int32, id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range f.bookmarks[pid] {
		if h == id {
			return nil
		}
	}
	f.bookmarks[pid] = append([]uint64{id}, f.bookmarks[pid]...)
	return nil
}
func (f *fakeStore) RemoveBookmark(_ context.Context, pid int32, id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keep []uint64
	for _, h := range f.bookmarks[pid] {
		if h != id {
			keep = append(keep, h)
		}
	}
	f.bookmarks[pid] = keep
	return nil
}
func (f *fakeStore) Bookmarks(_ context.Context, pid int32) ([]uint64, error) {
	return f.bookmarks[pid], nil
}
func (f *fakeStore) ResolveNNID(_ context.Context, n string) (int32, error) {
	if p, ok := f.nnids[strings.ToLower(n)]; ok {
		return p, nil
	}
	return 0, ErrNotFound
}

func do(t *testing.T, h http.Handler, method, path, body string, key string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

func TestKeyRequired(t *testing.T) {
	s := &Server{Store: newFake(), Key: "k"}
	if rec, _ := do(t, s.Handler(), "GET", "/v1/courses", "", ""); rec.Code != 401 {
		t.Errorf("no key: %d", rec.Code)
	}
	if rec, _ := do(t, s.Handler(), "GET", "/v1/courses", "", "bad"); rec.Code != 401 {
		t.Errorf("bad key: %d", rec.Code)
	}
	if rec, _ := do(t, (&Server{Store: newFake()}).Handler(), "GET", "/v1/courses", "", ""); rec.Code != 401 {
		t.Errorf("no key configured must refuse everything: %d", rec.Code)
	}
}

func TestCourseFieldsAndDifficulty(t *testing.T) {
	s := &Server{Store: newFake(), Key: "k"}
	rec, m := do(t, s.Handler(), "GET", "/v1/courses/4348-0000-000E-64A6", "", "k")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if m["id"] != "4348-0000-000E-64A6" || m["data_id"].(float64) != 943270 || m["maker_id"] != "7" || m["difficulty"] != "easy" {
		t.Errorf("course = %v", m)
	}
	_, m = do(t, s.Handler(), "GET", "/v1/courses/950491", "", "k")
	if m["difficulty"] != "super_expert" {
		t.Errorf("1 clear in 100 attempts is super expert, got %v", m["difficulty"])
	}
	if rec, _ := do(t, s.Handler(), "GET", "/v1/courses/1", "", "k"); rec.Code != 404 {
		t.Errorf("unknown course: %d", rec.Code)
	}
}

func TestListFilters(t *testing.T) {
	f := newFake()
	s := &Server{Store: f, Key: "k"}
	if rec, _ := do(t, s.Handler(), "GET", "/v1/courses?difficulty=expert&sort=stars&q=ea&maker=7&limit=5", "", "k"); rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	q := f.lastQ
	if q.FailMin == nil || *q.FailMin != 75 || *q.FailMax != 95 || q.Sort != "stars" || q.Name != "ea" || q.OwnerPID != 7 || q.Limit != 5 {
		t.Errorf("query = %+v", q)
	}
	for _, bad := range []string{"?difficulty=hard", "?sort=sideways", "?updated_since=yesterday", "?maker=x"} {
		if rec, _ := do(t, s.Handler(), "GET", "/v1/courses"+bad, "", "k"); rec.Code != 400 {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	_, m := do(t, s.Handler(), "GET", "/v1/courses?limit=100000", "", "k")
	if m["total"].(float64) != 2 || f.lastQ.Limit != maxLimit {
		t.Errorf("limit must be clamped: %d", f.lastQ.Limit)
	}
}

func TestMakersCarryMedals(t *testing.T) {
	s := &Server{Store: newFake(), Key: "k"}
	_, m := do(t, s.Handler(), "GET", "/v1/makers", "", "k")
	mk := m["makers"].([]any)[0].(map[string]any)
	if mk["medals"].(float64) != 2 { // 60 stars: 1st (1) and 2nd (50) medals
		t.Errorf("maker = %v", mk)
	}
}

func TestSingleMaker(t *testing.T) {
	h := (&Server{Store: newFake(), Key: "k"}).Handler()
	_, m := do(t, h, "GET", "/v1/makers/7", "", "k")
	if m["id"] != "7" || m["name"] != "mari0" || m["medals"].(float64) != 2 {
		t.Errorf("maker = %v", m)
	}
	for path, want := range map[string]int{"/v1/makers/8": 404, "/v1/makers/x": 404} {
		if rec, _ := do(t, h, "GET", path, "", "k"); rec.Code != want {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

func TestThumbnail(t *testing.T) {
	s := &Server{Store: newFake(), Key: "k"}
	rec, _ := do(t, s.Handler(), "GET", "/v1/courses/943270/thumbnail", "", "k")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("thumbnail %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec, _ := do(t, s.Handler(), "GET", "/v1/courses/950491/thumbnail", "", "k"); rec.Code != 404 {
		t.Errorf("no thumbnail: %d", rec.Code)
	}
}

func TestBookmarkRoundTrip(t *testing.T) {
	f := newFake()
	s := &Server{Store: f, Key: "k"}
	h := s.Handler()

	// * By NNID, case-insensitive; the same course twice is one bookmark
	for i := 0; i < 2; i++ {
		if rec, _ := do(t, h, "PUT", "/v1/bookmarks", `{"nnid":"Luigi","course":"4348-0000-000E-64A6"}`, "k"); rec.Code != 200 {
			t.Fatalf("put %d: %s", rec.Code, rec.Body)
		}
	}
	if rec, _ := do(t, h, "PUT", "/v1/bookmarks", `{"pid":9,"course":"950491"}`, "k"); rec.Code != 200 {
		t.Fatalf("put by pid: %s", rec.Body)
	}
	_, m := do(t, h, "GET", "/v1/bookmarks?nnid=luigi", "", "k")
	list := m["courses"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["data_id"].(float64) != 950491 {
		t.Fatalf("list = %v", m)
	}

	if rec, _ := do(t, h, "DELETE", "/v1/bookmarks", `{"pid":9,"course":"950491"}`, "k"); rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	_, m = do(t, h, "GET", "/v1/bookmarks?pid=9", "", "k")
	if len(m["courses"].([]any)) != 1 {
		t.Errorf("after delete: %v", m)
	}

	for body, want := range map[string]int{
		`{"nnid":"nobody","course":"943270"}`:   404, // never played, no maker profile
		`{"pid":9,"course":"31337"}`:            404, // no such course
		`{"pid":9,"course":"4348-0000-000E-0"}`: 400,
		`{"course":"943270"}`:                   400,
		`not json`:                              400,
	} {
		if rec, _ := do(t, h, "PUT", "/v1/bookmarks", body, "k"); rec.Code != want {
			t.Errorf("%s: %d, want %d", body, rec.Code, want)
		}
	}
}

func TestBookmarkCap(t *testing.T) {
	f := newFake()
	for i := 0; i < maxBookmarks; i++ {
		f.bookmarks[9] = append(f.bookmarks[9], uint64(1000000+i))
	}
	h := (&Server{Store: f, Key: "k"}).Handler()
	if rec, _ := do(t, h, "PUT", "/v1/bookmarks", `{"pid":9,"course":"943270"}`, "k"); rec.Code != 409 {
		t.Errorf("full list: %d", rec.Code)
	}
}

func TestDothubPickup(t *testing.T) {
	var logged []string
	s := &Server{Store: newFake(), Logf: func(f string, a ...any) { logged = append(logged, f) }}
	h := s.DothubHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/pickup/super_expert", nil))
	b, _ := io.ReadAll(rec.Body)
	want := `<root><courses><course><id>950491</id></course><course><id>943270</id></course></courses></root>`
	if rec.Code != 200 || !strings.Contains(string(b), want) {
		t.Fatalf("pickup %d: %s", rec.Code, b)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/pickup/easy", nil))
	if !strings.Contains(rec.Body.String(), "<courses></courses>") {
		t.Errorf("empty pickup: %s", rec.Body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/pickup/impossible", nil))
	if rec.Code != 404 {
		t.Errorf("unknown difficulty: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/cleared_courses", strings.NewReader("<x/>")))
	if rec.Code != 404 || len(logged) != 1 {
		t.Errorf("an unknown Dothub request must be logged and refused: %d %v", rec.Code, logged)
	}
}
