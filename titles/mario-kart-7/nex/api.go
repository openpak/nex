package nex

// Read-only JSON API over what players of Mario Kart 7 have put on the server: time-trial
// ghosts and the per-course ranking. Same shape as the Mario Kart 8 title's API (the OpenKart
// site consumes both); nothing here writes. Served on MK7_API_ADDR, container network only.
//
// FACTS: the Ranking protocol carries the time-trial board; the DataStore carries ghosts.
// INFERRED (no console has uploaded yet): the ranking category and the ghost object's data_type
// are the course id, and the first score is the time in milliseconds (lower is better). The raw
// data_type, tags and metadata are in every object so the site can be corrected without a
// server change.

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/minio/minio-go/v7"
)

const (
	apiDefaultLimit = 20
	apiMaxLimit     = 100
	// apiMaxBody caps a proxied object body. A ghost or a thumbnail is far below this.
	apiMaxBody = 8 << 20
)

// apiObject is a DataStore object as the API shows it. IDs that can exceed 2^53 are strings.
type apiObject struct {
	ID        uint64    `json:"id"`
	Owner     string    `json:"owner"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	DataType  int       `json:"data_type"`
	Slot      *int      `json:"slot,omitempty"`
	Tags      []string  `json:"tags"`
	MetaSize  int       `json:"meta_size"`
	MetaHex   string    `json:"meta_hex,omitempty"`
	ReferID   uint64    `json:"refer_data_id,omitempty"`
	Thumbnail *uint64   `json:"thumbnail,omitempty"`
	Rating    *apiRated `json:"rating,omitempty"`
	// YouTubeID is part of the shared object shape; Mario Kart 7 has no video feature, so it is
	// always empty (and omitted).
	// empty (and omitted) until then. The site embeds it only when present.
	YouTubeID string `json:"youtube_id,omitempty"`
	// Ghost fields: the course (the object's data_type), and the owner's stored time on that
	// course when the ranking has one. INFERRED, see the header.
	Course  *int      `json:"course,omitempty"`
	TimeMS  *int64    `json:"time_ms,omitempty"`
	Time    string    `json:"time,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type apiRated struct {
	Total int64 `json:"total"`
	Count int64 `json:"count"`
}

type apiRank struct {
	Rank    int       `json:"rank"`
	Player  string    `json:"player"`
	Score   int64     `json:"score"`
	TimeMS  int64     `json:"time_ms"`
	Time    string    `json:"time"`
	Updated time.Time `json:"updated"`
}

// apiPlayerRank is one player's standing on one course.
type apiPlayerRank struct {
	Course int    `json:"course"`
	Rank   int    `json:"rank"`
	Of     int    `json:"of"`
	TimeMS int64  `json:"time_ms"`
	Time   string `json:"time"`
}

type apiCourse struct {
	Course int `json:"course"`
	Ranked int `json:"ranked"`
	Ghosts int `json:"ghosts"`
}

type apiPage[T any] struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
	Items  []T `json:"items"`
}

var errAPINotFound = errors.New("not found")

// apiStore is the data the handlers need. The Postgres implementation is pgAPI.
type apiStore interface {
	Object(ctx context.Context, id uint64) (apiObject, error)
	Ghosts(ctx context.Context, course int, owner string, limit, offset int) ([]apiObject, int, error)
	Rankings(ctx context.Context, course, limit, offset int) ([]apiRank, int, error)
	Courses(ctx context.Context) ([]apiCourse, error)
	PlayerRankings(ctx context.Context, pid string) ([]apiPlayerRank, error)
}

// apiBodies opens an object body; nil when the title has no S3 store configured.
type apiBodies func(ctx context.Context, id uint64) (io.ReadCloser, error)

type apiServer struct {
	store  apiStore
	bodies apiBodies
	mux    *http.ServeMux
}

