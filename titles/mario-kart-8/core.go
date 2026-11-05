package mk8wiiu

import (
	"context"
	"encoding/hex"
	"sort"
	"strconv"

	"github.com/lib/pq"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/internal/coredatastore"
	"github.com/openpak/nex/internal/coreranking"
)

// coreTitle is Mario Kart 8 in the core services: every course ranks ascending (lower time
// is better), per network (cross_network false).
var coreTitle = nexhost.CoreTitle{Key: "mario-kart-8", Name: "Mario Kart 8", Network: "wiiu", Kind: "time", Order: "asc"}

// coreMaxObjectBytes is Mario Kart 8's per-object cap in title-storage: a ghost or a Mario Kart
// TV highlight's replay data, the same ceiling the read API proxies (apiMaxBody).
const coreMaxObjectBytes = apiMaxBody

// coreAPI is the read API over the core stores the title is flipped to: rankings from
// leaderboards (NEX_CORE_STORAGE=rankings), ghosts and highlights from title-storage
// (NEX_CORE_STORAGE=datastore). Either may be nil; what is not flipped, and tournaments and
// the MKTV YouTube ids (mk8.mktv_videos, the title's own feature state), come from the
// title's own tables.
type coreAPI struct {
	*pgAPI
	core *coreranking.Store
	ds   *coredatastore.Store
}

func (c coreAPI) Rankings(ctx context.Context, course, limit, offset int) ([]apiRank, int, error) {
	if c.core == nil {
		return c.pgAPI.Rankings(ctx, course, limit, offset)
	}
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
	if c.core == nil {
		return c.pgAPI.PlayerRankings(ctx, pid)
	}
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
	byCourse := map[int]apiCourse{}
	for _, l := range local {
		if c.core != nil {
			l.Ranked = 0 // the core is the ranking now
		}
		if c.ds != nil {
			l.Ghosts = 0 // and the ghosts
		}
		byCourse[l.Course] = l
	}
	if c.core != nil {
		counts, err := c.core.Counts(ctx)
		if err != nil {
			return nil, err
		}
		for cat, n := range counts {
			x := byCourse[int(cat)]
			x.Course, x.Ranked = int(cat), n
			byCourse[int(cat)] = x
		}
	}
	if c.ds != nil {
		ghosts, err := c.ds.GhostCounts(ctx)
		if err != nil {
			return nil, err
		}
		for course, n := range ghosts {
			x := byCourse[course]
			x.Course, x.Ghosts = course, n
			byCourse[course] = x
		}
	}
	out := make([]apiCourse, 0, len(byCourse))
	for _, x := range byCourse {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Course < out[j].Course })
	return out, nil
}

// ownerPID parses the API's owner parameter ("" = anyone); a non-number matches nobody.
func ownerPID(owner string) (uint64, bool) {
	if owner == "" {
		return 0, true
	}
	n, err := strconv.ParseUint(owner, 10, 64)
	return n, err == nil && n != 0
}

func (c coreAPI) Highlights(ctx context.Context, owner string, limit, offset int) ([]apiObject, int, error) {
	if c.ds == nil {
		return c.pgAPI.Highlights(ctx, owner, limit, offset)
	}
	pid, ok := ownerPID(owner)
	if !ok {
		return nil, 0, nil
	}
	items, total, err := c.ds.PublicList(ctx, coredatastore.Query{OwnerPID: pid, DataType: c.mktvType, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	out, err := c.apiObjects(ctx, items)
	return out, total, err
}

func (c coreAPI) Ghosts(ctx context.Context, course int, owner string, limit, offset int) ([]apiObject, int, error) {
	if c.ds == nil {
		return c.pgAPI.Ghosts(ctx, course, owner, limit, offset)
	}
	pid, ok := ownerPID(owner)
	if !ok {
		return nil, 0, nil
	}
	items, total, err := c.ds.PublicList(ctx, coredatastore.Query{OwnerPID: pid, DataType: course, Ghosts: true, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	out, err := c.apiObjects(ctx, items)
	return out, total, err
}

func (c coreAPI) Object(ctx context.Context, id uint64) (apiObject, error) {
	if c.ds == nil {
		return c.pgAPI.Object(ctx, id)
	}
	it, ok, err := c.ds.PublicObject(ctx, id)
	if err != nil {
		return apiObject{}, err
	}
	if !ok {
		return apiObject{}, errAPINotFound
	}
	out, err := c.apiObjects(ctx, []coredatastore.Item{it})
	if err != nil {
		return apiObject{}, err
	}
	return out[0], nil
}

// apiObjects renders core objects as the API's, with each one's thumbnail (an object referring
// to it) and the YouTube id the MKTV worker recorded locally.
func (c coreAPI) apiObjects(ctx context.Context, items []coredatastore.Item) ([]apiObject, error) {
	out := make([]apiObject, 0, len(items))
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		o := apiObject{ID: it.ID, Owner: strconv.FormatUint(it.OwnerPID, 10), Name: it.Name, Size: int64(it.Size),
			DataType: int(it.DataType), Tags: it.Tags, MetaSize: len(it.Info), ReferID: uint64(it.ReferID),
			Created: it.Created, Updated: it.Updated}
		if it.Slot != nil {
			s := int(*it.Slot)
			o.Slot = &s
		}
		if len(it.Info) > 0 && len(it.Info) <= 256 {
			o.MetaHex = hex.EncodeToString(it.Info)
		}
		if it.Rating0 != nil {
			o.Rating = &apiRated{Total: it.Rating0.GetTotal(), Count: int64(it.Rating0.GetCount())}
		}
		thumb, ok, err := c.ds.Thumbnail(ctx, it.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			o.Thumbnail = &thumb
		}
		out = append(out, o)
		ids = append(ids, int64(it.ID))
	}
	var have bool
	if len(ids) == 0 || c.db.QueryRowContext(ctx, `SELECT to_regclass('mk8.mktv_videos') IS NOT NULL`).Scan(&have) != nil || !have {
		return out, nil
	}
	rows, err := c.db.QueryContext(ctx, `SELECT data_id, youtube_id FROM mk8.mktv_videos
		WHERE data_id = ANY($1) AND status = 'uploaded' AND youtube_id IS NOT NULL`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	yt := map[uint64]string{}
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		yt[uint64(id)] = v
	}
	for i := range out {
		out[i].YouTubeID = yt[out[i].ID]
	}
	return out, rows.Err()
}
