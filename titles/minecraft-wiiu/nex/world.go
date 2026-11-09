package nex

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nattraversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	"github.com/openpak/nex/titles/minecraft-wiiu/globals"

	"github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
)

// The service world (card openpak/prds#162): one persistent gathering that the common/minecraft service hosts. Its owner is a
// PID that is no NEX connection; the console is pointed at the service's PIA station for it and the service answers as the host
// (Java Edition server behind it). Enabled by MINECRAFT_SERVICE_URL; without it nothing here is active.

// defaultWorldPID is the PID the service stands in for (no account has it: accounts are below 2^31 - 1 000 000 000).
const defaultWorldPID = 2000000001

// worldRVCID is the service host's RendezVous connection id (common/minecraft pia5.HostRVCID).
const worldRVCID = 0xFFF0

var worldGID atomic.Uint32

func worldPID() uint32 {
	if n, err := strconv.ParseUint(os.Getenv("MINECRAFT_WORLD_PID"), 10, 32); err == nil && n != 0 {
		return uint32(n)
	}
	return defaultWorldPID
}

func worldEnabled() bool { return os.Getenv("MINECRAFT_SERVICE_URL") != "" }

func isWorld(gid uint32) bool { return worldEnabled() && gid != 0 && gid == worldGID.Load() }

// StartWorld creates the world gathering if it is missing and keeps registering its PIA leg with the service: the service holds
// legs in memory, so a restart is healed within a minute.
func StartWorld() {
	if !worldEnabled() {
		return
	}
	go func() {
		for {
			if err := syncWorld(); err != nil {
				globals.Logger.Errorf("minecraft service world: %v", err)
			}
			time.Sleep(time.Minute)
		}
	}()
}

func syncWorld() error {
	gid, key, err := ensureWorld()
	if err != nil {
		return err
	}
	worldGID.Store(gid)
	// ponytail: nothing tells NEX when a console leaves the PIA world, so its participant list only grows; the service tracks the
	// players, so the list is cleared every minute (max 8 would otherwise close the world after eight joins)
	globals.Postgres.Exec(`UPDATE matchmaking.gatherings SET participants = '{}' WHERE id = $1`, int64(gid))
	body, _ := json.Marshal(map[string]any{"gathering": gid, "host_pid": worldPID(), "session_key": hex.EncodeToString(key)})
	req, _ := http.NewRequest("POST", strings.TrimRight(os.Getenv("MINECRAFT_SERVICE_URL"), "/")+"/internal/legs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+os.Getenv("MINECRAFT_INTERNAL_KEY"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("service answered %d", resp.StatusCode)
	}
	return nil
}

