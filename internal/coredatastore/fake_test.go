package coredatastore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/openpak/nex/internal/coreid"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// fakeBucket is the S3 side of the blob tier: a POST form upload (fields, then "file") and a GET
// of the key, as a console does them.
type fakeBucket struct {
	mu   sync.Mutex
	data map[string][]byte
	srv  *httptest.Server
}

func newFakeBucket() *fakeBucket {
	b := &fakeBucket{data: map[string][]byte{}}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			if err := r.ParseMultipartForm(16 << 20); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if r.FormValue("policy") == "" {
				http.Error(w, "no policy", 403)
				return
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			body, _ := io.ReadAll(f)
			b.data[r.FormValue("key")] = body
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			body, ok := b.data[strings.TrimPrefix(r.URL.Path, "/title-storage/")]
			if !ok {
				http.Error(w, "NoSuchKey", 404)
				return
			}
			_, _ = w.Write(body)
		}
	}))
	return b
}

type fakeRating struct {
	policy *tspb.RatingPolicy
	total  int64
	count  uint32
	raters map[string]int64
}

type fakeObj struct {
	o       *tspb.Object // metadata as stored (no ratings, no url)
	key     string       // bucket key of a blob
	ratings map[int32]*fakeRating
}

type fakePending struct {
	req *tspb.PrepareUploadRequest
	key string
}

// fakeTS is an in-memory title-storage with the rules the adapter relies on: viewer visibility
// (public, friends, recipients, private), the owner-only write rule, the blob tier's
// prepare/complete, metadata-only updates and rating slots.
type fakeTS struct {
	tspb.TitleStorageClient
	mu      sync.Mutex
	objs    map[uint64]*fakeObj
	pending map[uint64]*fakePending
	next    uint64
	clock   int64
	friends map[string][]string
	bucket  *fakeBucket
	titles  map[string]*tspb.UpsertTitleRequest
	mapped  map[string]string
	down    bool
	calls   map[string]int
}

func newFakeTS() *fakeTS {
	return &fakeTS{objs: map[uint64]*fakeObj{}, pending: map[uint64]*fakePending{}, next: 100, clock: 1_700_000_000_000,
		friends: map[string][]string{}, bucket: newFakeBucket(), titles: map[string]*tspb.UpsertTitleRequest{},
		mapped: map[string]string{}, calls: map[string]int{}}
}

func (f *fakeTS) hit(m string) error {
	f.calls[m]++
	if f.down {
		return status.Error(codes.Unavailable, "down")
	}
	return nil
}

func (f *fakeTS) now() int64 { f.clock += 10; return f.clock }

func (f *fakeTS) friendOf(a, b string) bool {
	for _, x := range f.friends[b] {
		if x == a {
			return true
		}
	}
	return false
}

func (f *fakeTS) visible(viewer string, o *tspb.Object) bool {
	if viewer != "" && viewer == o.Owner {
		return true
	}
	switch o.Visibility {
	case tspb.Visibility_VISIBILITY_PUBLIC:
		return true
	case tspb.Visibility_VISIBILITY_FRIENDS:
		return viewer != "" && f.friendOf(viewer, o.Owner)
	case tspb.Visibility_VISIBILITY_RECIPIENTS:
		if viewer == "" {
			return false
		}
		for _, r := range o.Recipients {
			if r == viewer {
				return true
			}
		}
		return o.RecipientsIncludeFriends && f.friendOf(viewer, o.Owner)
	}
	return false
}

func (f *fakeTS) render(x *fakeObj, ratings bool) *tspb.Object {
	o := proto.Clone(x.o).(*tspb.Object)
	if x.key != "" {
		o.GetUrl = f.bucket.srv.URL + "/title-storage/" + x.key
	}
	if ratings {
		slots := make([]int32, 0, len(x.ratings))
		for s := range x.ratings {
			slots = append(slots, s)
		}
		sort.Slice(slots, func(i, j int) bool { return slots[i] < slots[j] })
		for _, s := range slots {
			o.Ratings = append(o.Ratings, f.rating(o.Id, s, x.ratings[s], ""))
		}
	}
	return o
}

