// Package f12011 is F1 2011 (3DS) on the shared NEX host, adapted from Pretendo's
// f1-2011 server (AGPL-3.0; history merged here). The server code is upstream's:
// it builds its own authentication and secure PRUDP servers over UDP and logs consoles in
// through the account server (nn-account) over gRPC, so it is a Run-shaped title.
//
// Run translates the host Config into the PN_* environment upstream reads, then calls
// upstream's former init() and main(). Variables outside the Config (AES keys, S3, local
// auth) pass through the environment untouched; see README.md.
package f12011

import (
	"os"
	"strconv"
	"strings"

	nexhost "github.com/openpak/nex"
)

// Title is this game's entry in the shared NEX host.
var Title = nexhost.Title{Name: "f1-2011", Defaults: defaults, Run: Run}

func defaults(c *nexhost.Config) { c.Transport = "udp" }

// Run maps the Config onto upstream's environment, then starts upstream's servers.
func Run(cfg nexhost.Config) error {
	setenv("PN_F1_ACCOUNT_GRPC_API_KEY", cfg.AccountGRPCKey)
	setenv("PN_F1_ACCOUNT_GRPC_HOST", hostOf(cfg.AccountGRPCAddr))
	setenv("PN_F1_ACCOUNT_GRPC_PORT", portOf(cfg.AccountGRPCAddr))
	setenv("PN_F1_AUTHENTICATION_SERVER_PORT", strconv.Itoa(cfg.AuthPort()))
	setenv("PN_F1_FRIENDS_GRPC_API_KEY", cfg.FriendsGRPCKey)
	setenv("PN_F1_FRIENDS_GRPC_HOST", hostOf(cfg.FriendsGRPCAddr))
	setenv("PN_F1_FRIENDS_GRPC_PORT", portOf(cfg.FriendsGRPCAddr))
	setenv("PN_F1_POSTGRES_URI", cfg.DatabaseURL)
	setenv("PN_F1_SECURE_SERVER_HOST", cfg.SecureHost)
	setenv("PN_F1_SECURE_SERVER_PORT", strconv.Itoa(cfg.SecurePort()))
	initialize()
	run()
	return nil
}

func setenv(name, value string) {
	if value != "" && os.Getenv(name) == "" {
		_ = os.Setenv(name, value)
	}
}

func hostOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i]
	}
	return addr
}

func portOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

var _ = strconv.Itoa
