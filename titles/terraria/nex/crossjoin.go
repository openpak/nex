package nex

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	"github.com/openpak/nex/titles/terraria/globals"
)

// CrossJoinsRefused counts Wii U joins into another console's room since start (prds#145): cross-play listing is on, the relay
// that would carry the game is not built yet, so each attempt is answered and counted to keep the gap visible.
var CrossJoinsRefused atomic.Int64

// crossJoinError answers a join into a shadow gathering with RendezVous SessionClosed (participation closed), the closest
// result the game already shows as "could not join"; nil for every other gathering.
func crossJoinError(gid types.UInt32, joinerPID uint32) *nex.Error {
	if globals.IsCrossRoom == nil || !globals.IsCrossRoom(uint32(gid)) {
		return nil
	}
	if globals.RelayStation != "" {
		if err := registerLeg(uint32(gid), joinerPID); err != nil {
			n := CrossJoinsRefused.Add(1)
			globals.Logger.Warningf("cross-play join: relay leg not registered (%v): gathering %d, refused %d since start", err, uint32(gid), n)
			return nex.NewError(nex.ResultCodes.RendezVous.SessionClosed, "cross-play relay unavailable")
		}
		globals.Logger.Infof("cross-play join through the relay %s: gathering %d", globals.RelayStation, uint32(gid))
		return nil
	}
	n := CrossJoinsRefused.Add(1)
	globals.Logger.Warningf("cross-play join not yet supported (relay pending): gathering %d, refused %d since start", uint32(gid), n)
	return nex.NewError(nex.ResultCodes.RendezVous.SessionClosed, "cross-play join not yet supported (relay pending)")
}

// relayStationURL is the station URL a Wii U gets for a cross room's host: the relay's address with the room owner's PID as the
// PIA constant id. Fields follow a NEX public station URL (type 3 = public, stream 10 / sid 15 as the game's own station).
func relayStationURL(ownerPID uint32) types.StationURL {
	host, port, _ := strings.Cut(globals.RelayStation, ":")
	return types.NewStationURL(types.String(fmt.Sprintf("prudp:/address=%s;port=%s;PID=%d;sid=15;stream=10;type=3;natm=0;natf=0;upnp=0;pmp=0;probeinit=0", host, port, ownerPID)))
}

// relaySessionURLs wraps GetSessionURLs: a cross room answers the relay's station URL (relay configured), everything else the
// common handler.
func relaySessionURLs(p *matchmaking.Protocol) {
	get := p.GetSessionURLs
	if get == nil {
		return
	}
	p.GetSessionURLs = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32) (*nex.RMCMessage, *nex.Error) {
		if err != nil || globals.RelayStation == "" || globals.IsCrossRoom == nil || !globals.IsCrossRoom(uint32(gid)) {
			return get(err, packet, callID, gid)
		}
		var owner int64
		if e := globals.Postgres.QueryRow(`SELECT owner_pid FROM matchmaking.gatherings WHERE id=$1`, int64(gid)).Scan(&owner); e != nil {
			return nil, nex.NewError(nex.ResultCodes.RendezVous.SessionVoid, "cross room gone")
		}
		connection := packet.Sender().(*nex.PRUDPConnection)
		endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)
		urls := types.NewList[types.StationURL]()
		urls = append(urls, relayStationURL(uint32(owner)))
		out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
		urls.WriteTo(out)
		res := nex.NewRMCSuccess(endpoint, out.Bytes())
		res.ProtocolID = matchmaking.ProtocolID
		res.MethodID = matchmaking.MethodGetSessionURLs
		res.CallID = callID
		globals.Logger.Infof("cross-play: gathering %d host URL -> relay %s", uint32(gid), globals.RelayStation)
		return res, nil
	}
}

// refuseCrossJoins wraps the join handlers the common protocol installed.
func refuseCrossJoins(p *matchmakeextension.Protocol) {
	if join := p.JoinMatchmakeSession; join != nil {
		p.JoinMatchmakeSession = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32, msg types.String) (*nex.RMCMessage, *nex.Error) {
			if e := crossJoinError(gid, pidOf(packet)); err == nil && e != nil {
				return nil, e
			}
			return join(err, packet, callID, gid, msg)
		}
	}
	if join := p.JoinMatchmakeSessionEx; join != nil {
		p.JoinMatchmakeSessionEx = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32, msg types.String, dontCare types.Bool, count types.UInt16) (*nex.RMCMessage, *nex.Error) {
			if e := crossJoinError(gid, pidOf(packet)); err == nil && e != nil {
				return nil, e
			}
			return join(err, packet, callID, gid, msg, dontCare, count)
		}
	}
}

func pidOf(packet nex.PacketInterface) uint32 {
	if c, ok := packet.Sender().(*nex.PRUDPConnection); ok {
		return uint32(c.PID())
	}
	return 0
}

// relayHTTP is the client for the Terraria service's internal API.
var relayHTTP = &http.Client{Timeout: 3 * time.Second}

// registerLeg tells the Terraria service (TERRARIA_RELAY_URL, bearer TERRARIA_INTERNAL_KEY) that a Wii U is about to join a cross
// room through it: the shadow's session key (PIA keys come from it), the host PID the relay stands in for, the core session.
// Without a service URL (a test probe set only TERRARIA_RELAY_STATION) nothing is registered.
func registerLeg(gid, joinerPID uint32) error {
	url := os.Getenv("TERRARIA_RELAY_URL")
	if url == "" {
		return nil
	}
	var key []byte
	var owner int64
	var core string
	err := globals.Postgres.QueryRow(`SELECT ms.session_key, g.owner_pid, s.core_session FROM matchmaking.gatherings g
		JOIN matchmaking.matchmake_sessions ms ON ms.id = g.id JOIN openpak_crossplay.shadow_gatherings s ON s.gathering_id = g.id
		WHERE g.id = $1`, int64(gid)).Scan(&key, &owner, &core)
	if err != nil {
		return fmt.Errorf("shadow %d: %w", gid, err)
	}
	body, _ := json.Marshal(map[string]any{"core_session": core, "gathering": gid, "host_pid": owner, "joiner_pid": joinerPID,
		"session_key": hex.EncodeToString(key)})
	req, _ := http.NewRequest("POST", strings.TrimRight(url, "/")+"/internal/legs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+os.Getenv("TERRARIA_INTERNAL_KEY"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := relayHTTP.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("relay answered %d", resp.StatusCode)
	}
	return nil
}
