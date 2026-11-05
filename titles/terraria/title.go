// Package terraria is Terraria (Wii U) on the shared NEX host, adapted from Pretendo's
// terraria server (AGPL-3.0; history merged here). The server code is upstream's:
// it builds its own authentication and secure PRUDP servers over UDP and logs consoles in
// through the account server (nn-account) over gRPC, so it is a Run-shaped title.
//
// Run translates the host Config into the PN_* environment upstream reads, then calls
// upstream's former init() and main(). Variables outside the Config pass through the
// environment untouched; see README.md.
package terraria

import (
	"context"
	"log"
	"os"
	"time"
	"strconv"
	"strings"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles/terraria/globals"
)

// Title is this game's entry in the shared NEX host.
var Title = nexhost.Title{Name: "terraria", Defaults: defaults, Run: Run}

func defaults(c *nexhost.Config) { c.Transport = "udp" }

// Run maps the Config onto upstream's environment, then starts upstream's servers.
func Run(cfg nexhost.Config) error {
	// Rewards: count online sessions (two players sharing a gathering for 3 minutes).
	nexhost.WatchSessions(cfg, "game_sessions.terraria", "wiiu")
	setenv("PN_TERRARIA_ACCOUNT_GRPC_API_KEY", cfg.AccountGRPCKey)
	setenv("PN_TERRARIA_ACCOUNT_GRPC_HOST", hostOf(cfg.AccountGRPCAddr))
	setenv("PN_TERRARIA_ACCOUNT_GRPC_PORT", portOf(cfg.AccountGRPCAddr))
	setenv("PN_TERRARIA_AUTHENTICATION_SERVER_PORT", strconv.Itoa(cfg.AuthPort()))
	setenv("PN_TERRARIA_FRIENDS_GRPC_API_KEY", cfg.FriendsGRPCKey)
	setenv("PN_TERRARIA_FRIENDS_GRPC_HOST", hostOf(cfg.FriendsGRPCAddr))
	setenv("PN_TERRARIA_FRIENDS_GRPC_PORT", portOf(cfg.FriendsGRPCAddr))
	setenv("PN_TERRARIA_POSTGRES_URI", cfg.DatabaseURL)
	setenv("PN_TERRARIA_SECURE_SERVER_HOST", cfg.SecureHost)
	setenv("PN_TERRARIA_SECURE_SERVER_PORT", strconv.Itoa(cfg.SecurePort()))
	initialize()
	startCoreMirror(cfg)
	nexhost.ServeRankings(nexhost.ScoresRankings(globals.Postgres, nil))
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

// startCoreMirror copies the title's gatherings into the shared matchmaking core under the neutral
// title key "terraria", with the compatibility group assigned by the server. Off unless
// TERRARIA_CORE_MIRROR=1; rollback is unsetting it (the NEX layer never depended on it). A failed
// start is logged and the title keeps serving from its own tables.
func startCoreMirror(cfg nexhost.Config) {
	if os.Getenv("TERRARIA_CORE_MIRROR") != "1" {
		return
	}
	group := os.Getenv("TERRARIA_COMPAT_GROUP")
	if group == "" {
		group = "old-gen-1.2.4"
	}
	m, err := nexhost.StartCoreMirror(cfg, globals.Postgres, nexhost.MirrorSpec{Title: "terraria", Network: "wiiu", Compat: group, Console: "wiiu"})
	if err != nil {
		log.Printf("[terraria] core mirror not started: %v", err)
		return
	}
	// TERRARIA_CORE_SEARCH=1 (needs the mirror): the console's search sees only the rooms the core lists
	// for this group. A core error leaves the search unrestricted. Rollback: unset it.
	if os.Getenv("TERRARIA_CORE_SEARCH") == "1" {
		globals.CoreCandidates = func() []uint32 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return m.Candidates(ctx)
		}
		log.Printf("[terraria] core search on: rooms come from the matchmaking core, group %s", group)
	}
}
