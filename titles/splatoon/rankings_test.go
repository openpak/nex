package splatoonwiiu

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/openpak/nex/internal/rankingsapi"
)

// Set SPLATOON_TEST_DSN to a scratch Postgres to run the SQL paths.
func TestFestRankings(t *testing.T) {
	dsn := os.Getenv("SPLATOON_TEST_DSN")
	if dsn == "" {
		t.Skip("SPLATOON_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`DROP SCHEMA IF EXISTS ranking_splatoon CASCADE`,
		`CREATE SCHEMA ranking_splatoon`,
		`CREATE TABLE ranking_splatoon.user_scores (uploader_pid bigint, splatfest_id bigint, team_id smallint, score bigint, last_updated timestamptz DEFAULT now())`,
		`INSERT INTO ranking_splatoon.user_scores (uploader_pid, splatfest_id, team_id, score) VALUES (1,4,0,500),(2,4,1,900),(3,5,0,10)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	src := festRankings(db)
	boards, err := src.Boards(context.Background())
	if err != nil || len(boards) != 2 || boards[0].Name != "Splatfest 4" || boards[0].Order != "desc" {
		t.Fatalf("boards: %+v %v", boards, err)
	}
	p, err := src.Rows(context.Background(), "4", rankingsapi.Query{Limit: 5})
	if err != nil || p.Total != 2 || p.Rows[0].PID != "2" || p.Rows[0].Score != 900 || p.Rows[1].Rank != 2 {
		t.Fatalf("rows: %+v %v", p, err)
	}
}
