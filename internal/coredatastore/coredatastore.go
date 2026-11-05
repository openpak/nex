// Package coredatastore is the NEX DataStore (0x73) store over the OpenPak core: objects, their
// bodies and their ratings live in title-storage. A title flipped to it (NEX_CORE_STORAGE
// contains "datastore", docs/core-backed-storage.md) keeps no DataStore table of its own.
//
// It owns the DataStore method handlers itself (wire.go) instead of filling the common
// protocol's callbacks: those callbacks do not carry the caller, and every core read needs the
// caller's account as the viewer (visibility is the core's job, not a re-check here).
//
// Mapping (title-storage v0.7.0; title = the NEX_TITLE name, network wiiu/3ds, per network):
//
//   - data id          -> object id (stable, assigned by PrepareUpload / PutObject)
//   - owner PID        -> owner account (nn-account Resolution, cached); the PID is kept in
//     meta_json so a read needs no reverse lookup
//   - data type        -> collection (decimal) and meta_json.data_type
//   - meta binary      -> info
//   - name, period, status, flag, tags, refer data id, persistence slot, permissions with
//     their recipient PIDs, size, rating init params -> meta_json (the wire echoes them)
//   - permission 0/1/2/3/4 -> visibility public/friends/recipients/private/recipients+friends,
//     recipients = RecipientIDs mapped to accounts (an unlinked PID is left out)
//   - body             -> blob tier: PrepareUpload(post_form) / CompleteUpload, get_url on reads
//   - ratings          -> InitRating / RateObject / include_ratings
//   - ChangeMeta       -> UpdateObjectMeta (keeps the body and the ratings)
//   - SearchObject     -> ListObjects
//
// No pending rows: every DataStore write is answered with a data id or a URL the console uses at
// once, so a write the core cannot take (no account link, core down) is a NEX error to the
// console, never a local row.
package coredatastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	dstypes "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openpak/nex/internal/coreid"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// noPersistenceSlot is the persistence slot meaning "not persisted".
const noPersistenceSlot = 0xFFFF

// searchLimit caps one SearchObject page, as the local stores do.
const searchLimit = 100

// Config is a title's place in the core.
type Config struct {
	Title   string // neutral key and native id on Network ("mario-kart-8")
	Name    string
	Network string // "wiiu" or "3ds"
	// MaxObjectBytes raises the title's per-object cap in title-storage (0 keeps the service
	// default, 4 MiB).
	MaxObjectBytes uint32
}

// Store is the core-backed DataStore of one title.
type Store struct {
	Cfg Config
	TS  tspb.TitleStorageClient
	IDs coreid.Resolver
	// Friends is the caller's friend PIDs (nn-friends); only SearchTypePublicExcludeOwnAndFriends
	// needs it. Nil = no friends.
	Friends func(pid uint32) []uint32
	// Timeout bounds every core call (default 3 s).
	Timeout time.Duration
}

func (s *Store) ctx() (context.Context, context.CancelFunc) {
	t := s.Timeout
	if t == 0 {
		t = 3 * time.Second
	}
	return context.WithTimeout(context.Background(), t)
}

func (s *Store) ref() *tspb.TitleRef {
	return &tspb.TitleRef{Network: s.Cfg.Network, Title: s.Cfg.Title}
}

// Register maps the title in title-storage (neutral key = native id), makes it per-network and
// sets its object cap. Safe on every start; coreranking registers the same title the same way.
func (s *Store) Register(ctx context.Context) error {
	if _, err := s.TS.MapTitle(ctx, &tspb.MapTitleRequest{Title: s.Cfg.Title, Name: s.Cfg.Name, Network: s.Cfg.Network, NativeId: s.Cfg.Title}); err != nil {
		return fmt.Errorf("title-storage MapTitle: %w", err)
	}
	if _, err := s.TS.UpsertTitle(ctx, &tspb.UpsertTitleRequest{Title: s.Cfg.Title, Name: s.Cfg.Name,
		CrossNetwork: tspb.Tristate_TRISTATE_FALSE, MaxObjectBytes: s.Cfg.MaxObjectBytes}); err != nil {
		return fmt.Errorf("title-storage UpsertTitle: %w", err)
	}
	return nil
}

// ---- metadata ----

// ratingInit is a DataStoreRatingInitParamWithSlot as kept in meta_json until the object is
// complete (a pending upload cannot hold rating slots yet).
type ratingInit struct {
	Slot         int8  `json:"slot"`
	Flag         uint8 `json:"flag"`
	InternalFlag uint8 `json:"internal_flag"`
	LockType     uint8 `json:"lock_type"`
	InitialValue int64 `json:"initial_value"`
	RangeMin     int32 `json:"range_min"`
	RangeMax     int32 `json:"range_max"`
	PeriodHour   int8  `json:"period_hour"`
	PeriodDay    int16 `json:"period_duration"`
}

