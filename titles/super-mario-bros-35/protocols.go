package smb35

// Ranking2 (0x7A) and MatchmakeReferee (0x78): the two protocols this game calls that
// nex-protocols-common-go has no implementation of. nex-protocols-go still supplies the
// wire types and the method dispatch, so what is written here is only the behaviour.
//
// Both stores are in memory on purpose. Ranking2 common data is the player's name and Mii,
// which the game uploads again every time it connects; a round is a single match in
// progress. Neither survives a restart usefully, and neither is worth a table.

import (
	"fmt"
	"github.com/openpak/nex/redact"
	"sync"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmake_extension_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	matchmake_referee "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-referee"
	matchmake_referee_types "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-referee/types"
	notifications_constants "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
	ranking2 "github.com/PretendoNetwork/nex-protocols-go/v2/ranking2"
	ranking2_constants "github.com/PretendoNetwork/nex-protocols-go/v2/ranking2/constants"
	ranking2_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking2/types"
)

// ---------------------------------------------------------------------------
// Ranking2 (0x7A)
// ---------------------------------------------------------------------------

// ranking2Store holds each player's common data, keyed by PID and then by the NEX unique
// ID the game files it under.
type ranking2Store struct {
	mu   sync.RWMutex
	data map[uint64]map[uint64]ranking2_types.Ranking2CommonData
}

func newRanking2Store() *ranking2Store {
	return &ranking2Store{data: make(map[uint64]map[uint64]ranking2_types.Ranking2CommonData)}
}

func (r *ranking2Store) get(pid, uniqueID uint64) (ranking2_types.Ranking2CommonData, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, ok := r.data[pid][uniqueID]
	return data, ok
}

func (r *ranking2Store) put(pid, uniqueID uint64, data ranking2_types.Ranking2CommonData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data[pid] == nil {
		r.data[pid] = make(map[uint64]ranking2_types.Ranking2CommonData)
	}
	r.data[pid][uniqueID] = data
}

func (s *Server) getCommonData(err error, packet nex.PacketInterface, callID uint32, _ ranking2_constants.Ranking2GetOptionFlags, principalID types.PID, nexUniqueID types.UInt64) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Ranking2.InvalidArgument, err.Error())
	}
	data, ok := s.common.get(uint64(principalID), uint64(nexUniqueID))
	if !ok {
		// The game's original server answers InvalidArgument for common data that was never put, and the
		// game treats it as "this player has nothing yet" rather than as a failure.
		return nil, nex.NewError(nex.ResultCodes.Ranking2.InvalidArgument, fmt.Sprintf("no common data for pid %s", redact.Tag(uint64(principalID))))
	}
	return ranking2Response(packet, callID, ranking2.MethodGetCommonData, data), nil
}

func (s *Server) putCommonData(err error, packet nex.PacketInterface, callID uint32, commonData ranking2_types.Ranking2CommonData, nexUniqueID types.UInt64) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Ranking2.InvalidArgument, err.Error())
	}
	s.common.put(uint64(packet.Sender().PID()), uint64(nexUniqueID), commonData)
	return emptyResponse(packet, callID, ranking2.ProtocolID, ranking2.MethodPutCommonData), nil
}

// getRanking answers an empty board. Super Mario Bros. 35 has no server-side leaderboard:
// earlier research notes record nothing here either, and no capture or document describes what
// a populated board would have contained. An empty board is a fact about the game, not a
// gap in this server.
func (s *Server) getRanking(err error, packet nex.PacketInterface, callID uint32, _ ranking2_types.Ranking2GetParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Ranking2.InvalidArgument, err.Error())
	}
	info := ranking2_types.NewRanking2Info()
	info.LowestRank = 10000
	info.NumRankedIn = 0
	info.Season = 0
	return ranking2Response(packet, callID, ranking2.MethodGetRanking, info), nil
}

