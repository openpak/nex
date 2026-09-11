package local_matchmake_extension

import (
	"math"

	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

func BrowseMatchmakeSessionNoHolderNoResultRange(err error, packet nex.PacketInterface, callID uint32, searchCriteria match_making_types.MatchmakeSessionSearchCriteria) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	if searchCriteria.ResultRange.Offset == math.MaxUint32 {
		searchCriteria.ResultRange.Offset = 0
	}

	searchCriterias := []match_making_types.MatchmakeSessionSearchCriteria{searchCriteria}

	// Just a default in-case the search criteria doesn't have one set
	resultRange := types.NewResultRange()
	resultRange.Length = 50

	globals.MatchmakingManager.Mutex.RLock()

	sessions, nexError := database.FindMatchmakeSessionBySearchCriteria(globals.MatchmakingManager, connection, searchCriterias, resultRange, nil)
	if nexError != nil {
		globals.MatchmakingManager.Mutex.RUnlock()
		return nil, nexError
	}

	for i := range sessions {
		// * Scrap session key and user password
		sessions[i].SessionKey = make([]byte, 0)
		sessions[i].UserPassword = ""
	}

	globals.MatchmakingManager.Mutex.RUnlock()

	rmcResponseStream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	types.List[match_making_types.MatchmakeSession](sessions).WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(endpoint, rmcResponseBody)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.MethodID = matchmake_extension.MethodBrowseMatchmakeSessionNoHolderNoResultRange
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