func newAPI(store apiStore, bodies apiBodies) http.Handler {
	a := &apiServer{store: store, bodies: bodies, mux: http.NewServeMux()}
	a.mux.HandleFunc("GET /api/v1/objects/{id}", a.object)
	a.mux.HandleFunc("GET /api/v1/objects/{id}/body", a.body)
	a.mux.HandleFunc("GET /api/v1/courses", a.courses)
	a.mux.HandleFunc("GET /api/v1/courses/{course}/ghosts", a.ghosts)
	a.mux.HandleFunc("GET /api/v1/courses/{course}/rankings", a.rankings)
	a.mux.HandleFunc("GET /api/v1/players/{pid}/rankings", a.playerRankings)
	return a
}

func (a *apiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	a.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=15")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAPINotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		log.Printf("[mario-kart-7] api: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store unavailable"})
	}
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

// paging reads limit and offset; ok is false after it has already answered 400.
func paging(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	limit, offset = apiDefaultLimit, 0
	for name, dst := range map[string]*int{"limit": &limit, "offset": &offset} {
		if v := r.URL.Query().Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				badRequest(w, name+" must be a non-negative integer")
				return 0, 0, false
			}
			*dst = n
		}
	}
	if limit == 0 {
		limit = apiDefaultLimit
	}
	if limit > apiMaxLimit {
		limit = apiMaxLimit
	}
	return limit, offset, true
}

func pathUint(w http.ResponseWriter, r *http.Request, name string, bits int) (uint64, bool) {
	n, err := strconv.ParseUint(r.PathValue(name), 10, bits)
	if err != nil {
		badRequest(w, name+" must be a number")
		return 0, false
	}
	return n, true
}

// ownerParam reads ?owner= (a PID, digits only); empty means everyone. ok is false after a 400.
func ownerParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	o := r.URL.Query().Get("owner")
	if o == "" {
		return "", true
	}
	if len(o) > 20 || strings.Trim(o, "0123456789") != "" {
		badRequest(w, "owner must be a PID")
		return "", false
	}
	return o, true
}

