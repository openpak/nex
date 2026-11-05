package nexhost

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestRunShutdownRunsHooksWithinBound(t *testing.T) {
	var ran atomic.Bool
	OnShutdown(func(ctx context.Context) { <-ctx.Done() }) // runs last (reverse order), hangs
	OnShutdown(func(context.Context) { ran.Store(true) })
	start := time.Now()
	if RunShutdown(200 * time.Millisecond) {
		t.Fatal("hanging hook should report timeout")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("shutdown took %v, bound is 200ms", d)
	}
	if !ran.Load() {
		t.Fatal("hook did not run")
	}
	shutdownMu.Lock()
	shutdownHooks = nil
	shutdownMu.Unlock()
}

func TestExitOnSignalSIGTERMExitsZero(t *testing.T) {
	code := make(chan int, 1)
	ExitOnSignal(func(c int) { code <- c })
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-code:
		if c != 0 {
			t.Fatalf("exit code %d, want 0", c)
		}
	case <-time.After(ShutdownTimeout + time.Second):
		t.Fatal("no exit after SIGTERM within bound")
	}
}
