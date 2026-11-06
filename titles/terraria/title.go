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
	"strconv"
	"strings"
	"time"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles/terraria/globals"
	terrarianex "github.com/openpak/nex/titles/terraria/nex"
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
	spec := nexhost.MirrorSpec{Title: "terraria", Network: "wiiu", Compat: group, Console: "wiiu",
		// Cross-play is always on (owner rule, prds#145): PS3 rooms of the same group appear in the Wii U's own search as shadow
		// gatherings. TERRARIA_CROSSPLAY_OFF=1 is an incident kill switch only; the shadows are dropped on the next pass.
		// Moves into common/terraria with the rest of the Terraria policy.
		Cross:  crossNetworks(),
		Shadow: shadowTemplate,
		// A PS3 player joining this Wii U room through the relay: a NEX participant, and the host is told (prds#145).
		OnForeignJoin:  terrarianex.ForeignJoin,
		OnForeignLeave: terrarianex.ForeignLeave,
	}
	m, err := nexhost.StartCoreMirror(cfg, globals.Postgres, spec)
	if err != nil {
		log.Printf("[terraria] core mirror not started: %v", err)
		return
	}
	globals.IsCrossRoom = m.IsShadow
	if globals.RelayStation = os.Getenv("TERRARIA_RELAY_STATION"); globals.RelayStation != "" {
		log.Printf("[terraria] cross-play joins go through the relay at %s", globals.RelayStation)
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

// shadowTemplate is the MatchmakeSession a real Wii U Terraria host creates (observed 2026-11-05, card #145): game mode 1, six
// attributes [0,1,0,0,0,0], system type 2, application buffer 01 02 03, friends-only policy 98, flags 512, progress score 100.
var shadowTemplate = nexhost.ShadowTemplate{GameMode: 1, Attribs: []uint32{0, 1, 0, 0, 0, 0}, SystemType: 2,
	AppBuffer: []byte{1, 2, 3}, Policy: 98, Flags: 512, ProgressScore: 100}

// crossNetworks are the other networks of the old-gen group whose rooms the Wii U lists: always the PS3 (np), unless the
// incident kill switch TERRARIA_CROSSPLAY_OFF=1 is set.
func crossNetworks() []string {
	if os.Getenv("TERRARIA_CROSSPLAY_OFF") == "1" {
		log.Printf("[terraria] cross-play listing OFF (kill switch TERRARIA_CROSSPLAY_OFF)")
		return nil
	}
	return []string{"np"}
}
