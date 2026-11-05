// Package coreranking is the NEX Ranking (0x70) store over the OpenPak core: scores live in the
// leaderboards service, the per-score common data in title-storage. A title flipped to it
// (NEX_CORE_STORAGE contains "rankings", docs/core-backed-storage.md) keeps no score table of
// its own; it sets the same eight hooks of the common ranking protocol a local store sets.
//
// Mapping (one leaderboards title per NEX title, cross_network false):
//
//   - category    -> board id (decimal); board order from Config.Order or the first OrderBy
//   - PID         -> account (nn-account Resolution, cached); entry sub 0: one score per player
//     per category, which is what UpdateModeNormal means
//   - score       -> entry score; UpdateModeNormal keeps the better one (aggregation "best"),
//     UpdateModeDeleteOld deletes the entry first, so the new one always lands
//   - pid, unique id, param, groups -> the entry's info blob (score-bound: it is replaced
//     only when the score is, exactly like a local row)
//   - common data -> title-storage object (collection "ranking_common", name = unique id,
//     owner = account, public)
//
// A score or common data the core cannot take (no account link, or the core is down) goes to
// the title database's corestore.pending_* tables and Drain moves it later. Nothing is dropped.
package coreranking

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	"github.com/PretendoNetwork/nex-protocols-go/v2/ranking/constants"
	rankingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openpak/nex/internal/coreid"
	lbpb "github.com/openpak/nex/internal/leaderboardspb"
	"github.com/openpak/nex/internal/rankingsapi"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// CommonCollection is the title-storage collection of ranking common data.
const CommonCollection = "ranking_common"

// scanLimit bounds a board read that has to filter adapter-side (group filter).
const scanLimit = 10000

// Config is a title's place in the core.
type Config struct {
	Title   string // neutral title key and native id on Network, e.g. "mario-kart-8"
	Name    string // display name of the title
	Network string // "wiiu" or "3ds"
	Kind    string // rankingsapi.KindTime or KindScore (display only)
	// Order forces every board's order ("asc" or "desc"); empty takes it from the OrderBy of
	// the first score uploaded to the board.
	Order string
	// BoardName names a category; nil gives rankingsapi.NeutralName.
	BoardName func(category uint32) string
}

