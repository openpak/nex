package coreid

import (
	"context"
	"errors"
	"testing"
	"time"
)

type counting struct {
	calls int
	known map[uint64]string
}

func (c *counting) Account(_ context.Context, pid uint64) (string, error) {
	c.calls++
	if a, ok := c.known[pid]; ok {
		return a, nil
	}
	return "", ErrNoLink
}

func (c *counting) PID(_ context.Context, account string) (uint64, error) {
	c.calls++
	for p, a := range c.known {
		if a == account {
			return p, nil
		}
	}
	return 0, ErrNoLink
}

func TestCacheTTLs(t *testing.T) {
	now := time.Unix(0, 0)
	l := &counting{known: map[uint64]string{1: "ACCT-1"}}
	c := &Cache{L: l, Now: func() time.Time { return now }}
	ctx := context.Background()
	if a, err := c.Account(ctx, 1); err != nil || a != "acct-1" {
		t.Fatal(a, err)
	}
	if p, err := c.PID(ctx, "acct-1"); err != nil || p != 1 || l.calls != 1 {
		t.Fatalf("reverse lookup not served from cache: %d %v calls %d", p, err, l.calls)
	}
	if _, err := c.Account(ctx, 2); !errors.Is(err, ErrNoLink) {
		t.Fatal(err)
	}
	l.known[2] = "acct-2"
	now = now.Add(10 * time.Second)
	if _, err := c.Account(ctx, 2); !errors.Is(err, ErrNoLink) {
		t.Fatal("negative answer should be cached for 30 s")
	}
	now = now.Add(31 * time.Second)
	if a, _ := c.Account(ctx, 2); a != "acct-2" {
		t.Fatal("a new link must be seen after the negative TTL")
	}
	calls := l.calls
	now = now.Add(4 * time.Minute)
	c.Account(ctx, 1)
	if l.calls != calls {
		t.Fatal("positive answer should live 5 minutes")
	}
	now = now.Add(2 * time.Minute)
	c.Account(ctx, 1)
	if l.calls != calls+1 {
		t.Fatal("positive answer should expire")
	}
}

func TestResolutionRejectsWidePIDs(t *testing.T) {
	r := &Resolution{}
	if _, err := r.Account(context.Background(), 1<<33); !errors.Is(err, ErrNoLink) {
		t.Fatal(err)
	}
}