func (f *fakeTS) rating(id uint64, slot int32, r *fakeRating, rater string) *tspb.Rating {
	out := &tspb.Rating{Id: id, Slot: slot, Total: r.total, Count: r.count, InitialValue: r.policy.GetInitialValue(), Policy: r.policy}
	if v, ok := r.raters[rater]; ok {
		out.Rated, out.RaterValue = true, v
	}
	return out
}

func (f *fakeTS) MapTitle(_ context.Context, r *tspb.MapTitleRequest, _ ...grpc.CallOption) (*tspb.MapTitleResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("MapTitle"); err != nil {
		return nil, err
	}
	f.mapped[r.Network+"/"+r.NativeId] = r.Title
	return &tspb.MapTitleResponse{}, nil
}

func (f *fakeTS) UpsertTitle(_ context.Context, r *tspb.UpsertTitleRequest, _ ...grpc.CallOption) (*tspb.TitleInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("UpsertTitle"); err != nil {
		return nil, err
	}
	f.titles[r.Title] = r
	return &tspb.TitleInfo{Title: r.Title}, nil
}

func (f *fakeTS) PrepareUpload(_ context.Context, r *tspb.PrepareUploadRequest, _ ...grpc.CallOption) (*tspb.PrepareUploadResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("PrepareUpload"); err != nil {
		return nil, err
	}
	if r.Writer != r.Owner {
		return nil, status.Error(codes.PermissionDenied, "writer")
	}
	if r.Size == 0 {
		return nil, status.Error(codes.InvalidArgument, "size")
	}
	f.next++
	key := fmt.Sprintf("%d/k%d", f.next, f.next)
	f.pending[f.next] = &fakePending{req: r, key: key}
	return &tspb.PrepareUploadResponse{Id: f.next, Method: "POST", PutUrl: f.bucket.srv.URL + "/title-storage/",
		FormFields: map[string]string{"key": key, "policy": "p", "x-amz-signature": "s"}}, nil
}

func (f *fakeTS) CompleteUpload(_ context.Context, r *tspb.CompleteUploadRequest, _ ...grpc.CallOption) (*tspb.ObjectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("CompleteUpload"); err != nil {
		return nil, err
	}
	p, ok := f.pending[r.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "no upload")
	}
	if r.Writer != p.req.Owner {
		return nil, status.Error(codes.PermissionDenied, "writer")
	}
	f.bucket.mu.Lock()
	body, ok := f.bucket.data[p.key]
	f.bucket.mu.Unlock()
	if !ok || uint64(len(body)) != p.req.Size {
		return nil, status.Error(codes.FailedPrecondition, "size")
	}
	delete(f.pending, r.Id)
	now := f.now()
	o := &tspb.Object{Id: r.Id, Title: "mario-kart-8", Owner: p.req.Owner, Collection: p.req.Collection, Name: strconv.FormatUint(r.Id, 10),
		Info: p.req.Info, MetaJson: p.req.MetaJson, Size: p.req.Size, Setter: p.req.Setter, Visibility: p.req.Visibility,
		Network: "wiiu", CreatedUnixMs: now, UpdatedUnixMs: now, Blob: true, Recipients: p.req.Recipients,
		RecipientsIncludeFriends: p.req.RecipientsIncludeFriends}
	f.objs[r.Id] = &fakeObj{o: o, key: p.key, ratings: map[int32]*fakeRating{}}
	return &tspb.ObjectResponse{Object: f.render(f.objs[r.Id], false), Found: true}, nil
}

