// Package nexhost is the shared NEX host that every OpenPak NEX title server imports.
//
// Pretendo's AGPL-3.0 nex-go and nex-protocols-go already implement PRUDP, kerberos and
// the standard NEX protocols. This package is only the part they leave to the operator:
//
//  1. Switch login. nex-protocols-common-go implements Login and LoginEx (the Wii U/3DS
//     names) but not ValidateAndRequestTicketWithCustomData or
//     ValidateAndRequestTicketWithParam, and those are the only ticket-granting methods a
//     NEX 4 Switch title uses. See login.go.
//  2. Identity. A console proves itself with a BAAS id token, not a NEX password; the
//     account behind it is resolved through nx-baas's internal API. See account.go.
//  3. The OpenPak service shape: one port block from ports.md, env-driven config, and an
//     HTTP /healthz, because nothing in OpenPak infers liveness from a UDP socket.
//
// A title server does:
//
//	h, err := nexhost.New(nexhost.FromEnv())
//	h.Secure.RegisterServiceProtocol(myMatchmakeExtensionProtocol)
//	log.Fatal(h.Run())
package nexhost

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_secure_connection "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
	common_ticket_granting "github.com/PretendoNetwork/nex-protocols-common-go/v2/ticket-granting"
	secure_connection "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"
	ticket_granting "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting"
)

// Config is the whole configuration of a NEX title server. Everything here is read from the
// environment by FromEnv; a title server that wants a different value sets the field itself.
type Config struct {
	Title      string // name in logs only
	PortBase   int    // NEX block base from ports.md: auth listens on +0, secure on +1
	AccessKey  string // the title's NEX access key
	Version    string // NEX library version the title was built against, e.g. "4.4.0"
	SecureHost string // address the console is told to reach the secure server on
	// StationScheme is "prudps" or "prudp", per title. It decides whether the console
	// treats the secure server as a Pia station. The wrong one is not a clean failure:
	// menus keep working and the title gives up later, when it actually needs the session.
	StationScheme string
	// StreamID is the PRUDP virtual port the authentication endpoint binds, and
	// SecureStreamID the one the secure endpoint binds. nex-go routes a packet by the stream
	// ID it is addressed to and silently drops anything for an unbound one, logging "trying
	// to connect to unbound PRUDPEndPoint".
	//
	// Whether the two are equal decides the shape of the server, because nex-go cannot bind
	// two endpoints with the same stream ID to one listener:
	//
	//   - equal (both 1, the default): two listeners, authentication on +0 and secure on +1,
	//     and the console dials the secure port from the station URL it is given.
	//   - different: ONE listener on +0 carrying both endpoints. The console never opens a
	//     second socket, it switches virtual port on the connection it already has, so the
	//     station URL it is handed is a placeholder. Retail shape here is stream 1 and
	//     stream 2. The block's +1 port stays reserved and unused.
	//
	// This one is genuinely per title and there is no safe default: working servers exist for
	// both shapes. Set it from what the title is known to do, and see the README table.
	//
	// The symptom of getting it wrong is not silence, despite appearances: nex-go logs
	// "trying to connect to unbound PRUDPEndPoint 2" for every packet a console sends to a
	// stream nothing is bound to. That line means "set NEX_SECURE_STREAM_ID=2".
	StreamID       uint8
	SecureStreamID uint8
	// SecurePID is the secure server's principal ID: the kerberos ticket's target, the PID in
	// the secure station URL, and the source of any notification the server pushes. 2 is the
	// conventional value; some titles have a retail-observed constant instead.
	SecurePID uint64
	// SourceKeyBytes is how many random bytes the source key is.
	//
	// The invariant that actually matters is that the key the ticket is encrypted with is the
	// key advertised in the response: RC4 takes any length and the console is told the key
	// rather than deriving it, so a mismatch between those two is what rejects a ticket, not
	// the number itself. This is therefore not a protocol constant.
	//
	// It is a knob rather than a constant because two titles have direct evidence for two
	// different values *for themselves*: a public reference server known to work against a
	// retail Super Mario Bros. 35 sends 16, and a live Mario Kart 8 Deluxe server sends 32.
	// Neither generalises. The failure mode is a login that fails with nothing logged on
	// either side, so this is the first thing to flip when a console refuses a ticket.
	SourceKeyBytes int
	// Transport is "websocket" (the default) or "udp". Every title that imports this package
	// is a Switch title and every Switch title speaks PRUDP over WebSocket, so the default is
	// the one that works; "udp" is the Wii U and 3DS era form and is here for a caller that
	// does not exist yet.
	//
	// Selecting websocket also selects PRUDP Lite: nex-go reads a packet as Lite only when a
	// websocket server is present, so the transport and the packet format are one choice.
	Transport string
	// SecureTransport overrides Transport for the secure listener alone; empty means "same
	// as Transport", which is every title but one. MHGU needs them to differ: its auth stage
	// is wss behind Traefik's SNI passthrough (the measured server has no UDP auth listener
	// at all, and the live dynamic.yml routes g2896bd04's SNI to the auth port over TCP),
	// while its secure stage is the 3DS-era raw PRUDP over UDP.
	SecureTransport string
	// TLSCert and TLSKey terminate TLS on the websocket listener. A console reaches a NEX
	// title over wss, so one of two things has to be true: either Traefik terminates and
	// forwards plain ws here (leave both empty), or Traefik passes the SNI through and this
	// server terminates (set both). Both are real OpenPak deployments -- ports.md's 21xxx
	// range exists for the passthrough shape -- so neither is forced. Setting exactly one is
	// always a mistake and is refused.
	TLSCert     string
	TLSKey      string
	HTTPAddr    string // /healthz listener
	InternalURL string // nx-baas game/internal API
	InternalKey string // nx-baas NX_INTERNAL_KEY, sent as X-Internal-Key
	// The Wii U and 3DS titles (Run shape) log consoles in through the account server and
	// read friend lists from the friends server, both over gRPC; Switch titles ignore these.
	DatabaseURL     string // the title's Postgres (NEX_DATABASE_URL); Switch titles read their own <TITLE>_DATABASE_URL
	AccountGRPCAddr string // nn-account gRPC, host:port (NEX_ACCOUNT_GRPC_ADDR)
	AccountGRPCKey  string // its API key (NEX_ACCOUNT_GRPC_KEY)
	FriendsGRPCAddr string // nn-friends gRPC, host:port (NEX_FRIENDS_GRPC_ADDR)
	FriendsGRPCKey  string // its API key (NEX_FRIENDS_GRPC_KEY)
}

