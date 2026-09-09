package mps

// The two matchmaking methods Mario Party Superstars needs and nex-protocols-common-go does
// not implement. Both are how the game follows a friend into a lobby rather than starting a
// new one, so without them "join my friend's room" cannot work at all — the game falls back
// to public auto-matchmaking and the joiner silently ends up in a session of their own.
//
//	MatchmakeExtension (0x6D) method 0x29  FindMatchmakeSessionByGatheringIDDetail
//	MatchmakeExtension (0x6D) method 0x33  FindMatchmakeSessionByParticipant
//
// Everything they need from the database already exists in the common library's own
// helpers, so these are two adapters, not a second matchmaking implementation.

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	match_making_constants "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/constants"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

// maxSessionsByParticipant is MAX_MATCHMAKE_SESSION_BY_PARTICIPANT, the ceiling the common
// library already applies to the equivalent GetPlayingSession call.
const maxSessionsByParticipant = 1000

func registerFriendJoin(protocol *matchmake_extension.Protocol, manager *common_globals.MatchmakingManager) {
	protocol.SetHandlerFindMatchmakeSessionByGatheringIDDetail(
		func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
			return findByGatheringIDDetail(manager, err, packet, callID, gid)
		})
	protocol.SetHandlerFindMatchmakeSessionByParticipant(
		func(err error, packet nex.PacketInterface, callID uint32, param match_making_types.FindMatchmakeSessionByParticipantParam) (*nex.RMCMessage, *nex.Error) {
			return findByParticipant(manager, err, packet, callID, param)
		})
}

// findByGatheringIDDetail answers "tell me about this lobby", the call the game's Join Room
// screen makes about once a second against a gathering ID it learned from friend presence.
// A gathering that is gone answers RendezVous::SessionVoid, which is what the game expects
// to see for a room that has closed.
func findByGatheringIDDetail(manager *common_globals.MatchmakingManager, err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}

	endpoint := packet.Sender().Endpoint().(*nex.PRUDPEndPoint)

	manager.Mutex.RLock()
	session, _, nexErr := database.GetMatchmakeSessionByID(manager, endpoint, uint32(gid))
	manager.Mutex.RUnlock()
	if nexErr != nil {
		return nil, nexErr
	}

	// The session key and passwords are the host's, not the caller's, and this method is
	// answered to anyone who knows the gathering ID. Joining is what hands those over.
	session.SessionKey = nil
	session.UserPassword = ""

	return respond(endpoint, callID, matchmake_extension.MethodFindMatchmakeSessionByGatheringIDDetail, session), nil
}

// findByParticipant answers "which lobby are these players in", the call a joiner makes
// about the host before trying to join. Answering it empty is what makes the game create a
// brand-new session instead of joining the friend's.
func findByParticipant(manager *common_globals.MatchmakingManager, err error, packet nex.PacketInterface, callID uint32, param match_making_types.FindMatchmakeSessionByParticipantParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}

	// MAX_PRINCIPALID_SIZE_TO_FIND_MATCHMAKE_SESSION, the same bound the common library's
	// GetPlayingSession applies to its own PID list.
	if len(param.PrincipalIDList) > 300 {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "principal ID list is over MAX_PRINCIPALID_SIZE_TO_FIND_MATCHMAKE_SESSION")
	}

	endpoint := packet.Sender().Endpoint().(*nex.PRUDPEndPoint)

	manager.Mutex.RLock()
	playing, nexErr := database.GetPlayingSession(manager, param.PrincipalIDList)
	manager.Mutex.RUnlock()
	if nexErr != nil {
		return nil, nexErr
	}

	results := make(types.List[match_making_types.FindMatchmakeSessionByParticipantResult], 0, len(playing))
	for _, session := range playing {
		if len(results) >= maxSessionsByParticipant {
			break
		}
		matchmakeSession, ok := session.Gathering.Object.(match_making_types.MatchmakeSession)
		if !ok {
			continue // a non-MatchmakeSession gathering is not something this method reports
		}

		result := match_making_types.NewFindMatchmakeSessionByParticipantResult()
		result.PrincipalID = session.PrincipalID
		result.Session = applyResultOption(matchmakeSession, param.ResultOptions)
		results = append(results, result)
	}

	return respond(endpoint, callID, matchmake_extension.MethodFindMatchmakeSessionByParticipant, results), nil
}

// applyResultOption drops the two fields the caller did not ask for. The client sizes its
// own buffers from what it requested, so sending more than that is not a kindness.
func applyResultOption(session match_making_types.MatchmakeSession, options match_making_constants.FindMatchmakeSessionResultOption) match_making_types.MatchmakeSession {
	if !options.HasFlag(match_making_constants.FindMatchmakeSessionResultOptionApplicationBuffer) {
		session.ApplicationBuffer = nil
	}
	if !options.HasFlag(match_making_constants.FindMatchmakeSessionResultOptionMatchmakeParam) {
		session.MatchmakeParam = match_making_types.NewMatchmakeParam()
	}
	return session
}

func respond(endpoint *nex.PRUDPEndPoint, callID uint32, methodID uint32, value types.RVType) *nex.RMCMessage {
	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	value.WriteTo(stream)

	response := nex.NewRMCSuccess(endpoint, stream.Bytes())
	response.ProtocolID = matchmake_extension.ProtocolID
	response.MethodID = methodID
	response.CallID = callID
	return response
}
