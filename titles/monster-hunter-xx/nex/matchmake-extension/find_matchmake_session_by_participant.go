package local_matchmake_extension

import (
	"github.com/PretendoNetwork/nex-go/v2"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmaking_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	local_matchmake_extension_database "github.com/PretendoNetwork/monster-hunter-xx/nex/matchmake-extension/database"
)

func FindMatchmakeSessionByParticipant(err error, packet nex.PacketInterface, callID uint32, param matchmaking_types.FindMatchmakeSessionByParticipantParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	globals.MatchmakingManager.Mutex.RLock()

	findByParticipantResults, nexError := local_matchmake_extension_database.FindMatchmakeSessionByParticipant(globals.MatchmakingManager, packet.Sender().(*nex.PRUDPConnection), param)
	if nexError != nil {
		globals.Logger.Error(nexError.Error())
		globals.MatchmakingManager.Mutex.RUnlock()
		return nil, nexError
	}

	globals.MatchmakingManager.Mutex.RUnlock()

	rmcResponseStream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	findByParticipantResults.WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(endpoint, rmcResponseBody)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.MethodID = matchmake_extension.MethodFindMatchmakeSessionByParticipant
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
