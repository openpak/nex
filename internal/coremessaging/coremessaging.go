// Package coremessaging is the core-backed NEX message store: a title flipped to it
// (NEX_CORE_STORAGE contains "messaging", docs/core-backed-storage.md) keeps no
// messaging.* rows. Stored messages (MessageDelivery 0x1B DeliverMessage without the
// instant flag, Messaging 0x17 reads and deletes) are chat's title messages
// (SendTitleMessage, ListTitleMessages, DeleteTitleMessages), scoped by the NEX_TITLE name
// and the network; instant messages are delivered to connected consoles and never stored.
//
// Recipient type 1 (a PID) is the recipient's account inbox; type 2 (a gathering) is the room
// "gathering:<id>", whose readers the title's ValidateMessageRecipient (gathering
// participants) decides. PIDs map to accounts through nn-account's Resolution (coreid cache).
// A sender or recipient with no account link, or a core that does not answer, is a NEX error:
// nothing is kept locally and nothing is read from the old tables.
package coremessaging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	messaging_constants "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/constants"
	messaging_types "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/types"

	chatpb "github.com/openpak/nex/internal/chatpb"
	"github.com/openpak/nex/internal/coreid"
)

// Config names the title in chat.
type Config struct {
	Title   string // neutral key = the NEX_TITLE name ("monster-hunter-4")
	Network string // "wiiu" or "3ds"
}

// Store is one title's message store in chat.
type Store struct {
	Cfg  Config
	Chat chatpb.ChatClient
	IDs  coreid.Resolver
}

// Every core call has a 3 s deadline, as the other core stores.
func (s *Store) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func sysErr(err error) *nex.Error { return nex.NewError(nex.ResultCodes.Core.SystemError, err.Error()) }

func nexErr(err error) *nex.Error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, coreid.ErrNoLink):
		return nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}
	switch status.Code(err) {
	case codes.InvalidArgument, codes.FailedPrecondition:
		return nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	case codes.PermissionDenied:
		return nex.NewError(nex.ResultCodes.Core.AccessDenied, err.Error())
	case codes.ResourceExhausted:
		return nex.NewError(nex.ResultCodes.Core.Exception, err.Error())
	}
	return sysErr(err)
}

// Room is the chat room of a gathering.
func Room(gatheringID uint64) string { return "gathering:" + strconv.FormatUint(gatheringID, 10) }

// inbox maps a NEX recipient to the chat inbox fields (account, room).
func (s *Store) inbox(ctx context.Context, recipientID uint64, recipientType messaging_constants.RecipientType) (string, string, error) {
	switch recipientType {
	case 1:
		acc, err := s.IDs.Account(ctx, recipientID)
		if err != nil {
			return "", "", fmt.Errorf("recipient %d: %w", recipientID, err)
		}
		return acc, "", nil
	case 2:
		return "", Room(recipientID), nil
	}
	return "", "", fmt.Errorf("coremessaging: recipient type %d: %w", recipientType, errBadRecipient)
}

var errBadRecipient = errors.New("invalid recipient type")

// Body is a message's payload: a TextMessage's text or a BinaryMessage's bytes.
type Body struct {
	Text   bool
	Bytes  []byte
	Object string // "TextMessage" or "BinaryMessage"
}

// bodyOf splits a NEX message holder into its header and body.
func bodyOf(message types.DataHolder) (messaging_types.UserMessage, Body, error) {
	switch m := message.Object.(type) {
	case messaging_types.TextMessage:
		return m.UserMessage, Body{Text: true, Bytes: []byte(string(m.StrTextBody)), Object: "TextMessage"}, nil
	case *messaging_types.TextMessage:
		return m.UserMessage, Body{Text: true, Bytes: []byte(string(m.StrTextBody)), Object: "TextMessage"}, nil
	case messaging_types.BinaryMessage:
		return m.UserMessage, Body{Bytes: []byte(m.BinaryBody), Object: "BinaryMessage"}, nil
	case *messaging_types.BinaryMessage:
		return m.UserMessage, Body{Bytes: []byte(m.BinaryBody), Object: "BinaryMessage"}, nil
	}
	return messaging_types.UserMessage{}, Body{}, errors.New("coremessaging: not a TextMessage or BinaryMessage")
}

// Send stores one message for one recipient in chat and returns its id. header is the
// prepared header (sender PID and name, reception time already set by PrepareMessage).
func (s *Store) Send(header messaging_types.UserMessage, body Body, recipientID uint64, recipientType messaging_constants.RecipientType) (uint32, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	from, err := s.IDs.Account(ctx, uint64(header.PIDSender))
	if err != nil {
		return 0, fmt.Errorf("sender: %w", err)
	}
	to, room, err := s.inbox(ctx, recipientID, recipientType)
	if err != nil {
		return 0, err
	}
	kind := "binary"
	if body.Text {
		kind = "text"
	}
	r, err := s.Chat.SendTitleMessage(ctx, &chatpb.SendTitleMessageRequest{
		Title: s.Cfg.Title, Network: s.Cfg.Network,
		FromAccount: from, ToAccount: to, ToRoom: room,
		Subject: string(header.StrSubject), Body: body.Bytes, BodyKind: kind,
		LifetimeS: int64(header.UILifeTime), Flags: int64(header.UIFlags),
		ParentId: int64(header.UIParentID), SenderName: string(header.StrSender),
	})
	if err != nil {
		return 0, err
	}
	return uint32(r.GetMessage().GetId()), nil
}