// Defaults is the configuration before the environment and before any title's own values.
func Defaults() Config {
	return Config{
		Title:          "nex",
		Version:        "4.4.0",
		SecureHost:     "127.0.0.1",
		StationScheme:  "prudps",
		StreamID:       1,
		SecureStreamID: 1,
		SecurePID:      2,
		SourceKeyBytes: 32,
		Transport:      "websocket",
		InternalURL:    "http://127.0.0.1:20070",
	}
}

// FromEnv reads the configuration from the environment over Defaults.
func FromEnv() Config { return FromEnvWith(Defaults()) }

// FromEnvWith reads the configuration from the environment over base: a variable that is set
// wins, one that is not leaves base's value. A title's Defaults go into base, so the
// operator's environment still has the last word.
func FromEnvWith(base Config) Config {
	c := base
	c.Title = env("NEX_TITLE", c.Title)
	c.PortBase = atoiOr(os.Getenv("NEX_PORT_BASE"), c.PortBase)
	c.AccessKey = env("NEX_ACCESS_KEY", c.AccessKey)
	c.Version = env("NEX_VERSION", c.Version)
	c.SecureHost = env("NEX_SECURE_HOST", c.SecureHost)
	c.StationScheme = env("NEX_STATION_SCHEME", c.StationScheme)
	c.StreamID = uint8(atoiOr(os.Getenv("NEX_STREAM_ID"), int(c.StreamID)))
	c.SecureStreamID = uint8(atoiOr(os.Getenv("NEX_SECURE_STREAM_ID"), int(c.SecureStreamID)))
	c.SecurePID = uint64(atoiOr(os.Getenv("NEX_SECURE_PID"), int(c.SecurePID)))
	c.SourceKeyBytes = atoiOr(os.Getenv("NEX_SOURCE_KEY_BYTES"), c.SourceKeyBytes)
	c.Transport = env("NEX_TRANSPORT", c.Transport)
	c.SecureTransport = env("NEX_SECURE_TRANSPORT", c.SecureTransport)
	c.TLSCert = env("NEX_TLS_CERT", c.TLSCert)
	c.TLSKey = env("NEX_TLS_KEY", c.TLSKey)
	c.HTTPAddr = env("NEX_HTTP_ADDR", c.HTTPAddr)
	if c.HTTPAddr == "" {
		c.HTTPAddr = fmt.Sprintf(":%d", c.PortBase+2)
	}
	c.InternalURL = env("NX_INTERNAL_URL", c.InternalURL)
	c.InternalKey = env("NX_INTERNAL_KEY", c.InternalKey)
	c.DatabaseURL = env("NEX_DATABASE_URL", c.DatabaseURL)
	c.AccountGRPCAddr = env("NEX_ACCOUNT_GRPC_ADDR", c.AccountGRPCAddr)
	c.AccountGRPCKey = env("NEX_ACCOUNT_GRPC_KEY", c.AccountGRPCKey)
	c.FriendsGRPCAddr = env("NEX_FRIENDS_GRPC_ADDR", c.FriendsGRPCAddr)
	c.FriendsGRPCKey = env("NEX_FRIENDS_GRPC_KEY", c.FriendsGRPCKey)
	return c
}

