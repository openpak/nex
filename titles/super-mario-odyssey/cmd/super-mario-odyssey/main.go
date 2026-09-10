// Command super-mario-odyssey is the OpenPak game server for Super Mario Odyssey.
//
// Odyssey is the odd one out of the NEX fleet: it has no matchmaking, no lobby and no
// peer-to-peer play at all. Its entire online surface is Balloon World — a player hides a
// balloon at an in-game spot (a screenshot plus costume snapshot) and other players search
// for and pop it — and that is DataStore (protocol 0x73) end to end. So this binary registers
// no MatchMaking/MatchmakeExtension/NATTraversal, and the one protocol the whole title lives
// on has to be a real implementation rather than a stub.
//
// openpak/nex owns everything up to and including login: PRUDP over WebSocket, kerberos, the
// Switch ticket-granting methods, SecureConnection, the nx-baas account lookup, the port
// block and /healthz. Pretendo's nex-protocols-common-go owns the DataStore and Utility
// handlers. What is written here is only:
//
//	Utility   (0x6E)  NEX unique IDs, which the DataStore post flow acquires before it posts.
//	DataStore (0x73)  Balloon World: place, find, rate, delete. Real — see datastore.go.
//
// Not registered, and why, in docs/design.md: Ranking (0x70), and Odyssey's own Balloon
// extension methods 47-53 (SearchBalloon, FetchMyInfos, the pose-buffer queues), which are a
// title-specific protocol that no common library implements and that a capture must define.
package main

import (
	"database/sql"
	"log"
	"os"

	nexhost "github.com/openpak/nex"

	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"

	_ "github.com/lib/pq"
)

func main() {
	config := nexhost.FromEnv()
	config.Title = "super-mario-odyssey"

	host, err := nexhost.New(config)
	if err != nil {
		log.Fatal(err)
	}

	database, err := openDatabase()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	// Utility (0x6E): AcquireNexUniqueID, which the console calls to name a new balloon object
	// before it posts it. The manager keeps its counter in Postgres and creates its own schema.
	utilityProtocol := utility.NewProtocol()
	host.Secure.RegisterServiceProtocol(utilityProtocol)
	common_utility.NewCommonProtocol(utilityProtocol).SetManager(common_globals.NewUtilityManager(host.Secure, database))

	if err := registerDataStore(host, database, host.FriendPIDs); err != nil {
		log.Fatal(err)
	}

	log.Fatal(host.Run())
}

// openDatabase is the Postgres the DataStore metadata and the Utility counter live in. The
// common handlers create their own schemas on first run; datastore.go creates odyssey.
func openDatabase() (*sql.DB, error) {
	url := os.Getenv("SMO_DATABASE_URL")
	if url == "" {
		return nil, errRequired("SMO_DATABASE_URL")
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
