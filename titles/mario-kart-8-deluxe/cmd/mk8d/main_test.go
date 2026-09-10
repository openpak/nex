package main

import (
	"testing"

	nexhost "github.com/openpak/nex"
)

// TestConfigDefaults pins the four per-title values. They are the difference between a
// server a console can talk to and one it cannot, and none of them is the host's default.
func TestConfigDefaults(t *testing.T) {
	t.Setenv("NEX_PORT_BASE", "22150")
	t.Setenv("NEX_ACCESS_KEY", "09c1c475")
	t.Setenv("NX_INTERNAL_KEY", "k")

	cfg := config()
	if cfg.Title != "mario-kart-8-deluxe" {
		t.Errorf("title = %q", cfg.Title)
	}
	if cfg.Version != "4.0.0" {
		t.Errorf("NEX version = %q, want 4.0.0: MK8D is a 4.0.0 title and 4.4.0+ would make the console use login method 6", cfg.Version)
	}
	if cfg.Transport != "websocket" {
		t.Errorf("transport = %q, want websocket: a Switch never speaks PRUDP over UDP", cfg.Transport)
	}
	if cfg.StationScheme != "prudp" {
		t.Errorf("station scheme = %q, want prudp", cfg.StationScheme)
	}
	if _, err := nexhost.New(cfg); err != nil {
		t.Fatalf("the defaults do not make a valid host config: %v", err)
	}
}

// TestConfigEnvWins guards the override: an operator setting the variable must not have it
// quietly replaced by the title default.
func TestConfigEnvWins(t *testing.T) {
	t.Setenv("NEX_PORT_BASE", "22150")
	t.Setenv("NEX_ACCESS_KEY", "09c1c475")
	t.Setenv("NX_INTERNAL_KEY", "k")
	t.Setenv("NEX_VERSION", "4.4.0")
	t.Setenv("NEX_TRANSPORT", "udp")
	t.Setenv("NEX_STATION_SCHEME", "prudps")

	cfg := config()
	if cfg.Version != "4.4.0" || cfg.Transport != "udp" || cfg.StationScheme != "prudps" {
		t.Errorf("environment overridden by title defaults: %+v", cfg)
	}
}
