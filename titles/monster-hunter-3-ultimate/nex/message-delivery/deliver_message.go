package message_delivery

import (
	"github.com/openpak/nex/titles/monster-hunter-3-ultimate/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"

	message_delivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
)

func DeliverMessage(err error, packet nex.PacketInterface, callID uint32, oUserMessage types.DataHolder) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	// NOTE - This method will silently fail on any errors
	rmcResponse := nex.NewRMCSuccess(endpoint, nil)
	rmcResponse.ProtocolID = message_delivery.ProtocolID
	rmcResponse.MethodID = message_delivery.MethodDeliverMessage
	rmcResponse.CallID = callID

	recipientID, recipientType, nexError := globals.MessagingManager.ValidateMessage(oUserMessage)
	if nexError != nil {
		common_globals.Logger.Error(nexError.Error())
		return rmcResponse, nil
	}

	// * Skip PrepareMessage

	_, _, _, nexError = globals.MessagingManager.ProcessMessage(globals.MessagingManager, oUserMessage, types.List[types.UInt64]{recipientID}, recipientType, true)
	if nexError != nil {
		common_globals.Logger.Error(nexError.Error())
		return rmcResponse, nil
	}

	return rmcResponse, nil
}
