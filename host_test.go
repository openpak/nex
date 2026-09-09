package nexhost

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
)

func testConfig() Config {
	return Config{
		Title: "test", PortBase: 22100, AccessKey: "ridfebb9", Version: "4.4.0",
		SecureHost: "127.0.0.1", HTTPAddr: ":0",
		InternalURL: "http://127.0.0.1:1", InternalKey: "k",
	}
}

func TestConfigValidate(t *testing.T) {
	if err := testConfig().validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"port outside the NEX range": func(c *Config) { c.PortBase = 22200 },
		"port not a block base":      func(c *Config) { c.PortBase = 22105 },
		"no access key":              func(c *Config) { c.AccessKey = "" },
		"no internal key":            func(c *Config) { c.InternalKey = "" },
		"bad version":                func(c *Config) { c.Version = "4.4" },
	} {
		cfg := testConfig()
		mutate(&cfg)
		if err := cfg.validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestOpenPakClaim(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"openpak":"nx2.abc.def"}`))
	idToken := "header." + payload + ".signature"

	got, err := openpakClaim(idToken)
	if err != nil || got != "nx2.abc.def" {
		t.Fatalf("jwt: got %q, %v", got, err)
	}

	// A bare openpak token has three dot-separated parts too, so the "nx2." prefix is what
	// keeps it from being mistaken for a JWT and having its middle segment decoded.
	if got, err := openpakClaim("nx2.abc.def"); err != nil || got != "nx2.abc.def" {
		t.Fatalf("bare token: got %q, %v", got, err)
	}

	for _, bad := range []string{"", "  ", "header.bm90LWpzb24.sig"} {
		if _, err := openpakClaim(bad); !errors.Is(err, errTokenRejected) {
			t.Errorf("openpakClaim(%q): want errTokenRejected, got %v", bad, err)
		}
	}
}

func TestPlayerByToken(t *testing.T) {
	var gotKey, gotClaim string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Internal-Key")
		var in struct{ OpenPak string }
		json.NewDecoder(r.Body).Decode(&in)
		gotClaim = in.OpenPak
		if in.OpenPak != "nx2.good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(Player{PID: 1800000042, BaasUserID: "abc", Nickname: "leia",
			Friends: []Friend{{PID: 1800000043, BaasUserID: "def", Nickname: "hansolo"}}})
	}))
	defer stub.Close()

	cfg := testConfig()
	cfg.InternalURL = stub.URL
	host, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	player, err := host.PlayerByToken("nx2.good")
	if err != nil {
		t.Fatalf("PlayerByToken: %v", err)
	}
	if player.PID != 1800000042 || len(player.Friends) != 1 {
		t.Errorf("got %+v", player)
	}
	if gotKey != "k" || gotClaim != "nx2.good" {
		t.Errorf("nx-baas saw key=%q claim=%q", gotKey, gotClaim)
	}

	// nx-baas refusing the token must stay distinguishable from an unknown account: only
	// the first makes the console fetch a new token.
	if _, err := host.PlayerByToken("nx2.bad"); !errors.Is(err, errTokenRejected) {
		t.Errorf("rejected token: want errTokenRejected, got %v", err)
	}
}

// TestIssueTicket walks the ticket back the way the console and the secure server do: the
// client decrypts the outer ticket with the source key it was handed, the secure server
// decrypts the internal ticket with its own key, and both must agree on the session key.
func TestIssueTicket(t *testing.T) {
	host, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	sourceKey := []byte("0123456789abcdef")
	sourcePID := types.NewPID(1800000042)

	ticket, err := issueTicket(host.Auth, sourcePID, host.secureAccount, sourceKey)
	if err != nil {
		t.Fatal(err)
	}

	// The client's half.
	decrypted, err := nex.NewKerberosEncryption(sourceKey).Decrypt(ticket)
	if err != nil {
		t.Fatalf("client could not decrypt the ticket with the source key: %v", err)
	}
	stream := nex.NewByteStreamIn(decrypted, host.Auth.LibraryVersions(), host.Auth.ByteStreamSettings())
	sessionKey := stream.ReadBytesNext(int64(host.authServer.SessionKeyLength))
	var targetPID types.PID
	if err := targetPID.ExtractFrom(stream); err != nil {
		t.Fatal(err)
	}
	internalData := types.NewBuffer(nil)
	if err := internalData.ExtractFrom(stream); err != nil {
		t.Fatal(err)
	}
	if targetPID != host.secureAccount.PID {
		t.Errorf("target pid: got %d, want %d", targetPID, host.secureAccount.PID)
	}

	// The secure server's half.
	internal := nex.NewKerberosTicketInternalData(host.secureServer)
	targetKey := nex.DeriveKerberosKey(host.secureAccount.PID, []byte(host.secureAccount.Password))
	err = internal.Decrypt(nex.NewByteStreamIn(internalData, host.Secure.LibraryVersions(), host.Secure.ByteStreamSettings()), targetKey)
	if err != nil {
		t.Fatalf("secure server could not decrypt the internal ticket: %v", err)
	}
	if internal.SourcePID != sourcePID {
		t.Errorf("source pid: got %d, want %d", internal.SourcePID, sourcePID)
	}
	if string(internal.SessionKey) != string(sessionKey) {
		t.Error("session key the client sees differs from the one the secure server reads")
	}

	// A ticket must not open with anything but the key the client was handed.
	if _, err := nex.NewKerberosEncryption([]byte("fedcba9876543210")).Decrypt(ticket); err == nil {
		t.Error("ticket decrypted with the wrong source key")
	}
}

func TestSecureStationURL(t *testing.T) {
	host, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	url := host.secureStationURL().String()
	if !strings.Contains(url, "port=22101") || !strings.Contains(url, "address=127.0.0.1") {
		t.Errorf("secure station url does not point at the block's secure port: %s", url)
	}
}
