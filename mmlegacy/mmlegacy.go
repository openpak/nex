// Package mmlegacy implements the five matchmaking methods that the 3DS and Wii U lineage
// titles call and that nex-protocols-common-go (v2.6.1) does not implement:
//
//	MatchMaking        0x15.0x16  FindByOwner
//	MatchmakeExtension 0x6D.0x0C  UpdateMatchmakeSessionAttribute
//	MatchmakeExtension 0x6D.0x31  FindMatchmakeSessionBySingleGatheringID
//	MatchmakeExtension 0x6D.0x34  BrowseMatchmakeSessionNoHolderNoResultRange
//	MatchmakeExtension 0x6D.0x33  FindMatchmakeSessionByParticipant
//
// Left unset, each of them answers Core::NotImplemented and the title stops there. The
// implementations are Pretendo's Monster Hunter XX server's (AGPL-3.0, in this repository at
// titles/monster-hunter-xx), lifted here with the matchmaking manager passed in rather than
// read from a package global, so a second title can call them: Monster Hunter Generations
// Ultimate is the Switch port of Monster Hunter XX and carries that game's NEX code, so it
// calls exactly this set.
//
// The SQL matches nex-protocols-common-go's own gathering queries: only registered,
// open-participation, unlocked, non-full MatchmakeSessions with a real host are visible.
package mmlegacy

import (
	"database/sql"
	"math"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	mme_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	pqextended "github.com/PretendoNetwork/pq-extended"
)

// Register attaches the five handlers to protocols the caller has already registered on its
// secure endpoint, and to the caller's matchmaking manager.
func Register(matchMaking *match_making.Protocol, matchmakeExtension *matchmake_extension.Protocol, manager *common_globals.MatchmakingManager) {
	matchMaking.FindByOwner = findByOwner(manager)
	matchmakeExtension.FindMatchmakeSessionByParticipant = findByParticipant(manager)
	matchmakeExtension.FindMatchmakeSessionBySingleGatheringID = findBySingleGatheringID(manager)
	matchmakeExtension.BrowseMatchmakeSessionNoHolderNoResultRange = browseNoHolderNoResultRange(manager)
	matchmakeExtension.UpdateMatchmakeSessionAttribute = updateSessionAttribute(manager)
}

// sessionColumns is the column list every query below scans, in the order scanSession reads
// them. One list, so a query and its scan cannot drift apart.
const sessionColumns = `g.id, g.owner_pid, g.host_pid, g.min_participants, g.max_participants,
	g.participation_policy, g.policy_argument, g.flags, g.state, g.description,
	array_length(g.participants, 1), g.started_time,
	ms.game_mode, ms.attribs, ms.open_participation, ms.matchmake_system_type,
	ms.application_buffer, ms.progress_score, ms.session_key, ms.option_zero,
	ms.matchmake_param, ms.user_password, ms.refer_gid, ms.user_password_enabled,
	ms.system_password_enabled, ms.codeword`

// joinableSessions is the WHERE every search shares: a session a stranger could actually
// join. A locked, full, unregistered or hostless gathering is not an answer to any of these
// methods, and returning one sends the console at a session it will be refused from.
const joinableSessions = `g.registered=true AND
	g.type='MatchmakeSession' AND
	g.host_pid <> 0 AND
	ms.open_participation=true AND
	array_length(g.participants, 1) < g.max_participants AND
	ms.user_password_enabled=false AND
	ms.system_password_enabled=false`