func (f *fakeTS) PutObject(_ context.Context, r *tspb.PutObjectRequest, _ ...grpc.CallOption) (*tspb.ObjectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("PutObject"); err != nil {
		return nil, err
	}
	if r.Writer != r.Owner {
		return nil, status.Error(codes.PermissionDenied, "writer")
	}
	f.next++
	now := f.now()
	name := r.Name
	if name == "" {
		name = strconv.FormatUint(f.next, 10)
	}
	o := &tspb.Object{Id: f.next, Title: "mario-kart-8", Owner: r.Owner, Collection: r.Collection, Name: name, Info: r.Info,
		MetaJson: r.MetaJson, Body: r.Body, Size: uint64(len(r.Body)), Setter: r.Setter, Visibility: r.Visibility, Network: "wiiu",
		CreatedUnixMs: now, UpdatedUnixMs: now, Recipients: r.Recipients, RecipientsIncludeFriends: r.RecipientsIncludeFriends}
	f.objs[f.next] = &fakeObj{o: o, ratings: map[int32]*fakeRating{}}
	return &tspb.ObjectResponse{Object: f.render(f.objs[f.next], false), Found: true}, nil
}

func (f *fakeTS) GetObjectByID(_ context.Context, r *tspb.GetObjectByIDRequest, _ ...grpc.CallOption) (*tspb.ObjectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("GetObjectByID"); err != nil {
		return nil, err
	}
	x, ok := f.objs[r.Id]
	if !ok || !f.visible(r.Viewer, x.o) {
		return &tspb.ObjectResponse{}, nil
	}
	return &tspb.ObjectResponse{Object: f.render(x, r.IncludeRatings), Found: true}, nil
}

// contains is jsonb @> for the shapes the adapter sends: scalars and arrays of strings.
func contains(have, want map[string]any) bool {
	for k, w := range want {
		h, ok := have[k]
		if !ok {
			return false
		}
		if wa, isArr := w.([]any); isArr {
			ha, _ := h.([]any)
			for _, x := range wa {
				found := false
				for _, y := range ha {
					if x == y {
						found = true
					}
				}
				if !found {
					return false
				}
			}
			continue
		}
		if h != w {
			return false
		}
	}
	return true
}

func (f *fakeTS) ListObjects(_ context.Context, r *tspb.ListObjectsRequest, _ ...grpc.CallOption) (*tspb.ListObjectsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("ListObjects"); err != nil {
		return nil, err
	}
	n := 0
	if len(r.Owners) > 0 {
		n++
	}
	if r.FriendsOfViewer {
		n++
	}
	if r.AllOwners {
		n++
	}
	if n != 1 {
		return nil, status.Error(codes.InvalidArgument, "scope")
	}
	var want, ranges map[string]any
	if r.MetaFilterJson != "" {
		if err := json.Unmarshal([]byte(r.MetaFilterJson), &want); err != nil {
			return nil, status.Error(codes.InvalidArgument, "filter")
		}
	}
	if r.MetaRangeJson != "" {
		if err := json.Unmarshal([]byte(r.MetaRangeJson), &ranges); err != nil {
			return nil, status.Error(codes.InvalidArgument, "range")
		}
	}
	var hits []*fakeObj
	for _, x := range f.objs {
		o := x.o
		if !f.visible(r.Viewer, o) {
			continue
		}
		switch {
		case len(r.Owners) > 0:
			in := false
			for _, ow := range r.Owners {
				in = in || ow == o.Owner
			}
			if !in {
				continue
			}
		case r.FriendsOfViewer:
			if !f.friendOf(o.Owner, r.Viewer) && !(r.IncludeSelf && o.Owner == r.Viewer) {
				continue
			}
		}
		skip := false
		for _, ex := range r.ExcludeOwners {
			skip = skip || ex == o.Owner
		}
		if r.Received {
			named := false
			for _, rc := range o.Recipients {
				named = named || rc == r.Viewer
			}
			skip = skip || !named
		}
		if r.Collection != "" || len(r.Collections) > 0 {
			in := o.Collection == r.Collection
			for _, c := range r.Collections {
				in = in || c == o.Collection
			}
			skip = skip || !in
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(o.MetaJson), &m)
		if want != nil && !contains(m, want) {
			skip = true
		}
		for k, b := range ranges {
			v, isNum := m[k].(float64)
			if !isNum {
				skip = true
				continue
			}
			bm := b.(map[string]any)
			if ge, ok := bm["ge"].(float64); ok && v < ge {
				skip = true
			}
			if le, ok := bm["le"].(float64); ok && v > le {
				skip = true
			}
		}
		if (r.CreatedAfterMs != 0 && o.CreatedUnixMs < r.CreatedAfterMs) || (r.CreatedBeforeMs != 0 && o.CreatedUnixMs > r.CreatedBeforeMs) ||
			(r.UpdatedAfterMs != 0 && o.UpdatedUnixMs < r.UpdatedAfterMs) || (r.UpdatedBeforeMs != 0 && o.UpdatedUnixMs > r.UpdatedBeforeMs) {
			skip = true
		}
		if strings.HasPrefix(r.Sort, "rating:") && r.MinRatingCount > 0 {
			slot, _ := strconv.Atoi(strings.TrimPrefix(r.Sort, "rating:"))
			if rt, ok := x.ratings[int32(slot)]; !ok || rt.count < r.MinRatingCount {
				skip = true
			}
		}
		if !skip {
			hits = append(hits, x)
		}
	}
	key := func(x *fakeObj) int64 {
		switch {
		case r.Sort == "created":
			return x.o.CreatedUnixMs
		case r.Sort == "size":
			return int64(x.o.Size)
		case r.Sort == "id":
			return int64(x.o.Id)
		case strings.HasPrefix(r.Sort, "rating:"):
			slot, _ := strconv.Atoi(strings.TrimPrefix(r.Sort, "rating:"))
			if rt, ok := x.ratings[int32(slot)]; ok {
				return rt.total
			}
			return -1 << 62
		}
		return x.o.UpdatedUnixMs
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := key(hits[i]), key(hits[j])
		if a == b {
			return hits[i].o.Id < hits[j].o.Id
		}
		if r.Ascending {
			return a < b
		}
		return a > b
	})
	out := &tspb.ListObjectsResponse{Hits: uint32(len(hits))}
	limit := int(r.Limit)
	if limit == 0 {
		limit = 100
	}
	for i := int(r.Offset); i < len(hits) && len(out.Objects) < limit; i++ {
		out.Objects = append(out.Objects, f.render(hits[i], r.IncludeRatings))
	}
	return out, nil
}

