package nex

// The bridge's sink half (card openpak/prds#177): a game a player hosts on another console is listed here as a gathering owned by the
// hosting player's Wii U PID, with no NEX connection behind it, so the host's Wii U friends can join it through the game's own Join
// tab (friend presence, nn-friends) and JoinMatchmakeSession. common/minecraft's session directory keeps it in step (bearer
// MINECRAFT_INTERNAL_KEY, same listener and key as the routes in bridge.go):
//
//	PUT    /internal/minecraft/bridged/{game}  {game, host_pid, application_buffer (hex, 212 B), max_participants, open}: create or
//	                                           update; answers {gathering}
//	DELETE /internal/minecraft/bridged/{game}  take it off the lists
//	GET    /internal/minecraft/joins?after=N   what Wii Us did with bridged gatherings (join, session URLs, probe data), in full
//
// The gathering's description is "openpak-bridge:<game>": that marks it as bridged, so it is never reported back as a console-hosted game.
// A Wii U that joins gets the normal JoinMatchmakeSession reply and, from GetSessionURLs, the bridge's station (the game traffic leg is
// a separate piece); every step is logged and kept in the joins feed for the directory.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonglobals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles/minecraft-wiiu/globals"
)

const bridgeMarker = "openpak-bridge:"

// bridgedRVCID is the connection id of a bridged host's station URL: another value than the world's (worldRVCID) so a probe request for it
// is told apart, and one no live NEX connection has.
const bridgedRVCID = 0xFFF2

func mountBridged() {
	nexhost.HealthMux.HandleFunc("PUT /internal/minecraft/bridged/{game}", bridgeAuth(bridgedPut))
	nexhost.HealthMux.HandleFunc("DELETE /internal/minecraft/bridged/{game}", bridgeAuth(bridgedDelete))
	nexhost.HealthMux.HandleFunc("GET /internal/minecraft/joins", bridgeAuth(bridgedJoins))
}

type bridgedReq struct {
	Game              string `json:"game"`
	HostPID           uint32 `json:"host_pid"`
	ApplicationBuffer string `json:"application_buffer"`
	MaxParticipants   int    `json:"max_participants"`
	Open              bool   `json:"open"`
}

func validGameID(s string) bool {
	if s == "" || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == ':' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// bridgedAttribs is attribute 0 of a bridged friends game. The capture's row carries 0x30881 (the public-game marker); a bridged game is
// never public, so the marker is left out and only the net version stays (guess: the real value of a friends-only game is not captured).
const bridgedAttribs = "{2177,0,0,0,0,0}"

func bridgedPut(w http.ResponseWriter, r *http.Request) {
	var q bridgedReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&q); err != nil || q.Game != r.PathValue("game") || !validGameID(q.Game) || q.HostPID == 0 || uint64(q.HostPID) == uint64(worldPID()) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	buf, err := hex.DecodeString(q.ApplicationBuffer)
	if err != nil || len(buf) != 212 || buf[0] != 0x08 || buf[1] != 0x81 {
		http.Error(w, "application_buffer: want 212 bytes of a Wii U v688 buffer (net version 0x0881)", http.StatusBadRequest)
		return
	}
	if q.MaxParticipants < 1 || q.MaxParticipants > 64 {
		http.Error(w, "max_participants out of range", http.StatusBadRequest)
		return
	}
	m := globals.MatchmakingManager
	m.Mutex.Lock()
	defer m.Mutex.Unlock()
	var gid int64
	err = globals.Postgres.QueryRow(`SELECT id FROM matchmaking.gatherings WHERE description = $1 ORDER BY id DESC LIMIT 1`, bridgeMarker+q.Game).Scan(&gid)
	switch {
	case err == nil:
		if _, err = globals.Postgres.Exec(`UPDATE matchmaking.gatherings SET max_participants = $2, owner_pid = $3, host_pid = $3, registered = true WHERE id = $1`,
			gid, q.MaxParticipants, int64(q.HostPID)); err == nil {
			_, err = globals.Postgres.Exec(`UPDATE matchmaking.matchmake_sessions SET application_buffer = $2, open_participation = $3 WHERE id = $1`, gid, buf, q.Open)
		}
	case err == sql.ErrNoRows:
		gid, err = createBridged(q, buf)
	}
	if err != nil {
		globals.Logger.Errorf("minecraft bridged %s: %v", q.Game, err)
		http.Error(w, "database", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"gathering": gid})
}

