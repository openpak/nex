// Command golf runs the Mario Golf: Super Rush NEX server on the OpenPak shared NEX host.
//
// Golf is a peer-to-peer sports title: the server exists to log a console in, keep the
// lobby list, and get four consoles pointed at each other. Everything after the tee-off is
// Pia between the players. So the whole job here is
//
//	ticket-granting -> secure-connection -> matchmaking -> NAT traversal -> P2P
//
// and every one of those protocols is already implemented by Pretendo's AGPL-3.0
// nex-protocols-common-go. This file is the wiring: which protocols Golf gets, and where
// their state lives.
package main

import (
	"database/sql"
	"log"
	"os"

	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_match_making "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_match_making_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
	_ "github.com/lib/pq"
	nexhost "github.com/openpak/nex"
)

func main() {
	host, err := nexhost.New(config())
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("postgres", os.Getenv("GOLF_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("GOLF_DATABASE_URL: %v: matchmaking has nowhere to keep its lobbies", err)
	}

	registerProtocols(host, db)
	log.Fatal(host.Run())
}

// config is the shared host's environment configuration with Golf's two defaults applied:
// its own name in the logs, and the secure endpoint on stream 2.
//
// Stream 2 is the Switch shape and is not the shared host's default, which is still the Wii
// U and 3DS one. It is also not only a stream number: nex-go cannot bind two endpoints with
// the same stream ID to one server, so giving the secure endpoint its own stream is exactly
// what puts both endpoints on a single listener -- one TCP port at the block's +0, nothing
// at +1. Left at the default, this server would come up looking healthy on two ports and a
// console would never complete a connection.
func config() nexhost.Config {
	cfg := nexhost.FromEnv()
	if os.Getenv("NEX_TITLE") == "" {
		cfg.Title = "mario-golf-super-rush"
	}
	if os.Getenv("NEX_SECURE_STREAM_ID") == "" {
		cfg.SecureStreamID = 2
	}
	return cfg
}

// registerProtocols gives the secure endpoint everything Golf's online mode touches.
// openpak/nex has already registered ticket-granting on the authentication endpoint and
// secure-connection on this one.
//
// Order matters: match-making's SetManager creates the matchmaking schema and the
// gatherings table, and matchmake-extension's SetManager adds its tables to that schema.
func registerProtocols(host *nexhost.Host, db *sql.DB) {
	matchmaking := common_globals.NewMatchmakingManager(host.Secure, db)
	// OpenPak's friend graph comes from nx-baas, so Golf keeps no friend table: friend-only
	// lobbies are answered by the shared host, out of the same identity call every other
	// OpenPak service uses.
	matchmaking.GetUserFriendPIDs = host.FriendPIDs

	matchMakingProtocol := match_making.NewProtocol()
	common_match_making.NewCommonProtocol(matchMakingProtocol).SetManager(matchmaking)
	host.Secure.RegisterServiceProtocol(matchMakingProtocol)

	matchmakeExtensionProtocol := matchmake_extension.NewProtocol()
	common_matchmake_extension.NewCommonProtocol(matchmakeExtensionProtocol).SetManager(matchmaking)
	host.Secure.RegisterServiceProtocol(matchmakeExtensionProtocol)

	matchMakingExtProtocol := match_making_ext.NewProtocol()
	common_match_making_ext.NewCommonProtocol(matchMakingExtProtocol).SetManager(matchmaking)
	host.Secure.RegisterServiceProtocol(matchMakingExtProtocol)

	natTraversalProtocol := nat_traversal.NewProtocol()
	common_nat_traversal.NewCommonProtocol(natTraversalProtocol)
	host.Secure.RegisterServiceProtocol(natTraversalProtocol)

	utilityProtocol := utility.NewProtocol()
	utilityCommon := common_utility.NewCommonProtocol(utilityProtocol)
	utilityCommon.SetManager(common_globals.NewUtilityManager(host.Secure, db))
	host.Secure.RegisterServiceProtocol(utilityProtocol)

	// ponytail: Ranking (0x70) and DataStore (0x73) are deliberately not registered, because
	// nothing has ever observed Golf calling either -- the OpenPak server for this title
	// registers no DataStore at all and a generic Ranking handler that is really Mario Kart
	// 8 Deluxe's. Ceiling: a title that does call them gets "unsupported protocol". Upgrade
	// path: the RMC log names the protocol on the first real session; then Ranking is
	// common_ranking plus five callbacks (and nex-protocols-go v2.3.7, since v2.3.5's
	// ranking.Protocol does not satisfy its own ranking.Interface), and DataStore is
	// common_datastore plus an object store, which belongs in servers/nex where every title
	// that needs it can share it. docs/design.md carries the reasoning.
}
