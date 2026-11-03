package scoreboard

import (
	"database/sql"
	"os"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	rankingtypes "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	_ "github.com/lib/pq"
)

// Set NEX_TEST_DSN to a scratch Postgres; the test drops and recreates schema ranking.
func testStore(t *testing.T) *Store {
	dsn := os.Getenv("NEX_TEST_DSN")
	if dsn == "" {
		t.Skip("NEX_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS ranking CASCADE`); err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: db, Friends: func(pid uint32) []uint32 {
		if pid == 100 {
			return []uint32{101}
		}
		return nil
	}}
	if err := s.InitTables(); err != nil {
		t.Fatal(err)
	}
	return s
}

func put(t *testing.T, s *Store, pid uint64, cat uint32, score uint32, orderBy uint8, uid uint64) {
	t.Helper()
	d := rankingtypes.NewRankingScoreData()
	d.Category, d.Score, d.OrderBy = types.UInt32(cat), types.UInt32(score), 0
	if orderBy == 1 {
		d.OrderBy = 1
	}
	if err := s.Insert(types.NewPID(pid), d, types.UInt64(uid)); err != nil {
		t.Fatal(err)
	}
}

func order(length uint32) rankingtypes.RankingOrderParam {
	o := rankingtypes.NewRankingOrderParam()
	o.Length = types.UInt8(length)
	return o
}

func TestStoreRoundTrip(t *testing.T) {
	s := testStore(t)
	// Category 1 ranks higher first (OrderBy 1), category 2 lower first.
	for i, sc := range []uint32{30, 10, 20} {
		put(t, s, uint64(100+i), 1, sc, 1, uint64(i))
		put(t, s, uint64(100+i), 2, sc, 0, uint64(i))
	}
	g, total, err := s.Global(1, order(10))
	if err != nil || total != 3 || len(g) != 3 || g[0].Score != 30 || g[0].Order != 1 || g[2].Score != 10 {
		t.Fatalf("category 1: %v %d %v", g, total, err)
	}
	g, _, _ = s.Global(2, order(10))
	if len(g) != 3 || g[0].Score != 10 || g[2].Score != 30 {
		t.Fatalf("category 2 lower first: %v", g)
	}
	own, total, err := s.Own(101, 1, order(10))
	if err != nil || total != 1 || len(own) != 1 || own[0].Score != 10 || own[0].Order != 3 {
		t.Fatalf("own: %v %d %v", own, total, err)
	}
	near, _, err := s.Nearby(101, 1, order(2))
	if err != nil || len(near) == 0 {
		t.Fatalf("nearby: %v %v", near, err)
	}
	fr, total, err := s.FriendsRange(100, 1, order(10))
	if err != nil || total != 2 || len(fr) != 2 {
		t.Fatalf("friends of 100 (101 and self): %v %d %v", fr, total, err)
	}
	// An upload for the same unique id replaces the score instead of adding a row.
	put(t, s, 100, 1, 99, 1, 0)
	g, _, _ = s.Global(1, order(10))
	if len(g) != 3 || g[0].Score != 99 {
		t.Fatalf("replace: %v", g)
	}
}

func TestCommonData(t *testing.T) {
	s := testStore(t)
	put(t, s, 100, 1, 5, 1, 7)
	if err := s.UploadCommonData(types.NewPID(100), 7, types.Buffer("blob")); err != nil {
		t.Fatal(err)
	}
	if b, err := s.GetCommonData(7); err != nil || string(b) != "blob" {
		t.Fatalf("common data: %q %v", b, err)
	}
	if b, err := s.GetCommonData(8); err != nil || len(b) != 0 {
		t.Fatalf("unknown id: %q %v", b, err)
	}
	g, _, _ := s.Global(1, order(1))
	if len(g) != 1 || string(g[0].CommonData) != "blob" {
		t.Fatalf("common data joins the board: %v", g)
	}
}

func TestQueriesBuild(t *testing.T) {
	for m := modeGlobal; m <= modeOwn; m++ {
		if queries[m] == "" {
			t.Fatalf("mode %d has no query", m)
		}
	}
}