// getCategorySetting answers the same setting for every category, which matches
// earlier research notes; the game asks before it shows a ranking screen and only needs the shape.
func (s *Server) getCategorySetting(err error, packet nex.PacketInterface, callID uint32, _ types.UInt32) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Ranking2.InvalidArgument, err.Error())
	}
	setting := ranking2_types.NewRanking2CategorySetting()
	setting.MinScore = 0
	setting.MaxScore = 999999999
	setting.LowestRank = 10000
	setting.ResetMonth = 4095 // every month: the bitmask with all twelve bits set
	setting.ResetDay = 0
	setting.ResetHour = 0
	setting.ResetMode = 2
	setting.MaxSeasonsToGoBack = 3
	setting.ScoreOrder = types.Bool(ranking2_constants.ScoreOrderAsc)
	return ranking2Response(packet, callID, ranking2.MethodGetCategorySetting, setting), nil
}

// getEstimateScoreRank answers zeroes, echoing back the category asked about. There is no
// board to estimate a rank against.
func (s *Server) getEstimateScoreRank(err error, packet nex.PacketInterface, callID uint32, input ranking2_types.Ranking2EstimateScoreRankInput) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Ranking2.InvalidArgument, err.Error())
	}
	output := ranking2_types.NewRanking2EstimateScoreRankOutput()
	output.Category = input.Category
	return ranking2Response(packet, callID, ranking2.MethodGetEstimateScoreRank, output), nil
}

// ---------------------------------------------------------------------------
// MatchmakeReferee (0x78)
// ---------------------------------------------------------------------------

// round is one match in progress. The parameters are kept as the raw request body rather
// than as a decoded structure.
//
// ponytail: Super Mario Bros. 35 uses the Eagle variant of MatchmakeRefereeStartRoundParam,
// which has two fields (reportSummaryMode, eventID) that nex-protocols-go's type does not.
// They are trailing fields, so the leading ones this server actually reads — the gathering
// ID and the participants — still decode correctly, and GetStartRoundParam echoes the body
// back verbatim so the client gets every field it sent. The ceiling is that this server
// cannot look at those two values. Upgrade path: declare the Eagle-variant type here and
// decode properly, the day anything needs to act on them.
type round struct {
	gid     uint32
	pids    []uint64
	params  []byte
	started time.Time
}

type roundStore struct {
	mu     sync.Mutex
	next   uint64
	rounds map[uint64]round
}

func newRoundStore() *roundStore {
	return &roundStore{rounds: make(map[uint64]round)}
}

// roundLifetime is how long a round is remembered. A match is minutes long; anything still
// here after this was abandoned without an EndRound and is only holding memory.
const roundLifetime = time.Hour

func (r *roundStore) start(rd round) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, old := range r.rounds {
		if time.Since(old.started) > roundLifetime {
			delete(r.rounds, id)
		}
	}
	r.next++
	rd.started = time.Now()
	r.rounds[r.next] = rd
	return r.next
}

func (r *roundStore) get(id uint64) (round, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rd, ok := r.rounds[id]
	return rd, ok
}

func (r *roundStore) end(id uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.rounds[id]
	delete(r.rounds, id)
	return ok
}

// startRound opens a round and tells every listed player it has begun.
//
// The caller names the players, so the server has to check they are really in the gathering
// they claim — otherwise any client could push a round-start notification at any other
// player on the server.
func (s *Server) startRound(err error, packet nex.PacketInterface, callID uint32, param matchmake_referee_types.MatchmakeRefereeStartRoundParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil || len(param.PIDs) == 0 {
		return nil, nex.NewError(nex.ResultCodes.MatchmakeReferee.InvalidArgument, "MatchmakeReferee::StartRound needs at least one participant")
	}
	if nexError := s.checkParticipants(uint32(param.GID), param.PIDs); nexError != nil {
		return nil, nexError
	}

	pids := make([]uint64, 0, len(param.PIDs))
	for _, pid := range param.PIDs {
		pids = append(pids, uint64(pid))
	}
	roundID := s.rounds.start(round{
		gid:    uint32(param.GID),
		pids:   pids,
		params: packet.RMCMessage().Parameters,
	})

	event := notifications_types.NewNotificationEvent()
	event.PIDSource = packet.Sender().PID()
	event.Type = notifications_constants.NotificationEvents(116000) // round started
	event.Param1 = types.UInt64(roundID)
	common_globals.SendNotificationEvent(s.host.Secure, event, pids)

	endpoint := packet.Sender().Endpoint()
	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	types.UInt64(roundID).WriteTo(stream)
	return rmcResponse(packet, callID, matchmake_referee.ProtocolID, matchmake_referee.MethodStartRound, stream.Bytes()), nil
}

