package coremessaging

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	messaging_constants "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/constants"
	messaging_types "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/types"

	chatpb "github.com/openpak/nex/internal/chatpb"
	"github.com/openpak/nex/internal/coreid"
)

// fakeChat is an in-memory chat with the title-message rules that matter here: scope by
// title and network, account xor room inbox, oldest first, ids inside the inbox only.
type fakeChat struct {
	chatpb.ChatClient
	mu   sync.Mutex
	msgs []*chatpb.TitleMessage
	next int64
	down bool
}

func (f *fakeChat) SendTitleMessage(_ context.Context, r *chatpb.SendTitleMessageRequest, _ ...grpc.CallOption) (*chatpb.SendTitleMessageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, status.Error(codes.Unavailable, "down")
	}
	if (r.GetToAccount() == "") == (r.GetToRoom() == "") {
		return nil, status.Error(codes.InvalidArgument, "one recipient")
	}
	f.next++
	now := time.Now().UnixMilli()
	m := &chatpb.TitleMessage{Id: f.next, Title: r.GetTitle(), Network: r.GetNetwork(), FromAccount: r.GetFromAccount(),
		ToAccount: r.GetToAccount(), ToRoom: r.GetToRoom(), Subject: r.GetSubject(), Body: r.GetBody(),
		BodyKind: r.GetBodyKind(), SenderName: r.GetSenderName(), Flags: r.GetFlags(), ParentId: r.GetParentId(),
		SentAtUnixMs: now, ExpiresAtUnixMs: now + r.GetLifetimeS()*1000}
	f.msgs = append(f.msgs, m)
	return &chatpb.SendTitleMessageResponse{Message: m}, nil
}

func (f *fakeChat) inbox(title, network, account, room string) []*chatpb.TitleMessage {
	var out []*chatpb.TitleMessage
	for _, m := range f.msgs {
		if m.Title == title && m.Network == network && m.ToAccount == account && m.ToRoom == room {
			out = append(out, m)
		}
	}
	return out
}

func (f *fakeChat) ListTitleMessages(_ context.Context, r *chatpb.ListTitleMessagesRequest, _ ...grpc.CallOption) (*chatpb.ListTitleMessagesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, status.Error(codes.Unavailable, "down")
	}
	all := f.inbox(r.GetTitle(), r.GetNetwork(), r.GetAccount(), r.GetRoom())
	var page []*chatpb.TitleMessage
	for _, m := range all {
		if len(r.GetIds()) > 0 && !has(r.GetIds(), m.Id) {
			continue
		}
		page = append(page, m)
	}
	if int(r.GetOffset()) < len(page) {
		page = page[r.GetOffset():]
	} else {
		page = nil
	}
	if r.GetLimit() > 0 && len(page) > int(r.GetLimit()) {
		page = page[:r.GetLimit()]
	}
	return &chatpb.ListTitleMessagesResponse{Messages: page, Total: int64(len(all))}, nil
}

func (f *fakeChat) DeleteTitleMessages(_ context.Context, r *chatpb.DeleteTitleMessagesRequest, _ ...grpc.CallOption) (*chatpb.DeleteTitleMessagesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keep []*chatpb.TitleMessage
	var n int64
	for _, m := range f.msgs {
		in := m.Title == r.GetTitle() && m.Network == r.GetNetwork() && m.ToAccount == r.GetAccount() && m.ToRoom == r.GetRoom()
		if in && (r.GetAll() || has(r.GetIds(), m.Id)) {
			n++
			continue
		}
		keep = append(keep, m)
	}
	f.msgs = keep
	return &chatpb.DeleteTitleMessagesResponse{Deleted: n}, nil
}

