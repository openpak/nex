package nex_subscription

import (
	"github.com/PretendoNetwork/nex-go/v2"
	rating "github.com/PretendoNetwork/nex-protocols-go/v2/rating"
	rating_types "github.com/PretendoNetwork/nex-protocols-go/v2/rating/types"
	"github.com/openpak/nex/titles/pokemon-gen6/globals"
)

func Unk2(err error, packet nex.PacketInterface, callID uint32, sessionToken rating_types.RatingSessionToken) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}

	client := packet.Sender()

	endpoint := client.Endpoint().(*nex.PRUDPEndPoint)

	rmcResponse := nex.NewRMCSuccess(endpoint, nil)
	rmcResponse.ProtocolID = rating.ProtocolID
	rmcResponse.MethodID = rating.MethodUnk2
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
