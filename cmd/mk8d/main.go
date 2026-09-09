// Command mk8d runs the Mario Kart 8 Deluxe NEX server on the OpenPak shared NEX host.
//
// MK8D's online mode is peer-to-peer once a race starts. The server's whole job is
//
//	ticket-granting -> secure-connection -> matchmaking -> NAT traversal -> P2P
//
// plus Utility (the console asks for a NEX unique ID and its settings right after it
// registers) and Ranking (time-trial scores, and the "common data" blob other players read
// for a racer's name and Mii). Every one of those protocols is already implemented by
// Pretendo's AGPL-3.0 nex-protocols-common-go and the login by openpak/nex, so this file is
// wiring: which protocols MK8D gets, the per-title defaults, where the state lives, and the
// one thing neither library does for us -- the Switch byte-stream profile.
//
// What MK8D asks for that is NOT here: its own matchmake-extension methods above 0x36
// (tournaments, room codes). See docs/design.md.
package main

import (
	"database/sql"
	"log"
	"os"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_match_making "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_match_making_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_ranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
	_ "github.com/lib/pq"
	nexhost "openpak/nex"
)

func main() {
	host, err := nexhost.New(config())
	if err != nil {
		log.Fatal(err)
	}
	switchByteStream(host)

	db, err := sql.Open("postgres", os.Getenv("MK8D_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("MK8D_DATABASE_URL: %v: matchmaking has nowhere to keep its lobbies", err)
	}

	if err := registerProtocols(host, db); err != nil {
		log.Fatal(err)
	}
	log.Fatal(host.Run())
}

// config is the host configuration with MK8D's measured values as defaults. Every one is
// still overridable from the environment, because every one of them is a claim a capture
// could overturn -- see docs/provenance.md.
func config() nexhost.Config {
	cfg := nexhost.FromEnv()
	if os.Getenv("NEX_TITLE") == "" {
		cfg.Title = "mario-kart-8-deluxe"
	}
	if os.Getenv("NEX_VERSION") == "" {
		// MK8D reports NEX 4.0.0, not the host's 4.4.0 default. It decides the structure
		// encoding and which ticket-granting method the console calls: 4.0.0 means method 2,
		// ValidateAndRequestTicketWithCustomData, not method 6.
		cfg.Version = "4.0.0"
	}
	if os.Getenv("NEX_TRANSPORT") == "" {
		// A Switch NEX title speaks PRUDPLite over a WebSocket, never UDP. TLS is the front
		// proxy's job, so the listeners themselves are plain ws.
		cfg.Transport = "websocket"
	}
	if os.Getenv("NEX_STATION_SCHEME") == "" {
		// MK8D's secure station is advertised as prudp, not prudps.
		cfg.StationScheme = "prudp"
	}
	return cfg
}

// switchByteStream applies the two wire settings a NEX 4 Switch title needs and that nex-go
// defaults the other way for Wii U and 3DS: 8-byte PIDs, and a version+length header in
// front of every structure. Get either wrong and every field after the first is read at the
// wrong offset.
//
// ponytail: set here because openpak/nex builds the servers itself and has no Switch
// profile yet. Delete it once servers/nex does this -- it is per-console, not per-title.
func switchByteStream(host *nexhost.Host) {
	for _, endpoint := range []*nex.PRUDPEndPoint{host.Auth, host.Secure} {
		endpoint.Server.ByteStreamSettings.PIDSize = 8
		endpoint.Server.ByteStreamSettings.UseStructureHeader = true
	}
}

// registerProtocols gives the secure endpoint everything MK8D's online mode touches.
// openpak/nex has already registered ticket-granting on the authentication endpoint and
// secure-connection on this one.
//
// Order matters: match-making's SetManager creates the matchmaking schema and the
// gatherings table, and matchmake-extension's SetManager adds its tables to that schema.
func registerProtocols(host *nexhost.Host, db *sql.DB) error {
	matchmaking := common_globals.NewMatchmakingManager(host.Secure, db)
	// OpenPak's friend graph lives in nx-baas, so MK8D keeps no friend table.
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
	utilityManager := common_globals.NewUtilityManager(host.Secure, db)
	// MK8D calls GetIntegerSettings as soon as it has registered. An empty map is a real
	// answer; a missing callback is Core::NotImplemented, which stops the boot. There is no
	// MK8D setting whose meaning we know.
	// ponytail: empty maps. Fill one in when a capture shows the console acting on a value.
	utilityManager.GetIntegerSettings = func(*common_globals.UtilityManager, types.PID, types.UInt32) (types.Map[types.UInt16, types.UInt32], *nex.Error) {
		return types.NewMap[types.UInt16, types.UInt32](), nil
	}
	utilityManager.GetStringSettings = func(*common_globals.UtilityManager, types.PID, types.UInt32) (types.Map[types.UInt16, types.String], *nex.Error) {
		return types.NewMap[types.UInt16, types.String](), nil
	}
	common_utility.NewCommonProtocol(utilityProtocol).SetManager(utilityManager)
	host.Secure.RegisterServiceProtocol(utilityProtocol)

	// Ranking (0x70) carries MK8D's time-trial scores and the per-player "common data"
	// blob, so unlike a pure P2P title it needs a backend. Only the three methods MK8D was
	// observed calling are backed; the leaderboard reads stay unset, and the common handlers
	// answer Core::NotImplemented for them rather than crashing.
	rankingStore, err := newRankingStore(db)
	if err != nil {
		return err
	}
	rankingProtocol := ranking.NewProtocol()
	rankingCommon := common_ranking.NewCommonProtocol(rankingProtocol)
	rankingCommon.InsertRankingByPIDAndRankingScoreData = rankingStore.insertScore
	rankingCommon.UploadCommonData = rankingStore.uploadCommonData
	rankingCommon.GetCommonData = rankingStore.getCommonData
	host.Secure.RegisterServiceProtocol(rankingProtocol)

	return nil
}
