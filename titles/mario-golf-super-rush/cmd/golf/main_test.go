package main

import (
	"bufio"
	"os"
	"strings"
	"testing"

	nexhost "github.com/openpak/nex"
)

// Golf must come up in the Switch shape: WebSocket transport, and the secure endpoint on
// its own stream, which is what puts both endpoints on one listener. Neither is visible in
// a passing server -- left wrong, this serves happily on two ports and no console ever
// completes a connection.
func TestConfigDefaults(t *testing.T) {
	t.Setenv("NEX_PORT_BASE", "22130")

	cfg := config()
	if cfg.Transport != "websocket" {
		t.Errorf("transport = %q, want websocket", cfg.Transport)
	}
	if !cfg.SingleListener() {
		t.Errorf("secure stream %d equals auth stream %d: want one listener carrying both",
			cfg.SecureStreamID, cfg.StreamID)
	}
	if cfg.Title != "mario-golf-super-rush" {
		t.Errorf("title = %q, want mario-golf-super-rush", cfg.Title)
	}
	if cfg.AuthPort() != 22130 {
		t.Errorf("auth port = %d, want 22130", cfg.AuthPort())
	}

	t.Setenv("NEX_SECURE_STREAM_ID", "1")
	t.Setenv("NEX_TITLE", "golf-on-a-laptop")
	cfg = config()
	if cfg.SingleListener() || cfg.Title != "golf-on-a-laptop" {
		t.Errorf("overrides ignored: secure stream %d title %q", cfg.SecureStreamID, cfg.Title)
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
	if cfg.AuthPort() != 22130 || cfg.HTTPAddr != ":22132" {
		t.Errorf("block 22130 in ports.md, got listener %d healthz %s", cfg.AuthPort(), cfg.HTTPAddr)
	}
	if cfg.Transport != "websocket" || !cfg.SingleListener() {
		t.Errorf("transport = %q, single listener = %v: want websocket on one listener",
			cfg.Transport, cfg.SingleListener())
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
