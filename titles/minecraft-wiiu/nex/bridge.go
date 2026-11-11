package nex

// The bridge's Wii U-host leg (card openpak/prds#177): common/minecraft joins a world hosted on a real console on behalf of a
// player who is on another console. That player has a Wii U PID (every account has an identity on every network) but no NEX
// connection, so the joiner's NEX side is done here, server-side, as internal HTTP calls on the title's health listener
// (NEX_HTTP_ADDR, loopback on the box), bearer MINECRAFT_INTERNAL_KEY (the key nex already sends the service):
//
//	GET  /internal/minecraft/gatherings  registered console-hosted gatherings whose host is online: host PID and station URLs
//	POST /internal/minecraft/join        {gathering, pid, stations}: what a real JoinMatchmakeSession plus the joiner's
//	                                     RequestProbeInitiationExt cause, and the session key (hex; never logged)
//	POST /internal/minecraft/leave       {gathering, pid}: what a real EndParticipation causes
//
// The host console's PIA connect job only accepts a station whose PID NEX told it joined (notification 3001 subtype 1,
// common/minecraft docs/wiiu-join.md section 7), and it probes a joiner only when NEX relays the joiner's probe request
// (InitiateProbe with the joiner's station URL, section 3 step 4).

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	pb "github.com/PretendoNetwork/grpc/go/account/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	mmdb "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making/database"
	"github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making/tracking"
	mmconst "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/constants"
	notifconst "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
	notiftypes "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
	pqextended "github.com/PretendoNetwork/pq-extended"
	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles/minecraft-wiiu/globals"
	"google.golang.org/grpc/metadata"
)

// StartBridge mounts the routes on the health listener.
func StartBridge() {
	nexhost.HealthMux.HandleFunc("GET /internal/minecraft/gatherings", bridgeAuth(bridgeGatherings))
	nexhost.HealthMux.HandleFunc("POST /internal/minecraft/join", bridgeAuth(bridgeJoin))
	nexhost.HealthMux.HandleFunc("POST /internal/minecraft/leave", bridgeAuth(bridgeLeave))
	mountBridged()
}

func bridgeAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := os.Getenv("MINECRAFT_INTERNAL_KEY")
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if key == "" || !ok || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

type bridgeGathering struct {
	Gathering    uint32   `json:"gathering"`
	OwnerPID     uint64   `json:"owner_pid"`
	HostPID      uint64   `json:"host_pid"`
	Participants []uint64 `json:"participants"`
	Stations     []string `json:"stations"`
	// what the session directory needs to advertise the game elsewhere (prds#177): the game's own data and limits
	MaxParticipants int    `json:"max_participants"`
	Open            bool   `json:"open"`
	Buffer          string `json:"application_buffer"` // hex
}

func hostStations(pid uint64) []string {
	c := globals.SecureEndpoint.FindConnectionByPID(pid)
	if c == nil {
		return nil
	}
	var out []string
	for _, u := range c.StationURLs {
		out = append(out, u.URL())
	}
	return out
}

func bridgeGatherings(w http.ResponseWriter, r *http.Request) {
	// bridged gatherings (bridged.go) are other consoles' games listed here: never reported back as console-hosted
	rows, err := globals.Postgres.QueryContext(r.Context(), `SELECT g.id, g.owner_pid, g.host_pid, g.participants, g.max_participants, s.open_participation, s.application_buffer
		FROM matchmaking.gatherings g JOIN matchmaking.matchmake_sessions s ON s.id = g.id
		WHERE g.registered AND g.type = 'MatchmakeSession' AND g.owner_pid <> $1 AND g.description NOT LIKE $2 ORDER BY g.id DESC LIMIT 50`, int64(worldPID()), bridgeMarker+"%")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []bridgeGathering{}
	for rows.Next() {
		var g bridgeGathering
		var buf []byte
		if err := rows.Scan(&g.Gathering, &g.OwnerPID, &g.HostPID, pqextended.Array(&g.Participants), &g.MaxParticipants, &g.Open, &buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		g.Buffer = hex.EncodeToString(buf)
		if g.Stations = hostStations(g.HostPID); g.Stations != nil { // the host is online: its gathering is joinable
			out = append(out, g)
		}
	}
	json.NewEncoder(w).Encode(out)
}

type bridgeJoinReq struct {
	Gathering uint32   `json:"gathering"`
	PID       uint32   `json:"pid"`
	Stations  []string `json:"stations"` // the joiner's station URLs, as a console sends them in RequestProbeInitiationExt
}

// checkJoinerStations accepts only station URLs that carry an address, a port and the joiner's own PID.
func checkJoinerStations(pid uint32, stations []string) ([]types.StationURL, bool) {
	var out []types.StationURL
	for _, s := range stations {
		u := types.NewStationURL(types.String(s))
		_, okA := u.Address()
		_, okP := u.PortNumber()
		p, okID := u.PrincipalID()
		if !okA || !okP || !okID || uint64(p) != uint64(pid) {
			return nil, false
		}
		out = append(out, u)
	}
	return out, len(out) > 0
}

// participationEvent is the 3001 notification a real join or leave sends (nex-protocols-common JoinGathering and
// EndGatheringParticipation): source and Param2 the participant, Param1 the gathering, Param3 the participant count on join.
func participationEvent(sub notifconst.ParticipationEvents, gid uint32, pid uint32, count int) notiftypes.NotificationEvent {
	e := notiftypes.NewNotificationEvent()
	e.PIDSource = types.NewPID(uint64(pid))
	e.Type = notifconst.NotificationCategoryParticipationEvent.Build(sub)
	e.Param1 = types.UInt64(gid)
	e.Param2 = types.UInt64(pid)
	e.StrParam = types.NewString("")
	if sub == notifconst.ParticipationEventsParticipate {
		e.Param3 = types.UInt64(count)
	}
	return e
}

// notifyTargets mirrors the common rule: everyone in the gathering when a verbose-participants flag is set, else the owner;
// never the bridged player, who has no NEX connection.
func notifyTargets(flags mmconst.GatheringFlags, owner uint64, participants []uint64, pid uint32) []uint64 {
	t := []uint64{owner}
	if flags.HasFlag(mmconst.GatheringFlagNotifyParticipationEventsToAllParticipants) || flags.HasFlag(mmconst.GatheringFlagNotifyParticipationEventsToAllParticipantsReproducibly) {
		t = common_globals.RemoveDuplicates(participants)
	}
	return slices.DeleteFunc(t, func(p uint64) bool { return p == uint64(pid) })
}

func accountName(pid uint32) string {
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), common_globals.GRPCAccountCommonMetadata), 5*time.Second)
	defer cancel()
	r, err := common_globals.GRPCAccountClient.GetUserData(ctx, &pb.GetUserDataRequest{Pid: pid})
	if err != nil {
		return ""
	}
	return r.Username
}

