package splatoon2

// The Splatoon 2 methods that sit above the standard NEX protocols.
//
// nex-protocols-go implements each protocol's documented method set and answers
// Core::NotImplemented to anything past it. Splatoon 2 calls a few method IDs that
// are past it -- the splatfest queries, a tag-id request, and a per-player
// UpdateCurrentUser that gates the title's move to network-ready. Left unanswered
// each is a comm error (2306-0103) or a failed bring-up, so each is answered here.
//
// nex-go routes exactly these method IDs to us through the protocol's own Patches
// hook (Protocol.Patches + Protocol.PatchedMethods): the standard switch never sees
// them, so there is no double response and every other method is still the common
// implementation, untouched.
//
// The answers are deliberately minimal. Splatoon 2's official servers are gone, so a
// live splatfest can never be observed to reproduce; an empty list is the
// best-grounded reply for a fest query, and UpdateCurrentUser is a measured empty
// success (see docs/provenance.md). None of this is storage -- it is the shape the
// title needs to keep going.

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	proto_globals "github.com/PretendoNetwork/nex-protocols-go/v2/globals"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
)

// Method IDs Splatoon 2 uses that are above each protocol's standard set.
const (
	methodUtilityAcquireTagID      uint32 = 9  // Utility 0x6E.9  AcquireTagId(list<u64>) -> u64
	methodUtilityUpdateCurrentUser uint32 = 10 // Utility 0x6E.10 UpdateCurrentUser(param) -> void
	methodRankingGetFestivalResult uint32 = 18 // Ranking 0x70.18 -> list
	methodRankingGetFestivalRank   uint32 = 19 // Ranking 0x70.19 -> list
	methodRankingGetEventMatch     uint32 = 25 // Ranking 0x70.25 GetEventMatchResult -> list
	methodMatchmakeExtSimplePlay   uint32 = 60 // MatchmakeExtension 0x6D.60 -> list
)

// methodPatch is a nex.ServiceProtocol that answers a fixed set of method IDs and
// nothing else. It is assigned to a standard protocol's Patches field, so nex-go
// only ever hands it the methods named in PatchedMethods.
type methodPatch struct {
	endpoint   nex.EndpointInterface
	protocolID uint16
	handlers   map[uint32]func(packet nex.PacketInterface) ([]byte, *nex.Error)
}

func (p *methodPatch) Endpoint() nex.EndpointInterface            { return p.endpoint }
func (p *methodPatch) SetEndpoint(endpoint nex.EndpointInterface) { p.endpoint = endpoint }

func (p *methodPatch) HandlePacket(packet nex.PacketInterface) {
	message := packet.RMCMessage()
	handler, ok := p.handlers[message.MethodID]
	if !ok {
		// nex-go only routes PatchedMethods here, so this is unreachable; answer
		// rather than drop the packet if the two lists ever drift apart.
		proto_globals.RespondError(packet, p.protocolID, nex.NewError(nex.ResultCodes.Core.NotImplemented, "unpatched method"))
		return
	}
	body, nexError := handler(packet)
	if nexError != nil {
		proto_globals.RespondError(packet, p.protocolID, nexError)
		return
	}
	response := nex.NewRMCSuccess(packet.Sender().Endpoint(), body)
	response.ProtocolID = p.protocolID
	response.MethodID = message.MethodID
	response.CallID = message.CallID
	proto_globals.Respond(packet, response)
}

// emptyList is the four-byte body of a NEX list with zero elements. Every fest query
// and the custom playing-session query answer with one.
func emptyList(packet nex.PacketInterface) ([]byte, *nex.Error) {
	endpoint := packet.Sender().Endpoint()
	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	types.NewUInt32(0).WriteTo(stream)
	return stream.Bytes(), nil
}

func patchUtility(protocol *utility.Protocol) {
	patch := &methodPatch{
		endpoint:   protocol.Endpoint(),
		protocolID: utility.ProtocolID,
		handlers: map[uint32]func(nex.PacketInterface) ([]byte, *nex.Error){
			// UpdateCurrentUser: a measured empty success. This success is what lets the
			// title cross from cAcquireNexUniqueID to cNetworkReadySuccess; an error here
			// drops it to cNetworkReadyFailed. See docs/provenance.md.
			methodUtilityUpdateCurrentUser: func(packet nex.PacketInterface) ([]byte, *nex.Error) {
				return nil, nil
			},
			// AcquireTagId: a benign non-zero id. The title shares it in-session; it is
			// never a bring-up gate. Derived from the caller PID so it is stable per player.
			methodUtilityAcquireTagID: func(packet nex.PacketInterface) ([]byte, *nex.Error) {
				tagID := uint64(packet.Sender().PID())
				if tagID == 0 {
					tagID = 1
				}
				endpoint := packet.Sender().Endpoint()
				stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
				types.NewUInt64(tagID).WriteTo(stream)
				return stream.Bytes(), nil
			},
		},
	}
	protocol.Patches = patch
	protocol.PatchedMethods = []uint32{methodUtilityUpdateCurrentUser, methodUtilityAcquireTagID}
}

func patchRanking(protocol *ranking.Protocol) {
	protocol.Patches = &methodPatch{
		endpoint:   protocol.Endpoint(),
		protocolID: ranking.ProtocolID,
		handlers: map[uint32]func(nex.PacketInterface) ([]byte, *nex.Error){
			methodRankingGetFestivalResult: emptyList,
			methodRankingGetFestivalRank:   emptyList,
			methodRankingGetEventMatch:     emptyList,
		},
	}
	protocol.PatchedMethods = []uint32{methodRankingGetFestivalResult, methodRankingGetFestivalRank, methodRankingGetEventMatch}
}

func patchMatchmakeExtension(protocol *matchmake_extension.Protocol) {
	protocol.Patches = &methodPatch{
		endpoint:   protocol.Endpoint(),
		protocolID: matchmake_extension.ProtocolID,
		handlers: map[uint32]func(nex.PacketInterface) ([]byte, *nex.Error){
			methodMatchmakeExtSimplePlay: emptyList,
		},
	}
	protocol.PatchedMethods = []uint32{methodMatchmakeExtSimplePlay}
}
