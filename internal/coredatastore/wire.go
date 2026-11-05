package coredatastore

// The DataStore method handlers: decode is the protocol library's, the answer is the Store's,
// and this file only writes the reply. The response layouts are those of
// nex-protocols-common-go's common DataStore handlers, which this replaces for a flipped title.

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore "github.com/PretendoNetwork/nex-protocols-go/v2/datastore"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	dstypes "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
)

// Install sets every DataStore handler the title serves on its protocol.
func (s *Store) Install(p datastore.Interface) {
	p.SetHandlerGetMeta(s.getMeta)
	p.SetHandlerGetMetas(s.getMetas)
	p.SetHandlerGetMetasMultipleParam(s.getMetasMultipleParam)
	p.SetHandlerSearchObject(s.searchObject)
	p.SetHandlerPrepareGetObject(s.prepareGetObject)
	p.SetHandlerPreparePostObject(s.preparePostObject)
	p.SetHandlerCompletePostObject(s.completePostObject)
	p.SetHandlerCompletePostObjects(s.completePostObjects)
	p.SetHandlerPostMetaBinary(s.postMetaBinary)
	p.SetHandlerChangeMeta(s.changeMeta)
	p.SetHandlerDeleteObject(s.deleteObject)
	p.SetHandlerRateObject(s.rateObject)
	p.SetHandlerRateObjects(s.rateObjects)
}

func decodeErr(err error) *nex.Error {
	return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
}

func reply(packet nex.PacketInterface, callID, method uint32, write func(*nex.ByteStreamOut)) *nex.RMCMessage {
	ep := packet.Sender().Endpoint()
	out := nex.NewByteStreamOut(ep.LibraryVersions(), ep.ByteStreamSettings())
	if write != nil {
		write(out)
	}
	var body []byte
	if write != nil {
		body = out.Bytes()
	}
	m := nex.NewRMCSuccess(ep, body)
	m.ProtocolID, m.MethodID, m.CallID = datastore.ProtocolID, method, callID
	return m
}

func caller(packet nex.PacketInterface) uint64 { return uint64(packet.Sender().PID()) }

func (s *Store) getMeta(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStoreGetMetaParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	mi, ne := s.GetMeta(caller(packet), p)
	if ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodGetMeta, func(o *nex.ByteStreamOut) { mi.WriteTo(o) }), nil
}

func writeMetas(o *nex.ByteStreamOut, metas []dstypes.DataStoreMetaInfo, errs []*nex.Error) {
	list := types.NewList[dstypes.DataStoreMetaInfo]()
	results := types.NewList[types.QResult]()
	for i := range metas {
		list = append(list, metas[i])
		if errs[i] != nil {
			results = append(results, types.NewQResultError(errs[i].ResultCode))
		} else {
			results = append(results, types.NewQResultSuccess(nex.ResultCodes.DataStore.Unknown))
		}
	}
	list.WriteTo(o)
	results.WriteTo(o)
}

func (s *Store) getMetas(err error, packet nex.PacketInterface, callID uint32, ids types.List[types.UInt64], p dstypes.DataStoreGetMetaParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, uint64(id))
	}
	metas, errs := s.GetMetas(caller(packet), raw, p)
	return reply(packet, callID, datastore.MethodGetMetas, func(o *nex.ByteStreamOut) { writeMetas(o, metas, errs) }), nil
}

func (s *Store) getMetasMultipleParam(err error, packet nex.PacketInterface, callID uint32, params types.List[dstypes.DataStoreGetMetaParam]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	metas := make([]dstypes.DataStoreMetaInfo, len(params))
	errs := make([]*nex.Error, len(params))
	for i, p := range params {
		metas[i], errs[i] = s.GetMeta(caller(packet), p)
	}
	return reply(packet, callID, datastore.MethodGetMetasMultipleParam, func(o *nex.ByteStreamOut) { writeMetas(o, metas, errs) }), nil
}

