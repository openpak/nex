package matchmake_extension

import (
	"github.com/PretendoNetwork/nex-go/v2"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	"github.com/PretendoNetwork/yo-kai-watch-blasters/globals"
)

// replace later
func ClearMyBlockList(err error, packet nex.PacketInterface, callID uint32) (*nex.RMCMessage, *nex.Error) {

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, nil)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.MethodID = matchmake_extension.MethodClearMyBlockList
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
