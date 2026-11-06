package coremirror

import (
	"context"
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
