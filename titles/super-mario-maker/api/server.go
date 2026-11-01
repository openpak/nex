package api

import (
	"crypto/subtle"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	datastore_smm_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore/super-mario-maker"
	nexsmm "github.com/openpak/nex/titles/super-mario-maker/nex/datastore/super-mario-maker"
)

const (
	defaultLimit = 50
	maxLimit     = 200
	// maxBookmarks bounds one maker's bookmark list.
	maxBookmarks = 1000
	// maxPickup is how many course IDs Nintendo's pickup lists held at most (Pretendo issue 21,
	// jonbarrow, from packet dumps).
	maxPickup = 400
)

// Server serves the internal API (Handler) and the Dothub subset (DothubHandler).
type Server struct {
	Store Store
	// Key is the shared secret of the internal API (header X-Internal-Key). Empty disables it.
	Key string
	// Logf receives one line per request the Dothub handler does not know.
	Logf func(format string, args ...any)
}

func (s *Server) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

// Handler is the internal JSON API the course site reads (/v1/...). Every request needs the key.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/courses", s.courses)
	mux.HandleFunc("GET /v1/courses/{id}", s.course)
	mux.HandleFunc("GET /v1/courses/{id}/thumbnail", s.thumbnail)
	mux.HandleFunc("GET /v1/makers", s.makers)
	mux.HandleFunc("GET /v1/bookmarks", s.listBookmarks)
	mux.HandleFunc("PUT /v1/bookmarks", s.putBookmark)
	mux.HandleFunc("DELETE /v1/bookmarks", s.deleteBookmark)
	return s.keyed(mux)
}

func (s *Server) keyed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Key == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Key")), []byte(s.Key)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or wrong key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "store error")
}

func intParam(r *http.Request, name string, def, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || v < 0 {
		return def
	}
	if max > 0 && v > max {
		return max
	}
	return v
}