// ServeHealth answers GET /healthz on addr for a Run-shaped title, so "is it up" is an HTTP
// question for every title regardless of who owns the PRUDP listeners. It returns as soon as
// the listener is bound; the server runs for the life of the process.
func ServeHealth(addr, title string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "ok %s\n", title)
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("healthz %s: %w", addr, err)
	}
	go func() { _ = http.Serve(ln, mux) }()
	return nil
}

func atoiOr(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// AuthPort and SecurePort are the two listeners of the title's block (ports.md: +0 auth, +1
// secure). In single-listener mode SecurePort is reserved but nothing binds it.
func (c Config) AuthPort() int   { return c.PortBase }
func (c Config) SecurePort() int { return c.PortBase + 1 }

// SingleListener reports whether both endpoints share one listener, which they do exactly
// when they are on different streams -- see StreamID.
func (c Config) SingleListener() bool { return c.SecureStreamID != c.StreamID }

// SecureTransportOr is the secure listener's transport: NEX_SECURE_TRANSPORT when set,
// otherwise NEX_TRANSPORT.
func (c Config) SecureTransportOr() string {
	if c.SecureTransport != "" {
		return c.SecureTransport
	}
	return c.Transport
}

// validate fails loudly at startup rather than quietly serving on the wrong port.
func (c Config) validate() error {
	inNEXRange := (c.PortBase >= 22100 && c.PortBase <= 22190) || (c.PortBase >= 22300 && c.PortBase <= 22590)
	switch {
	case !inNEXRange || c.PortBase%10 != 0:
		return fmt.Errorf("NEX_PORT_BASE=%d is not a NEX block base: want a multiple of ten in 22100-22199 or 22300-22599 (ports.md)", c.PortBase)
	case c.AccessKey == "":
		return errors.New("NEX_ACCESS_KEY is required: it is the title's NEX access key")
	case c.InternalKey == "":
		return errors.New("NX_INTERNAL_KEY is required: without it every login is refused by nx-baas")
	case c.StationScheme != "prudps" && c.StationScheme != "prudp":
		return fmt.Errorf("NEX_STATION_SCHEME=%q: want prudps or prudp", c.StationScheme)
	case c.Transport != "udp" && c.Transport != "websocket":
		return fmt.Errorf("NEX_TRANSPORT=%q: want udp or websocket", c.Transport)
	case c.SecureTransport != "" && c.SecureTransport != "udp" && c.SecureTransport != "websocket":
		return fmt.Errorf("NEX_SECURE_TRANSPORT=%q: want udp or websocket", c.SecureTransport)
	case c.SecureTransport != "" && c.SingleListener():
		// One listener carries both streams, so there is only one transport to pick.
		return errors.New("NEX_SECURE_TRANSPORT is set but NEX_SECURE_STREAM_ID makes one listener carry both streams")
	case c.StreamID == 0 || c.StreamID > 15 || c.SecureStreamID == 0 || c.SecureStreamID > 15:
		// PRUDP v0 and v1 carry the stream ID in four bits.
		return fmt.Errorf("stream IDs must be 1-15, got auth %d secure %d", c.StreamID, c.SecureStreamID)
	case c.SourceKeyBytes < 16 || c.SourceKeyBytes > 64:
		// The two attested values are 16 and 32; the range is wide enough for a third if
		// evidence turns one up, and narrow enough that a typo does not reach a console.
		return fmt.Errorf("NEX_SOURCE_KEY_BYTES=%d is outside 16-64", c.SourceKeyBytes)
	case c.SecurePID == 0:
		return errors.New("NEX_SECURE_PID must not be 0: it is the kerberos ticket's target")
	case (c.TLSCert == "") != (c.TLSKey == ""):
		return errors.New("NEX_TLS_CERT and NEX_TLS_KEY go together: set both to terminate TLS here, or neither to let the front proxy do it")
	case c.Transport == "udp" && c.SecureTransportOr() == "udp" && c.TLSCert != "":
		return errors.New("NEX_TLS_CERT is set but both listeners are NEX_TRANSPORT=udp: PRUDP over UDP has no TLS layer")
	}
	if _, err := parseVersion(c.Version); err != nil {
		return err
	}
	return nil
}

// parseVersion accepts both spellings of a NEX version: "4.6.5", and the integer form the
// console reports as apiVersionGeneral, 40605 = major*10000 + minor*100 + patch.
func parseVersion(v string) (*nex.LibraryVersion, error) {
	if n, err := strconv.Atoi(v); err == nil {
		if n < 10000 || n > 999999 {
			return nil, fmt.Errorf("NEX_VERSION=%q is not a NEX version integer like 40605", v)
		}
		return nex.NewLibraryVersion(n/10000, (n/100)%100, n%100), nil
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("NEX_VERSION=%q is not major.minor.patch", v)
	}
	n := make([]int, 3)
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("NEX_VERSION=%q is not major.minor.patch", v)
		}
		n[i] = x
	}
	return nex.NewLibraryVersion(n[0], n[1], n[2]), nil
}

