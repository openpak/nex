package smb35

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/PretendoNetwork/nex-go/v2/types"
)

// The handoff token is the one thing here a client cannot tell us we got wrong: the game
// carries it to the relay, and a relay that rejects it produces a lobby that hangs with no
// error. So check the shape a relay will see, field by field.
func TestBuildEagleToken(t *testing.T) {
	key := []byte("0123456789abcdef")
	expires := time.Unix(1700000000, 0)

	encoded, err := buildEagleToken(key, 1800000042, 1234, expires)
	if err != nil {
		t.Fatalf("buildEagleToken: %v", err)
	}

	envelope, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("token is not base64: %v", err)
	}

	// Decode into a bare map first: the field NAMES are the contract with the relay, and a
	// struct with json tags would happily accept a token that spelled them differently.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(envelope, &raw); err != nil {
		t.Fatalf("token is not JSON: %v", err)
	}
	for _, field := range []string{"payload", "signature", "version"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("token has no %q field: %s", field, envelope)
		}
	}

	var payload map[string]string
	if err := json.Unmarshal(raw["payload"], &payload); err != nil {
		t.Fatalf("payload is not a JSON object of strings: %v", err)
	}
	want := map[string]string{
		"expires_at": "1700000000",
		"server_env": "lp1",
		"server_id":  "1234",             // the gathering ID, decimal
		"user_id":    "000000006b49d22a", // the PID, sixteen hex digits
	}
	for field, expect := range want {
		if payload[field] != expect {
			t.Errorf("payload[%q] = %q, want %q", field, payload[field], expect)
		}
	}
	if len(payload) != len(want) {
		t.Errorf("payload has %d fields, want %d: %s", len(payload), len(want), raw["payload"])
	}

	// The signature covers the payload exactly as it was serialised, so a relay
	// re-marshalling the same struct must arrive at the same MAC.
	var signature string
	if err := json.Unmarshal(raw["signature"], &signature); err != nil {
		t.Fatalf("signature is not a string: %v", err)
	}
	signed, err := json.Marshal(eagleTokenPayload{
		ExpiresAt: "1700000000",
		ServerEnv: "lp1",
		ServerID:  "1234",
		UserID:    "000000006b49d22a",
	})
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(signed)
	if got := base64.StdEncoding.EncodeToString(mac.Sum(nil)); got != signature {
		t.Errorf("signature = %q, want %q", signature, got)
	}
}

// A different player in the same lobby must get a different token, or one leaked token
// would let anyone join any match as anyone.
func TestEagleTokenIsPerPlayer(t *testing.T) {
	key := []byte("0123456789abcdef")
	expires := time.Unix(1700000000, 0)

	a, err := buildEagleToken(key, 1800000042, 7, expires)
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildEagleToken(key, 1800000043, 7, expires)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two players in one gathering were handed the same token")
	}
}

func TestEagleURL(t *testing.T) {
	if got := eagleURL("wss://eagle.example:22138", 42); got != "wss://eagle.example:22138/42" {
		t.Errorf("eagleURL = %q", got)
	}
}

func TestRoundStore(t *testing.T) {
	store := newRoundStore()

	first := store.start(round{gid: 1, pids: []uint64{10, 11}, params: []byte("body")})
	second := store.start(round{gid: 2})
	if first == second {
		t.Fatalf("round IDs repeated: %d", first)
	}

	rd, ok := store.get(first)
	if !ok {
		t.Fatal("round not found after start")
	}
	if string(rd.params) != "body" {
		t.Errorf("params = %q, want the bytes StartRound was called with", rd.params)
	}

	if !store.end(first) {
		t.Error("end reported an unknown round")
	}
	if _, ok := store.get(first); ok {
		t.Error("round survived end")
	}
	if store.end(first) {
		t.Error("end reported the same round twice")
	}

	// A round abandoned without an EndRound is dropped on the next start rather than kept
	// for the life of the process.
	store.rounds[second] = round{started: time.Now().Add(-2 * roundLifetime)}
	store.start(round{gid: 3})
	if _, ok := store.get(second); ok {
		t.Error("an abandoned round was not pruned")
	}
}

func TestIntegerSettings(t *testing.T) {
	table, err := integerSettings(nil, types.NewPID(0), 0)
	if err != nil {
		t.Fatalf("index 0: %v", err)
	}
	if len(table) != 17 {
		t.Errorf("index 0 has %d entries, want 17", len(table))
	}
	if table[15] != 180 {
		t.Errorf("index 0 entry 15 = %d, want 180", table[15])
	}

	table, err = integerSettings(nil, types.NewPID(0), 10)
	if err != nil {
		t.Fatalf("index 10: %v", err)
	}
	if len(table) != 0 {
		t.Errorf("index 10 has %d entries, want none", len(table))
	}

	if _, err := integerSettings(nil, types.NewPID(0), 1); err == nil {
		t.Error("an unknown settings index was answered instead of refused")
	}
}

// FromEnv must refuse a relay address with no signing key: that combination hands every
// player a token the relay is obliged to reject, which reads to a player as the game
// hanging rather than as a misconfiguration.
func TestFromEnvRefusesEagleURLWithoutKey(t *testing.T) {
	t.Setenv("SMB35_DATABASE_URL", "postgres://localhost/x")
	t.Setenv("SMB35_EAGLE_URL", "wss://eagle.example:22138")
	t.Setenv("SMB35_EAGLE_KEY", "")

	if _, err := FromEnv(); err == nil {
		t.Error("FromEnv accepted an Eagle URL with no signing key")
	}

	t.Setenv("SMB35_EAGLE_KEY", "000102030405060708090a0b0c0d0e0f")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if len(cfg.EagleKey) != 16 {
		t.Errorf("EagleKey is %d bytes, want 16", len(cfg.EagleKey))
	}
	if cfg.EagleURL != "wss://eagle.example:22138" {
		t.Errorf("EagleURL = %q", cfg.EagleURL)
	}
}

func TestFromEnvRequiresDatabase(t *testing.T) {
	t.Setenv("SMB35_DATABASE_URL", "")
	if _, err := FromEnv(); err == nil {
		t.Error("FromEnv accepted a missing SMB35_DATABASE_URL")
	}
}
