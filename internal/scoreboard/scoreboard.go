// Package scoreboard is the shared Ranking (0x70) store for the titles that register the
// protocol but had nothing behind it. It is the Puyo Puyo Tetris / Sonic Transformed /
// Terraria implementation in one place: scores in ranking.scores, category ordering in
// ranking.categories, per-score common data in ranking.common_data, read back by every
// ranking mode the game can ask for. nexhost.ScoresRankings serves the same tables on the
// rankings contract.
package scoreboard

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	rankingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	"github.com/lib/pq"
)

// Friends returns the friend pids of a player.
type Friends func(pid uint32) []uint32

// Store is a ranking store over one Postgres database.
type Store struct {
	DB      *sql.DB
	Friends Friends
}

// Install creates the tables and sets every storage hook of the common ranking protocol.
// friends may be nil (friend boards then hold only the player).
func Install(p *commonranking.CommonProtocol, db *sql.DB, friends Friends) error {
	s := &Store{DB: db, Friends: friends}
	if err := s.InitTables(); err != nil {
		return err
	}
	p.InsertRankingByPIDAndRankingScoreData = s.Insert
	p.UploadCommonData = s.UploadCommonData
	p.GetCommonData = s.GetCommonData
	p.GetRankingsAndCountByCategoryAndRankingOrderParam = s.Global
	p.GetNearbyRankingsAndCountByCategoryAndRankingOrderParam = s.Nearby
	p.GetFriendsRankingsAndCountByCategoryAndRankingOrderParam = s.FriendsRange
	p.GetNearbyFriendsRankingsAndCountByCategoryAndRankingOrderParam = s.NearbyFriends
	p.GetOwnRankingByCategoryAndRankingOrderParam = s.Own
	return nil
}

