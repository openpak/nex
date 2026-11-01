package datastore_smm_db

import (
	"strings"
	"testing"
)

func intp(v int) *int       { return &v }
func int64p(v int64) *int64 { return &v }

func TestFailureRate(t *testing.T) {
	cases := []struct {
		clears, attempts int64
		want             int
	}{
		{0, 0, -1},
		{0, 10, 100},
		{10, 10, 0},
		{1, 3, 67}, // 33.3% clear
		{1, 2, 50}, // 50% clear
		{1, 8, 87}, // 12.5% clear rounds up to 13
		{5, 3, 0},  // more clears than attempts cannot go below 0
		{-1, 10, 100},
	}
	for _, c := range cases {
		if got := FailureRate(c.clears, c.attempts); got != c.want {
			t.Errorf("FailureRate(%d,%d) = %d, want %d", c.clears, c.attempts, got, c.want)
		}
	}
}

func TestBuildCourseQueryDefaultsOnlyServeCourses(t *testing.T) {
	q, args := BuildCourseQuery(CourseFilter{Limit: 25})
	for _, want := range []string{
		"object.upload_completed = TRUE", "object.deleted = FALSE", "object.under_review = FALSE",
		"object.data_type > 2 AND object.data_type < 50",
		"LEFT JOIN datastore.object_custom_rankings",
		"COALESCE(ranking.value, 0) DESC",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %q", want)
		}
	}
	if strings.Contains(q, "object_ratings") {
		t.Error("no difficulty filter, so no ratings join")
	}
	if len(args) != 3 || args[1] != 25 {
		t.Errorf("args = %v", args)
	}
}

func TestBuildCourseQueryDifficulty(t *testing.T) {
	q, args := BuildCourseQuery(CourseFilter{Random: true, Limit: 10, FailMin: intp(35), FailMax: intp(74)})
	if !strings.Contains(q, "clears.slot = 2") || !strings.Contains(q, "attempts.slot = 3") {
		t.Error("clears and attempts must come from rating slots 2 and 3")
	}
	if !strings.Contains(q, "COALESCE(attempts.total_value, 0) > 0") {
		t.Error("unattempted courses must be excluded from a difficulty band")
	}
	if !strings.Contains(q, "ORDER BY RANDOM()") {
		t.Error("expected random order")
	}
	if len(args) != 5 || args[3] != 35 || args[4] != 74 {
		t.Errorf("args = %v", args)
	}
	if !strings.Contains(q, ">= $4") || !strings.Contains(q, "<= $5") {
		t.Errorf("placeholders out of order:\n%s", q)
	}
}

func TestBuildCourseQueryMakersScoreAndName(t *testing.T) {
	q, args := BuildCourseQuery(CourseFilter{AppID: 400000000, Makers: true, ScoreMin: int64p(1), ScoreMax: int64p(49), Name: "100%_x", Newest: true, Limit: 5, Offset: 10})
	if !strings.Contains(q, "object.data_type = 1") || strings.Contains(q, "data_type > 2") {
		t.Error("maker ranking must select maker objects only")
	}
	if !strings.Contains(q, "ORDER BY object.creation_date DESC") {
		t.Error("expected newest order")
	}
	if args[0] != uint32(400000000) || args[2] != 10 {
		t.Errorf("args = %v", args)
	}
	if got := args[len(args)-1]; got != `100\%\_x` {
		t.Errorf("name must be escaped for LIKE, got %v", got)
	}
	if !IsMakerRanking(400000000) || IsMakerRanking(300000000) || IsMakerRanking(0) {
		t.Error("IsMakerRanking")
	}
}
