package nex

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PretendoNetwork/plogger-go"
	"github.com/openpak/nex/titles/minecraft-wiiu/globals"
)

func quietLogger() { globals.Logger = plogger.NewLogger() }

// A bridged host's station is the bridge's address with the host's own PID and the bridged RVCID, never the world's: a probe request for it
// must be told apart from the world's, and the joiner needs the host's PID to match the gathering's owner.
func TestBridgedStationURL(t *testing.T) {
	t.Setenv("MINECRAFT_SERVICE_STATION", "145.241.206.67:22960")
	u, ok := bridgedStationURL(1358756791)
	if !ok {
		t.Fatal("no station")
	}
	// the serializer orders parameters as it likes: compare the fields
	for _, f := range []string{"address=145.241.206.67", "port=22960", "PID=1358756791", "RVCID=65522", "natm=1", "natf=1", "type=3", "sid=15"} {
		if !strings.Contains(u.URL(), f) {
			t.Fatalf("%s missing from %s", f, u.URL())
		}
	}
	if id, _ := u.RVConnectionID(); id == worldRVCID {
		t.Fatal("bridged RVCID equals the world's")
	}
	t.Setenv("MINECRAFT_SERVICE_STATION", "")
	if _, ok := bridgedStationURL(1); ok {
		t.Fatal("a station without an address was handed out")
	}
}

// The joins feed numbers events in order, hands out only what is newer than the cursor, and keeps the last 200.
func TestJoinFeed(t *testing.T) {
	quietLogger()
	joinFeed.ev, joinFeed.seq = nil, 0
	recordJoin(joinEvent{Game: "ps3:r1", Kind: "join", PID: 7})
	recordJoin(joinEvent{Game: "ps3:r1", Kind: "probe", PID: 7, Data: map[string]string{"station": "prudp:/address=1.2.3.4;port=5"}})
	if got := joinsAfter(0); len(got) != 2 || got[0].Seq != 1 || got[1].Kind != "probe" || got[1].Data["station"] == "" {
		t.Fatalf("feed: %+v", got)
	}
	if got := joinsAfter(1); len(got) != 1 || got[0].Seq != 2 {
		t.Fatalf("after 1: %+v", got)
	}
	for i := 0; i < 250; i++ {
		recordJoin(joinEvent{Game: "g", Kind: "join"})
	}
	if got := joinsAfter(0); len(got) != joinFeedMax {
		t.Fatalf("kept %d", len(got))
	}
}

// The sink route refuses what is not a Wii U v688 game before it touches the database.
func TestBridgedPutRefusesBadInput(t *testing.T) {
	good := strings.Repeat("00", 210)
	for name, body := range map[string]string{
		"net version": `{"game":"ps3:r1","host_pid":5,"application_buffer":"0884` + good + `","max_participants":8}`,
		"short":       `{"game":"ps3:r1","host_pid":5,"application_buffer":"0881","max_participants":8}`,
		"other game":  `{"game":"ps3:r2","host_pid":5,"application_buffer":"0881` + good + `","max_participants":8}`,
		"no host":     `{"game":"ps3:r1","host_pid":0,"application_buffer":"0881` + good + `","max_participants":8}`,
		"max":         `{"game":"ps3:r1","host_pid":5,"application_buffer":"0881` + good + `","max_participants":0}`,
	} {
		mux := http.NewServeMux()
		mux.HandleFunc("PUT /internal/minecraft/bridged/{game}", bridgedPut)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("PUT", "/internal/minecraft/bridged/ps3:r1", strings.NewReader(body)))
		if w.Code != 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if validGameID("a/b") || validGameID("") || !validGameID("ps3:3f2a-1") {
		t.Fatal("game id check")
	}
}
