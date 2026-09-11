package nex

import (
	//"fmt"

	"github.com/PretendoNetwork/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonnattraversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	commonsecure "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
	nattraversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	secure "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"

	commonmatchmaking "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	commonmatchmakingext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	commonmatchmakeextension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	commonmessagedelivery "github.com/PretendoNetwork/nex-protocols-common-go/v2/message-delivery"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakingext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"

	matchmakingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	messagedelivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"

	local_match_making "github.com/PretendoNetwork/monster-hunter-xx/nex/match_making"
	local_matchmake_extension "github.com/PretendoNetwork/monster-hunter-xx/nex/matchmake-extension"
)

func CreateReportDBRecord(_ types.PID, _ types.UInt32, _ types.QBuffer) error {
	return nil
}

func registerCommonSecureServerProtocols() {
	secureProtocol := secure.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(secureProtocol)
	commonSecureProtocol := commonsecure.NewCommonProtocol(secureProtocol)
	commonSecureProtocol.EnableInsecureRegister()

	globals.MatchmakingManager.GetUserFriendPIDs = globals.GetUserFriendPIDs

	commonSecureProtocol.CreateReportDBRecord = CreateReportDBRecord

	natTraversalProtocol := nattraversal.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(natTraversalProtocol)
	commonnattraversal.NewCommonProtocol(natTraversalProtocol)

	matchMakingProtocol := matchmaking.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingProtocol)
	matchMakingProtocol.FindByOwner = local_match_making.FindByOwner
	commonmatchmaking.NewCommonProtocol(matchMakingProtocol).SetManager(globals.MatchmakingManager)

	matchMakingExtProtocol := matchmakingext.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingExtProtocol)
	commonmatchmakingext.NewCommonProtocol(matchMakingExtProtocol).SetManager(globals.MatchmakingManager)

	rankingProtocol := ranking.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(rankingProtocol)

	matchmakeExtensionProtocol := matchmakeextension.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol := commonmatchmakeextension.NewCommonProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol.SetManager(globals.MatchmakingManager)
	matchmakeExtensionProtocol.FindMatchmakeSessionByParticipant = local_matchmake_extension.FindMatchmakeSessionByParticipant
	matchmakeExtensionProtocol.BrowseMatchmakeSessionNoHolderNoResultRange = local_matchmake_extension.BrowseMatchmakeSessionNoHolderNoResultRange
	matchmakeExtensionProtocol.UpdateMatchmakeSessionAttribute = local_matchmake_extension.UpdateMatchmakeSessionAttribute
	matchmakeExtensionProtocol.FindMatchmakeSessionBySingleGatheringID = local_matchmake_extension.FindMatchmakeSessionBySingleGatheringId

	messageDeliveryProtocol := messagedelivery.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(messageDeliveryProtocol)
	commonmessagedelivery.NewCommonProtocol(messageDeliveryProtocol).SetManager(globals.MessagingManager)

	commonMatchmakeExtensionProtocol.CleanupMatchmakeSessionSearchCriterias = func(searchCriterias types.List[matchmakingtypes.MatchmakeSessionSearchCriteria]) {
		// for i := range searchCriterias {
		// }
	}
}