// Store is the core-backed ranking store of one title.
type Store struct {
	Cfg Config
	LB  lbpb.LeaderboardsClient
	TS  tspb.TitleStorageClient
	IDs coreid.Resolver
	// DB is the title database the pending tables live in; nil means a score the core cannot
	// take is an error to the console instead of being kept.
	DB *sql.DB
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

func (s *Store) ref(category uint32) *lbpb.BoardRef {
	return &lbpb.BoardRef{Network: s.Cfg.Network, Title: s.Cfg.Title, Board: strconv.FormatUint(uint64(category), 10)}
}

func (s *Store) tref() *tspb.TitleRef {
	return &tspb.TitleRef{Network: s.Cfg.Network, Title: s.Cfg.Title}
}

func (s *Store) boardName(category uint32) string {
	if s.Cfg.BoardName != nil {
		if n := s.Cfg.BoardName(category); n != "" {
			return n
		}
	}
	return rankingsapi.NeutralName(uint64(category))
}

func (s *Store) policy(category uint32, orderBy constants.OrderBy) *lbpb.Policy {
	order := s.Cfg.Order
	if order == "" {
		order = "asc"
		if orderBy == constants.OrderByDescending {
			order = "desc"
		}
	}
	kind := s.Cfg.Kind
	if kind == "" {
		kind = rankingsapi.KindScore
	}
	return &lbpb.Policy{Name: s.boardName(category), Kind: kind, Order: order, Aggregation: "best",
		CrossNetwork: lbpb.Tristate_TRISTATE_FALSE}
}

// Register maps the title in both core services (neutral key = native id = Cfg.Title) and
// makes it per-network. Safe to call on every start.
func (s *Store) Register(ctx context.Context) error {
	if _, err := s.LB.MapTitle(ctx, &lbpb.MapTitleRequest{Title: s.Cfg.Title, Name: s.Cfg.Name, Network: s.Cfg.Network, NativeId: s.Cfg.Title}); err != nil {
		return fmt.Errorf("leaderboards MapTitle: %w", err)
	}
	if _, err := s.TS.MapTitle(ctx, &tspb.MapTitleRequest{Title: s.Cfg.Title, Name: s.Cfg.Name, Network: s.Cfg.Network, NativeId: s.Cfg.Title}); err != nil {
		return fmt.Errorf("title-storage MapTitle: %w", err)
	}
	if _, err := s.TS.UpsertTitle(ctx, &tspb.UpsertTitleRequest{Title: s.Cfg.Title, Name: s.Cfg.Name, CrossNetwork: tspb.Tristate_TRISTATE_FALSE}); err != nil {
		return fmt.Errorf("title-storage UpsertTitle: %w", err)
	}
	return s.InitTables()
}

// Install sets every storage hook of the common ranking protocol.
func (s *Store) Install(p *commonranking.CommonProtocol) {
	p.InsertRankingByPIDAndRankingScoreData = s.Insert
	p.UploadCommonData = s.UploadCommonData
	p.GetCommonData = s.GetCommonData
	p.GetRankingsAndCountByCategoryAndRankingOrderParam = s.Global
	p.GetNearbyRankingsAndCountByCategoryAndRankingOrderParam = s.Nearby
	p.GetFriendsRankingsAndCountByCategoryAndRankingOrderParam = s.Friends
	p.GetNearbyFriendsRankingsAndCountByCategoryAndRankingOrderParam = s.NearbyFriends
	p.GetOwnRankingByCategoryAndRankingOrderParam = s.Own
}

func sysErr(err error) error { return nex.NewError(nex.ResultCodes.Core.SystemError, err.Error()) }

// ---- the info blob ----

// info is what a NEX rank row carries beside the score.
type info struct {
	PID, UID, Param uint64
	OrderBy         uint8
	Groups          []byte
}

const infoVersion = 1

func (i info) encode() []byte {
	b := make([]byte, 0, 26+len(i.Groups))
	b = append(b, infoVersion)
	b = binary.LittleEndian.AppendUint64(b, i.PID)
	b = binary.LittleEndian.AppendUint64(b, i.UID)
	b = binary.LittleEndian.AppendUint64(b, i.Param)
	b = append(b, i.OrderBy)
	return append(b, i.Groups...)
}

func decodeInfo(b []byte) (info, bool) {
	if len(b) < 26 || b[0] != infoVersion {
		return info{}, false
	}
	return info{
		PID:     binary.LittleEndian.Uint64(b[1:]),
		UID:     binary.LittleEndian.Uint64(b[9:]),
		Param:   binary.LittleEndian.Uint64(b[17:]),
		OrderBy: b[25],
		Groups:  append([]byte(nil), b[26:]...),
	}, true
}

// ---- writes ----

// Insert stores one uploaded score.
func (s *Store) Insert(pid types.PID, d rankingtypes.RankingScoreData, uniqueID types.UInt64) error {
	ctx, cancel := s.ctx()
	defer cancel()
	err := s.submit(ctx, uint64(pid), d, uint64(uniqueID))
	if err == nil {
		return nil
	}
	return s.spillScore(ctx, uint64(pid), d, uint64(uniqueID), err)
}

func (s *Store) submit(ctx context.Context, pid uint64, d rankingtypes.RankingScoreData, uid uint64) error {
	acct, err := s.IDs.Account(ctx, pid)
	if err != nil {
		return err
	}
	cat := uint32(d.Category)
	ref := s.ref(cat)
	if d.UpdateMode == constants.UpdateModeDeleteOld {
		if _, err := s.LB.Delete(ctx, &lbpb.DeleteRequest{Ref: ref, Account: acct}); err != nil && status.Code(err) != codes.NotFound {
			return err
		}
	}
	in := info{PID: pid, UID: uid, Param: uint64(d.Param), OrderBy: uint8(d.OrderBy), Groups: []byte(d.Groups)}
	_, err = s.LB.Submit(ctx, &lbpb.SubmitRequest{Ref: ref, Account: acct, Score: int64(uint32(d.Score)),
		Info: in.encode(), CreatePolicy: s.policy(cat, d.OrderBy)})
	return err
}

// UploadCommonData stores the blob the game attaches to its scores.
func (s *Store) UploadCommonData(pid types.PID, uniqueID types.UInt64, data types.Buffer) error {
	ctx, cancel := s.ctx()
	defer cancel()
	err := s.putCommon(ctx, uint64(pid), uint64(uniqueID), []byte(data))
	if err == nil {
		return nil
	}
	return s.spillCommon(ctx, uint64(pid), uint64(uniqueID), []byte(data), err)
}

func (s *Store) putCommon(ctx context.Context, pid, uid uint64, data []byte) error {
	acct, err := s.IDs.Account(ctx, pid)
	if err != nil {
		return err
	}
	if data == nil {
		data = []byte{}
	}
	_, err = s.TS.PutObject(ctx, &tspb.PutObjectRequest{Ref: s.tref(), Owner: acct, Writer: acct,
		Collection: CommonCollection, Name: strconv.FormatUint(uid, 10), Body: data, Visibility: tspb.Visibility_VISIBILITY_PUBLIC})
	return err
}

// ErrNoCommonData is GetCommonData's answer for a unique id with nothing stored; the common
// protocol turns it into Ranking::NotFound.
var ErrNoCommonData = errors.New("coreranking: no common data")

// GetCommonData returns the blob stored for a unique id.
func (s *Store) GetCommonData(uniqueID types.UInt64) (types.Buffer, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	r, err := s.TS.GetObject(ctx, &tspb.GetObjectRequest{Ref: s.tref(), Collection: CommonCollection,
		Name: strconv.FormatUint(uint64(uniqueID), 10), IncludeBody: true})
	if err != nil {
		return nil, sysErr(err)
	}
	if !r.GetFound() {
		return nil, ErrNoCommonData
	}
	return types.NewBuffer(r.GetObject().GetBody()), nil
}

// ---- reads ----

// row is one entry as NEX needs it.
type row struct {
	account string
	score   int64
	rank    int64 // competition rank ("1224")
	pos     int64 // unique position ("1234")
	updated time.Time
	in      info
}

func (s *Store) rows(ctx context.Context, es []*lbpb.Entry) []row {
	out := make([]row, 0, len(es))
	for _, e := range es {
		r := row{account: e.GetAccount(), score: e.GetScore(), rank: e.GetRank(), pos: e.GetPosition(),
			updated: time.UnixMilli(e.GetUpdatedUnixMs()).UTC()}
		in, ok := decodeInfo(e.GetInfo())
		if !ok { // written by something other than this adapter: ask who the player is
			p, err := s.IDs.PID(ctx, e.GetAccount())
			if err != nil {
				continue // no Wii U/3DS identity: a console cannot show the row
			}
			in = info{PID: p}
		}
		r.in = in
		out = append(out, r)
	}
	return out
}

func notFound(err error) bool { return status.Code(err) == codes.NotFound }

func groupFilter(o rankingtypes.RankingOrderParam) (idx int, num byte, on bool) {
	if o.GroupIndex == constants.FilterGroupIndexNone {
		return 0, 0, false
	}
	return int(o.GroupIndex), byte(o.GroupNum), true
}

func keep(rs []row, o rankingtypes.RankingOrderParam) []row {
	idx, num, on := groupFilter(o)
	if !on {
		return rs
	}
	out := rs[:0:0]
	for _, r := range rs {
		if len(r.in.Groups) > idx && r.in.Groups[idx] == num {
			out = append(out, r)
		}
	}
	return out
}

// renumber gives a filtered list its own ranks, ties sharing one in competition order.
func renumber(rs []row) {
	for i := range rs {
		rs[i].pos = int64(i + 1)
		if i > 0 && rs[i].score == rs[i-1].score {
			rs[i].rank = rs[i-1].rank
		} else {
			rs[i].rank = int64(i + 1)
		}
	}
}

func length(o rankingtypes.RankingOrderParam) int {
	if n := int(o.Length); n > 0 {
		return n
	}
	return 10
}

func window(rs []row, offset, n int) []row {
	lo := min(max(offset, 0), len(rs))
	return rs[lo:min(lo+n, len(rs))]
}

// all reads a whole board best first (capped), for reads that filter adapter-side.
func (s *Store) all(ctx context.Context, ref *lbpb.BoardRef) ([]*lbpb.Entry, error) {
	var out []*lbpb.Entry
	for len(out) < scanLimit {
		r, err := s.LB.Range(ctx, &lbpb.RangeRequest{Ref: ref, Start: int64(len(out) + 1), Limit: 100})
		if err != nil {
			return nil, err
		}
		out = append(out, r.GetEntries()...)
		if len(r.GetEntries()) < 100 {
			break
		}
	}
	return out, nil
}

type mode int

const (
	modeGlobal mode = iota
	modeNearby
	modeFriends
	modeNearbyFriends
	modeOwn
)

type list = types.List[rankingtypes.RankingRankData]

func (s *Store) read(m mode, pid types.PID, category types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	rs, total, err := s.page(ctx, m, uint64(pid), uint32(category), o)
	if notFound(err) || errors.Is(err, coreid.ErrNoLink) {
		return list{}, 0, nil // no board yet, or a caller the core does not know: nothing ranked
	}
	if err != nil {
		return nil, 0, sysErr(err)
	}
	return s.toNEX(ctx, uint32(category), rs, o), uint32(total), nil
}

func (s *Store) page(ctx context.Context, m mode, pid uint64, category uint32, o rankingtypes.RankingOrderParam) ([]row, int, error) {
	ref := s.ref(category)
	n := length(o)
	_, _, filtered := groupFilter(o)
	self := ""
	if m != modeGlobal {
		a, err := s.IDs.Account(ctx, pid)
		if err != nil {
			return nil, 0, err
		}
		self = a
	}
	switch m {
	case modeFriends, modeNearbyFriends:
		r, err := s.LB.FriendRanks(ctx, &lbpb.FriendRanksRequest{Ref: ref, Account: self, IncludeSelf: true, Limit: 100})
		if err != nil {
			return nil, 0, err
		}
		rs := keep(s.rows(ctx, r.GetEntries()), o)
		renumber(rs)
		if m == modeFriends {
			return window(rs, int(o.Offset), n), len(rs), nil
		}
		return around(rs, self, n), len(rs), nil
	case modeOwn:
		if filtered {
			rs, err := s.filteredBoard(ctx, ref, o)
			if err != nil {
				return nil, 0, err
			}
			return around(rs, self, 1), len(rs), nil
		}
		r, err := s.LB.ByAccounts(ctx, &lbpb.ByAccountsRequest{Ref: ref, Accounts: []string{self}})
		if err != nil {
			return nil, 0, err
		}
		return s.rows(ctx, r.GetEntries()), int(r.GetTotal()), nil
	}
	if filtered {
		rs, err := s.filteredBoard(ctx, ref, o)
		if err != nil {
			return nil, 0, err
		}
		if m == modeNearby {
			return around(rs, self, n), len(rs), nil
		}
		return window(rs, int(o.Offset), n), len(rs), nil
	}
	start := int64(o.Offset) + 1
	if m == modeNearby {
		me, err := s.LB.ByAccounts(ctx, &lbpb.ByAccountsRequest{Ref: ref, Accounts: []string{self}})
		if err != nil {
			return nil, 0, err
		}
		if len(me.GetEntries()) == 0 {
			return nil, int(me.GetTotal()), nil // the player has no score: nothing is "nearby"
		}
		start = max(me.GetEntries()[0].GetPosition()-int64(n/2), 1)
	}
	r, err := s.LB.Range(ctx, &lbpb.RangeRequest{Ref: ref, Start: start, Limit: int64(n)})
	if err != nil {
		return nil, 0, err
	}
	return s.rows(ctx, r.GetEntries()), int(r.GetTotal()), nil
}

func (s *Store) filteredBoard(ctx context.Context, ref *lbpb.BoardRef, o rankingtypes.RankingOrderParam) ([]row, error) {
	es, err := s.all(ctx, ref)
	if err != nil {
		return nil, err
	}
	rs := keep(s.rows(ctx, es), o)
	renumber(rs)
	return rs, nil
}

// around is the window of n rows centred on account, empty when the account is not listed.
func around(rs []row, account string, n int) []row {
	for i, r := range rs {
		if r.account == account {
			return window(rs, i-n/2, n)
		}
	}
	return nil
}

// common fetches the common data of the listed rows in one call.
func (s *Store) common(ctx context.Context, rs []row) map[string][]byte {
	out := map[string][]byte{}
	if len(rs) == 0 {
		return out
	}
	owners := make([]string, 0, len(rs))
	seen := map[string]bool{}
	for _, r := range rs {
		if !seen[r.account] {
			seen[r.account] = true
			owners = append(owners, r.account)
		}
	}
	resp, err := s.TS.ListObjects(ctx, &tspb.ListObjectsRequest{Ref: s.tref(), Owners: owners, Collection: CommonCollection,
		IncludeBody: true, Limit: 1000})
	if err != nil {
		log.Printf("[%s] ranking common data: %v (rows sent without it)", s.Cfg.Title, err)
		return out
	}
	for _, o := range resp.GetObjects() {
		out[o.GetOwner()+"/"+o.GetName()] = o.GetBody()
	}
	return out
}

func (s *Store) toNEX(ctx context.Context, category uint32, rs []row, o rankingtypes.RankingOrderParam) list {
	common := s.common(ctx, rs)
	out := make(list, 0, len(rs))
	for _, r := range rs {
		d := rankingtypes.NewRankingRankData()
		d.PrincipalID = types.NewPID(r.in.PID)
		d.UniqueID = types.NewUInt64(r.in.UID)
		ord := r.rank
		if o.OrderCalculation == constants.OrderCalculation123 {
			ord = r.pos
		}
		d.Order = types.NewUInt32(uint32(ord))
		d.Category = types.NewUInt32(category)
		d.Score = types.NewUInt32(uint32(r.score))
		d.Groups = types.NewBuffer(r.in.Groups)
		d.Param = types.NewUInt64(r.in.Param)
		d.CommonData = types.NewBuffer(common[r.account+"/"+strconv.FormatUint(r.in.UID, 10)])
		var dt types.DateTime
		d.UpdateTime = dt.FromTimestamp(r.updated)
		out = append(out, d)
	}
	return out
}

// Global is the whole board, best first.
func (s *Store) Global(c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeGlobal, 0, c, o)
}

