package coredatastore

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	dstypes "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"

	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

func newStore() (*Store, *fakeTS, *fakeIDs) {
	ts, ids := newFakeTS(), &fakeIDs{}
	s := &Store{Cfg: Config{Title: "mario-kart-8", Name: "Mario Kart 8", Network: "wiiu", MaxObjectBytes: 8 << 20}, TS: ts, IDs: ids,
		Friends: func(pid uint32) []uint32 {
			if pid == 1 {
				return []uint32{2}
			}
			if pid == 2 {
				return []uint32{1}
			}
			return nil
		}}
	ts.friends[acct(1)] = []string{acct(2)}
	ts.friends[acct(2)] = []string{acct(1)}
	return s, ts, ids
}

func TestRoundTripAgainstFakeCore(t *testing.T) {
	s, _, _ := newStore()
	roundTrip(t, s, 1, 3, 17)
}

func TestRegisterMapsTitlePerNetworkWithCap(t *testing.T) {
	s, ts, _ := newStore()
	if err := s.Register(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ts.mapped["wiiu/mario-kart-8"] != "mario-kart-8" {
		t.Fatalf("mapped = %v", ts.mapped)
	}
	u := ts.titles["mario-kart-8"]
	if u.CrossNetwork != tspb.Tristate_TRISTATE_FALSE || u.MaxObjectBytes != 8<<20 {
		t.Fatalf("upsert = %+v", u)
	}
}

func TestPermissionsAreCoreVisibility(t *testing.T) {
	s, ts, _ := newStore()
	cases := []struct {
		perm        constants.Permission
		recipients  []uint64
		vis         tspb.Visibility
		withFriends bool
		readers     map[uint64]bool // 2 is a friend of 1, 3 and 4 are not
	}{
		{constants.PermissionPublic, nil, tspb.Visibility_VISIBILITY_PUBLIC, false, map[uint64]bool{2: true, 3: true, 500: true}},
		{constants.PermissionFriend, nil, tspb.Visibility_VISIBILITY_FRIENDS, false, map[uint64]bool{2: true, 3: false}},
		{constants.PermissionSpecified, []uint64{3, 500}, tspb.Visibility_VISIBILITY_RECIPIENTS, false, map[uint64]bool{2: false, 3: true, 4: false}},
		{constants.PermissionPrivate, nil, tspb.Visibility_VISIBILITY_PRIVATE, false, map[uint64]bool{2: false, 3: false}},
		{constants.PermissionSpecifiedFriend, []uint64{3}, tspb.Visibility_VISIBILITY_RECIPIENTS, true, map[uint64]bool{2: true, 3: true, 4: false}},
	}
	for _, c := range cases {
		p := postParam(10, 5, c.perm, noPersistenceSlot)
		for _, r := range c.recipients {
			p.Permission.RecipientIDs = append(p.Permission.RecipientIDs, types.NewPID(r))
		}
		id := post(t, s, 1, p, make([]byte, 10))
		o := ts.objs[id].o
		if o.Visibility != c.vis || o.RecipientsIncludeFriends != c.withFriends {
			t.Fatalf("perm %d: visibility %v friends %v", c.perm, o.Visibility, o.RecipientsIncludeFriends)
		}
		// An unlinked recipient (500) is left out of the core list but echoed on the wire.
		if c.perm == constants.PermissionSpecified && (len(o.Recipients) != 1 || o.Recipients[0] != acct(3)) {
			t.Fatalf("recipients = %v", o.Recipients)
		}
		mi, ne := s.GetMeta(1, byID(id))
		if ne != nil || len(mi.Permission.RecipientIDs) != len(c.recipients) {
			t.Fatalf("owner read: %v, recipients %v", ne, mi.Permission.RecipientIDs)
		}
		for reader, want := range c.readers {
			_, ne := s.GetMeta(reader, byID(id))
			if (ne == nil) != want {
				t.Fatalf("perm %d reader %d: %v, want readable %v", c.perm, reader, ne, want)
			}
		}
	}
}

func TestNoLinkAndCoreDown(t *testing.T) {
	s, ts, ids := newStore()
	id := post(t, s, 1, postParam(4, 5, constants.PermissionPublic, noPersistenceSlot), []byte("abcd"))
	// A player with no link reads public objects anonymously, but cannot write.
	if _, ne := s.GetMeta(500, byID(id)); ne != nil {
		t.Fatalf("anonymous public read: %v", ne)
	}
	if _, ne := s.PreparePost(500, postParam(4, 5, constants.PermissionPublic, noPersistenceSlot)); code(ne) != nex.ResultCodes.DataStore.PermissionDenied {
		t.Fatalf("unlinked post = %v", ne)
	}
	if _, ne := s.Rate(500, target(id, 0), rateParam(1)); code(ne) != nex.ResultCodes.DataStore.PermissionDenied {
		t.Fatalf("unlinked rate = %v", ne)
	}
	ts.down = true
	if _, ne := s.GetMeta(1, byID(id)); code(ne) != nex.ResultCodes.Core.SystemError {
		t.Fatalf("core down read = %v", ne)
	}
	if _, ne := s.PreparePost(1, postParam(4, 5, constants.PermissionPublic, noPersistenceSlot)); code(ne) != nex.ResultCodes.Core.SystemError {
		t.Fatalf("core down post = %v", ne)
	}
	ts.down, ids.down = false, true
	if _, ne := s.GetMeta(1, byID(id)); code(ne) != nex.ResultCodes.Core.SystemError {
		t.Fatalf("resolution down = %v", ne)
	}
}

func TestUploadMismatchAndFailedUpload(t *testing.T) {
	s, ts, _ := newStore()
	info, ne := s.PreparePost(1, postParam(10, 5, constants.PermissionPublic, noPersistenceSlot))
	if ne != nil {
		t.Fatal(ne)
	}
	if err := upload(string(info.URL), fields(info), []byte("short")); err != nil {
		t.Fatal(err)
	}
	if ne := s.CompletePost(1, complete(uint64(info.DataID))); code(ne) != nex.ResultCodes.DataStore.OperationNotAllowed {
		t.Fatalf("size mismatch = %v", ne)
	}
	if _, ne := s.GetMeta(1, byID(uint64(info.DataID))); code(ne) != nex.ResultCodes.DataStore.NotFound {
		t.Fatalf("incomplete object visible: %v", ne)
	}
	p := complete(uint64(info.DataID))
	p.IsSuccess = false
	if ne := s.CompletePost(1, p); ne != nil || ts.calls["CompleteUpload"] != 1 {
		t.Fatalf("failed upload: %v, CompleteUpload calls %d", ne, ts.calls["CompleteUpload"])
	}
}

func TestPersistenceSlotHoldsOneObject(t *testing.T) {
	s, ts, _ := newStore()
	first := post(t, s, 1, postParam(3, 5, constants.PermissionPublic, 2), []byte("one"))
	second := post(t, s, 1, postParam(3, 6, constants.PermissionPublic, 2), []byte("two"))
	if _, ok := ts.objs[first]; ok {
		t.Fatal("the slot's previous object was not retired")
	}
	g := dstypes.NewDataStoreGetMetaParam()
	g.PersistenceTarget.OwnerID, g.PersistenceTarget.PersistenceSlotID = types.NewPID(1), types.NewUInt16(2)
	mi, ne := s.GetMeta(3, g)
	if ne != nil || uint64(mi.DataID) != second {
		t.Fatalf("by persistence target = %d, %v", mi.DataID, ne)
	}
	gp := dstypes.NewDataStorePrepareGetParam()
	gp.PersistenceTarget = g.PersistenceTarget
	gi, ne := s.PrepareGet(3, gp)
	if ne != nil {
		t.Fatal(ne)
	}
	if b, err := download(string(gi.URL)); err != nil || string(b) != "two" {
		t.Fatalf("body = %q %v", b, err)
	}
}

func TestPostMetaBinaryIsVisibleAtOnce(t *testing.T) {
	s, _, _ := newStore()
	id, ne := s.PostMetaBinary(1, postParam(0, 5, constants.PermissionPublic, noPersistenceSlot))
	if ne != nil {
		t.Fatal(ne)
	}
	mi, ne := s.GetMeta(2, byID(id))
	if ne != nil || len(mi.Ratings) != 1 || uint32(mi.Size) != 0 {
		t.Fatalf("meta-only object: %v ratings %v", ne, mi.Ratings)
	}
	if _, ne := s.PrepareGet(2, getParam(id)); code(ne) != nex.ResultCodes.DataStore.NotFound {
		t.Fatalf("PrepareGet of a body-less object = %v", ne)
	}
}

func TestChangeAndDeleteNeedTheUpdatePermission(t *testing.T) {
	s, ts, _ := newStore()
	id := post(t, s, 1, postParam(2, 5, constants.PermissionPublic, noPersistenceSlot), []byte("ab"))
	cm := dstypes.NewDataStoreChangeMetaParam()
	cm.DataID = types.NewUInt64(id)
	cm.ModifiesFlag = constants.ModificationFlagPeriod | constants.ModificationFlagAccessPermission | constants.ModificationFlagName
	cm.Period = types.NewUInt16(7)
	cm.Name = types.NewString("renamed")
	cm.Permission = permission(uint8(constants.PermissionFriend), nil)
	if ne := s.ChangeMeta(3, cm); code(ne) != nex.ResultCodes.DataStore.PermissionDenied {
		t.Fatalf("stranger ChangeMeta = %v", ne)
	}
	if ne := s.ChangeMeta(1, cm); ne != nil {
		t.Fatal(ne)
	}
	o := ts.objs[id].o
	var m meta
	_ = json.Unmarshal([]byte(o.MetaJson), &m)
	if o.Visibility != tspb.Visibility_VISIBILITY_FRIENDS || m.Period != 7 || m.Name != "renamed" || m.Permission != 1 || ts.objs[id].key == "" {
		t.Fatalf("after change: vis %v meta %+v", o.Visibility, m)
	}
	dp := dstypes.NewDataStoreDeleteParam()
	dp.DataID = types.NewUInt64(id)
	if ne := s.Delete(2, dp); code(ne) != nex.ResultCodes.DataStore.PermissionDenied {
		t.Fatalf("friend delete of a private-delete object = %v", ne)
	}
	if ne := s.Delete(1, dp); ne != nil {
		t.Fatal(ne)
	}
}

func search(t *testing.T, s *Store, caller uint64, edit func(*dstypes.DataStoreSearchParam)) []uint64 {
	t.Helper()
	p := dstypes.NewDataStoreSearchParam()
	p.ResultOption = constants.ResultFlag(0xFF)
	edit(&p)
	found, _, ne := s.Search(caller, p)
	if ne != nil {
		t.Fatal(ne)
	}
	var ids []uint64
	for _, f := range found {
		ids = append(ids, uint64(f.DataID))
	}
	return ids
}

func same(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSearchTranslation(t *testing.T) {
	s, ts, _ := newStore()
	a := post(t, s, 1, postParam(1, 5, constants.PermissionPublic, noPersistenceSlot, "x"), []byte("a"))
	b := post(t, s, 2, postParam(1, 5, constants.PermissionPublic, noPersistenceSlot), []byte("b"))
	c := post(t, s, 3, postParam(1, 6, constants.PermissionPublic, noPersistenceSlot, "x"), []byte("c"))
	d := post(t, s, 3, postParam(1, 6, constants.PermissionPrivate, noPersistenceSlot), []byte("d"))
	sp := postParam(1, 7, constants.PermissionSpecified, noPersistenceSlot)
	sp.Permission.RecipientIDs = append(sp.Permission.RecipientIDs, types.NewPID(4))
	e := post(t, s, 3, sp, []byte("e"))
	// A title's other object (ranking common data) never shows up in a DataStore search.
	_, _ = ts.PutObject(context.Background(), &tspb.PutObjectRequest{Owner: acct(1), Writer: acct(1), Collection: "ranking_common",
		Name: "9", MetaJson: `{}`, Visibility: tspb.Visibility_VISIBILITY_PUBLIC})

	asc := func(p *dstypes.DataStoreSearchParam) { p.ResultOrderColumn = constants.SearchSortColumnDataID }
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) { asc(p); p.SearchTarget = constants.SearchTypePublic }); !same(got, []uint64{a, b, c}) {
		t.Fatalf("public = %v", got)
	}
	if got := search(t, s, 3, func(p *dstypes.DataStoreSearchParam) { asc(p); p.SearchTarget = constants.SearchTypeOwnAll }); !same(got, []uint64{c, d, e}) {
		t.Fatalf("own = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) { asc(p); p.SearchTarget = constants.SearchTypeFriend }); !same(got, []uint64{b}) {
		t.Fatalf("friends = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		asc(p)
		p.SearchTarget = constants.SearchTypePublicExcludeOwnAndFriends
	}); !same(got, []uint64{c}) {
		t.Fatalf("exclude own and friends = %v", got)
	}
	if got := search(t, s, 4, func(p *dstypes.DataStoreSearchParam) { asc(p); p.SearchTarget = constants.SearchTypeReceived }); !same(got, []uint64{e}) {
		t.Fatalf("received = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		asc(p)
		p.SearchTarget = constants.SearchTypePublic
		p.DataTypes = append(types.NewList[types.UInt16](), types.NewUInt16(6), types.NewUInt16(7))
	}); !same(got, []uint64{c}) {
		t.Fatalf("data types = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		asc(p)
		p.Tags = append(types.NewList[types.String](), types.NewString("x"))
	}); !same(got, []uint64{a, c}) {
		t.Fatalf("tags = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		asc(p)
		p.OwnerIDs = append(types.NewList[types.PID](), types.NewPID(3))
	}); !same(got, []uint64{c}) {
		t.Fatalf("owner ids = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		asc(p)
		p.OwnerType = constants.SearchTargetAnybodyExcludeSpecified
		p.OwnerIDs = append(types.NewList[types.PID](), types.NewPID(3))
	}); !same(got, []uint64{a, b}) {
		t.Fatalf("anybody except = %v", got)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		p.OwnerIDs = append(types.NewList[types.PID](), types.NewPID(500))
	}); len(got) != 0 {
		t.Fatalf("unlinked owner = %v", got)
	}
	// Rating order with a minimum frequency.
	if _, ne := s.Rate(2, target(c, 0), rateParam(3)); ne != nil {
		t.Fatal(ne)
	}
	if got := search(t, s, 1, func(p *dstypes.DataStoreSearchParam) {
		p.SearchTarget = constants.SearchTypePublic
		p.ResultOrderColumn = constants.SearchSortColumnRating0
		p.ResultOrder = constants.SearchSortOrderDesc
		p.MinimalRatingFrequency = types.NewUInt32(1)
	}); !same(got, []uint64{c}) {
		t.Fatalf("rating sort = %v", got)
	}
	req, _, _ := s.searchRequest(context.Background(), 1, acct(1), dstypes.NewDataStoreSearchParam())
	if !req.AllOwners || req.Limit != searchLimit || req.Sort != "id" || req.MetaRangeJson != `{"data_type":{}}` {
		t.Fatalf("default request = %+v", req)
	}
}

