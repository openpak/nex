package mta

// Ranking (protocol 0x70) storage: leaderboards and the per-player common data
// blob a leaderboard row carries.
//
// nex-protocols-common-go implements the protocol and asks us only where the rows
// live. They live in Postgres, in two tables of our own, and the ranks themselves
// are computed by the database's window functions rather than in Go.

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/PretendoNetwork/nex-go/v2/types"
	common_ranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	"github.com/PretendoNetwork/nex-protocols-go/v2/ranking/constants"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	"github.com/lib/pq"
)

type rankingStore struct {
	db      *sql.DB
	friends func(pid uint32) []uint32
}

func (s rankingStore) wire(common *common_ranking.CommonProtocol) {
	common.UploadCommonData = s.uploadCommonData
	common.GetCommonData = s.commonData
	common.InsertRankingByPIDAndRankingScoreData = s.insertScore
	common.GetRankingsAndCountByCategoryAndRankingOrderParam = s.rankings
	common.GetOwnRankingByCategoryAndRankingOrderParam = s.ownRanking
	common.GetNearbyRankingsAndCountByCategoryAndRankingOrderParam = s.nearbyRankings
	common.GetFriendsRankingsAndCountByCategoryAndRankingOrderParam = s.friendRankings
	common.GetNearbyFriendsRankingsAndCountByCategoryAndRankingOrderParam = s.nearbyFriendRankings
}

// insertScore commits an uploaded score, honouring the score data's own update mode
// and ordering: a "normal" upload keeps the better of the two, a delete-old upload
// always replaces.
func (s rankingStore) insertScore(pid types.PID, score ranking_types.RankingScoreData, uniqueID types.UInt64) error {
	_, err := s.db.Exec(`INSERT INTO ranking.scores
		(pid, category, score, order_by, groups, param, unique_id, updated_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (pid, category) DO UPDATE SET
			score = EXCLUDED.score,
			order_by = EXCLUDED.order_by,
			groups = EXCLUDED.groups,
			param = EXCLUDED.param,
			unique_id = EXCLUDED.unique_id,
			updated_time = now()
		WHERE `+keepsUpdate(score.UpdateMode, score.OrderBy),
		pid, uint32(score.Category), uint32(score.Score), uint8(score.OrderBy),
		[]byte(score.Groups), uint64(score.Param), uniqueID)
	return err
}

// keepsUpdate is the ON CONFLICT condition under which an uploaded score replaces
// the one already stored. Descending categories keep the highest score, ascending
// ones (times, stroke counts) the lowest, and a delete-old upload keeps whatever
// arrived last.
func keepsUpdate(mode constants.UpdateMode, order constants.OrderBy) string {
	if mode == constants.UpdateModeDeleteOld {
		return "true"
	}
	if order == constants.OrderByAscending {
		return "EXCLUDED.score < ranking.scores.score"
	}
	return "EXCLUDED.score > ranking.scores.score"
}

func (s rankingStore) uploadCommonData(pid types.PID, uniqueID types.UInt64, commonData types.Buffer) error {
	_, err := s.db.Exec(`INSERT INTO ranking.common_data (pid, unique_id, data)
		VALUES ($1, $2, $3)
		ON CONFLICT (pid) DO UPDATE SET unique_id = EXCLUDED.unique_id, data = EXCLUDED.data`,
		pid, uniqueID, []byte(commonData))
	return err
}

