package matchmaking

import (
	"github.com/openpak/nex/titles/monster-hunter-3-ultimate/globals"
	nex_matchmaking_database "github.com/openpak/nex/titles/monster-hunter-3-ultimate/nex/match-making/database"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
)

func FindByOwner(err error, packet nex.PacketInterface, callID uint32, id types.PID, resultRange types.ResultRange) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	gatheringHolders, nexError := nex_matchmaking_database.FindPersistentGatheringsByOwner(globals.MatchmakingManager, packet.Sender().(*nex.PRUDPConnection), id, connection.PID(), resultRange)
	if nexError != nil {
		globals.Logger.Error(nexError.Error())
		return nil, nexError
	}

	rmcResponseStream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	gatheringHolders.WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(endpoint, rmcResponseBody)
	rmcResponse.ProtocolID = match_making.ProtocolID
	rmcResponse.MethodID = match_making.MethodFindByOwner
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
