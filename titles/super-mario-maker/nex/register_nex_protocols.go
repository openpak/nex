package nex

import (
	message_delivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
	nex_message_delivery "github.com/openpak/nex/titles/super-mario-maker/nex/message-delivery"
)

func registerNEXProtocols() {
	messageDeliveryProtocol := message_delivery.NewProtocol()
	messageDeliveryProtocol.DeliverMessage = nex_message_delivery.DeliverMessage
	globals.SecureEndpoint.RegisterServiceProtocol(messageDeliveryProtocol)
}
