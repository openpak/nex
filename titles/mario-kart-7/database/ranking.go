package database

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// RankingSchema is the legacy Ranking tables of the mk7 schema. Scores are the list the game
// uploads (two values on the wire; the first is the time-trial time in milliseconds, the
// second is kept verbatim); the two "unknown" fields of UploadScore are stored as sent so a
// capture can tell what they are.
const RankingSchema = `
CREATE SCHEMA IF NOT EXISTS mk7;

CREATE TABLE IF NOT EXISTS mk7.ranking_scores (
	category bigint NOT NULL,
	pid numeric(20) NOT NULL,
	unique_id bigint NOT NULL,
	scores bigint[] NOT NULL,
	unknown1 smallint NOT NULL DEFAULT 0,
	unknown2 bigint NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (category, pid)
);
CREATE INDEX IF NOT EXISTS ranking_scores_category_time ON mk7.ranking_scores (category, (scores[1]));

CREATE TABLE IF NOT EXISTS mk7.ranking_common_data (
	unique_id bigint PRIMARY KEY,
	pid numeric(20) NOT NULL,
	data bytea NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
);
`

// RankEntry is one row of a category leaderboard.
type RankEntry struct {
	Rank       uint32
	PID        uint64
	UniqueID   uint32
	Scores     []uint32
	Unknown1   uint8
	Unknown2   uint32
	CommonData []byte
}

// Ranking is the legacy Ranking store on Postgres.
type Ranking struct{ DB *sql.DB }

// UploadScore keeps a player's best (lowest first score) per category; an equal or worse
// upload only refreshes nothing. It reports whether the stored row changed.
func (r Ranking) UploadScore(pid uint64, uniqueID uint32, category uint32, scores []uint32, unknown1 uint8, unknown2 uint32) (bool, error) {
	if len(scores) == 0 {
		return false, fmt.Errorf("no scores")
	}
	vals := make([]int64, len(scores))
	for i, s := range scores {
		vals[i] = int64(s)
	}
	res, err := r.DB.Exec(`INSERT INTO mk7.ranking_scores (category, pid, unique_id, scores, unknown1, unknown2, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,now())
		ON CONFLICT (category, pid) DO UPDATE SET unique_id = EXCLUDED.unique_id, scores = EXCLUDED.scores,
		  unknown1 = EXCLUDED.unknown1, unknown2 = EXCLUDED.unknown2, updated_at = now()
		WHERE EXCLUDED.scores[1] < mk7.ranking_scores.scores[1]`,
		int64(category), fmt.Sprint(pid), int64(uniqueID), pq.Array(vals), int(unknown1), int64(unknown2))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteScore removes a player's score in one category.
func (r Ranking) DeleteScore(pid uint64, category uint32) error {
	_, err := r.DB.Exec(`DELETE FROM mk7.ranking_scores WHERE pid = $1 AND category = $2`, fmt.Sprint(pid), int64(category))
	return err
}

// DeleteAllScores removes every score of a player.
func (r Ranking) DeleteAllScores(pid uint64) error {
	_, err := r.DB.Exec(`DELETE FROM mk7.ranking_scores WHERE pid = $1`, fmt.Sprint(pid))
	return err
}

// UploadCommonData stores the blob that rides along a player's scores.
func (r Ranking) UploadCommonData(pid uint64, uniqueID uint32, data []byte) error {
	_, err := r.DB.Exec(`INSERT INTO mk7.ranking_common_data (unique_id, pid, data, updated_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (unique_id) DO UPDATE SET pid = EXCLUDED.pid, data = EXCLUDED.data, updated_at = now()`,
		int64(uniqueID), fmt.Sprint(pid), data)
	return err
}

// DeleteCommonData removes it again.
func (r Ranking) DeleteCommonData(pid uint64, uniqueID uint32) error {
	_, err := r.DB.Exec(`DELETE FROM mk7.ranking_common_data WHERE unique_id = $1 AND pid = $2`, int64(uniqueID), fmt.Sprint(pid))
	return err
}

// CommonData returns the blob for a unique id (empty when there is none).
func (r Ranking) CommonData(uniqueID uint32) ([]byte, error) {
	var data []byte
	err := r.DB.QueryRow(`SELECT data FROM mk7.ranking_common_data WHERE unique_id = $1`, int64(uniqueID)).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}

// Total is the number of ranked players in a category.
func (r Ranking) Total(category uint32) (uint32, error) {
	var n uint32
	err := r.DB.QueryRow(`SELECT count(*) FROM mk7.ranking_scores WHERE category = $1`, int64(category)).Scan(&n)
	return n, err
}

// Page lists a category sorted by one score index, rank starting at offset+1.
func (r Ranking) Page(category uint32, index int, desc bool, offset, length uint32) ([]RankEntry, error) {
	if index != 0 && index != 1 {
		index = 0
	}
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	rows, err := r.DB.Query(fmt.Sprintf(`SELECT s.pid, s.unique_id, s.scores, s.unknown1, s.unknown2, coalesce(c.data, '')
		FROM mk7.ranking_scores s LEFT JOIN mk7.ranking_common_data c ON c.unique_id = s.unique_id
		WHERE s.category = $1 ORDER BY s.scores[%d] %s, s.updated_at ASC, s.pid ASC OFFSET $2 LIMIT $3`, index+1, dir),
		int64(category), int64(offset), int64(length))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RankEntry
	for rows.Next() {
		var e RankEntry
		var pid string
		var uid, u2 int64
		var u1 int
		var scores pq.Int64Array
		if err := rows.Scan(&pid, &uid, &scores, &u1, &u2, &e.CommonData); err != nil {
			return nil, err
		}
		fmt.Sscan(pid, &e.PID)
		e.UniqueID, e.Unknown1, e.Unknown2 = uint32(uid), uint8(u1), uint32(u2)
		for _, s := range scores {
			e.Scores = append(e.Scores, uint32(s))
		}
		e.Rank = offset + uint32(len(out)) + 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// Position is the zero-based rank of a player in a category by one score index (0 when
// absent is reported through ok=false).
func (r Ranking) Position(pid uint64, category uint32, index int, desc bool) (pos uint32, ok bool, err error) {
	if index != 0 && index != 1 {
		index = 0
	}
	cmp := "<"
	if desc {
		cmp = ">"
	}
	err = r.DB.QueryRow(fmt.Sprintf(`SELECT count(*) FROM mk7.ranking_scores x WHERE x.category = $1
		AND x.scores[%[1]d] %[2]s (SELECT scores[%[1]d] FROM mk7.ranking_scores WHERE category = $1 AND pid = $2)`, index+1, cmp),
		int64(category), fmt.Sprint(pid)).Scan(&pos)
	if err != nil {
		return 0, false, err
	}
	var have bool
	err = r.DB.QueryRow(`SELECT EXISTS (SELECT 1 FROM mk7.ranking_scores WHERE category = $1 AND pid = $2)`,
		int64(category), fmt.Sprint(pid)).Scan(&have)
	return pos, have, err
}
