package database

import (
	"database/sql"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	"github.com/openpak/nex/titles/miracle-cure/globals"
)

func GetRankingsAndCountByCategoryAndRankingOrderParam(category types.UInt32, rankingOrderParam ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	rankings := types.NewList[ranking_types.RankingRankData]()

	rows, err := Postgres.Query(`
		SELECT
		owner_pid,
		score,
		groups,
		param
		FROM rankings WHERE category=$1 ORDER BY score DESC LIMIT $2 OFFSET $3`,
		uint32(category),
		uint8(rankingOrderParam.Length),
		uint32(rankingOrderParam.Offset),
	)
	if err != nil {
		return rankings, 0, err
	}
	defer rows.Close()

	row := 1
	for rows.Next() {
		rankingRankData := ranking_types.NewRankingRankData()
		rankingRankData.UniqueID = types.NewUInt64(0)
		rankingRankData.Order = types.NewUInt32(uint32(row))
		rankingRankData.Category = category

		var pid uint64
		var score uint32
		var groups []byte
		var param uint64
		if err := rows.Scan(&pid, &score, &groups, &param); err != nil && err != sql.ErrNoRows {
			globals.Logger.Critical(err.Error())
		}
		rankingRankData.Score = types.NewUInt32(score)
		rankingRankData.Groups = types.NewBuffer(groups)
		rankingRankData.Param = types.NewUInt64(param)

		var commonData []byte
		err = Postgres.QueryRow(`
			SELECT
			common_data
			FROM common_datas WHERE owner_pid=$1 AND unique_id=$2`,
			pid,
			uint64(rankingRankData.UniqueID),
		).Scan(&commonData)
		if err != nil && err != sql.ErrNoRows {
			globals.Logger.Critical(err.Error())
			return rankings, 0, err
		}
		rankingRankData.CommonData = types.NewBuffer(commonData)
		rankingRankData.PrincipalID = types.NewPID(pid)

		if err == nil {
			rankings = append(rankings, rankingRankData)
			row += 1
		}
	}

	return rankings, uint32(len(rankings)), nil
}