// scanSession reads one row into a MatchmakeSession. The matchmake parameter is stored as
// the bytes the console sent, so it is decoded with the endpoint's own stream settings.
func scanSession(rows *sql.Rows, endpoint *nex.PRUDPEndPoint, session *match_making_types.MatchmakeSession) error {
	var startedTime time.Time
	var attribs []uint32
	var matchmakeParam []byte

	err := rows.Scan(
		&session.Gathering.ID,
		&session.Gathering.OwnerPID,
		&session.Gathering.HostPID,
		&session.Gathering.MinimumParticipants,
		&session.Gathering.MaximumParticipants,
		&session.Gathering.ParticipationPolicy,
		&session.Gathering.PolicyArgument,
		&session.Gathering.Flags,
		&session.Gathering.State,
		&session.Gathering.Description,
		&session.ParticipationCount,
		&startedTime,
		&session.GameMode,
		pqextended.Array(&attribs),
		&session.OpenParticipation,
		&session.MatchmakeSystemType,
		&session.ApplicationBuffer,
		&session.ProgressScore,
		&session.SessionKey,
		&session.Option0,
		&matchmakeParam,
		&session.UserPassword,
		&session.ReferGID,
		&session.UserPasswordEnabled,
		&session.SystemPasswordEnabled,
		&session.CodeWord,
	)
	if err != nil {
		return err
	}

	session.StartedTime = session.StartedTime.FromTimestamp(startedTime)

	attributes := make([]types.UInt32, len(attribs))
	for i, value := range attribs {
		attributes[i] = types.NewUInt32(value)
	}
	session.Attributes = attributes

	stream := nex.NewByteStreamIn(matchmakeParam, endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	session.MatchmakeParam.ExtractFrom(stream)
	return nil
}

// respond writes body as a success for one protocol and method.
func respond(endpoint *nex.PRUDPEndPoint, protocolID uint16, methodID uint32, callID uint32, write func(types.Writable)) *nex.RMCMessage {
	var body []byte
	if write != nil {
		stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
		write(stream)
		body = stream.Bytes()
	}
	response := nex.NewRMCSuccess(endpoint, body)
	response.ProtocolID = protocolID
	response.MethodID = methodID
	response.CallID = callID
	return response
}

func endpointOf(packet nex.PacketInterface) (*nex.PRUDPConnection, *nex.PRUDPEndPoint) {
	connection := packet.Sender().(*nex.PRUDPConnection)
	return connection, connection.Endpoint().(*nex.PRUDPEndPoint)
}

// queryError maps a failed query the same way the common protocols do: no rows is an empty
// session, anything else is unknown.
func queryError(err error) *nex.Error {
	if err == sql.ErrNoRows {
		return nex.NewError(nex.ResultCodes.RendezVous.SessionVoid, err.Error())
	}
	return nex.NewError(nex.ResultCodes.Core.Unknown, err.Error())
}

// findByOwner is MatchMaking 0x15.0x16: every joinable session a given player owns.
func findByOwner(manager *common_globals.MatchmakingManager) func(error, nex.PacketInterface, uint32, types.PID, types.ResultRange) (*nex.RMCMessage, *nex.Error) {
	return func(err error, packet nex.PacketInterface, callID uint32, id types.PID, resultRange types.ResultRange) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			common_globals.Logger.Error(err.Error())
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		_, endpoint := endpointOf(packet)

		// A console that wants "everything" sends length 0 or offset 0xFFFFFFFF; neither is
		// a range, and passed through they mean LIMIT 0 and an offset past every row.
		if resultRange.Length == 0 {
			resultRange.Length = types.NewUInt32(50)
		}
		if resultRange.Offset == types.NewUInt32(math.MaxUint32) {
			resultRange.Offset = types.NewUInt32(0)
		}

		manager.Mutex.RLock()
		rows, qerr := manager.Database.Query(`SELECT `+sessionColumns+`
			FROM matchmaking.gatherings AS g
			INNER JOIN matchmaking.matchmake_sessions AS ms ON ms.id = g.id
			WHERE `+joinableSessions+` AND g.owner_pid=$1
			LIMIT $2 OFFSET $3`, id, resultRange.Length, resultRange.Offset)
		if qerr != nil {
			manager.Mutex.RUnlock()
			return nil, queryError(qerr)
		}

		holders := types.NewList[match_making_types.GatheringHolder]()
		for rows.Next() {
			session := match_making_types.NewMatchmakeSession()
			if serr := scanSession(rows, endpoint, &session); serr != nil {
				common_globals.Logger.Critical(serr.Error())
				continue
			}
			holder := match_making_types.NewGatheringHolder()
			holder.Object = session.Copy().(match_making_types.MatchmakeSession)
			holders = append(holders, holder)
		}
		rows.Close()
		manager.Mutex.RUnlock()

		return respond(endpoint, match_making.ProtocolID, match_making.MethodFindByOwner, callID, holders.WriteTo), nil
	}
}

// findByParticipant is MatchmakeExtension 0x6D.0x33: the sessions any of the named players
// are in. It is how a console follows a friend into a hunt.
func findByParticipant(manager *common_globals.MatchmakingManager) func(error, nex.PacketInterface, uint32, match_making_types.FindMatchmakeSessionByParticipantParam) (*nex.RMCMessage, *nex.Error) {
	return func(err error, packet nex.PacketInterface, callID uint32, param match_making_types.FindMatchmakeSessionByParticipantParam) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			common_globals.Logger.Error(err.Error())
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		_, endpoint := endpointOf(packet)

		manager.Mutex.RLock()
		rows, qerr := manager.Database.Query(`SELECT `+sessionColumns+`
			FROM matchmaking.gatherings AS g
			INNER JOIN matchmaking.matchmake_sessions AS ms ON ms.id = g.id
			WHERE `+joinableSessions+` AND g.owner_pid <> 0 AND g.participants && $1
			LIMIT 50`, pqextended.Array(param.PrincipalIDList))
		if qerr != nil {
			manager.Mutex.RUnlock()
			return nil, queryError(qerr)
		}

		results := types.NewList[match_making_types.FindMatchmakeSessionByParticipantResult]()
		for rows.Next() {
			result := match_making_types.NewFindMatchmakeSessionByParticipantResult()
			if serr := scanSession(rows, endpoint, &result.Session); serr != nil {
				common_globals.Logger.Critical(serr.Error())
				continue
			}
			results = append(results, result)
		}
		rows.Close()
		manager.Mutex.RUnlock()

		return respond(endpoint, matchmake_extension.ProtocolID, matchmake_extension.MethodFindMatchmakeSessionByParticipant, callID, results.WriteTo), nil
	}
}

