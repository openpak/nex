package lm3

// The game's own station/presence protocol, 0x79.
//
// Luigi's Mansion 3 does not do "join my friend" through the standard matchmake-extension
// methods. It runs a title-specific protocol on 0x79 that has no common implementation:
//
//	0x79.5   UpdateApplicationBuffer  publish-only
//	0x79.13  UpdateState              publish-only ("here is my state", a room the player opened)
//	0x79.14  InitializeStation        "I am connecting, who is here?" -> station/friend records
//	0x79.15  GetStationLocation       "where are my friends?"        -> station/friend records
//
// The two publish-only methods take a body and the working server for this title answers them
// empty. The two query methods answer with a list of station records — a friend's room ID, the
// station URL for the Pia mesh, a display name — assembled from what other players have
// published and from live connection/station state.
//
// This is a stub, and honestly so. It keeps the connection alive by acknowledging every method
// the game is known to call — an unacknowledged 0x79 call gets Core::NotImplemented, which is
// the one answer most likely to hard-stall the session — but it publishes nothing and reports
// no stations: the two query methods return an empty record list (count = 0). "No friends have
// a room open" is a correct answer for a solo host and a plausible one for a first live run; it
// is not the whole protocol.
//
// What is missing, and why it is deferred: the populated station/friend record layout was
// reverse-engineered from a retail capture and depends on enumerating live connections and
// their registered Pia station URLs. OpenPak needs its OWN capture to reproduce that layout in
// the clean room, and the shared host (github.com/openpak/nex) does not yet expose the
// connection/station enumeration it would need. Both are next-round work — see docs/design.md.

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-protocols-go/v2/globals"
	nexhost "github.com/openpak/nex"
)

// presenceProtocolID is the title-specific station/presence protocol.
const presenceProtocolID uint16 = 0x79

const (
	presenceMethodUpdateApplicationBuffer = 5
	presenceMethodUpdateState             = 13
	presenceMethodInitializeStation       = 14
	presenceMethodGetStationLocation      = 15
)

// presence is a minimal ServiceProtocol: it acknowledges the 0x79 methods the game calls and
// implements none of their content. Every registered protocol on an endpoint sees every data
// packet, so it ignores anything not addressed to 0x79.
type presence struct {
	endpoint nex.EndpointInterface
}

func registerPresence(host *nexhost.Host) {
	host.Secure.RegisterServiceProtocol(&presence{})
}

func (p *presence) Endpoint() nex.EndpointInterface     { return p.endpoint }
func (p *presence) SetEndpoint(e nex.EndpointInterface) { p.endpoint = e }

func (p *presence) HandlePacket(packet nex.PacketInterface) {
	message := packet.RMCMessage()
	if !message.IsRequest || message.ProtocolID != presenceProtocolID {
		return
	}

	body, handled := presenceResponseBody(message.MethodID)
	if !handled {
		globals.RespondError(packet, presenceProtocolID,
			nex.NewError(nex.ResultCodes.Core.NotImplemented, "LM3 presence (0x79) method not implemented"))
		return
	}
	p.respond(packet, message.MethodID, body)
}

// presenceResponseBody is the stub's whole decision: which 0x79 methods are acknowledged and
// with what body. Publish-only methods answer empty (nil); location queries answer an empty
// record list (a u32 count of zero, little-endian); anything else is not handled here.
func presenceResponseBody(methodID uint32) (body []byte, handled bool) {
	switch methodID {
	case presenceMethodUpdateApplicationBuffer, presenceMethodUpdateState:
		return nil, true
	case presenceMethodInitializeStation, presenceMethodGetStationLocation:
		return []byte{0, 0, 0, 0}, true
	default:
		return nil, false
	}
}

func (p *presence) respond(packet nex.PacketInterface, methodID uint32, body []byte) {
	endpoint := packet.Sender().Endpoint()
	response := nex.NewRMCSuccess(endpoint, body)
	response.ProtocolID = presenceProtocolID
	response.MethodID = methodID
	response.CallID = packet.RMCMessage().CallID
	globals.Respond(packet, response)
}
