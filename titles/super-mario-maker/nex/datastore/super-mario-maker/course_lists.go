package nex_datastore_super_mario_maker

import (
	"strconv"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_super_mario_maker "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker"
	datastore_super_mario_maker_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker/types"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	datastore_smm_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore/super-mario-maker"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
)

// The course lists below (Latest, GetCustomRanking, ScoreRangeCascaded, BestScoreRate) are the
// DataStore methods of the Wii U game's Course World, 100 Mario Challenge and MAKERS tabs that
// upstream Pretendo does not implement. Method IDs and structures are from the NintendoClients
// wiki ("Data Store Protocol (SMM)", MIT, https://github.com/kinnay/NintendoClients). What the
// real client sends in extraData for ScoreRangeCascaded ([rankingAppID, min, max, flag]) and how
// the windowed ranking IDs work was observed by Protarium's Pretendo fork (AGPL-3.0,
// github.com/Protarium-Network/super-mario-maker, commit 6d49108) and is used here as fact; this
// file is a rewrite over OpenPak's shared course query, not a copy.

// maxListLength caps one page. The client asks for about 25 at a time; 100 Mario starts with a
// larger one (constants.ONE_HUNDRED_MARIO_RECOMMENDED_COURSE_COUNT).
const maxListLength = 100

func clampLength(length types.UInt32) int {
	if length == 0 || length > maxListLength {
		return maxListLength
	}
	return int(length)
}

func writeCourseList(callID uint32, methodID uint32, list types.List[datastore_super_mario_maker_types.DataStoreCustomRankingResult]) *nex.RMCMessage {
	stream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)
	list.WriteTo(stream)

	rmc := nex.NewRMCSuccess(globals.SecureEndpoint, stream.Bytes())
	rmc.ProtocolID = datastore_super_mario_maker.ProtocolID
	rmc.MethodID = methodID
	rmc.CallID = callID

	return rmc
}

// applyResultOption drops what ResultOption did not ask for. Bit 0x1 tags, 0x2 ratings, 0x4 meta binary.
func applyResultOption(list types.List[datastore_super_mario_maker_types.DataStoreCustomRankingResult], option types.UInt8) {
	for i := range list {
		if option&0x1 == 0 {
			list[i].MetaInfo.Tags = types.NewList[types.String]()
		}

		if option&0x2 == 0 {
			list[i].MetaInfo.Ratings = types.NewList[datastore_types.DataStoreRatingInfoWithSlot]()
		}

		if option&0x4 == 0 {
			list[i].MetaInfo.MetaBinary = types.NewQBuffer(nil)
		}
	}
}

// LatestCourseSearchObject is the newest-first course list.
func LatestCourseSearchObject(err error, packet nex.PacketInterface, callID uint32, param datastore_types.DataStoreSearchParam, extraData types.List[types.String]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	globals.Logger.Infof("LatestCourseSearchObject range=%d+%d extra=%v", param.ResultRange.Offset, param.ResultRange.Length, extraData)

	list, nexError := datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{
		Newest: true,
		Offset: int(param.ResultRange.Offset),
		Limit:  clampLength(param.ResultRange.Length),
	})
	if nexError != nil {
		return nil, nexError
	}

	return writeCourseList(callID, datastore_super_mario_maker.MethodLatestCourseSearchObject, list), nil
}

