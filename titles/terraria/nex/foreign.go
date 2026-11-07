package nex

import (
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	nat_traversal "github.com/PretendoNetwork/nex-protocols-go/v2/nat-traversal"
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
	if _, err := globals.Postgres.Exec(`INSERT INTO matchmaking.join_messages (gathering_id, pid, message) VALUES ($1, $2, '') ON CONFLICT DO NOTHING`, int64(gid), int64(pid)); err != nil {
		globals.Logger.Errorf("cross-play join: join message: %v", err)
	}
	notify(gid, pid, owner, count, notifications_constants.ParticipationEventsParticipate)
	globals.Logger.Infof("cross-play join: PID %d added to gathering %d (%d participants), host %d notified", pid, gid, count, owner)
	if globals.RelayStation != "" {
		initiateProbe(uint64(owner), relayStationURL(uint32(pid)))
	}
}

// initiateProbe tells the host (NATTraversal InitiateProbe, as a joiner's RequestProbeInitiationExt would) to probe the relay's
// station, so the host's PIA learns the joiner's address and accepts its connection.
func initiateProbe(hostPID uint64, station types.StationURL) {
	host := globals.SecureEndpoint.FindConnectionByPID(hostPID)
	if host == nil {
		globals.Logger.Warningf("cross-play: host %d not connected, no InitiateProbe", hostPID)
		return
	}
	stream := nex.NewByteStreamOut(globals.SecureEndpoint.LibraryVersions(), globals.SecureEndpoint.ByteStreamSettings())
	types.NewString(station.String()).WriteTo(stream)
	req := nex.NewRMCRequest(globals.SecureEndpoint)
	req.ProtocolID = nat_traversal.ProtocolID
	req.MethodID = nat_traversal.MethodInitiateProbe
	req.CallID = 0xFFFF0000 + uint32(hostPID&0xFFFF)
	req.Parameters = stream.Bytes()
	var pkt nex.PRUDPPacketInterface
	server := globals.SecureEndpoint.Server
	switch host.DefaultPRUDPVersion {
	case 0:
		pkt, _ = nex.NewPRUDPPacketV0(server, host, nil)
	case 1:
		pkt, _ = nex.NewPRUDPPacketV1(server, host, nil)
	default:
		pkt, _ = nex.NewPRUDPPacketLite(server, host, nil)
	}
	pkt.SetType(constants.DataPacket)
	pkt.AddFlag(constants.PacketFlagNeedsAck)
	pkt.AddFlag(constants.PacketFlagReliable)
	pkt.SetSourceVirtualPortStreamType(host.StreamType)
	pkt.SetSourceVirtualPortStreamID(globals.SecureEndpoint.StreamID)
	pkt.SetDestinationVirtualPortStreamType(host.StreamType)
	pkt.SetDestinationVirtualPortStreamID(host.StreamID)
	pkt.SetPayload(req.Bytes())
	server.Send(pkt)
	globals.Logger.Infof("cross-play: InitiateProbe to host %d for %s", hostPID, station.String())
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
