//go:build integration

package api

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/PretendoNetwork/plogger-go"
	_ "github.com/lib/pq"
	"github.com/openpak/nex/titles/super-mario-maker/database"
	datastore_smm_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore/super-mario-maker"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
)

// Run with: SMM_TEST_PG='postgres://postgres:pw@127.0.0.1:55437/postgres?sslmode=disable' go test -tags integration ./api
func TestPostgres(t *testing.T) {
	dsn := os.Getenv("SMM_TEST_PG")
	if dsn == "" {
		t.Skip("SMM_TEST_PG not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`DROP SCHEMA IF EXISTS datastore CASCADE`, `CREATE SCHEMA datastore`,
		`CREATE TABLE datastore.objects (data_id bigint PRIMARY KEY, upload_completed boolean NOT NULL DEFAULT FALSE, deleted boolean NOT NULL DEFAULT FALSE,
			deletion_reason int NOT NULL DEFAULT 0, under_review boolean NOT NULL DEFAULT FALSE, owner int, size int, name text, data_type int, meta_binary bytea,
			permission int, permission_recipients int[], delete_permission int, delete_permission_recipients int[], flag int, period int, refer_data_id bigint,
			tags text[], persistence_slot_id int, extra_data text[], access_password bigint NOT NULL DEFAULT 0, update_password bigint NOT NULL DEFAULT 0,
			creation_date timestamp, update_date timestamp)`,
		`CREATE TABLE datastore.object_ratings (data_id bigint, slot smallint, flag smallint, internal_flag smallint, lock_type smallint, initial_value bigint,
			range_min int, range_max int, period_hour smallint, period_duration int, total_value bigint, count int NOT NULL DEFAULT 0, PRIMARY KEY(data_id, slot))`,
		`CREATE TABLE datastore.object_custom_rankings (data_id bigint, application_id bigint, value bigint, PRIMARY KEY(data_id, application_id))`,
		`CREATE TABLE datastore.bookmarks (maker_pid int NOT NULL, data_id bigint NOT NULL, created timestamp NOT NULL DEFAULT NOW(), PRIMARY KEY(maker_pid, data_id))`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	now := time.Now()
	obj := func(id int64, owner int, name string, typ int, done, deleted bool, refer int64) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO datastore.objects (data_id, owner, name, data_type, upload_completed, deleted, refer_data_id, size, permission, delete_permission,
			permission_recipients, delete_permission_recipients, flag, period, tags, meta_binary, creation_date, update_date)
			VALUES ($1,$2,$3,$4,$5,$6,$7,10,0,3,'{}','{}',0,0,'{"tag"}','\x00',$8,$8)`, id, owner, name, typ, done, deleted, refer, now.Add(time.Duration(id)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	rate := func(id int64, slot int, v int64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO datastore.object_ratings (data_id, slot, total_value, count, initial_value) VALUES ($1,$2,$3,1,0)`, id, slot, v); err != nil {
			t.Fatal(err)
		}
	}
	obj(1, 7, "mario_nnid", 1, true, false, 0)       // maker profile of pid 7
	obj(900000, 2, "", 50, true, false, 0)           // event metadata: never a course
	obj(943270, 7, "Easy_One", 3, true, false, 0)    // 9 of 10 clears: failure 10, easy
	obj(943271, 7, "Hard one", 3, true, false, 0)    // 1 of 100: failure 99, super expert
	obj(943272, 8, "Unplayed", 3, true, false, 0)    // no attempts
	obj(943273, 8, "Deleted", 3, true, true, 0)      // deleted
	obj(943274, 8, "Unfinished", 3, false, false, 0) // upload not completed
	obj(943275, 8, "Thumb", 2, true, false, 943270)  // thumbnail object of 943270
	rate(943270, 0, 10)
	rate(943270, 2, 9)
	rate(943270, 3, 10)
	rate(943270, 6, 2)
	rate(943271, 0, 100)
	rate(943271, 2, 1)
	rate(943271, 3, 100)
	for _, q := range []string{
		`INSERT INTO datastore.object_custom_rankings VALUES (943270, 0, 3), (943271, 0, 60)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	s := &PGStore{DB: db}

	list, total, err := s.Courses(ctx, CourseQuery{Sort: SortNewest, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(list) != 3 || list[0].DataID != 943272 {
		t.Fatalf("visible courses = %d %+v", total, list)
	}

	lo, hi := 75, 100
	list, total, err = s.Courses(ctx, CourseQuery{FailMin: &lo, FailMax: &hi, Limit: 10})
	if err != nil || total != 1 || list[0].DataID != 943271 {
		t.Fatalf("difficulty band: %v %d %+v", err, total, list)
	}
	e0, e1 := 0, 34
	list, _, err = s.Courses(ctx, CourseQuery{FailMin: &e0, FailMax: &e1, Limit: 10})
	if err != nil || len(list) != 1 || list[0].DataID != 943270 {
		t.Fatalf("easy band: %v %+v", err, list)
	}

	list, _, _ = s.Courses(ctx, CourseQuery{Name: "easy_one", Limit: 10})
	if len(list) != 1 {
		t.Fatalf("name search: %+v", list)
	}
	list, _, _ = s.Courses(ctx, CourseQuery{Name: "asy_o", Limit: 10})
	if len(list) != 1 {
		t.Fatalf("literal underscore: %+v", list)
	}
	list, _, _ = s.Courses(ctx, CourseQuery{Name: "asyXone", Limit: 10})
	if len(list) != 0 {
		t.Fatalf("underscore must not be a wildcard: %+v", list)
	}

	c, err := s.Course(ctx, 943270)
	if err != nil {
		t.Fatal(err)
	}
	if c.Stars != 3 || c.Plays != 10 || c.Clears != 9 || c.Attempts != 10 || c.Comments != 2 || !c.HasThumbnail || c.MakerName != "mario_nnid" || c.OwnerPID != 7 {
		t.Fatalf("course = %+v", c)
	}
	if _, err := s.Course(ctx, 900000); err != ErrNotFound {
		t.Fatalf("the event metadata object is not a course: %v", err)
	}
	if _, err := s.Course(ctx, 943273); err != ErrNotFound {
		t.Fatalf("deleted: %v", err)
	}

	makers, mt, err := s.Makers(ctx, 10, 0)
	if err != nil || mt != 2 || makers[0].PID != 7 || makers[0].Stars != 63 || makers[0].Courses != 2 || makers[0].Name != "mario_nnid" {
		t.Fatalf("makers: %v %d %+v", err, mt, makers)
	}

	ids, err := s.Pickup(ctx, 96, 100, 400)
	if err != nil || len(ids) != 1 || ids[0] != 943271 {
		t.Fatalf("pickup: %v %v", err, ids)
	}

	pid, err := s.ResolveNNID(ctx, "MARIO_NNID")
	if err != nil || pid != 7 {
		t.Fatalf("nnid: %v %d", err, pid)
	}
	if _, err := s.ResolveNNID(ctx, "nobody"); err != ErrNotFound {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := s.AddBookmark(ctx, 7, 943271); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.AddBookmark(ctx, 7, 943270)
	b, err := s.Bookmarks(ctx, 7)
	if err != nil || len(b) != 2 {
		t.Fatalf("bookmarks: %v %v", err, b)
	}
	_ = s.RemoveBookmark(ctx, 7, 943271)
	if b, _ = s.Bookmarks(ctx, 7); len(b) != 1 || b[0] != 943270 {
		t.Fatalf("after remove: %v", b)
	}

	// * The NEX-side query over the same rows
	database.Postgres = db
	globals.Logger = plogger.NewLogger()
	hardLo, hardHi := 96, 100
	res, nerr := datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{Random: true, Limit: 10, FailMin: &hardLo, FailMax: &hardHi})
	if nerr != nil || len(res) != 1 || res[0].MetaInfo.DataID != 943271 || res[0].Score != 60 {
		t.Fatalf("QueryCourses difficulty: %v %+v", nerr, res)
	}
	res, nerr = datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{Newest: true, Limit: 10})
	if nerr != nil || len(res) != 3 {
		t.Fatalf("QueryCourses newest: %v %d", nerr, len(res))
	}
	res, nerr = datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{Limit: 10})
	if nerr != nil || len(res) != 3 || res[0].MetaInfo.DataID != 943271 || res[0].Order != 1 {
		t.Fatalf("QueryCourses by stars: %v %+v", nerr, res)
	}
	one, nerr := datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{Makers: true, Limit: 10})
	if nerr != nil || len(one) != 1 || one[0].MetaInfo.DataID != 1 {
		t.Fatalf("makers listing: %v %+v", nerr, one)
	}
	mn, mx := int64(50), int64(70)
	res, nerr = datastore_smm_db.QueryCourses(datastore_smm_db.CourseFilter{AppID: 0, ScoreMin: &mn, ScoreMax: &mx, Random: true, Limit: 10})
	if nerr != nil || len(res) != 1 || res[0].MetaInfo.DataID != 943271 {
		t.Fatalf("score range: %v %+v", nerr, res)
	}
	if prev, ok := datastore_smm_db.PreviousRankingPeriod(100000005); ok {
		t.Fatalf("no earlier period expected, got %d", prev)
	}
}