// GetCustomRanking lists courses (or makers, for the maker star ranking) by a custom ranking
// selected by ApplicationID: the star ranking is 0, the windowed rankings are type*100000000 +
// the date of the window start.
func GetCustomRanking(err error, packet nex.PacketInterface, callID uint32, param datastore_super_mario_maker_types.DataStoreGetCustomRankingParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	globals.Logger.Infof("GetCustomRanking app=%d range=%d+%d option=%#x", param.ApplicationID, param.ResultRange.Offset, param.ResultRange.Length, param.ResultOption)

	appID := uint32(param.ApplicationID)
	filter := datastore_smm_db.CourseFilter{
		AppID:  appID,
		Makers: datastore_smm_db.IsMakerRanking(appID),
		Offset: int(param.ResultRange.Offset),
		Limit:  clampLength(param.ResultRange.Length),
	}

	list, nexError := datastore_smm_db.QueryCourses(filter)
	if nexError != nil {
		return nil, nexError
	}

	// * A windowed ranking is empty until somebody stars something in the current window, and the
	// * client shows an error for an empty list. Fall back to the latest earlier window that has data.
	if len(list) == 0 && appID >= 100000000 {
		if prev, ok := datastore_smm_db.PreviousRankingPeriod(appID); ok {
			globals.Logger.Infof("GetCustomRanking app=%d is empty, falling back to %d", appID, prev)
			filter.AppID = prev
			list, nexError = datastore_smm_db.QueryCourses(filter)
			if nexError != nil {
				return nil, nexError
			}
		}
	}

	applyResultOption(list, param.ResultOption)

	// * Like GetCustomRankingByDataID, a QResult per ranking follows the list
	stream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)
	list.WriteTo(stream)

	results := make(types.List[types.QResult], 0, len(list))
	for range list {
		results = append(results, types.NewQResultSuccess(nex.ResultCodes.Core.Unknown))
	}
	results.WriteTo(stream)

	rmc := nex.NewRMCSuccess(globals.SecureEndpoint, stream.Bytes())
	rmc.ProtocolID = datastore_super_mario_maker.ProtocolID
	rmc.MethodID = datastore_super_mario_maker.MethodGetCustomRanking
	rmc.CallID = callID

	return rmc, nil
}

// ScoreRangeCascadedSearchObject is called by the 100 Mario Challenge with
// extraData = [rankingAppID, minScore, maxScore, flag] (seen: "300000000" "1" "49" "0"). It returns
// random courses whose score in that custom ranking is inside [min, max]. When nothing matches
// (a young server has no rows in that ranking) it widens to any course so the challenge can start.
func ScoreRangeCascadedSearchObject(err error, packet nex.PacketInterface, callID uint32, param datastore_types.DataStoreSearchParam, extraData types.List[types.String]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	globals.Logger.Infof("ScoreRangeCascadedSearchObject range=%d+%d extra=%v", param.ResultRange.Offset, param.ResultRange.Length, extraData)

	length := clampLength(param.ResultRange.Length)

	var list types.List[datastore_super_mario_maker_types.DataStoreCustomRankingResult]
	if len(extraData) >= 3 {
		app, errApp := strconv.ParseUint(string(extraData[0]), 10, 32)
		minScore, errMin := strconv.ParseInt(string(extraData[1]), 10, 64)
		maxScore, errMax := strconv.ParseInt(string(extraData[2]), 10, 64)
		if errApp == nil && errMin == nil && errMax == nil {
			var nexError *nex.Error
			list, nexError = datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{
				AppID: uint32(app), Random: true, ScoreMin: &minScore, ScoreMax: &maxScore, Limit: length,
			})
			if nexError != nil {
				return nil, nexError
			}
		}
	}

	if len(list) == 0 {
		var nexError *nex.Error
		list, nexError = datastore_smm_db.GetRandomCoursesWithLimit(length)
		if nexError != nil {
			return nil, nexError
		}
	}

	return writeCourseList(callID, datastore_super_mario_maker.MethodScoreRangeCascadedSearchObject, list), nil
}

// BestScoreRateCourseSearchObject backs a star-ranked course list. Its extraData is logged so the
// filter can be decoded from a real client; until then it lists the most starred courses, and
// applies a failure-rate range when extraData carries one in the Recommended layout.
func BestScoreRateCourseSearchObject(err error, packet nex.PacketInterface, callID uint32, param datastore_types.DataStoreSearchParam, extraData types.List[types.String]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	globals.Logger.Infof("BestScoreRateCourseSearchObject range=%d+%d extra=%v", param.ResultRange.Offset, param.ResultRange.Length, extraData)

	filter := datastore_smm_db.CourseFilter{
		Offset: int(param.ResultRange.Offset),
		Limit:  clampLength(param.ResultRange.Length),
	}
	if lo, hi, ok := ParseDifficultyExtra(extraData); ok {
		filter.FailMin, filter.FailMax = &lo, &hi
	}

	list, nexError := datastore_smm_db.QueryCourses(filter)
	if nexError != nil {
		return nil, nexError
	}

	return writeCourseList(callID, datastore_super_mario_maker.MethodBestScoreRateCourseSearchObject, list), nil
}