func (f *fakeTS) DeleteObjects(_ context.Context, r *tspb.DeleteObjectsRequest, _ ...grpc.CallOption) (*tspb.DeleteResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("DeleteObjects"); err != nil {
		return nil, err
	}
	if r.Writer != r.Owner {
		return nil, status.Error(codes.PermissionDenied, "writer")
	}
	var n uint32
	for id, x := range f.objs {
		if x.o.Owner != r.Owner || x.o.Collection != r.Collection {
			continue
		}
		for _, name := range r.Names {
			if name == x.o.Name {
				delete(f.objs, id)
				n++
			}
		}
	}
	return &tspb.DeleteResponse{Deleted: n}, nil
}

func (f *fakeTS) UpdateObjectMeta(_ context.Context, r *tspb.UpdateObjectMetaRequest, _ ...grpc.CallOption) (*tspb.ObjectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("UpdateObjectMeta"); err != nil {
		return nil, err
	}
	x, ok := f.objs[r.Id]
	if !ok {
		return &tspb.ObjectResponse{}, nil
	}
	if r.Writer != x.o.Owner {
		return nil, status.Error(codes.PermissionDenied, "writer")
	}
	if r.Info != nil {
		x.o.Info = r.Info
	}
	if r.MetaMergeJson != "" {
		var m, add map[string]any
		_ = json.Unmarshal([]byte(x.o.MetaJson), &m)
		if err := json.Unmarshal([]byte(r.MetaMergeJson), &add); err != nil {
			return nil, status.Error(codes.InvalidArgument, "merge")
		}
		for k, v := range add {
			m[k] = v
		}
		b, _ := json.Marshal(m)
		x.o.MetaJson = string(b)
	}
	if r.Collection != nil {
		x.o.Collection = *r.Collection
	}
	if r.Visibility != tspb.Visibility_VISIBILITY_UNSPECIFIED {
		x.o.Visibility, x.o.Recipients, x.o.RecipientsIncludeFriends = r.Visibility, r.Recipients, r.RecipientsIncludeFriends
	}
	if r.Setter != "" {
		x.o.Setter = r.Setter
	}
	x.o.UpdatedUnixMs = f.now()
	return &tspb.ObjectResponse{Object: f.render(x, false), Found: true}, nil
}

