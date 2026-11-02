package nexhost

// registration_test.go validates registration.sql, the staged nn-account
// `servers` rows (platform-wiiu-prd WU-5). The file applies on the production
// box by hand, so this test is the gate that catches a bad row before the
// deploy pass: an unbindable block (host.go validate would refuse the
// container at startup), a game-server id that is not the low 32 bits of its
// title id, or two rows fighting over one id or one port.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// rowTuple is one uncommented VALUES tuple.
type rowTuple struct {
	gameServerID string
	clientID     string
	titleIDs     []string
	port         int
	line         int
}

var (
	tupleRe = regexp.MustCompile(`^\(\s*'([0-9A-Fa-f]+)'\s*,\s*'([a-z]+)'\s*,\s*(\d+)\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([A-Z]+)'\s*,\s*(ARRAY\[[^\]]*\]|'\{\}')\s*,\s*:'host'\s*,\s*(\d+)\s*,\s*:'aes_key'\s*\)\s*,?\s*$`)
	titleRe = regexp.MustCompile(`'([0-9A-Fa-f]{16})'`)
)

func parseRegistration(t *testing.T) []rowTuple {
	t.Helper()
	raw, err := os.ReadFile("registration.sql")
	if err != nil {
		t.Fatalf("read registration.sql: %v", err)
	}
	var rows []rowTuple
	for i, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		m := tupleRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue // INSERT/ON CONFLICT boilerplate, not a tuple
		}
		port, err := strconv.Atoi(m[8])
		if err != nil {
			t.Fatalf("line %d: bad port %q", i+1, m[8])
		}
		rows = append(rows, rowTuple{gameServerID: m[1], clientID: m[4], titleIDs: pickGroup(titleRe.FindAllStringSubmatch(m[7], -1), 1), port: port, line: i + 1})
	}
	if len(rows) == 0 {
		t.Fatal("no VALUES tuples parsed from registration.sql")
	}
	return rows
}

// pickGroup flattens FindAllStringSubmatch to one capture group per match.
func pickGroup(ms [][]string, group int) []string {
	var out []string
	for _, m := range ms {
		if group < len(m) {
			out = append(out, m[group])
		}
	}
	return out
}

func validBlockBase(port int) bool {
	inRange := (port >= 22100 && port <= 22190) || (port >= 22300 && port <= 22590)
	return inRange && port%10 == 0
}

func TestRegistrationBlocksAreBindable(t *testing.T) {
	for _, r := range parseRegistration(t) {
		if !validBlockBase(r.port) {
			t.Errorf("line %d: block %d is not bindable: host.go validate wants a multiple of ten in 22100-22199 or 22300-22599 (gsid %s)", r.line, r.port, r.gameServerID)
		}
	}
}

func TestRegistrationLow32Derivation(t *testing.T) {
	for _, r := range parseRegistration(t) {
		if r.gameServerID == "00003200" {
			continue // friends server: fixed id, per-console, not title-derived
		}
		if len(r.titleIDs) == 0 {
			t.Errorf("line %d: gsid %s has no title ids to derive from", r.line, r.gameServerID)
			continue
		}
		for _, tid := range r.titleIDs {
			if want := strings.ToUpper(tid[len(tid)-8:]); !strings.EqualFold(r.gameServerID, want) {
				t.Errorf("line %d: gsid %s is not the low 32 bits of title %s (want %s)", r.line, r.gameServerID, tid, want)
			}
		}
	}
}

func TestRegistrationNoCollisions(t *testing.T) {
	seenID := map[string]int{}
	seenPort := map[int]string{} // port -> client_id (region rows of one title share a block)
	for _, r := range parseRegistration(t) {
		key := r.gameServerID + "/prod"
		if prev, ok := seenID[key]; ok {
			t.Errorf("duplicate game_server_id %s on lines %d and %d (second apply would overwrite the first)", r.gameServerID, prev, r.line)
		}
		seenID[key] = r.line
		if prev, ok := seenPort[r.port]; ok && prev != r.clientID {
			t.Errorf("line %d: port %d is shared by %s and %s (one NEX block per title container)", r.line, r.port, prev, r.clientID)
		}
		seenPort[r.port] = r.clientID
	}
}

func TestRegistrationFriendsRowPresent(t *testing.T) {
	for _, r := range parseRegistration(t) {
		if r.gameServerID == "00003200" {
			if r.port != 22360 {
				t.Errorf("friends row port is %d, want 22360 (platform PRD: nn-friends NEX UDP 22360/22361)", r.port)
			}
			return
		}
	}
	t.Error("no friends row (game_server_id 00003200) staged: friend sessions fail nex_token with 1021")
}
