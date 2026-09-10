package mps

import (
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	match_making_constants "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/constants"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
)

func TestParseIntegerSettings(t *testing.T) {
	settings := parseIntegerSettings("0:0=60, 0:1=30 10:7=-1, nonsense, 0:bad=1, 1:2=notanumber")

	if len(settings) != 2 {
		t.Fatalf("want settings for two indexes, got %d: %v", len(settings), settings)
	}
	if got := settings[0][types.NewUInt16(0)]; got != types.NewUInt32(60) {
		t.Errorf("index 0 key 0: want 60, got %d", got)
	}
	if got := settings[0][types.NewUInt16(1)]; got != types.NewUInt32(30) {
		t.Errorf("index 0 key 1: want 30, got %d", got)
	}
	// Signed on the wire, so -1 has to survive as the same 32 bits.
	if got := settings[10][types.NewUInt16(7)]; int32(got) != -1 {
		t.Errorf("index 10 key 7: want -1, got %d", int32(got))
	}
	if _, ok := settings[1]; ok {
		t.Error("an entry with an unparsable value must be skipped, not stored")
	}
}

func TestParseIntegerSettingsEmpty(t *testing.T) {
	if settings := parseIntegerSettings(""); len(settings) != 0 {
		t.Errorf("unset MPS_INTEGER_SETTINGS must configure nothing, got %v", settings)
	}
}

func TestApplyResultOptionDropsUnrequestedFields(t *testing.T) {
	session := match_making_types.NewMatchmakeSession()
	session.ApplicationBuffer = types.NewBuffer([]byte{1, 2, 3})
	session.MatchmakeParam.Params = types.NewMap[types.String, types.Variant]()
	session.MatchmakeParam.Params[types.NewString("x")] = types.NewVariant()

	kept := applyResultOption(session, match_making_constants.FindMatchmakeSessionResultOptionApplicationBuffer)
	if len(kept.ApplicationBuffer) != 3 {
		t.Errorf("a requested application buffer must survive, got %v", kept.ApplicationBuffer)
	}
	if len(kept.MatchmakeParam.Params) != 0 {
		t.Errorf("an unrequested matchmake param must be cleared, got %v", kept.MatchmakeParam.Params)
	}

	dropped := applyResultOption(session, match_making_constants.FindMatchmakeSessionResultOptionNone)
	if len(dropped.ApplicationBuffer) != 0 {
		t.Errorf("an unrequested application buffer must be cleared, got %v", dropped.ApplicationBuffer)
	}
}

func TestFromEnvDefaultsToThisTitle(t *testing.T) {
	t.Setenv("NEX_ACCESS_KEY", "e915510f")
	t.Setenv("NX_INTERNAL_KEY", "test")
	t.Setenv("MPS_DATABASE_URL", "postgres://localhost/test")

	cfg := FromEnv()

	if cfg.Host.PortBase != PortBase {
		t.Errorf("want the title's own block %d, got %d", PortBase, cfg.Host.PortBase)
	}
	if cfg.Host.Version != NEXVersion {
		t.Errorf("want NEX %s, got %s", NEXVersion, cfg.Host.Version)
	}
	if cfg.Host.HTTPAddr != ":22112" {
		t.Errorf("want healthz on the block's +2, got %s", cfg.Host.HTTPAddr)
	}
	// Both of these are per-title measured values, not house defaults. The wrong station
	// scheme fails late, when the game needs the session; the wrong secure stream ID puts
	// the secure endpoint where this title does not look for it.
	if cfg.Host.StationScheme != StationScheme {
		t.Errorf("want station scheme %s, got %s", StationScheme, cfg.Host.StationScheme)
	}
	if cfg.Host.SecureStreamID != SecureStreamID {
		t.Errorf("want secure stream ID %d (two listeners), got %d", SecureStreamID, cfg.Host.SecureStreamID)
	}
	// The wrong source key length fails silently: the login just does not work, and
	// nothing is logged on either side. Pinned rather than inherited for that reason.
	if cfg.Host.SourceKeyBytes != SourceKeyBytes {
		t.Errorf("want a %d-byte source key, got %d", SourceKeyBytes, cfg.Host.SourceKeyBytes)
	}
}

func TestFromEnvHonoursOverrides(t *testing.T) {
	t.Setenv("NEX_PORT_BASE", "22300")
	t.Setenv("NEX_VERSION", "4.4.0")
	t.Setenv("NEX_ACCESS_KEY", "e915510f")
	t.Setenv("NX_INTERNAL_KEY", "test")

	cfg := FromEnv()

	if cfg.Host.PortBase != 22300 {
		t.Errorf("NEX_PORT_BASE must win, got %d", cfg.Host.PortBase)
	}
	if cfg.Host.Version != "4.4.0" {
		t.Errorf("NEX_VERSION must win, got %s", cfg.Host.Version)
	}
}

func TestFromEnvHonoursWireShapeOverrides(t *testing.T) {
	t.Setenv("NEX_ACCESS_KEY", "e915510f")
	t.Setenv("NX_INTERNAL_KEY", "test")
	t.Setenv("NEX_STATION_SCHEME", "prudp")
	t.Setenv("NEX_SECURE_STREAM_ID", "2")

	cfg := FromEnv()

	if cfg.Host.StationScheme != "prudp" {
		t.Errorf("NEX_STATION_SCHEME must win, got %s", cfg.Host.StationScheme)
	}
	if cfg.Host.SecureStreamID != 2 {
		t.Errorf("NEX_SECURE_STREAM_ID must win, got %d", cfg.Host.SecureStreamID)
	}
}
