package nex

import (
	"fmt"
	"os"
	"strconv"

	"github.com/openpak/nex/titles/monster-hunter-3-ultimate/globals"
	"github.com/PretendoNetwork/nex-go/v2"
)

func StartAuthenticationServer() {
	globals.AuthenticationServer = nex.NewPRUDPServer()
	globals.AuthenticationServer.PRUDPV1Settings.LegacyConnectionSignature = true

	globals.AuthenticationEndpoint = nex.NewPRUDPEndPoint(1)
	globals.AuthenticationEndpoint.ServerAccount = globals.AuthenticationServerAccount
	globals.AuthenticationEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.AuthenticationEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.AuthenticationServer.BindPRUDPEndPoint(globals.AuthenticationEndpoint)

	globals.AuthenticationServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(3, 0, 5))
	globals.AuthenticationServer.AccessKey = "cb2b2f5a"

	globals.AuthenticationEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()

		fmt.Println("==Monster Hunter 3 Ultimate - Auth==")
		fmt.Printf("Protocol ID: %#v\n", request.ProtocolID)
		fmt.Printf("Method ID: %#v\n", request.MethodID)
		fmt.Println("===============")
	})

	globals.AuthenticationEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Auth: %v", err)
	})

	registerCommonAuthenticationServerProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_MH3U_AUTHENTICATION_SERVER_PORT"))

	globals.AuthenticationServer.Listen(port)
}