// Nearby is the window around the requester.
func (s *Store) Nearby(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeNearby, pid, c, o)
}

// Friends is the board of the requester and their friends (the account core's friend list).
func (s *Store) Friends(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeFriends, pid, c, o)
}

// NearbyFriends is the window around the requester among friends.
func (s *Store) NearbyFriends(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeNearbyFriends, pid, c, o)
}

// Own is the requester's own entry.
func (s *Store) Own(pid types.PID, c types.UInt32, o rankingtypes.RankingOrderParam) (list, uint32, error) {
	return s.read(modeOwn, pid, c, o)
}

// ---- the rankings contract (website) and title APIs ----

// Boards lists the title's boards on the rankings contract.
func (s *Store) Boards(ctx context.Context) ([]rankingsapi.Board, error) {
	r, err := s.LB.ListBoards(ctx, &lbpb.ListBoardsRequest{Network: s.Cfg.Network, Title: s.Cfg.Title})
	if notFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]rankingsapi.Board, 0, len(r.GetBoards()))
	for _, b := range r.GetBoards() {
		p := b.GetPolicy()
		out = append(out, rankingsapi.Board{ID: b.GetId(), Name: p.GetName(), Kind: p.GetKind(), Order: p.GetOrder(), Unit: p.GetUnit(), Season: p.GetSeason()})
	}
	sort.Slice(out, func(i, j int) bool {
		a, ea := strconv.ParseUint(out[i].ID, 10, 64)
		b, eb := strconv.ParseUint(out[j].ID, 10, 64)
		if ea == nil && eb == nil {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Rows answers the rankings contract for one board. Rows carry AccountID; Around is an account id.
func (s *Store) Rows(ctx context.Context, board string, q rankingsapi.Query) (rankingsapi.Page, error) {
	ref := &lbpb.BoardRef{Network: s.Cfg.Network, Title: s.Cfg.Title, Board: board}
	r, err := s.LB.Range(ctx, &lbpb.RangeRequest{Ref: ref, Start: int64(q.Offset) + 1, Limit: int64(q.Limit)})
	if notFound(err) {
		return rankingsapi.Page{}, rankingsapi.ErrNoBoard
	}
	if err != nil {
		return rankingsapi.Page{}, err
	}
	p := rankingsapi.Page{Total: int(r.GetTotal()), Rows: contractRows(r.GetEntries())}
	if q.Around != "" {
		me, err := s.LB.ByAccounts(ctx, &lbpb.ByAccountsRequest{Ref: ref, Accounts: []string{q.Around}})
		if err == nil && len(me.GetEntries()) > 0 {
			start := max(me.GetEntries()[0].GetPosition()-rankingsapi.AroundSpan, 1)
			if a, err := s.LB.Range(ctx, &lbpb.RangeRequest{Ref: ref, Start: start, Limit: 2*rankingsapi.AroundSpan + 1}); err == nil {
				p.Around = contractRows(a.GetEntries())
			}
		}
	}
	return p, nil
}

func contractRows(es []*lbpb.Entry) []rankingsapi.Row {
	out := make([]rankingsapi.Row, 0, len(es))
	for _, e := range es {
		r := rankingsapi.Row{Rank: int(e.GetRank()), AccountID: e.GetAccount(), Score: e.GetScore(), UpdatedAt: time.UnixMilli(e.GetUpdatedUnixMs()).UTC()}
		if in, ok := decodeInfo(e.GetInfo()); ok {
			r.PID = strconv.FormatUint(in.PID, 10)
		}
		out = append(out, r)
	}
	return out
}

// Entry is one ranked score for a title's own API.
type Entry struct {
	PID      uint64
	Score    int64
	Rank     int64 // competition rank
	Position int64
	Updated  time.Time
}

// Page returns a board's window (offset 0-based) and its total; a board with no scores is empty.
func (s *Store) Page(ctx context.Context, category uint32, offset, limit int) ([]Entry, int, error) {
	r, err := s.LB.Range(ctx, &lbpb.RangeRequest{Ref: s.ref(category), Start: int64(offset) + 1, Limit: int64(limit)})
	if notFound(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return entries(s.rows(ctx, r.GetEntries())), int(r.GetTotal()), nil
}

// PlayerBoard is a player's place on one board.
type PlayerBoard struct {
	Category uint32
	Entry    Entry
	Of       int
}

// Player lists a PID's score on every board of the title.
func (s *Store) Player(ctx context.Context, pid uint64) ([]PlayerBoard, error) {
	acct, err := s.IDs.Account(ctx, pid)
	if errors.Is(err, coreid.ErrNoLink) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	boards, err := s.Boards(ctx)
	if err != nil {
		return nil, err
	}
	var out []PlayerBoard
	for _, b := range boards {
		c, err := strconv.ParseUint(b.ID, 10, 32)
		if err != nil {
			continue
		}
		r, err := s.LB.ByAccounts(ctx, &lbpb.ByAccountsRequest{Ref: s.ref(uint32(c)), Accounts: []string{acct}})
		if err != nil {
			return nil, err
		}
		if es := entries(s.rows(ctx, r.GetEntries())); len(es) > 0 {
			out = append(out, PlayerBoard{Category: uint32(c), Entry: es[0], Of: int(r.GetTotal())})
		}
	}
	return out, nil
}

// Counts is the number of ranked players per category.
func (s *Store) Counts(ctx context.Context) (map[uint32]int, error) {
	boards, err := s.Boards(ctx)
	if err != nil {
		return nil, err
	}
	out := map[uint32]int{}
	for _, b := range boards {
		c, err := strconv.ParseUint(b.ID, 10, 32)
		if err != nil {
			continue
		}
		t, err := s.LB.Total(ctx, &lbpb.TotalRequest{Ref: s.ref(uint32(c))})
		if err != nil {
			return nil, err
		}
		if t.GetTotal() > 0 {
			out[uint32(c)] = int(t.GetTotal())
		}
	}
	return out, nil
}

func entries(rs []row) []Entry {
	out := make([]Entry, 0, len(rs))
	for _, r := range rs {
		out = append(out, Entry{PID: r.in.PID, Score: r.score, Rank: r.rank, Position: r.pos, Updated: r.updated})
	}
	return out
}

// ---- pending rows (no link yet, or the core was down) ----

// InitTables creates the pending tables in the title database (no-op without DB).
func (s *Store) InitTables() error {
	if s.DB == nil {
		return nil
	}
	for _, q := range []string{
		`CREATE SCHEMA IF NOT EXISTS corestore`,
		`CREATE TABLE IF NOT EXISTS corestore.pending_scores (
			id bigserial PRIMARY KEY,
			owner_pid numeric(20) NOT NULL,
			unique_id numeric(20) NOT NULL,
			category bigint NOT NULL,
			score bigint NOT NULL,
			order_by smallint NOT NULL,
			update_mode smallint NOT NULL,
			groups bytea NOT NULL DEFAULT '',
			param numeric(20) NOT NULL,
			created timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS corestore.pending_common_data (
			owner_pid numeric(20) NOT NULL,
			unique_id numeric(20) NOT NULL,
			data bytea NOT NULL,
			created timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (owner_pid, unique_id)
		)`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			return fmt.Errorf("corestore schema: %w", err)
		}
	}
	return nil
}

// retryable says whether a failed write may wait in a pending row: no link yet, or the core is
// unreachable. A refusal (bad argument, too large) goes back to the console instead.
func retryable(err error) bool {
	if errors.Is(err, coreid.ErrNoLink) {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown, codes.Canceled:
		return true
	}
	return false
}

func (s *Store) spillScore(ctx context.Context, pid uint64, d rankingtypes.RankingScoreData, uid uint64, cause error) error {
	if s.DB == nil || !retryable(cause) {
		return sysErr(cause)
	}
	_, err := s.DB.ExecContext(context.WithoutCancel(ctx), `INSERT INTO corestore.pending_scores
		(owner_pid, unique_id, category, score, order_by, update_mode, groups, param) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		strconv.FormatUint(pid, 10), strconv.FormatUint(uid, 10), int64(uint32(d.Category)), int64(uint32(d.Score)),
		int(d.OrderBy), int(d.UpdateMode), []byte(d.Groups), strconv.FormatUint(uint64(d.Param), 10))
	if err != nil {
		return sysErr(fmt.Errorf("core: %v; pending row: %w", cause, err))
	}
	if !errors.Is(cause, coreid.ErrNoLink) {
		log.Printf("[%s] ranking score kept pending: core: %v", s.Cfg.Title, cause)
	}
	return nil
}

func (s *Store) spillCommon(ctx context.Context, pid, uid uint64, data []byte, cause error) error {
	if s.DB == nil || !retryable(cause) {
		return sysErr(cause)
	}
	if data == nil {
		data = []byte{}
	}
	_, err := s.DB.ExecContext(context.WithoutCancel(ctx), `INSERT INTO corestore.pending_common_data (owner_pid, unique_id, data)
		VALUES ($1,$2,$3) ON CONFLICT (owner_pid, unique_id) DO UPDATE SET data = EXCLUDED.data, created = now()`,
		strconv.FormatUint(pid, 10), strconv.FormatUint(uid, 10), data)
	if err != nil {
		return sysErr(fmt.Errorf("core: %v; pending row: %w", cause, err))
	}
	if !errors.Is(cause, coreid.ErrNoLink) {
		log.Printf("[%s] ranking common data kept pending: core: %v", s.Cfg.Title, cause)
	}
	return nil
}

// Drain moves pending rows whose player now has a link into the core, oldest first, and deletes
// each row only once the core has it. It returns how many rows moved.
func (s *Store) Drain(ctx context.Context) (int, error) {
	if s.DB == nil {
		return 0, nil
	}
	moved := 0
	rows, err := s.DB.QueryContext(ctx, `SELECT id, owner_pid::text, unique_id::text, category, score, order_by, update_mode, groups, param::text
		FROM corestore.pending_scores ORDER BY id LIMIT 1000`)
	if err != nil {
		return 0, err
	}
	type pend struct {
		id       int64
		pid, uid uint64
		d        rankingtypes.RankingScoreData
	}
	var ps []pend
	for rows.Next() {
		var p pend
		var pidS, uidS, paramS string
		var cat, score int64
		var ob, um int
		var groups []byte
		if err := rows.Scan(&p.id, &pidS, &uidS, &cat, &score, &ob, &um, &groups, &paramS); err != nil {
			rows.Close()
			return 0, err
		}
		p.pid, _ = strconv.ParseUint(pidS, 10, 64)
		p.uid, _ = strconv.ParseUint(uidS, 10, 64)
		param, _ := strconv.ParseUint(paramS, 10, 64)
		p.d = rankingtypes.NewRankingScoreData()
		p.d.Category, p.d.Score = types.NewUInt32(uint32(cat)), types.NewUInt32(uint32(score))
		p.d.OrderBy, p.d.UpdateMode = constants.OrderBy(ob), constants.UpdateMode(um)
		p.d.Groups, p.d.Param = types.NewBuffer(groups), types.NewUInt64(param)
		ps = append(ps, p)
	}
	rows.Close()
	for _, p := range ps {
		if err := s.submit(ctx, p.pid, p.d, p.uid); err != nil {
			continue // still no link, or the core is still down: next round
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM corestore.pending_scores WHERE id = $1`, p.id); err != nil {
			return moved, err
		}
		moved++
	}
	crow, err := s.DB.QueryContext(ctx, `SELECT owner_pid::text, unique_id::text, data FROM corestore.pending_common_data ORDER BY created LIMIT 1000`)
	if err != nil {
		return moved, err
	}
	type cpend struct {
		pid, uid uint64
		data     []byte
	}
	var cs []cpend
	for crow.Next() {
		var c cpend
		var pidS, uidS string
		if err := crow.Scan(&pidS, &uidS, &c.data); err != nil {
			crow.Close()
			return moved, err
		}
		c.pid, _ = strconv.ParseUint(pidS, 10, 64)
		c.uid, _ = strconv.ParseUint(uidS, 10, 64)
		cs = append(cs, c)
	}
	crow.Close()
	for _, c := range cs {
		if err := s.putCommon(ctx, c.pid, c.uid, c.data); err != nil {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM corestore.pending_common_data WHERE owner_pid = $1::numeric AND unique_id = $2::numeric`,
			strconv.FormatUint(c.pid, 10), strconv.FormatUint(c.uid, 10)); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

var drainOnce sync.Once

// StartDrain runs Drain now and every ten minutes, for the life of the process.
func (s *Store) StartDrain() {
	drainOnce.Do(func() {
		go func() {
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				if n, err := s.Drain(ctx); err != nil {
					log.Printf("[%s] ranking pending rows: %v", s.Cfg.Title, err)
				} else if n > 0 {
					log.Printf("[%s] ranking pending rows moved to the core: %d", s.Cfg.Title, n)
				}
				cancel()
				time.Sleep(10 * time.Minute)
			}
		}()
	})
}