// commonData reads a blob back by unique ID. A title that does not use unique IDs
// sends 0, and there is then no way to tell whose blob is wanted — but such a title
// reads its blobs off the leaderboard rows, which carry them per player anyway.
func (s rankingStore) commonData(uniqueID types.UInt64) (types.Buffer, error) {
	if uniqueID == 0 {
		return nil, errors.New("no common data for unique id 0")
	}
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM ranking.common_data WHERE unique_id = $1 LIMIT 1`, uniqueID).Scan(&data)
	if err != nil {
		return nil, err
	}
	return types.NewBuffer(data), nil
}

func (s rankingStore) rankings(category types.UInt32, order ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return s.board(category, order, "", "true")
}

func (s rankingStore) ownRanking(pid types.PID, category types.UInt32, order ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return s.board(category, order, "", "r.pid = $4", pid)
}

func (s rankingStore) nearbyRankings(pid types.PID, category types.UInt32, order ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return s.board(category, order, "", nearbyScope, pid, halfWindow(order))
}

func (s rankingStore) friendRankings(pid types.PID, category types.UInt32, order ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return s.board(category, order, " AND pid = ANY($4)", "true", s.friendCohort(pid))
}

func (s rankingStore) nearbyFriendRankings(pid types.PID, category types.UInt32, order ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return s.board(category, order, " AND pid = ANY($6)", nearbyScope, pid, halfWindow(order), s.friendCohort(pid))
}

// nearbyScope selects the rows around the caller's own position, which is what the
// "near" ranking modes ask for. A caller with no score of their own starts at 1.
const nearbyScope = `r.position >= GREATEST(COALESCE((SELECT position FROM ranked WHERE pid = $4), 1) - $5, 1)`

func halfWindow(order ranking_types.RankingOrderParam) int {
	return boardLength(order) / 2
}

// friendCohort is the caller plus their friends: the set a friends-only board ranks
// within. The friend graph is OpenPak's, through nx-baas.
func (s rankingStore) friendCohort(pid types.PID) any {
	cohort := []int64{int64(pid)}
	for _, friend := range s.friends(uint32(pid)) {
		cohort = append(cohort, int64(friend))
	}
	return pq.Array(cohort)
}

// boardLength is how many rows the client asked for, bounded so a bad or absent
// length cannot ask the database for an entire leaderboard.
func boardLength(order ranking_types.RankingOrderParam) int {
	length := int(order.Length)
	if length <= 0 || length > 100 {
		return 100
	}
	return length
}

// board runs one leaderboard query. cohort narrows who is ranked (inside the CTE, so
// positions are relative to that set); scope narrows which of the ranked rows come
// back. Parameters $1..$3 are always category, limit and offset; extra ones start at $4.
func (s rankingStore) board(category types.UInt32, order ranking_types.RankingOrderParam, cohort, scope string, extra ...any) (types.List[ranking_types.RankingRankData], uint32, error) {
	descending, err := s.categoryIsDescending(category)
	if err != nil {
		return nil, 0, err
	}

	args := append([]any{uint32(category), boardLength(order), uint32(order.Offset)}, extra...)
	rows, err := s.db.Query(boardSQL(descending, order.OrderCalculation == constants.OrderCalculation123, cohort, scope), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	board := make(types.List[ranking_types.RankingRankData], 0, boardLength(order))
	var total uint32
	for rows.Next() {
		rank := ranking_types.NewRankingRankData()
		rank.Category = category
		if err := rows.Scan(&rank.PrincipalID, &rank.UniqueID, &rank.Order, &rank.Score,
			&rank.Groups, &rank.Param, &rank.UpdateTime, &rank.CommonData, &total); err != nil {
			return nil, 0, err
		}
		board = append(board, rank)
	}
	return board, total, rows.Err()
}

// categoryIsDescending reads the ordering the title itself declared when it uploaded
// scores for this category. Descending (higher is better) is the assumption for an
// empty board, where it changes nothing.
func (s rankingStore) categoryIsDescending(category types.UInt32) (bool, error) {
	var orderBy uint8
	err := s.db.QueryRow(`SELECT order_by FROM ranking.scores WHERE category = $1 LIMIT 1`, uint32(category)).Scan(&orderBy)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return constants.OrderBy(orderBy) == constants.OrderByDescending, nil
}

// boardSQL builds the leaderboard query. Ranks come from a window function: RANK for
// the standard "1224" tie handling, ROW_NUMBER for strictly ordinal "1234", with ties
// broken by the earlier upload and then the lower PID, as the protocol describes.
func boardSQL(descending, ordinal bool, cohort, scope string) string {
	position := "RANK()"
	if ordinal {
		position = "ROW_NUMBER()"
	}
	direction := "ASC"
	if descending {
		direction = "DESC"
	}
	return fmt.Sprintf(`WITH ranked AS (
		SELECT pid, unique_id, score, groups, param, updated_time,
			%s OVER (ORDER BY score %s, updated_time ASC, pid ASC) AS position
		FROM ranking.scores WHERE category = $1%s
	)
	SELECT r.pid, r.unique_id, r.position, r.score, r.groups, r.param, r.updated_time,
		COALESCE(c.data, ''::bytea), (SELECT count(*) FROM ranked)
	FROM ranked r LEFT JOIN ranking.common_data c ON c.pid = r.pid
	WHERE %s
	ORDER BY r.position
	LIMIT $2 OFFSET $3`, position, direction, cohort, scope)
}
