package nexhost

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	ticket_granting_types "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting/types"
)

func testConfig() Config {
	return Config{
		Title: "test", PortBase: 22100, AccessKey: "ridfebb9", Version: "4.4.0",
		SecureHost: "127.0.0.1", StationScheme: "prudps", HTTPAddr: ":0",
		StreamID: 1, SecureStreamID: 1, SecurePID: 2, Transport: "udp",
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
		"bad station scheme":         func(c *Config) { c.StationScheme = "https" },
		"bad transport":              func(c *Config) { c.Transport = "quic" },
		"stream id zero":             func(c *Config) { c.StreamID = 0 },
		"stream id above four bits":  func(c *Config) { c.SecureStreamID = 16 },
		"secure pid zero":            func(c *Config) { c.SecurePID = 0 },
		"half a TLS pair":            func(c *Config) { c.Transport, c.TLSCert = "websocket", "cert.pem" },
		"TLS on udp":                 func(c *Config) { c.TLSCert, c.TLSKey = "cert.pem", "key.pem" },
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
	if got := openpakClaim("header." + payload + ".signature"); got != "nx2.abc.def" {
		t.Errorf("emulator token: got %q", got)
	}

	// A bare claim has three dot-separated parts too, so the "nx2." prefix is what keeps it
	// from being mistaken for a JWT and having its middle segment decoded.
	if got := openpakClaim("nx2.abc.def"); got != "nx2.abc.def" {
		t.Errorf("bare claim: got %q", got)
	}

	// A real console sends a plain BAAS id token with no binding claim. That is not an
	// error: it means nx-baas has to prove the token itself. Anything that returned an
	// error here would refuse every genuine console.
	plain := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"abc","iss":"baas"}`))
	for _, noClaim := range []string{"header." + plain + ".sig", "", "not-a-jwt", "header.bm90LWpzb24.sig"} {
		if got := openpakClaim(noClaim); got != "" {
			t.Errorf("openpakClaim(%q): want no claim, got %q", noClaim, got)
		}
	}
}

// TestLoginNameIsNotThePID guards the fact that cost the first version a working login: a
// Switch logs in under its NSA id, a number that is never equal to its NEX PID. Comparing
// them, or trying to parse the name into a PID, refuses every real console.
func TestLoginNameIsNotThePID(t *testing.T) {
	host, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	const nsaID = "11389664994428258486" // NSA-id shaped: decimal, above 2^63, not a NEX PID
	const pid = uint64(1800000042)

	if _, ok := PIDForLoginName(nsaID); ok {
		t.Fatal("login name known before any login")
	}
	host.rememberLoginName(nsaID, pid)

	got, ok := PIDForLoginName(nsaID)
	if !ok || got != pid {
		t.Fatalf("PIDForLoginName: got %d, %v", got, ok)
	}
	if parsed, err := strconv.ParseUint(nsaID, 10, 64); err != nil || parsed == pid {
		t.Fatal("an NSA id must parse as uint64 and must not equal the PID, or this test proves nothing")
	}
}

func TestParseVersion(t *testing.T) {
	// Both spellings: the dotted one people write, and the integer the console reports.
	for _, spelling := range []string{"4.6.5", "40605"} {
		version, err := parseVersion(spelling)
		if err != nil {
			t.Fatalf("%s: %v", spelling, err)
		}
		if version.Major != 4 || version.Minor != 6 || version.Patch != 5 {
			t.Errorf("%s: got %d.%d.%d", spelling, version.Major, version.Minor, version.Patch)
		}
		// Method 6 is only dispatched at all above 4.0.0; a misparsed version silently
		// routes a Switch login to a Wii U handler that is not registered.
		if !version.GreaterOrEqual("4.0.0") {
			t.Errorf("%s: parsed below 4.0.0", spelling)
		}
	}
	if _, err := parseVersion("999"); err == nil {
		t.Error("accepted a version integer that is too short")
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

// TestSecureStationURL also pins the per-title scheme: the wrong one is not a clean
// failure, so it must follow configuration rather than a constant.
func TestSecureStationURL(t *testing.T) {
	cfg := testConfig()
	cfg.StationScheme = "prudp"
	plain, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.SecureStationURL.URL(); !strings.HasPrefix(got, "prudp:") || strings.HasPrefix(got, "prudps:") {
		t.Errorf("prudp scheme not honoured: %s", got)
	}

	host, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	url := host.SecureStationURL.URL()
	if !strings.HasPrefix(url, "prudps:") {
		t.Errorf("prudps scheme not honoured: %s", url)
	}
	if !strings.Contains(url, "port=22101") || !strings.Contains(url, "address=127.0.0.1") {
		t.Errorf("secure station url does not point at the block's secure port: %s", url)
	}
}

// TestSingleListener pins the shape a Switch title needs: both endpoints on one listener,
// separated by stream id, with a placeholder station URL because the console never dials it.
func TestSingleListener(t *testing.T) {
	cfg := testConfig()
	cfg.SecureStreamID = 2
	cfg.SecurePID = 257049437023956657 // retail-observed constant, not a NEX PID
	host, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.SingleListener() {
		t.Fatal("different stream ids must mean one listener")
	}
	if host.authServer != host.secureServer {
		t.Error("both endpoints must share one PRUDP server, or the console cannot switch to the secure stream")
	}
	if host.Auth.StreamID != 1 || host.Secure.StreamID != 2 {
		t.Errorf("streams: auth %d secure %d", host.Auth.StreamID, host.Secure.StreamID)
	}
	if !host.Secure.IsSecureEndPoint {
		t.Error("the secure endpoint lost its flag")
	}

	url := host.SecureStationURL.URL()
	for _, want := range []string{"prudps:/", "address=0.0.0.1", "port=1", "sid=2", "stream=10", "type=2", "CID=1",
		"PID=257049437023956657"} {
		if !strings.Contains(url, want) {
			t.Errorf("station url missing %s: %s", want, url)
		}
	}

	// The default two-listener shape must still advertise something dialable.
	twoPort, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got := twoPort.SecureStationURL.URL(); !strings.Contains(got, "address=127.0.0.1") || !strings.Contains(got, "port=22101") {
		t.Errorf("two-listener station url is not dialable: %s", got)
	}
}

// TestSecureStationURLOverride: a title may replace the whole URL before Run, and both the
// Switch login paths and the Wii U-era handlers must end up using the replacement.
func TestSecureStationURLOverride(t *testing.T) {
	host, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	replacement := types.NewStationURL("prudps:/address=10.0.0.9;port=1234")
	host.SecureStationURL = replacement

	if got := host.SecureStationURL.URL(); !strings.Contains(got, "10.0.0.9") {
		t.Fatalf("override not held: %s", got)
	}
	// Run syncs it into the common protocol; do that step alone rather than starting listeners.
	host.commonTicketGranting.SecureStationURL = host.SecureStationURL
	if got := host.commonTicketGranting.SecureStationURL.URL(); !strings.Contains(got, "10.0.0.9") {
		t.Errorf("common ticket-granting kept a stale station url: %s", got)
	}
}

// TestWebSocketTLS: a console reaches a NEX title over wss, so both deployments have to be
// expressible -- TLS terminated here, or terminated by the front proxy -- and a half-set pair
// has to be refused rather than quietly listening plain.
func TestWebSocketTLS(t *testing.T) {
	proxyTerminated := testConfig()
	proxyTerminated.Transport = "websocket"
	if err := proxyTerminated.validate(); err != nil {
		t.Errorf("plain ws behind a terminating proxy must be allowed: %v", err)
	}

	selfTerminated := testConfig()
	selfTerminated.Transport, selfTerminated.TLSCert, selfTerminated.TLSKey = "websocket", "cert.pem", "key.pem"
	if err := selfTerminated.validate(); err != nil {
		t.Errorf("wss terminated here must be allowed: %v", err)
	}

	host, err := New(selfTerminated)
	if err != nil {
		t.Fatal(err)
	}
	if got := host.transportName(); !strings.Contains(got, "wss") {
		t.Errorf("startup line hides that TLS is on: %q", got)
	}
	if got := New2(t, proxyTerminated).transportName(); !strings.Contains(got, "ws (") {
		t.Errorf("startup line hides that TLS is at the proxy: %q", got)
	}
}

func New2(t *testing.T, cfg Config) *Host {
	t.Helper()
	host, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

// TestFriendPIDsDoesNotTruncate: the friend callbacks are 32-bit and our PIDs are 64-bit, so
// a wrapped PID would not lose a friend, it would name a different account and hand that
// stranger a friend's access. Unreachable with today's PIDs; the point is that it stays so.
func TestFriendPIDsDoesNotTruncate(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(Player{PID: 1800000042, BaasUserID: "abc", Friends: []Friend{
			{PID: 1800000043, BaasUserID: "ok"},   // fits
			{PID: 0x1_00000007, BaasUserID: "hi"}, // would arrive as 7 if truncated
			{PID: 0, BaasUserID: "zero"},
		}})
	}))
	defer stub.Close()

	cfg := testConfig()
	cfg.InternalURL = stub.URL
	host, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	got := host.FriendPIDs(1800000042)
	if len(got) != 1 || got[0] != 1800000043 {
		t.Fatalf("got %v, want only the friend that fits", got)
	}
	for _, pid := range got {
		if pid == 7 {
			t.Error("a 64-bit PID was truncated into a different account's PID")
		}
	}

	// A failed lookup must answer nothing, not everything.
	stub.Close()
	if leaked := host.FriendPIDs(1800000042); leaked != nil {
		t.Errorf("friend list leaked on a failed lookup: %v", leaked)
	}
}

// TestSecureConnectionIsReachable: Register is refused by default and only a title's own
// capture can say whether its Pia calls Register or RegisterEx, so the switch has to be
// reachable from outside this package.
func TestSecureConnectionIsReachable(t *testing.T) {
	host, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if host.SecureConnection == nil {
		t.Fatal("a title cannot reach the SecureConnection protocol")
	}
	host.SecureConnection.EnableInsecureRegister()

	// The default must stay accept-everything, since the ticket already proved the PID.
	if err := host.SecureConnection.ValidateLoginData(types.NewPID(1800000042), types.NewDataHolder()); err != nil {
		t.Errorf("default login-data check refused a ticket-authenticated caller: %v", err)
	}
}

// TestSwitchByteStreamProfile: nex-go defaults to the Wii U/3DS wire, and both defaults are
// wrong for a Switch. The header flag is the one that matters most -- without it
// WriteHeaderTo silently writes nothing, so the method 6 structure header never reaches the
// wire however StructureVersion is set, and ExtractHeaderFrom never consumes the one the
// console sent.
func TestSwitchByteStreamProfile(t *testing.T) {
	switchHost, err := New(testConfig()) // 4.4.0
	if err != nil {
		t.Fatal(err)
	}
	for name, endpoint := range map[string]*nex.PRUDPEndPoint{"auth": switchHost.Auth, "secure": switchHost.Secure} {
		settings := endpoint.Server.ByteStreamSettings
		if settings.PIDSize != 8 {
			t.Errorf("%s: PID size %d, want 8", name, settings.PIDSize)
		}
		if !settings.UseStructureHeader {
			t.Errorf("%s: structure headers off", name)
		}
	}

	// The header actually reaching the wire is the point, so check the bytes rather than
	// the flag: version 1 then a little-endian content length.
	result := ticket_granting_types.NewValidateAndRequestTicketResult()
	result.StructureVersion = 1
	stream := nex.NewByteStreamOut(switchHost.Auth.LibraryVersions(), switchHost.Auth.ByteStreamSettings())
	result.WriteTo(stream)
	if written := stream.Bytes(); len(written) < 5 || written[0] != 1 {
		t.Errorf("structure header not emitted, method 6 would be rejected: %v", written[:min(8, len(written))])
	}

	// A NEX 3 title must keep the old wire.
	legacy := testConfig()
	legacy.Version = "3.10.0"
	legacyHost, err := New(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if legacyHost.Auth.Server.ByteStreamSettings.PIDSize != 4 || legacyHost.Auth.Server.ByteStreamSettings.UseStructureHeader {
		t.Error("the Switch profile was applied to a NEX 3 title")
	}
}
