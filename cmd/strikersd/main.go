// Command strikersd is the OpenPak game server for Mario Strikers: Battle League.
//
// Strikers is a NEX 4 title. openpak/nex owns everything up to and including login — PRUDP,
// kerberos, the Switch ticket-granting methods and the nx-baas account lookup — so this
// binary is only the protocol set Strikers speaks on the secure endpoint:
//
//	MatchMaking (0x15), MatchMakingExt (0x32), MatchmakeExtension (0x6D)
//	  the online lobby lifecycle: quick battle, matchmaking, and the club sessions Strikers
//	  builds on top of gatherings.
//	NATTraversal (0x03)
//	  two consoles telling each other how to be reached.
//	Utility (0x6E)
//	  NEX unique IDs.
//	DataStore (0x73)
//	  registered over Postgres so the game's 0x73 init calls get a real, empty-store answer
//	  instead of Core::NotImplemented. See datastore.go and docs/design.md.
//
// SecureConnection (0x0B) and TicketGranting (0x0A) are registered by openpak/nex.
//
// What is NOT registered here — Ranking (0x70), Subscriber (0x79), Ranking2 (0x7A),
// MatchmakeReferee (0x78) and Strikers' game-specific MatchmakeExtension club methods
// (0x6D method 54+) — is capture-driven next-round work; see docs/provenance.md.
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
	config.Title = "mario-strikers"

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

	// Every protocol has to be registered on the endpoint before its common protocol is
	// built: RegisterServiceProtocol is what sets protocol.Endpoint(), and the common
	// constructors read it immediately.
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
	url := os.Getenv("STRIKERS_DATABASE_URL")
	if url == "" {
		return nil, errRequired("STRIKERS_DATABASE_URL")
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
