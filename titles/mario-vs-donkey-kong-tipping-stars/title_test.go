package mvdk

import "testing"

func TestVersionAndConfig(t *testing.T) {
	v, err := parseVersion("3.7.1")
	if err != nil || v.Major != 3 || v.Minor != 7 || v.Patch != 1 {
		t.Fatalf("parseVersion: %v %v", v, err)
	}
	if _, err := parseVersion("3.7"); err == nil {
		t.Fatal("two-part version accepted")
	}
	h, p := hostPort("openpak-nn-account:20051")
	if h != "openpak-nn-account" || p != 20051 {
		t.Fatalf("hostPort: %s %d", h, p)
	}
	cfg := Title.Config()
	if cfg.Transport != "udp" || cfg.Version != "3.7.1" {
		t.Fatalf("defaults: %+v", cfg)
	}
}
