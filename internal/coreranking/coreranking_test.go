package coreranking

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	commonranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	"github.com/PretendoNetwork/nex-protocols-go/v2/ranking/constants"
	rankingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	_ "github.com/lib/pq"

	"github.com/openpak/nex/internal/rankingsapi"
)

func newStore(known ...uint64) (*Store, *fakeLB, *fakeTS, *fakeIDs) {
	lb, ts := newFakeLB(), newFakeTS()
	ids := &fakeIDs{known: map[uint64]bool{}}
	for _, p := range known {
		ids.known[p] = true
	}
	return &Store{Cfg: Config{Title: "mario-kart-8", Name: "Mario Kart 8", Network: "wiiu", Kind: "time", Order: "asc"},
		LB: lb, TS: ts, IDs: ids}, lb, ts, ids
}

func score(cat, s uint32, mode constants.UpdateMode, groups ...byte) rankingtypes.RankingScoreData {
	d := rankingtypes.NewRankingScoreData()
	d.Category, d.Score, d.UpdateMode = types.NewUInt32(cat), types.NewUInt32(s), mode
	d.Groups, d.Param = types.NewBuffer(groups), types.NewUInt64(77)
	return d
}

func order(offset uint32, length uint8) rankingtypes.RankingOrderParam {
	o := rankingtypes.NewRankingOrderParam()
	o.GroupIndex = constants.FilterGroupIndexNone
	o.Offset, o.Length = types.NewUInt32(offset), types.NewUInt8(length)
	return o
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func pids(l types.List[rankingtypes.RankingRankData]) []uint64 {
	var out []uint64
	for _, d := range l {
		out = append(out, uint64(d.PrincipalID))
	}
	return out
}

func eq(a, b []uint64) bool {
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

func TestInstallSetsEveryHook(t *testing.T) {
	s, _, _, _ := newStore()
	p := &commonranking.CommonProtocol{}
	s.Install(p)
	if p.InsertRankingByPIDAndRankingScoreData == nil || p.UploadCommonData == nil || p.GetCommonData == nil ||
		p.GetRankingsAndCountByCategoryAndRankingOrderParam == nil || p.GetNearbyRankingsAndCountByCategoryAndRankingOrderParam == nil ||
		p.GetFriendsRankingsAndCountByCategoryAndRankingOrderParam == nil || p.GetNearbyFriendsRankingsAndCountByCategoryAndRankingOrderParam == nil ||
		p.GetOwnRankingByCategoryAndRankingOrderParam == nil {
		t.Fatal("a hook is unset")
	}
}

func TestRegisterMapsBothServicesPerNetwork(t *testing.T) {
	s, lb, ts, _ := newStore()
	must(t, s.Register(context.Background()))
	if lb.titles["wiiu/mario-kart-8"] == nil || !ts.titles["wiiu/mario-kart-8"] {
		t.Fatalf("title not mapped: lb %v ts %v", lb.titles, ts.titles)
	}
}

func TestBestScoreKeptAndInfoTravelsWithIt(t *testing.T) {
	s, lb, _, _ := newStore(10)
	must(t, s.Insert(10, score(3, 90000, constants.UpdateModeNormal, 1, 2), 555))
	must(t, s.Insert(10, score(3, 95000, constants.UpdateModeNormal, 9, 9), 556)) // worse: ignored
	l, total, err := s.Global(3, order(0, 10))
	must(t, err)
	if total != 1 || len(l) != 1 || l[0].Score != 90000 || l[0].UniqueID != 555 || l[0].Groups[0] != 1 || l[0].Param != 77 || l[0].Order != 1 {
		t.Fatalf("got %d %+v", total, l)
	}
	p := lb.boards["mario-kart-8/3"]
	if p.GetOrder() != "asc" || p.GetAggregation() != "best" || p.GetCrossNetwork() != 2 {
		t.Fatalf("board policy %+v", p)
	}
	must(t, s.Insert(10, score(3, 99000, constants.UpdateModeDeleteOld), 557)) // delete-old: always lands
	l, _, _ = s.Global(3, order(0, 10))
	if l[0].Score != 99000 || l[0].UniqueID != 557 {
		t.Fatalf("delete-old not applied: %+v", l[0])
	}
}

func TestOrderFromFirstScoreWhenNotForced(t *testing.T) {
	s, lb, _, _ := newStore(1)
	s.Cfg.Order = ""
	d := score(8, 5, constants.UpdateModeNormal)
	d.OrderBy = constants.OrderByDescending
	must(t, s.Insert(1, d, 1))
	if lb.boards["mario-kart-8/8"].GetOrder() != "desc" {
		t.Fatal("descending OrderBy should make a desc board")
	}
}

func TestModes(t *testing.T) {
	s, lb, _, _ := newStore(1, 2, 3, 4, 5)
	for pid, sc := range map[uint64]uint32{1: 100, 2: 200, 3: 300, 4: 400, 5: 500} {
		g := byte(0)
		if pid%2 == 0 {
			g = 1
		}
		must(t, s.Insert(types.PID(pid), score(1, sc, constants.UpdateModeNormal, g, 0), types.UInt64(pid)))
	}
	lb.friends[acct(3)] = []string{acct(1), acct(5)}

	l, total, err := s.Global(1, order(1, 2))
	must(t, err)
	if total != 5 || !eq(pids(l), []uint64{2, 3}) || l[0].Order != 2 {
		t.Fatalf("global window: %d %v", total, pids(l))
	}
	l, _, _ = s.Nearby(4, 1, order(0, 3))
	if !eq(pids(l), []uint64{3, 4, 5}) {
		t.Fatalf("nearby: %v", pids(l))
	}
	l, total, _ = s.Friends(3, 1, order(0, 10))
	if total != 3 || !eq(pids(l), []uint64{1, 3, 5}) || l[1].Order != 2 {
		t.Fatalf("friends: %d %v", total, pids(l))
	}
	l, _, _ = s.NearbyFriends(5, 1, order(0, 1)) // 5 has no friends listed: only itself
	if !eq(pids(l), []uint64{5}) {
		t.Fatalf("nearby friends: %v", pids(l))
	}
	l, _, _ = s.Own(4, 1, order(0, 10))
	if !eq(pids(l), []uint64{4}) || l[0].Order != 4 {
		t.Fatalf("own: %v", l)
	}
	// Group filter: group 0 == 1 keeps the even PIDs, ranked among themselves.
	o := order(0, 10)
	o.GroupIndex, o.GroupNum = 0, 1
	l, total, _ = s.Global(1, o)
	if total != 2 || !eq(pids(l), []uint64{2, 4}) || l[1].Order != 2 {
		t.Fatalf("group filter: %d %v", total, pids(l))
	}
	l, _, _ = s.Own(4, 1, o)
	if len(l) != 1 || l[0].Order != 2 {
		t.Fatalf("own with group filter: %v", l)
	}
}

func TestTiesShareRankUnlessOrdinal(t *testing.T) {
	s, _, _, _ := newStore(1, 2)
	must(t, s.Insert(1, score(1, 100, constants.UpdateModeNormal), 1))
	must(t, s.Insert(2, score(1, 100, constants.UpdateModeNormal), 2))
	l, _, _ := s.Global(1, order(0, 10))
	if l[0].Order != 1 || l[1].Order != 1 {
		t.Fatalf("1224: %d %d", l[0].Order, l[1].Order)
	}
	o := order(0, 10)
	o.OrderCalculation = constants.OrderCalculation123
	l, _, _ = s.Global(1, o)
	if l[0].Order != 1 || l[1].Order != 2 {
		t.Fatalf("1234: %d %d", l[0].Order, l[1].Order)
	}
}

func TestCommonDataRoundTripAndJoin(t *testing.T) {
	s, _, ts, _ := newStore(1)
	must(t, s.UploadCommonData(1, 42, types.NewBuffer([]byte("mii"))))
	b, err := s.GetCommonData(42)
	must(t, err)
	if string(b) != "mii" {
		t.Fatalf("common data %q", b)
	}
	if o := ts.objects[acct(1)+"/"+CommonCollection+"/42"]; o == nil || o.Visibility != 3 {
		t.Fatalf("object %+v", o)
	}
	if _, err := s.GetCommonData(43); !errors.Is(err, ErrNoCommonData) {
		t.Fatalf("missing common data: %v", err)
	}
	must(t, s.Insert(1, score(2, 1, constants.UpdateModeNormal), 42))
	l, _, _ := s.Global(2, order(0, 1))
	if string(l[0].CommonData) != "mii" {
		t.Fatalf("rank row without its common data: %q", l[0].CommonData)
	}
}

func TestEmptyAndUnknown(t *testing.T) {
	s, _, _, _ := newStore()
	l, total, err := s.Global(99, order(0, 10)) // no such board
	if err != nil || total != 0 || len(l) != 0 {
		t.Fatalf("empty board: %v %d %v", l, total, err)
	}
	l, _, err = s.Own(7, 99, order(0, 10)) // caller with no link
	if err != nil || len(l) != 0 {
		t.Fatalf("unknown caller: %v %v", l, err)
	}
}

func TestNoLinkWithoutDBIsAnError(t *testing.T) {
	s, lb, _, _ := newStore()
	if err := s.Insert(9, score(1, 1, constants.UpdateModeNormal), 1); err == nil {
		t.Fatal("a score that cannot be stored anywhere must fail")
	}
	if lb.calls["Submit"] != 0 {
		t.Fatal("submitted for an unlinked PID")
	}
}

func TestContractSource(t *testing.T) {
	s, _, _, _ := newStore(1, 2)
	must(t, s.Insert(1, score(4, 100, constants.UpdateModeNormal), 1))
	must(t, s.Insert(2, score(4, 200, constants.UpdateModeNormal), 2))
	var src rankingsapi.Source = s
	bs, err := src.Boards(context.Background())
	must(t, err)
	if len(bs) != 1 || bs[0].ID != "4" || bs[0].Kind != "time" || bs[0].Order != "asc" {
		t.Fatalf("boards %+v", bs)
	}
	p, err := src.Rows(context.Background(), "4", rankingsapi.Query{Limit: 10, Around: acct(2)})
	must(t, err)
	if p.Total != 2 || p.Rows[0].AccountID != acct(1) || p.Rows[0].PID != "1" || len(p.Around) != 2 {
		t.Fatalf("page %+v", p)
	}
	if _, err := src.Rows(context.Background(), "5", rankingsapi.Query{Limit: 10}); !errors.Is(err, rankingsapi.ErrNoBoard) {
		t.Fatalf("missing board: %v", err)
	}
	pb, err := s.Player(context.Background(), 2)
	must(t, err)
	if len(pb) != 1 || pb[0].Entry.Rank != 2 || pb[0].Of != 2 {
		t.Fatalf("player %+v", pb)
	}
	c, err := s.Counts(context.Background())
	must(t, err)
	if c[4] != 2 {
		t.Fatalf("counts %v", c)
	}
}

func TestInfoRoundTrip(t *testing.T) {
	in := info{PID: 1 << 40, UID: 7, Param: 1<<63 + 5, OrderBy: 1, Groups: []byte{3, 4}}
	out, ok := decodeInfo(in.encode())
	if !ok || out.PID != in.PID || out.UID != 7 || out.Param != in.Param || out.OrderBy != 1 || string(out.Groups) != "\x03\x04" {
		t.Fatalf("%+v", out)
	}
	if _, ok := decodeInfo([]byte{2}); ok {
		t.Fatal("foreign info decoded")
	}
}

// TestPendingRowsDrainOnceLinked needs a throwaway Postgres (NEX_TEST_DSN): a score from a
// player with no link, and one met by a core outage, wait in corestore.pending_* and move to the
// core once possible; nothing is dropped.
func TestPendingRowsDrainOnceLinked(t *testing.T) {
	dsn := os.Getenv("NEX_TEST_DSN")
	if dsn == "" {
		t.Skip("NEX_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	must(t, err)
	defer db.Close()
	_, _ = db.Exec(`DROP SCHEMA IF EXISTS corestore CASCADE`)
	s, lb, ts, ids := newStore(1)
	s.DB = db
	must(t, s.InitTables())

	must(t, s.Insert(9, score(1, 50, constants.UpdateModeNormal, 1), 90)) // no link yet
	must(t, s.UploadCommonData(9, 90, types.NewBuffer([]byte("x"))))
	lb.down, ts.down = true, true
	must(t, s.Insert(1, score(1, 60, constants.UpdateModeNormal), 10)) // core down
	lb.down, ts.down = false, false

	var n int
	must(t, db.QueryRow(`SELECT count(*) FROM corestore.pending_scores`).Scan(&n))
	if n != 2 {
		t.Fatalf("pending scores %d", n)
	}
	moved, err := s.Drain(context.Background())
	must(t, err)
	if moved != 1 { // the outage row; PID 9 still has no link
		t.Fatalf("moved %d", moved)
	}
	ids.known[9] = true
	moved, err = s.Drain(context.Background())
	must(t, err)
	if moved != 2 { // the score and the common data of PID 9
		t.Fatalf("moved %d", moved)
	}
	must(t, db.QueryRow(`SELECT (SELECT count(*) FROM corestore.pending_scores) + (SELECT count(*) FROM corestore.pending_common_data)`).Scan(&n))
	if n != 0 {
		t.Fatalf("left %d", n)
	}
	l, total, err := s.Global(1, order(0, 10))
	must(t, err)
	if total != 2 || !eq(pids(l), []uint64{9, 1}) || string(l[0].CommonData) != "x" || l[0].Groups[0] != 1 {
		t.Fatalf("after drain: %d %v", total, l)
	}
	_, _ = db.Exec(`DROP SCHEMA corestore CASCADE`)
}
