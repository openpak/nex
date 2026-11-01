package nex

import (
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/ranking/legacy/constants"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/legacy/types"

	"github.com/openpak/nex/titles/mario-kart-7/database"
)

// memRanking is an in-memory rankingBackend: one score row per (category, pid), first score
// ascending, mirroring the Postgres store's ordering.
type memRanking struct {
	rows []database.RankEntry
	cat  map[uint64]uint32
}

func (m *memRanking) UploadScore(pid uint64, uid, category uint32, scores []uint32, u1 uint8, u2 uint32) (bool, error) {
	for i, r := range m.rows {
		if r.PID == pid && m.cat[uint64(i)] == category {
			if scores[0] >= r.Scores[0] {
				return false, nil
			}
			m.rows[i].Scores = scores
			return true, nil
		}
	}
	m.cat[uint64(len(m.rows))] = category
	m.rows = append(m.rows, database.RankEntry{PID: pid, UniqueID: uid, Scores: scores, Unknown1: u1, Unknown2: u2})
	return true, nil
}
func (m *memRanking) DeleteScore(uint64, uint32) error              { return nil }
func (m *memRanking) DeleteAllScores(uint64) error                  { return nil }
func (m *memRanking) UploadCommonData(uint64, uint32, []byte) error { return nil }
func (m *memRanking) DeleteCommonData(uint64, uint32) error         { return nil }
func (m *memRanking) CommonData(uint32) ([]byte, error)             { return nil, nil }
func (m *memRanking) Total(c uint32) (uint32, error)                { return uint32(len(m.sorted(c, false))), nil }
func (m *memRanking) sorted(c uint32, desc bool) []database.RankEntry {
	var out []database.RankEntry
	for i, r := range m.rows {
		if m.cat[uint64(i)] == c {
			out = append(out, r)
		}
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if (out[j].Scores[0] < out[i].Scores[0]) != desc {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
func (m *memRanking) Page(c uint32, _ int, desc bool, off, length uint32) ([]database.RankEntry, error) {
	all := m.sorted(c, desc)
	var out []database.RankEntry
	for i := off; i < uint32(len(all)) && uint32(len(out)) < length; i++ {
		e := all[i]
		e.Rank = i + 1
		out = append(out, e)
	}
	return out, nil
}
func (m *memRanking) Position(pid uint64, c uint32, _ int, desc bool) (uint32, bool, error) {
	for i, e := range m.sorted(c, desc) {
		if e.PID == pid {
			return uint32(i), true, nil
		}
	}
	return 0, false, nil
}

func seeded() *legacyRanking {
	m := &memRanking{cat: map[uint64]uint32{}}
	for i, t := range []uint32{90000, 80000, 85000, 70000, 95000} {
		m.UploadScore(uint64(100+i), uint32(i), 4, []uint32{t, 1}, 0, 0)
	}
	return &legacyRanking{store: m}
}

func TestWindowRange(t *testing.T) {
	l := seeded()
	got, err := l.window(0, 4, ranking_types.NewRankingOrderParam(), 3, constants.RankingModeRange, 1)
	if err != nil || len(got) != 3 || got[0].PID != 101 || got[0].Rank != 2 || got[2].Scores[0] != 90000 {
		t.Fatalf("range: %+v %v", got, err)
	}
	if got, _ = l.window(0, 4, ranking_types.NewRankingOrderParam(), 3, constants.RankingModeRange, 5000); len(got) != 0 {
		t.Fatal("offset past the ranking range must be empty")
	}
}

func TestWindowNearAndUser(t *testing.T) {
	l := seeded()
	// pid 102 (85000) is third of five: a window of 3 centres on it.
	got, _ := l.window(102, 4, ranking_types.NewRankingOrderParam(), 3, constants.RankingModeNear, 99)
	if len(got) != 3 || got[0].Rank != 2 || got[1].PID != 102 {
		t.Fatalf("near: %+v", got)
	}
	// the fastest player has nothing before them: the window starts at rank 1
	got, _ = l.window(103, 4, ranking_types.NewRankingOrderParam(), 5, constants.RankingModeNear, 0)
	if len(got) != 5 || got[0].PID != 103 {
		t.Fatalf("near at the top: %+v", got)
	}
	got, _ = l.window(100, 4, ranking_types.NewRankingOrderParam(), 5, constants.RankingModeUser, 0)
	if len(got) != 1 || got[0].PID != 100 || got[0].Rank != 4 {
		t.Fatalf("user: %+v", got)
	}
	if got, _ = l.window(999, 4, ranking_types.NewRankingOrderParam(), 5, constants.RankingModeNear, 0); len(got) != 0 {
		t.Fatalf("an unranked player has no window: %+v", got)
	}
}

func TestWindowDescendingOrder(t *testing.T) {
	l := seeded()
	op := ranking_types.NewRankingOrderParam()
	op.ScoreOrder = constants.OrderByDescending
	got, _ := l.window(0, 4, op, 1, constants.RankingModeRange, 0)
	if len(got) != 1 || got[0].Scores[0] != 95000 {
		t.Fatalf("descending: %+v", got)
	}
}

func TestStoreKeepsTheBestTime(t *testing.T) {
	l := seeded()
	if e := l.store1(100, 0, 4, []uint32{99999, 1}, 0, 0); e != nil {
		t.Fatal(e)
	}
	if n, _ := l.store.Total(4); n != 5 {
		t.Fatalf("total %d", n)
	}
	got, _ := l.store.Page(4, 0, false, 3, 1)
	if got[0].PID != 100 || got[0].Scores[0] != 90000 {
		t.Fatalf("a slower upload replaced the best: %+v", got)
	}
	if e := l.store1(100, 0, 4, nil, 0, 0); e == nil {
		t.Fatal("an empty score list was accepted")
	}
}

func TestRankDataRoundTrip(t *testing.T) {
	lib := nex.NewLibraryVersions()
	lib.SetDefault(nex.NewLibraryVersion(2, 4, 3))
	settings := nex.NewByteStreamSettings()
	out := nex.NewByteStreamOut(lib, settings)
	writeRankData(out, 4, []database.RankEntry{{Rank: 1, PID: 1800000005, UniqueID: 5, Scores: []uint32{83456, 7}, Unknown1: 2, Unknown2: 3, CommonData: []byte("mii")}})
	in := nex.NewByteStreamIn(out.Bytes(), lib, settings)
	var list types.List[ranking_types.RankingData]
	if err := list.ExtractFrom(in); err != nil || len(list) != 1 {
		t.Fatalf("decode: %v %d", err, len(list))
	}
	d := list[0]
	if uint64(d.PrincipalID) != 1800000005 || d.Order != 1 || d.Category != 4 || len(d.Scores) != 2 || d.Scores[0] != 83456 || string(d.CommonData) != "mii" || d.Unknown1 != 2 {
		t.Fatalf("round trip: %+v", d)
	}
}

func TestToU32(t *testing.T) {
	if got := toU32(types.List[types.UInt32]{1, 2}); len(got) != 2 || got[1] != 2 {
		t.Fatalf("%v", got)
	}
}