// findBySingleGatheringID is MatchmakeExtension 0x6D.0x31: one session by its gathering ID.
func findBySingleGatheringID(manager *common_globals.MatchmakingManager) func(error, nex.PacketInterface, uint32, types.UInt32) (*nex.RMCMessage, *nex.Error) {
	return func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			common_globals.Logger.Error(err.Error())
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		_, endpoint := endpointOf(packet)

		manager.Mutex.RLock()
		session, _, nexError := mme_database.GetMatchmakeSessionByID(manager, endpoint, uint32(gid))
		manager.Mutex.RUnlock()
		if nexError != nil {
			return nil, nexError
		}

		return respond(endpoint, matchmake_extension.ProtocolID, matchmake_extension.MethodFindMatchmakeSessionBySingleGatheringID, callID, session.WriteTo), nil
	}
}

// browseNoHolderNoResultRange is MatchmakeExtension 0x6D.0x34: a search that carries its
// range inside the criteria instead of alongside it, and whose results are bare sessions
// rather than gathering holders.
func browseNoHolderNoResultRange(manager *common_globals.MatchmakingManager) func(error, nex.PacketInterface, uint32, match_making_types.MatchmakeSessionSearchCriteria) (*nex.RMCMessage, *nex.Error) {
	return func(err error, packet nex.PacketInterface, callID uint32, searchCriteria match_making_types.MatchmakeSessionSearchCriteria) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			common_globals.Logger.Error(err.Error())
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		connection, endpoint := endpointOf(packet)

		if searchCriteria.ResultRange.Offset == types.NewUInt32(math.MaxUint32) {
			searchCriteria.ResultRange.Offset = types.NewUInt32(0)
		}

		// The method's name is the whole point: there is no ResultRange argument, so the
		// cap has to come from somewhere. 50 is what the common browse uses.
		resultRange := types.NewResultRange()
		resultRange.Length = types.NewUInt32(50)

		manager.Mutex.RLock()
		sessions, nexError := mme_database.FindMatchmakeSessionBySearchCriteria(manager, connection,
			[]match_making_types.MatchmakeSessionSearchCriteria{searchCriteria}, resultRange, nil, false)
		manager.Mutex.RUnlock()
		if nexError != nil {
			return nil, nexError
		}

		// A browse result is a poster, not an invitation: the session key and the user
		// password are handed out by Join, to a console that got that far.
		for i := range sessions {
			sessions[i].SessionKey = make([]byte, 0)
			sessions[i].UserPassword = ""
		}

		list := types.List[match_making_types.MatchmakeSession](sessions)
		return respond(endpoint, matchmake_extension.ProtocolID, matchmake_extension.MethodBrowseMatchmakeSessionNoHolderNoResultRange, callID, list.WriteTo), nil
	}
}

// updateSessionAttribute is MatchmakeExtension 0x6D.0x0C: the owner rewrites its session's
// attribute slots in place. Answers nothing on success.
func updateSessionAttribute(manager *common_globals.MatchmakingManager) func(error, nex.PacketInterface, uint32, types.UInt32, types.List[types.UInt32]) (*nex.RMCMessage, *nex.Error) {
	return func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32, attribs types.List[types.UInt32]) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			common_globals.Logger.Error(err.Error())
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		connection, endpoint := endpointOf(packet)

		manager.Mutex.Lock()
		defer manager.Mutex.Unlock()

		session, _, nexError := mme_database.GetMatchmakeSessionByID(manager, endpoint, uint32(gid))
		if nexError != nil {
			return nil, nexError
		}
		if !session.Gathering.OwnerPID.Equals(connection.PID()) {
			return nil, nex.NewError(nex.ResultCodes.RendezVous.PermissionDenied, "not the session owner")
		}
		// Writing past the slots the session was created with would grow the array in the
		// database and desync it from what every other reader expects.
		if len(attribs) > len(session.Attributes) {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "more attributes than the session has slots")
		}

		for i, attrib := range attribs {
			if nexError := mme_database.UpdateGameAttribute(manager, uint32(gid), uint32(i), uint32(attrib)); nexError != nil {
				common_globals.Logger.Error(nexError.Error())
				return nil, nexError
			}
		}

		return respond(endpoint, matchmake_extension.ProtocolID, matchmake_extension.MethodUpdateMatchmakeSessionAttribute, callID, nil), nil
	}
}