// getStartRoundParam hands back exactly the bytes StartRound was called with. The request
// body of StartRound is the parameter structure and nothing else, and the response of this
// method is the same structure under the same library version, so echoing is not a shortcut
// around the encoding — it is the encoding, including the two fields this server does not
// decode. See the note on round.
func (s *Server) getStartRoundParam(err error, packet nex.PacketInterface, callID uint32, roundID types.UInt64) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.MatchmakeReferee.InvalidArgument, err.Error())
	}
	rd, ok := s.rounds.get(uint64(roundID))
	if !ok {
		return nil, nex.NewError(nex.ResultCodes.MatchmakeReferee.RoundNotFound, fmt.Sprintf("no round %d", roundID))
	}
	return rmcResponse(packet, callID, matchmake_referee.ProtocolID, matchmake_referee.MethodGetStartRoundParam, rd.params), nil
}

// endRound closes a round.
//
// ponytail: the per-player results the client reports are read past and discarded. They are
// in the Eagle variant's layout, which nex-protocols-go decodes incorrectly, and there is
// nothing in this game that reads them back — no leaderboard, no rating, no stats method the
// client calls. The upgrade path is the same as for StartRound: declare the variant type
// the day a result is worth keeping. The round ID is decoded before the results, so closing
// the round is reliable even when the results are not.
func (s *Server) endRound(_ error, packet nex.PacketInterface, callID uint32, param matchmake_referee_types.MatchmakeRefereeEndRoundParam) (*nex.RMCMessage, *nex.Error) {
	return s.closeRound(packet, callID, uint64(param.RoundID), matchmake_referee.MethodEndRound)
}

func (s *Server) endRoundWithoutReport(err error, packet nex.PacketInterface, callID uint32, roundID types.UInt64) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.MatchmakeReferee.InvalidArgument, err.Error())
	}
	return s.closeRound(packet, callID, uint64(roundID), matchmake_referee.MethodEndRoundWithoutReport)
}

func (s *Server) closeRound(packet nex.PacketInterface, callID uint32, roundID uint64, methodID uint32) (*nex.RMCMessage, *nex.Error) {
	if !s.rounds.end(roundID) {
		return nil, nex.NewError(nex.ResultCodes.MatchmakeReferee.RoundNotFound, fmt.Sprintf("no round %d", roundID))
	}
	return emptyResponse(packet, callID, matchmake_referee.ProtocolID, methodID), nil
}

// checkParticipants refuses a round whose named players are not all in the gathering.
func (s *Server) checkParticipants(gid uint32, pids types.List[types.PID]) *nex.Error {
	s.matchmaking.Mutex.RLock()
	defer s.matchmaking.Mutex.RUnlock()

	if nexError := matchmake_extension_database.CheckGatheringForParticipation(s.matchmaking, gid, pids); nexError != nil {
		return nex.NewError(nex.ResultCodes.MatchmakeReferee.NotParticipatedGathering, nexError.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

func ranking2Response(packet nex.PacketInterface, callID uint32, methodID uint32, value types.RVType) *nex.RMCMessage {
	endpoint := packet.Sender().Endpoint()
	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	value.WriteTo(stream)
	return rmcResponse(packet, callID, ranking2.ProtocolID, methodID, stream.Bytes())
}

func emptyResponse(packet nex.PacketInterface, callID uint32, protocolID uint16, methodID uint32) *nex.RMCMessage {
	return rmcResponse(packet, callID, protocolID, methodID, nil)
}

func rmcResponse(packet nex.PacketInterface, callID uint32, protocolID uint16, methodID uint32, body []byte) *nex.RMCMessage {
	response := nex.NewRMCSuccess(packet.Sender().Endpoint(), body)
	response.ProtocolID = protocolID
	response.MethodID = methodID
	response.CallID = callID
	return response
}
