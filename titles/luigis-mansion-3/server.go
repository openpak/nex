// Package lm3 is the OpenPak NEX game server for Luigi's Mansion 3
// (title 0100DCA0064A6000, NEX 4.0.0, Pia 5.17).
//
// Everything shared with other NEX titles — PRUDP over WebSocket, kerberos, the Switch login
// methods and OpenPak identity — comes from github.com/openpak/nex. Everything shared with NEX
// servers of any title — match-making, matchmake-extension, NAT traversal and utility — comes
// from Pretendo's nex-protocols-common-go. Luigi's Mansion 3 online is co-op matchmaking
// (ScreamPark and ScareScraper), so what this package adds on top is small:
//
//  1. The matchmaking set wired to one manager, with friend-only lobbies going to nx-baas.
//  2. Utility integer/string settings, which the game asks for the moment it connects.
//  3. A minimal acknowledgement for the game's own station/presence protocol (0x79). It keeps
//     the connection alive; the populated responses are the next round's capture work. See
//     presence.go and docs/design.md.
//
// DataStore (0x73) is deliberately not registered: nothing in Luigi's Mansion 3 has ever been
// observed making a DataStore call, so there is nothing to back.
package lm3

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
	// TitleID is Luigi's Mansion 3 on Switch.
	TitleID = "0100DCA0064A6000"
	// NEXVersion is the NEX library the game was built against (apiVersionGeneral 40000).
	NEXVersion = "4.0.0"
	// StationScheme is per title: the console treats the secure server as a Pia station under
	// prudps, which is what the working server for this title hands out.
	StationScheme = "prudps"
	// SecureStreamID 1 is the two-listener shape: authentication on PortBase+0, secure on its
	// own dialable PortBase+1. The working server for this title runs two separate WSS
	// listeners and its secure station URL carries sid=1.
	SecureStreamID = 1
	// SourceKeyBytes is the length of the kerberos source key handed to the console. 32 for
	// this title: the working server derives it with SHA-256 and hands over all 32 bytes.
	SourceKeyBytes = 32
	// PortBase is this title's NEX block from ports.md (auth 22190, secure 22191, healthz 22192).
	PortBase = 22190
)

// Register wires the title's protocols onto the shared host.
func Register(host *nexhost.Host) error {
	url := os.Getenv("LM3_DATABASE_URL")
	if url == "" {
		return errors.New("LM3_DATABASE_URL is required: matchmaking and NEX unique IDs both keep their state in Postgres")
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("LM3_DATABASE_URL: %w", err)
	}

	registerMatchmaking(host, db)
	registerUtility(host, db)

	natProtocol := nat_traversal.NewProtocol()
	host.Secure.RegisterServiceProtocol(natProtocol)
	common_nat_traversal.NewCommonProtocol(natProtocol)

	// The game's own station/presence protocol. Acknowledged, not implemented — see presence.go.
	registerPresence(host)

	return nil
}

// registerMatchmaking wires the three matchmaking protocols to one manager. Friend-only
// lobbies go to nx-baas through the shared host.
func registerMatchmaking(host *nexhost.Host, db *sql.DB) {
	manager := common_globals.NewMatchmakingManager(host.Secure, db)
	// Friend-only lobbies. Upstream documents this callback as not working on Switch because
	// it has no Switch friend graph; OpenPak has one, in nx-baas.
	manager.GetUserFriendPIDs = host.FriendPIDs

	// Register before wrapping, all three: a common protocol reads the endpoint off the
	// protocol it is given, and a protocol has no endpoint until it is registered. match-making
	// goes first because its SetManager creates the matchmaking schema the other two use.
	matchmakingProtocol := matchmaking.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchmakingProtocol)
	common_matchmaking.NewCommonProtocol(matchmakingProtocol).SetManager(manager)

	extensionProtocol := matchmake_extension.NewProtocol()
	host.Secure.RegisterServiceProtocol(extensionProtocol)
	common_matchmake_extension.NewCommonProtocol(extensionProtocol).SetManager(manager)

	extProtocol := matchmaking_ext.NewProtocol()
	host.Secure.RegisterServiceProtocol(extProtocol)
	common_matchmaking_ext.NewCommonProtocol(extProtocol).SetManager(manager)
}

// registerUtility wires Utility (0x6E): NEX unique IDs against the database, plus the integer
// and string settings the game reads immediately after connecting.
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
var Title = nexhost.Title{Name: "luigis-mansion-3", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
	c.PortBase = PortBase
	c.Version = NEXVersion
	c.StationScheme = StationScheme
	c.SecureStreamID = SecureStreamID
	c.SourceKeyBytes = SourceKeyBytes
}
