// Command mhgud runs the Monster Hunter Generations Ultimate NEX server.
//
// Everything generic — PRUDP, kerberos, the ticket-granting handshake, OpenPak identity —
// comes from servers/nex. Everything standard — matchmaking, NAT traversal, unique IDs,
// DataStore — comes from Pretendo's nex-protocols-common-go. What is left, and all this
// file is, is the wiring plus the two things MHGU does not share with a native Switch
// title: a 3DS-era kerberos ticket, and a DataStore that has to actually store something.
package mhgu

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/openpak/nex/redact"
	"log"
	"os"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/mmlegacy"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	common_match_making "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	common_match_making_ext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	common_matchmake_extension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	common_message_delivery "github.com/PretendoNetwork/nex-protocols-common-go/v2/message-delivery"
	common_nat_traversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	common_utility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	message_delivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Register wires the title's protocols onto the shared host.
func Register(host *nexhost.Host) error {

	// MHGU is the Switch port of a 3DS game and its online code came with it: it wants the
	// older kerberos ticket, version 0, without the random-key wrapper version 1 puts in
	// front of the encrypted body. See docs/design.md — this is also the only lever anyone
	// has found against the client-side 2306-0116 the ticket size provokes.
	host.Auth.Server.KerberosTicketVersion = 0
	host.Secure.Server.KerberosTicketVersion = 0

	// Station URL type stays at servers/nex's default, the Public flag alone: that is what
	// Pretendo's Monster Hunter XX server sends, and MHGU runs Monster Hunter XX's NEX code.
	// BehindNAT|Public is only a reading, not a capture. See docs/provenance.md: a shipped
	// server for the same engine outranks a reading.

	// Register (0x0B.1) rather than RegisterEx (0x0B.4). The 3DS-era Pia calls the plain
	// one, which nex-protocols-common-go refuses unless this is switched on.
	host.SecureConnection.EnableInsecureRegister()
	// SendReport (0x0B.6) with nothing behind it answers Core::NotImplemented. Monster
	// Hunter XX accepts the report and drops it, which is the difference between the title
	// filing a report and the title being told the call does not exist.
	host.SecureConnection.CreateReportDBRecord = func(types.PID, types.UInt32, types.QBuffer) error { return nil }

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("DATABASE_URL: %w", err)
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}

	friendPIDs := func(pid uint32) []uint32 {
		player, err := host.PlayerByPID(uint64(pid))
		if err != nil {
			log.Printf("friends of %s: %s", redact.Tag(uint64(pid)), redact.Err(err))
			return nil
		}
		pids := make([]uint32, 0, len(player.Friends))
		for _, friend := range player.Friends {
			pids = append(pids, uint32(friend.PID))
		}
		return pids
	}

	matchmaking := common_globals.NewMatchmakingManager(host.Secure, db)
	matchmaking.GetUserFriendPIDs = friendPIDs

	// Register first, then hand the protocol to its common implementation: registering is
	// what gives the protocol its endpoint, and the common constructors read it.
	// match-making leads, because its SetManager creates the schema the other two need.
	matchMaking := match_making.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchMaking)
	common_match_making.NewCommonProtocol(matchMaking).SetManager(matchmaking)

	matchmakeExtension := matchmake_extension.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchmakeExtension)
	common_matchmake_extension.NewCommonProtocol(matchmakeExtension).SetManager(matchmaking)

	matchMakingExt := match_making_ext.NewProtocol()
	host.Secure.RegisterServiceProtocol(matchMakingExt)
	common_match_making_ext.NewCommonProtocol(matchMakingExt).SetManager(matchmaking)

	// The five methods nex-protocols-common-go does not implement and this title calls:
	// FindByOwner, FindMatchmakeSessionByParticipant, FindMatchmakeSessionBySingleGatheringID,
	// BrowseMatchmakeSessionNoHolderNoResultRange, UpdateMatchmakeSessionAttribute. Monster
	// Hunter XX ships its own copies of exactly these five; this is that set, shared.
	mmlegacy.Register(matchMaking, matchmakeExtension, matchmaking)

	// Message Delivery 0x1B. Monster Hunter XX registers it, so the Switch port has it too.
	messaging := common_globals.NewMessagingManager(host.Secure, db)
	messageDelivery := message_delivery.NewProtocol()
	host.Secure.RegisterServiceProtocol(messageDelivery)
	common_message_delivery.NewCommonProtocol(messageDelivery).SetManager(messaging)

	// Ranking 0x70, registered without an implementation, which is what Monster Hunter XX
	// does. A registered protocol answers Core::NotImplemented per method and is logged;
	// what this title wants from ranking is unmeasured, and the log is how that is found.
	host.Secure.RegisterServiceProtocol(ranking.NewProtocol())

	natTraversal := nat_traversal.NewProtocol()
	host.Secure.RegisterServiceProtocol(natTraversal)
	common_nat_traversal.NewCommonProtocol(natTraversal)

	utilities := common_globals.NewUtilityManager(host.Secure, db)
	// ponytail: empty settings maps. A title that asks for settings and gets
	// Core::NotImplemented stops there, and no MHGU setting index is known to us; empty is
	// what unblocks it. Answer a real index once a capture shows the game reading one.
	utilities.GetIntegerSettings = func(*common_globals.UtilityManager, types.PID, types.UInt32) (types.Map[types.UInt16, types.UInt32], *nex.Error) {
		return types.NewMap[types.UInt16, types.UInt32](), nil
	}
	utilities.GetStringSettings = func(*common_globals.UtilityManager, types.PID, types.UInt32) (types.Map[types.UInt16, types.String], *nex.Error) {
		return types.NewMap[types.UInt16, types.String](), nil
	}
	utilityProtocol := utility.NewProtocol()
	host.Secure.RegisterServiceProtocol(utilityProtocol)
	common_utility.NewCommonProtocol(utilityProtocol).SetManager(utilities)

	objects, err := newStore(db)
	if err != nil {
		return fmt.Errorf("datastore schema: %w", err)
	}
	dataStore := datastore.NewProtocol()
	host.Secure.RegisterServiceProtocol(dataStore)
	commonDataStore := common_datastore.NewCommonProtocol(dataStore)
	objects.wire(commonDataStore, friendPIDs)
	if err := attachS3(commonDataStore); err != nil {
		return err
	}
	return nil
}

// attachS3 gives DataStore somewhere to put object bodies. Without it the metadata half
// still works and the transfer half answers Core::NotImplemented, which is a better state
// to run in than refusing to start: nothing else in this server needs a bucket.
func attachS3(common *common_datastore.CommonProtocol) error {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		log.Print("S3_ENDPOINT unset: DataStore metadata only, no object upload or download")
		return nil
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), ""),
		Secure: os.Getenv("S3_USE_TLS") == "1",
	})
	if err != nil {
		return fmt.Errorf("S3_ENDPOINT: %w", err)
	}
	common.SetMinIOClient(client)
	common.S3Bucket = envOr("S3_BUCKET", "mhgu")
	common.SetDataKeyBase("objects")
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// Title is this game"s entry in the shared NEX host.
var Title = nexhost.Title{Name: "monster-hunter-generations-ultimate", Defaults: defaults, Register: Register}

func defaults(c *nexhost.Config) {
}