// Stored is a message read back from chat.
type Stored struct {
	Header messaging_types.UserMessage // recipient fields left for the caller (library version)
	Body   Body
}

func (s *Store) fromCore(ctx context.Context, m *chatpb.TitleMessage) Stored {
	var h messaging_types.UserMessage
	h.UIID = types.UInt32(m.GetId())
	h.UIParentID = types.UInt32(m.GetParentId())
	// A sender whose link is gone (erased, unlinked) reads as PID 0; the message is still theirs.
	if pid, err := s.IDs.PID(ctx, m.GetFromAccount()); err == nil {
		h.PIDSender = types.PID(pid)
	}
	sent := time.UnixMilli(m.GetSentAtUnixMs()).UTC()
	var dt types.DateTime
	h.Receptiontime = dt.FromTimestamp(sent)
	h.UILifeTime = types.UInt32((m.GetExpiresAtUnixMs() - m.GetSentAtUnixMs()) / 1000)
	h.UIFlags = messaging_constants.MessageFlag(m.GetFlags())
	h.StrSubject = types.String(m.GetSubject())
	h.StrSender = types.String(m.GetSenderName())
	b := Body{Bytes: m.GetBody(), Object: "BinaryMessage"}
	if m.GetBodyKind() == "text" {
		b.Text, b.Object = true, "TextMessage"
	}
	return Stored{Header: h, Body: b}
}

// List returns a page of a recipient's inbox (oldest first) and the inbox's total. ids, when
// given, narrow the page to those messages.
func (s *Store) List(recipientID uint64, recipientType messaging_constants.RecipientType, offset uint32, length uint32, ids []uint32, headersOnly bool) ([]Stored, uint32, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	acc, room, err := s.inbox(ctx, recipientID, recipientType)
	if err != nil {
		return nil, 0, err
	}
	req := &chatpb.ListTitleMessagesRequest{Title: s.Cfg.Title, Network: s.Cfg.Network, Account: acc, Room: room,
		Offset: int64(offset), Limit: int32(length), HeadersOnly: headersOnly}
	for _, id := range ids {
		req.Ids = append(req.Ids, int64(id))
	}
	if len(ids) > 0 {
		req.Offset, req.Limit = 0, int32(len(ids))
	}
	r, err := s.Chat.ListTitleMessages(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Stored, 0, len(r.GetMessages()))
	for _, m := range r.GetMessages() {
		out = append(out, s.fromCore(ctx, m))
	}
	return out, uint32(r.GetTotal()), nil
}

// Delete removes messages from a recipient's inbox: ids, or all of it.
func (s *Store) Delete(recipientID uint64, recipientType messaging_constants.RecipientType, ids []uint32, all bool) (int64, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	acc, room, err := s.inbox(ctx, recipientID, recipientType)
	if err != nil {
		return 0, err
	}
	req := &chatpb.DeleteTitleMessagesRequest{Title: s.Cfg.Title, Network: s.Cfg.Network, Account: acc, Room: room, All: all}
	for _, id := range ids {
		req.Ids = append(req.Ids, int64(id))
	}
	r, err := s.Chat.DeleteTitleMessages(ctx, req)
	if err != nil {
		return 0, err
	}
	return r.GetDeleted(), nil
}

// holder builds the NEX holder of a stored message with its recipient fields set for the
// title's messaging library version.
func holder(st Stored, lib *nex.LibraryVersion, recipientID uint64, recipientType messaging_constants.RecipientType) types.DataHolder {
	h := setRecipient(lib, st.Header, recipientID, recipientType)
	var d types.DataHolder
	if st.Body.Text {
		tm := messaging_types.NewTextMessage()
		tm.UserMessage = h
		tm.StrTextBody = types.String(strings.ToValidUTF8(string(st.Body.Bytes), ""))
		d.Object = tm.Copy().(types.DataInterface)
	} else {
		bm := messaging_types.NewBinaryMessage()
		bm.UserMessage = h
		bm.BinaryBody = types.QBuffer(st.Body.Bytes)
		d.Object = bm.Copy().(types.DataInterface)
	}
	return d
}

// setRecipient mirrors common_globals.SetUserMessageRecipientData without its logger.
func setRecipient(lib *nex.LibraryVersion, m messaging_types.UserMessage, id uint64, t messaging_constants.RecipientType) messaging_types.UserMessage {
	if lib != nil && lib.GreaterOrEqual("4.0.0") {
		m.MessageRecipient.UIRecipientType = t
		switch t {
		case 1:
			m.MessageRecipient.PrincipalID = types.PID(id)
		case 2:
			m.MessageRecipient.GatheringID = types.UInt32(id)
		}
		return m
	}
	m.IDRecipient = types.UInt32(id)
	m.UIRecipientType = t
	return m
}