func (f *fakeTS) InitRating(_ context.Context, r *tspb.InitRatingRequest, _ ...grpc.CallOption) (*tspb.RatingResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("InitRating"); err != nil {
		return nil, err
	}
	x, ok := f.objs[r.Id]
	if !ok {
		return &tspb.RatingResponse{}, nil
	}
	if r.Writer != x.o.Owner {
		return nil, status.Error(codes.PermissionDenied, "writer")
	}
	if _, ok := x.ratings[r.Slot]; !ok {
		x.ratings[r.Slot] = &fakeRating{policy: r.Policy, total: r.Policy.GetInitialValue(), raters: map[string]int64{}}
	}
	return &tspb.RatingResponse{Rating: f.rating(r.Id, r.Slot, x.ratings[r.Slot], ""), Found: true}, nil
}

func (f *fakeTS) RateObject(_ context.Context, r *tspb.RateObjectRequest, _ ...grpc.CallOption) (*tspb.RatingResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.hit("RateObject"); err != nil {
		return nil, err
	}
	x, ok := f.objs[r.Id]
	if !ok || !f.visible(r.Rater, x.o) {
		return &tspb.RatingResponse{}, nil
	}
	rt, ok := x.ratings[r.Slot]
	if !ok {
		if r.Policy == nil {
			return nil, status.Error(codes.InvalidArgument, "no slot")
		}
		rt = &fakeRating{policy: r.Policy, total: r.Policy.GetInitialValue(), raters: map[string]int64{}}
		x.ratings[r.Slot] = rt
	}
	if rt.policy.GetDisableSelfRating() && r.Rater == x.o.Owner {
		return nil, status.Error(codes.PermissionDenied, "self")
	}
	if (rt.policy.RangeMin != nil && r.Value < *rt.policy.RangeMin) || (rt.policy.RangeMax != nil && r.Value > *rt.policy.RangeMax) {
		return nil, status.Error(codes.InvalidArgument, "range")
	}
	if _, rated := rt.raters[r.Rater]; rated && rt.policy.GetLock() == "permanent" {
		return &tspb.RatingResponse{Rating: f.rating(r.Id, r.Slot, rt, r.Rater), Found: true, Locked: true}, nil
	}
	rt.total += r.Value
	rt.count++
	rt.raters[r.Rater] += r.Value
	return &tspb.RatingResponse{Rating: f.rating(r.Id, r.Slot, rt, r.Rater), Found: true}, nil
}

// fakeIDs links PIDs 1-99 (and any in extra) to accounts; others have no link.
type fakeIDs struct{ down bool }

func acct(pid uint64) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", pid) }

func (f *fakeIDs) Account(_ context.Context, pid uint64) (string, error) {
	if f.down {
		return "", status.Error(codes.Unavailable, "down")
	}
	if pid == 0 || pid >= 100 {
		return "", coreid.ErrNoLink
	}
	return acct(pid), nil
}

func (f *fakeIDs) PID(_ context.Context, account string) (uint64, error) {
	var p uint64
	if _, err := fmt.Sscanf(account, "00000000-0000-0000-0000-%012d", &p); err != nil {
		return 0, coreid.ErrNoLink
	}
	return p, nil
}

// upload is the console's half of a post: the form fields, then the file.
func upload(url string, fields map[string]string, body []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = mw.WriteField(k, fields[k])
	}
	fw, _ := mw.CreateFormFile("file", "file")
	_, _ = fw.Write(body)
	_ = mw.Close()
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Post(url, mw.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload: HTTP %d %s", resp.StatusCode, msg)
	}
	return nil
}

func download(url string) ([]byte, error) {
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
