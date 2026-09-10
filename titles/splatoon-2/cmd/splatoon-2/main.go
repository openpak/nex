// Command splatoon-2 serves the NEX game server for Splatoon 2.
//
// The PRUDP transport, kerberos handshake, Switch login and OpenPak account
// identity come from openpak/nex; the standard NEX service protocols come from
// Pretendo's nex-protocols-common-go. What is written here is only what neither of
// those can know: which protocols this title speaks, the Postgres/S3 storage behind
// DataStore and Ranking, and the handful of Splatoon 2 methods that live above the
// standard protocols and would otherwise stall the game (see splatfest.go).
//
// Splatoon 2 is a NEX title. Splatoon 3 is NPLN and is a separate server entirely.
package main

import (
	"database/sql"
	"log"
	"os"

	nex "github.com/PretendoNetwork/nex-go/v2"
	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_match_making "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_match_making_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_ranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
	_ "github.com/lib/pq"
	nexhost "github.com/openpak/nex"
)

func main() {
	cfg := nexhost.FromEnv()
	cfg.Title = "splatoon-2"
	if os.Getenv("NEX_VERSION") == "" {
		// The value the OpenPak server runs this title with (40000). Not a capture, but
		// better grounded than an era guess. docs/provenance.md, docs/design.md.
		cfg.Version = "4.0.0"
	}

	host, err := nexhost.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("DATABASE_URL: ", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatal("database unreachable: ", err)
	}
	if err := createTables(db); err != nil {
		log.Fatal("schema: ", err)
	}

	registerProtocols(host, db)
	log.Fatal(host.Run())
}

// registerProtocols wires every protocol this title speaks onto the secure endpoint.
// The set is the one the OpenPak server registers, which is what the console was
// observed to use; see docs/provenance.md.
func registerProtocols(host *nexhost.Host, db *sql.DB) {
	matchmaking := common_globals.NewMatchmakingManager(host.Secure, db)
	matchmaking.GetUserFriendPIDs = host.FriendPIDs

	// Order matters: nex-protocols-common-go reads the endpoint off the protocol when
	// its common half is built, and a protocol has no endpoint until it is registered.
	matchMaking := match_making.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchMaking)
	common_match_making.NewCommonProtocol(matchMaking).SetManager(matchmaking)

	extension := matchmake_extension.NewProtocol()
	host.Secure.RegisterServiceProtocol(extension)
	common_matchmake_extension.NewCommonProtocol(extension).SetManager(matchmaking)
	patchMatchmakeExtension(extension)

	matchMakingExt := match_making_ext.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchMakingExt)
	common_match_making_ext.NewCommonProtocol(matchMakingExt).SetManager(matchmaking)

	natTraversal := nat_traversal.NewProtocol()
	host.Secure.RegisterServiceProtocol(natTraversal)
	common_nat_traversal.NewCommonProtocol(natTraversal)

	utilityProtocol := utility.NewProtocol()
	host.Secure.RegisterServiceProtocol(utilityProtocol)
	common_utility.NewCommonProtocol(utilityProtocol).SetManager(common_globals.NewUtilityManager(host.Secure, db))
	patchUtility(utilityProtocol)

	rankingProtocol := ranking.NewProtocol()
	host.Secure.RegisterServiceProtocol(rankingProtocol)
	rankingStore{db: db, friends: host.FriendPIDs}.wire(common_ranking.NewCommonProtocol(rankingProtocol))
	patchRanking(rankingProtocol)

	dataStoreProtocol := datastore.NewProtocol()
	host.Secure.RegisterServiceProtocol(dataStoreProtocol)
	objectStore{db: db}.wire(common_datastore.NewCommonProtocol(dataStoreProtocol), host.FriendPIDs)
}

// storeError turns a storage failure into the NEX error the caller gets. Every
// storage callback funnels through it so a Postgres outage reads as one code in the
// console's log rather than a different one per protocol.
func storeError(code uint32, err error) *nex.Error {
	return nex.NewError(code, err.Error())
}