// meta is what meta_json holds for a DataStore object. Keys a search filters on are always
// present: data_type (it also tells a DataStore object from a title's other objects),
// permission, refer_data_id, tags and persistence_slot.
type meta struct {
	DataType        uint16       `json:"data_type"`
	Name            string       `json:"name"`
	OwnerPID        uint64       `json:"owner_pid"`
	Size            uint32       `json:"size"`
	Permission      uint8        `json:"permission"`
	RecipientPIDs   []uint64     `json:"recipient_pids,omitempty"`
	DelPermission   uint8        `json:"del_permission"`
	DelRecipients   []uint64     `json:"del_recipient_pids,omitempty"`
	Period          uint16       `json:"period"`
	Status          uint8        `json:"status"`
	ReferDataID     uint32       `json:"refer_data_id"`
	Flag            uint32       `json:"flag"`
	Tags            []string     `json:"tags"`
	PersistenceSlot uint16       `json:"persistence_slot"` // 65535: none (always present, so a search can ask for none)
	RatingInit      []ratingInit `json:"rating_init,omitempty"`
}

func pids(l types.List[types.PID]) []uint64 {
	out := make([]uint64, 0, len(l))
	for _, p := range l {
		out = append(out, uint64(p))
	}
	return out
}

func strs(l types.List[types.String]) []string {
	out := make([]string, 0, len(l))
	for _, v := range l {
		out = append(out, string(v))
	}
	return out
}

func metaFromPost(owner uint64, p dstypes.DataStorePreparePostParam) meta {
	m := meta{
		DataType: uint16(p.DataType), Name: string(p.Name), OwnerPID: owner, Size: uint32(p.Size),
		Permission: uint8(p.Permission.Permission), RecipientPIDs: pids(p.Permission.RecipientIDs),
		DelPermission: uint8(p.DelPermission.Permission), DelRecipients: pids(p.DelPermission.RecipientIDs),
		Period: uint16(p.Period), ReferDataID: uint32(p.ReferDataID), Flag: uint32(p.Flag), Tags: strs(p.Tags),
	}
	if p.Flag.HasFlag(constants.DataFlagNeedReview) {
		m.Status = uint8(constants.DataStatusPending)
	}
	m.PersistenceSlot = uint16(p.PersistenceInitParam.PersistenceSlotID)
	for _, r := range p.RatingInitParams {
		m.RatingInit = append(m.RatingInit, ratingInit{Slot: int8(r.Slot), Flag: uint8(r.Param.Flag),
			InternalFlag: uint8(r.Param.InternalFlag), LockType: uint8(r.Param.LockType),
			InitialValue: int64(r.Param.InitialValue), RangeMin: int32(r.Param.RangeMin), RangeMax: int32(r.Param.RangeMax),
			PeriodHour: int8(r.Param.PeriodHour), PeriodDay: int16(r.Param.PeriodDuration)})
	}
	return m
}

