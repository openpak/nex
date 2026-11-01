package nex

import (
	"fmt"
	"os"
	"strconv"

	"github.com/PretendoNetwork/bayonetta-2/globals"
	"github.com/PretendoNetwork/nex-go/v2"
)

func StartAuthenticationServer() {
	globals.AuthenticationServer = nex.NewPRUDPServer()
	globals.AuthenticationServer.ByteStreamSettings.UseStructureHeader = true

	globals.AuthenticationEndpoint = nex.NewPRUDPEndPoint(1)
	globals.AuthenticationEndpoint.ServerAccount = globals.AuthenticationServerAccount
	globals.AuthenticationEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.AuthenticationEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.AuthenticationServer.BindPRUDPEndPoint(globals.AuthenticationEndpoint)

	globals.AuthenticationServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(3, 5, 2))
	globals.AuthenticationServer.AccessKey = "fd40a3cc"

	globals.AuthenticationEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()
		protocol := globals.GetProtocolByID(request.ProtocolID)

		fmt.Println("==Bayonetta 2 - Auth==")
		fmt.Printf("User: %d\n", packet.Sender().PID())
		fmt.Printf("Protocol ID: %d (%s)\n", request.ProtocolID, protocol.Protocol())
		fmt.Printf("Method ID: %d (%s)\n", request.MethodID, protocol.GetMethodByID(request.MethodID))
		fmt.Println("===============")
	})

	globals.AuthenticationEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Auth: %v", err)
	})

	registerCommonAuthenticationServerProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_BAYONETTA2_AUTHENTICATION_SERVER_PORT"))

	globals.AuthenticationServer.Listen(port)
}
