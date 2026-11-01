package nex_datastore_super_mario_maker

import (
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_super_mario_maker "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	datastore_smm_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore/super-mario-maker"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
)

func RecommendedCourseSearchObject(err error, packet nex.PacketInterface, callID uint32, param datastore_types.DataStoreSearchParam, extraData types.List[types.String]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	// * This method is used in 100 Mario and Course World
	// *
	// * extraData seems to be a set of filters defining a
	// * range for a courses success rate
	// *
	// * Course World (All)          ["",  "",   "",    "0", "0"]
	// * Course World (Easy)         ["1", "0",  "34",  "0", "0"]
	// * Course World (Normal)       ["1", "35", "74",  "0", "0"]
	// * Course World (Expert)       ["1", "75", "95",  "0", "0"]
	// * Course World (Super Expert) ["1", "96", "100", "0", "0"]
	// *
	// * Indexes 1 and 2 are the inclusive min and max *failure* rate
	// * of the courses (see ParseDifficultyExtra). The failure rate is
	// * 100 minus the clear rate, from the course's clear and attempt
	// * ratings (slots 2 and 3). Courses nobody has attempted have no
	// * rate, so they only appear in the unfiltered "All" list.
	// * The last 2 values always seem to be 0, and the first seems
	// * to always be 1 besides filtering for "All"

	// HACK The database load is exponential here and
	length := int(param.ResultRange.Length)
	maxLength := 100 // * 100 Mario asks for ONE_HUNDRED_MARIO_RECOMMENDED_COURSE_COUNT at once
	if length <= 0 || length > maxLength {
		globals.Logger.Warningf("Limiting request to %d courses (was %d)", maxLength, length)
		length = maxLength
	}

	filter := datastore_smm_db.CourseFilter{Random: true, Limit: length}
	if lo, hi, ok := ParseDifficultyExtra(extraData); ok {
		filter.FailMin, filter.FailMax = &lo, &hi
	}

	// TODO - Use the offet? Real client never uses it, but might be nice for completeness sake?
	pRankingResults, nexError := datastore_smm_db.QueryCourses(filter)
	if nexError != nil {
		return nil, nexError
	}

	rmcResponseStream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)

	pRankingResults.WriteTo(rmcResponseStream)

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, rmcResponseStream.Bytes())
	rmcResponse.ProtocolID = datastore_super_mario_maker.ProtocolID
	rmcResponse.MethodID = datastore_super_mario_maker.MethodRecommendedCourseSearchObject
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
