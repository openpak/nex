package nex_datastore_super_smash_bros_4

import (
	"fmt"
	"github.com/openpak/nex/redact"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_super_smash_bros_4_protocol "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-smash-bros-4"
	datastore_super_smash_bros_4_protocol_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-smash-bros-4/types"
	"github.com/openpak/nex/titles/super-smash-bros-3ds/globals"
)

func CheckPostReplay(err error, packet nex.PacketInterface, callID uint32, param datastore_super_smash_bros_4_protocol_types.DataStorePreparePostReplayParam) (*nex.RMCMessage, *nex.Error) {
	if redact.TraceBodies {
		fmt.Printf("Post Param: %s\n", param.String())
	}

	pIsRequired := types.NewBool(true)
	pIsRare := types.NewBool(false) /// no idea what this is lmao

	rmcResponseStream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)

	pIsRequired.WriteTo(rmcResponseStream)
	pIsRare.WriteTo(rmcResponseStream)

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, rmcResponseStream.Bytes())
	rmcResponse.ProtocolID = datastore_super_smash_bros_4_protocol.ProtocolID
	rmcResponse.MethodID = datastore_super_smash_bros_4_protocol.MethodCheckPostReplay
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
