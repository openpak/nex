package nex_matchmake_extension

import notification_constants "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"

// Wii U Chat signals calls with notifications the current nex-protocols-go no longer names:
// upstream's older library called them NotificationCategories.RequestJoinGathering (101) and
// EndGathering (102) with sub-type None, and composed the event type as category*1000 +
// subtype. The values are kept here so the wire stays identical.
const (
	requestJoinGatheringCategory uint32                                    = 101
	requestJoinGatheringEvent    notification_constants.NotificationEvents = 101000
	endGatheringCategory         uint32                                    = 102
	endGatheringEvent            notification_constants.NotificationEvents = 102000
)
