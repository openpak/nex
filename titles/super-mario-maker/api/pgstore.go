package api

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	datastore_smm_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore/super-mario-maker"
)

// PGStore is the Store over the title's Postgres and MinIO.
type PGStore struct {
	DB *sql.DB
	// S3 and Bucket/KeyBase locate thumbnails: <KeyBase>/<attachment data id>.jpg.
	S3      *minio.Client
	Bucket  string
	KeyBase string
}

const maxThumbBytes = 8 << 20

// courseSelect lists visible courses with their counters. Plays, clears and attempts are the
// object rating slots; stars are custom ranking 0. A thumbnail is a completed PrepareAttachFile
// object (data type 2) that refers to the course.
const courseSelect = `
	SELECT o.data_id, COALESCE(o.owner, 0), COALESCE(o.name, ''), COALESCE(o.tags, '{}'),
		o.creation_date, o.update_date,
		COALESCE(st.value, 0), COALESCE(pl.total_value, 0), COALESCE(cl.total_value, 0),
		COALESCE(at.total_value, 0), COALESCE(fa.total_value, 0), COALESCE(mi.total_value, 0),
		COALESCE(th.data_id, 0), COALESCE(mk.name, '')
	FROM datastore.objects o
	LEFT JOIN datastore.object_custom_rankings st ON st.data_id = o.data_id AND st.application_id = 0
	LEFT JOIN datastore.object_ratings pl ON pl.data_id = o.data_id AND pl.slot = %d
	LEFT JOIN datastore.object_ratings cl ON cl.data_id = o.data_id AND cl.slot = %d
	LEFT JOIN datastore.object_ratings at ON at.data_id = o.data_id AND at.slot = %d
	LEFT JOIN datastore.object_ratings fa ON fa.data_id = o.data_id AND fa.slot = %d
	LEFT JOIN datastore.object_ratings mi ON mi.data_id = o.data_id AND mi.slot = %d
	LEFT JOIN LATERAL (
		SELECT t.data_id FROM datastore.objects t
		WHERE t.refer_data_id = o.data_id AND t.data_type = 2 AND t.upload_completed AND NOT t.deleted
		ORDER BY t.data_id DESC LIMIT 1) th ON TRUE
	LEFT JOIN LATERAL (
		SELECT m.name FROM datastore.objects m
		WHERE m.owner = o.owner AND m.data_type = 1 AND NOT m.deleted
		ORDER BY m.data_id DESC LIMIT 1) mk ON TRUE
	WHERE o.upload_completed AND NOT o.deleted AND NOT o.under_review
		AND o.data_type > 2 AND o.data_type < 50`

func selectCourses() string {
	return fmt.Sprintf(courseSelect,
		datastore_smm_db.RatingSlotPlays, datastore_smm_db.RatingSlotClears, datastore_smm_db.RatingSlotAttempts,
		datastore_smm_db.RatingSlotFailures, datastore_smm_db.RatingSlotMiiverseCounts)
}

type scanner interface{ Scan(dest ...any) error }

func scanCourse(row scanner, thumbID *uint64) (Course, error) {
	var c Course
	var tags []string
	var created, updated sql.NullTime
	var dataID, thumb int64
	if err := row.Scan(&dataID, &c.OwnerPID, &c.Name, pq.Array(&tags), &created, &updated,
		&c.Stars, &c.Plays, &c.Clears, &c.Attempts, &c.Failures, &c.Comments, &thumb, &c.MakerName); err != nil {
		return c, err
	}
	c.DataID = uint64(dataID)
	c.Tags = tags
	c.Created, c.Updated = created.Time, updated.Time
	c.HasThumbnail = thumb != 0
	if thumbID != nil {
		*thumbID = uint64(thumb)
	}
	return c, nil
}

// courseFilters turns q into WHERE clauses (appended to courseSelect) and arguments.
func courseFilters(q CourseQuery) (string, []any) {
	var sb strings.Builder
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if !q.UpdatedSince.IsZero() {
		sb.WriteString(" AND o.update_date >= " + arg(q.UpdatedSince.UTC()))
	}
	if q.Name != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q.Name)
		sb.WriteString(` AND o.name ILIKE '%' || ` + arg(esc) + ` || '%' ESCAPE '\'`)
	}
	if q.OwnerPID > 0 {
		sb.WriteString(" AND o.owner = " + arg(q.OwnerPID))
	}
	if q.FailMin != nil || q.FailMax != nil {
		sb.WriteString(" AND COALESCE(at.total_value, 0) > 0")
		rate := "(100 - ROUND(COALESCE(cl.total_value, 0) * 100.0 / at.total_value))"
		if q.FailMin != nil {
			sb.WriteString(" AND " + rate + " >= " + arg(*q.FailMin))
		}
		if q.FailMax != nil {
			sb.WriteString(" AND " + rate + " <= " + arg(*q.FailMax))
		}
	}
	return sb.String(), args
}

func courseOrder(sort string) string {
	switch sort {
	case SortStars:
		return "COALESCE(st.value, 0) DESC, o.data_id DESC"
	case SortPlays:
		return "COALESCE(pl.total_value, 0) DESC, o.data_id DESC"
	case SortClears:
		return "COALESCE(cl.total_value, 0) DESC, o.data_id DESC"
	case SortRandom:
		return "RANDOM()"
	}
	return "o.creation_date DESC, o.data_id DESC"
}

