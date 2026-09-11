package local_matchmake_extension

import (
	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmake_extension_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

func UpdateMatchmakeSessionAttribute(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32, attribs types.List[types.UInt32]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	globals.MatchmakingManager.Mutex.Lock()

	session, _, nexError := matchmake_extension_database.GetMatchmakeSessionByID(globals.MatchmakingManager, endpoint, uint32(gid))
	if nexError != nil {
		globals.MatchmakingManager.Mutex.Unlock()
		return nil, nexError
	}

	if !session.Gathering.OwnerPID.Equals(connection.PID()) {
		globals.MatchmakingManager.Mutex.Unlock()
		return nil, nex.NewError(nex.ResultCodes.RendezVous.PermissionDenied, "change_error")
	}

	// Hopefully this works and doesn't break any legitimate requests
	if len(attribs) > len(session.Attributes) {
		globals.MatchmakingManager.Mutex.Unlock()
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	for i, attrib := range attribs {
		nexError := matchmake_extension_database.UpdateGameAttribute(globals.MatchmakingManager, uint32(gid), uint32(i), uint32(attrib))
		if nexError != nil {
			globals.Logger.Error(nexError.Error())
			globals.MatchmakingManager.Mutex.Unlock()
			return nil, nexError
		}
	}

	globals.MatchmakingManager.Mutex.Unlock()

	rmcResponse := nex.NewRMCSuccess(endpoint, nil)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.MethodID = matchmake_extension.MethodUpdateMatchmakeSessionAttribute
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
