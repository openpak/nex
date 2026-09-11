package database

import (
	"time"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
)

func InsertRankingByPIDAndRankingScoreData(pid types.PID, rankingScoreData ranking_types.RankingScoreData, uniqueID types.UInt64) error {
	now := time.Now().UnixNano()

	if rankingScoreData.Score == 0 {
		return nil
	}

	_, err := Postgres.Exec(`
		INSERT INTO rankings (
			owner_pid,
			category,
			score,
			order_by,
			update_mode,
			groups,
			param,
			unique_id,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (owner_pid, unique_id, category) DO UPDATE 
		SET score = excluded.score;`,
		uint64(pid),
		uint32(rankingScoreData.Category),
		uint32(rankingScoreData.Score),
		uint8(rankingScoreData.OrderBy),
		uint8(rankingScoreData.UpdateMode),
		[]byte(rankingScoreData.Groups),
		uint64(rankingScoreData.Param)&0x7FFFFFFFFFFFFFFF,
		uint64(uniqueID),
		now,
	)

	return err
}
