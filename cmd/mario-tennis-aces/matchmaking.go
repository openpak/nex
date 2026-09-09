package main

// The two matchmaking behaviours this title needs that nex-protocols-common-go does
// not provide. Everything else about match-making, matchmake-extension and
// match-making-ext is the common implementation, unmodified.

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

// openParticipationOnCreate marks a session open at the moment it is created.
//
// This title creates a session closed and only opens it with a later
// OpenParticipation call. A search only ever returns sessions whose
// open_participation is true, so two players searching inside that window each
// miss the other's session, each create one, and both then wait for a join that
// never comes. Marking the session open as it is created closes the race.
//
// Every method that can create a session is wrapped, because which one this title
// uses has not been seen on the wire yet.
func openParticipationOnCreate(protocol *matchmake_extension.Protocol) {
	createSession := protocol.CreateMatchmakeSession
	protocol.SetHandlerCreateMatchmakeSession(func(err error, packet nex.PacketInterface, callID uint32, anyGathering match_making_types.GatheringHolder, message types.String, participationCount types.UInt16) (*nex.RMCMessage, *nex.Error) {
		return createSession(err, packet, callID, openGathering(anyGathering), message, participationCount)
	})

	createWithParam := protocol.CreateMatchmakeSessionWithParam
	protocol.SetHandlerCreateMatchmakeSessionWithParam(func(err error, packet nex.PacketInterface, callID uint32, param match_making_types.CreateMatchmakeSessionParam) (*nex.RMCMessage, *nex.Error) {
		param.SourceMatchmakeSession = openSession(param.SourceMatchmakeSession)
		return createWithParam(err, packet, callID, param)
	})

	autoMatchmake := protocol.AutoMatchmakePostpone
	protocol.SetHandlerAutoMatchmakePostpone(func(err error, packet nex.PacketInterface, callID uint32, anyGathering match_making_types.GatheringHolder, message types.String) (*nex.RMCMessage, *nex.Error) {
		return autoMatchmake(err, packet, callID, openGathering(anyGathering), message)
	})

	autoMatchmakeWithParam := protocol.AutoMatchmakeWithParamPostpone
	protocol.SetHandlerAutoMatchmakeWithParamPostpone(func(err error, packet nex.PacketInterface, callID uint32, param match_making_types.AutoMatchmakeParam) (*nex.RMCMessage, *nex.Error) {
		param.SourceMatchmakeSession = openSession(param.SourceMatchmakeSession)
		return autoMatchmakeWithParam(err, packet, callID, param)
	})

	autoMatchmakeWithCriteria := protocol.AutoMatchmakeWithSearchCriteriaPostpone
	protocol.SetHandlerAutoMatchmakeWithSearchCriteriaPostpone(func(err error, packet nex.PacketInterface, callID uint32, criteria types.List[match_making_types.MatchmakeSessionSearchCriteria], anyGathering match_making_types.GatheringHolder, message types.String) (*nex.RMCMessage, *nex.Error) {
		return autoMatchmakeWithCriteria(err, packet, callID, criteria, openGathering(anyGathering), message)
	})
}

// openGathering opens the MatchmakeSession a gathering holder carries. A holder
// carrying anything else is passed through untouched: the common handler rejects
// it, and that rejection is its business, not ours.
func openGathering(holder match_making_types.GatheringHolder) match_making_types.GatheringHolder {
	if session, ok := holder.Object.(match_making_types.MatchmakeSession); ok {
		holder.Object = openSession(session)
	}
	return holder
}

func openSession(session match_making_types.MatchmakeSession) match_making_types.MatchmakeSession {
	session.OpenParticipation = types.NewBool(true)
	return session
}

// findMatchmakeSessionByParticipant answers "which session is this player in", the
// call behind playing with a friend. nex-protocols-common-go has no handler for it,
// so without this the title answers Core::NotImplemented and a friend's open room is
// never found. The lookup itself is the common database's playing-session query.
func findMatchmakeSessionByParticipant(protocol *matchmake_extension.Protocol, manager *common_globals.MatchmakingManager) {
	protocol.SetHandlerFindMatchmakeSessionByParticipant(func(err error, packet nex.PacketInterface, callID uint32, param match_making_types.FindMatchmakeSessionByParticipantParam) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}

		endpoint := packet.Sender().Endpoint()

		manager.Mutex.RLock()
		playing, nexError := database.GetPlayingSession(manager, param.PrincipalIDList)
		manager.Mutex.RUnlock()
		if nexError != nil {
			return nil, nexError
		}

		results := make(types.List[match_making_types.FindMatchmakeSessionByParticipantResult], 0, len(playing))
		for _, session := range playing {
			matchmakeSession, ok := session.Gathering.Object.(match_making_types.MatchmakeSession)
			if !ok {
				continue
			}
			result := match_making_types.NewFindMatchmakeSessionByParticipantResult()
			result.PrincipalID = session.PrincipalID
			result.Session = matchmakeSession
			results = append(results, result)
		}

		// ponytail: the block list in param.BlockListParam is ignored, because OpenPak
		// has no block list to consult yet. Filter here once the account service has one.
		stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
		results.WriteTo(stream)

		response := nex.NewRMCSuccess(endpoint, stream.Bytes())
		response.ProtocolID = matchmake_extension.ProtocolID
		response.MethodID = matchmake_extension.MethodFindMatchmakeSessionByParticipant
		response.CallID = callID
		return response, nil
	})
}
