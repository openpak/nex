package nex

import (
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	notifications_constants "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
	"github.com/openpak/nex/titles/terraria/globals"
)

// ForeignJoin makes a player from another network (joined in the core through the relay) a participant of a Wii U gathering and
// sends the host the same participation event a NEX join sends, so the host's PIA accepts that PID's station (prds#145).
func ForeignJoin(gid, pid uint64) {
	var owner int64
	var count int
	err := globals.Postgres.QueryRow(`UPDATE matchmaking.gatherings SET participants = CASE WHEN $2 = ANY(participants) THEN participants
		ELSE array_append(participants, $2::numeric(20)) END WHERE id=$1 RETURNING owner_pid, cardinality(participants)`, int64(gid), int64(pid)).Scan(&owner, &count)
	if err != nil {
		globals.Logger.Errorf("cross-play join: gathering %d: %v", gid, err)
		return
	}
	notify(gid, pid, owner, count, notifications_constants.ParticipationEventsParticipate)
	globals.Logger.Infof("cross-play join: PID %d added to gathering %d (%d participants), host %d notified", pid, gid, count, owner)
}

// ForeignLeave removes the player again and tells the host it disconnected.
func ForeignLeave(gid, pid uint64) {
	var owner int64
	var count int
	err := globals.Postgres.QueryRow(`UPDATE matchmaking.gatherings SET participants = array_remove(participants, $2::numeric(20))
		WHERE id=$1 RETURNING owner_pid, cardinality(participants)`, int64(gid), int64(pid)).Scan(&owner, &count)
	if err != nil {
		return
	}
	notify(gid, pid, owner, count, notifications_constants.ParticipationEventsDisconnect)
}

func notify(gid, pid uint64, owner int64, count int, ev notifications_constants.ParticipationEvents) {
	e := notifications_types.NewNotificationEvent()
	e.PIDSource = types.NewPID(pid)
	e.Type = notifications_constants.NotificationCategoryParticipationEvent.Build(ev)
	e.Param1 = types.UInt64(gid)
	e.Param2 = types.UInt64(pid)
	e.StrParam = types.NewString("")
	e.Param3 = types.UInt64(count)
	common_globals.SendNotificationEvent(globals.SecureEndpoint, e, []uint64{uint64(owner)})
}
