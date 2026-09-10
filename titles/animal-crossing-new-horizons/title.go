// Command acnhd is the OpenPak game server for Animal Crossing: New Horizons.
//
// ACNH is a NEX title. openpak/nex owns everything up to and including login — PRUDP,
// kerberos, the Switch ticket-granting methods and the nx-baas account lookup — so this
// binary is only the protocol set ACNH itself speaks on the secure endpoint:
//
//	MatchMaking (0x15), MatchMakingExt (0x32), MatchmakeExtension (0x6D)
//	  gatherings: creating an island session, browsing/joining by Dodo Code (the code word
//	  in the search criteria), and auto-matchmake.
//	NATTraversal (0x03)
//	  two consoles telling each other how to be reached for the island visit.
//	Utility (0x6E)
//	  NEX unique IDs.
//	DataStore (0x73)
//	  real, backed by Postgres + presigned S3 — see datastore.go. earlier-nex has no
//	  DataStore, so the ExternalReference ACNH server ran 0x73 in "discovery" mode (log every
//	  call, answer empty-success); which methods ACNH actually uses is unmapped. See design.md.
//
// SecureConnection (0x0B) and TicketGranting (0x0A) are registered by openpak/nex.
//
// KNOWN GAPS — features Pretendo's common libraries do not cover and that a first live
// capture must settle before they can be built (see docs/design.md and docs/provenance.md):
//
//	MatchmakeExtension::FindMatchmakeSessionByParticipant (0x6D.0x33) — friend island visits.
//	MatchmakeExtension::UpdateMatchmakeSession           (0x6D.0x2C) — writing a Dodo Code
//	  onto a live session. Codeword set at CreateMatchmakeSession time IS stored and searched
//	  by the common library; only the after-the-fact partial update is missing.
//	Utility 0x6E.9 (AcquireTagId), 0x6E.10 (UpdateCurrentUser).
//	Ranking 0x70.25 (GetEventMatchResult).
//	The public P2P station shape: common secure-connection advertises type=0x03
//	  (Public|BehindNAT); ACNH's Pia 5.x wants type=0x0B. This is an openpak/nex change.
//
// These return Core::NotImplemented today rather than a wrong guess.
package acnh

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
	url := os.Getenv("ACNH_DATABASE_URL")
	if url == "" {
		return nil, errRequired("ACNH_DATABASE_URL")
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
var Title = nexhost.Title{Name: "animal-crossing-new-horizons", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
}
