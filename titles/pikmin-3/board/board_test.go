package board

import "testing"

func e(pid, score uint32, at int64) Entry { return Entry{PID: pid, Score: score, UpdatedAt: at} }

func TestKeep(t *testing.T) {
	old := &Entry{Score: 100}
	cases := []struct {
		name  string
		old   *Entry
		in    Entry
		mode  uint8
		keeps bool
	}{
		{"first", nil, Entry{Score: 1}, UpdateNormal, true},
		{"desc better", old, Entry{Score: 101, OrderBy: Descending}, UpdateNormal, true},
		{"desc worse", old, Entry{Score: 99, OrderBy: Descending}, UpdateNormal, false},
		{"asc better", old, Entry{Score: 99, OrderBy: Ascending}, UpdateNormal, true},
		{"asc worse", old, Entry{Score: 101, OrderBy: Ascending}, UpdateNormal, false},
		{"tie keeps old", old, Entry{Score: 100, OrderBy: Descending}, UpdateNormal, false},
		{"delete old overwrites worse", old, Entry{Score: 1, OrderBy: Descending}, UpdateDeleteOld, true},
	}
	for _, c := range cases {
		if got := Keep(c.old, c.in, c.mode); got != c.keeps {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestRankTies(t *testing.T) {
	in := []Entry{e(3, 50, 3), e(1, 90, 1), e(2, 90, 2), e(4, 10, 4)}
	r113 := Rank(in, Descending, Calc113)
	want113 := []uint32{1, 1, 3, 4}
	r123 := Rank(in, Descending, Calc123)
	want123 := []uint32{1, 2, 3, 4}
	for i := range in {
		if r113[i].Order != want113[i] {
			t.Errorf("113[%d]=%d want %d", i, r113[i].Order, want113[i])
		}
		if r123[i].Order != want123[i] {
			t.Errorf("123[%d]=%d want %d", i, r123[i].Order, want123[i])
		}
	}
	// the earlier update wins a tie
	if r123[0].PID != 1 || r123[1].PID != 2 {
		t.Errorf("tie order %d %d", r123[0].PID, r123[1].PID)
	}
	// input untouched
	if in[0].PID != 3 {
		t.Error("Rank sorted its input")
	}
}

func TestRankAscending(t *testing.T) {
	r := Rank([]Entry{e(1, 300, 1), e(2, 120, 2)}, Ascending, Calc123)
	if r[0].PID != 2 || r[0].Order != 1 {
		t.Errorf("lowest time should be first: %+v", r)
	}
}

func ranked(n int) []Ranked {
	var in []Entry
	for i := 1; i <= n; i++ {
		in = append(in, e(uint32(i), uint32(1000-i), int64(i)))
	}
	return Rank(in, Descending, Calc123)
}

func TestRange(t *testing.T) {
	r := ranked(10)
	if got := Range(r, 0, 3); len(got) != 3 || got[0].Order != 1 {
		t.Errorf("first page %+v", got)
	}
	if got := Range(r, 8, 5); len(got) != 2 || got[0].Order != 9 {
		t.Errorf("clipped page %+v", got)
	}
	if Range(r, 10, 5) != nil || Range(r, 0, 0) != nil {
		t.Error("past end / zero length must be empty")
	}
}

func TestNear(t *testing.T) {
	r := ranked(20)
	cases := []struct {
		pid        uint32
		length     uint32
		first, len int
	}{
		{10, 5, 8, 5}, // centred: 8..12
		{1, 5, 1, 5},  // top: slides, does not shrink
		{20, 5, 16, 5},
		{3, 50, 1, 20}, // longer than the board
	}
	for _, c := range cases {
		got := Near(r, c.pid, c.length)
		if len(got) != c.len || int(got[0].Order) != c.first {
			t.Errorf("pid %d len %d: got first=%d n=%d", c.pid, c.length, got[0].Order, len(got))
		}
	}
	if Near(r, 99, 5) != nil {
		t.Error("a player without a score has no neighbourhood")
	}
}

func TestOwnAndFilters(t *testing.T) {
	in := []Entry{{PID: 1, Score: 5, Groups: []byte{7, 1}}, {PID: 2, Score: 6, Groups: []byte{7, 2}}, {PID: 3, Score: 7, Groups: []byte{8, 2}}}
	if got := FilterGroup(in, 0, 7); len(got) != 2 {
		t.Errorf("group filter %d", len(got))
	}
	if got := FilterGroup(in, 255, 0); len(got) != 3 {
		t.Error("index 255 must not filter")
	}
	if got := FilterGroup(in, 5, 0); len(got) != 0 {
		t.Error("index past the groups matches nothing")
	}
	if got := FilterPIDs(in, 1, 3); len(got) != 2 {
		t.Errorf("pid filter %d", len(got))
	}
	r := Rank(in, Descending, Calc113)
	if got := Own(r, 2); len(got) != 1 || got[0].Order != 2 {
		t.Errorf("own %+v", got)
	}
}