func (m meta) json() string {
	if m.Tags == nil {
		m.Tags = []string{}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// decodeMeta reads a DataStore object's metadata; ok is false for any other object of the
// title (ranking common data, a variable's object).
func decodeMeta(o *tspb.Object) (meta, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(o.GetMetaJson()), &raw) != nil {
		return meta{}, false
	}
	if _, ok := raw["data_type"]; !ok {
		return meta{}, false
	}
	var m meta
	if json.Unmarshal([]byte(o.GetMetaJson()), &m) != nil {
		return meta{}, false
	}
	return m, true
}

// policy is a rating slot's NEX init param as a title-storage policy (adapter-guide.md).
func (r ratingInit) policy() *tspb.RatingPolicy {
	p := &tspb.RatingPolicy{InitialValue: r.InitialValue,
		Modifiable:        constants.RatingFlag(r.Flag).HasFlag(constants.RatingFlagModifiable),
		RoundMinus:        constants.RatingFlag(r.Flag).HasFlag(constants.RatingFlagRoundMinus),
		DisableSelfRating: constants.RatingFlag(r.Flag).HasFlag(constants.RatingFlagDisableSelfRating)}
	switch constants.RatingLockType(r.LockType) {
	case constants.RatingLockInterval:
		p.Lock, p.IntervalSeconds = "interval", uint32(r.PeriodDay)
	case constants.RatingLockPeriod:
		p.Lock, p.PeriodDay, p.PeriodHour = "period", int32(r.PeriodDay), int32(r.PeriodHour)
	case constants.RatingLockPermanent:
		p.Lock = "permanent"
	default:
		p.Lock = "none"
	}
	if constants.RatingInternalFlag(r.InternalFlag).HasFlag(constants.RatingInternalFlagUseRangeMin) {
		v := int64(r.RangeMin)
		p.RangeMin = &v
	}
	if constants.RatingInternalFlag(r.InternalFlag).HasFlag(constants.RatingInternalFlagUseRangeMax) {
		v := int64(r.RangeMax)
		p.RangeMax = &v
	}
	return p
}

// visibility maps a NEX permission onto title-storage.
func visibility(p uint8) (tspb.Visibility, bool) {
	switch constants.Permission(p) {
	case constants.PermissionPublic:
		return tspb.Visibility_VISIBILITY_PUBLIC, false
	case constants.PermissionFriend:
		return tspb.Visibility_VISIBILITY_FRIENDS, false
	case constants.PermissionSpecified:
		return tspb.Visibility_VISIBILITY_RECIPIENTS, false
	case constants.PermissionSpecifiedFriend:
		return tspb.Visibility_VISIBILITY_RECIPIENTS, true
	default:
		return tspb.Visibility_VISIBILITY_PRIVATE, false
	}
}

// accounts maps PIDs to accounts, leaving out the ones with no link (they cannot be a
// recipient or a search owner). A core failure is an error.
func (s *Store) accounts(ctx context.Context, ps []uint64) ([]string, error) {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		a, err := s.IDs.Account(ctx, p)
		if errors.Is(err, coreid.ErrNoLink) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// viewer is the caller's account, or "" (anonymous: public objects only) without a link.
func (s *Store) viewer(ctx context.Context, pid uint64) (string, error) {
	a, err := s.IDs.Account(ctx, pid)
	if errors.Is(err, coreid.ErrNoLink) {
		return "", nil
	}
	return a, err
}

// ---- errors ----

// nexErr turns a core failure into the DataStore code the console acts on.
func nexErr(err error) *nex.Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, coreid.ErrNoLink) {
		return nex.NewError(nex.ResultCodes.DataStore.PermissionDenied, "no OpenPak account link")
	}
	var ne *nex.Error
	if errors.As(err, &ne) {
		return ne
	}
	switch status.Code(err) {
	case codes.NotFound:
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, err.Error())
	case codes.InvalidArgument, codes.AlreadyExists:
		return nex.NewError(nex.ResultCodes.DataStore.InvalidArgument, err.Error())
	case codes.PermissionDenied:
		return nex.NewError(nex.ResultCodes.DataStore.PermissionDenied, err.Error())
	case codes.ResourceExhausted:
		return nex.NewError(nex.ResultCodes.DataStore.OverCapacity, err.Error())
	case codes.FailedPrecondition:
		return nex.NewError(nex.ResultCodes.DataStore.OperationNotAllowed, err.Error())
	}
	log.Printf("core datastore: %v", err)
	return nex.NewError(nex.ResultCodes.Core.SystemError, err.Error())
}

func notFound(what string) *nex.Error {
	return nex.NewError(nex.ResultCodes.DataStore.NotFound, what)
}

// ---- reads ----

func dateTime(ms int64) types.DateTime {
	d := types.NewDateTime(0)
	return d.FromTimestamp(time.UnixMilli(ms).UTC())
}

func permission(p uint8, recipients []uint64) dstypes.DataStorePermission {
	out := dstypes.NewDataStorePermission()
	out.Permission = constants.Permission(p)
	out.RecipientIDs = types.NewList[types.PID]()
	for _, r := range recipients {
		out.RecipientIDs = append(out.RecipientIDs, types.NewPID(r))
	}
	return out
}

// metaInfo renders a core object as the wire's DataStoreMetaInfo.
func metaInfo(o *tspb.Object, m meta) dstypes.DataStoreMetaInfo {
	mi := dstypes.NewDataStoreMetaInfo()
	mi.DataID = types.NewUInt64(o.GetId())
	mi.OwnerID = types.NewPID(m.OwnerPID)
	size := m.Size
	if o.GetBlob() {
		size = uint32(o.GetSize())
	}
	mi.Size = types.NewUInt32(size)
	mi.Name = types.NewString(m.Name)
	mi.DataType = types.NewUInt16(m.DataType)
	mi.MetaBinary = types.NewQBuffer(o.GetInfo())
	mi.Permission = permission(m.Permission, m.RecipientPIDs)
	mi.DelPermission = permission(m.DelPermission, m.DelRecipients)
	mi.CreatedTime = dateTime(o.GetCreatedUnixMs())
	mi.UpdatedTime = dateTime(o.GetUpdatedUnixMs())
	mi.Period = types.NewUInt16(m.Period)
	mi.Status = constants.DataStatus(m.Status)
	mi.ReferredCnt = types.NewUInt32(0)
	mi.ReferDataID = types.NewUInt32(m.ReferDataID)
	mi.Flag = constants.DataFlag(m.Flag)
	mi.ReferredTime = mi.UpdatedTime
	// period is in days; 0 means never, which NEX spells as the far future (as the local stores).
	created := time.UnixMilli(o.GetCreatedUnixMs()).UTC()
	if m.Period == 0 {
		mi.ExpireTime = dateTime(created.AddDate(100, 0, 0).UnixMilli())
	} else {
		mi.ExpireTime = dateTime(created.AddDate(0, 0, int(m.Period)).UnixMilli())
	}
	mi.Tags = types.NewList[types.String]()
	for _, t := range m.Tags {
		mi.Tags = append(mi.Tags, types.NewString(t))
	}
	mi.Ratings = types.NewList[dstypes.DataStoreRatingInfoWithSlot]()
	for _, r := range o.GetRatings() {
		w := dstypes.NewDataStoreRatingInfoWithSlot()
		w.Slot = types.NewInt8(int8(r.GetSlot()))
		w.Rating = ratingInfo(r)
		mi.Ratings = append(mi.Ratings, w)
	}
	return mi
}

