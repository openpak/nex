package coreranking

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/openpak/nex/internal/coreid"
	lbpb "github.com/openpak/nex/internal/leaderboardspb"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// fakeLB is an in-memory leaderboards service with the real one's rules that matter here:
// best aggregation, competition rank and unique position, per-network boards, created on the
// first Submit with its create_policy.
type fakeLB struct {
	lbpb.LeaderboardsClient
	mu      sync.Mutex
	boards  map[string]*lbpb.Policy           // title/board
	entries map[string]map[string]*lbpb.Entry // title/board -> account
	friends map[string][]string               // account -> friends
	titles  map[string]*lbpb.MapTitleRequest  // network/native
	down    bool
	calls   map[string]int
	clock   int64
}

func newFakeLB() *fakeLB {
	return &fakeLB{boards: map[string]*lbpb.Policy{}, entries: map[string]map[string]*lbpb.Entry{},
		friends: map[string][]string{}, titles: map[string]*lbpb.MapTitleRequest{}, calls: map[string]int{}}
}

func (f *fakeLB) hit(m string) error {
	f.calls[m]++
	if f.down {
		return status.Error(codes.Unavailable, "down")
	}
	return nil
}

func key(r *lbpb.BoardRef) string { return r.GetTitle() + "/" + r.GetBoard() }

func (f *fakeLB) MapTitle(_ context.Context, r *lbpb.MapTitleRequest, _ ...grpc.CallOption) (*lbpb.MapTitleResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("MapTitle"); err != nil {
		return nil, err
	}
	f.titles[r.GetNetwork()+"/"+r.GetNativeId()] = r
	return &lbpb.MapTitleResponse{Title: r.GetTitle()}, nil
}

func (f *fakeLB) Submit(_ context.Context, r *lbpb.SubmitRequest, _ ...grpc.CallOption) (*lbpb.SubmitResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("Submit"); err != nil {
		return nil, err
	}
	k := key(r.GetRef())
	p, ok := f.boards[k]
	if !ok {
		p = r.GetCreatePolicy()
		f.boards[k] = p
		f.entries[k] = map[string]*lbpb.Entry{}
	}
	f.clock++
	cur, had := f.entries[k][r.GetAccount()]
	asc := p.GetOrder() == "asc"
	better := !had || r.GetAggregationOverride() == "latest" ||
		(asc && r.GetScore() < cur.GetScore()) || (!asc && r.GetScore() > cur.GetScore())
	if better {
		f.entries[k][r.GetAccount()] = &lbpb.Entry{Account: r.GetAccount(), Score: r.GetScore(), Info: r.GetInfo(), Groups: r.GetGroups(),
			Network: r.GetRef().GetNetwork(), UpdatedUnixMs: time.Unix(1700000000+f.clock, 0).UnixMilli()}
	}
	return &lbpb.SubmitResponse{Changed: better}, nil
}

func (f *fakeLB) Delete(_ context.Context, r *lbpb.DeleteRequest, _ ...grpc.CallOption) (*lbpb.DeleteResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("Delete"); err != nil {
		return nil, err
	}
	es, ok := f.entries[key(r.GetRef())]
	if !ok {
		return nil, status.Error(codes.NotFound, "no board")
	}
	if _, ok := es[r.GetAccount()]; ok {
		delete(es, r.GetAccount())
		return &lbpb.DeleteResponse{Deleted: 1}, nil
	}
	return &lbpb.DeleteResponse{}, nil
}

// ranked is a board best first with rank and position filled in.
func (f *fakeLB) ranked(ref *lbpb.BoardRef, gf *lbpb.GroupsFilter) ([]*lbpb.Entry, error) {
	k := key(ref)
	p, ok := f.boards[k]
	if !ok {
		return nil, status.Error(codes.NotFound, "no board")
	}
	var out []*lbpb.Entry
	for _, e := range f.entries[k] {
		if gf != nil && gf.GetGroupIndex() >= 0 {
			g := e.GetGroups()
			if int(gf.GetGroupIndex()) >= len(g) || uint32(g[gf.GetGroupIndex()]) != gf.GetGroupValue() {
				continue
			}
		}
		out = append(out, proto.Clone(e).(*lbpb.Entry))
	}
	asc := p.GetOrder() == "asc"
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return (out[i].Score < out[j].Score) == asc
		}
		if out[i].UpdatedUnixMs != out[j].UpdatedUnixMs {
			return out[i].UpdatedUnixMs < out[j].UpdatedUnixMs
		}
		return out[i].Account < out[j].Account
	})
	for i, e := range out {
		e.Position = int64(i + 1)
		e.Rank = int64(i + 1)
		if i > 0 && e.Score == out[i-1].Score {
			e.Rank = out[i-1].Rank
		}
	}
	return out, nil
}

func (f *fakeLB) Range(_ context.Context, r *lbpb.RangeRequest, _ ...grpc.CallOption) (*lbpb.RangeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("Range"); err != nil {
		return nil, err
	}
	all, err := f.ranked(r.GetRef(), r.GetGroupsFilter())
	if err != nil {
		return nil, err
	}
	lim := r.GetLimit()
	if lim == 0 {
		lim = 25
	}
	lo := min(max(r.GetStart()-1, 0), int64(len(all)))
	hi := min(lo+lim, int64(len(all)))
	return &lbpb.RangeResponse{Entries: all[lo:hi], Total: int64(len(all))}, nil
}

