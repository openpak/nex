package nex

import (
	"fmt"

	"github.com/PretendoNetwork/animal-crossing-new-leaf/globals"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonnattraversal "github.com/PretendoNetwork/nex-protocols-common-go/v2/nat-traversal"
	commonsecure "github.com/PretendoNetwork/nex-protocols-common-go/v2/secure-connection"
	nattraversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
	secure "github.com/PretendoNetwork/nex-protocols-go/v2/secure-connection"

	commonmatchmaking "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making"
	commonmatchmakingext "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making-ext"
	commonmatchmakeextension "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension"
	commonutility "github.com/PretendoNetwork/nex-protocols-common-go/v2/utility"
	commondatastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	matchmaking "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmakingext "github.com/PretendoNetwork/nex-protocols-go/v2/match-making-ext"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"

	"strconv"
	"strings"

	"github.com/PretendoNetwork/nex-go/v2"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmakingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	messagedelivery "github.com/PretendoNetwork/nex-protocols-go/v2/message-delivery"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"
	local_utility "github.com/PretendoNetwork/animal-crossing-new-leaf/nex/utility"
)

func updateNotificationData(err error, packet nex.PacketInterface, callID uint32, uiType types.UInt32, uiParam1 types.UInt32, uiParam2 types.UInt32, strParam types.String) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}
	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	rmcResponse := nex.NewRMCSuccess(endpoint, nil)
	rmcResponse.ProtocolID = matchmakeextension.ProtocolID
	rmcResponse.MethodID = matchmakeextension.MethodUpdateNotificationData
	rmcResponse.CallID = callID
	return rmcResponse, nil
}
func getFriendNotificationData(err error, packet nex.PacketInterface, callID uint32, uiType types.Int32) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		common_globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	connection := packet.Sender().(*nex.PRUDPConnection)
	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	dataList := types.NewList[*notifications_types.NotificationEvent]()

	rmcResponseStream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())

	dataList.WriteTo(rmcResponseStream)

	rmcResponseBody := rmcResponseStream.Bytes()

	rmcResponse := nex.NewRMCSuccess(endpoint, rmcResponseBody)
	rmcResponse.ProtocolID = matchmakeextension.ProtocolID
	rmcResponse.MethodID = matchmakeextension.MethodGetFriendNotificationData
	rmcResponse.CallID = callID

	return rmcResponse, nil
}

// Is this needed? -Ash
func cleanupSearchMatchmakeSessionHandler(matchmakeSession *matchmakingtypes.MatchmakeSession) {
	matchmakeSession.Attributes[2] = types.NewUInt32(0)
	matchmakeSession.MatchmakeParam = matchmakingtypes.NewMatchmakeParam()
	matchmakeSession.ApplicationBuffer = types.NewBuffer(make([]byte, 0))
	matchmakeSession.GameMode = types.NewUInt32(33)
	globals.Logger.Info(matchmakeSession.String())
}

func CreateReportDBRecord(_ types.PID, _ types.UInt32, _ types.QBuffer) error {
	return nil
}

// from nex-protocols-common-go/matchmaking_utils.go
func compareSearchCriteria[T ~uint16 | ~uint32](original T, search string) bool {
	if search == "" { // * Accept any value
		return true
	}

	before, after, found := strings.Cut(search, ",")
	if found {
		min, err := strconv.ParseUint(before, 10, 64)
		if err != nil {
			return false
		}

		max, err := strconv.ParseUint(after, 10, 64)
		if err != nil {
			return false
		}

		return min <= uint64(original) && max >= uint64(original)
	} else {
		searchNum, err := strconv.ParseUint(before, 10, 64)
		if err != nil {
			return false
		}

		return searchNum == uint64(original)
	}
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
	commonmatchmaking.NewCommonProtocol(matchMakingProtocol).SetManager(globals.MatchmakingManager)

	dataStoreProtocol := datastore.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(dataStoreProtocol)
	commondatastore.NewCommonProtocol(dataStoreProtocol)

	matchMakingExtProtocol := matchmakingext.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchMakingExtProtocol)
	commonmatchmakingext.NewCommonProtocol(matchMakingExtProtocol).SetManager(globals.MatchmakingManager)

	rankingProtocol := ranking.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(rankingProtocol)
	messagedeliveryProtocol := messagedelivery.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(messagedeliveryProtocol)
	messagedeliveryProtocol.DeliverMessage = func(err error, packet nex.PacketInterface, callID uint32, oUserMessage types.DataHolder) (*nex.RMCMessage, *nex.Error) {
		fmt.Println(err)
		fmt.Printf("MESSAGE DATA: %x\n", packet.RMCMessage().Parameters)
		rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, nil)
		rmcResponse.ProtocolID = messagedelivery.ProtocolID
		rmcResponse.MethodID = messagedelivery.MethodDeliverMessage
		rmcResponse.CallID = callID

		return rmcResponse, nil
	}

	utilityProtocol := utility.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(utilityProtocol)
	commonutility.NewCommonProtocol(utilityProtocol)

	utilityProtocol.GetIntegerSettings = local_utility.GetIntegerSettings
	
	matchmakeExtensionProtocol := matchmakeextension.NewProtocol()
	globals.SecureEndpoint.RegisterServiceProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol := commonmatchmakeextension.NewCommonProtocol(matchmakeExtensionProtocol)
	commonMatchmakeExtensionProtocol.SetManager(globals.MatchmakingManager)

	commonMatchmakeExtensionProtocol.CleanupSearchMatchmakeSession = cleanupSearchMatchmakeSessionHandler
	commonMatchmakeExtensionProtocol.CleanupMatchmakeSessionSearchCriterias = func(searchCriterias types.List[matchmakingtypes.MatchmakeSessionSearchCriteria]) {
		for _, searchCriteria := range searchCriterias {
			searchCriteria.Attribs[2] = types.NewString("")
		}

	}

}
