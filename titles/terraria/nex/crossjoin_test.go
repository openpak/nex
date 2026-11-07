package nex

import (
	"testing"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	matchmakingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	"github.com/PretendoNetwork/plogger-go"
	"github.com/openpak/nex/titles/terraria/globals"
)

type fakePacket struct{}

func (fakePacket) Sender() nex.ConnectionInterface { return nil }
func (fakePacket) Payload() []byte                 { return nil }
func (fakePacket) SetPayload([]byte)               {}
func (fakePacket) RMCMessage() *nex.RMCMessage     { return nil }
func (fakePacket) SetRMCMessage(*nex.RMCMessage)   {}

func TestCrossJoinRefusedAndCounted(t *testing.T) {
	globals.Logger = plogger.NewLogger()
	globals.IsCrossRoom = func(gid uint32) bool { return gid == 99 }
	defer func() { globals.IsCrossRoom = nil }()
	if e := crossJoinError(types.UInt32(7), 1); e != nil {
		t.Fatal("a native gathering must not be refused")
	}
	before := CrossJoinsRefused.Load()
	e := crossJoinError(types.UInt32(99), 1)
	if e == nil || e.ResultCode&^0x80000000 != nex.ResultCodes.RendezVous.SessionClosed&^0x80000000 || CrossJoinsRefused.Load() != before+1 {
		t.Fatalf("cross join: %v", e)
	}
}

// Terraria joins with JoinMatchmakeSessionWithParam (0x27), not the two handlers the guard first
// wrapped (real Wii U, 2026-11-07: the join fell through to the stock handler, GetSessionURLs failed
// with "Host client not found" and the console showed 118-0502).
func TestRefuseCrossJoinsWithParam(t *testing.T) {
	globals.Logger = plogger.NewLogger()
	globals.IsCrossRoom = func(gid uint32) bool { return gid == 99 }
	defer func() { globals.IsCrossRoom = nil }()
	p := matchmakeextension.NewProtocol()
	called := false
	p.JoinMatchmakeSessionWithParam = func(err error, packet nex.PacketInterface, callID uint32, param matchmakingtypes.JoinMatchmakeSessionParam) (*nex.RMCMessage, *nex.Error) {
		called = true
		return nil, nil
	}
	refuseCrossJoins(p)
	if _, e := p.JoinMatchmakeSessionWithParam(nil, fakePacket{}, 1, matchmakingtypes.JoinMatchmakeSessionParam{GID: types.UInt32(99)}); e == nil || called {
		t.Fatalf("cross join with param must be refused before the handler: %v called=%v", e, called)
	}
	if _, e := p.JoinMatchmakeSessionWithParam(nil, fakePacket{}, 2, matchmakingtypes.JoinMatchmakeSessionParam{GID: types.UInt32(7)}); e != nil || !called {
		t.Fatalf("native join with param must reach the handler: %v called=%v", e, called)
	}
	autoCalled := false
	p.AutoMatchmakeWithGatheringIDPostpone = func(err error, packet nex.PacketInterface, callID uint32, gids types.List[types.UInt32], gathering matchmakingtypes.GatheringHolder, msg types.String) (*nex.RMCMessage, *nex.Error) {
		autoCalled = true
		return nil, nil
	}
	refuseCrossJoins(p)
	if _, e := p.AutoMatchmakeWithGatheringIDPostpone(nil, fakePacket{}, 3, types.List[types.UInt32]{types.UInt32(99)}, matchmakingtypes.GatheringHolder{}, types.String("")); e == nil || autoCalled {
		t.Fatalf("auto matchmake into a cross room must be refused: %v called=%v", e, autoCalled)
	}
	if _, e := p.AutoMatchmakeWithGatheringIDPostpone(nil, fakePacket{}, 4, types.List[types.UInt32]{types.UInt32(7)}, matchmakingtypes.GatheringHolder{}, types.String("")); e != nil || !autoCalled {
		t.Fatalf("auto matchmake into a native room must reach the handler: %v called=%v", e, autoCalled)
	}
}

func TestCrossJoinAllowedWithRelay(t *testing.T) {
	globals.Logger = plogger.NewLogger()
	globals.IsCrossRoom = func(gid uint32) bool { return gid == 99 }
	globals.RelayStation = "192.0.2.10:49200"
	defer func() { globals.IsCrossRoom, globals.RelayStation = nil, "" }()
	if e := crossJoinError(types.UInt32(99), 1); e != nil {
		t.Fatal("a cross join must go through when the relay is configured")
	}
	u := relayStationURL(4242)
	if a, _ := u.Address(); a != "192.0.2.10" {
		t.Fatalf("address %q in %s", a, u.String())
	}
	if p, _ := u.PortNumber(); p != 49200 {
		t.Fatalf("port %d", p)
	}
	if v, ok := u.ParamValue("PID"); !ok || v != "4242" {
		t.Fatalf("PID %q", v)
	}
}
