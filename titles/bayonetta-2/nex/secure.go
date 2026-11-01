package nex

import (
	"fmt"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"os"
	"strconv"

	"github.com/PretendoNetwork/bayonetta-2/globals"
	"github.com/PretendoNetwork/nex-go/v2"
)

func StartSecureServer() {
	globals.SecureServer = nex.NewPRUDPServer()
	globals.SecureServer.ByteStreamSettings.UseStructureHeader = true

	globals.SecureEndpoint = nex.NewPRUDPEndPoint(1)
	globals.SecureEndpoint.IsSecureEndPoint = true
	globals.SecureEndpoint.ServerAccount = globals.SecureServerAccount
	globals.SecureEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.SecureEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.SecureServer.BindPRUDPEndPoint(globals.SecureEndpoint)

	globals.SecureServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(3, 5, 2))
	globals.SecureServer.AccessKey = "fd40a3cc"

	globals.SecureEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()
		protocol := globals.GetProtocolByID(request.ProtocolID)

		fmt.Println("==Bayonetta 2 - Secure==")
		fmt.Printf("User: %d\n", packet.Sender().PID())
		fmt.Printf("Protocol ID: %d (%s)\n", request.ProtocolID, protocol.Protocol())
		fmt.Printf("Method ID: %d (%s)\n", request.MethodID, protocol.GetMethodByID(request.MethodID))
		fmt.Println("===============")
	})

	globals.SecureEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Secure: %v", err)
	})

	globals.MatchmakingManager = common_globals.NewMatchmakingManager(globals.SecureEndpoint, globals.Postgres)

	registerCommonSecureServerProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_BAYONETTA2_SECURE_SERVER_PORT"))

	globals.SecureServer.Listen(port)
}
