package board

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// Set PIKMIN3_TEST_DSN to a scratch Postgres to run the SQL paths.
func testStore(t *testing.T) *Store {
	dsn := os.Getenv("PIKMIN3_TEST_DSN")
	if dsn == "" {
		t.Skip("PIKMIN3_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS rankings, ranking_common_data`); err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: db, Friends: func(pid uint32) []uint32 {
		if pid == 1 {
			return []uint32{2}
		}
		return nil
	}}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil { // idempotent
		t.Fatal(err)
	}
	return s
}

func TestStoreUploadAndQueries(t *testing.T) {
	s := testStore(t)
	up := func(pid, score uint32, ob, mode uint8) {
		t.Helper()
		if err := s.Upload(Entry{PID: pid, Category: 7, Score: score, OrderBy: ob}, mode); err != nil {
			t.Fatal(err)
		}
	}
	up(1, 100, Descending, UpdateNormal)
	up(1, 50, Descending, UpdateNormal) // worse: ignored
	up(2, 300, Descending, UpdateNormal)
	up(3, 200, Descending, UpdateNormal)
	up(3, 10, Descending, UpdateDeleteOld) // forced overwrite

	rows, total, err := s.Run(Query{Mode: "range", Category: 7, Calc: Calc123, GroupIndex: 255, Length: 10})
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("range: %v total=%d rows=%d", err, total, len(rows))
	}
	if rows[0].PID != 2 || rows[1].PID != 1 || rows[1].Score != 100 || rows[2].Score != 10 {
		t.Errorf("range order %+v", rows)
	}
	rows, _, _ = s.Run(Query{Mode: "user", Category: 7, PID: 1, GroupIndex: 255})
	if len(rows) != 1 || rows[0].Order != 2 {
		t.Errorf("own rank %+v", rows)
	}
	rows, total, _ = s.Run(Query{Mode: "friend-range", Category: 7, PID: 1, GroupIndex: 255, Length: 10})
	if total != 2 || len(rows) != 2 || rows[0].PID != 2 {
		t.Errorf("friends %+v total=%d", rows, total)
	}
	rows, _, _ = s.Run(Query{Mode: "near", Category: 7, PID: 3, GroupIndex: 255, Length: 2})
	if len(rows) != 2 || rows[1].PID != 3 {
		t.Errorf("near %+v", rows)
	}
	cats, _ := s.Categories()
	if len(cats) != 1 || cats[0] != 7 {
		t.Errorf("categories %v", cats)
	}
}

func TestStoreCommonData(t *testing.T) {
	s := testStore(t)
	if _, err := s.CommonData(5); err == nil {
		t.Error("missing blob must error")
	}
	if err := s.PutCommonData(1, 5, []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutCommonData(1, 5, []byte{3}); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.CommonData(5); len(b) != 1 || b[0] != 3 {
		t.Errorf("blob %v", b)
	}
	if err := s.Upload(Entry{PID: 1, Category: 1, Score: 1, UniqueID: 5}, UpdateNormal); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.Run(Query{Mode: "user", Category: 1, PID: 1, GroupIndex: 255})
	if len(rows) != 1 || len(rows[0].Common) != 1 {
		t.Errorf("common data not joined: %+v", rows)
	}
}
