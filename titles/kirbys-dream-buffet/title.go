// Package kdb is Kirby's Dream Buffet (0100A8E016236000) on the shared NEX host.
//
// Scaffold: the common NEX set a Switch title with online rooms needs (MatchMaking,
// MatchMakingExt, MatchmakeExtension, NATTraversal, Utility) over Postgres, and nothing this
// title has been observed to do. Four-player online races and rankings (2022). The two facts a console needs are outside this
// package and not public yet: the NEX access key and the game-server id (the g<id> host).
// Neither is in Kinnay's Game Server List nor in any published server; both come from the
// game binary or a capture. Until then NEX_ACCESS_KEY has no value to hold and the title is
// not deployable. See README.md.
package kdb

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
	return nil
}

// openDatabase is the Postgres nex-protocols-common-go's matchmaking and utility handlers
// keep their state in. They create their own schemas on first run.
func openDatabase() (*sql.DB, error) {
	url := os.Getenv("KDB_DATABASE_URL")
	if url == "" {
		return nil, errRequired("KDB_DATABASE_URL")
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
var Title = nexhost.Title{Name: "kirbys-dream-buffet", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
}
