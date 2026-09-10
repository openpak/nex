// Package mps is the OpenPak NEX game server for Mario Party Superstars
// (title 01006FE013472000, NEX 4.6.5, Pia 5.33.0).
//
// Everything shared with other NEX titles — PRUDP over WebSocket, kerberos, the Switch login
// methods and OpenPak identity — comes from github.com/openpak/nex. Everything shared with
// NEX servers of any title — match-making, matchmake-extension, NAT traversal and utility —
// comes from Pretendo's nex-protocols-common-go. What is left, and all this package is, is
// the two things Mario Party Superstars needs that neither of those provides:
//
//  1. FindMatchmakeSessionByParticipant and FindMatchmakeSessionByGatheringIDDetail, the two
//     methods the game's "Join Room" screen uses to follow a friend into a lobby. See
//     matchmaking.go.
//  2. Real Utility integer settings, because the game asks for them the moment it connects.
//     See utility.go.
package mps

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	nexhost "github.com/openpak/nex"

	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_matchmaking "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_matchmaking_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmaking_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"

	_ "github.com/lib/pq"
)

// Facts about the game rather than deployment choices, all recorded with their sources in
// docs/provenance.md. The access key is configuration and has no default here.
const (
	// TitleID is Mario Party Superstars on Switch.
	TitleID = "01006FE013472000"
	// NEXVersion is the NEX library the game was built against (apiVersionGeneral 40605).
	NEXVersion = "4.6.5"
	// StationScheme is per title, and this one is measured: the game hands over its
	// kerberos ticket in CONNECT under prudps, and sends an empty CONNECT payload under
	// prudp.
	StationScheme = "prudps"
	// SecureStreamID 1 puts the secure endpoint on its own listener at PortBase+1. This
	// title is one of the two-listener ones.
	SecureStreamID = 1
	// SourceKeyBytes is the length of the kerberos source key handed to the console. Per
	// title, and the wrong value fails silently -- the login just does not work, with
	// nothing logged on either side. 32 for this title, measured: the working server for it
	// derives the source key with SHA-256 and hands over all 32 bytes.
	SourceKeyBytes = 32
	// PortBase is this title's NEX block from ports.md.
	PortBase = 22110
)

// Register wires the title's protocols onto the shared host.
func Register(host *nexhost.Host) error {
	url := os.Getenv("MPS_DATABASE_URL")
	if url == "" {
		return errors.New("MPS_DATABASE_URL is required: matchmaking and NEX unique IDs both keep their state in Postgres")
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("MPS_DATABASE_URL: %w", err)
	}

	registerMatchmaking(host, db)
	registerUtility(host, db)

	// DataStore (0x73) is deliberately not registered. See docs/design.md: nothing has ever
	// observed this title make a DataStore call, and nex-go answers an unregistered
	// protocol with Core::NotImplemented rather than dropping it, which is what a stub
	// would say anyway.

	natProtocol := nat_traversal.NewProtocol()
	host.Secure.RegisterServiceProtocol(natProtocol)
	common_nat_traversal.NewCommonProtocol(natProtocol)

	return nil
}

// registerMatchmaking wires the three matchmaking protocols to one manager, then adds the
// two methods the common library does not implement but this title needs (matchmaking.go).
func registerMatchmaking(host *nexhost.Host, db *sql.DB) {
	manager := common_globals.NewMatchmakingManager(host.Secure, db)
	// Friend-only lobbies. Upstream documents this callback as not working on Switch
	// because it has no Switch friend graph; OpenPak has one, in nx-baas.
	manager.GetUserFriendPIDs = host.FriendPIDs

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

func registerUtility(host *nexhost.Host, db *sql.DB) {
	protocol := utility.NewProtocol()
	host.Secure.RegisterServiceProtocol(protocol)
	common := common_utility.NewCommonProtocol(protocol)

	manager := common_globals.NewUtilityManager(host.Secure, db)
	manager.GetIntegerSettings = integerSettings
	manager.GetStringSettings = stringSettings
	common.SetManager(manager)
}

// Title is this game's entry in the shared NEX host.
var Title = nexhost.Title{Name: "mario-party-superstars", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
	c.PortBase = PortBase
	c.Version = NEXVersion
	c.StationScheme = StationScheme
	c.SecureStreamID = SecureStreamID
	c.SourceKeyBytes = SourceKeyBytes
}