// ensureWorld returns the world's gathering id and PIA session key, creating the rows on first use. The rows copy the shape of
// the real host's gathering in the 2026-11-07 capture: game mode 15, up to 8 participants, participation policy 95.
func ensureWorld() (uint32, []byte, error) {
	var gid int64
	var key []byte
	err := globals.Postgres.QueryRow(`SELECT g.id, m.session_key FROM matchmaking.gatherings g JOIN matchmaking.matchmake_sessions m ON m.id = g.id
		WHERE g.owner_pid = $1 ORDER BY g.id DESC LIMIT 1`, int64(worldPID())).Scan(&gid, &key)
	if err == nil {
		globals.Postgres.Exec(`UPDATE matchmaking.gatherings SET registered = true, state = 0 WHERE id = $1`, gid)
		return uint32(gid), key, nil
	}
	key = make([]byte, 32)
	rand.Read(key)
	tx, err := globals.Postgres.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	if err := tx.QueryRow(`INSERT INTO matchmaking.gatherings (owner_pid, host_pid, min_participants, max_participants, participation_policy, policy_argument,
		flags, state, description, registered, type, started_time, participants) VALUES ($1,$1,0,8,95,0,2560,0,'',true,'MatchmakeSession',now(),'{}') RETURNING id`,
		int64(worldPID())).Scan(&gid); err != nil {
		return 0, nil, err
	}
	// attribute 0 carries the public-game marker (low 24 bits 0x30881) so the game lists it as an open game
	if _, err := tx.Exec(`INSERT INTO matchmaking.matchmake_sessions (id, game_mode, attribs, open_participation, matchmake_system_type, application_buffer,
		progress_score, session_key, option_zero, matchmake_param, refer_gid, user_password_enabled, system_password_enabled, codeword)
		VALUES ($1,15,'{198785,0,0,0,0,0}',true,1,$2,100,$3,0,$4,0,false,false,'')`, gid, worldApplicationBuffer, key, worldMatchmakeParam); err != nil {
		return 0, nil, err
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	globals.Logger.Infof("minecraft service world created: gathering %d, owner PID %d", gid, worldPID())
	return uint32(gid), key, nil
}

// worldStationURL is the station URL a console gets for the world's host: the service's PIA address, the owner PID as the PIA
// constant id (the same shape the Terraria relay uses).
func worldStationURL() types.StationURL {
	host, port, _ := strings.Cut(os.Getenv("MINECRAFT_SERVICE_STATION"), ":")
	// natm=1;natf=1: endpoint independent mapping and filtering (the service is an open public host); a real host publishes its
	// measured NAT types, and unknown (0) makes a joiner treat the host as unreachable.
	// RVCID is the connection id a real host's registered URL carries (nex-protocols-common sets PID and RVCID on every station);
	// the PIA host answers as RVCID 1 (common/minecraft internal/pia5 HostRVCID); a value no live NEX connection has, because RequestProbeInitiationExt is relayed
	// to the connection with that id.
	return types.NewStationURL(types.String(fmt.Sprintf("prudp:/address=%s;port=%s;PID=%d;RVCID=%d;sid=15;stream=10;type=3;natm=1;natf=1;upnp=0;pmp=0;probeinit=0", host, port, worldPID(), worldRVCID)))
}

// worldSessionURLs wraps GetSessionURLs: the world answers the service's station, everything else the common handler.
func worldSessionURLs(p *matchmaking.Protocol) {
	get := p.GetSessionURLs
	if get == nil {
		return
	}
	p.GetSessionURLs = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
		if err != nil || !isWorld(uint32(gid)) {
			return get(err, packet, callID, gid)
		}
		endpoint := packet.Sender().(*nex.PRUDPConnection).Endpoint().(*nex.PRUDPEndPoint)
		urls := types.NewList[types.StationURL]()
		urls = append(urls, worldStationURL())
		out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
		urls.WriteTo(out)
		res := nex.NewRMCSuccess(endpoint, out.Bytes())
		res.ProtocolID = matchmaking.ProtocolID
		res.MethodID = matchmaking.MethodGetSessionURLs
		res.CallID = callID
		globals.Logger.Infof("minecraft: gathering %d host URL -> service %s", uint32(gid), os.Getenv("MINECRAFT_SERVICE_STATION"))
		return res, nil
	}
}

// worldBrowse answers BrowseMatchmakeSession with the world and nothing else: public matchmaking between players stays off
// (see the stub's reason in register_common_secure_server_protocols.go), the world is the one open game.
func worldBrowse(err error, packet nex.PacketInterface, callID uint32, _ matchmakingtypes.MatchmakeSessionSearchCriteria, _ types.ResultRange) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}
	endpoint := packet.Sender().(*nex.PRUDPConnection).Endpoint().(*nex.PRUDPEndPoint)
	list := types.NewList[matchmakingtypes.GatheringHolder]()
	if gid := worldGID.Load(); gid != 0 && worldEnabled() {
		globals.MatchmakingManager.Mutex.RLock()
		session, _, nerr := database.GetMatchmakeSessionByID(globals.MatchmakingManager, endpoint, gid)
		globals.MatchmakingManager.Mutex.RUnlock()
		if nerr == nil {
			session.SessionKey = make([]byte, 0)
			session.UserPassword = ""
			h := matchmakingtypes.NewGatheringHolder()
			h.Object = session.Copy().(matchmakingtypes.GatheringInterface)
			list = append(list, h)
		}
	}
	out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	list.WriteTo(out)
	res := nex.NewRMCSuccess(endpoint, out.Bytes())
	res.ProtocolID = matchmakeextension.ProtocolID
	res.MethodID = matchmakeextension.MethodBrowseMatchmakeSession
	res.CallID = callID
	return res, nil
}

