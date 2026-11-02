package pikmin3

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/openpak/nex/internal/rankingsapi"
	"github.com/openpak/nex/titles/pikmin-3/board"
)

// Set PIKMIN3_TEST_DSN to a scratch Postgres to run the SQL paths.
func TestRankingsSource(t *testing.T) {
	dsn := os.Getenv("PIKMIN3_TEST_DSN")
	if dsn == "" {
		t.Skip("PIKMIN3_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE IF EXISTS rankings, ranking_common_data`); err != nil {
		t.Fatal(err)
	}
	s := &board.Store{DB: db}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	// Category 12 ranks lower first (OrderBy 0), category 13 higher first.
	for pid, score := range map[uint32]uint32{10: 300, 11: 100, 12: 200} {
		if err := s.Upload(board.Entry{PID: pid, Category: 12, Score: score, OrderBy: board.Ascending}, board.UpdateNormal); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Upload(board.Entry{PID: 10, Category: 13, Score: 5, OrderBy: board.Descending}, board.UpdateNormal); err != nil {
		t.Fatal(err)
	}
	src := Rankings{Store: s}
	boards, err := src.Boards(context.Background())
	if err != nil || len(boards) != 2 || boards[0].ID != "12" || boards[0].Order != "asc" || boards[1].Order != "desc" || boards[0].Name != "Board 12" {
		t.Fatalf("boards: %+v %v", boards, err)
	}
	p, err := src.Rows(context.Background(), "12", rankingsapi.Query{Limit: 2, Offset: 0, Around: "12"})
	if err != nil || p.Total != 3 || len(p.Rows) != 2 || p.Rows[0].PID != "11" || p.Rows[0].Score != 100 || p.Rows[1].PID != "12" || len(p.Around) != 3 {
		t.Fatalf("rows: %+v %v", p, err)
	}
	if _, err := src.Rows(context.Background(), "99", rankingsapi.Query{Limit: 5}); err != rankingsapi.ErrNoBoard {
		t.Fatalf("missing: %v", err)
	}
}
