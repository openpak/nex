package mvdk

import (
	"strings"
	"testing"
)

// The login response must name the secure server with prudps, or the console sends its secure
// CONNECT without a Kerberos ticket (Wii U 106-0502).
func TestSecureStationURLIsPRUDPS(t *testing.T) {
	got := secureStationURL("203.0.113.5", 22851).String()
	if !strings.Contains(got, "\"prudps:/") || !strings.Contains(got, "port=22851") || !strings.Contains(got, "PID=2") {
		t.Fatalf("secure station URL = %q, want prudps:/ ... port=22851 ... PID=2", got)
	}
}
