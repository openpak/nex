package nexhost

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ShutdownTimeout bounds the whole graceful stop. systemd/podman wait 10 s before SIGKILL.
const ShutdownTimeout = 5 * time.Second

var (
	shutdownMu    sync.Mutex
	shutdownHooks []func(context.Context)
)

// OnShutdown registers fn to run when the process is asked to stop (SIGTERM/SIGINT). Hooks run
// in reverse order of registration and share one deadline; they must respect ctx.
func OnShutdown(fn func(context.Context)) {
	shutdownMu.Lock()
	shutdownHooks = append(shutdownHooks, fn)
	shutdownMu.Unlock()
}

// RunShutdown runs the registered hooks and the built-in cleanup (login-name DB pool) within
// timeout. It returns true if everything finished in time; it never blocks past the bound.
func RunShutdown(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		shutdownMu.Lock()
		hooks := append([]func(context.Context){}, shutdownHooks...)
		shutdownMu.Unlock()
		for i := len(hooks) - 1; i >= 0; i-- {
			hooks[i](ctx)
		}
		loginMu.Lock()
		if loginDB != nil {
			_ = loginDB.Close()
			loginDB = nil
		}
		loginMu.Unlock()
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// ExitOnSignal waits for SIGTERM or SIGINT in the background, then runs RunShutdown and calls
// exit(0). A stop requested by systemd/podman is a clean exit, not a failure (Go's default
// for an unhandled SIGTERM is exit status 2).
func ExitOnSignal(exit func(int)) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-ch
		log.Printf("received %v, shutting down", s)
		if !RunShutdown(ShutdownTimeout) {
			log.Printf("shutdown hooks exceeded %v, exiting anyway", ShutdownTimeout)
		}
		exit(0)
	}()
}
