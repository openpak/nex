// Package board holds the Pikmin 3 leaderboard rules: which upload replaces which,
// how a category is ranked, and how the four read modes (range, near, friends, own) pick
// their window. It is pure so it can be tested without a database; store.go does the SQL.
package board

import (
	"sort"
)

// Order of a category, as the game sends it in RankingScoreData.OrderBy.
const (
	Ascending  uint8 = 0 // lower is better
	Descending uint8 = 1 // higher is better
)

// Update modes, as RankingScoreData.UpdateMode.
const (
	UpdateNormal    uint8 = 0 // keep the better of old and new
	UpdateDeleteOld uint8 = 1 // always overwrite
)

// Tie handling, RankingOrderParam.OrderCalculation.
const (
	Calc113 uint8 = 0 // competition ranking: 1, 2, 2, 4
	Calc123 uint8 = 1 // ordinal: ties broken by older update, then lower pid
)

// MaxRange is the deepest rank a range query reaches (the protocol documents 1000).
const MaxRange = 1000

// Entry is one player's score in one category.
type Entry struct {
	PID       uint32
	Category  uint32
	Score     uint32
	OrderBy   uint8
	Groups    []byte
	Param     uint64
	Common    []byte
	UniqueID  uint64
	UpdatedAt int64 // unix nanoseconds
}

// Ranked is an Entry with its place.
type Ranked struct {
	Entry
	Order uint32
}

// Better reports whether score a beats score b under orderBy.
func Better(orderBy uint8, a, b uint32) bool {
	if orderBy == Ascending {
		return a < b
	}
	return a > b
}

// Keep decides whether an upload replaces the stored entry. old is nil for a first score.
func Keep(old *Entry, incoming Entry, mode uint8) bool {
	if old == nil || mode == UpdateDeleteOld {
		return true
	}
	// Equal scores do not replace: the earlier achievement keeps its timestamp.
	return Better(incoming.OrderBy, incoming.Score, old.Score)
}

// Rank sorts a category and numbers it. The category's order is taken from orderBy.
func Rank(entries []Entry, orderBy, calc uint8) []Ranked {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Score != b.Score {
			return Better(orderBy, a.Score, b.Score)
		}
		if a.UpdatedAt != b.UpdatedAt {
			return a.UpdatedAt < b.UpdatedAt
		}
		return a.PID < b.PID
	})
	out := make([]Ranked, len(sorted))
	for i, e := range sorted {
		order := uint32(i + 1)
		if calc == Calc113 && i > 0 && e.Score == sorted[i-1].Score {
			order = out[i-1].Order
		}
		out[i] = Ranked{Entry: e, Order: order}
	}
	return out
}

// FilterGroup keeps entries whose Groups[index] equals num. index 255 (none) keeps all.
func FilterGroup(entries []Entry, index, num uint8) []Entry {
	if index == 255 {
		return entries
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if int(index) < len(e.Groups) && e.Groups[index] == num {
			out = append(out, e)
		}
	}
	return out
}

// FilterPIDs keeps entries owned by one of pids.
func FilterPIDs(entries []Entry, pids ...uint32) []Entry {
	set := make(map[uint32]struct{}, len(pids))
	for _, p := range pids {
		set[p] = struct{}{}
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if _, ok := set[e.PID]; ok {
			out = append(out, e)
		}
	}
	return out
}

// Range returns length entries starting at offset (offset 0 is first place), at most MaxRange deep.
func Range(ranked []Ranked, offset, length uint32) []Ranked {
	if offset >= uint32(len(ranked)) || offset >= MaxRange || length == 0 {
		return nil
	}
	end := offset + length
	if end > uint32(len(ranked)) {
		end = uint32(len(ranked))
	}
	if end > MaxRange {
		end = MaxRange
	}
	return ranked[offset:end]
}

// Near returns length entries centred on pid. At either end the window slides rather than shrinks.
func Near(ranked []Ranked, pid uint32, length uint32) []Ranked {
	idx := -1
	for i, r := range ranked {
		if r.PID == pid {
			idx = i
			break
		}
	}
	if idx < 0 || length == 0 {
		return nil
	}
	n := int(length)
	if n > len(ranked) {
		n = len(ranked)
	}
	start := idx - n/2
	if start < 0 {
		start = 0
	}
	if start+n > len(ranked) {
		start = len(ranked) - n
	}
	return ranked[start : start+n]
}

// Own returns pid's entries.
func Own(ranked []Ranked, pid uint32) []Ranked {
	var out []Ranked
	for _, r := range ranked {
		if r.PID == pid {
			out = append(out, r)
		}
	}
	return out
}
