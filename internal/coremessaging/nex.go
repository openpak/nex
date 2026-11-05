package coremessaging

import (
	"log"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	match_making_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/match-making/database"
	messaging "github.com/PretendoNetwork/nex-protocols-go/v2/messaging"
	messaging_constants "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/constants"
	messaging_types "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/types"
)

// Hook installs the store on a title's MessagingManager: ProcessMessage (both protocols'
// DeliverMessage and DeliverMessageMultiTarget) stores in chat instead of messaging.*.
// Call it before the common protocols' SetManager, which keeps a ProcessMessage already set.
func (s *Store) Hook(manager *common_globals.MessagingManager) {
	manager.ProcessMessage = s.ProcessMessage
}

// ProcessMessage is common_globals.MessagingManager.ProcessMessage over chat. Instant
// messages (flag bit 0) are delivered to the connected recipients and not stored, as the
// upstream store's instant tables were a log nothing read back; other messages are stored
// in chat, one per recipient, and not pushed (the console fetches them).
func (s *Store) ProcessMessage(manager *common_globals.MessagingManager, message types.DataHolder, recipientIDs types.List[types.UInt64], recipientType messaging_constants.RecipientType, sendMessage bool) (types.DataHolder, types.List[types.UInt32], types.List[types.PID], *nex.Error) {
	header, body, err := bodyOf(message)
	if err != nil {
		return message, nil, nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}
	instant := header.UIFlags&1 != 0

	var targets []*nex.PRUDPConnection
	var nodes types.List[types.UInt32]
	var participants types.List[types.PID]
	add := func(c *nex.PRUDPConnection) {
		targets = append(targets, c)
		nodes = append(nodes, types.UInt32(c.ID))
		participants = append(participants, c.PID())
	}

	switch recipientType {
	case 1:
		if instant {
			for _, id := range recipientIDs {
				if c := manager.Endpoint.FindConnectionByPID(uint64(id)); c != nil {
					add(c)
				}
			}
		}
	case 2:
		if manager.MatchmakingManager == nil {
			return message, nil, nil, nex.NewError(nex.ResultCodes.Core.NotImplemented, "Messages to gatherings are not implemented")
		}
		if len(recipientIDs) != 1 {
			return message, nil, nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "Messages to multiple gatherings are not supported")
		}
		manager.MatchmakingManager.Mutex.RLock()
		_, _, members, _, nexError := match_making_database.FindGatheringByID(manager.MatchmakingManager, uint32(recipientIDs[0]))
		if nexError != nil {
			manager.MatchmakingManager.Mutex.RUnlock()
			return message, nil, nil, nexError
		}
		if instant && sendMessage {
			for _, pid := range members {
				if c := manager.Endpoint.FindConnectionByPID(pid); c != nil {
					add(c)
				}
			}
		}
		manager.MatchmakingManager.Mutex.RUnlock()
	default:
		return message, nil, nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "Invalid recipient type")
	}

	if !instant {
		for _, id := range recipientIDs {
			if _, err := s.Send(header, body, uint64(id), recipientType); err != nil {
				log.Printf("[%s] core messaging: store for recipient type %d: %v", s.Cfg.Title, recipientType, err)
				return message, nil, nil, nexErr(err)
			}
		}
		return message, nodes, participants, nil
	}

	if sendMessage {
		lib := manager.Endpoint.Server.LibraryVersions.Messaging
		if recipientType == 1 {
			for _, c := range targets {
				h := setRecipient(lib, header, uint64(c.PID()), recipientType)
				m, nexError := manager.SetMessageHeader(message, h)
				if nexError != nil {
					return message, nil, nil, nexError
				}
				common_globals.SendMessage(manager.Endpoint, m, []*nex.PRUDPConnection{c})
			}
		} else {
			common_globals.SendMessage(manager.Endpoint, message, targets)
		}
	}
	return message, nodes, participants, nil
}

