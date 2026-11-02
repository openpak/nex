package nexhost

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/openpak/nex/internal/rankingsapi"
)

// Set NEX_TEST_DSN to a scratch Postgres to run the SQL paths; the test creates and drops the
// title tables it reads (rankings, ranking.*, mk7.ranking_scores).
func rankingsDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("NEX_TEST_DSN")
	if dsn == "" {
		t.Skip("NEX_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, src rankingsapi.Source, path string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rankingsapi.Handler(RankingsNetwork, src).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code
}

func TestLegacyRankings(t *testing.T) {
	db := rankingsDB(t)
	mustExec(t, db, `DROP TABLE IF EXISTS rankings`)
	mustExec(t, db, `CREATE TABLE rankings (owner_pid integer, unique_id bigint, category integer, score integer, created_at bigint)`)
	for i, s := range []int{50, 90, 70, 90, 10} {
		mustExec(t, db, `INSERT INTO rankings VALUES ($1, 0, 3, $2, $3)`, 1800000000+i, s, int64(1_700_000_000_000_000_000+i))
	}
	mustExec(t, db, `INSERT INTO rankings VALUES (1800000009, 0, 4, 5, 1700000000000000000)`)
	src := LegacyRankings(db, nil)

	var bl rankingsapi.BoardList
	if code := get(t, src, "/internal/rankings/boards", &bl); code != 200 || bl.Network != "wiiu" || len(bl.Boards) != 2 {
		t.Fatalf("boards: %d %+v", code, bl)
	}
	if b := bl.Boards[0]; b.ID != "3" || b.Name != "Board 3" || b.Order != "desc" || b.Kind != "score" {
		t.Fatalf("board: %+v", b)
	}

	var p rankingsapi.Page
	if code := get(t, src, "/internal/rankings/boards/3?limit=2&offset=1", &p); code != 200 {
		t.Fatal(code)
	}
	// 90 (older), 90, 70, 50, 10: the window is places 2 and 3.
	if p.Total != 5 || len(p.Rows) != 2 || p.Rows[0].Rank != 2 || p.Rows[0].PID != "1800000003" || p.Rows[0].Score != 90 || p.Rows[1].Score != 70 || p.Rows[0].Display != "90" {
		t.Fatalf("page: %+v", p)
	}
	if p.Rows[0].UpdatedAt.Year() != 2023 {
		t.Fatalf("updated_at: %v", p.Rows[0].UpdatedAt)
	}
	p = rankingsapi.Page{}
	get(t, src, "/internal/rankings/boards/3?limit=1&around=1800000004", &p)
	if len(p.Around) != 5 || p.Around[4].PID != "1800000004" || p.Around[4].Rank != 5 || p.Around[0].Rank != 1 {
		t.Fatalf("around: %+v", p.Around)
	}
	if code := get(t, src, "/internal/rankings/boards/99", &p); code != 404 {
		t.Fatalf("missing board: %d", code)
	}
	if code := get(t, src, "/internal/rankings/boards/abc", &p); code != 404 {
		t.Fatalf("bad board: %d", code)
	}
}

func TestScoresRankingsOrdersByCategory(t *testing.T) {
	db := rankingsDB(t)
	mustExec(t, db, `DROP SCHEMA IF EXISTS ranking CASCADE`)
	mustExec(t, db, `CREATE SCHEMA ranking`)
	mustExec(t, db, `CREATE TABLE ranking.categories (category int PRIMARY KEY, golf_scoring boolean, creation_date timestamp)`)
	mustExec(t, db, `CREATE TABLE ranking.scores (deleted boolean NOT NULL DEFAULT FALSE, unique_id bigint, owner_pid bigint, category int, score int, update_date timestamp)`)
	mustExec(t, db, `INSERT INTO ranking.categories VALUES (1, true, now()), (2, false, now())`)
	for _, c := range []int{1, 2} {
		for i, s := range []int{30, 10, 20} {
			mustExec(t, db, `INSERT INTO ranking.scores VALUES (false, 0, $1, $2, $3, now())`, 100+i, c, s)
		}
	}
	mustExec(t, db, `INSERT INTO ranking.scores VALUES (true, 0, 999, 1, 5000, now())`)
	src := ScoresRankings(db, nil)
	boards, err := src.Boards(context.Background())
	if err != nil || len(boards) != 2 || boards[0].Order != "desc" || boards[1].Order != "asc" {
		t.Fatalf("boards: %+v %v", boards, err)
	}
	for id, want := range map[string]int64{"1": 30, "2": 10} {
		p, err := src.Rows(context.Background(), id, rankingsapi.Query{Limit: 10})
		if err != nil || p.Total != 3 || p.Rows[0].Score != want || p.Rows[0].Rank != 1 {
			t.Fatalf("board %s: %+v %v", id, p, err)
		}
	}
}

func TestTimeTrialRankings(t *testing.T) {
	db := rankingsDB(t)
	mustExec(t, db, `CREATE SCHEMA IF NOT EXISTS mk7`)
	mustExec(t, db, `DROP TABLE IF EXISTS mk7.ranking_scores`)
	mustExec(t, db, `CREATE TABLE mk7.ranking_scores (category bigint, pid numeric(20), scores bigint[], updated_at timestamptz DEFAULT now())`)
	mustExec(t, db, `INSERT INTO mk7.ranking_scores (category, pid, scores) VALUES (7, 5, '{91234,1}'), (7, 6, '{80500,2}'), (7, 7, '{99999,3}')`)
	src := TimeTrialRankings(db, "mk7.ranking_scores", "scores[1]")
	p, err := src.Rows(context.Background(), "7", rankingsapi.Query{Limit: 10, Around: "7"})
	if err != nil || p.Total != 3 || p.Rows[0].PID != "6" || p.Rows[0].Display != "1:20.500" || p.Rows[2].PID != "7" || len(p.Around) != 3 {
		t.Fatalf("%+v %v", p, err)
	}
	boards, _ := src.Boards(context.Background())
	if len(boards) != 1 || boards[0].Kind != "time" || boards[0].Order != "asc" {
		t.Fatalf("%+v", boards)
	}
}