type wireCourse struct {
	ID           string   `json:"id"`
	DataID       uint64   `json:"data_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	MakerID      string   `json:"maker_id"`
	MakerName    string   `json:"maker_name"`
	Style        string   `json:"style"`
	Theme        string   `json:"theme"`
	Difficulty   string   `json:"difficulty"`
	Tags         []string `json:"tags"`
	Created      string   `json:"created"`
	Updated      string   `json:"updated"`
	Stars        int      `json:"stars"`
	Plays        int      `json:"plays"`
	Clears       int      `json:"clears"`
	Attempts     int      `json:"attempts"`
	Failures     int      `json:"failures"`
	Comments     int      `json:"miiverse_comments"`
	HasThumbnail bool     `json:"has_thumbnail"`
}

func toWire(c Course) wireCourse {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	return wireCourse{
		ID: CourseCode(c.DataID), DataID: c.DataID, Name: c.Name,
		MakerID: strconv.Itoa(int(c.OwnerPID)), MakerName: c.MakerName,
		// * Style and theme live inside the course file, which the server does not parse; the
		// * difficulty is the in-game band of the failure rate (clears over attempts)
		Difficulty: string(nexsmm.DifficultyOf(datastore_smm_db.FailureRate(int64(c.Clears), int64(c.Attempts)))),
		Tags:       tags, Created: c.Created.UTC().Format(time.RFC3339), Updated: c.Updated.UTC().Format(time.RFC3339),
		Stars: c.Stars, Plays: c.Plays, Clears: c.Clears, Attempts: c.Attempts, Failures: c.Failures,
		Comments: c.Comments, HasThumbnail: c.HasThumbnail,
	}
}

func (s *Server) courses(w http.ResponseWriter, r *http.Request) {
	q := CourseQuery{
		Sort:   r.URL.Query().Get("sort"),
		Limit:  intParam(r, "limit", defaultLimit, maxLimit),
		Offset: intParam(r, "offset", 0, 0),
		Name:   strings.TrimSpace(r.URL.Query().Get("q")),
	}
	switch q.Sort {
	case "", SortNewest, SortStars, SortPlays, SortClears, SortRandom:
	default:
		writeError(w, http.StatusBadRequest, "unknown sort")
		return
	}
	if v := r.URL.Query().Get("updated_since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad updated_since")
			return
		}
		q.UpdatedSince = t
	}
	if v := r.URL.Query().Get("maker"); v != "" {
		pid, err := strconv.ParseInt(v, 10, 32)
		if err != nil || pid <= 0 {
			writeError(w, http.StatusBadRequest, "bad maker")
			return
		}
		q.OwnerPID = int32(pid)
	}
	if v := r.URL.Query().Get("difficulty"); v != "" {
		lo, hi, ok := nexsmm.FailureRange(nexsmm.Difficulty(v))
		if !ok {
			writeError(w, http.StatusBadRequest, "difficulty is easy, normal, expert or super_expert")
			return
		}
		q.FailMin, q.FailMax = &lo, &hi
	}

	list, total, err := s.Store.Courses(r.Context(), q)
	if err != nil {
		storeError(w, err)
		return
	}
	out := make([]wireCourse, 0, len(list))
	for _, c := range list {
		out = append(out, toWire(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "courses": out})
}

func (s *Server) course(w http.ResponseWriter, r *http.Request) {
	id, ok := ParseCourseCode(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not a course id")
		return
	}
	c, err := s.Store.Course(r.Context(), id)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toWire(c))
}

func (s *Server) thumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := ParseCourseCode(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not a course id")
		return
	}
	b, err := s.Store.Thumbnail(r.Context(), id)
	if err != nil {
		storeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(b)
}

type wireMaker struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Courses int    `json:"courses"`
	Stars   int    `json:"stars"`
	Plays   int    `json:"plays"`
	Medals  int    `json:"medals"`
	Created string `json:"created"`
}

func (s *Server) makers(w http.ResponseWriter, r *http.Request) {
	list, total, err := s.Store.Makers(r.Context(), intParam(r, "limit", defaultLimit, maxLimit), intParam(r, "offset", 0, 0))
	if err != nil {
		storeError(w, err)
		return
	}
	out := make([]wireMaker, 0, len(list))
	for _, m := range list {
		out = append(out, wireMaker{
			ID: strconv.Itoa(int(m.PID)), Name: m.Name, Courses: m.Courses, Stars: m.Stars, Plays: m.Plays,
			Medals: Medals(m.Stars), Created: m.Created.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "makers": out})
}

// bookmarkRef is a maker (by PID or by the NNID the game uploaded as its maker profile) and a course.
type bookmarkRef struct {
	PID    int32  `json:"pid"`
	NNID   string `json:"nnid"`
	Course string `json:"course"`
}

func (s *Server) owner(r *http.Request, ref bookmarkRef) (int32, int, string) {
	if ref.PID > 0 {
		return ref.PID, 0, ""
	}
	if ref.NNID == "" {
		return 0, http.StatusBadRequest, "pid or nnid is required"
	}
	pid, err := s.Store.ResolveNNID(r.Context(), ref.NNID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, http.StatusNotFound, "no maker with that name has played the game yet"
		}
		return 0, http.StatusInternalServerError, "store error"
	}
	return pid, 0, ""
}

func (s *Server) listBookmarks(w http.ResponseWriter, r *http.Request) {
	ref := bookmarkRef{NNID: r.URL.Query().Get("nnid")}
	if v, err := strconv.ParseInt(r.URL.Query().Get("pid"), 10, 32); err == nil {
		ref.PID = int32(v)
	}
	pid, status, msg := s.owner(r, ref)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	ids, err := s.Store.Bookmarks(r.Context(), pid)
	if err != nil {
		storeError(w, err)
		return
	}
	out := make([]wireCourse, 0, len(ids))
	for _, id := range ids {
		c, err := s.Store.Course(r.Context(), id)
		if err != nil {
			continue // deleted or hidden since it was bookmarked
		}
		out = append(out, toWire(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"pid": pid, "courses": out})
}

func (s *Server) readRef(w http.ResponseWriter, r *http.Request) (int32, uint64, bool) {
	var ref bookmarkRef
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&ref); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return 0, 0, false
	}
	id, ok := ParseCourseCode(ref.Course)
	if !ok {
		writeError(w, http.StatusBadRequest, "course is not a course id")
		return 0, 0, false
	}
	pid, status, msg := s.owner(r, ref)
	if status != 0 {
		writeError(w, status, msg)
		return 0, 0, false
	}
	return pid, id, true
}

func (s *Server) putBookmark(w http.ResponseWriter, r *http.Request) {
	pid, id, ok := s.readRef(w, r)
	if !ok {
		return
	}
	if _, err := s.Store.Course(r.Context(), id); err != nil {
		storeError(w, err)
		return
	}
	have, err := s.Store.Bookmarks(r.Context(), pid)
	if err != nil {
		storeError(w, err)
		return
	}
	if len(have) >= maxBookmarks {
		already := false
		for _, h := range have {
			already = already || h == id
		}
		if !already {
			writeError(w, http.StatusConflict, fmt.Sprintf("a maker can bookmark at most %d courses", maxBookmarks))
			return
		}
	}
	if err := s.Store.AddBookmark(r.Context(), pid, id); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pid": pid, "course": CourseCode(id)})
}

func (s *Server) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	pid, id, ok := s.readRef(w, r)
	if !ok {
		return
	}
	if err := s.Store.RemoveBookmark(r.Context(), pid, id); err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pid": pid, "course": CourseCode(id)})
}

// DothubHandler is what the game's HTTP side needs from wup-ama.app.nintendo.net ("Dothub", the
// service the Bookmark site shared with the game). Only the pickup list is known:
//
//	GET /api/v1/pickup/{easy|normal|expert|super_expert}
//	<root><courses><course><id>DECIMAL_DATA_ID</id></course>...</courses></root>
//
// at most 400 entries (Pretendo issue 21, jonbarrow, from packet dumps). Anything else is logged
// in full and answered 404, so a first console session shows what else the game asks for.
func (s *Server) DothubHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/pickup/{difficulty}", s.pickup)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		s.logf("dothub unhandled: %s %s content-type=%q body=%q", r.Method, r.URL.RequestURI(), r.Header.Get("Content-Type"), body)
		http.NotFound(w, r)
	})
	return mux
}

type pickupXML struct {
	XMLName xml.Name `xml:"root"`
	Courses struct {
		Course []pickupCourse `xml:"course"`
	} `xml:"courses"`
}

type pickupCourse struct {
	ID uint64 `xml:"id"`
}

func (s *Server) pickup(w http.ResponseWriter, r *http.Request) {
	lo, hi, ok := nexsmm.FailureRange(nexsmm.Difficulty(r.PathValue("difficulty")))
	if !ok {
		http.NotFound(w, r)
		return
	}
	ids, err := s.Store.Pickup(r.Context(), lo, hi, maxPickup)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	var doc pickupXML
	doc.Courses.Course = make([]pickupCourse, 0, len(ids))
	for _, id := range ids {
		doc.Courses.Course = append(doc.Courses.Course, pickupCourse{ID: id})
	}
	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, xml.Header)
	_ = xml.NewEncoder(w).Encode(doc)
}