func (a *apiServer) playerRankings(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	if pid == "" || len(pid) > 20 || strings.Trim(pid, "0123456789") != "" {
		badRequest(w, "pid must be a number")
		return
	}
	items, err := a.store.PlayerRankings(r.Context(), pid)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if items == nil {
		items = []apiPlayerRank{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func page[T any](items []T, total, offset, limit int) apiPage[T] {
	if items == nil {
		items = []T{}
	}
	return apiPage[T]{Total: total, Offset: offset, Limit: limit, Items: items}
}

func (a *apiServer) object(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id", 64)
	if !ok {
		return
	}
	o, err := a.store.Object(r.Context(), id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// body streams an object's bytes from S3. Only objects the store lists (completed, not deleted,
// public) are served, and the type is sniffed so a thumbnail is an image and nothing else is
// ever rendered by a browser.
func (a *apiServer) body(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id", 64)
	if !ok {
		return
	}
	if a.bodies == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this server stores no object bodies"})
		return
	}
	if _, err := a.store.Object(r.Context(), id); err != nil {
		writeAPIError(w, err)
		return
	}
	rc, err := a.bodies(r.Context(), id)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			writeAPIError(w, errAPINotFound)
			return
		}
		writeAPIError(w, err)
		return
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, apiMaxBody))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	ct := http.DetectContentType(data)
	if !strings.HasPrefix(ct, "image/png") && !strings.HasPrefix(ct, "image/jpeg") {
		ct = "application/octet-stream"
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (a *apiServer) courses(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.Courses(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if items == nil {
		items = []apiCourse{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *apiServer) ghosts(w http.ResponseWriter, r *http.Request) {
	course, ok := pathUint(w, r, "course", 31)
	if !ok {
		return
	}
	limit, offset, ok := paging(w, r)
	if !ok {
		return
	}
	owner, ok := ownerParam(w, r)
	if !ok {
		return
	}
	items, total, err := a.store.Ghosts(r.Context(), int(course), owner, limit, offset)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page(items, total, offset, limit))
}

func (a *apiServer) rankings(w http.ResponseWriter, r *http.Request) {
	course, ok := pathUint(w, r, "course", 31)
	if !ok {
		return
	}
	limit, offset, ok := paging(w, r)
	if !ok {
		return
	}
	items, total, err := a.store.Rankings(r.Context(), int(course), limit, offset)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page(items, total, offset, limit))
}

// formatTime renders a time-trial score (milliseconds) the way the game shows it: m'ss"mmm.
func formatTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%d:%02d.%03d", ms/60000, ms/1000%60, ms%1000)
}

// --- Postgres ---

type pgAPI struct {
	db *sql.DB
}

const apiObjectColumnsBase = `o.data_id, o.owner_pid, o.name, o.size, o.data_type, o.persistence_slot, o.tags,
	octet_length(o.meta_binary), o.meta_binary, o.refer_data_id, o.created, o.updated,
	(SELECT t.data_id FROM mk7.datastore_objects t WHERE t.refer_data_id = o.data_id AND t.uploaded AND NOT t.deleted
		ORDER BY t.data_id LIMIT 1),
	r.total, r.count, `

// columns is the select list: the object, its first thumbnail, rating slot 0, and the owner's
// stored time on the object's course (data_type) from the ranking.
func (p *pgAPI) columns(ctx context.Context) string {
	return apiObjectColumnsBase + `NULL::text,
	(SELECT s.scores[1] FROM mk7.ranking_scores s WHERE s.pid = o.owner_pid AND s.category = o.data_type)`
}

const apiObjectFrom = ` FROM mk7.datastore_objects o LEFT JOIN mk7.datastore_ratings r ON r.data_id = o.data_id AND r.slot = 0 `

// visible is the filter every object read shares: finished, not deleted, public.
const apiVisible = `o.uploaded AND NOT o.deleted AND o.permission = 0`

func scanAPIObject(row interface{ Scan(...any) error }) (apiObject, error) {
	var o apiObject
	var slot sql.NullInt64
	var tags pq.StringArray
	var meta []byte
	var thumb sql.NullInt64
	var rtotal, rcount sql.NullInt64
	var id, refer int64
	var owner string
	var yt sql.NullString
	var best sql.NullInt64
	if err := row.Scan(&id, &owner, &o.Name, &o.Size, &o.DataType, &slot, &tags, &o.MetaSize, &meta,
		&refer, &o.Created, &o.Updated, &thumb, &rtotal, &rcount, &yt, &best); err != nil {
		return o, err
	}
	o.ID, o.Owner, o.ReferID = uint64(id), owner, uint64(refer)
	o.YouTubeID = yt.String
	o.Tags = []string(tags)
	if o.Tags == nil {
		o.Tags = []string{}
	}
	if slot.Valid && slot.Int64 != noPersistenceSlot {
		s := int(slot.Int64)
		o.Slot = &s
	}
	if thumb.Valid {
		t := uint64(thumb.Int64)
		o.Thumbnail = &t
	}
	if o.Slot != nil {
		c := o.DataType
		o.Course = &c
		if best.Valid {
			ms := best.Int64
			o.TimeMS, o.Time = &ms, formatTime(ms)
		}
	}
	if rcount.Valid {
		o.Rating = &apiRated{Total: rtotal.Int64, Count: rcount.Int64}
	}
	if len(meta) > 0 && len(meta) <= 256 {
		o.MetaHex = hex.EncodeToString(meta)
	}
	return o, nil
}

func (p *pgAPI) objects(ctx context.Context, where string, args []any, order string, limit, offset int) ([]apiObject, int, error) {
	var total int
	if err := p.db.QueryRowContext(ctx, `SELECT count(*)`+apiObjectFrom+`WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := p.db.QueryContext(ctx, `SELECT `+p.columns(ctx)+apiObjectFrom+`WHERE `+where+
		` ORDER BY `+order+fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []apiObject
	for rows.Next() {
		o, err := scanAPIObject(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

func (p *pgAPI) Object(ctx context.Context, id uint64) (apiObject, error) {
	o, err := scanAPIObject(p.db.QueryRowContext(ctx, `SELECT `+p.columns(ctx)+apiObjectFrom+
		`WHERE `+apiVisible+` AND o.data_id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return o, errAPINotFound
	}
	return o, err
}

func (p *pgAPI) Ghosts(ctx context.Context, course int, owner string, limit, offset int) ([]apiObject, int, error) {
	where := apiVisible + ` AND o.persistence_slot IS NOT NULL AND o.persistence_slot <> ` + strconv.Itoa(noPersistenceSlot) + ` AND o.data_type = $1`
	args := []any{course}
	if owner != "" {
		args = append(args, owner)
		where += ` AND o.owner_pid = $2::numeric`
	}
	return p.objects(ctx, where, args, `o.updated DESC, o.data_id DESC`, limit, offset)
}

func (p *pgAPI) PlayerRankings(ctx context.Context, pid string) ([]apiPlayerRank, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT s.category, s.scores[1],
			(SELECT count(*) FROM mk7.ranking_scores x WHERE x.category = s.category AND x.scores[1] < s.scores[1]) + 1,
			(SELECT count(*) FROM mk7.ranking_scores x WHERE x.category = s.category)
		FROM mk7.ranking_scores s WHERE s.pid = $1::numeric ORDER BY s.category`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []apiPlayerRank
	for rows.Next() {
		var r apiPlayerRank
		if err := rows.Scan(&r.Course, &r.TimeMS, &r.Rank, &r.Of); err != nil {
			return nil, err
		}
		r.Time = formatTime(r.TimeMS)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *pgAPI) Rankings(ctx context.Context, course, limit, offset int) ([]apiRank, int, error) {
	var total int
	if err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM mk7.ranking_scores WHERE category = $1`, course).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := p.db.QueryContext(ctx, `SELECT pid, scores[1], updated_at FROM mk7.ranking_scores WHERE category = $1
		ORDER BY scores[1] ASC, updated_at ASC LIMIT $2 OFFSET $3`, course, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []apiRank
	for rows.Next() {
		var r apiRank
		if err := rows.Scan(&r.Player, &r.Score, &r.Updated); err != nil {
			return nil, 0, err
		}
		r.Rank = offset + len(out) + 1
		r.TimeMS, r.Time = r.Score, formatTime(r.Score)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (p *pgAPI) Courses(ctx context.Context) ([]apiCourse, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT c, sum(ranked)::int, sum(ghosts)::int FROM (
		SELECT category AS c, count(*) AS ranked, 0 AS ghosts FROM mk7.ranking_scores GROUP BY category
		UNION ALL
		SELECT o.data_type::bigint, 0, count(*) FROM mk7.datastore_objects o
			WHERE `+apiVisible+` AND o.persistence_slot IS NOT NULL AND o.persistence_slot <> `+strconv.Itoa(noPersistenceSlot)+`
			GROUP BY o.data_type
	) x GROUP BY c ORDER BY c`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []apiCourse
	for rows.Next() {
		var c apiCourse
		if err := rows.Scan(&c.Course, &c.Ranked, &c.Ghosts); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// startAPI serves the read API on MK7_API_ADDR (unset = off). Object bodies come from the same
// S3 bucket as the DataStore when it is configured.
// StartAPI serves the read API on MK7_API_ADDR (unset = off).
func StartAPI(db *sql.DB) {
	addr := os.Getenv("MK7_API_ADDR")
	if addr == "" {
		return
	}
	var bodies apiBodies
	if client, bucket, err := s3Client(); err == nil && client != nil {
		bodies = func(ctx context.Context, id uint64) (io.ReadCloser, error) {
			return client.GetObject(ctx, bucket, fmt.Sprintf("mk7/objects/%d.bin", id), minio.GetObjectOptions{})
		}
	}
	srv := &http.Server{Addr: addr, Handler: newAPI(&pgAPI{db: db}, bodies), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("[mario-kart-7] read API on %s", addr)
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("[mario-kart-7] read API stopped: %v", err)
		}
	}()
}
