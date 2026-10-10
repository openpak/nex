package nexhost

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ticket_granting_types "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting/types"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A login that succeeds and one that is refused must leave no pid, nickname, account id,
// login name or token in the log: tags only.
func TestLoginLogsCarryNoPersonalData(t *testing.T) {
	const (
		pid       = uint64(1800000042)
		nickname  = "leia-nickname"
		baasID    = "baas-user-7f3a91"
		loginName = "11389664994428258486"
		goodToken = "nx2.good"
		badToken  = "nx2.bad"
	)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ OpenPak string }
		json.NewDecoder(r.Body).Decode(&in)
		if in.OpenPak != goodToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(Player{PID: pid, BaasUserID: baasID, Nickname: nickname,
			Friends: []Friend{{PID: 1800000043, BaasUserID: "baas-friend-02", Nickname: "han-solo"}}})
	}))
	defer stub.Close()

	cfg := testConfig()
	cfg.InternalURL = stub.URL
	host, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var out lockedBuf
	log.SetOutput(&out)
	defer log.SetOutput(log.Writer())

	login := func(token string) {
		info := ticket_granting_types.NewAuthenticationInfo()
		info.Token = types.String(token)
		host.authorize(host.Auth, loginName, types.DataHolder{Object: info})
	}
	login(goodToken)
	login(badToken)

	got := out.String()
	if !strings.Contains(got, "login pid=~") || !strings.Contains(got, "login refused:") {
		t.Fatalf("expected a tagged login line and a refusal line:\n%s", got)
	}
	for _, secret := range []string{
		goodToken, badToken, loginName, nickname, baasID, "baas-friend-02", "han-solo",
		"1800000042", "1800000043", "6b49d22a", // decimal pids and the hex form of the pid
	} {
		if strings.Contains(got, secret) {
			t.Errorf("log contains %q:\n%s", secret, got)
		}
	}
}
