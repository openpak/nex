package coremirror

import (
	"context"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/openpak/nex/internal/coreid"
	mm "github.com/openpak/nex/internal/matchmakingpb"
)

type fakeMM struct {
	mm.MatchmakingClient
	creates []*mm.CreateSessionRequest
	joins   []*mm.JoinSessionRequest
	leaves  []*mm.LeaveSessionRequest
	updates []*mm.UpdateSessionRequest
	beats   []string
}

func (f *fakeMM) CreateOrJoinSession(_ context.Context, r *mm.CreateSessionRequest, _ ...grpc.CallOption) (*mm.SessionResponse, error) {
	f.creates = append(f.creates, r)
	return &mm.SessionResponse{Session: &mm.Session{Id: "s-" + r.UniqueKey}}, nil
}
func (f *fakeMM) JoinSession(_ context.Context, r *mm.JoinSessionRequest, _ ...grpc.CallOption) (*mm.SessionResponse, error) {
	f.joins = append(f.joins, r)
	return &mm.SessionResponse{}, nil
}
func (f *fakeMM) LeaveSession(_ context.Context, r *mm.LeaveSessionRequest, _ ...grpc.CallOption) (*mm.LeaveResponse, error) {
	f.leaves = append(f.leaves, r)
	return &mm.LeaveResponse{Left: true}, nil
}
func (f *fakeMM) UpdateSession(_ context.Context, r *mm.UpdateSessionRequest, _ ...grpc.CallOption) (*mm.SessionResponse, error) {
	f.updates = append(f.updates, r)
	return &mm.SessionResponse{}, nil
}
func (f *fakeMM) Heartbeat(_ context.Context, r *mm.HeartbeatRequest, _ ...grpc.CallOption) (*mm.HeartbeatResponse, error) {
	f.beats = append(f.beats, r.AccountId)
	return &mm.HeartbeatResponse{}, nil
}

type ids map[uint64]string

func (i ids) Account(_ context.Context, pid uint64) (string, error) {
	if a, ok := i[pid]; ok {
		return a, nil
	}
	return "", coreid.ErrNoLink
}
func (i ids) PID(context.Context, string) (uint64, error) { return 0, coreid.ErrNoLink }

func newMirror(f *fakeMM, links ids) *Mirror {
	return &Mirror{MM: f, IDs: links, Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4", Console: "wiiu"}
}

func attr(r *mm.CreateSessionRequest, name string) *mm.Attr {
	for _, a := range r.Attrs {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func TestCreateCarriesGroupAndAttributes(t *testing.T) {
	f := &fakeMM{}
	m := newMirror(f, ids{1: "acc-1"})
	m.Tick(context.Background(), []Gathering{{ID: 7, Owner: 1, Participants: []uint64{1}, Max: 8, GameMode: 1, Attribs: []uint32{0, 1, 0, 0, 0, 0}, AppBuffer: []byte{1, 2, 3}, Open: true}})
	if len(f.creates) != 1 {
		t.Fatalf("creates = %d", len(f.creates))
	}
	c := f.creates[0]
	if c.Title.Title != "terraria" || c.Title.Network != "wiiu" || c.Creator != "acc-1" || c.UniqueKey != "g:7" || c.MaxMembers != 8 || c.Kind != "room" || c.Closed {
		t.Fatalf("create = %+v", c)
	}
	if a := attr(c, "compat"); a == nil || a.StringValue != "old-gen-1.2.4" || !a.Searchable {
		t.Fatalf("compat attr = %+v", a)
	}
	if a := attr(c, "attr1"); a == nil || a.IntValue != 1 || !a.Searchable {
		t.Fatalf("attr1 = %+v", a)
	}
	if string(c.Data) != "\x01\x02\x03" {
		t.Fatalf("data = %x", c.Data)
	}
}

func TestIdempotentAndMembership(t *testing.T) {
	f := &fakeMM{}
	m := newMirror(f, ids{1: "a1", 2: "a2"})
	g := Gathering{ID: 7, Owner: 1, Participants: []uint64{1}, Max: 8, Open: true}
	m.Tick(context.Background(), []Gathering{g})
	m.Tick(context.Background(), []Gathering{g})
	if len(f.creates) != 1 || len(f.joins) != 0 {
		t.Fatalf("creates=%d joins=%d", len(f.creates), len(f.joins))
	}
	g.Participants = []uint64{1, 2}
	m.Tick(context.Background(), []Gathering{g})
	if len(f.joins) != 1 || f.joins[0].AccountId != "a2" || f.joins[0].SessionId != "s-g:7" || !f.joins[0].AllowClosed {
		t.Fatalf("joins = %+v", f.joins)
	}
	g.Participants = []uint64{1}
	m.Tick(context.Background(), []Gathering{g})
	if len(f.leaves) != 1 || f.leaves[0].AccountId != "a2" {
		t.Fatalf("leaves = %+v", f.leaves)
	}
}

func TestGatheringGoneEndsSession(t *testing.T) {
	f := &fakeMM{}
	m := newMirror(f, ids{1: "a1"})
	m.Tick(context.Background(), []Gathering{{ID: 7, Owner: 1, Participants: []uint64{1}, Max: 8, Open: true}})
	m.Tick(context.Background(), nil)
	if len(f.leaves) != 1 || f.leaves[0].AccountId != "a1" || f.leaves[0].SessionId != "s-g:7" {
		t.Fatalf("leaves = %+v", f.leaves)
	}
	m.Tick(context.Background(), nil)
	if len(f.leaves) != 1 {
		t.Fatal("ended twice")
	}
}

func TestChangedAttributesUpdate(t *testing.T) {
	f := &fakeMM{}
	m := newMirror(f, ids{1: "a1"})
	g := Gathering{ID: 7, Owner: 1, Participants: []uint64{1}, Max: 8, Attribs: []uint32{0, 1}, Open: true}
	m.Tick(context.Background(), []Gathering{g})
	g.Open = false
	m.Tick(context.Background(), []Gathering{g})
	if len(f.updates) != 1 || !f.updates[0].SetOpen || f.updates[0].Open || f.updates[0].Actor != "a1" {
		t.Fatalf("updates = %+v", f.updates)
	}
}

func TestUnlinkedOwnerSkippedThenPicksUp(t *testing.T) {
	f := &fakeMM{}
	links := ids{}
	m := newMirror(f, links)
	g := Gathering{ID: 7, Owner: 1, Participants: []uint64{1}, Max: 8, Open: true}
	m.Tick(context.Background(), []Gathering{g})
	if len(f.creates) != 0 {
		t.Fatal("created without a link")
	}
	links[1] = "a1"
	m.Tick(context.Background(), []Gathering{g})
	if len(f.creates) != 1 {
		t.Fatal("not created after link")
	}
}

func TestHeartbeatsEveryMinute(t *testing.T) {
	f := &fakeMM{}
	now := time.Unix(1000, 0)
	m := newMirror(f, ids{1: "a1"})
	m.Now = func() time.Time { return now }
	g := []Gathering{{ID: 7, Owner: 1, Participants: []uint64{1}, Max: 8, Open: true}}
	m.Tick(context.Background(), g)
	now = now.Add(10 * time.Second)
	m.Tick(context.Background(), g)
	now = now.Add(61 * time.Second)
	m.Tick(context.Background(), g)
	if len(f.beats) != 2 {
		t.Fatalf("beats = %d (%s)", len(f.beats), strconv.Itoa(len(f.beats)))
	}
}
