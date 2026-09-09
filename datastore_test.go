package mps

import (
	"database/sql"
	"os"
	"testing"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	_ "github.com/lib/pq"
)

// The DataStore store is the one piece of this server that holds state, so it gets checked
// against a real Postgres rather than a mock. Point MPS_TEST_DATABASE_URL at a throwaway
// database to run these; without one they skip, so `go test ./...` needs nothing installed.
func testStore(t *testing.T) *store {
	t.Helper()

	url := os.Getenv("MPS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set MPS_TEST_DATABASE_URL to a throwaway Postgres to run the DataStore checks")
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// A server and endpoint that never listen: all the store wants from them is the
	// library version its metadata is serialised at.
	server := nex.NewPRUDPServer()
	server.LibraryVersions.SetDefault(nex.NewLibraryVersion(4, 6, 5))
	endpoint := nex.NewPRUDPEndPoint(1)
	server.BindPRUDPEndPoint(endpoint)

	s := newStore(db, endpoint)
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`TRUNCATE datastore.objects`); err != nil {
		t.Fatal(err)
	}
	return s
}

func preparePost(name string, slot uint16, deleteLast bool) datastore_types.DataStorePreparePostParam {
	param := datastore_types.NewDataStorePreparePostParam()
	param.Name = types.NewString(name)
	param.Size = types.NewUInt32(12)
	param.DataType = types.NewUInt16(3)
	param.Tags = types.List[types.String]{types.NewString("board")}
	param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(slot)
	param.PersistenceInitParam.DeleteLastObject = types.NewBool(deleteLast)
	return param
}

func TestObjectLifecycle(t *testing.T) {
	s := testStore(t)
	owner := types.NewPID(1800000003)

	dataID, nexErr := s.initializeObject(owner, preparePost("record", noPersistenceSlot, false))
	if nexErr != nil {
		t.Fatal(nexErr)
	}

	// Before the upload completes the object must not be readable, or complete_post_object
	// would refuse every first upload as "already uploaded".
	if _, nexErr := s.objectByDataID(types.NewUInt64(dataID)); nexErr == nil {
		t.Fatal("an object whose upload has not completed must not be readable")
	}
	// The owner and size are readable, though: complete_post_object checks both.
	if got, nexErr := s.ownerByDataID(types.NewUInt64(dataID)); nexErr != nil || got != uint32(owner) {
		t.Fatalf("owner before completion: got %d, %v", got, nexErr)
	}
	if got, nexErr := s.sizeByDataID(types.NewUInt64(dataID)); nexErr != nil || got != 12 {
		t.Fatalf("size before completion: got %d, %v", got, nexErr)
	}

	if nexErr := s.setUploadCompleted(types.NewUInt64(dataID), true); nexErr != nil {
		t.Fatal(nexErr)
	}

	meta, nexErr := s.objectByDataID(types.NewUInt64(dataID))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if uint64(meta.DataID) != dataID || string(meta.Name) != "record" || uint16(meta.DataType) != 3 {
		t.Fatalf("metadata did not survive the round trip: %+v", meta)
	}
	if len(meta.Tags) != 1 || string(meta.Tags[0]) != "board" {
		t.Fatalf("tags did not survive the round trip: %v", meta.Tags)
	}

	if nexErr := s.updateDataType(types.NewUInt64(dataID), types.NewUInt16(9), 0); nexErr != nil {
		t.Fatal(nexErr)
	}
	if meta, _ := s.objectByDataID(types.NewUInt64(dataID)); uint16(meta.DataType) != 9 {
		t.Errorf("data type was not updated, got %d", meta.DataType)
	}

	if nexErr := s.delete(dataID); nexErr != nil {
		t.Fatal(nexErr)
	}
	if _, nexErr := s.objectByDataID(types.NewUInt64(dataID)); nexErr == nil {
		t.Error("a deleted object must not be readable")
	}
	if nexErr := s.delete(dataID); nexErr == nil {
		t.Error("deleting an already deleted object must answer NotFound")
	}
}

