package nex

import (
	"fmt"
	"os"
	"strconv"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/encryption"
	"github.com/PretendoNetwork/resident-evil-the-mercenaries-3d/globals"
)

func StartAuthenticationServer() {
	globals.AuthenticationServer = nex.NewPRUDPServer()

	globals.AuthenticationEndpoint = nex.NewPRUDPEndPoint(1)
	globals.AuthenticationEndpoint.ServerAccount = globals.AuthenticationServerAccount
	globals.AuthenticationEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.AuthenticationEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.AuthenticationEndpoint.DefaultStreamSettings.EncryptionAlgorithm = encryption.NewQuazalRC4Encryption()

	globals.AuthenticationServer.BindPRUDPEndPoint(globals.AuthenticationEndpoint)

	globals.AuthenticationServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(1, 1, 0))
	globals.AuthenticationServer.AccessKey = "36bd3c25"
	globals.AuthenticationServer.SetFragmentSize(962)
	globals.AuthenticationServer.PRUDPV0Settings.LegacyConnectionSignature = true

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

		fmt.Println("== Resident Evil - The Mercenaries 3D - Auth ==")
		//fmt.Printf("User: %d\n", packet.Sender().PID())
		fmt.Printf("Protocol: %d (%s)\n", request.ProtocolID, protocol.Protocol())
		fmt.Printf("Method: %d (%s)\n", request.MethodID, protocol.GetMethodByID(request.MethodID))
		fmt.Println("===============")
	})

	globals.AuthenticationEndpoint.OnError(func(err *nex.Error) {
		globals.Logger.Errorf("Auth: %v", err)
	})

	registerCommonAuthenticationServerProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_RETM_AUTHENTICATION_SERVER_PORT"))

	globals.AuthenticationServer.Listen(port)
}