// Courses implements Store.
func (s *PGStore) Courses(ctx context.Context, q CourseQuery) ([]Course, int, error) {
	filters, args := courseFilters(q)

	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+selectCourses()+filters+") c", args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	paged := append(append([]any{}, args...), q.Limit, q.Offset)
	rows, err := s.DB.QueryContext(ctx, selectCourses()+filters+" ORDER BY "+courseOrder(q.Sort)+
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2), paged...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Course
	for rows.Next() {
		c, err := scanCourse(rows, nil)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// Course implements Store.
func (s *PGStore) Course(ctx context.Context, dataID uint64) (Course, error) {
	c, err := scanCourse(s.DB.QueryRowContext(ctx, selectCourses()+" AND o.data_id = $1", int64(dataID)), nil)
	if err == sql.ErrNoRows {
		return Course{}, ErrNotFound
	}
	return c, err
}

// Thumbnail implements Store.
func (s *PGStore) Thumbnail(ctx context.Context, dataID uint64) ([]byte, error) {
	var thumb uint64
	if _, err := func() (Course, error) {
		return scanCourse(s.DB.QueryRowContext(ctx, selectCourses()+" AND o.data_id = $1", int64(dataID)), &thumb)
	}(); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if thumb == 0 || s.S3 == nil {
		return nil, ErrNotFound
	}
	obj, err := s.S3.GetObject(ctx, s.Bucket, fmt.Sprintf("%s/%d.jpg", s.KeyBase, thumb), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	b, err := io.ReadAll(io.LimitReader(obj, maxThumbBytes+1))
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if len(b) > maxThumbBytes {
		return nil, fmt.Errorf("thumbnail of %d is larger than %d bytes", dataID, maxThumbBytes)
	}
	return b, nil
}

// Makers implements Store. A maker is anyone who owns a visible course, best-starred first.
func (s *PGStore) Makers(ctx context.Context, limit, offset int) ([]Maker, int, error) {
	const base = `
		SELECT c.owner, COALESCE(mk.name, ''), COUNT(*), COALESCE(SUM(c.stars), 0), COALESCE(SUM(c.plays), 0), MIN(c.created)
		FROM (
			SELECT o.owner, o.creation_date AS created, COALESCE(st.value, 0) AS stars, COALESCE(pl.total_value, 0) AS plays
			FROM datastore.objects o
			LEFT JOIN datastore.object_custom_rankings st ON st.data_id = o.data_id AND st.application_id = 0
			LEFT JOIN datastore.object_ratings pl ON pl.data_id = o.data_id AND pl.slot = %d
			WHERE o.upload_completed AND NOT o.deleted AND NOT o.under_review AND o.data_type > 2 AND o.data_type < 50 AND o.owner IS NOT NULL
		) c
		LEFT JOIN LATERAL (
			SELECT m.name FROM datastore.objects m
			WHERE m.owner = c.owner AND m.data_type = 1 AND NOT m.deleted ORDER BY m.data_id DESC LIMIT 1) mk ON TRUE
		GROUP BY c.owner, mk.name`
	query := fmt.Sprintf(base, datastore_smm_db.RatingSlotPlays)

	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+query+") m").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, query+" ORDER BY 4 DESC, 1 LIMIT $1 OFFSET $2", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Maker
	for rows.Next() {
		var m Maker
		var created sql.NullTime
		if err := rows.Scan(&m.PID, &m.Name, &m.Courses, &m.Stars, &m.Plays, &created); err != nil {
			return nil, 0, err
		}
		m.Created = created.Time
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// Pickup implements Store.
func (s *PGStore) Pickup(ctx context.Context, failMin, failMax, limit int) ([]uint64, error) {
	filters, args := courseFilters(CourseQuery{FailMin: &failMin, FailMax: &failMax})
	rows, err := s.DB.QueryContext(ctx, selectCourses()+filters+" ORDER BY COALESCE(st.value, 0) DESC, o.data_id DESC"+
		fmt.Sprintf(" LIMIT $%d", len(args)+1), append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint64
	for rows.Next() {
		c, err := scanCourse(rows, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, c.DataID)
	}
	return out, rows.Err()
}

// AddBookmark implements Store. Adding the same course again keeps the first date.
func (s *PGStore) AddBookmark(ctx context.Context, pid int32, dataID uint64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO datastore.bookmarks (maker_pid, data_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, pid, int64(dataID))
	return err
}

// RemoveBookmark implements Store.
func (s *PGStore) RemoveBookmark(ctx context.Context, pid int32, dataID uint64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM datastore.bookmarks WHERE maker_pid = $1 AND data_id = $2`, pid, int64(dataID))
	return err
}

// Bookmarks implements Store, newest bookmark first.
func (s *PGStore) Bookmarks(ctx context.Context, pid int32) ([]uint64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data_id FROM datastore.bookmarks WHERE maker_pid = $1 ORDER BY created DESC, data_id DESC`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, uint64(id))
	}
	return out, rows.Err()
}

// ResolveNNID implements Store: the game uploads a maker profile object (data type 1) named
// with the player's NNID (Pretendo smm1-course-archive reads it the same way).
func (s *PGStore) ResolveNNID(ctx context.Context, name string) (int32, error) {
	var pid int32
	err := s.DB.QueryRowContext(ctx, `SELECT owner FROM datastore.objects
		WHERE data_type = 1 AND NOT deleted AND owner IS NOT NULL AND lower(name) = lower($1)
		ORDER BY data_id DESC LIMIT 1`, name).Scan(&pid)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return pid, err
}
