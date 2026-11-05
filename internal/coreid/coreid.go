// Package coreid maps Wii U and 3DS NEX PIDs to OpenPak account ids and back, through
// nn-account's Resolution service (the same door erase.go and the friend graph use), with a
// short in-process cache so a ranking or DataStore call does not pay a lookup per row.
//
// Positive answers are cached for TTL (five minutes by default); "no such link" is cached
// for NegativeTTL (thirty seconds), so a player who links during a session is picked up
// quickly. Errors are never cached.
package coreid

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	resolutionv1 "github.com/openpak/nex/internal/resolutionpb"
)

// ErrNoLink means the PID (or the account) has no OpenPak counterpart on this network.
var ErrNoLink = errors.New("coreid: no account link")

// Resolver maps PIDs to accounts and back.
type Resolver interface {
	Account(ctx context.Context, pid uint64) (string, error)
	PID(ctx context.Context, account string) (uint64, error)
}

// Cache is a Resolver over an uncached Resolver with positive and negative TTLs.
type Cache struct {
	L           Resolver
	TTL         time.Duration // default 5 minutes
	NegativeTTL time.Duration // default 30 seconds
	Now         func() time.Time

	mu sync.Mutex
	m  map[string]entry
}

type entry struct {
	account string
	pid     uint64
	none    bool
	exp     time.Time
}

// maxEntries bounds the cache; past it the cache starts over (rare: one title, few players).
const maxEntries = 50000

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Cache) get(k string) (entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok || c.now().After(e.exp) {
		return entry{}, false
	}
	return e, true
}

func (c *Cache) put(k string, e entry) {
	ttl := c.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	if e.none {
		ttl = c.NegativeTTL
		if ttl == 0 {
			ttl = 30 * time.Second
		}
	}
	e.exp = c.now().Add(ttl)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= maxEntries {
		c.m = map[string]entry{}
	}
	c.m[k] = e
}

func pidKey(pid uint64) string { return "p" + strconv.FormatUint(pid, 10) }

// Account resolves a PID.
func (c *Cache) Account(ctx context.Context, pid uint64) (string, error) {
	if e, ok := c.get(pidKey(pid)); ok {
		if e.none {
			return "", ErrNoLink
		}
		return e.account, nil
	}
	a, err := c.L.Account(ctx, pid)
	if errors.Is(err, ErrNoLink) {
		c.put(pidKey(pid), entry{none: true})
		return "", ErrNoLink
	}
	if err != nil {
		return "", err
	}
	a = strings.ToLower(a)
	c.put(pidKey(pid), entry{account: a})
	c.put("a"+a, entry{pid: pid})
	return a, nil
}

// PID resolves an account.
func (c *Cache) PID(ctx context.Context, account string) (uint64, error) {
	account = strings.ToLower(account)
	if e, ok := c.get("a" + account); ok {
		if e.none {
			return 0, ErrNoLink
		}
		return e.pid, nil
	}
	p, err := c.L.PID(ctx, account)
	if errors.Is(err, ErrNoLink) {
		c.put("a"+account, entry{none: true})
		return 0, ErrNoLink
	}
	if err != nil {
		return 0, err
	}
	c.put("a"+account, entry{pid: p})
	c.put(pidKey(p), entry{account: account})
	return p, nil
}

// Resolution is the uncached Resolver over nn-account's Resolution gRPC for one namespace ("wiiu" or
// "3ds"). The key travels as X-API-Key, as nn-account expects.
type Resolution struct {
	C         resolutionv1.ResolutionClient
	Key       string
	Namespace string
}

// DialResolution connects to nn-account's gRPC.
func DialResolution(addr, key, namespace string) (*Resolution, error) {
	if addr == "" {
		return nil, errors.New("coreid: nn-account address is required")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Resolution{C: resolutionv1.NewResolutionClient(conn), Key: key, Namespace: namespace}, nil
}

func (r *Resolution) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	return metadata.AppendToOutgoingContext(ctx, "X-API-Key", r.Key), cancel
}

// Account implements Resolver. Wii U and 3DS PIDs are 32-bit; a larger one has no link.
func (r *Resolution) Account(ctx context.Context, pid uint64) (string, error) {
	if pid == 0 || pid > 0xFFFFFFFF {
		return "", ErrNoLink
	}
	ctx, cancel := r.ctx(ctx)
	defer cancel()
	resp, err := r.C.ResolvePid(ctx, &resolutionv1.ResolvePidRequest{Namespace: r.Namespace, Pid: uint32(pid)})
	if err != nil {
		return "", err
	}
	if !resp.GetFound() || resp.GetAccountId() == "" {
		return "", ErrNoLink
	}
	return resp.GetAccountId(), nil
}

// PID implements Resolver.
func (r *Resolution) PID(ctx context.Context, account string) (uint64, error) {
	ctx, cancel := r.ctx(ctx)
	defer cancel()
	resp, err := r.C.ResolveAccount(ctx, &resolutionv1.ResolveAccountRequest{Namespace: r.Namespace, AccountId: account})
	if err != nil {
		return 0, err
	}
	if !resp.GetFound() || resp.GetPid() == 0 {
		return 0, ErrNoLink
	}
	return uint64(resp.GetPid()), nil
}