// createBridged writes the rows of a new bridged gathering in the shape of the real host's gathering of the 2026-11-07 capture (ensureWorld):
// game mode 15, participation policy 95, flags 2560; the host is its only participant.
func createBridged(q bridgedReq, buf []byte) (int64, error) {
	key := make([]byte, 32)
	rand.Read(key)
	tx, err := globals.Postgres.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var gid int64
	if err := tx.QueryRow(`INSERT INTO matchmaking.gatherings (owner_pid, host_pid, min_participants, max_participants, participation_policy, policy_argument,
		flags, state, description, registered, type, started_time, participants) VALUES ($1,$1,0,$2,95,0,2560,0,$3,true,'MatchmakeSession',now(),ARRAY[$1::numeric]) RETURNING id`,
		int64(q.HostPID), q.MaxParticipants, bridgeMarker+q.Game).Scan(&gid); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO matchmaking.matchmake_sessions (id, game_mode, attribs, open_participation, matchmake_system_type, application_buffer,
		progress_score, session_key, option_zero, matchmake_param, refer_gid, user_password_enabled, system_password_enabled, codeword)
		VALUES ($1,15,$2,$3,1,$4,100,$5,0,$6,0,false,false,'')`, gid, bridgedAttribs, q.Open, buf, key, worldMatchmakeParam); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	globals.Logger.Infof("minecraft bridged game %s: gathering %d owned by PID %d", q.Game, gid, q.HostPID)
	return gid, nil
}

func bridgedDelete(w http.ResponseWriter, r *http.Request) {
	game := r.PathValue("game")
	if !validGameID(game) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	m := globals.MatchmakingManager
	m.Mutex.Lock()
	res, err := globals.Postgres.Exec(`UPDATE matchmaking.gatherings SET registered = false, participants = ARRAY[]::numeric[] WHERE description = $1 AND registered`, bridgeMarker+game)
	m.Mutex.Unlock()
	if err != nil {
		http.Error(w, "database", http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	globals.Logger.Infof("minecraft bridged game %s withdrawn (%d gatherings)", game, n)
	w.Write([]byte("ok\n"))
}

// bridgedGame is the game id behind a bridged gathering ("" when the gathering is not bridged or not listed any more).
func bridgedGame(gid uint32) string {
	var d string
	if globals.Postgres.QueryRow(`SELECT description FROM matchmaking.gatherings WHERE id = $1 AND registered`, int64(gid)).Scan(&d) != nil {
		return ""
	}
	return strings.TrimPrefix(d, bridgeMarker)
}

// bridgedByHost is the bridged gathering a host PID owns, for a probe request that names only the station.
func bridgedByHost(pid uint64) (gid uint32, game string) {
	var d string
	var id int64
	if globals.Postgres.QueryRow(`SELECT id, description FROM matchmaking.gatherings WHERE owner_pid = $1 AND registered AND description LIKE $2 ORDER BY id DESC LIMIT 1`,
		int64(pid), bridgeMarker+"%").Scan(&id, &d) != nil {
		return 0, ""
	}
	return uint32(id), strings.TrimPrefix(d, bridgeMarker)
}

// bridgedStationURL is the station a Wii U that joined a bridged gathering is given for its host: the bridge's PIA station (the same
// address as the world's), the host's own PID and the bridged RVCID. natm/natf 1 as the world's: a joiner treats an unknown type as unreachable.
func bridgedStationURL(hostPID uint64) (types.StationURL, bool) {
	host, port, ok := strings.Cut(os.Getenv("MINECRAFT_SERVICE_STATION"), ":")
	if !ok || host == "" || port == "" {
		return types.StationURL{}, false
	}
	return types.NewStationURL(types.String(fmt.Sprintf("prudp:/address=%s;port=%s;PID=%d;RVCID=%d;sid=15;stream=10;type=3;natm=1;natf=1;upnp=0;pmp=0;probeinit=0", host, port, hostPID, bridgedRVCID))), true
}

// joinEvent is one thing a Wii U did with a bridged gathering.
type joinEvent struct {
	Seq  int64             `json:"seq"`
	Time time.Time         `json:"time"`
	Game string            `json:"game"`
	Kind string            `json:"kind"` // "join" (JoinMatchmakeSession's permission check), "urls" (GetSessionURLs), "probe" (RequestProbeInitiationExt)
	PID  uint32            `json:"pid"`
	Data map[string]string `json:"data,omitempty"`
	Err  string            `json:"err,omitempty"`
}

var joinFeed struct {
	mu  sync.Mutex
	seq int64
	ev  []joinEvent
}

const joinFeedMax = 200

// recordJoin logs the event in full (ids and station data, no session keys) and keeps it for the directory.
func recordJoin(e joinEvent) {
	e.Time = time.Now().UTC()
	joinFeed.mu.Lock()
	joinFeed.seq++
	e.Seq = joinFeed.seq
	joinFeed.ev = append(joinFeed.ev, e)
	if over := len(joinFeed.ev) - joinFeedMax; over > 0 {
		joinFeed.ev = joinFeed.ev[over:]
	}
	joinFeed.mu.Unlock()
	globals.Logger.Infof("minecraft bridged join: game=%s kind=%s pid=%d data=%v err=%q", e.Game, e.Kind, e.PID, e.Data, e.Err)
}

func joinsAfter(after int64) []joinEvent {
	joinFeed.mu.Lock()
	defer joinFeed.mu.Unlock()
	out := []joinEvent{}
	for _, e := range joinFeed.ev {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out
}

func bridgedJoins(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	json.NewEncoder(w).Encode(joinsAfter(after))
}

// canJoinBridged wraps the title's join check: a join of a bridged gathering is recorded with its verdict.
func canJoinBridged(inner func(*commonglobals.MatchmakingManager, types.PID, matchmakingtypes.MatchmakeSession) *nex.Error) func(*commonglobals.MatchmakingManager, types.PID, matchmakingtypes.MatchmakeSession) *nex.Error {
	return func(m *commonglobals.MatchmakingManager, pid types.PID, s matchmakingtypes.MatchmakeSession) *nex.Error {
		nerr := inner(m, pid, s)
		if game := bridgedGame(uint32(s.ID)); game != "" {
			e := joinEvent{Game: game, Kind: "join", PID: uint32(pid), Data: map[string]string{"gathering": strconv.FormatUint(uint64(s.ID), 10), "host_pid": strconv.FormatUint(uint64(s.OwnerPID), 10)}}
			if nerr != nil {
				e.Err = nerr.Message
			}
			recordJoin(e)
		}
		return nerr
	}
}

// bridgedSessionURLs wraps GetSessionURLs on top of worldSessionURLs: a bridged gathering answers the bridge's station for its host.
func bridgedSessionURLs(p *matchmaking.Protocol) {
	get := p.GetSessionURLs
	if get == nil {
		return
	}
	p.GetSessionURLs = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
		game := ""
		if err == nil {
			game = bridgedGame(uint32(gid))
		}
		if game == "" {
			return get(err, packet, callID, gid)
		}
		conn := packet.Sender().(*nex.PRUDPConnection)
		var owner uint64
		globals.Postgres.QueryRow(`SELECT owner_pid FROM matchmaking.gatherings WHERE id = $1`, int64(gid)).Scan(&owner)
		station, ok := bridgedStationURL(owner)
		ev := joinEvent{Game: game, Kind: "urls", PID: uint32(conn.PID()), Data: map[string]string{"gathering": strconv.FormatUint(uint64(gid), 10)}}
		if !ok {
			ev.Err = "MINECRAFT_SERVICE_STATION is not set: no station to hand out"
			recordJoin(ev)
			return nil, nex.NewError(nex.ResultCodes.RendezVous.SessionVoid, "no bridge station")
		}
		ev.Data["station"] = station.URL()
		recordJoin(ev)
		endpoint := conn.Endpoint().(*nex.PRUDPEndPoint)
		urls := types.NewList[types.StationURL]()
		urls = append(urls, station)
		out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
		urls.WriteTo(out)
		res := nex.NewRMCSuccess(endpoint, out.Bytes())
		res.ProtocolID = matchmaking.ProtocolID
		res.MethodID = matchmaking.MethodGetSessionURLs
		res.CallID = callID
		return res, nil
	}
}

// recordBridgedProbe logs a joiner's RequestProbeInitiationExt that names a bridged host's station, with the joiner's own station URL
// (its address and ports, what the transport leg needs to reach it). It returns true when the request was for a bridged host.
func recordBridgedProbe(packet nex.PacketInterface, targets types.List[types.String], station types.String) bool {
	found := false
	for _, t := range targets {
		u := types.NewStationURL(t)
		if id, ok := u.RVConnectionID(); !ok || id != bridgedRVCID {
			continue
		}
		host, _ := u.PrincipalID()
		gid, game := bridgedByHost(uint64(host))
		pid := uint32(packet.Sender().(*nex.PRUDPConnection).PID())
		recordJoin(joinEvent{Game: game, Kind: "probe", PID: pid, Data: map[string]string{"gathering": strconv.FormatUint(uint64(gid), 10), "target": string(t), "station": string(station)}})
		found = true
	}
	return found
}

// bridgedFind wraps FindBySingleID, the call the game makes for a friend's presence (groupId = the gathering) before it lists the game: every
// call is logged (who asked, which gathering, whether it was answered), and for a bridged gathering it is also kept in the joins feed
// ("find"), so a console that never asks shows as a missing event, not as silence.
func bridgedFind(p *matchmaking.Protocol) {
	get := p.FindBySingleID
	if get == nil {
		return
	}
	p.FindBySingleID = func(err error, packet nex.PacketInterface, callID uint32, id types.UInt32) (*nex.RMCMessage, *nex.Error) {
		res, nerr := get(err, packet, callID, id)
		if err == nil {
			pid := uint32(packet.Sender().(*nex.PRUDPConnection).PID())
			game := bridgedGame(uint32(id))
			ev := joinEvent{Game: game, Kind: "find", PID: pid, Data: map[string]string{"gathering": strconv.FormatUint(uint64(id), 10)}}
			if nerr != nil {
				ev.Err = nerr.Message
			}
			if game != "" {
				recordJoin(ev)
			} else {
				globals.Logger.Infof("minecraft FindBySingleID: pid=%d gathering=%d answered=%t", pid, uint32(id), nerr == nil)
			}
		}
		return res, nerr
	}
}
