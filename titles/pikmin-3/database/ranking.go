package database

import (
	"time"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"

	"github.com/openpak/nex/titles/pikmin-3/board"
	"github.com/openpak/nex/titles/pikmin-3/globals"
)

// InsertRankingByPIDAndRankingScoreData stores an upload.
func InsertRankingByPIDAndRankingScoreData(pid types.PID, d ranking_types.RankingScoreData, uniqueID types.UInt64) error {
	if globals.Logger != nil {
		// The category to stage/mode map is not documented anywhere; this line is how it is learnt.
		globals.Logger.Infof("UploadScore pid=%d category=%d score=%d orderBy=%d updateMode=%d groups=%x param=%d uniqueID=%d",
			uint32(pid), uint32(d.Category), uint32(d.Score), uint8(d.OrderBy), uint8(d.UpdateMode), []byte(d.Groups), uint64(d.Param), uint64(uniqueID))
	}
	return Board.Upload(board.Entry{
		PID: uint32(pid), Category: uint32(d.Category), Score: uint32(d.Score), OrderBy: uint8(d.OrderBy),
		Groups: []byte(d.Groups), Param: uint64(d.Param), UniqueID: uint64(uniqueID),
	}, uint8(d.UpdateMode))
}

// UploadCommonData stores the caller's common data blob.
func UploadCommonData(pid types.PID, uniqueID types.UInt64, data types.Buffer) error {
	return Board.PutCommonData(uint32(pid), uint64(uniqueID), []byte(data))
}

// GetCommonData returns the blob for a unique id.
func GetCommonData(uniqueID types.UInt64) (types.Buffer, error) {
	b, err := Board.CommonData(uint64(uniqueID))
	return types.Buffer(b), err
}

func query(mode string, pid types.PID, category types.UInt32, p ranking_types.RankingOrderParam) board.Query {
	return board.Query{Mode: mode, Category: uint32(category), PID: uint32(pid), Calc: uint8(p.OrderCalculation),
		GroupIndex: uint8(p.GroupIndex), GroupNum: uint8(p.GroupNum), Offset: uint32(p.Offset), Length: uint32(p.Length)}
}

func run(q board.Query) (types.List[ranking_types.RankingRankData], uint32, error) {
	rows, total, err := Board.Run(q)
	if err != nil {
		return nil, 0, err
	}
	return ToRankData(rows), total, nil
}

// ToRankData converts ranked rows to the wire type.
func ToRankData(rows []board.Ranked) types.List[ranking_types.RankingRankData] {
	out := types.NewList[ranking_types.RankingRankData]()
	for _, r := range rows {
		d := ranking_types.NewRankingRankData()
		d.PrincipalID = types.NewPID(uint64(r.PID))
		d.UniqueID = types.UInt64(r.UniqueID)
		d.Order = types.UInt32(r.Order)
		d.Category = types.UInt32(r.Category)
		d.Score = types.UInt32(r.Score)
		d.Groups = types.Buffer(r.Groups)
		d.Param = types.UInt64(r.Param)
		d.CommonData = types.Buffer(r.Common)
		ut := types.NewDateTime(0)
		d.UpdateTime = ut.FromTimestamp(time.Unix(0, r.UpdatedAt))
		out = append(out, d)
	}
	return out
}

func GetRankingsAndCountByCategoryAndRankingOrderParam(category types.UInt32, p ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return run(query("range", 0, category, p))
}

func GetNearbyRankingsAndCountByCategoryAndRankingOrderParam(pid types.PID, category types.UInt32, p ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return run(query("near", pid, category, p))
}

func GetFriendsRankingsAndCountByCategoryAndRankingOrderParam(pid types.PID, category types.UInt32, p ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return run(query("friend-range", pid, category, p))
}

func GetNearbyFriendsRankingsAndCountByCategoryAndRankingOrderParam(pid types.PID, category types.UInt32, p ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return run(query("friend-near", pid, category, p))
}

func GetOwnRankingByCategoryAndRankingOrderParam(pid types.PID, category types.UInt32, p ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
	return run(query("user", pid, category, p))
}
