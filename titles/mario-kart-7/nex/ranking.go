package nex

// Legacy Ranking (0x70) for Mario Kart 7: time-trial leaderboards.
//
// FACTS: the protocol number, the method numbers and the request layouts are the library's
// (nex-protocols-go ranking/legacy, from Pretendo); the server's NEX version is 2.4.3.
// INFERRED (no public document describes Mario Kart 7's use, and no console has talked to
// this server yet): the category is the course, the first score of the list is the time in
// milliseconds and lower is better, and the response layouts below. Every request's raw
// parameters are logged in hex so a capture settles the guesses; see docs/ranking.md.

import (
	"encoding/hex"
	"fmt"
	"log"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/legacy"
	"github.com/PretendoNetwork/nex-protocols-go/v2/ranking/legacy/constants"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/legacy/types"

	"github.com/openpak/nex/titles/mario-kart-7/database"
)

// rankingBackend is what the legacy Ranking handlers need from storage (database.Ranking).
type rankingBackend interface {
	UploadScore(pid uint64, uniqueID uint32, category uint32, scores []uint32, unknown1 uint8, unknown2 uint32) (bool, error)
	DeleteScore(pid uint64, category uint32) error
	DeleteAllScores(pid uint64) error
	UploadCommonData(pid uint64, uniqueID uint32, data []byte) error
	DeleteCommonData(pid uint64, uniqueID uint32) error
	CommonData(uniqueID uint32) ([]byte, error)
	Total(category uint32) (uint32, error)
	Page(category uint32, index int, desc bool, offset, length uint32) ([]database.RankEntry, error)
	Position(pid uint64, category uint32, index int, desc bool) (uint32, bool, error)
}

// maxWindow caps how many entries one GetScore may return (the library's MaxRangeRankingOrder).
const maxWindow = 255

type legacyRanking struct{ store rankingBackend }

func registerLegacyRanking(store rankingBackend) *ranking.Protocol {
	p := ranking.NewProtocol()
	l := &legacyRanking{store: store}
	p.SetHandlerUploadScore(l.uploadScore)
	p.SetHandlerUploadScores(l.uploadScores)
	p.SetHandlerUploadScoreWithLimit(l.uploadScoreWithLimit)
	p.SetHandlerUploadScoresWithLimit(l.uploadScoresWithLimit)
	p.SetHandlerDeleteScore(l.deleteScore)
	p.SetHandlerDeleteAllScore(l.deleteAllScore)
	p.SetHandlerUploadCommonData(l.uploadCommonData)
	p.SetHandlerDeleteCommonData(l.deleteCommonData)
	p.SetHandlerGetCommonData(l.getCommonData)
	p.SetHandlerGetTopScore(l.getTopScore)
	p.SetHandlerGetScore(l.getScore)
	p.SetHandlerGetSelfScore(l.getSelfScore)
	p.SetHandlerGetTotal(l.getTotal)
	return p
}

// logParams writes a request's raw parameters, verbatim, for a later capture comparison.
func logParams(packet nex.PacketInterface, name string) {
	m := packet.RMCMessage()
	log.Printf("[mario-kart-7] Ranking::%s method=%d params=%s", name, m.MethodID, hex.EncodeToString(m.Parameters))
}

func senderPID(packet nex.PacketInterface) uint64 { return uint64(packet.Sender().PID()) }

func storeFailure(err error) *nex.Error {
	return nex.NewError(nex.ResultCodes.Ranking.Unknown, err.Error())
}

func invalidRequest(err error) *nex.Error {
	return nex.NewError(nex.ResultCodes.Ranking.InvalidArgument, err.Error())
}

// reply builds the success response for the request in packet.
func reply(packet nex.PacketInterface, callID uint32, body func(out *nex.ByteStreamOut)) *nex.RMCMessage {
	endpoint := packet.Sender().Endpoint()
	out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	if body != nil {
		body(out)
	}
	msg := nex.NewRMCSuccess(endpoint, out.Bytes())
	msg.ProtocolID = ranking.ProtocolID
	msg.MethodID = packet.RMCMessage().MethodID
	msg.CallID = callID
	return msg
}

