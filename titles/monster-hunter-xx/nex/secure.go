package nex

import (
	"fmt"
	"os"
	"strconv"

	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
)

func StartSecureServer() {
	globals.SecureServer = nex.NewPRUDPServer()
	globals.SecureServer.ByteStreamSettings.UseStructureHeader = true
	globals.SecureServer.ByteStreamSettings.PIDSize = 8

	globals.SecureEndpoint = nex.NewPRUDPEndPoint(1)
	globals.SecureEndpoint.IsSecureEndPoint = true
	globals.SecureEndpoint.ServerAccount = globals.SecureServerAccount
	globals.SecureEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.SecureEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername

	globals.SecureServer.BindPRUDPEndPoint(globals.SecureEndpoint)

	globals.SecureServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(4, 4, 0))
	globals.SecureServer.AccessKey = "4152f312"

	globals.SecureEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()
		protocol := globals.GetProtocolByID(request.ProtocolID)

		//userData, err := globals.UserDataFromPID(packet.Sender().PID())

		// var username string
		// if err != 0 {
		// 	// Some edge cases probably apply, but generally this is fine
		// 	username = "3DS User"
		// } else {
		// 	username = userData.Username
		// }

		fmt.Println("== Monster Hunter XX - Secure ==")
		fmt.Printf("User: %d\n", packet.Sender().PID())
		fmt.Printf("Protocol: %d (%s)\n", request.ProtocolID, protocol.Protocol())
		fmt.Printf("Method: %d (%s)\n", request.MethodID, protocol.GetMethodByID(request.MethodID))
		fmt.Println("===============")
	})

	globals.SecureEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Secure: %v", err)
	})

	globals.MatchmakingManager = common_globals.NewMatchmakingManager(globals.SecureEndpoint, globals.Postgres)
	globals.MessagingManager = common_globals.NewMessagingManager(globals.SecureEndpoint, globals.Postgres)

	registerCommonSecureServerProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_MHXX_SECURE_SERVER_PORT"))

	globals.SecureServer.Listen(port)
}
