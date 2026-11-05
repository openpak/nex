package coredatastore

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	dstypes "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
)

func postParam(size int, dataType uint16, perm constants.Permission, slot uint16, tags ...string) dstypes.DataStorePreparePostParam {
	p := dstypes.NewDataStorePreparePostParam()
	p.Size = types.NewUInt32(uint32(size))
	p.Name = types.NewString("ghost")
	p.DataType = types.NewUInt16(dataType)
	p.MetaBinary = types.NewQBuffer([]byte{1, 2, 3})
	p.Permission = permission(uint8(perm), nil)
	p.DelPermission = permission(uint8(constants.PermissionPrivate), nil)
	p.Flag = constants.DataFlagNeedCompletion
	p.Period = types.NewUInt16(90)
	p.Tags = types.NewList[types.String]()
	for _, t := range tags {
		p.Tags = append(p.Tags, types.NewString(t))
	}
	r := dstypes.NewDataStoreRatingInitParamWithSlot()
	r.Slot = types.NewInt8(0)
	r.Param.LockType = constants.RatingLockPermanent
	p.RatingInitParams = types.NewList[dstypes.DataStoreRatingInitParamWithSlot]()
	p.RatingInitParams = append(p.RatingInitParams, r)
	p.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(slot)
	return p
}

func complete(id uint64) dstypes.DataStoreCompletePostParam {
	p := dstypes.NewDataStoreCompletePostParam()
	p.DataID, p.IsSuccess = types.NewUInt64(id), true
	return p
}

func byID(id uint64) dstypes.DataStoreGetMetaParam {
	p := dstypes.NewDataStoreGetMetaParam()
	p.DataID = types.NewUInt64(id)
	p.ResultOption = constants.ResultFlag(0xFF)
	return p
}

func getParam(id uint64) dstypes.DataStorePrepareGetParam {
	p := dstypes.NewDataStorePrepareGetParam()
	p.DataID = types.NewUInt64(id)
	return p
}

func target(id uint64, slot uint8) dstypes.DataStoreRatingTarget {
	t := dstypes.NewDataStoreRatingTarget()
	t.DataID, t.Slot = types.NewUInt64(id), types.NewUInt8(slot)
	return t
}

func rateParam(v int32) dstypes.DataStoreRateObjectParam {
	p := dstypes.NewDataStoreRateObjectParam()
	p.RatingValue = types.NewInt32(v)
	return p
}

func code(ne *nex.Error) uint32 {
	if ne == nil {
		return 0
	}
	return ne.ResultCode &^ 0x80000000 // the error bit NewError sets
}

func fields(info dstypes.DataStoreReqPostInfo) map[string]string {
	m := map[string]string{}
	for _, kv := range info.FormFields {
		m[string(kv.Key)] = string(kv.Value)
	}
	return m
}

// post is the console's whole post: prepare, send the body to the URL it got, complete.
func post(t *testing.T, s *Store, owner uint64, p dstypes.DataStorePreparePostParam, body []byte) uint64 {
	t.Helper()
	info, ne := s.PreparePost(owner, p)
	if ne != nil {
		t.Fatalf("PreparePost: %v", ne)
	}
	if len(info.RequestHeaders) != 0 || string(info.URL) == "" {
		t.Fatalf("ReqPostInfo: url %q headers %d", info.URL, len(info.RequestHeaders))
	}
	if err := upload(string(info.URL), fields(info), body); err != nil {
		t.Fatal(err)
	}
	if ne := s.CompletePost(owner, complete(uint64(info.DataID))); ne != nil {
		t.Fatalf("CompletePost: %v", ne)
	}
	return uint64(info.DataID)
}

