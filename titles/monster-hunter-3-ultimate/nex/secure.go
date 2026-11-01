package nex

import (
	"fmt"
	"github.com/openpak/nex/titles/monster-hunter-3-ultimate/database"
	"github.com/openpak/nex/titles/monster-hunter-3-ultimate/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	match_making_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making/database"
	"os"
	"strconv"
)

func StartSecureServer() {
	globals.SecureServer = nex.NewPRUDPServer()

	globals.SecureEndpoint = nex.NewPRUDPEndPoint(1)
	globals.SecureEndpoint.IsSecureEndPoint = true
	globals.SecureEndpoint.ServerAccount = globals.SecureServerAccount
	globals.SecureEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.SecureEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.SecureServer.BindPRUDPEndPoint(globals.SecureEndpoint)
	globals.SecureServer.PRUDPV1Settings.LegacyConnectionSignature = true

	globals.SecureServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(3, 0, 5))
	globals.SecureServer.AccessKey = "cb2b2f5a"

	globals.SecureEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()

		fmt.Println("==Monster Hunter 3 Ultimate - Secure==")
		fmt.Printf("Protocol ID: %d\n", request.ProtocolID)
		fmt.Printf("Method ID: %d\n", request.MethodID)
		fmt.Println("===============")
	})

	globals.SecureEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Secure: %v", err)
	})

	globals.MatchmakingManager = common_globals.NewMatchmakingManager(globals.SecureEndpoint, globals.Postgres)
	globals.MatchmakingManager.GetUserFriendPIDs = globals.GetUserFriendPIDs

	// * Game specific changes. MatchmakeSessions contain 9 attributes, and buffers appear to be larger than 512. Capping at 768 for now.
	globals.MatchmakingManager.MatchmakeSessionAttrNum = 9
	globals.MatchmakingManager.MatchmakeSessionBufferLength = 768

	globals.MessagingManager = common_globals.NewMessagingManager(globals.SecureEndpoint, globals.Postgres)

	port, _ := strconv.Atoi(os.Getenv("PN_MH3U_SECURE_SERVER_PORT"))

	registerCommonSecureServerProtocols()

	database.InitializeGatherings()

	// * This is being run in case a user disconnects unnaturally while inside of a world.
	globals.SecureEndpoint.OnConnectionEnded(func(connection *nex.PRUDPConnection) {
		match_making_database.DisconnectParticipant(globals.MatchmakingManager, connection)
	})

	globals.SecureServer.Listen(port)
}