// Host is a running NEX title server: an authentication endpoint and a secure endpoint,
// wired to OpenPak accounts. A title registers its own protocols on Secure before Run.
type Host struct {
	Config Config

	Auth   *nex.PRUDPEndPoint
	Secure *nex.PRUDPEndPoint

	// SecureConnection is the common SecureConnection protocol, kept so a title can reach it.
	// Two things on it are title decisions this package must not make:
	//
	//   - EnableInsecureRegister(). Register (0x0B.1) is refused by default and only
	//     RegisterEx (0x0B.4) works. A title on an older Pia may call plain Register, and
	//     only its own capture can say so.
	//   - ValidateLoginData. It is set here to accept, because by the time a packet reaches
	//     the secure endpoint the caller's PID came out of a kerberos ticket this process
	//     minted minutes ago. A title that finds something it must check can replace it.
	SecureConnection *common_secure_connection.CommonProtocol

	// SecureStationURL is what the login response tells the console about the secure server.
	// A title may overwrite it before Run to match a retail-observed shape; the default is
	// built from the configuration and is a placeholder in single-listener mode, because
	// there is nothing for the console to dial.
	SecureStationURL types.StationURL

	authServer   *nex.PRUDPServer
	secureServer *nex.PRUDPServer

	// The two special accounts. Their passwords are the kerberos keys for tickets this
	// process issues and reads back, so they are generated per process: nothing outside
	// this process ever derives a key from them, and a restart invalidates only tickets
	// that nex-go would already have rejected as older than two minutes.
	authAccount   *nex.Account
	secureAccount *nex.Account

	// Kept so Run can hand the final station URL to the Wii U-era handlers too, after a
	// title has had its chance to overwrite it.
	commonTicketGranting     *common_ticket_granting.CommonProtocol
	secureConnectionProtocol *secure_connection.Protocol
}

