// Command super-smash-bros-ultimate is the OpenPak game server for Super Smash Bros. Ultimate.
//
// SSBU is a NEX title. openpak/nex owns everything up to and including login — PRUDP over
// WebSocket, kerberos, the Switch ticket-granting methods and the nx-baas account lookup — so
// this binary is only the protocol set SSBU itself speaks on the secure endpoint:
//
//	MatchMaking (0x15), MatchMakingExt (0x32), MatchmakeExtension (0x6D)
//	  online play: quickplay, background matchmaking, and Battle Arenas.
//	NATTraversal (0x03)
//	  two consoles telling each other how to be reached; play is peer to peer once matched.
//	Utility (0x6E)
//	  NEX unique IDs. SSBU calls these during online bring-up; the ExternalReference server
//	  answered them with an empty list, so they were never real there. Real here.
//	DataStore (0x73)
//	  SSBU's online init reads DataStore. The ExternalReference server had no DataStore at all
//	  (earlier-nex never implemented it) and answered an empty list, with GetMeta forced to
//	  NotFound. Real here — see datastore.go.
//
// SecureConnection (0x0B) and TicketGranting (0x0A) are registered by openpak/nex. Ranking
// (0x70) is deliberately not registered; see docs/provenance.md.
package main

import (
	"database/sql"
	"log"
	"os"

	nexhost "github.com/openpak/nex"

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
)

func main() {
	config := nexhost.FromEnv()
	config.Title = "super-smash-bros-ultimate"
	if os.Getenv("NEX_VERSION") == "" {
		// Unverified. The ExternalReference SSBU server sets NEX version 40000 (4.0.0), but the
		// file that carries it is a copied Mario Kart 8 Deluxe template (its own doc comment
		// still names mk8), so 4.0.0 is a starting guess, not an SSBU fact. A capture settles
		// it: the console reports apiVersionGeneral. See docs/provenance.md and docs/design.md.
		config.Version = "4.0.0"
	}

	host, err := nexhost.New(config)
	if err != nil {
		log.Fatal(err)
	}

	database, err := openDatabase()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	matchmaking := common_globals.NewMatchmakingManager(host.Secure, database)
	matchmaking.GetUserFriendPIDs = host.FriendPIDs

	// Every protocol has to be registered on the endpoint before its common protocol is built:
	// RegisterServiceProtocol is what sets protocol.Endpoint(), and the common constructors
	// read it immediately.
	matchMaking := match_making.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchMaking)
	common_match_making.NewCommonProtocol(matchMaking).SetManager(matchmaking)

	matchMakingExt := match_making_ext.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchMakingExt)
	common_match_making_ext.NewCommonProtocol(matchMakingExt).SetManager(matchmaking)

	matchmakeExtension := matchmake_extension.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchmakeExtension)
	common_matchmake_extension.NewCommonProtocol(matchmakeExtension).SetManager(matchmaking)

	natTraversal := nat_traversal.NewProtocol()
	host.Secure.RegisterServiceProtocol(natTraversal)
	common_nat_traversal.NewCommonProtocol(natTraversal)

	// Utility: the NEX-unique-ID getters SSBU asks for during online bring-up. The common
	// manager keeps them in Postgres; the ExternalReference stub answered every one with an
	// empty list, so this is one of the two protocols made real here.
	utilityProtocol := utility.NewProtocol()
	host.Secure.RegisterServiceProtocol(utilityProtocol)
	common_utility.NewCommonProtocol(utilityProtocol).SetManager(common_globals.NewUtilityManager(host.Secure, database))

	if err := registerDataStore(host, database, host.FriendPIDs); err != nil {
		log.Fatal(err)
	}

	log.Fatal(host.Run())
}

// openDatabase is the Postgres nex-protocols-common-go's matchmaking, utility and DataStore
// handlers all keep their state in. They create their own schemas on first run.
func openDatabase() (*sql.DB, error) {
	url := os.Getenv("SSBU_DATABASE_URL")
	if url == "" {
		return nil, errRequired("SSBU_DATABASE_URL")
	}
	database, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

type errRequired string

func (e errRequired) Error() string { return string(e) + " is required" }