func ratingInfo(r *tspb.Rating) dstypes.DataStoreRatingInfo {
	ri := dstypes.NewDataStoreRatingInfo()
	ri.TotalValue = types.NewInt64(r.GetTotal())
	ri.Count = types.NewUInt32(r.GetCount())
	ri.InitialValue = types.NewInt64(r.GetInitialValue())
	return ri
}

// object reads one DataStore object as viewer; not found (or not visible, or not a DataStore
// object) is DataStore::NotFound.
func (s *Store) object(ctx context.Context, viewer string, id uint64) (*tspb.Object, meta, *nex.Error) {
	r, err := s.TS.GetObjectByID(ctx, &tspb.GetObjectByIDRequest{Network: s.Cfg.Network, Viewer: viewer, Id: id, IncludeRatings: true})
	if err != nil {
		return nil, meta{}, nexErr(err)
	}
	if !r.GetFound() {
		return nil, meta{}, notFound("no such object")
	}
	m, ok := decodeMeta(r.GetObject())
	if !ok {
		return nil, meta{}, notFound("no such object")
	}
	return r.GetObject(), m, nil
}

// persistent finds the live object in an owner's persistence slot.
func (s *Store) persistent(ctx context.Context, viewer string, t dstypes.DataStorePersistenceTarget) (*tspb.Object, meta, *nex.Error) {
	owner, err := s.IDs.Account(ctx, uint64(t.OwnerID))
	if errors.Is(err, coreid.ErrNoLink) {
		return nil, meta{}, notFound("no such object")
	}
	if err != nil {
		return nil, meta{}, nexErr(err)
	}
	filter, _ := json.Marshal(map[string]any{"persistence_slot": uint16(t.PersistenceSlotID)})
	r, err := s.TS.ListObjects(ctx, &tspb.ListObjectsRequest{Ref: s.ref(), Viewer: viewer, Owners: []string{owner},
		MetaFilterJson: string(filter), Sort: "updated", Limit: 1, IncludeRatings: true})
	if err != nil {
		return nil, meta{}, nexErr(err)
	}
	for _, o := range r.GetObjects() {
		if m, ok := decodeMeta(o); ok {
			return o, m, nil
		}
	}
	return nil, meta{}, notFound("no such object")
}

// GetMeta answers one DataStoreGetMetaParam (by data id, or by persistence target when the
// data id is 0, as the real server does).
func (s *Store) GetMeta(caller uint64, p dstypes.DataStoreGetMetaParam) (dstypes.DataStoreMetaInfo, *nex.Error) {
	ctx, cancel := s.ctx()
	defer cancel()
	viewer, err := s.viewer(ctx, caller)
	if err != nil {
		return dstypes.NewDataStoreMetaInfo(), nexErr(err)
	}
	var o *tspb.Object
	var m meta
	var ne *nex.Error
	if p.DataID == 0 {
		o, m, ne = s.persistent(ctx, viewer, p.PersistenceTarget)
	} else {
		o, m, ne = s.object(ctx, viewer, uint64(p.DataID))
	}
	if ne != nil {
		return dstypes.NewDataStoreMetaInfo(), ne
	}
	mi := metaInfo(o, m)
	mi.FilterPropertiesByResultOption(p.ResultOption)
	return mi, nil
}

// GetMetas answers GetMetas: one result per id, a failed one as an empty meta and its code.
func (s *Store) GetMetas(caller uint64, ids []uint64, p dstypes.DataStoreGetMetaParam) ([]dstypes.DataStoreMetaInfo, []*nex.Error) {
	out := make([]dstypes.DataStoreMetaInfo, len(ids))
	errs := make([]*nex.Error, len(ids))
	for i, id := range ids {
		q := p
		q.DataID = types.NewUInt64(id)
		out[i], errs[i] = s.GetMeta(caller, q)
	}
	return out, errs
}