// Install replaces the Messaging (0x17) read and delete handlers of a title's protocol with
// chat-backed ones. Call it after the common protocol's NewCommonProtocol/SetManager.
// Reading without bLeaveOnServer consumes the message (it is deleted), as the upstream
// store's "read" flag hid it for good.
func (s *Store) Install(p messaging.Interface, manager *common_globals.MessagingManager) {
	type recipient = messaging_types.MessageRecipient
	lib := func() *nex.LibraryVersion { return manager.Endpoint.Server.LibraryVersions.Messaging }
	// who resolves and checks the recipient for the caller; ok=false means "no entries".
	who := func(packet nex.PacketInterface, r recipient) (uint64, messaging_constants.RecipientType, bool) {
		id, t := common_globals.GetMessageRecipientData(lib(), r)
		ok := manager.ValidateMessageRecipient(manager, packet.Sender().PID(), id, t)
		return uint64(id), t, ok
	}
	reply := func(method uint32, callID uint32, w func(*nex.ByteStreamOut)) *nex.RMCMessage {
		var body []byte
		if w != nil {
			out := nex.NewByteStreamOut(manager.Endpoint.LibraryVersions(), manager.Endpoint.ByteStreamSettings())
			w(out)
			body = out.Bytes()
		}
		r := nex.NewRMCSuccess(manager.Endpoint, body)
		r.ProtocolID, r.MethodID, r.CallID = messaging.ProtocolID, method, callID
		return r
	}
	holders := func(sts []Stored, id uint64, t messaging_constants.RecipientType) types.List[types.DataHolder] {
		l := make(types.List[types.DataHolder], 0, len(sts))
		for _, st := range sts {
			l = append(l, holder(st, lib(), id, t))
		}
		return l
	}
	consume := func(id uint64, t messaging_constants.RecipientType, sts []Stored) {
		if len(sts) == 0 {
			return
		}
		ids := make([]uint32, 0, len(sts))
		for _, st := range sts {
			ids = append(ids, uint32(st.Header.UIID))
		}
		if _, err := s.Delete(id, t, ids, false); err != nil {
			log.Printf("[%s] core messaging: consume %d read messages: %v", s.Cfg.Title, len(ids), err)
		}
	}

	p.SetHandlerGetNumberOfMessages(func(err error, packet nex.PacketInterface, callID uint32, r recipient) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		var n types.UInt32
		if id, t, ok := who(packet, r); ok {
			_, total, err := s.List(id, t, 0, 1, nil, true)
			if err != nil {
				return nil, nexErr(err)
			}
			n = types.UInt32(total)
		}
		return reply(messaging.MethodGetNumberOfMessages, callID, func(o *nex.ByteStreamOut) { n.WriteTo(o) }), nil
	})

	p.SetHandlerGetMessagesHeaders(func(err error, packet nex.PacketInterface, callID uint32, r recipient, rr types.ResultRange) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		if rr.Length > 100 {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "ResultRange.Length is over 100")
		}
		l := types.List[messaging_types.UserMessage]{}
		if id, t, ok := who(packet, r); ok {
			sts, _, err := s.List(id, t, uint32(rr.Offset), uint32(rr.Length), nil, true)
			if err != nil {
				return nil, nexErr(err)
			}
			for _, st := range sts {
				l = append(l, setRecipient(lib(), st.Header, id, t))
			}
		}
		return reply(messaging.MethodGetMessagesHeaders, callID, func(o *nex.ByteStreamOut) { l.WriteTo(o) }), nil
	})

	p.SetHandlerRetrieveAllMessagesWithinRange(func(err error, packet nex.PacketInterface, callID uint32, r recipient, rr types.ResultRange) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		if rr.Length > 100 {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "ResultRange.Length is over 100")
		}
		l := types.List[types.DataHolder]{}
		if id, t, ok := who(packet, r); ok {
			sts, _, err := s.List(id, t, uint32(rr.Offset), uint32(rr.Length), nil, false)
			if err != nil {
				return nil, nexErr(err)
			}
			l = holders(sts, id, t)
			consume(id, t, sts)
		}
		return reply(messaging.MethodRetrieveAllMessagesWithinRange, callID, func(o *nex.ByteStreamOut) { l.WriteTo(o) }), nil
	})

	p.SetHandlerRetrieveMessages(func(err error, packet nex.PacketInterface, callID uint32, r recipient, ids types.List[types.UInt32], leave types.Bool) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		if len(ids) == 0 || len(ids) > 100 {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "lstMsgIDs must hold 1 to 100 ids")
		}
		l := types.List[types.DataHolder]{}
		if id, t, ok := who(packet, r); ok {
			want := make([]uint32, 0, len(ids))
			for _, x := range ids {
				want = append(want, uint32(x))
			}
			sts, _, err := s.List(id, t, 0, uint32(len(want)), want, false)
			if err != nil {
				return nil, nexErr(err)
			}
			l = holders(sts, id, t)
			if !bool(leave) {
				consume(id, t, sts)
			}
		}
		return reply(messaging.MethodRetrieveMessages, callID, func(o *nex.ByteStreamOut) { l.WriteTo(o) }), nil
	})

	p.SetHandlerDeleteMessages(func(err error, packet nex.PacketInterface, callID uint32, r recipient, ids types.List[types.UInt32]) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		if len(ids) == 0 {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "lstMessagesToDelete must not be empty")
		}
		if id, t, ok := who(packet, r); ok {
			want := make([]uint32, 0, len(ids))
			for _, x := range ids {
				want = append(want, uint32(x))
			}
			if _, err := s.Delete(id, t, want, false); err != nil {
				return nil, nexErr(err)
			}
		}
		return reply(messaging.MethodDeleteMessages, callID, nil), nil
	})

	p.SetHandlerDeleteAllMessages(func(err error, packet nex.PacketInterface, callID uint32, r recipient) (*nex.RMCMessage, *nex.Error) {
		if err != nil {
			return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
		}
		var n types.UInt32
		if id, t, ok := who(packet, r); ok {
			d, err := s.Delete(id, t, nil, true)
			if err != nil {
				return nil, nexErr(err)
			}
			n = types.UInt32(d)
		}
		return reply(messaging.MethodDeleteAllMessages, callID, func(o *nex.ByteStreamOut) { n.WriteTo(o) }), nil
	})
}
