package nex_matchmake_extension

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/openpak/nex/titles/wiiu-chat/database"
	"github.com/openpak/nex/titles/wiiu-chat/globals"

	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	notification_constants "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
)

func GetFriendNotificationData(err error, packet nex.PacketInterface, callID uint32, uiType notification_constants.NotificationCategorySigned) (*nex.RMCMessage, *nex.Error) {
	dataList := types.NewList[notifications_types.NotificationEvent]()

	caller, target, ringing := database.GetCallInfoByTarget(packet.Sender().PID())

	// TODO: Multiple calls. Wii U Chat can handle it, but we don't support it yet
	if caller != 0 && target == packet.Sender().PID() && ringing {
		// Being called
		notificationType := requestJoinGatheringEvent

		notification := notifications_types.NewNotificationEvent()

		notification.PIDSource = caller
		notification.Type = notificationType
		notification.Param1 = types.NewUInt64(uint64(caller))
		notification.Param2 = types.NewUInt64(uint64(target))
		notification.StrParam = "Invite Request"

		dataList = append(dataList, notification)
	}

	rmcResponseStream := nex.NewByteStreamOut(globals.SecureServer.LibraryVersions, globals.SecureServer.ByteStreamSettings)
	dataList.WriteTo(rmcResponseStream)

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, rmcResponseStream.Bytes())
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.CallID = callID
	rmcResponse.MethodID = matchmake_extension.MethodGetFriendNotificationData

	return rmcResponse, nil
}