// PrepareGet answers PrepareGetObject with the core's presigned GET.
func (s *Store) PrepareGet(caller uint64, p dstypes.DataStorePrepareGetParam) (dstypes.DataStoreReqGetInfo, *nex.Error) {
	info := dstypes.NewDataStoreReqGetInfo()
	ctx, cancel := s.ctx()
	defer cancel()
	viewer, err := s.viewer(ctx, caller)
	if err != nil {
		return info, nexErr(err)
	}
	var o *tspb.Object
	var ne *nex.Error
	if p.DataID == 0 {
		o, _, ne = s.persistent(ctx, viewer, p.PersistenceTarget)
	} else {
		o, _, ne = s.object(ctx, viewer, uint64(p.DataID))
	}
	if ne != nil {
		return info, ne
	}
	if o.GetGetUrl() == "" {
		return info, notFound("object has no body")
	}
	info.URL = types.NewString(o.GetGetUrl())
	info.RequestHeaders = types.NewList[dstypes.DataStoreKeyValue]()
	info.Size = types.NewUInt32(uint32(o.GetSize()))
	info.RootCACert = types.NewBuffer(nil)
	info.DataID = types.NewUInt64(o.GetId())
	return info, nil
}

// ---- writes ----

// retire deletes the owner's live object in a persistence slot before a new one takes it
// (whatever DeleteLastObject says, as the local stores: a slot holds one object).
func (s *Store) retire(ctx context.Context, owner string, slot uint16) error {
	filter, _ := json.Marshal(map[string]any{"persistence_slot": slot})
	r, err := s.TS.ListObjects(ctx, &tspb.ListObjectsRequest{Ref: s.ref(), Viewer: owner, Owners: []string{owner},
		MetaFilterJson: string(filter), Limit: 100})
	if err != nil {
		return err
	}
	for _, o := range r.GetObjects() {
		if _, ok := decodeMeta(o); !ok {
			continue
		}
		if _, err := s.TS.DeleteObjects(ctx, &tspb.DeleteObjectsRequest{Ref: s.ref(), Owner: owner, Writer: owner,
			Collection: o.GetCollection(), Names: []string{o.GetName()}}); err != nil {
			return err
		}
	}
	return nil
}

func collection(dataType uint16) string { return strconv.FormatUint(uint64(dataType), 10) }

// PreparePost answers PreparePostObject: the core reserves the object and presigns the
// console's POST to the bucket.
func (s *Store) PreparePost(caller uint64, p dstypes.DataStorePreparePostParam) (dstypes.DataStoreReqPostInfo, *nex.Error) {
	info := dstypes.NewDataStoreReqPostInfo()
	ctx, cancel := s.ctx()
	defer cancel()
	owner, err := s.IDs.Account(ctx, caller)
	if err != nil {
		return info, nexErr(err)
	}
	m := metaFromPost(caller, p)
	if m.PersistenceSlot != noPersistenceSlot {
		if err := s.retire(ctx, owner, m.PersistenceSlot); err != nil {
			return info, nexErr(err)
		}
	}
	vis, withFriends := visibility(m.Permission)
	recipients, err := s.accounts(ctx, m.RecipientPIDs)
	if err != nil {
		return info, nexErr(err)
	}
	r, err := s.TS.PrepareUpload(ctx, &tspb.PrepareUploadRequest{Ref: s.ref(), Owner: owner, Writer: owner,
		Setter: strconv.FormatUint(caller, 10), Collection: collection(m.DataType), Size: uint64(p.Size),
		Info: []byte(p.MetaBinary), MetaJson: m.json(), Visibility: vis, PostForm: true,
		Recipients: recipients, RecipientsIncludeFriends: withFriends})
	if err != nil {
		return info, nexErr(err)
	}
	info.DataID = types.NewUInt64(r.GetId())
	info.URL = types.NewString(r.GetPutUrl())
	info.RequestHeaders = types.NewList[dstypes.DataStoreKeyValue]()
	info.FormFields = types.NewList[dstypes.DataStoreKeyValue]()
	keys := make([]string, 0, len(r.GetFormFields()))
	for k := range r.GetFormFields() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kv := dstypes.NewDataStoreKeyValue()
		kv.Key, kv.Value = types.NewString(k), types.NewString(r.GetFormFields()[k])
		info.FormFields = append(info.FormFields, kv)
	}
	info.RootCACert = types.NewBuffer(nil)
	return info, nil
}

// initRatings creates the rating slots the post asked for, once the object is visible.
func (s *Store) initRatings(ctx context.Context, writer string, id uint64, m meta) error {
	for _, r := range m.RatingInit {
		if _, err := s.TS.InitRating(ctx, &tspb.InitRatingRequest{Network: s.Cfg.Network, Id: id, Writer: writer,
			Slot: int32(r.Slot), Policy: r.policy()}); err != nil {
			return err
		}
	}
	return nil
}

// CompletePost answers CompletePostObject. A failed upload needs nothing: the pending upload
// expires in the core and its bytes go with it.
func (s *Store) CompletePost(caller uint64, p dstypes.DataStoreCompletePostParam) *nex.Error {
	if !p.IsSuccess {
		return nil
	}
	ctx, cancel := s.ctx()
	defer cancel()
	writer, err := s.IDs.Account(ctx, caller)
	if err != nil {
		return nexErr(err)
	}
	r, err := s.TS.CompleteUpload(ctx, &tspb.CompleteUploadRequest{Id: uint64(p.DataID), Writer: writer})
	if err != nil {
		return nexErr(err)
	}
	if m, ok := decodeMeta(r.GetObject()); ok {
		return nexErr(s.initRatings(ctx, writer, r.GetObject().GetId(), m))
	}
	return nil
}

