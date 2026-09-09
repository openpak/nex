package main

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_constants "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	ranking_constants "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/constants"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	_ "github.com/lib/pq"
)

// The queries in datastore.go and ranking.go are strings until a database parses
// them. Point SMM2_TEST_DATABASE_URL at a throwaway Postgres to have them parsed:
//
//	podman run --rm -d -e POSTGRES_PASSWORD=check -p 23099:5432 postgres:16-alpine
//	SMM2_TEST_DATABASE_URL='postgres://postgres:check@127.0.0.1:23099/postgres?sslmode=disable' go test ./...
//
// Without it there is nothing to talk to and the database-backed tests skip.
func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("SMM2_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SMM2_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := createTables(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// A course posted into a persistence slot replaces the slot's previous occupant: this
// is how a maker's "my courses" slots behave, and it is what the base object store owes
// SMM2. Two posts into the same slot must leave exactly one live object in it.
func TestPersistenceSlotHoldsOnlyTheLatestCourse(t *testing.T) {
	db := testDatabase(t)
	store := objectStore{db: db}
	owner := types.NewPID(1800000009)
	if _, err := db.Exec(`DELETE FROM datastore.objects WHERE owner_pid = $1`, owner); err != nil {
		t.Fatal(err)
	}

	post := func(name string) uint64 {
		param := datastore_types.NewDataStorePreparePostParam()
		param.DataType = types.NewUInt16(0)
		param.Name = types.NewString(name)
		param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(3)
		param.PersistenceInitParam.DeleteLastObject = types.NewBool(false)
		id, nexError := store.createObject(owner, param)
		if nexError != nil {
			t.Fatal(nexError)
		}
		return id
	}

	post("first course")
	second := post("second course")

	target := datastore_types.NewDataStorePersistenceTarget()
	target.OwnerID = owner
	target.PersistenceSlotID = types.NewUInt16(3)
	meta, nexError := store.objectBySlot(target, types.NewUInt64(0))
	if nexError != nil {
		t.Fatal(nexError)
	}
	if uint64(meta.DataID) != second || string(meta.Name) != "second course" {
		t.Errorf("slot 3 should hold the latest course %d, got %d %q", second, meta.DataID, meta.Name)
	}
}

func TestPostedCourseComesBackAndIsSearchable(t *testing.T) {
	db := testDatabase(t)
	store := objectStore{db: db}
	owner := types.NewPID(1800000003)
	if _, err := db.Exec(`DELETE FROM datastore.objects WHERE owner_pid = $1`, owner); err != nil {
		t.Fatal(err)
	}

	param := datastore_types.NewDataStorePreparePostParam()
	param.DataType = types.NewUInt16(7)
	param.Name = types.NewString("test course")
	param.MetaBinary = types.NewQBuffer([]byte{1, 2, 3})
	param.Tags = types.List[types.String]{types.NewString("speedrun")}
	param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(noPersistenceSlot)

	dataID, nexError := store.createObject(owner, param)
	if nexError != nil {
		t.Fatal(nexError)
	}

	meta, nexError := store.object(types.NewUInt64(dataID))
	if nexError != nil {
		t.Fatal(nexError)
	}
	if string(meta.Name) != "test course" || meta.DataType != 7 {
		t.Errorf("course came back as %q type %d", meta.Name, meta.DataType)
	}

	search := datastore_types.NewDataStoreSearchParam()
	search.OwnerIDs = types.List[types.PID]{owner}
	search.DataType = types.NewUInt16(7)
	search.ResultRange.Length = types.NewUInt32(10)
	found, total, nexError := store.search(search, owner)
	if nexError != nil {
		t.Fatal(nexError)
	}
	if total != 1 || len(found) != 1 || found[0].DataID != types.NewUInt64(dataID) {
		t.Errorf("search returned %d of %d courses", len(found), total)
	}
}

func TestScoreBoardKeepsTheBestScoreAndRanksIt(t *testing.T) {
	db := testDatabase(t)
	store := rankingStore{db: db, friends: func(uint32) []uint32 { return nil }}
	category := types.NewUInt32(9001)
	if _, err := db.Exec(`DELETE FROM ranking.scores WHERE category = $1`, uint32(category)); err != nil {
		t.Fatal(err)
	}

	score := ranking_types.NewRankingScoreData()
	score.Category = category
	score.OrderBy = ranking_constants.OrderByDescending
	score.UpdateMode = ranking_constants.UpdateModeNormal

	for _, upload := range []struct {
		pid   uint64
		score uint32
	}{{1800000001, 50}, {1800000002, 70}, {1800000001, 30}} {
		score.Score = types.NewUInt32(upload.score)
		if err := store.insertScore(types.NewPID(upload.pid), score, 0); err != nil {
			t.Fatal(err)
		}
	}

	order := ranking_types.NewRankingOrderParam()
	order.Length = types.NewUInt8(10)
	board, total, err := store.rankings(category, order)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(board) != 2 {
		t.Fatalf("want two ranked players, got %d rows of %d", len(board), total)
	}
	if board[0].Score != 70 || board[0].Order != 1 {
		t.Errorf("the best score should rank first: %d at %d", board[0].Score, board[0].Order)
	}
	if board[1].Score != 50 {
		t.Errorf("a worse re-upload must not replace a better score: got %d", board[1].Score)
	}
}

func TestUploadedScoreReplacesOnlyABetterOne(t *testing.T) {
	cases := []struct {
		name  string
		mode  ranking_constants.UpdateMode
		order ranking_constants.OrderBy
		want  string
	}{
		{"higher wins", ranking_constants.UpdateModeNormal, ranking_constants.OrderByDescending, "EXCLUDED.score > ranking.scores.score"},
		{"lower wins", ranking_constants.UpdateModeNormal, ranking_constants.OrderByAscending, "EXCLUDED.score < ranking.scores.score"},
		{"forced overwrite", ranking_constants.UpdateModeDeleteOld, ranking_constants.OrderByDescending, "true"},
	}
	for _, c := range cases {
		if got := keepsUpdate(c.mode, c.order); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBoardSQLRanksAsTheOrderParamAsks(t *testing.T) {
	standard := boardSQL(true, false, "", "true")
	if !strings.Contains(standard, "RANK() OVER (ORDER BY score DESC") {
		t.Errorf("descending standard board should rank with RANK() DESC:\n%s", standard)
	}
	ordinal := boardSQL(false, true, " AND pid = ANY($4)", "r.pid = $4")
	if !strings.Contains(ordinal, "ROW_NUMBER() OVER (ORDER BY score ASC") {
		t.Errorf("ascending ordinal board should rank with ROW_NUMBER() ASC:\n%s", ordinal)
	}
}

// An unset search filter must not become a filter on zero: a search for "any course
// type" would otherwise return only courses of type 0.
func TestSearchConditionsSkipUnsetFilters(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	where, args := searchConditions(param)
	if where != "deleted = false" || len(args) != 0 {
		t.Errorf("an empty search should filter on nothing: %q with %d args", where, len(args))
	}

	param.DataType = types.NewUInt16(7)
	param.OwnerIDs = types.List[types.PID]{types.NewPID(1800000042)}
	param.Tags = types.List[types.String]{types.NewString("speedrun")}
	where, args = searchConditions(param)
	if len(args) != 3 {
		t.Fatalf("three filters should bind three arguments, got %d", len(args))
	}
	for i, want := range []string{"owner_pid = ANY($1)", "data_type = $2", "tags && $3"} {
		if !strings.Contains(where, want) {
			t.Errorf("filter %d missing from %q, want %q", i, where, want)
		}
	}
}

func TestSearchOrderFallsBackToDataID(t *testing.T) {
	if got := searchOrder(datastore_constants.SearchSortColumnCreatedTime, datastore_constants.SearchSortOrderDesc); got != "created_time DESC" {
		t.Errorf("got %q", got)
	}
	if got := searchOrder(datastore_constants.SearchSortColumn(200), datastore_constants.SearchSortOrderAsc); got != "data_id ASC" {
		t.Errorf("an unknown sort column should fall back to data_id, got %q", got)
	}
}
