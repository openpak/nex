package main

import (
	"testing"

	nexhost "openpak/nex"
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

// TestSwitchByteStream is the one that matters most: a NEX 4 Switch title reads 8-byte PIDs
// and a header in front of every structure, and nex-go defaults to neither.
func TestSwitchByteStream(t *testing.T) {
	host, err := nexhost.New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if host.Auth.Server.ByteStreamSettings.PIDSize == 8 || host.Auth.Server.ByteStreamSettings.UseStructureHeader {
		t.Fatal("something already applies the Switch profile; switchByteStream and this test can go")
	}

	switchByteStream(host)

	for name, settings := range map[string]*struct {
		size   int
		header bool
	}{
		"auth":   {host.Auth.Server.ByteStreamSettings.PIDSize, host.Auth.Server.ByteStreamSettings.UseStructureHeader},
		"secure": {host.Secure.Server.ByteStreamSettings.PIDSize, host.Secure.Server.ByteStreamSettings.UseStructureHeader},
	} {
		if settings.size != 8 {
			t.Errorf("%s PID size = %d, want 8", name, settings.size)
		}
		if !settings.header {
			t.Errorf("%s endpoint is not writing structure headers", name)
		}
	}
}

func testConfig() nexhost.Config {
	return nexhost.Config{
		Title: "mk8d-test", PortBase: 22150, AccessKey: "09c1c475", Version: "4.0.0",
		SecureHost: "127.0.0.1", StationScheme: "prudp", Transport: "websocket",
		StreamID: 1, SecureStreamID: 1, SecurePID: 2, HTTPAddr: ":0",
		InternalURL: "http://127.0.0.1:1", InternalKey: "k",
	}
}