func TestReadAPI(t *testing.T) {
	s, _, _ := newStore()
	hl := post(t, s, 1, postParam(5, 3, constants.PermissionPublic, noPersistenceSlot), []byte("video"))
	thumbP := postParam(3, 3, constants.PermissionPublic, noPersistenceSlot)
	thumbP.ReferDataID = types.NewUInt32(uint32(hl))
	thumb := post(t, s, 1, thumbP, []byte("png"))
	ghost := post(t, s, 2, postParam(5, 12, constants.PermissionPublic, 0), []byte("ghost"))
	post(t, s, 2, postParam(5, 12, constants.PermissionPrivate, 1), []byte("priv!"))
	ctx := context.Background()

	items, total, err := s.PublicList(ctx, Query{DataType: -1, Limit: 20})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != hl || items[0].OwnerPID != 1 {
		t.Fatalf("highlights = %+v %d %v", items, total, err)
	}
	if items, _, _ := s.PublicList(ctx, Query{DataType: -1, OwnerPID: 2, Limit: 20}); len(items) != 0 {
		t.Fatalf("highlights of 2 = %+v", items)
	}
	if items, _, _ := s.PublicList(ctx, Query{DataType: -1, OwnerPID: 500, Limit: 20}); len(items) != 0 {
		t.Fatalf("highlights of an unlinked PID = %+v", items)
	}
	g, total, err := s.PublicList(ctx, Query{DataType: 12, Ghosts: true, Limit: 20})
	if err != nil || total != 1 || g[0].ID != ghost || g[0].Slot == nil || *g[0].Slot != 0 {
		t.Fatalf("ghosts = %+v %v", g, err)
	}
	if id, ok, _ := s.Thumbnail(ctx, hl); !ok || id != thumb {
		t.Fatalf("thumbnail = %d %v", id, ok)
	}
	if counts, _ := s.GhostCounts(ctx); counts[12] != 1 || len(counts) != 1 {
		t.Fatalf("ghost counts = %v", counts)
	}
	rc, err := s.PublicBody(ctx, hl)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "video" {
		t.Fatalf("body = %q", b)
	}
	if pid, ok, _ := s.Owner(ctx, ghost); !ok || pid != 2 {
		t.Fatalf("owner = %d %v", pid, ok)
	}
	if _, ok, _ := s.PublicObject(ctx, ghost+1); ok {
		t.Fatal("a private object is served by the read API")
	}
}