// PostMetaBinary answers PostMetaBinary: an object with no body, visible at once.
func (s *Store) PostMetaBinary(caller uint64, p dstypes.DataStorePreparePostParam) (uint64, *nex.Error) {
	ctx, cancel := s.ctx()
	defer cancel()
	owner, err := s.IDs.Account(ctx, caller)
	if err != nil {
		return 0, nexErr(err)
	}
	m := metaFromPost(caller, p)
	m.Size = 0
	if m.PersistenceSlot != noPersistenceSlot {
		if err := s.retire(ctx, owner, m.PersistenceSlot); err != nil {
			return 0, nexErr(err)
		}
	}
	vis, withFriends := visibility(m.Permission)
	recipients, err := s.accounts(ctx, m.RecipientPIDs)
	if err != nil {
		return 0, nexErr(err)
	}
	r, err := s.TS.PutObject(ctx, &tspb.PutObjectRequest{Ref: s.ref(), Owner: owner, Writer: owner,
		Setter: strconv.FormatUint(caller, 10), Collection: collection(m.DataType), Info: []byte(p.MetaBinary),
		MetaJson: m.json(), Visibility: vis, Recipients: recipients, RecipientsIncludeFriends: withFriends})
	if err != nil {
		return 0, nexErr(err)
	}
	id := r.GetObject().GetId()
	return id, nexErr(s.initRatings(ctx, owner, id, m))
}

// mayModify is the NEX update permission (DelPermission) check the common protocol makes before
// a change or a delete: the owner always, otherwise what the permission names. The core's write
// rule applies on top.
func (s *Store) mayModify(caller uint64, m meta) *nex.Error {
	if caller == m.OwnerPID {
		return nil
	}
	switch constants.Permission(m.DelPermission) {
	case constants.PermissionPublic:
		return nil
	case constants.PermissionFriend:
		if s.Friends != nil {
			for _, f := range s.Friends(uint32(m.OwnerPID)) {
				if uint64(f) == caller {
					return nil
				}
			}
		}
	case constants.PermissionSpecified:
		for _, r := range m.DelRecipients {
			if r == caller {
				return nil
			}
		}
	}
	return nex.NewError(nex.ResultCodes.DataStore.PermissionDenied, "not allowed to change this object")
}

// ChangeMeta answers ChangeMeta with one metadata-only update (the body and ratings stay).
func (s *Store) ChangeMeta(caller uint64, p dstypes.DataStoreChangeMetaParam) *nex.Error {
	ctx, cancel := s.ctx()
	defer cancel()
	writer, err := s.IDs.Account(ctx, caller)
	if err != nil {
		return nexErr(err)
	}
	_, m, ne := s.object(ctx, writer, uint64(p.DataID))
	if ne != nil {
		return ne
	}
	if ne := s.mayModify(caller, m); ne != nil {
		return ne
	}
	f := p.ModifiesFlag
	req := &tspb.UpdateObjectMetaRequest{Network: s.Cfg.Network, Id: uint64(p.DataID), Writer: writer,
		Setter: strconv.FormatUint(caller, 10)}
	merge := map[string]any{}
	if f.HasFlag(constants.ModificationFlagName) {
		merge["name"] = string(p.Name)
	}
	if f.HasFlag(constants.ModificationFlagAccessPermission) {
		perm := uint8(p.Permission.Permission)
		rp := pids(p.Permission.RecipientIDs)
		merge["permission"], merge["recipient_pids"] = perm, rp
		vis, withFriends := visibility(perm)
		recipients, err := s.accounts(ctx, rp)
		if err != nil {
			return nexErr(err)
		}
		req.Visibility, req.Recipients, req.RecipientsIncludeFriends = vis, recipients, withFriends
	}
	if f.HasFlag(constants.ModificationFlagUpdatePermission) {
		merge["del_permission"], merge["del_recipient_pids"] = uint8(p.DelPermission.Permission), pids(p.DelPermission.RecipientIDs)
	}
	if f.HasFlag(constants.ModificationFlagPeriod) {
		merge["period"] = uint16(p.Period)
	}
	if f.HasFlag(constants.ModificationFlagMetaBinary) {
		req.Info = []byte(p.MetaBinary)
		if req.Info == nil {
			req.Info = []byte{}
		}
	}
	if f.HasFlag(constants.ModificationFlagTags) {
		merge["tags"] = strs(p.Tags)
	}
	if f.HasFlag(constants.ModificationFlagDataType) {
		merge["data_type"] = uint16(p.DataType)
		c := collection(uint16(p.DataType))
		req.Collection = &c
	}
	if f.HasFlag(constants.ModificationFlagStatus) {
		merge["status"] = uint8(p.Status)
	}
	if len(merge) == 0 && req.Info == nil && req.Collection == nil && req.Visibility == tspb.Visibility_VISIBILITY_UNSPECIFIED {
		return nil // only flags this store does not keep (updated time, referred count)
	}
	if len(merge) > 0 {
		b, _ := json.Marshal(merge)
		req.MetaMergeJson = string(b)
	}
	r, err := s.TS.UpdateObjectMeta(ctx, req)
	if err != nil {
		return nexErr(err)
	}
	if !r.GetFound() {
		return notFound("no such object")
	}
	return nil
}