func bridgeJoin(w http.ResponseWriter, r *http.Request) {
	var q bridgeJoinReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); err != nil || q.Gathering == 0 || q.PID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	joiner, ok := checkJoinerStations(q.PID, q.Stations)
	if !ok {
		http.Error(w, "stations: want station URLs with address, port and PID = pid", http.StatusBadRequest)
		return
	}
	m := globals.MatchmakingManager
	m.Mutex.Lock()
	g, _, participants, _, nerr := mmdb.FindGatheringByID(m, q.Gathering)
	if nerr != nil {
		m.Mutex.Unlock()
		http.Error(w, "no such gathering", http.StatusNotFound)
		return
	}
	host := globals.SecureEndpoint.FindConnectionByPID(uint64(g.HostPID))
	if host == nil || isWorld(q.Gathering) {
		m.Mutex.Unlock()
		http.Error(w, "the gathering's host is not online", http.StatusConflict)
		return
	}
	// a bridged player whose last session ended without a leave joins again
	others := slices.DeleteFunc(slices.Clone(participants), func(p uint64) bool { return p == uint64(q.PID) })
	if g.MaximumParticipants != 0 && len(others)+1 > int(g.MaximumParticipants) {
		m.Mutex.Unlock()
		http.Error(w, "the gathering is full", http.StatusConflict)
		return
	}
	total := append([]uint64{uint64(q.PID)}, others...)
	if _, err := m.Database.Exec(`UPDATE matchmaking.gatherings SET participants = $1 WHERE id = $2`, pqextended.Array(total), q.Gathering); err != nil {
		m.Mutex.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pid := types.NewPID(uint64(q.PID))
	mmdb.RecordJoinMessage(m, q.Gathering, pid, "")
	tracking.LogJoinGathering(m.Database, pid, q.Gathering, []uint64{uint64(q.PID)}, total)
	var key []byte
	err := m.Database.QueryRow(`SELECT session_key FROM matchmaking.matchmake_sessions WHERE id = $1`, q.Gathering).Scan(&key)
	m.Mutex.Unlock()
	if err != nil || len(key) < 16 {
		http.Error(w, "no session key", http.StatusInternalServerError)
		return
	}
	common_globals.SendNotificationEvent(globals.SecureEndpoint, participationEvent(notifconst.ParticipationEventsParticipate, q.Gathering, q.PID, len(total)),
		notifyTargets(g.Flags, uint64(g.OwnerPID), total, q.PID))
	// the joiner's RequestProbeInitiationExt(host URLs, own URL): one InitiateProbe per host URL (the common handler's relay)
	stations := hostStations(uint64(g.HostPID))
	for _, u := range joiner {
		for range stations {
			initiateProbe(host, u)
		}
	}
	globals.Logger.Infof("minecraft bridge: PID %d joined gathering %d (host %d, %d participants)", q.PID, q.Gathering, uint64(g.HostPID), len(total))
	json.NewEncoder(w).Encode(map[string]any{"gathering": q.Gathering, "host_pid": uint64(g.HostPID), "stations": stations,
		"participants": total, "name": accountName(q.PID), "session_key": hex.EncodeToString(key)})
}

func bridgeLeave(w http.ResponseWriter, r *http.Request) {
	var q bridgeJoinReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); err != nil || q.Gathering == 0 || q.PID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	m := globals.MatchmakingManager
	m.Mutex.Lock()
	g, _, participants, _, nerr := mmdb.FindGatheringByID(m, q.Gathering)
	if nerr != nil || !slices.Contains(participants, uint64(q.PID)) {
		m.Mutex.Unlock()
		http.Error(w, "not a participant", http.StatusNotFound)
		return
	}
	left, nerr := mmdb.RemoveParticipantFromGathering(m, q.Gathering, uint64(q.PID))
	if nerr == nil {
		tracking.LogLeaveGathering(m.Database, types.NewPID(uint64(q.PID)), q.Gathering, left)
	}
	m.Mutex.Unlock()
	if nerr != nil {
		http.Error(w, nerr.Error(), http.StatusInternalServerError)
		return
	}
	common_globals.SendNotificationEvent(globals.SecureEndpoint, participationEvent(notifconst.ParticipationEventsEndParticipation, q.Gathering, q.PID, 0),
		notifyTargets(g.Flags, uint64(g.OwnerPID), participants, q.PID))
	globals.Logger.Infof("minecraft bridge: PID %d left gathering %d", q.PID, q.Gathering)
	w.Write([]byte("ok\n"))
}