func toU32(l types.List[types.UInt32]) []uint32 {
	out := make([]uint32, len(l))
	for i, v := range l {
		out[i] = uint32(v)
	}
	return out
}

func (l *legacyRanking) store1(pid uint64, uniqueID, category uint32, scores []uint32, u1 uint8, u2 uint32) *nex.Error {
	if len(scores) == 0 {
		return invalidRequest(fmt.Errorf("empty score list"))
	}
	if _, err := l.store.UploadScore(pid, uniqueID, category, scores, u1, u2); err != nil {
		return storeFailure(err)
	}
	return nil
}

func (l *legacyRanking) uploadScore(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, category types.UInt32, scores types.List[types.UInt32], u1 types.UInt8, u2 types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "UploadScore")
	if err != nil {
		return nil, invalidRequest(err)
	}
	if e := l.store1(senderPID(packet), uint32(uniqueID), uint32(category), toU32(scores), uint8(u1), uint32(u2)); e != nil {
		return nil, e
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) uploadScoreWithLimit(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, category types.UInt32, scores types.List[types.UInt32], u1 types.UInt8, u2 types.UInt32, limit types.UInt16) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "UploadScoreWithLimit")
	if err != nil {
		return nil, invalidRequest(err)
	}
	if e := l.store1(senderPID(packet), uint32(uniqueID), uint32(category), toU32(scores), uint8(u1), uint32(u2)); e != nil {
		return nil, e
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) uploadScores(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, scores types.List[ranking_types.RankingScore]) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "UploadScores")
	if err != nil {
		return nil, invalidRequest(err)
	}
	for _, s := range scores {
		if e := l.store1(senderPID(packet), uint32(uniqueID), uint32(s.Category), toU32(s.Score), uint8(s.Unknown1), uint32(s.Unknown2)); e != nil {
			return nil, e
		}
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) uploadScoresWithLimit(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, scores types.List[ranking_types.RankingScoreWithLimit]) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "UploadScoresWithLimit")
	if err != nil {
		return nil, invalidRequest(err)
	}
	for _, s := range scores {
		if e := l.store1(senderPID(packet), uint32(uniqueID), uint32(s.Category), toU32(s.Score), uint8(s.Unknown1), uint32(s.Unknown2)); e != nil {
			return nil, e
		}
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) deleteScore(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, category types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "DeleteScore")
	if err != nil {
		return nil, invalidRequest(err)
	}
	if err := l.store.DeleteScore(senderPID(packet), uint32(category)); err != nil {
		return nil, storeFailure(err)
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) deleteAllScore(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "DeleteAllScore")
	if err != nil {
		return nil, invalidRequest(err)
	}
	if err := l.store.DeleteAllScores(senderPID(packet)); err != nil {
		return nil, storeFailure(err)
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) uploadCommonData(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, data types.Buffer) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "UploadCommonData")
	if err != nil {
		return nil, invalidRequest(err)
	}
	if len(data) > 1024 {
		return nil, invalidRequest(fmt.Errorf("common data of %d bytes", len(data)))
	}
	if err := l.store.UploadCommonData(senderPID(packet), uint32(uniqueID), []byte(data)); err != nil {
		return nil, storeFailure(err)
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) deleteCommonData(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "DeleteCommonData")
	if err != nil {
		return nil, invalidRequest(err)
	}
	if err := l.store.DeleteCommonData(senderPID(packet), uint32(uniqueID)); err != nil {
		return nil, storeFailure(err)
	}
	return reply(packet, callID, nil), nil
}

func (l *legacyRanking) getCommonData(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "GetCommonData")
	if err != nil {
		return nil, invalidRequest(err)
	}
	data, e := l.store.CommonData(uint32(uniqueID))
	if e != nil {
		return nil, storeFailure(e)
	}
	return reply(packet, callID, func(out *nex.ByteStreamOut) { types.NewBuffer(data).WriteTo(out) }), nil
}

