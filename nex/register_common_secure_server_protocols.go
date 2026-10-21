package nex

import (
	"github.com/PretendoNetwork/monster-hunter-3-ultimate/globals"
	"github.com/PretendoNetwork/nex-go/v2/types"

	local_matchmaking "github.com/PretendoNetwork/monster-hunter-3-ultimate/nex/match-making"
	local_message_delivery "github.com/PretendoNetwork/monster-hunter-3-ultimate/nex/message-delivery"
	local_messaging_database "github.com/PretendoNetwork/monster-hunter-3-ultimate/nex/messaging/database"

	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakingext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	messagedelivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
	messaging "github.com/PretendoNetwork/nex-protocols-go/v2/messaging"
	nattraversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	secure "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"

	commonmatchmaking "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	commonmatchmakingext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	commonmatchmakeextension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	commonmessagedelivery "github.com/PretendoNetwork/nex-protocols-common-go/v2/message-delivery"
	commonmessaging "github.com/PretendoNetwork/nex-protocols-common-go/v2/messaging"
	commonnattraversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	commonsecure "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
)

func CreateReportDBRecord(_ types.PID, _ types.UInt32, _ types.QBuffer) error {
	return nil
}

func registerCommonSecureServerProtocols() {

	secureProtocol := secure.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(secureProtocol)
	commonSecureProtocol := commonsecure.NewCommonProtocol(secureProtocol)
	commonSecureProtocol.EnableInsecureRegister()

	commonSecureProtocol.CreateReportDBRecord = CreateReportDBRecord

	// * At the moment, I'm unsure what it uses DataStore for, but it gets called on every startup. It goes past fine, even with Core::NotImplemented.
	dataStoreProtocol := datastore.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(dataStoreProtocol)

	matchMakingProtocol := matchmaking.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingProtocol)
	commonMatchMakingProtocol := commonmatchmaking.NewCommonProtocol(matchMakingProtocol)
	commonMatchMakingProtocol.SetManager(globals.MatchmakingManager)

	matchMakingProtocol.FindByOwner = local_matchmaking.FindByOwner

	matchMakingExtProtocol := matchmakingext.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingExtProtocol)
	commonMatchMakingExtProtocol := commonmatchmakingext.NewCommonProtocol(matchMakingExtProtocol)
	commonMatchMakingExtProtocol.SetManager(globals.MatchmakingManager)

	matchmakeExtensionProtocol := matchmakeextension.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol := commonmatchmakeextension.NewCommonProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol.SetManager(globals.MatchmakingManager)

	messageDeliveryProtocol := messagedelivery.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(messageDeliveryProtocol)
	commonmessagedelivery.NewCommonProtocol(messageDeliveryProtocol).SetManager(globals.MessagingManager)

	messageDeliveryProtocol.DeliverMessage = local_message_delivery.DeliverMessage

	messagingProtocol := messaging.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(messagingProtocol)
	commonmessaging.NewCommonProtocol(messagingProtocol).SetManager(globals.MessagingManager)

	globals.MessagingManager.ValidateMessage = globals.ValidateUserMessage
	globals.MessagingManager.ProcessMessage = local_messaging_database.ProcessMessage
	globals.MessagingManager.MatchmakingManager = globals.MatchmakingManager

	natTraversalProtocol := nattraversal.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(natTraversalProtocol)
	commonnattraversal.NewCommonProtocol(natTraversalProtocol)
}
