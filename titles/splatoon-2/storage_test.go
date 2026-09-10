package splatoon2

import (
	"database/sql"
	"os"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	ranking_constants "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/constants"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	_ "github.com/lib/pq"
)

// The queries in datastore.go and ranking.go are strings until a database parses them.
// Point S2_TEST_DATABASE_URL at a throwaway Postgres to have them parsed:
//
//	podman run --rm -d -e POSTGRES_PASSWORD=check -p 23099:5432 postgres:16-alpine
//	S2_TEST_DATABASE_URL='postgres://postgres:check@127.0.0.1:23099/postgres?sslmode=disable' go test ./...
//
// Without it there is nothing to talk to and the test skips.
func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("S2_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("S2_TEST_DATABASE_URL is not set")
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

	// The other four ranking modes are four more query strings, and a query string is
	// only known to be valid once Postgres has parsed it.
	player := types.NewPID(1800000001)
	for name, mode := range map[string]func(types.PID, types.UInt32, ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error){
		"own":            store.ownRanking,
		"nearby":         store.nearbyRankings,
		"friends":        store.friendRankings,
		"nearby friends": store.nearbyFriendRankings,
	} {
		rows, _, err := mode(player, category, order)
		if err != nil {
			t.Errorf("%s ranking: %v", name, err)
		} else if len(rows) == 0 {
			t.Errorf("%s ranking returned nothing for a player who has a score", name)
		}
	}
}

func TestPostedObjectComesBackAndIsSearchable(t *testing.T) {
	db := testDatabase(t)
	store := objectStore{db: db}
	owner := types.NewPID(1800000003)
	if _, err := db.Exec(`DELETE FROM datastore.objects WHERE owner_pid = $1`, owner); err != nil {
		t.Fatal(err)
	}

	param := datastore_types.NewDataStorePreparePostParam()
	param.DataType = types.NewUInt16(7)
	param.Name = types.NewString("splatfest entry")
	param.MetaBinary = types.NewQBuffer([]byte{1, 2, 3})
	param.Tags = types.List[types.String]{types.NewString("splatfest")}
	param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(noPersistenceSlot)

	dataID, nexError := store.createObject(owner, param)
	if nexError != nil {
		t.Fatal(nexError)
	}

	meta, nexError := store.object(types.NewUInt64(dataID))
	if nexError != nil {
		t.Fatal(nexError)
	}
	if string(meta.Name) != "splatfest entry" || meta.DataType != 7 {
		t.Errorf("object came back as %q type %d", meta.Name, meta.DataType)
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
		t.Errorf("search returned %d of %d objects", len(found), total)
	}
}
