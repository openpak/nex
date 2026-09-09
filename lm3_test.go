package lm3

import (
	"testing"

	nexhost "github.com/openpak/nex"
)

// TestFromEnvDefaults: the per-title facts (docs/provenance.md) reach a valid host config.
// A wrong value here is a silent login failure on hardware, so this is the cheapest guard
// there is — it does not need anything running.
func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("NEX_ACCESS_KEY", "aab95246")
	t.Setenv("NX_INTERNAL_KEY", "test-internal-key")
	t.Setenv("LM3_DATABASE_URL", "postgres://x/y") // presence only; Run pings it, New does not

	cfg := FromEnv()

	if cfg.Host.PortBase != PortBase {
		t.Errorf("PortBase = %d, want %d", cfg.Host.PortBase, PortBase)
	}
	if cfg.Host.Version != NEXVersion {
		t.Errorf("Version = %q, want %q", cfg.Host.Version, NEXVersion)
	}
	if cfg.Host.StationScheme != StationScheme {
		t.Errorf("StationScheme = %q, want %q", cfg.Host.StationScheme, StationScheme)
	}
	if cfg.Host.SecureStreamID != SecureStreamID {
		t.Errorf("SecureStreamID = %d, want %d", cfg.Host.SecureStreamID, SecureStreamID)
	}
	if cfg.Host.SourceKeyBytes != SourceKeyBytes {
		t.Errorf("SourceKeyBytes = %d, want %d", cfg.Host.SourceKeyBytes, SourceKeyBytes)
	}

	// New validates the config and builds the endpoints (it does not listen). If any per-title
	// fact were out of range, this is where it fails.
	if _, err := nexhost.New(cfg.Host); err != nil {
		t.Fatalf("nexhost.New with LM3 defaults: %v", err)
	}
}

// TestFromEnvOverride: an explicit env var wins over the per-title default.
func TestFromEnvOverride(t *testing.T) {
	t.Setenv("NEX_ACCESS_KEY", "aab95246")
	t.Setenv("NX_INTERNAL_KEY", "test-internal-key")
	t.Setenv("NEX_VERSION", "4.0.3")

	if got := FromEnv().Host.Version; got != "4.0.3" {
		t.Errorf("Version override = %q, want 4.0.3", got)
	}
}

// TestPresenceResponseBody: the 0x79 stub acknowledges exactly the methods the game is known
// to call, and only those. Publish-only methods answer empty; location queries answer an
// empty record list (count = 0); everything else is left unhandled (NotImplemented).
func TestPresenceResponseBody(t *testing.T) {
	empty := func(b []byte) bool { return len(b) == 0 }
	countZero := func(b []byte) bool { return len(b) == 4 && b[0] == 0 && b[1] == 0 && b[2] == 0 && b[3] == 0 }

	for _, tc := range []struct {
		method  uint32
		handled bool
		check   func([]byte) bool
		name    string
	}{
		{presenceMethodUpdateApplicationBuffer, true, empty, "UpdateApplicationBuffer -> empty"},
		{presenceMethodUpdateState, true, empty, "UpdateState -> empty"},
		{presenceMethodInitializeStation, true, countZero, "InitializeStation -> count 0"},
		{presenceMethodGetStationLocation, true, countZero, "GetStationLocation -> count 0"},
		{1, false, nil, "unknown method not handled"},
		{99, false, nil, "unknown method not handled"},
	} {
		body, handled := presenceResponseBody(tc.method)
		if handled != tc.handled {
			t.Errorf("%s: handled = %v, want %v", tc.name, handled, tc.handled)
			continue
		}
		if handled && !tc.check(body) {
			t.Errorf("%s: body = %v failed its shape check", tc.name, body)
		}
	}
}
