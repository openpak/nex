package nexhost

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A banned account is refused with the NEX "account disabled" result on login and on every
// PID lookup -- the one WatchBans reads to drop a live connection.
func TestBannedAccountIsDisabled(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"account_banned"}`))
	}))
	defer stub.Close()

	cfg := testConfig()
	cfg.InternalURL = stub.URL
	host, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := host.PlayerByToken("nx2.good"); !errors.Is(err, errAccountBanned) {
		t.Fatalf("PlayerByToken: want errAccountBanned, got %v", err)
	}
	if errors.Is(errAccountBanned, errTokenRejected) {
		t.Fatal("a ban must not look like a rejected token: the console would loop fetching new ones")
	}
	_, nexErr := host.accountByPID(types.NewPID(1800000042))
	if !isBanned(nexErr) {
		t.Fatalf("accountByPID: want AccountDisabled, got %v", nexErr)
	}
}

func TestIsBanned(t *testing.T) {
	if !isBanned(nex.NewError(nex.ResultCodes.RendezVous.AccountDisabled, "")) {
		t.Error("AccountDisabled with the error bit set is a ban")
	}
	if isBanned(nex.NewError(nex.ResultCodes.RendezVous.InvalidPID, "")) || isBanned(nil) {
		t.Error("only AccountDisabled is a ban: a lookup outage must not drop sessions")
	}
}

func TestNEXPasswordResult(t *testing.T) {
	cases := []struct {
		err  error
		want uint32
	}{
		{status.Error(codes.InvalidArgument, "Account is banned or deleted"), nex.ResultCodes.RendezVous.AccountDisabled},
		{status.Error(codes.InvalidArgument, "No NEX account found"), nex.ResultCodes.RendezVous.InvalidUsername},
		{status.Error(codes.Internal, "Core account service unavailable"), nex.ResultCodes.RendezVous.InvalidUsername},
		{errors.New("dial tcp: refused"), nex.ResultCodes.RendezVous.InvalidUsername},
	}
	for _, c := range cases {
		if got := NEXPasswordResult(c.err); got != c.want {
			t.Errorf("%v: got %#x, want %#x", c.err, got, c.want)
		}
	}
}
