package coredatastore

// Reads for a title's own web API (Mario Kart 8's MKTV site, MK8_API_ADDR): public, completed
// DataStore objects straight from the core, read as an anonymous viewer, so nothing that is not
// public can show up.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/openpak/nex/internal/coreid"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// Item is a public DataStore object as a title's web API shows it.
type Item struct {
	ID       uint64
	OwnerPID uint64
	Name     string
	Size     uint64
	DataType uint16
	Slot     *uint16 // persistence slot; nil when the object is not persisted
	Tags     []string
	Info     []byte // the meta binary
	ReferID  uint32
	Rating0  *tspb.Rating // slot 0, when it exists
	Created  time.Time
	Updated  time.Time
	GetURL   string // presigned GET of the body; empty for an object with no body
}

// Query selects public objects.
type Query struct {
	OwnerPID uint64 // 0: anyone
	DataType int    // < 0: any
	// Ghosts: objects in a persistence slot; otherwise highlights: not persisted and referring
	// to no other object.
	Ghosts        bool
	Limit, Offset int
}

func item(o *tspb.Object, m meta) Item {
	it := Item{ID: o.GetId(), OwnerPID: m.OwnerPID, Name: m.Name, Size: o.GetSize(), DataType: m.DataType,
		Tags: m.Tags, Info: o.GetInfo(), ReferID: m.ReferDataID, GetURL: o.GetGetUrl(),
		Created: time.UnixMilli(o.GetCreatedUnixMs()).UTC(), Updated: time.UnixMilli(o.GetUpdatedUnixMs()).UTC()}
	if !o.GetBlob() {
		it.Size = uint64(m.Size)
	}
	if m.PersistenceSlot != noPersistenceSlot {
		slot := m.PersistenceSlot
		it.Slot = &slot
	}
	if it.Tags == nil {
		it.Tags = []string{}
	}
	for _, r := range o.GetRatings() {
		if r.GetSlot() == 0 {
			it.Rating0 = r
		}
	}
	return it
}

func (s *Store) publicRequest(q Query) *tspb.ListObjectsRequest {
	filter := map[string]any{"permission": 0}
	ranges := map[string]any{"data_type": map[string]any{}}
	req := &tspb.ListObjectsRequest{Ref: s.ref(), AllOwners: true, IncludeRatings: true}
	if q.Ghosts {
		ranges["persistence_slot"] = map[string]any{"le": noPersistenceSlot - 1}
		req.Sort = "updated"
	} else {
		filter["persistence_slot"] = noPersistenceSlot
		filter["refer_data_id"] = 0
		req.Sort = "created"
	}
	if q.DataType >= 0 {
		req.Collection = collection(uint16(q.DataType))
	}
	f, _ := json.Marshal(filter)
	r, _ := json.Marshal(ranges)
	req.MetaFilterJson, req.MetaRangeJson = string(f), string(r)
	req.Offset, req.Limit = uint32(q.Offset), uint32(q.Limit)
	return req
}

// PublicList is one page of public objects, newest first, and the total.
func (s *Store) PublicList(ctx context.Context, q Query) ([]Item, int, error) {
	req := s.publicRequest(q)
	if q.OwnerPID != 0 {
		owner, err := s.IDs.Account(ctx, q.OwnerPID)
		if errors.Is(err, coreid.ErrNoLink) {
			return nil, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		req.AllOwners, req.Owners = false, []string{owner}
	}
	r, err := s.TS.ListObjects(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	var out []Item
	for _, o := range r.GetObjects() {
		if m, ok := decodeMeta(o); ok {
			out = append(out, item(o, m))
		}
	}
	return out, int(r.GetHits()), nil
}

// PublicObject is one public DataStore object; found is false for anything else.
func (s *Store) PublicObject(ctx context.Context, id uint64) (Item, bool, error) {
	r, err := s.TS.GetObjectByID(ctx, &tspb.GetObjectByIDRequest{Network: s.Cfg.Network, Id: id, IncludeRatings: true})
	if err != nil || !r.GetFound() {
		return Item{}, false, err
	}
	m, ok := decodeMeta(r.GetObject())
	if !ok || m.Permission != 0 {
		return Item{}, false, nil
	}
	return item(r.GetObject(), m), true, nil
}

// Thumbnail is the oldest public object that refers to id (a highlight's thumbnail).
func (s *Store) Thumbnail(ctx context.Context, id uint64) (uint64, bool, error) {
	f, _ := json.Marshal(map[string]any{"permission": 0, "refer_data_id": id})
	r, err := s.TS.ListObjects(ctx, &tspb.ListObjectsRequest{Ref: s.ref(), AllOwners: true, MetaFilterJson: string(f),
		MetaRangeJson: `{"data_type":{}}`, Sort: "id", Ascending: true, Limit: 1})
	if err != nil || len(r.GetObjects()) == 0 {
		return 0, false, err
	}
	return r.GetObjects()[0].GetId(), true, nil
}

// GhostCounts is the number of public ghosts per data type (course).
func (s *Store) GhostCounts(ctx context.Context) (map[int]int, error) {
	out := map[int]int{}
	q := Query{Ghosts: true, DataType: -1, Limit: 1000}
	for page := 0; page < 50; page++ {
		q.Offset = page * q.Limit
		items, total, err := s.PublicList(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			out[int(it.DataType)]++
		}
		if len(items) == 0 || q.Offset+len(items) >= total {
			break
		}
	}
	return out, nil
}

// ErrNoBody is a public object without a body in the bucket.
var ErrNoBody = errors.New("object has no body")

// PublicBody opens a public object's body through its presigned GET (the public bucket host,
// as a console would fetch it).
func (s *Store) PublicBody(ctx context.Context, id uint64) (io.ReadCloser, error) {
	it, ok, err := s.PublicObject(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok || it.GetURL == "" {
		return nil, ErrNoBody
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, it.GetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("body fetch: HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Owner is the PID that owns a public DataStore object (MKTV's YouTube worker: whose consent
// decides). found is false for a missing or non-public object.
func (s *Store) Owner(ctx context.Context, id uint64) (uint64, bool, error) {
	it, ok, err := s.PublicObject(ctx, id)
	return it.OwnerPID, ok, err
}