// roundTrip is upload, download, rate, change meta, search and delete through the adapter,
// with the body going over HTTP to the URLs the core hands out, exactly as a console sends it.
// It runs against the fake core and, with NEX_LIVE_* set, against a real title-storage.
func roundTrip(t *testing.T, s *Store, owner, other uint64, dataType uint16) {
	body := make([]byte, 3000)
	_, _ = rand.Read(body)
	tagb := make([]byte, 6)
	_, _ = rand.Read(tagb)
	tag := "rt-" + hex.EncodeToString(tagb)

	id := post(t, s, owner, postParam(len(body), dataType, constants.PermissionPublic, noPersistenceSlot, tag), body)
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			p := dstypes.NewDataStoreDeleteParam()
			p.DataID = types.NewUInt64(id)
			_ = s.Delete(owner, p)
		}
	})

	mi, ne := s.GetMeta(other, byID(id))
	if ne != nil {
		t.Fatalf("GetMeta: %v", ne)
	}
	if uint64(mi.OwnerID) != owner || uint32(mi.Size) != uint32(len(body)) || string(mi.Name) != "ghost" ||
		uint16(mi.DataType) != dataType || len(mi.Tags) != 1 || string(mi.Tags[0]) != tag ||
		!bytes.Equal(mi.MetaBinary, []byte{1, 2, 3}) || uint16(mi.Period) != 90 {
		t.Fatalf("meta = owner %d size %d name %q type %d tags %v meta %v period %d", mi.OwnerID, mi.Size, mi.Name, mi.DataType, mi.Tags, mi.MetaBinary, mi.Period)
	}
	if len(mi.Ratings) != 1 || int8(mi.Ratings[0].Slot) != 0 || uint32(mi.Ratings[0].Rating.Count) != 0 {
		t.Fatalf("ratings after post = %+v", mi.Ratings)
	}

	gi, ne := s.PrepareGet(other, getParam(id))
	if ne != nil {
		t.Fatalf("PrepareGet: %v", ne)
	}
	got, err := download(string(gi.URL))
	if err != nil || !bytes.Equal(got, body) || uint32(gi.Size) != uint32(len(body)) {
		t.Fatalf("download: %v, %d bytes, size %d", err, len(got), gi.Size)
	}

	r, ne := s.Rate(other, target(id, 0), rateParam(4))
	if ne != nil || int64(r.TotalValue) != 4 || uint32(r.Count) != 1 {
		t.Fatalf("Rate = %+v %v", r, ne)
	}
	if _, ne := s.Rate(other, target(id, 0), rateParam(4)); code(ne) != nex.ResultCodes.DataStore.OperationNotAllowed {
		t.Fatalf("second rate on a permanent lock = %v, want OperationNotAllowed", ne)
	}

	cm := dstypes.NewDataStoreChangeMetaParam()
	cm.DataID = types.NewUInt64(id)
	cm.ModifiesFlag = constants.ModificationFlagMetaBinary | constants.ModificationFlagTags | constants.ModificationFlagDataType
	cm.MetaBinary = types.NewQBuffer([]byte{9, 9})
	cm.Tags = types.NewList[types.String]()
	cm.Tags = append(cm.Tags, types.NewString(tag), types.NewString("changed"))
	cm.DataType = types.NewUInt16(dataType + 1)
	if ne := s.ChangeMeta(owner, cm); ne != nil {
		t.Fatalf("ChangeMeta: %v", ne)
	}
	mi, ne = s.GetMeta(other, byID(id))
	if ne != nil || !bytes.Equal(mi.MetaBinary, []byte{9, 9}) || uint16(mi.DataType) != dataType+1 || len(mi.Tags) != 2 {
		t.Fatalf("after ChangeMeta: %v meta %v type %d tags %v", ne, mi.MetaBinary, mi.DataType, mi.Tags)
	}
	if len(mi.Ratings) != 1 || int64(mi.Ratings[0].Rating.TotalValue) != 4 {
		t.Fatalf("ChangeMeta lost the ratings: %+v", mi.Ratings)
	}
	gi, ne = s.PrepareGet(other, getParam(id))
	if ne != nil {
		t.Fatalf("PrepareGet after ChangeMeta: %v", ne)
	}
	if got, err := download(string(gi.URL)); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("ChangeMeta lost the body: %v", err)
	}

	sp := dstypes.NewDataStoreSearchParam()
	sp.SearchTarget = constants.SearchTypePublic
	sp.DataType = types.NewUInt16(dataType + 1)
	sp.Tags = types.NewList[types.String]()
	sp.Tags = append(sp.Tags, types.NewString(tag))
	sp.ResultOrderColumn = constants.SearchSortColumnRating0
	sp.ResultOrder = constants.SearchSortOrderDesc
	sp.ResultRange.Length = types.NewUInt32(10)
	sp.ResultOption = constants.ResultFlag(0xFF)
	found, total, ne := s.Search(other, sp)
	if ne != nil || total != 1 || len(found) != 1 || uint64(found[0].DataID) != id {
		t.Fatalf("Search = %d of %d, %v", len(found), total, ne)
	}

	dp := dstypes.NewDataStoreDeleteParam()
	dp.DataID = types.NewUInt64(id)
	if ne := s.Delete(owner, dp); ne != nil {
		t.Fatalf("Delete: %v", ne)
	}
	deleted = true
	if _, ne := s.GetMeta(other, byID(id)); code(ne) != nex.ResultCodes.DataStore.NotFound {
		t.Fatalf("GetMeta after delete = %v, want NotFound", ne)
	}
}
