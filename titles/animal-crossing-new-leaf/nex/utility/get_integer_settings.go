package utility 

import(
	"github.com/PretendoNetwork/animal-crossing-new-leaf/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	nex_types "github.com/PretendoNetwork/nex-go/v2/types"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
)

func GetIntegerSettings(err error, packet nex.PacketInterface, callID uint32, integerStringIndex nex_types.UInt32) (*nex.RMCMessage, *nex.Error){
	l := nex_types.NewMap[nex_types.UInt16, nex_types.Int32]()
	l[nex_types.NewUInt16(0)] = nex_types.NewInt32(1)
	l[nex_types.NewUInt16(1)] = nex_types.NewInt32(2)
	l[nex_types.NewUInt16(2)] = nex_types.NewInt32(0)
	l[nex_types.NewUInt16(3)] = nex_types.NewInt32(4)
	rmcResponseStream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)

	l.WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, rmcResponseBody)
	rmcResponse.ProtocolID = utility.ProtocolID
	rmcResponse.MethodID = utility.MethodGetIntegerSettings
	rmcResponse.CallID = callID

	return rmcResponse, nil
}

