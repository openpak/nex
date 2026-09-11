package nex

import (
	"fmt"
	"os"
	"strconv"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/encryption"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	"github.com/PretendoNetwork/resident-evil-the-mercenaries-3d/globals"
)

func StartSecureServer() {
	globals.SecureServer = nex.NewPRUDPServer()

	globals.SecureEndpoint = nex.NewPRUDPEndPoint(1)
	globals.SecureEndpoint.IsSecureEndPoint = true
	globals.SecureEndpoint.ServerAccount = globals.SecureServerAccount
	globals.SecureEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.SecureEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.SecureEndpoint.DefaultStreamSettings.EncryptionAlgorithm = encryption.NewQuazalRC4Encryption()

	globals.SecureServer.BindPRUDPEndPoint(globals.SecureEndpoint)

	globals.SecureServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(1, 1, 0))
	globals.SecureServer.AccessKey = "36bd3c25"
	globals.SecureServer.SessionKeyLength = 16
	globals.SecureServer.SetFragmentSize(962)
	globals.SecureServer.PRUDPV0Settings.LegacyConnectionSignature = true
	globals.SecureServer.PRUDPV0Settings.EncryptedConnect = true

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

		fmt.Println("== Resident Evil - The Mercenaries 3D - Secure ==")
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

	port, _ := strconv.Atoi(os.Getenv("PN_RETM_SECURE_SERVER_PORT"))

	globals.SecureServer.Listen(port)
}