// Delete answers DeleteObject.
func (s *Store) Delete(caller uint64, p dstypes.DataStoreDeleteParam) *nex.Error {
	ctx, cancel := s.ctx()
	defer cancel()
	writer, err := s.IDs.Account(ctx, caller)
	if err != nil {
		return nexErr(err)
	}
	o, m, ne := s.object(ctx, writer, uint64(p.DataID))
	if ne != nil {
		return ne
	}
	if ne := s.mayModify(caller, m); ne != nil {
		return ne
	}
	r, err := s.TS.DeleteObjects(ctx, &tspb.DeleteObjectsRequest{Ref: s.ref(), Owner: o.GetOwner(), Writer: writer,
		Collection: o.GetCollection(), Names: []string{o.GetName()}})
	if err != nil {
		return nexErr(err)
	}
	if r.GetDeleted() == 0 {
		return notFound("no such object")
	}
	return nil
}

// Rate answers one RateObject target. The slot's policy travels along, so a slot whose
// InitRating never ran is created with the policy the post asked for.
func (s *Store) Rate(caller uint64, t dstypes.DataStoreRatingTarget, p dstypes.DataStoreRateObjectParam) (dstypes.DataStoreRatingInfo, *nex.Error) {
	out := dstypes.NewDataStoreRatingInfo()
	ctx, cancel := s.ctx()
	defer cancel()
	rater, err := s.IDs.Account(ctx, caller)
	if err != nil {
		return out, nexErr(err)
	}
	_, m, ne := s.object(ctx, rater, uint64(t.DataID))
	if ne != nil {
		return out, ne
	}
	req := &tspb.RateObjectRequest{Network: s.Cfg.Network, Id: uint64(t.DataID), Slot: int32(t.Slot), Rater: rater, Value: int64(p.RatingValue)}
	for _, r := range m.RatingInit {
		if int32(r.Slot) == int32(t.Slot) {
			req.Policy = r.policy()
		}
	}
	r, err := s.TS.RateObject(ctx, req)
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return out, nex.NewError(nex.ResultCodes.DataStore.OperationNotAllowed, err.Error())
		}
		return out, nexErr(err)
	}
	if !r.GetFound() {
		return out, notFound("no such object")
	}
	if r.GetLocked() {
		return out, nex.NewError(nex.ResultCodes.DataStore.OperationNotAllowed, "rating locked")
	}
	return ratingInfo(r.GetRating()), nil
}

// ---- search ----

func (s *Store) friendAccounts(ctx context.Context, caller uint64) ([]string, error) {
	if s.Friends == nil {
		return nil, nil
	}
	fs := s.Friends(uint32(caller))
	ps := make([]uint64, 0, len(fs))
	for _, f := range fs {
		ps = append(ps, uint64(f))
	}
	return s.accounts(ctx, ps)
}