// InitTables creates the ranking schema (the same one the other common-protocol titles use).
func (s *Store) InitTables() error {
	for _, q := range []string{
		`CREATE SCHEMA IF NOT EXISTS ranking`,
		`CREATE TABLE IF NOT EXISTS ranking.scores (
			deleted boolean NOT NULL DEFAULT FALSE,
			unique_id bigint,
			owner_pid bigint,
			category int,
			groups bytea,
			score int,
			param bigint,
			creation_date timestamp,
			update_date timestamp,
			PRIMARY KEY (category, owner_pid, unique_id)
		)`,
		`CREATE INDEX IF NOT EXISTS score_index ON ranking.scores (score)`,
		`CREATE INDEX IF NOT EXISTS category_index ON ranking.scores (category)`,
		`CREATE TABLE IF NOT EXISTS ranking.categories (
			category int PRIMARY KEY,
			golf_scoring boolean,
			creation_date timestamp
		)`,
		`CREATE TABLE IF NOT EXISTS ranking.common_data (
			deleted boolean NOT NULL DEFAULT FALSE,
			unique_id bigint,
			owner_pid bigint,
			data bytea,
			creation_date timestamp,
			update_date timestamp,
			PRIMARY KEY (owner_pid, unique_id)
		)`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func sysErr(err error) error { return nex.NewError(nex.ResultCodes.Core.SystemError, err.Error()) }

// Insert stores one uploaded score. The category's order is fixed by its first upload.
func (s *Store) Insert(pid types.PID, d rankingtypes.RankingScoreData, uniqueID types.UInt64) error {
	now := time.Now().UTC()
	tx, err := s.DB.Begin()
	if err != nil {
		return sysErr(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO ranking.categories (category, golf_scoring, creation_date)
		VALUES ($1, $2, $3) ON CONFLICT (category) DO NOTHING`, int64(d.Category), d.OrderBy == 1, now); err != nil {
		return sysErr(err)
	}
	if _, err = tx.Exec(`INSERT INTO ranking.scores (unique_id, owner_pid, category, groups, score, param, creation_date, update_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		ON CONFLICT (category, owner_pid, unique_id) DO UPDATE SET score = excluded.score, update_date = excluded.update_date`,
		int64(uniqueID), int64(pid), int64(d.Category), []byte(d.Groups), int64(d.Score), int64(d.Param), now); err != nil {
		return sysErr(err)
	}
	if err = tx.Commit(); err != nil {
		return sysErr(err)
	}
	return nil
}

// UploadCommonData stores the opaque blob the game attaches to a score.
func (s *Store) UploadCommonData(pid types.PID, uniqueID types.UInt64, data types.Buffer) error {
	now := time.Now().UTC()
	if _, err := s.DB.Exec(`INSERT INTO ranking.common_data (unique_id, owner_pid, data, creation_date, update_date)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (owner_pid, unique_id) DO UPDATE SET data = excluded.data, update_date = excluded.update_date`,
		int64(uniqueID), int64(pid), []byte(data), now); err != nil {
		return sysErr(err)
	}
	return nil
}

// GetCommonData returns the blob stored for a score, empty when there is none.
func (s *Store) GetCommonData(uniqueID types.UInt64) (types.Buffer, error) {
	var b []byte
	err := s.DB.QueryRow(`SELECT data FROM ranking.common_data WHERE unique_id = $1 AND NOT deleted
		ORDER BY update_date DESC LIMIT 1`, int64(uniqueID)).Scan(&b)
	if err == sql.ErrNoRows {
		return types.Buffer{}, nil
	}
	if err != nil {
		return nil, sysErr(err)
	}
	return types.Buffer(b), nil
}

type mode int

const (
	modeGlobal mode = iota
	modeNearby
	modeFriends
	modeNearbyFriends
	modeOwn
)

// query builds the read for a mode. $1 category, $2 ties share a rank, $3 group index,
// $4 group number, $5 offset, $6 length, $7 requester pid, $8 friend pids.
func query(m mode) string {
	filter := ""
	switch m {
	case modeFriends, modeNearbyFriends:
		filter = `(owner_pid = ANY($8) OR owner_pid = $7) AND`
	}
	central := ""
	from := `FROM scores`
	where := ``
	switch m {
	case modeNearby, modeNearbyFriends:
		central = `, central_user AS (
			SELECT scores.ord, GREATEST(scores.ord - $6 / 2, 1) AS min_ord,
				LEAST(scores.ord + $6 / 2, (SELECT max(scores.ord) FROM scores)) AS max_ord
			FROM scores WHERE owner_pid = $7
		)`
		from = `FROM central_user, scores`
		where = `WHERE scores.ord >= central_user.min_ord AND scores.ord <= central_user.max_ord`
	case modeOwn:
		where = `WHERE scores.owner_pid = $7`
	}
	return fmt.Sprintf(`
		WITH scores AS (
			WITH cat AS (SELECT golf_scoring FROM ranking.categories WHERE category = $1)
			SELECT *,
				CASE WHEN $2 THEN
					RANK() OVER (ORDER BY CASE WHEN cat.golf_scoring THEN ranking.scores.score END DESC,
						CASE WHEN NOT cat.golf_scoring THEN ranking.scores.score END ASC)
				ELSE
					ROW_NUMBER() OVER (ORDER BY CASE WHEN cat.golf_scoring THEN ranking.scores.score END DESC,
						CASE WHEN NOT cat.golf_scoring THEN ranking.scores.score END ASC)
				END AS ord
			FROM cat, ranking.scores
			WHERE %s category = $1 AND NOT deleted AND
				CASE WHEN $3 < 2 AND length(groups) >= 2 THEN get_byte(groups, $3) = $4 ELSE TRUE END
		)%s
		SELECT scores.unique_id, scores.owner_pid, scores.category, scores.groups, scores.score, scores.param,
			scores.update_date, COALESCE(ranking.common_data.data, ''::bytea), scores.ord, COUNT(*) OVER () AS count
		%s
			LEFT OUTER JOIN ranking.common_data
			ON scores.unique_id = ranking.common_data.unique_id AND scores.owner_pid = ranking.common_data.owner_pid
		%s
		ORDER BY scores.ord OFFSET $5 LIMIT $6`, filter, central, from, where)
}

var queries = map[mode]string{
	modeGlobal: query(modeGlobal), modeNearby: query(modeNearby), modeFriends: query(modeFriends),
	modeNearbyFriends: query(modeNearbyFriends), modeOwn: query(modeOwn),
}

type list = types.List[rankingtypes.RankingRankData]

func (s *Store) read(m mode, pid types.PID, category types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	friends := []int64{}
	if m == modeFriends || m == modeNearbyFriends {
		if s.Friends != nil {
			for _, f := range s.Friends(uint32(pid)) {
				friends = append(friends, int64(f))
			}
		}
	}
	args := []any{int64(category), o.OrderCalculation == 0, int64(o.GroupIndex), int64(o.GroupNum), int64(o.Offset), int64(o.Length)}
	if m != modeGlobal {
		args = append(args, int64(pid))
	}
	if m == modeFriends || m == modeNearbyFriends {
		args = append(args, pq.Array(friends))
	}
	rows, err := s.DB.Query(queries[m], args...)
	if err != nil {
		return nil, 0, sysErr(err)
	}
	defer rows.Close()
	out := make(list, 0, int(o.Length))
	var total uint32
	for rows.Next() {
		var (
			uid, owner, param int64
			cat, score, ord   int64
			groups, common    []byte
			updated           time.Time
		)
		if err := rows.Scan(&uid, &owner, &cat, &groups, &score, &param, &updated, &common, &ord, &total); err != nil {
			return nil, 0, sysErr(err)
		}
		r := rankingtypes.NewRankingRankData()
		r.UniqueID, r.PrincipalID = types.UInt64(uid), types.NewPID(uint64(owner))
		r.Category, r.Score, r.Param = types.UInt32(cat), types.UInt32(score), types.UInt64(param)
		r.Groups, r.CommonData, r.Order = types.Buffer(groups), types.Buffer(common), types.UInt32(ord)
		r.UpdateTime = r.UpdateTime.FromTimestamp(updated)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, sysErr(err)
	}
	return out, total, nil
}

// Global is the whole board, best first.
func (s *Store) Global(c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeGlobal, 0, c, o)
}

// Nearby is the window around the requester.
func (s *Store) Nearby(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeNearby, pid, c, o)
}

// FriendsRange is the board of the requester and their friends.
func (s *Store) FriendsRange(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeFriends, pid, c, o)
}

// NearbyFriends is the window around the requester among friends.
func (s *Store) NearbyFriends(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeNearbyFriends, pid, c, o)
}

// Own is the requester's own entries.
func (s *Store) Own(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeOwn, pid, c, o)
}
