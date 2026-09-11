package nex

import (
	"fmt"
	"os"
	"strconv"

	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2"
)

func StartAuthenticationServer() {
	globals.AuthenticationServer = nex.NewPRUDPServer()
	globals.AuthenticationServer.ByteStreamSettings.UseStructureHeader = true
	globals.AuthenticationServer.ByteStreamSettings.PIDSize = 8

	globals.AuthenticationEndpoint = nex.NewPRUDPEndPoint(1)
	globals.AuthenticationEndpoint.ServerAccount = globals.AuthenticationServerAccount
	globals.AuthenticationEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.AuthenticationEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername

	globals.AuthenticationServer.BindPRUDPEndPoint(globals.AuthenticationEndpoint)

	globals.AuthenticationServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(4, 4, 0))
	globals.AuthenticationServer.AccessKey = "4152f312"

	globals.AuthenticationEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()
		protocol := globals.GetProtocolByID(request.ProtocolID)

		//userData, err := globals.UserDataFromPID(packet.Sender().PID())

		// var username string
		// if err != 0 {
		// 	username = "Failed to get username"
		// } else {
		// 	username = userData.Username
		// }

		fmt.Println("== Monster Hunter XX - Auth ==")
		//fmt.Printf("User: %d\n", packet.Sender().PID())
		fmt.Printf("Protocol: %d (%s)\n", request.ProtocolID, protocol.Protocol())
		fmt.Printf("Method: %d (%s)\n", request.MethodID, protocol.GetMethodByID(request.MethodID))
		fmt.Println("===============")
	})

	globals.AuthenticationEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Auth: %v", err)
	})

	registerCommonAuthenticationServerProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_MHXX_AUTHENTICATION_SERVER_PORT"))

	globals.AuthenticationServer.Listen(port)
}
