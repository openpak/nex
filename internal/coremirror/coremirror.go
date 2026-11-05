// Package coremirror copies a title's NEX gatherings into the shared matchmaking core
// (platforms/core/infra/matchmaking) as sessions of one neutral title key, tagged with a
// compatibility group assigned by the server.
//
// The NEX layer (nex-protocols-common-go and its Postgres tables) keeps answering the console:
// this is the write-through step of the Terraria migration (prds/repos/terraria-unified-plan.md,
// section 3). The core gets the room, its members and its searchable attributes; nothing is
// read back yet. A restart is harmless: sessions are keyed by a unique key (the gathering id),
// creation is CreateOrJoin, joins are idempotent, and sessions of gatherings that vanished while
// the process was down expire with the core's TTL.
//
// The console sends no version string at matchmaking (observed on a real Wii U, card #145), so
// the compat group is not read from the session: it is configured per title (Compat).
package coremirror

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/openpak/nex/internal/coreid"
	mm "github.com/openpak/nex/internal/matchmakingpb"
)

// Gathering is the part of a title's MatchmakeSession the core needs.
type Gathering struct {
	ID           uint64
	Owner        uint64 // owner PID
	Host         uint64 // host PID, used when the owner has no link
	Participants []uint64
	Max          uint32
	GameMode     uint32
	Attribs      []uint32
	AppBuffer    []byte
	Open         bool
}

// Mirror holds the id maps for the sessions it created; the sessions themselves live in the core.
type Mirror struct {
	MM      mm.MatchmakingClient
	IDs     coreid.Resolver
	Title   string // neutral key, "terraria"
	Network string // account-link namespace, "wiiu"
	Compat  string // compatibility group assigned by the server, "old-gen-1.2.4"
	Console string // console tag as a searchable attribute, "wiiu"
	TTL     uint32 // session ttl seconds (0 = core default)
	Now     func() time.Time

	mu       sync.Mutex
	known    map[uint64]*sessionState
	lastBeat map[string]time.Time
	skipped  map[uint64]bool
}

type sessionState struct {
	id      string
	owner   string
	members map[string]uint64 // account -> pid
	sig     string
}

func (m *Mirror) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func uniqueKey(id uint64) string { return "g:" + strconv.FormatUint(id, 10) }

func intAttr(name string, v int64) *mm.Attr {
	return &mm.Attr{Name: name, Type: mm.AttrType_ATTR_TYPE_INT, IntValue: v, Searchable: true}
}

func strAttr(name, v string) *mm.Attr {
	return &mm.Attr{Name: name, Type: mm.AttrType_ATTR_TYPE_STRING, StringValue: v, Searchable: true}
}

func (m *Mirror) attrs(g Gathering) []*mm.Attr {
	out := []*mm.Attr{
		strAttr("compat", m.Compat),
		strAttr("console", m.Console),
		intAttr("game_mode", int64(g.GameMode)),
	}
	for i, v := range g.Attribs {
		out = append(out, intAttr("attr"+strconv.Itoa(i), int64(v)))
	}
	return out
}

func signature(g Gathering) string {
	return fmt.Sprintf("%d|%d|%v|%x|%t", g.Max, g.GameMode, g.Attribs, g.AppBuffer, g.Open)
}

func (m *Mirror) title() *mm.TitleRef { return &mm.TitleRef{Network: m.Network, Title: m.Title} }

func (m *Mirror) account(ctx context.Context, pid uint64) (string, bool) {
	a, err := m.IDs.Account(ctx, pid)
	if err != nil {
		if !errors.Is(err, coreid.ErrNoLink) {
			log.Printf("[%s] coremirror: resolve pid %d: %v", m.Title, pid, err)
		}
		return "", false
	}
	return a, true
}

// Tick reconciles the core with the gatherings present now. Errors of single calls are logged
// and retried by the next tick.
func (m *Mirror) Tick(ctx context.Context, gs []Gathering) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.known == nil {
		m.known = map[uint64]*sessionState{}
		m.lastBeat = map[string]time.Time{}
		m.skipped = map[uint64]bool{}
	}
	present := map[uint64]bool{}
	for _, g := range gs {
		present[g.ID] = true
		m.sync(ctx, g)
	}
	for id, st := range m.known {
		if present[id] {
			continue
		}
		// The gathering is gone: the owner leaves, which ends a room (migrate_owner is off).
		if _, err := m.MM.LeaveSession(ctx, &mm.LeaveSessionRequest{SessionId: st.id, AccountId: st.owner, Reason: "ended"}); err != nil {
			log.Printf("[%s] coremirror: end session of gathering %d: %v", m.Title, id, err)
			continue
		}
		delete(m.known, id)
	}
	for id := range m.skipped {
		if !present[id] {
			delete(m.skipped, id)
		}
	}
	m.heartbeats(ctx)
}

