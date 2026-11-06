package nex

import (
	"testing"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/plogger-go"
	"github.com/openpak/nex/titles/terraria/globals"
)

func TestCrossJoinRefusedAndCounted(t *testing.T) {
	globals.Logger = plogger.NewLogger()
	globals.IsCrossRoom = func(gid uint32) bool { return gid == 99 }
	defer func() { globals.IsCrossRoom = nil }()
	if e := crossJoinError(types.UInt32(7)); e != nil {
		t.Fatal("a native gathering must not be refused")
	}
	before := CrossJoinsRefused.Load()
	e := crossJoinError(types.UInt32(99))
	if e == nil || e.ResultCode&^0x80000000 != nex.ResultCodes.RendezVous.SessionClosed&^0x80000000 || CrossJoinsRefused.Load() != before+1 {
		t.Fatalf("cross join: %v", e)
	}
}

func TestCrossJoinAllowedWithRelay(t *testing.T) {
	globals.Logger = plogger.NewLogger()
	globals.IsCrossRoom = func(gid uint32) bool { return gid == 99 }
	globals.RelayStation = "192.0.2.10:49200"
	defer func() { globals.IsCrossRoom, globals.RelayStation = nil, "" }()
	if e := crossJoinError(types.UInt32(99)); e != nil {
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