// New builds the host. It does not listen; call Run.
func New(cfg Config) (*Host, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	version, _ := parseVersion(cfg.Version)

	h := &Host{
		Config: cfg,
		// The two non-user accounts every NEX server has. The names are the ones NEX
		// itself uses; see the Pretendo wiki's kerberos page.
		authAccount:   nex.NewAccount(types.NewPID(1), "Quazal Authentication", randomPassword(), false),
		secureAccount: nex.NewAccount(types.NewPID(cfg.SecurePID), "Quazal Rendez-Vous", randomPassword(), false),
	}

	h.authServer, h.Auth = h.newServer(version, cfg.StreamID)
	if cfg.SingleListener() {
		// One listener, two endpoints. nex-go already routes a packet to the endpoint whose
		// stream ID it is addressed to, so this is the whole of it.
		h.secureServer = h.authServer
		h.Secure = h.bindEndpoint(h.authServer, cfg.SecureStreamID)
	} else {
		h.secureServer, h.Secure = h.newServer(version, cfg.SecureStreamID)
	}
	h.Auth.ServerAccount = h.authAccount
	h.Secure.ServerAccount = h.secureAccount
	h.Secure.IsSecureEndPoint = true
	h.SecureStationURL = h.defaultSecureStationURL()

	h.registerTicketGranting()
	h.registerSecureConnection()
	return h, nil
}

func (h *Host) bindEndpoint(server *nex.PRUDPServer, streamID uint8) *nex.PRUDPEndPoint {
	endpoint := nex.NewPRUDPEndPoint(streamID)
	endpoint.AccountDetailsByPID = h.accountByPID
	endpoint.AccountDetailsByUsername = h.accountByUsername
	server.BindPRUDPEndPoint(endpoint)

	// Packet-level tracing for online debugging (NEX_PACKET_LOG=1): every DATA packet's
	// virtual ports, size and the first RMC bytes, so a stalled title shows exactly where
	// the conversation stops and which stream it was talking to when it did.
	if os.Getenv("NEX_PACKET_LOG") == "1" {
		endpoint.OnData(func(packet nex.PacketInterface) {
			p, ok := packet.(nex.PRUDPPacketInterface)
			if !ok {
				log.Printf("[%s] DATA unhandled packet type %T", h.Config.Title, packet)
				return
			}

			payload := p.Payload()
			head := payload
			if len(head) > 6 {
				head = head[:6]
			}

			extra := ""
			if rmc := p.RMCMessage(); rmc != nil {
				extra = fmt.Sprintf(" proto=%d method=0x%X params=%dB", rmc.ProtocolID, rmc.MethodID, len(rmc.Parameters))
			}

			log.Printf("[%s] DATA dst-stream=%d src-stream=%d len=%d % X%s", h.Config.Title,
				p.DestinationVirtualPortStreamID(), p.SourceVirtualPortStreamID(), len(payload), head, extra)
		})
	}

	return endpoint
}