func TestPersistenceSlotHoldsOneObject(t *testing.T) {
	s := testStore(t)
	owner := types.NewPID(1800000004)

	first, nexErr := s.initializeObject(owner, preparePost("save-1", 7, false))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if nexErr := s.setUploadCompleted(types.NewUInt64(first), true); nexErr != nil {
		t.Fatal(nexErr)
	}

	target := datastore_types.NewDataStorePersistenceTarget()
	target.OwnerID = owner
	target.PersistenceSlotID = types.NewUInt16(7)

	meta, nexErr := s.objectByPersistenceTarget(target, 0)
	if nexErr != nil || uint64(meta.DataID) != first {
		t.Fatalf("slot 7 should hold the first object, got %v, %v", meta.DataID, nexErr)
	}

	// Posting to the same slot again hands it to the new object. The old one survives as
	// an ordinary object because DeleteLastObject was not set.
	second, nexErr := s.initializeObject(owner, preparePost("save-2", 7, false))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if nexErr := s.setUploadCompleted(types.NewUInt64(second), true); nexErr != nil {
		t.Fatal(nexErr)
	}

	meta, nexErr = s.objectByPersistenceTarget(target, 0)
	if nexErr != nil || uint64(meta.DataID) != second {
		t.Fatalf("slot 7 should now hold the second object, got %v, %v", meta.DataID, nexErr)
	}
	if _, nexErr := s.objectByDataID(types.NewUInt64(first)); nexErr != nil {
		t.Error("the displaced object should still exist when DeleteLastObject is not set")
	}
}

func TestRatingAccumulates(t *testing.T) {
	s := testStore(t)

	param := preparePost("minigame", noPersistenceSlot, false)
	ratingInit := datastore_types.NewDataStoreRatingInitParamWithSlot()
	ratingInit.Slot = types.NewInt8(0)
	ratingInit.Param.InitialValue = types.NewInt64(100)
	param.RatingInitParams = types.List[datastore_types.DataStoreRatingInitParamWithSlot]{ratingInit}

	dataID, nexErr := s.initializeObject(types.NewPID(1800000003), param)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	for _, initParam := range param.RatingInitParams {
		if nexErr := s.initializeRating(dataID, initParam); nexErr != nil {
			t.Fatal(nexErr)
		}
	}

	rating, nexErr := s.rate(types.NewUInt64(dataID), types.NewUInt8(0), types.NewInt32(5), 0)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if int64(rating.TotalValue) != 105 || uint32(rating.Count) != 1 {
		t.Fatalf("after one vote of 5 on an initial 100: want 105/1, got %d/%d", rating.TotalValue, rating.Count)
	}

	rating, nexErr = s.rate(types.NewUInt64(dataID), types.NewUInt8(0), types.NewInt32(-3), 0)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if int64(rating.TotalValue) != 102 || uint32(rating.Count) != 2 {
		t.Fatalf("after a second vote of -3: want 102/2, got %d/%d", rating.TotalValue, rating.Count)
	}

	if _, nexErr := s.rate(types.NewUInt64(dataID), types.NewUInt8(3), types.NewInt32(1), 0); nexErr == nil {
		t.Error("rating a slot the object does not have must answer NotFound")
	}
}

func TestSearchFiltersAndCounts(t *testing.T) {
	s := testStore(t)
	host, joiner := types.NewPID(1800000003), types.NewPID(1800000004)

	for _, seed := range []struct {
		owner types.PID
		name  string
	}{{host, "a"}, {host, "b"}, {joiner, "c"}} {
		dataID, nexErr := s.initializeObject(seed.owner, preparePost(seed.name, noPersistenceSlot, false))
		if nexErr != nil {
			t.Fatal(nexErr)
		}
		if nexErr := s.setUploadCompleted(types.NewUInt64(dataID), true); nexErr != nil {
			t.Fatal(nexErr)
		}
	}

	param := datastore_types.NewDataStoreSearchParam()
	param.OwnerIDs = types.List[types.PID]{host}
	param.ResultRange.Length = types.NewUInt32(10)

	objects, total, nexErr := s.search(param, host)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if total != 2 || len(objects) != 2 {
		t.Fatalf("searching one owner's two objects: want 2/2, got %d/%d", total, len(objects))
	}

	// The result range pages; the total still counts everything that matched.
	param.ResultRange.Length = types.NewUInt32(1)
	objects, total, nexErr = s.search(param, host)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if total != 2 || len(objects) != 1 {
		t.Fatalf("a one-object page of two matches: want total 2 and 1 object, got %d/%d", total, len(objects))
	}

	// An unmatched data type must answer empty, never NotFound.
	param.DataType = types.NewUInt16(200)
	objects, total, nexErr = s.search(param, host)
	if nexErr != nil {
		t.Fatalf("a search with no results is not an error: %v", nexErr)
	}
	if total != 0 || len(objects) != 0 {
		t.Fatalf("want no results, got %d/%d", total, len(objects))
	}
}
