// Package mps is the OpenPak NEX game server for Mario Party Superstars
// (title 01006FE013472000, NEX 4.6.5, Pia 5.33.0).
//
// Everything shared with other NEX titles — PRUDP, kerberos, the Switch login methods and
// OpenPak identity — comes from openpak/nex. Everything shared with other NEX servers of any
// title — match-making, matchmake-extension, NAT traversal, utility and DataStore — comes
// from Pretendo's nex-protocols-common-go. What is left, and all this package is, is the
// three things Mario Party Superstars needs that neither of those provides:
//
//  1. FindMatchmakeSessionByParticipant and FindMatchmakeSessionByGatheringIDDetail, the two
//     methods the game's "Join Room" screen uses to follow a friend into a lobby. See
//     matchmaking.go.
//  2. Real Utility integer settings, because an empty settings map drops the connection. See
//     utility.go.
//  3. A DataStore object store, so DataStore is answered for real rather than stubbed. See
//     datastore.go.
package mps

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	nexhost "openpak/nex"

	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_matchmaking "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_matchmaking_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmaking_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"

	_ "github.com/lib/pq"
)

// Defaults that are facts about the game rather than deployment choices. Both are recorded
// in docs/provenance.md; the access key is configuration and has no default here.
const (
	// TitleID is Mario Party Superstars on Switch.
	TitleID = "01006FE013472000"
	// NEXVersion is the NEX library the game was built against (apiVersionGeneral 40605).
	NEXVersion = "4.6.5"
	// PortBase is this title's NEX block from ports.md.
	PortBase = 22110
)

// Config is the whole configuration of the server: the shared host's, plus the two backing
// stores matchmaking and DataStore need.
type Config struct {
	Host nexhost.Config

	// DatabaseURL is the Postgres the common matchmaking, utility and DataStore protocols
	// keep their state in. Required: without it the game has no lobbies.
	DatabaseURL string

	// S3 is the object store DataStore hands out presigned upload and download URLs for.
	// Optional — see datastore.go for what stops working without it.
	S3 S3Config
}

// FromEnv reads the configuration from the environment, defaulting everything this title
// already knows about itself.
func FromEnv() Config {
	host := nexhost.FromEnv()
	if host.Title == "nex" {
		host.Title = "mario-party-superstars"
	}
	if host.PortBase == 0 {
		host.PortBase = PortBase
		host.HTTPAddr = env("NEX_HTTP_ADDR", fmt.Sprintf(":%d", PortBase+2))
	}
	if os.Getenv("NEX_VERSION") == "" {
		host.Version = NEXVersion
	}
	return Config{
		Host:        host,
		DatabaseURL: os.Getenv("MPS_DATABASE_URL"),
		S3:          s3FromEnv(),
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// Run starts the server and blocks.
func Run(cfg Config) error {
	if cfg.DatabaseURL == "" {
		return errors.New("MPS_DATABASE_URL is required: matchmaking, utility and DataStore all keep their state in Postgres")
	}

	host, err := nexhost.New(cfg.Host)
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("MPS_DATABASE_URL: %w", err)
	}

	registerMatchmaking(host, db)
	registerUtility(host, db)
	registerDataStore(host, db, cfg.S3)

	natProtocol := nat_traversal.NewProtocol()
	host.Secure.RegisterServiceProtocol(natProtocol)
	common_nat_traversal.NewCommonProtocol(natProtocol)

	return host.Run()
}

// registerMatchmaking wires the three matchmaking protocols to one manager, then adds the
// two methods the common library does not implement but this title needs (matchmaking.go).
func registerMatchmaking(host *nexhost.Host, db *sql.DB) {
	manager := common_globals.NewMatchmakingManager(host.Secure, db)
	manager.GetUserFriendPIDs = friendPIDs(host)

	// Register before wrapping, both times: a common protocol reads the endpoint off the
	// protocol it is given, and a protocol has no endpoint until it is registered.
	// match-making goes first because its SetManager creates the matchmaking schema the
	// other two then create their tables in.
	matchmakingProtocol := matchmaking.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchmakingProtocol)
	common_matchmaking.NewCommonProtocol(matchmakingProtocol).SetManager(manager)

	extensionProtocol := matchmake_extension.NewProtocol()
	host.Secure.RegisterServiceProtocol(extensionProtocol)
	common_matchmake_extension.NewCommonProtocol(extensionProtocol).SetManager(manager)
	registerFriendJoin(extensionProtocol, manager)

	extProtocol := matchmaking_ext.NewProtocol()
	host.Secure.RegisterServiceProtocol(extProtocol)
	common_matchmaking_ext.NewCommonProtocol(extProtocol).SetManager(manager)
}

// friendPIDs answers the common protocols' "who are this player's friends" question out of
// nx-baas, which is where an OpenPak player's friend graph already lives. Friend-only
// lobbies and DataStore's friends-only permission both read it.
//
// ponytail: no cache. One internal HTTP call on a LAN per question; add a short-TTL cache
// only if it ever shows up in nx-baas latency.
func friendPIDs(host *nexhost.Host) func(uint32) []uint32 {
	return func(pid uint32) []uint32 {
		player, err := host.PlayerByPID(uint64(pid))
		if err != nil {
			common_globals.Logger.Warningf("friend lookup for pid %d: %v", pid, err)
			return nil
		}
		pids := make([]uint32, 0, len(player.Friends))
		for _, friend := range player.Friends {
			pids = append(pids, uint32(friend.PID))
		}
		return pids
	}
}

func registerUtility(host *nexhost.Host, db *sql.DB) {
	protocol := utility.NewProtocol()
	host.Secure.RegisterServiceProtocol(protocol)
	common := common_utility.NewCommonProtocol(protocol)

	manager := common_globals.NewUtilityManager(host.Secure, db)
	manager.GetIntegerSettings = integerSettings
	manager.GetStringSettings = stringSettings
	common.SetManager(manager)
}

func registerDataStore(host *nexhost.Host, db *sql.DB, s3 S3Config) {
	protocol := datastore.NewProtocol()
	host.Secure.RegisterServiceProtocol(protocol)
	common := common_datastore.NewCommonProtocol(protocol)
	common.GetUserFriendPIDs = friendPIDs(host)

	store := newStore(db, host.Secure)
	store.wire(common)
	if err := store.migrate(); err != nil {
		common_globals.Logger.Errorf("DataStore schema: %v", err)
	}
	if err := s3.attach(common); err != nil {
		common_globals.Logger.Warningf("DataStore object storage disabled: %v", err)
	}
}