func (h *Host) newServer(version *nex.LibraryVersion, streamID uint8) (*nex.PRUDPServer, *nex.PRUDPEndPoint) {
	server := nex.NewPRUDPServer()
	server.AccessKey = h.Config.AccessKey
	server.LibraryVersions.SetDefault(version)
	// Switch tickets use the newer internal-ticket format: a random key in the clear next
	// to the encrypted body, final key MD5(target key + random key). Kinnay's kerberos page.
	server.KerberosTicketVersion = 1
	server.SessionKeyLength = 32

	// The Switch byte-stream profile. nex-go defaults to the Wii U/3DS wire: 4-byte PIDs and
	// no structure headers. NEX 4 is the Switch generation and needs both changed, and this
	// is per console, not per title, so it belongs here rather than in forty title mains.
	//
	// It is not cosmetic in either direction. Without the header flag, ExtractHeaderFrom
	// does not consume the header the console sent, so every field of a login request is
	// read one header out of place; and WriteHeaderTo silently writes nothing, so a
	// response's StructureVersion never reaches the wire no matter what it is set to. That
	// second half is what makes the method 6 structure header actually appear.
	//
	// A title that finds otherwise can still change these: Host.Auth and Host.Secure expose
	// Server.ByteStreamSettings.
	if version.GreaterOrEqual("4.0.0") {
		server.ByteStreamSettings.PIDSize = 8
		server.ByteStreamSettings.UseStructureHeader = true
	}

	return server, h.bindEndpoint(server, streamID)
}

func (h *Host) registerTicketGranting() {
	protocol := ticket_granting.NewProtocol()
	common := common_ticket_granting.NewCommonProtocol(protocol)
	common.SecureServerAccount = h.secureAccount
	common.SessionKeyLength = h.authServer.SessionKeyLength
	common.BuildName = types.NewString("OpenPak " + h.Config.Title)
	h.commonTicketGranting = common

	// The Switch login methods. nex-protocols-common-go leaves both unset, so on a NEX 4
	// title its ticket-granting protocol answers "not implemented" to every login.
	protocol.ValidateAndRequestTicketWithParam = h.validateAndRequestTicketWithParam
	protocol.ValidateAndRequestTicketWithCustomData = h.validateAndRequestTicketWithCustomData

	h.Auth.RegisterServiceProtocol(protocol)
}

func (h *Host) registerSecureConnection() {
	protocol := secure_connection.NewProtocol()
	common := common_secure_connection.NewCommonProtocol(protocol)
	// RegisterEx re-checks the login data. By the time a packet reaches the secure endpoint
	// the caller's PID came out of a kerberos ticket this process minted minutes ago, so
	// there is nothing left for a second token check to prove. A title that disagrees
	// replaces this through Host.SecureConnection.
	common.ValidateLoginData = func(types.PID, types.DataHolder) *nex.Error { return nil }

	// TestConnectivity (0x0B.5) takes nothing and returns nothing; the console is asking
	// whether the socket still carries RMC, and the answer is the reply arriving at all.
	// nex-protocols-common-go leaves it unset, so it answers Core::NotImplemented -- one
	// title was measured calling it 23 times in six hours, and it was that title's only
	// error. Nothing here is title-specific, so it is answered for everyone rather than
	// left for forty titles to set identically.
	protocol.TestConnectivity = func(_ error, packet nex.PacketInterface, callID uint32) (*nex.RMCMessage, *nex.Error) {
		endpoint := packet.Sender().Endpoint()
		response := nex.NewRMCSuccess(endpoint, nil)
		response.ProtocolID = secure_connection.ProtocolID
		response.MethodID = secure_connection.MethodTestConnectivity
		response.CallID = callID
		return response, nil
	}

	h.SecureConnection = common
	h.secureConnectionProtocol = protocol
	h.Secure.RegisterServiceProtocol(protocol)
}

