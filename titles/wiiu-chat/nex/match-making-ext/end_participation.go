package nex_match_making_ext

import (
	"fmt"
	"github.com/openpak/nex/redact"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	match_making_ext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	"github.com/openpak/nex/titles/wiiu-chat/globals"
)

func EndParticipation(err error, packet nex.PacketInterface, callID uint32, idGathering types.UInt32, strMessage types.String) (*nex.RMCMessage, *nex.Error) {
	globals.Logger.Info(fmt.Sprintf("ENDING PARTICIPATION: GID: %s, MESSAGE: %dB", redact.Tag(uint64(idGathering)), len(strMessage)))

	rmcResponseStream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)

	// TODO - Check if this is always true or not, and if not when to set to false
	rmcResponseStream.WriteBool(true)

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, rmcResponseStream.Bytes())
	rmcResponse.ProtocolID = match_making_ext.ProtocolID
	rmcResponse.CallID = callID
	rmcResponse.MethodID = match_making_ext.MethodEndParticipation

	return rmcResponse, nil
}