func (f *fakeLB) ByAccounts(_ context.Context, r *lbpb.ByAccountsRequest, _ ...grpc.CallOption) (*lbpb.RangeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("ByAccounts"); err != nil {
		return nil, err
	}
	return f.byAccounts(r.GetRef(), r.GetAccounts(), r.GetGroupsFilter())
}

func (f *fakeLB) byAccounts(ref *lbpb.BoardRef, accounts []string, gf *lbpb.GroupsFilter) (*lbpb.RangeResponse, error) {
	all, err := f.ranked(ref, gf)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, a := range accounts {
		want[a] = true
	}
	var out []*lbpb.Entry
	for _, e := range all {
		if want[e.Account] {
			out = append(out, e)
		}
	}
	return &lbpb.RangeResponse{Entries: out, Total: int64(len(all))}, nil
}

func (f *fakeLB) FriendRanks(_ context.Context, r *lbpb.FriendRanksRequest, _ ...grpc.CallOption) (*lbpb.RangeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("FriendRanks"); err != nil {
		return nil, err
	}
	accts := append([]string(nil), f.friends[r.GetAccount()]...)
	if r.GetIncludeSelf() {
		accts = append(accts, r.GetAccount())
	}
	resp, err := f.byAccounts(r.GetRef(), accts, r.GetGroupsFilter())
	if err != nil {
		return nil, err
	}
	resp.Hits = int64(len(resp.Entries))
	return resp, nil
}

func (f *fakeLB) Total(_ context.Context, r *lbpb.TotalRequest, _ ...grpc.CallOption) (*lbpb.TotalResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all, err := f.ranked(r.GetRef(), r.GetGroupsFilter())
	if err != nil {
		return nil, err
	}
	return &lbpb.TotalResponse{Total: int64(len(all))}, nil
}

func (f *fakeLB) ListBoards(_ context.Context, r *lbpb.ListBoardsRequest, _ ...grpc.CallOption) (*lbpb.ListBoardsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &lbpb.ListBoardsResponse{}
	for k, p := range f.boards {
		t, b, _ := strings.Cut(k, "/")
		if t == r.GetTitle() {
			out.Boards = append(out.Boards, &lbpb.Board{Title: t, Id: b, Policy: p})
		}
	}
	return out, nil
}

// fakeTS is an in-memory title-storage with the object calls the ranking store uses.
type fakeTS struct {
	tspb.TitleStorageClient
	mu      sync.Mutex
	objects map[string]*tspb.Object // owner/collection/name
	titles  map[string]bool
	down    bool
}

func newFakeTS() *fakeTS {
	return &fakeTS{objects: map[string]*tspb.Object{}, titles: map[string]bool{}}
}

func (f *fakeTS) MapTitle(_ context.Context, r *tspb.MapTitleRequest, _ ...grpc.CallOption) (*tspb.MapTitleResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.titles[r.GetNetwork()+"/"+r.GetNativeId()] = true
	return &tspb.MapTitleResponse{}, nil
}

func (f *fakeTS) UpsertTitle(_ context.Context, r *tspb.UpsertTitleRequest, _ ...grpc.CallOption) (*tspb.TitleInfo, error) {
	return &tspb.TitleInfo{Title: r.GetTitle(), CrossNetwork: r.GetCrossNetwork() == tspb.Tristate_TRISTATE_TRUE}, nil
}

func (f *fakeTS) PutObject(_ context.Context, r *tspb.PutObjectRequest, _ ...grpc.CallOption) (*tspb.ObjectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, status.Error(codes.Unavailable, "down")
	}
	o := &tspb.Object{Owner: r.GetOwner(), Collection: r.GetCollection(), Name: r.GetName(), Body: r.GetBody(), Visibility: r.GetVisibility()}
	f.objects[r.GetOwner()+"/"+r.GetCollection()+"/"+r.GetName()] = o
	return &tspb.ObjectResponse{Object: o, Found: true}, nil
}

func (f *fakeTS) GetObject(_ context.Context, r *tspb.GetObjectRequest, _ ...grpc.CallOption) (*tspb.ObjectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.objects {
		if (r.GetOwner() == "" || o.Owner == r.GetOwner()) && o.Collection == r.GetCollection() && o.Name == r.GetName() {
			return &tspb.ObjectResponse{Object: o, Found: true}, nil
		}
	}
	return &tspb.ObjectResponse{}, nil
}

func (f *fakeTS) ListObjects(_ context.Context, r *tspb.ListObjectsRequest, _ ...grpc.CallOption) (*tspb.ListObjectsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	owners := map[string]bool{}
	for _, o := range r.GetOwners() {
		owners[o] = true
	}
	out := &tspb.ListObjectsResponse{}
	for _, o := range f.objects {
		if owners[o.Owner] && o.Collection == r.GetCollection() {
			out.Objects = append(out.Objects, o)
		}
	}
	return out, nil
}

// fakeIDs maps PID n to account "acct-n" for the PIDs it knows.
type fakeIDs struct {
	mu    sync.Mutex
	known map[uint64]bool
}

func (f *fakeIDs) Account(_ context.Context, pid uint64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.known[pid] {
		return "", coreid.ErrNoLink
	}
	return acct(pid), nil
}

func (f *fakeIDs) PID(_ context.Context, account string) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for p := range f.known {
		if acct(p) == account {
			return p, nil
		}
	}
	return 0, coreid.ErrNoLink
}

func acct(pid uint64) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", pid) }