// defaultSecureStationURL builds what the login response says about the secure server.
//
// In single-listener mode the address and port are a placeholder rather than a mistake: the
// console does not dial them, it switches to the secure stream on the connection it already
// has. Only the stream id in this URL actually steers it there.
func (h *Host) defaultSecureStationURL() types.StationURL {
	address, port := h.Config.SecureHost, uint16(h.Config.SecurePort())
	if h.Config.SingleListener() {
		address, port = "0.0.0.1", 1

		// An operator who names a real secure host wants the console to dial that host
		// rather than switch streams on the connection it already has: MK8D was observed
		// doing exactly that — it never opens the block's second port and it never sends
		// secure traffic to the authentication connection, it re-dials. Hand it the named
		// host on the front port the internet can actually reach (the edge's 443), with
		// the stream id steering the new connection to this endpoint. NEX_SECURE_FRONT_PORT
		// overrides the port for an edge that listens elsewhere.
		if h.Config.SecureHost != "" && h.Config.SecureHost != "127.0.0.1" {
			address = h.Config.SecureHost
			port = uint16(atoiOr(os.Getenv("NEX_SECURE_FRONT_PORT"), 443))
		}
	}
	url := types.NewStationURL("")
	urlType := constants.StationURLPRUDPS
	if h.Config.StationScheme == "prudp" {
		urlType = constants.StationURLPRUDP
	}
	url.SetURLType(urlType)
	url.SetAddress(address)
	url.SetPortNumber(port)
	url.SetConnectionID(1)
	url.SetPrincipalID(h.secureAccount.PID)
	url.SetStreamID(h.Config.SecureStreamID)
	url.SetStreamType(constants.StreamTypeRVSecure)
	url.SetType(2)
	return url
}

// Run serves /healthz and both PRUDP listeners. It blocks.
func (h *Host) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if h.Config.SingleListener() {
			fmt.Fprintf(w, "ok %s %s=%d auth-stream=%d secure-stream=%d\n", h.Config.Title,
				h.Config.Transport, h.Config.AuthPort(), h.Config.StreamID, h.Config.SecureStreamID)
			return
		}
		fmt.Fprintf(w, "ok %s auth=%d/%s secure=%d/%s\n", h.Config.Title,
			h.Config.AuthPort(), h.Config.Transport,
			h.Config.SecurePort(), h.Config.SecureTransportOr())
	})
	srv := &http.Server{Addr: h.Config.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	// A title may have overwritten SecureStationURL since New; the Wii U-era handlers hold
	// their own copy, so hand them the final one now rather than the one built at New.
	h.commonTicketGranting.SecureStationURL = h.SecureStationURL
	go WatchBans(h.Config.Title, h.Secure)

	if h.Config.SingleListener() {
		log.Printf("[%s] %s :%d (auth stream %d, secure stream %d) healthz %s nex %s",
			h.Config.Title, h.transportName(h.Config.Transport), h.Config.AuthPort(),
			h.Config.StreamID, h.Config.SecureStreamID, h.Config.HTTPAddr, h.Config.Version)
		go h.listen(h.authServer, h.Config.AuthPort(), h.Config.Transport)
	} else {
		log.Printf("[%s] auth :%d %s, secure :%d %s, healthz %s nex %s",
			h.Config.Title, h.Config.AuthPort(), h.transportName(h.Config.Transport),
			h.Config.SecurePort(), h.transportName(h.Config.SecureTransportOr()),
			h.Config.HTTPAddr, h.Config.Version)
		go h.listen(h.authServer, h.Config.AuthPort(), h.Config.Transport)
		go h.listen(h.secureServer, h.Config.SecurePort(), h.Config.SecureTransportOr())
	}
	return srv.ListenAndServe()
}

// transportName says which of the three listener shapes is running. A console failing its
// TLS handshake against a plain ws listener looks like silence, so this is worth a line.
func (h *Host) transportName(transport string) string {
	if transport != "websocket" {
		return "udp"
	}
	if h.Config.TLSCert != "" {
		return "wss (TLS terminated here)"
	}
	return "ws (TLS at the front proxy)"
}

// listen blocks; both callers run it in a goroutine.
func (h *Host) listen(server *nex.PRUDPServer, port int, transport string) {
	switch {
	case transport == "websocket" && h.Config.TLSCert != "":
		server.ListenWebSocketSecure(port, h.Config.TLSCert, h.Config.TLSKey)
	case transport == "websocket":
		server.ListenWebSocket(port)
	default:
		server.Listen(port)
	}
}
