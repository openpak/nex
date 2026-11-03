package smmwiiu

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/openpak/nex/internal/rankingsapi"
)

// Set NEX_TEST_DSN to a scratch Postgres; the test recreates schema datastore.
func TestMakerRankings(t *testing.T) {
	dsn := os.Getenv("NEX_TEST_DSN")
	if dsn == "" {
		t.Skip("NEX_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`DROP SCHEMA IF EXISTS datastore CASCADE`, `CREATE SCHEMA datastore`,
		`CREATE TABLE datastore.objects (data_id bigint, owner int, data_type int, upload_completed boolean, deleted boolean, under_review boolean, update_date timestamp)`,
		`CREATE TABLE datastore.object_custom_rankings (data_id bigint, application_id bigint, value bigint, PRIMARY KEY(data_id, application_id))`,
		`CREATE TABLE datastore.course_records (data_id bigint, slot int, first_pid int, best_pid int, best_score int, creation_date timestamp, update_date timestamp, PRIMARY KEY(data_id, slot))`,
		// courses 10, 11 by maker 1; course 12 by maker 2; 13 is deleted; 14 is a maker object
		`INSERT INTO datastore.objects VALUES (10,1,3,true,false,false,now()),(11,1,3,true,false,false,now()),(12,2,3,true,false,false,now()),(13,2,3,true,true,false,now()),(14,1,1,true,false,false,now())`,
		`INSERT INTO datastore.object_custom_rankings VALUES (10,0,5),(11,0,7),(12,0,20),(13,0,900),(10,400000001,3)`,
		`INSERT INTO datastore.course_records VALUES (10,0,1,2,41000,now(),now()),(12,0,2,1,38000,now(),now()),(13,0,2,2,1,now(),now())`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	src := rankings(db)
	ctx := context.Background()
	boards, err := src.Boards(ctx)
	if err != nil || len(boards) != 3 || boards[0].Name != "Course stars" || boards[1].Name != "Maker stars" || boards[2].Name != "Fastest clears" || boards[2].Kind != "time" || boards[2].Order != "asc" || boards[0].Order != "desc" {
		t.Fatalf("boards: %+v %v", boards, err)
	}
	p, err := src.Rows(ctx, "1", rankingsapi.Query{Limit: 10})
	if err != nil || p.Total != 3 || p.Rows[0].Score != 20 || p.Rows[0].PID != "2" || p.Rows[2].Score != 5 {
		t.Fatalf("course stars: %+v %v", p, err)
	}
	p, err = src.Rows(ctx, "2", rankingsapi.Query{Limit: 10, Around: "1"})
	if err != nil || p.Total != 2 || p.Rows[0].PID != "2" || p.Rows[0].Score != 20 || p.Rows[1].PID != "1" || p.Rows[1].Score != 12 || len(p.Around) == 0 {
		t.Fatalf("maker stars: %+v %v", p, err)
	}
	p, err = src.Rows(ctx, "3", rankingsapi.Query{Limit: 10})
	if err != nil || p.Total != 2 || p.Rows[0].PID != "1" || p.Rows[0].Score != 38000 || p.Rows[1].Score != 41000 {
		t.Fatalf("fastest clears: %+v %v", p, err)
	}
	if _, err := src.Rows(ctx, "9", rankingsapi.Query{Limit: 10}); err != rankingsapi.ErrNoBoard {
		t.Fatalf("unknown board: %v", err)
	}
}