// searchRequest translates a DataStoreSearchParam (docs/core-backed-storage.md, D3). empty is
// true when the answer is known to be empty without asking (owners named, none linked).
func (s *Store) searchRequest(ctx context.Context, caller uint64, viewer string, p dstypes.DataStoreSearchParam) (*tspb.ListObjectsRequest, bool, error) {
	req := &tspb.ListObjectsRequest{Ref: s.ref(), Viewer: viewer, IncludeRatings: true}
	filter := map[string]any{}
	// Every search is over DataStore objects only: data_type is a number on those and absent on
	// a title's other objects (ranking common data).
	ranges := map[string]any{"data_type": map[string]any{}}

	self := func() bool {
		if viewer == "" {
			return false
		}
		req.Owners = []string{viewer}
		return true
	}
	scope := "all"
	switch p.SearchTarget {
	case constants.SearchTypePublic:
		filter["permission"] = uint8(constants.PermissionPublic)
	case constants.SearchTypeOwn, constants.SearchTypeOwnAll, constants.SearchTypePrivate,
		constants.SearchTypeOwnPending, constants.SearchTypeOwnRejected, constants.SearchTypeSend,
		constants.SearchTypeSendFriend, constants.SearchTypeSendSpecified, constants.SearchTypeSendSpecifiedFriend:
		if !self() {
			return nil, true, nil
		}
		scope = "self"
	case constants.SearchTypeFriend:
		scope = "friends"
	case constants.SearchTypeReceived, constants.SearchTypeReceivedSpecified:
		if viewer == "" {
			return nil, true, nil
		}
		req.Received = true
	case constants.SearchTypePublicExcludeOwnAndFriends:
		filter["permission"] = uint8(constants.PermissionPublic)
		ex, err := s.friendAccounts(ctx, caller)
		if err != nil {
			return nil, false, err
		}
		if viewer != "" {
			ex = append(ex, viewer)
		}
		req.ExcludeOwners = ex
	}
	switch p.OwnerType {
	case constants.SearchTargetFriend:
		if scope == "all" {
			scope = "friends"
		}
	case constants.SearchTargetAnybodyExcludeSpecified:
		ex, err := s.accounts(ctx, pids(p.OwnerIDs))
		if err != nil {
			return nil, false, err
		}
		req.ExcludeOwners = append(req.ExcludeOwners, ex...)
	default:
		if len(p.OwnerIDs) > 0 && scope == "all" {
			owners, err := s.accounts(ctx, pids(p.OwnerIDs))
			if err != nil {
				return nil, false, err
			}
			if len(owners) == 0 {
				return nil, true, nil
			}
			req.Owners, scope = owners, "owners"
		}
	}
	switch scope {
	case "friends":
		if viewer == "" {
			return nil, true, nil
		}
		req.FriendsOfViewer = true
	case "all":
		req.AllOwners = true
	}

	if len(p.DataTypes) > 0 {
		for _, dt := range p.DataTypes {
			req.Collections = append(req.Collections, collection(uint16(dt)))
		}
	} else if p.DataType != 0 {
		req.Collection = collection(uint16(p.DataType))
	}
	if p.ReferDataID != 0 {
		filter["refer_data_id"] = uint32(p.ReferDataID)
	}
	if len(p.Tags) > 0 {
		filter["tags"] = strs(p.Tags)
	}
	ms := func(d types.DateTime) int64 {
		if d == 0 {
			return 0
		}
		return d.Standard().UnixMilli()
	}
	req.CreatedAfterMs, req.CreatedBeforeMs = ms(p.CreatedAfter), ms(p.CreatedBefore)
	req.UpdatedAfterMs, req.UpdatedBeforeMs = ms(p.UpdatedAfter), ms(p.UpdatedBefore)

	col := p.ResultOrderColumn
	switch {
	case col == constants.SearchSortColumnSize:
		req.Sort = "size"
	case col == constants.SearchSortColumnCreatedTime:
		req.Sort = "created"
	case col == constants.SearchSortColumnUpdatedTime:
		req.Sort = "updated"
	case col >= constants.SearchSortColumnRating0 && col <= constants.SearchSortColumnRating15:
		req.Sort = fmt.Sprintf("rating:%d", col-constants.SearchSortColumnRating0)
	case col >= constants.SearchSortColumnRatingAverage0 && col <= constants.SearchSortColumnRatingAverage15:
		req.Sort = fmt.Sprintf("rating_avg:%d", col-constants.SearchSortColumnRatingAverage0)
	default:
		// data id; also name (the core name is the id), data type and referred count, which the
		// core cannot sort by (referred count is never incremented, so id order is its answer).
		req.Sort = "id"
	}
	if len(req.Sort) > 6 && req.Sort[:6] == "rating" {
		req.MinRatingCount = uint32(p.MinimalRatingFrequency)
	}
	req.Ascending = p.ResultOrder != constants.SearchSortOrderDesc
	limit := uint32(p.ResultRange.Length)
	if limit == 0 || limit > searchLimit {
		limit = searchLimit
	}
	req.Offset, req.Limit = uint32(p.ResultRange.Offset), limit

	if len(filter) > 0 {
		b, _ := json.Marshal(filter)
		req.MetaFilterJson = string(b)
	}
	b, _ := json.Marshal(ranges)
	req.MetaRangeJson = string(b)
	return req, false, nil
}

// Search answers SearchObject: the page of objects and the unpaged total.
func (s *Store) Search(caller uint64, p dstypes.DataStoreSearchParam) ([]dstypes.DataStoreMetaInfo, uint32, *nex.Error) {
	ctx, cancel := s.ctx()
	defer cancel()
	viewer, err := s.viewer(ctx, caller)
	if err != nil {
		return nil, 0, nexErr(err)
	}
	req, empty, err := s.searchRequest(ctx, caller, viewer, p)
	if err != nil {
		return nil, 0, nexErr(err)
	}
	if empty {
		return nil, 0, nil
	}
	r, err := s.TS.ListObjects(ctx, req)
	if err != nil {
		return nil, 0, nexErr(err)
	}
	out := make([]dstypes.DataStoreMetaInfo, 0, len(r.GetObjects()))
	for _, o := range r.GetObjects() {
		m, ok := decodeMeta(o)
		if !ok {
			continue
		}
		mi := metaInfo(o, m)
		mi.FilterPropertiesByResultOption(p.ResultOption)
		out = append(out, mi)
	}
	return out, r.GetHits(), nil
}
