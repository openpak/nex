package coremirror

import (
	"context"
	"google.golang.org/grpc"
	"strings"
	"testing"

	"github.com/openpak/nex/internal/coreid"
	mm "github.com/openpak/nex/internal/matchmakingpb"
)

// pids resolves accounts to PIDs (the reverse of ids).
type pids map[string]uint64

func (p pids) Account(context.Context, uint64) (string, error) { return "", coreid.ErrNoLink }
func (p pids) PID(_ context.Context, a string) (uint64, error) {
	if n, ok := p[a]; ok {
		return n, nil
	}
	return 0, coreid.ErrNoLink
}

func ps3Room(id, owner string) *mm.Session {
	return &mm.Session{Id: id, Title: &mm.TitleRef{Network: "np", Title: "terraria"}, OwnerId: owner, MaxMembers: 8, Open: true,
		Attrs: []*mm.Attr{{Name: "$members", Type: mm.AttrType_ATTR_TYPE_INT, IntValue: 1}}}
}

func TestForeignOffByDefault(t *testing.T) {
	f := &fakeMM{}
	m := &Mirror{MM: f, IDs: pids{}, Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4"}
	if r, err := m.Foreign(context.Background()); r != nil || err != nil || len(f.searches) != 0 {
		t.Fatal("no cross networks must mean no search and no rooms")
	}
}

func TestForeignListsGroupRoomsWithResolvedOwners(t *testing.T) {
	f := &fakeMM{found: []*mm.Session{ps3Room("s-1", "acc-ps3"), ps3Room("s-2", "acc-nolink")}}
	m := &Mirror{MM: f, IDs: pids{"acc-ps3": 1234}, Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4", Cross: []string{"np"}}
	rooms, err := m.Foreign(context.Background())
	if err != nil || len(rooms) != 1 {
		t.Fatalf("rooms %+v %v", rooms, err)
	}
	r := rooms[0]
	if r.Session != "s-1" || r.Network != "np" || r.OwnerPID != 1234 || r.Max != 8 || r.Members != 1 || !r.Open {
		t.Fatalf("room %+v", r)
	}
	q := f.searches[0]
	if len(q.Networks) != 1 || q.Networks[0] != "np" || q.Network != "wiiu" || len(q.Filters) != 1 || q.Filters[0].StringValue != "old-gen-1.2.4" {
		t.Fatalf("search %+v", q)
	}
}

func TestCandidatesIncludeShadows(t *testing.T) {
	f := &fakeMM{found: []*mm.Session{{Id: "s-own", UniqueKey: "g:7"}, {Id: "s-ps3"}, {Id: "s-unknown"}}}
	m := &Mirror{MM: f, IDs: pids{}, Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4"}
	m.SetShadows(map[string]uint32{"s-ps3": 99})
	got := m.Candidates(context.Background())
	if len(got) != 2 || got[0] != 7 || got[1] != 99 {
		t.Fatalf("candidates %v", got)
	}
}

// The mirrored session carries no identity attribute (owner rule: names come from the account core at read time).
func TestMirrorAttrsCarryNoIdentity(t *testing.T) {
	m := &Mirror{Compat: "old-gen-1.2.4", Console: "wiiu"}
	for _, a := range m.attrs(Gathering{Attribs: []uint32{0, 1, 0, 0, 0, 0}}) {
		n := strings.ToLower(a.Name)
		for _, bad := range []string{"name", "nick", "mii", "pnid", "handle", "email", "country", "owner"} {
			if strings.Contains(n, bad) {
				t.Fatalf("mirrored attribute %q looks like identity", a.Name)
			}
		}
	}
}

type foreignMM struct {
	*fakeMM
	members []*mm.Member
}

func (f *foreignMM) GetSession(context.Context, *mm.GetSessionRequest, ...grpc.CallOption) (*mm.SessionResponse, error) {
	return &mm.SessionResponse{Session: &mm.Session{Members: f.members}}, nil
}

// A PS3 player who joined the Wii U room in the core (through the relay) becomes a participant once, and leaves once.
func TestForeignMembersInjectedOnce(t *testing.T) {
	f := &foreignMM{fakeMM: &fakeMM{}}
	type ev struct{ gid, pid uint64 }
	var joins, leaves []ev
	both := idsBoth{acc: map[uint64]string{1: "acc-wiiu"}, pid: map[string]uint64{"acc-ps3": 77}}
	m := &Mirror{MM: f, IDs: both, Title: "terraria", Network: "wiiu", Compat: "old-gen-1.2.4",
		OnForeignJoin:  func(g, p uint64) { joins = append(joins, ev{g, p}) },
		OnForeignLeave: func(g, p uint64) { leaves = append(leaves, ev{g, p}) }}
	g := Gathering{ID: 5, Owner: 1, Participants: []uint64{1}, Max: 8, Open: true}
	f.members = []*mm.Member{{AccountId: "acc-wiiu", Network: "wiiu"}, {AccountId: "acc-ps3", Network: "np"}}
	m.Tick(context.Background(), []Gathering{g})
	g.Participants = []uint64{1, 77} // the title added it
	m.Tick(context.Background(), []Gathering{g})
	if len(joins) != 1 || joins[0] != (ev{5, 77}) {
		t.Fatalf("joins %v", joins)
	}
	for _, j := range f.joins {
		if j.AccountId == "acc-ps3" {
			t.Fatal("an injected PID must not be mirrored back as a wiiu member")
		}
	}
	f.members = f.members[:1]
	m.Tick(context.Background(), []Gathering{g})
	if len(leaves) != 1 || leaves[0] != (ev{5, 77}) {
		t.Fatalf("leaves %v", leaves)
	}
}

type idsBoth struct {
	acc map[uint64]string
	pid map[string]uint64
}

func (i idsBoth) Account(_ context.Context, p uint64) (string, error) {
	if a, ok := i.acc[p]; ok {
		return a, nil
	}
	return "", coreid.ErrNoLink
}
func (i idsBoth) PID(_ context.Context, a string) (uint64, error) {
	if p, ok := i.pid[a]; ok {
		return p, nil
	}
	return 0, coreid.ErrNoLink
}
