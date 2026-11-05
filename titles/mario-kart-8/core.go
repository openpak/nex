package mk8wiiu

import (
	"context"
	"sort"
	"strconv"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/internal/coreranking"
)

// coreTitle is Mario Kart 8 in the core services: every course ranks ascending (lower time
// is better), per network (cross_network false).
var coreTitle = nexhost.CoreTitle{Key: "mario-kart-8", Name: "Mario Kart 8", Network: "wiiu", Kind: "time", Order: "asc"}

// coreAPI is the read API with the rankings read from the core (NEX_CORE_STORAGE=rankings);
// ghosts, highlights and tournaments still come from the title's own tables.
type coreAPI struct {
	*pgAPI
	core *coreranking.Store
}

func (c coreAPI) Rankings(ctx context.Context, course, limit, offset int) ([]apiRank, int, error) {
	es, total, err := c.core.Page(ctx, uint32(course), offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]apiRank, 0, len(es))
	for _, e := range es {
		out = append(out, apiRank{Rank: int(e.Position), Player: strconv.FormatUint(e.PID, 10), Score: e.Score,
			TimeMS: e.Score, Time: formatTime(e.Score), Updated: e.Updated})
	}
	return out, total, nil
}

func (c coreAPI) PlayerRankings(ctx context.Context, pid string) ([]apiPlayerRank, error) {
	n, err := strconv.ParseUint(pid, 10, 64)
	if err != nil {
		return nil, nil
	}
	bs, err := c.core.Player(ctx, n)
	if err != nil {
		return nil, err
	}
	out := make([]apiPlayerRank, 0, len(bs))
	for _, b := range bs {
		out = append(out, apiPlayerRank{Course: int(b.Category), Rank: int(b.Entry.Rank), Of: b.Of,
			TimeMS: b.Entry.Score, Time: formatTime(b.Entry.Score)})
	}
	return out, nil
}

func (c coreAPI) Courses(ctx context.Context) ([]apiCourse, error) {
	local, err := c.pgAPI.Courses(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := c.core.Counts(ctx)
	if err != nil {
		return nil, err
	}
	byCourse := map[int]apiCourse{}
	for _, l := range local {
		l.Ranked = 0 // the core is the ranking now
		byCourse[l.Course] = l
	}
	for cat, n := range counts {
		c := byCourse[int(cat)]
		c.Course, c.Ranked = int(cat), n
		byCourse[int(cat)] = c
	}
	local = local[:0]
	for _, c := range byCourse {
		local = append(local, c)
	}
	sort.Slice(local, func(i, j int) bool { return local[i].Course < local[j].Course })
	return local, nil
}
