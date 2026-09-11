package local_matchmake_extension

import (
	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmake_extension_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

func FindMatchmakeSessionBySingleGatheringId(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	globals.MatchmakingManager.Mutex.RLock()

	findBySingleGatheringIDResults, _, nexError := matchmake_extension_database.GetMatchmakeSessionByID(globals.MatchmakingManager, endpoint, uint32(gid))
	if nexError != nil {
		globals.Logger.Error(nexError.Error())
		globals.MatchmakingManager.Mutex.RUnlock()
		return nil, nexError
	}

	globals.MatchmakingManager.Mutex.RUnlock()

	rmcResponseStream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	findBySingleGatheringIDResults.WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(endpoint, rmcResponseBody)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.MethodID = matchmake_extension.MethodFindMatchmakeSessionBySingleGatheringID
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