func (s *Store) searchObject(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStoreSearchParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	objects, total, ne := s.Search(caller(packet), p)
	if ne != nil {
		return nil, ne
	}
	res := dstypes.NewDataStoreSearchResult()
	res.Result = types.NewList[dstypes.DataStoreMetaInfo]()
	res.Result = append(res.Result, objects...)
	res.TotalCountType = constants.SearchResultTotalExact
	if total != uint32(len(objects)) {
		res.TotalCountType = constants.SearchResultTotalMinimum
	}
	ep := packet.Sender().Endpoint()
	if (p.StructureVersion >= 3 || ep.LibraryVersions().DataStore.GreaterOrEqual("4.0.0")) && !bool(p.TotalCountEnabled) {
		total, res.TotalCountType = 0, constants.SearchResultTotalDisabled
	}
	res.TotalCount = types.NewUInt32(total)
	return reply(packet, callID, datastore.MethodSearchObject, func(o *nex.ByteStreamOut) { res.WriteTo(o) }), nil
}

func (s *Store) prepareGetObject(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStorePrepareGetParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	info, ne := s.PrepareGet(caller(packet), p)
	if ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodPrepareGetObject, func(o *nex.ByteStreamOut) { info.WriteTo(o) }), nil
}

func (s *Store) preparePostObject(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStorePreparePostParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	info, ne := s.PreparePost(caller(packet), p)
	if ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodPreparePostObject, func(o *nex.ByteStreamOut) { info.WriteTo(o) }), nil
}

func (s *Store) completePostObject(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStoreCompletePostParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	if ne := s.CompletePost(caller(packet), p); ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodCompletePostObject, nil), nil
}

func (s *Store) completePostObjects(err error, packet nex.PacketInterface, callID uint32, ids types.List[types.UInt64]) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	for _, id := range ids {
		p := dstypes.NewDataStoreCompletePostParam()
		p.DataID, p.IsSuccess = id, true
		if ne := s.CompletePost(caller(packet), p); ne != nil {
			return nil, ne
		}
	}
	return reply(packet, callID, datastore.MethodCompletePostObjects, nil), nil
}

func (s *Store) postMetaBinary(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStorePreparePostParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	id, ne := s.PostMetaBinary(caller(packet), p)
	if ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodPostMetaBinary, func(o *nex.ByteStreamOut) { o.WriteUInt64LE(id) }), nil
}

func (s *Store) changeMeta(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStoreChangeMetaParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	if ne := s.ChangeMeta(caller(packet), p); ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodChangeMeta, nil), nil
}

func (s *Store) deleteObject(err error, packet nex.PacketInterface, callID uint32, p dstypes.DataStoreDeleteParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	if ne := s.Delete(caller(packet), p); ne != nil {
		return nil, ne
	}
	return reply(packet, callID, datastore.MethodDeleteObject, nil), nil
}

func (s *Store) rateObject(err error, packet nex.PacketInterface, callID uint32, t dstypes.DataStoreRatingTarget, p dstypes.DataStoreRateObjectParam, fetch types.Bool) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	r, ne := s.Rate(caller(packet), t, p)
	if ne != nil {
		return nil, ne
	}
	if !fetch {
		r = dstypes.NewDataStoreRatingInfo()
	}
	return reply(packet, callID, datastore.MethodRateObject, func(o *nex.ByteStreamOut) { r.WriteTo(o) }), nil
}

func (s *Store) rateObjects(err error, packet nex.PacketInterface, callID uint32, targets types.List[dstypes.DataStoreRatingTarget], params types.List[dstypes.DataStoreRateObjectParam], _ types.Bool, fetch types.Bool) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, decodeErr(err)
	}
	if len(targets) != len(params) {
		return nil, nex.NewError(nex.ResultCodes.DataStore.InvalidArgument, "targets and params differ in length")
	}
	ratings := types.NewList[dstypes.DataStoreRatingInfo]()
	for i, t := range targets {
		r, ne := s.Rate(caller(packet), t, params[i])
		if ne != nil {
			return nil, ne
		}
		if fetch {
			ratings = append(ratings, r)
		}
	}
	return reply(packet, callID, datastore.MethodRateObjects, func(o *nex.ByteStreamOut) {
		ratings.WriteTo(o)
		types.NewList[types.QResult]().WriteTo(o)
	}), nil
}
