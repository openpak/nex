package nex

import (
	"fmt"
	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/redact"
	"os"
	"strconv"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/openpak/nex/titles/pokemon-gen6/globals"
)

func StartSecureServer() {
	globals.SecureServer = nex.NewPRUDPServer()

	globals.SecureEndpoint = nex.NewPRUDPEndPoint(1)
	go nexhost.WatchBans("pokemon-gen6", globals.SecureEndpoint)
	globals.SecureEndpoint.IsSecureEndPoint = true
	globals.SecureEndpoint.ServerAccount = globals.SecureServerAccount
	globals.SecureEndpoint.AccountDetailsByPID = globals.AccountDetailsByPID
	globals.SecureEndpoint.AccountDetailsByUsername = globals.AccountDetailsByUsername
	globals.SecureServer.BindPRUDPEndPoint(globals.SecureEndpoint)

	globals.SecureServer.LibraryVersions.SetDefault(nex.NewLibraryVersion(3, 3, 0))
	globals.SecureServer.AccessKey = "876138df"

	globals.SubscriptionTimeline = make(globals.SubscriptionDataTimeline)
	globals.SubscriptionTargets = make(globals.SubscriptionDataTargets)

	globals.SecureEndpoint.OnData(func(packet nex.PacketInterface) {
		request := packet.RMCMessage()
		protocol := globals.GetProtocolByID(request.ProtocolID)

		fmt.Println("=== Pokemon X/Y/OR/AS (Gen 6) - Secure ===")
		fmt.Printf("User: %s\n", redact.Tag(uint64(packet.Sender().PID())))
		fmt.Printf("Protocol: %d (%s)\n", request.ProtocolID, protocol.Protocol())
		fmt.Printf("Method: %d (%s)\n", request.MethodID, protocol.GetMethodByID(request.MethodID))
		fmt.Println("====================")
	})

	globals.SecureEndpoint.OnConnectionEnded(func(connection *nex.PRUDPConnection) {
		// clear user notification data
		globals.SubscriptionTimeline.ClearData(connection.PID(), globals.SubscriptionTargets)
	})

	registerCommonSecureServerProtocols()
	registerSecureServerNEXProtocols()

	port, _ := strconv.Atoi(os.Getenv("PN_POKEGEN6_SECURE_SERVER_PORT"))

	globals.SecureServer.Listen(port)
}
