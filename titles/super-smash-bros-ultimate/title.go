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
//	  NEX unique IDs. SSBU calls these during online bring-up; answering them with an
//	  empty list is not enough. Real here.
//	DataStore (0x73)
//	  SSBU's online init reads DataStore; an empty list with GetMeta forced to NotFound
//	  is not enough. Real here — see datastore.go.
//
// SecureConnection (0x0B) and TicketGranting (0x0A) are registered by openpak/nex. Ranking
// (0x70) is deliberately not registered; see docs/provenance.md.
package ssbu

import (
	"database/sql"
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

// Register wires the title's protocols onto the shared host.
func Register(host *nexhost.Host) error {
	database, err := openDatabase()
	if err != nil {
		return err
	}

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
		return err
	}
	return nil
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

// Title is this game"s entry in the shared NEX host.
var Title = nexhost.Title{Name: "super-smash-bros-ultimate", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
	c.Version = "4.0.0"
}
