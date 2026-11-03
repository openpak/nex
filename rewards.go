package nexhost

// Rewards events: the one place NEX titles tell the rewards service that a player did
// something the server itself saw (a Splatfest match uploaded, a course cleared, a
// tournament run submitted). See rewards/docs/metrics.md for the metric names.
//
// The client is async and drops events rather than ever slowing or failing a NEX call; with
// REWARDS_EVENTS_URL / REWARDS_EVENTS_KEY unset every call here is a no-op.

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	emit "github.com/openpak/rewards-emit"
)

var (
	rewardsOnce   sync.Once
	rewardsClient *emit.Client
)

// Rewards returns the process-wide events client (source "nex-<NEX_TITLE>"). Never nil.
func Rewards() *emit.Client {
	rewardsOnce.Do(func() {
		// Every NEX unit is on the openpak.network bridge, where the shared .env's host-network
		// REWARDS_EMIT_DATABASE_URL (127.0.0.1) does not resolve; the bridge URL wins when set.
		if u := os.Getenv("REWARDS_EMIT_DATABASE_URL_BRIDGE"); u != "" {
			os.Setenv("REWARDS_EMIT_DATABASE_URL", u)
		}
		rewardsClient = emit.FromEnv("nex-" + env("NEX_TITLE", "host"))
	})
	return rewardsClient
}

// FlushRewards delivers what is queued, for at most two seconds; call it on shutdown.
func FlushRewards() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	Rewards().Close(ctx)
}

// PlayerOf names a Wii U / 3DS player for the rewards service: the account core links them as
// namespace "wiiu" (or "3ds") with the PID as the subject; rewards tries both.
//
// Switch titles are linked as namespace "switch" with the nx-baas PID as 16 hex digits, the same
// subject switch-nex sends.
func PlayerOf(namespace string, pid uint64) *emit.Identity {
	if namespace == "switch" {
		return &emit.Identity{Namespace: namespace, Subject: fmt.Sprintf("%016x", pid)}
	}
	return &emit.Identity{Namespace: namespace, Subject: strconv.FormatUint(pid, 10)}
}

// UseRewards swaps the process-wide client; tests point it at a fake rewards service.
func UseRewards(c *emit.Client) {
	rewardsOnce.Do(func() {})
	rewardsClient = c
}
