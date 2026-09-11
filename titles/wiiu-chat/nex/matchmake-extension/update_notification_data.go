package nex_matchmake_extension

import (
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	notification_constants "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
	"github.com/openpak/nex/titles/wiiu-chat/database"
	"github.com/openpak/nex/titles/wiiu-chat/globals"
	"github.com/openpak/nex/titles/wiiu-chat/grpc"
	nex_notifications "github.com/openpak/nex/titles/wiiu-chat/nex/notifications"

	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

func UpdateNotificationData(err error, packet nex.PacketInterface, callID uint32, uiType notification_constants.NotificationCategory, uiParam1 types.UInt64, uiParam2 types.UInt64, strParam types.String) (*nex.RMCMessage, *nex.Error) {
	globals.Logger.Infof("uiType: %d, uiParam1: %d, uiParam2: %d, strParam: %s\r\n", uiType, uiParam1, uiParam2, strParam)
	recipientClient := globals.SecureEndpoint.FindConnectionByPID(uint64(uiParam2))

	if uint32(uiType) == requestJoinGatheringCategory {
		notificationType := requestJoinGatheringEvent
		target := types.NewPID(uint64(uiParam2))
		database.NewCall(packet.Sender().PID(), target)

		// If they don't have a session with the app, tell Friends to alert them on the HOME menu.
		if recipientClient != nil && recipientClient.StationURLs != nil {
			nex_notifications.ProcessNotificationEvent(callID, packet, notificationType, uiParam1, uiParam2, strParam)
		} else {
			grpc.SendFriendsNotification(packet.Sender().PID(), types.NewPID(uint64(uiParam2)), true)
		}
	}

	if uint32(uiType) == endGatheringCategory {
		notificationType := endGatheringEvent
		caller := types.NewPID(uint64(uiParam1))

		database.EndCall(caller)

		// Alert the other side we aren't calling anymore.

		if recipientClient != nil && recipientClient.StationURLs != nil {
			nex_notifications.ProcessNotificationEvent(callID, packet, notificationType, uiParam1, uiParam2, strParam)
		} else {
			grpc.SendFriendsNotification(packet.Sender().PID(), types.NewPID(uint64(uiParam2)), false)
		}
	}

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, nil)
	rmcResponse.ProtocolID = matchmake_extension.ProtocolID
	rmcResponse.CallID = callID
	rmcResponse.MethodID = matchmake_extension.MethodUpdateNotificationData

	return rmcResponse, nil
}