func (m *Mirror) sync(ctx context.Context, g Gathering) {
	ownerPID := g.Owner
	owner, ok := m.account(ctx, ownerPID)
	if !ok && g.Host != 0 && g.Host != g.Owner {
		ownerPID = g.Host
		owner, ok = m.account(ctx, ownerPID)
	}
	st := m.known[g.ID]
	if !ok {
		if st == nil && !m.skipped[g.ID] {
			m.skipped[g.ID] = true
			log.Printf("[%s] coremirror: gathering %d not mirrored: owner pid has no account link", m.Title, g.ID)
		}
		return
	}
	if st == nil {
		resp, err := m.MM.CreateOrJoinSession(ctx, &mm.CreateSessionRequest{
			Title: m.title(), Creator: owner, Network: m.Network, Kind: "room",
			UniqueKey: uniqueKey(g.ID), MaxMembers: g.Max, Closed: !g.Open,
			Attrs: m.attrs(g), Data: g.AppBuffer, TtlSeconds: m.TTL,
		})
		if err != nil {
			log.Printf("[%s] coremirror: create session for gathering %d: %v", m.Title, g.ID, err)
			return
		}
		st = &sessionState{id: resp.GetSession().GetId(), owner: owner, members: map[string]uint64{owner: ownerPID}, sig: signature(g)}
		m.known[g.ID] = st
		log.Printf("[%s] coremirror: gathering %d -> session %s (compat %s)", m.Title, g.ID, st.id, m.Compat)
	} else if sig := signature(g); sig != st.sig {
		_, err := m.MM.UpdateSession(ctx, &mm.UpdateSessionRequest{
			SessionId: st.id, Actor: st.owner, SetAttrs: m.attrs(g),
			Data: g.AppBuffer, SetData: true, SetOpen: true, Open: g.Open, SetMaxMembers: true, MaxMembers: g.Max,
		})
		if err != nil {
			log.Printf("[%s] coremirror: update session of gathering %d: %v", m.Title, g.ID, err)
		} else {
			st.sig = sig
		}
	}
	want := map[string]uint64{owner: ownerPID}
	for _, pid := range g.Participants {
		if pid == ownerPID {
			continue
		}
		if a, ok := m.account(ctx, pid); ok {
			want[a] = pid
		}
	}
	accounts := make([]string, 0, len(want))
	for a := range want {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	for _, a := range accounts {
		if _, in := st.members[a]; in {
			continue
		}
		if _, err := m.MM.JoinSession(ctx, &mm.JoinSessionRequest{SessionId: st.id, AccountId: a, Network: m.Network, AllowClosed: true}); err != nil {
			log.Printf("[%s] coremirror: join gathering %d: %v", m.Title, g.ID, err)
			continue
		}
		st.members[a] = want[a]
	}
	for a := range st.members {
		if _, in := want[a]; in || a == st.owner {
			continue
		}
		if _, err := m.MM.LeaveSession(ctx, &mm.LeaveSessionRequest{SessionId: st.id, AccountId: a, Reason: "leave"}); err != nil {
			log.Printf("[%s] coremirror: leave gathering %d: %v", m.Title, g.ID, err)
			continue
		}
		delete(st.members, a)
	}
}

// heartbeats keeps the memberships of the accounts in live sessions alive (core ttl 300 s).
func (m *Mirror) heartbeats(ctx context.Context) {
	now := m.now()
	live := map[string]bool{}
	for _, st := range m.known {
		for a := range st.members {
			live[a] = true
		}
	}
	for a := range live {
		if now.Sub(m.lastBeat[a]) < 60*time.Second {
			continue
		}
		if _, err := m.MM.Heartbeat(ctx, &mm.HeartbeatRequest{AccountId: a}); err != nil {
			log.Printf("[%s] coremirror: heartbeat: %v", m.Title, err)
			continue
		}
		m.lastBeat[a] = now
	}
	for a := range m.lastBeat {
		if !live[a] {
			delete(m.lastBeat, a)
		}
	}
}