func (l *legacyRanking) getTotal(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, u1, u2, u3 types.UInt8, u4 types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "GetTotal")
	if err != nil {
		return nil, invalidRequest(err)
	}
	// INFERRED: the only 32-bit field after the three bytes is the category.
	total, e := l.store.Total(uint32(u4))
	if e != nil {
		return nil, storeFailure(e)
	}
	return reply(packet, callID, func(out *nex.ByteStreamOut) { out.WriteUInt32LE(total) }), nil
}

func (l *legacyRanking) getTopScore(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, category types.UInt32) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "GetTopScore")
	if err != nil {
		return nil, invalidRequest(err)
	}
	entries, e := l.store.Page(uint32(category), 0, false, 0, 1)
	if e != nil {
		return nil, storeFailure(e)
	}
	return reply(packet, callID, func(out *nex.ByteStreamOut) { writeRankData(out, uint32(category), entries) }), nil
}

func (l *legacyRanking) getSelfScore(err error, packet nex.PacketInterface, callID uint32, uniqueID types.UInt32, category types.UInt32, order ranking_types.RankingOrderParam, length types.UInt8) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "GetSelfScore")
	if err != nil {
		return nil, invalidRequest(err)
	}
	entries, e := l.window(senderPID(packet), uint32(category), order, uint32(length), constants.RankingModeNear, 0)
	if e != nil {
		return nil, storeFailure(e)
	}
	return reply(packet, callID, func(out *nex.ByteStreamOut) { writeRankData(out, uint32(category), entries) }), nil
}

func (l *legacyRanking) getScore(err error, packet nex.PacketInterface, callID uint32, mode constants.RankingMode, category types.UInt32, order ranking_types.RankingOrderParam, offset types.UInt32, length types.UInt8) (*nex.RMCMessage, *nex.Error) {
	logParams(packet, "GetScore")
	if err != nil {
		return nil, invalidRequest(err)
	}
	entries, e := l.window(senderPID(packet), uint32(category), order, uint32(length), mode, uint32(offset))
	if e != nil {
		return nil, storeFailure(e)
	}
	return reply(packet, callID, func(out *nex.ByteStreamOut) { writeRankData(out, uint32(category), entries) }), nil
}

// window picks the entries a mode asks for. Range lists from offset; Near centres on the
// caller; User (and the friend modes, there being no friend list here) is the caller alone.
func (l *legacyRanking) window(pid uint64, category uint32, order ranking_types.RankingOrderParam, length uint32, mode constants.RankingMode, offset uint32) ([]database.RankEntry, error) {
	index := int(order.ScoreIndex)
	desc := order.ScoreOrder == constants.OrderByDescending
	if length == 0 || length > maxWindow {
		length = 20
	}
	switch mode {
	case constants.RankingModeRange:
		if offset > constants.MaxRangeRankingOrder {
			return nil, nil
		}
		return l.store.Page(category, index, desc, offset, length)
	case constants.RankingModeNear:
		pos, ok, err := l.store.Position(pid, category, index, desc)
		if err != nil || !ok {
			return nil, err
		}
		start := uint32(0)
		if pos > length/2 {
			start = pos - length/2
		}
		return l.store.Page(category, index, desc, start, length)
	default:
		pos, ok, err := l.store.Position(pid, category, index, desc)
		if err != nil || !ok {
			return nil, err
		}
		entries, err := l.store.Page(category, index, desc, pos, 1)
		return entries, err
	}
}

// writeRankData writes a List<RankingData>.
func writeRankData(out *nex.ByteStreamOut, category uint32, entries []database.RankEntry) {
	list := types.NewList[ranking_types.RankingData]()
	for _, e := range entries {
		d := ranking_types.NewRankingData()
		d.UniqueID = types.NewUInt32(e.UniqueID)
		d.PrincipalID = types.NewPID(e.PID)
		d.Order = types.NewUInt32(e.Rank)
		d.Category = types.NewUInt32(category)
		for _, s := range e.Scores {
			d.Scores = append(d.Scores, types.NewUInt32(s))
		}
		d.Unknown1 = types.NewUInt8(e.Unknown1)
		d.Unknown2 = types.NewUInt32(e.Unknown2)
		d.CommonData = types.NewBuffer(e.CommonData)
		list = append(list, d)
	}
	list.WriteTo(out)
}
