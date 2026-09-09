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
	Title       string // name in logs only
	PortBase    int    // NEX block base from ports.md: auth listens on +0, secure on +1
	AccessKey   string // the title's NEX access key
	Version     string // NEX library version the title was built against, e.g. "4.4.0"
	SecureHost  string // address the console is told to reach the secure server on
	HTTPAddr    string // /healthz listener
	InternalURL string // nx-baas game/internal API
	InternalKey string // nx-baas NX_INTERNAL_KEY, sent as X-Internal-Key
}

// FromEnv reads the configuration from the environment.
func FromEnv() Config {
	base, _ := strconv.Atoi(os.Getenv("NEX_PORT_BASE"))
	return Config{
		Title:       env("NEX_TITLE", "nex"),
		PortBase:    base,
		AccessKey:   os.Getenv("NEX_ACCESS_KEY"),
		Version:     env("NEX_VERSION", "4.4.0"),
		SecureHost:  env("NEX_SECURE_HOST", "127.0.0.1"),
		HTTPAddr:    env("NEX_HTTP_ADDR", fmt.Sprintf(":%d", base+2)),
		InternalURL: env("NX_INTERNAL_URL", "http://127.0.0.1:20070"),
		InternalKey: os.Getenv("NX_INTERNAL_KEY"),
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// AuthPort and SecurePort are the two listeners of the title's block (ports.md: +0 auth, +1 secure).
func (c Config) AuthPort() int   { return c.PortBase }
func (c Config) SecurePort() int { return c.PortBase + 1 }

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
	}
	if _, err := parseVersion(c.Version); err != nil {
		return err
	}
	return nil
}

func parseVersion(v string) (*nex.LibraryVersion, error) {
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

	authServer   *nex.PRUDPServer
	secureServer *nex.PRUDPServer

	// The two special accounts. Their passwords are the kerberos keys for tickets this
	// process issues and reads back, so they are generated per process: nothing outside
	// this process ever derives a key from them, and a restart invalidates only tickets
	// that nex-go would already have rejected as older than two minutes.
	authAccount   *nex.Account
	secureAccount *nex.Account
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
		secureAccount: nex.NewAccount(types.NewPID(2), "Quazal Rendez-Vous", randomPassword(), false),
	}

	h.authServer, h.Auth = h.newServer(version, 1)
	h.secureServer, h.Secure = h.newServer(version, 1)
	h.Auth.ServerAccount = h.authAccount
	h.Secure.ServerAccount = h.secureAccount
	h.Secure.IsSecureEndPoint = true

	h.registerTicketGranting()
	h.registerSecureConnection()
	return h, nil
}

func (h *Host) newServer(version *nex.LibraryVersion, streamID uint8) (*nex.PRUDPServer, *nex.PRUDPEndPoint) {
	server := nex.NewPRUDPServer()
	server.AccessKey = h.Config.AccessKey
	server.LibraryVersions.SetDefault(version)
	// Switch tickets use the newer internal-ticket format: a random key in the clear next
	// to the encrypted body, final key MD5(target key + random key). Kinnay's kerberos page.
	server.KerberosTicketVersion = 1
	server.SessionKeyLength = 32

	endpoint := nex.NewPRUDPEndPoint(streamID)
	endpoint.AccountDetailsByPID = h.accountByPID
	endpoint.AccountDetailsByUsername = h.accountByUsername
	server.BindPRUDPEndPoint(endpoint)
	return server, endpoint
}

func (h *Host) registerTicketGranting() {
	protocol := ticket_granting.NewProtocol()
	common := common_ticket_granting.NewCommonProtocol(protocol)
	common.SecureServerAccount = h.secureAccount
	common.SessionKeyLength = h.authServer.SessionKeyLength
	common.SecureStationURL = h.secureStationURL()
	common.BuildName = types.NewString("OpenPak " + h.Config.Title)

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
	// there is nothing left for a second token check to prove.
	// ponytail: accept it; tighten if a capture ever shows a title sending something here
	// that we must act on.
	common.ValidateLoginData = func(types.PID, types.DataHolder) *nex.Error { return nil }
	h.Secure.RegisterServiceProtocol(protocol)
}

func (h *Host) secureStationURL() types.StationURL {
	url := types.NewStationURL("")
	url.SetURLType(constants.StationURLPRUDPS)
	url.SetAddress(h.Config.SecureHost)
	url.SetPortNumber(uint16(h.Config.SecurePort()))
	url.SetConnectionID(1)
	url.SetPrincipalID(h.secureAccount.PID)
	url.SetStreamID(1)
	url.SetStreamType(constants.StreamTypeRVSecure)
	url.SetType(2)
	return url
}

// Run serves /healthz and both PRUDP listeners. It blocks.
func (h *Host) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "ok %s auth=%d secure=%d\n", h.Config.Title, h.Config.AuthPort(), h.Config.SecurePort())
	})
	srv := &http.Server{Addr: h.Config.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	log.Printf("[%s] auth :%d secure :%d healthz %s nex %s",
		h.Config.Title, h.Config.AuthPort(), h.Config.SecurePort(), h.Config.HTTPAddr, h.Config.Version)

	go h.authServer.Listen(h.Config.AuthPort())
	go h.secureServer.Listen(h.Config.SecurePort())
	return srv.ListenAndServe()
}
