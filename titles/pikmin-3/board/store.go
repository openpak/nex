package board

import (
	"database/sql"
	"errors"
	"time"
)

// Store keeps one row per (player, category). Friends names a player's friend pids; nil means none.
type Store struct {
	DB      *sql.DB
	Friends func(pid uint32) []uint32
	Now     func() time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS rankings (
	id bigserial PRIMARY KEY,
	owner_pid integer,
	category integer,
	score integer,
	order_by integer,
	update_mode integer,
	groups bytea,
	param bigint,
	common_data bytea,
	created_at bigint
);
ALTER TABLE rankings ADD COLUMN IF NOT EXISTS unique_id bigint NOT NULL DEFAULT 0;
-- Older builds appended a row per upload; keep each player's newest row per category.
DELETE FROM rankings r USING rankings n
	WHERE r.owner_pid = n.owner_pid AND r.category = n.category AND r.id < n.id;
CREATE UNIQUE INDEX IF NOT EXISTS rankings_owner_category ON rankings (owner_pid, category);
CREATE INDEX IF NOT EXISTS rankings_category_score ON rankings (category, score);
CREATE TABLE IF NOT EXISTS ranking_common_data (
	unique_id bigint NOT NULL,
	owner_pid integer NOT NULL,
	data bytea NOT NULL,
	PRIMARY KEY (owner_pid, unique_id)
);`

// Migrate creates or upgrades the tables.
func (s *Store) Migrate() error {
	_, err := s.DB.Exec(schema)
	return err
}

func (s *Store) now() int64 {
	if s.Now != nil {
		return s.Now().UnixNano()
	}
	return time.Now().UnixNano()
}

// Upload applies a score upload under the category's update mode.
func (s *Store) Upload(in Entry, updateMode uint8) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var old *Entry
	var e Entry
	err = tx.QueryRow(`SELECT score, order_by FROM rankings WHERE owner_pid=$1 AND category=$2 FOR UPDATE`,
		int32(in.PID), int32(in.Category)).Scan(&e.Score, &e.OrderBy)
	switch {
	case err == nil:
		old = &e
	case errors.Is(err, sql.ErrNoRows):
	default:
		return err
	}
	if !Keep(old, in, updateMode) {
		return tx.Commit()
	}
	_, err = tx.Exec(`
		INSERT INTO rankings (owner_pid, category, score, order_by, update_mode, groups, param, common_data, created_at, unique_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (owner_pid, category) DO UPDATE SET
			score=EXCLUDED.score, order_by=EXCLUDED.order_by, update_mode=EXCLUDED.update_mode,
			groups=EXCLUDED.groups, param=EXCLUDED.param, created_at=EXCLUDED.created_at, unique_id=EXCLUDED.unique_id`,
		int32(in.PID), int32(in.Category), int32(in.Score), int32(in.OrderBy), int32(updateMode),
		nonNil(in.Groups), int64(in.Param), []byte{}, s.now(), int64(in.UniqueID))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func nonNil(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

// Category loads every entry in a category and the category's order (taken from the newest upload).
func (s *Store) Category(category uint32) ([]Entry, uint8, error) {
	rows, err := s.DB.Query(`
		SELECT r.owner_pid, r.score, r.order_by, r.groups, r.param,
		       COALESCE(c.data, r.common_data), r.unique_id, r.created_at
		FROM rankings r LEFT JOIN ranking_common_data c ON c.unique_id = r.unique_id AND c.owner_pid = r.owner_pid
		WHERE r.category=$1`, int32(category))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Entry
	var orderBy uint8
	var newest int64 = -1
	for rows.Next() {
		var pid, score, ob int32
		var param, uid, at int64
		var groups, common []byte
		if err := rows.Scan(&pid, &score, &ob, &groups, &param, &common, &uid, &at); err != nil {
			return nil, 0, err
		}
		out = append(out, Entry{PID: uint32(pid), Category: category, Score: uint32(score), OrderBy: uint8(ob),
			Groups: groups, Param: uint64(param), Common: common, UniqueID: uint64(uid), UpdatedAt: at})
		if at > newest {
			newest, orderBy = at, uint8(ob)
		}
	}
	return out, orderBy, rows.Err()
}

// Query is what a GetRanking call asks for.
type Query struct {
	Mode       string // range | near | friend-range | friend-near | user
	Category   uint32
	PID        uint32 // the caller, or the principal asked about
	Calc       uint8
	GroupIndex uint8
	GroupNum   uint8
	Offset     uint32
	Length     uint32
}

// Run answers a Query. total is the size of the set that was ranked.
func (s *Store) Run(q Query) (rows []Ranked, total uint32, err error) {
	entries, orderBy, err := s.Category(q.Category)
	if err != nil {
		return nil, 0, err
	}
	entries = FilterGroup(entries, q.GroupIndex, q.GroupNum)
	if q.Mode == "friend-range" || q.Mode == "friend-near" {
		var friends []uint32
		if s.Friends != nil {
			friends = s.Friends(q.PID)
		}
		entries = FilterPIDs(entries, append(friends, q.PID)...)
	}
	ranked := Rank(entries, orderBy, q.Calc)
	total = uint32(len(ranked))
	switch q.Mode {
	case "range", "friend-range":
		rows = Range(ranked, q.Offset, q.Length)
	case "near", "friend-near":
		rows = Near(ranked, q.PID, q.Length)
	case "user":
		rows = Own(ranked, q.PID)
	}
	return rows, total, nil
}

// PutCommonData stores a player's common data blob.
func (s *Store) PutCommonData(pid uint32, uniqueID uint64, data []byte) error {
	_, err := s.DB.Exec(`INSERT INTO ranking_common_data (unique_id, owner_pid, data) VALUES ($1,$2,$3)
		ON CONFLICT (owner_pid, unique_id) DO UPDATE SET data=EXCLUDED.data`, int64(uniqueID), int32(pid), nonNil(data))
	return err
}

// CommonData returns the blob stored for a unique id.
func (s *Store) CommonData(uniqueID uint64) ([]byte, error) {
	var data []byte
	err := s.DB.QueryRow(`SELECT data FROM ranking_common_data WHERE unique_id=$1 LIMIT 1`, int64(uniqueID)).Scan(&data)
	return data, err
}

// Categories lists every category that has a score (the histgrm job walks these).
func (s *Store) Categories() ([]uint32, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT category FROM rankings ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var c int32
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, uint32(c))
	}
	return out, rows.Err()
}
