package api

import "testing"

// The worked example of the NintendoClients wiki: level 77A9-0000-003D-1D4F is data ID 4005199.
func TestCourseCodeWikiExample(t *testing.T) {
	if got := CourseCode(0x3D1D4F); got != "77A9-0000-003D-1D4F" {
		t.Fatalf("CourseCode = %s", got)
	}
	id, ok := ParseCourseCode("77a9-0000-003d-1d4f")
	if !ok || id != 4005199 {
		t.Fatalf("parse = %d %v", id, ok)
	}
	if id, ok := ParseCourseCode("77A900003D1D4F00"[:16]); ok {
		t.Fatalf("a wrong checksum parsed as %d", id)
	}
}

// Codes taken from Pretendo issue 25 (smm.pretendo.zip course pages): code and data ID.
func TestCourseCodeAgainstKnownCourses(t *testing.T) {
	for code, id := range map[string]uint64{
		"4348-0000-000E-64A6": 943270,
		"3BC1-0000-000E-80DB": 950491,
		"404B-0000-000E-9704": 956164,
	} {
		if got := CourseCode(id); got != code {
			t.Errorf("CourseCode(%d) = %s, want %s", id, got, code)
		}
	}
}

func TestParseCourseCodeForms(t *testing.T) {
	if id, ok := ParseCourseCode("943270"); !ok || id != 943270 {
		t.Errorf("decimal: %d %v", id, ok)
	}
	if id, ok := ParseCourseCode("43480000000E64A6"); !ok || id != 943270 {
		t.Errorf("no dashes: %d %v", id, ok)
	}
	for _, bad := range []string{"", "0", "abc", "4348-0000-000E-64A7", "4348-0000-000E", "99999999999999999999"} {
		if _, ok := ParseCourseCode(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}
