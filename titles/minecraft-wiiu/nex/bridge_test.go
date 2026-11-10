package nex

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2"
	mmconst "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/constants"
	notifconst "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
)

// The 3001 participation event the bridge sends the host console for "PID 1358756791 joined gathering 5, 2 participants",
// serialized as this title's secure server does (NEX 3.10, structure headers on). Layout: structure header (version 0,
// u32 length 0x17), u32 source PID, u32 type 3001 (category 3 * 1000 + subtype 1), u32 gathering, u32 participant,
// String "" (u16 length 1 + NUL), u32 participant count. The console keys its participant list on Param2
// (ProcessNotificationEvent, rpx 0x035d661c; docs/wiiu-join.md section 7).
func TestParticipationEventBytes(t *testing.T) {
	out := nex.NewByteStreamOut(nex.NewLibraryVersions(), nex.NewByteStreamSettings())
	out.LibraryVersions.SetDefault(nex.NewLibraryVersion(3, 10, 0))
	out.Settings.UseStructureHeader = true
	participationEvent(notifconst.ParticipationEventsParticipate, 5, 1358756791, 2).WriteTo(out)
	got := hex.EncodeToString(out.Bytes())
	if want := "0017000000b7fbfc50b90b000005000000b7fbfc5001000002000000"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

// A real console joiner's station URL (nex log 2026-11-10 21:13:48, real Wii U as PID 1570020176) passes; a URL naming another
// PID, or none, is refused.
func TestCheckJoinerStations(t *testing.T) {
	real := "prudp:/address=37.228.205.125;port=5272;CID=1232220597;PID=1570020176;R=0;RVCID=2;Ra=0.0.0.0;Rp=0;Rsa=0.0.0.0;Rsp=0;natf=2;natm=1;probeinit=1;sid=15;stream=0;type=3"
	if _, ok := checkJoinerStations(1570020176, []string{real}); !ok {
		t.Fatal("real joiner URL refused")
	}
	for _, bad := range [][]string{{real}, nil, {"prudp:/port=1;PID=1358756791"}} {
		if _, ok := checkJoinerStations(1358756791, bad); ok {
			t.Fatalf("accepted %v", bad)
		}
	}
}

// Console-hosted worlds carry flags 2560 (0x800 reproducible participation events, 0x200 to all participants; gatherings 1-4
// on prod): the event goes to every participant but the bridged player. Without the flags, only the owner.
func TestNotifyTargets(t *testing.T) {
	got := notifyTargets(mmconst.GatheringFlags(2560), 1570020176, []uint64{1358756791, 1570020176, 1570020176}, 1358756791)
	if len(got) != 1 || got[0] != 1570020176 {
		t.Fatalf("flags 2560: %v", got)
	}
	if got := notifyTargets(0, 7, []uint64{1358756791, 9}, 1358756791); len(got) != 1 || got[0] != 7 {
		t.Fatalf("no flags: %v", got)
	}
}

func TestBridgeAuth(t *testing.T) {
	os.Setenv("MINECRAFT_INTERNAL_KEY", "k")
	defer os.Unsetenv("MINECRAFT_INTERNAL_KEY")
	h := bridgeAuth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	for auth, want := range map[string]int{"": 401, "Bearer x": 401, "k": 401, "Bearer k": 204} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/internal/minecraft/gatherings", nil)
		r.Header.Set("Authorization", auth)
		h(w, r)
		if w.Code != want {
			t.Fatalf("%q: %d want %d", auth, w.Code, want)
		}
	}
}
