package nex_datastore_super_mario_maker

import (
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
)

func extra(s ...string) types.List[types.String] {
	l := types.NewList[types.String]()
	for _, v := range s {
		l = append(l, types.String(v))
	}
	return l
}

func TestParseDifficultyExtra(t *testing.T) {
	cases := []struct {
		name     string
		in       types.List[types.String]
		min, max int
		ok       bool
	}{
		{"all", extra("", "", "", "0", "0"), 0, 0, false},
		{"easy", extra("1", "0", "34", "0", "0"), 0, 34, true},
		{"normal", extra("1", "35", "74", "0", "0"), 35, 74, true},
		{"expert", extra("1", "75", "95", "0", "0"), 75, 95, true},
		{"super expert", extra("1", "96", "100", "0", "0"), 96, 100, true},
		{"short", extra("1", "5"), 0, 0, false},
		{"backwards", extra("1", "80", "10"), 0, 0, false},
		{"out of range", extra("1", "0", "101"), 0, 0, false},
		{"not numbers", extra("1", "a", "b"), 0, 0, false},
	}
	for _, c := range cases {
		min, max, ok := ParseDifficultyExtra(c.in)
		if ok != c.ok || min != c.min || max != c.max {
			t.Errorf("%s: got (%d,%d,%v), want (%d,%d,%v)", c.name, min, max, ok, c.min, c.max, c.ok)
		}
	}
}

func TestDifficultyBandsPartitionTheRange(t *testing.T) {
	seen := map[Difficulty]bool{}
	for rate := 0; rate <= 100; rate++ {
		d := DifficultyOf(rate)
		if d == "" {
			t.Fatalf("rate %d has no band", rate)
		}
		seen[d] = true
		min, max, ok := FailureRange(d)
		if !ok || rate < min || rate > max {
			t.Fatalf("rate %d placed in %s (%d-%d)", rate, d, min, max)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 bands, got %v", seen)
	}
	if DifficultyOf(-1) != "" {
		t.Fatal("an unattempted course must have no difficulty")
	}
}
