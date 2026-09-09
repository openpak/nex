package main

import (
	"bufio"
	"os"
	"strings"
	"testing"

	nexhost "openpak/nex"
)

// Golf must default to the WebSocket transport, not the shared host's UDP: a Switch NEX
// title never speaks PRUDP over UDP, and a server that quietly listened on the wrong one
// would look healthy and answer nothing.
func TestConfigDefaults(t *testing.T) {
	t.Setenv("NEX_PORT_BASE", "22130")

	cfg := config()
	if cfg.Transport != "websocket" {
		t.Errorf("transport = %q, want websocket", cfg.Transport)
	}
	if cfg.Title != "mario-golf-super-rush" {
		t.Errorf("title = %q, want mario-golf-super-rush", cfg.Title)
	}
	if cfg.AuthPort() != 22130 || cfg.SecurePort() != 22131 {
		t.Errorf("ports = %d/%d, want 22130/22131", cfg.AuthPort(), cfg.SecurePort())
	}

	t.Setenv("NEX_TRANSPORT", "udp")
	t.Setenv("NEX_TITLE", "golf-on-a-laptop")
	cfg = config()
	if cfg.Transport != "udp" || cfg.Title != "golf-on-a-laptop" {
		t.Errorf("overrides ignored: transport %q title %q", cfg.Transport, cfg.Title)
	}
}

// The configuration this repository ships must actually be a configuration the shared host
// accepts, and must agree with the port block claimed in ports.md. Both drift silently
// otherwise: a bad block or a dropped access key only shows up when a console cannot
// connect.
func TestExampleEnvIsValid(t *testing.T) {
	for name, value := range readEnvFile(t, "../../example.env") {
		t.Setenv(name, value)
	}
	// The one value example.env deliberately leaves empty, because it is a secret.
	t.Setenv("NX_INTERNAL_KEY", "test-key")

	cfg := config()
	if cfg.AuthPort() != 22130 || cfg.SecurePort() != 22131 || cfg.HTTPAddr != ":22132" {
		t.Errorf("block 22130 in ports.md, got auth %d secure %d healthz %s",
			cfg.AuthPort(), cfg.SecurePort(), cfg.HTTPAddr)
	}
	if cfg.Transport != "websocket" {
		t.Errorf("transport = %q, want websocket", cfg.Transport)
	}
	if _, err := nexhost.New(cfg); err != nil {
		t.Errorf("example.env is not a configuration the host accepts: %v", err)
	}
}

func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		values[name] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}