// worldProbe answers a joiner's RequestProbeInitiationExt for the world: a console host probes the joiner's public address when it
// is told to, and the Wii U game waits for that (it gives up after about 15 s). The host here is the service, so it is asked over
// HTTP. Targets that are not the world host are left to the common handler (which relays to a live connection).
func worldProbe(packet nex.PacketInterface, targets types.List[types.String], stationToProbe types.String) {
	if !worldEnabled() {
		return
	}
	for _, t := range targets {
		globals.Logger.Infof("minecraft: RequestProbeInitiationExt target=%s station=%s", string(t), string(stationToProbe))
		if id, ok := types.NewStationURL(t).RVConnectionID(); !ok || id != worldRVCID {
			continue
		}
		to := types.NewStationURL(stationToProbe)
		addr, ok1 := to.Address()
		port, ok2 := to.PortNumber()
		if !ok1 || !ok2 {
			globals.Logger.Warningf("minecraft: probe request without an address: %s", string(stationToProbe))
			return
		}
		body, _ := json.Marshal(map[string]any{"gathering": worldGID.Load(), "address": addr, "port": port})
		req, _ := http.NewRequest("POST", strings.TrimRight(os.Getenv("MINECRAFT_SERVICE_URL"), "/")+"/internal/probe", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+os.Getenv("MINECRAFT_INTERNAL_KEY"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			globals.Logger.Errorf("minecraft: probe request: %v", err)
			return
		}
		resp.Body.Close()
		globals.Logger.Infof("minecraft: the joiner at %s:%d asked for probes: service answered %d", addr, port, resp.StatusCode)
		worldInitiateProbe(packet.Sender().(*nex.PRUDPConnection))
		return
	}
}

// worldInitiateProbe tells the joiner to probe the world's host, which a console host does by calling RequestProbeInitiationExt back
// as soon as it is told to probe a joiner (2026-11-07 capture: the joiner's two requests at 12:39:51.96, the host's two at 52.02, the
// joiner probes the host at 52.27). A joiner behind a port-restricted NAT drops the host's probes until it has sent to the host
// itself, so without this call the join times out (106-0334).
func worldInitiateProbe(joiner *nex.PRUDPConnection) {
	endpoint := joiner.Endpoint().(*nex.PRUDPEndPoint)
	server := endpoint.Server
	out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	worldStationURL().WriteTo(out)
	req := nex.NewRMCRequest(endpoint)
	req.ProtocolID = nattraversal.ProtocolID
	req.MethodID = nattraversal.MethodInitiateProbe
	req.CallID = 0xFFFF0000 + uint32(initiateProbeCalls.Add(1)&0xFFFF)
	req.Parameters = out.Bytes()

	var p nex.PRUDPPacketInterface
	switch joiner.DefaultPRUDPVersion {
	case 0:
		p, _ = nex.NewPRUDPPacketV0(server, joiner, nil)
	case 1:
		p, _ = nex.NewPRUDPPacketV1(server, joiner, nil)
	default:
		p, _ = nex.NewPRUDPPacketLite(server, joiner, nil)
	}
	p.SetType(constants.DataPacket)
	p.AddFlag(constants.PacketFlagNeedsAck)
	p.AddFlag(constants.PacketFlagReliable)
	p.SetSourceVirtualPortStreamType(joiner.StreamType)
	p.SetSourceVirtualPortStreamID(endpoint.StreamID)
	p.SetDestinationVirtualPortStreamType(joiner.StreamType)
	p.SetDestinationVirtualPortStreamID(joiner.StreamID)
	p.SetPayload(req.Bytes())
	server.Send(p)
	globals.Logger.Infof("minecraft: InitiateProbe(world host) -> joiner PID %d", joiner.PID())
}

var initiateProbeCalls atomic.Uint32