func has(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

type fakeIDs struct{ known map[uint64]bool }

func acct(pid uint64) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", pid) }

func (f fakeIDs) Account(_ context.Context, pid uint64) (string, error) {
	if !f.known[pid] {
		return "", coreid.ErrNoLink
	}
	return acct(pid), nil
}

func (f fakeIDs) PID(_ context.Context, account string) (uint64, error) {
	for p := range f.known {
		if acct(p) == account {
			return p, nil
		}
	}
	return 0, coreid.ErrNoLink
}

func newStore(known ...uint64) (*Store, *fakeChat) {
	ids := fakeIDs{known: map[uint64]bool{}}
	for _, p := range known {
		ids.known[p] = true
	}
	chat := &fakeChat{}
	return &Store{Cfg: Config{Title: "monster-hunter-4", Network: "3ds"}, Chat: chat, IDs: ids}, chat
}

func binary(from uint64, subject string, body []byte, flags uint32) types.DataHolder {
	bm := messaging_types.NewBinaryMessage()
	bm.PIDSender = types.PID(from)
	bm.StrSender = "Hunter"
	bm.StrSubject = types.String(subject)
	bm.UILifeTime = 600
	bm.UIFlags = messaging_constants.MessageFlag(flags)
	bm.BinaryBody = types.QBuffer(body)
	var d types.DataHolder
	d.Object = bm.Copy().(types.DataInterface)
	return d
}

func text(from uint64, body string) types.DataHolder {
	tm := messaging_types.NewTextMessage()
	tm.PIDSender = types.PID(from)
	tm.StrTextBody = types.String(body)
	tm.UILifeTime = 60
	var d types.DataHolder
	d.Object = tm.Copy().(types.DataInterface)
	return d
}

// DeliverMessage to a PID stores one chat title message in the recipient's account inbox,
// with the binary body, subject, sender name and lifetime; nothing is pushed.
func TestProcessMessageStoresInChat(t *testing.T) {
	s, chat := newStore(100, 200)
	mm := &common_globals.MessagingManager{}
	if _, _, _, e := s.ProcessMessage(mm, binary(100, "quest", []byte{0, 1, 0xff}, 0), types.List[types.UInt64]{200}, 1, true); e != nil {
		t.Fatal(e)
	}
	if len(chat.msgs) != 1 {
		t.Fatalf("stored %d", len(chat.msgs))
	}
	m := chat.msgs[0]
	if m.Title != "monster-hunter-4" || m.Network != "3ds" || m.FromAccount != acct(100) || m.ToAccount != acct(200) ||
		m.ToRoom != "" || m.BodyKind != "binary" || string(m.Body) != "\x00\x01\xff" || m.Subject != "quest" ||
		m.SenderName != "Hunter" || m.ExpiresAtUnixMs-m.SentAtUnixMs != 600000 {
		t.Fatalf("stored message: %+v", m)
	}
	if _, _, _, e := s.ProcessMessage(mm, text(200, "hi"), types.List[types.UInt64]{100}, 1, true); e != nil {
		t.Fatal(e)
	}
	if chat.msgs[1].BodyKind != "text" || string(chat.msgs[1].Body) != "hi" {
		t.Fatalf("text message: %+v", chat.msgs[1])
	}
}

// No account link (sender or recipient) and a core that is down are NEX errors; nothing is kept.
func TestProcessMessageFailures(t *testing.T) {
	s, chat := newStore(100)
	mm := &common_globals.MessagingManager{}
	if _, _, _, e := s.ProcessMessage(mm, binary(100, "", []byte{1}, 0), types.List[types.UInt64]{999}, 1, true); e == nil || e.ResultCode != nex.NewError(nex.ResultCodes.Core.InvalidArgument, "").ResultCode {
		t.Fatalf("unlinked recipient: %v", e)
	}
	if _, _, _, e := s.ProcessMessage(mm, binary(999, "", []byte{1}, 0), types.List[types.UInt64]{100}, 1, true); e == nil {
		t.Fatal("unlinked sender accepted")
	}
	s2, _ := newStore(100, 200)
	s2.Chat.(*fakeChat).down = true
	if _, _, _, e := s2.ProcessMessage(mm, binary(100, "", []byte{1}, 0), types.List[types.UInt64]{200}, 1, true); e == nil || e.ResultCode != nex.NewError(nex.ResultCodes.Core.SystemError, "").ResultCode {
		t.Fatalf("core down: %v", e)
	}
	if _, _, _, e := s.ProcessMessage(mm, binary(100, "", []byte{1}, 0), types.List[types.UInt64]{1}, 3, true); e == nil {
		t.Fatal("recipient type 3 accepted")
	}
	if len(chat.msgs) != 0 {
		t.Fatalf("a failed send kept something: %+v", chat.msgs)
	}
}

// The Messaging (0x17) store calls: inbox page, count, ids inside the inbox, delete, rooms.
func TestInboxReadsAndDeletes(t *testing.T) {
	s, _ := newStore(100, 200, 300)
	h := messaging_types.NewUserMessage()
	h.PIDSender, h.StrSubject, h.UILifeTime = 100, "s", 60
	for i := 0; i < 3; i++ {
		if _, err := s.Send(h, Body{Bytes: []byte{byte(i)}}, 200, 1); err != nil {
			t.Fatal(err)
		}
	}
	other, err := s.Send(h, Body{Bytes: []byte{9}}, 300, 1)
	if err != nil {
		t.Fatal(err)
	}
	room, err := s.Send(h, Body{Text: true, Bytes: []byte("all")}, 77, 2)
	if err != nil {
		t.Fatal(err)
	}

	page, total, err := s.List(200, 1, 1, 5, nil, false)
	if err != nil || total != 3 || len(page) != 2 || page[0].Body.Bytes[0] != 1 {
		t.Fatalf("page: %v %d %+v", err, total, page)
	}
	if page[0].Header.PIDSender != 100 || page[0].Header.StrSubject != "s" || page[0].Header.UILifeTime != 60 {
		t.Fatalf("header: %+v", page[0].Header)
	}
	byID, _, _ := s.List(200, 1, 0, 0, []uint32{uint32(page[0].Header.UIID), other}, false)
	if len(byID) != 1 {
		t.Fatalf("another inbox's id came back: %+v", byID)
	}
	rs, rt, _ := s.List(77, 2, 0, 10, nil, false)
	if rt != 1 || !rs[0].Body.Text || string(rs[0].Body.Bytes) != "all" || uint32(rs[0].Header.UIID) != room {
		t.Fatalf("room inbox: %+v", rs)
	}
	if n, err := s.Delete(200, 1, []uint32{other}, false); err != nil || n != 0 {
		t.Fatalf("deleting another inbox's id: %d %v", n, err)
	}
	if n, err := s.Delete(200, 1, nil, true); err != nil || n != 3 {
		t.Fatalf("delete all: %d %v", n, err)
	}
	if _, total, _ := s.List(300, 1, 0, 10, nil, true); total != 1 {
		t.Fatal("another inbox was touched")
	}
}

// A stored message turns back into the NEX holder of its kind, recipient fields per library.
func TestHolder(t *testing.T) {
	st := Stored{Header: messaging_types.NewUserMessage(), Body: Body{Text: true, Bytes: []byte("hey")}}
	st.Header.UIID = 5
	d := holder(st, nex.NewLibraryVersion(3, 4, 0), 200, 1)
	tm, ok := d.Object.(messaging_types.TextMessage)
	if !ok || string(tm.StrTextBody) != "hey" || tm.IDRecipient != 200 || tm.UIRecipientType != 1 {
		t.Fatalf("3.x text holder: %#v", d.Object)
	}
	st.Body = Body{Bytes: []byte{7}}
	d = holder(st, nex.NewLibraryVersion(4, 0, 0), 77, 2)
	bm, ok := d.Object.(messaging_types.BinaryMessage)
	if !ok || bm.MessageRecipient.GatheringID != 77 || bm.MessageRecipient.UIRecipientType != 2 || bm.BinaryBody[0] != 7 {
		t.Fatalf("4.x binary holder: %#v", d.Object)
	}
}
