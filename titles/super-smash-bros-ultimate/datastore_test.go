package ssbu

import (
	"database/sql"
	"os"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"

	_ "github.com/lib/pq"
)

// The DataStore SQL only means anything against a real Postgres, so this runs when one is
// offered and skips when it is not:
//
//	SSBU_TEST_DATABASE_URL=postgres://... go test ./...
//
// It uses the same schema the server creates, and cleans up after itself.
func testStore(t *testing.T) *store {
	t.Helper()

	url := os.Getenv("SSBU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SSBU_TEST_DATABASE_URL to exercise the DataStore SQL")
	}

	database, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(datastoreSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		database.Exec(`TRUNCATE ssbu.datastore_objects, ssbu.datastore_ratings`)
		database.Close()
	})
	if _, err := database.Exec(`TRUNCATE ssbu.datastore_objects, ssbu.datastore_ratings`); err != nil {
		t.Fatal(err)
	}
	return &store{db: database}
}

func postParam(name string, dataType uint16, tags ...string) datastore_types.DataStorePreparePostParam {
	param := datastore_types.NewDataStorePreparePostParam()
	param.Name = types.NewString(name)
	param.Size = types.NewUInt32(128)
	param.DataType = types.NewUInt16(dataType)
	param.MetaBinary = types.NewQBuffer([]byte("meta"))
	param.Tags = types.NewList[types.String]()
	for _, tag := range tags {
		param.Tags = append(param.Tags, types.NewString(tag))
	}
	param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(noPersistenceSlot)
	return param
}

// The whole PreparePostObject -> CompletePostObject -> GetMeta path, which is what a DataStore
// upload actually is.
func TestObjectRoundTrip(t *testing.T) {
	s := testStore(t)
	owner := types.NewPID(1800000042)

	param := postParam("arena", 7, "event")
	param.Flag = constants.DataFlagNeedCompletion

	dataID, nexErr := s.initializeObject(owner, param)
	if nexErr != nil {
		t.Fatal(nexErr)
	}

	// Until it is completed the object must not be readable — CompletePostObject relies on
	// exactly that to tell "not uploaded yet" from "already done".
	if _, nexErr := s.objectByDataID(types.NewUInt64(dataID)); nexErr == nil {
		t.Fatal("an object needing completion was readable before it was completed")
	}
	// Its owner and size must be readable anyway, because completion needs both.
	if got, nexErr := s.ownerByDataID(types.NewUInt64(dataID)); nexErr != nil || got != 1800000042 {
		t.Fatalf("owner of an incomplete object: got %d, %v", got, nexErr)
	}
	if got, nexErr := s.sizeByDataID(types.NewUInt64(dataID)); nexErr != nil || got != 128 {
		t.Fatalf("size of an incomplete object: got %d, %v", got, nexErr)
	}

	if nexErr := s.setUploaded(types.NewUInt64(dataID), true); nexErr != nil {
		t.Fatal(nexErr)
	}

	object, nexErr := s.objectByDataID(types.NewUInt64(dataID))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if uint64(object.DataID) != dataID {
		t.Errorf("data ID: got %d, want %d", uint64(object.DataID), dataID)
	}
	if string(object.Name) != "arena" {
		t.Errorf("name: got %q", string(object.Name))
	}
	if uint16(object.DataType) != 7 {
		t.Errorf("data type: got %d", uint16(object.DataType))
	}
	if string(object.MetaBinary) != "meta" {
		t.Errorf("meta binary: got %q", string(object.MetaBinary))
	}
	if len(object.Tags) != 1 || string(object.Tags[0]) != "event" {
		t.Errorf("tags: got %v", object.Tags)
	}
	if object.CreatedTime == 0 || object.ExpireTime == 0 {
		t.Error("timestamps did not survive the round trip")
	}

	if nexErr := s.deleteObject(types.NewUInt64(dataID)); nexErr != nil {
		t.Fatal(nexErr)
	}
	if _, nexErr := s.objectByDataID(types.NewUInt64(dataID)); nexErr == nil {
		t.Error("a deleted object was still readable")
	}
}

// A metadata-only object is visible at once: there is no body to wait for.
func TestMetaBinaryObjectNeedsNoCompletion(t *testing.T) {
	s := testStore(t)

	param := postParam("meta-only", 3)
	param.Flag = constants.DataFlagNotUseFileServer

	dataID, nexErr := s.initializeObject(types.NewPID(1800000001), param)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if _, nexErr := s.objectByDataID(types.NewUInt64(dataID)); nexErr != nil {
		t.Fatalf("a metadata-only object was not immediately readable: %v", nexErr)
	}
}

// A persistence slot holds one object: posting a new one retires the last.
func TestPersistenceSlotHoldsOneObject(t *testing.T) {
	s := testStore(t)
	owner := types.NewPID(1800000007)

	slotted := func(name string) datastore_types.DataStorePreparePostParam {
		param := postParam(name, 1)
		param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(3)
		return param
	}

	first, nexErr := s.initializeObject(owner, slotted("first"))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if _, nexErr := s.initializeObject(owner, slotted("second")); nexErr != nil {
		t.Fatal(nexErr)
	}

	if _, nexErr := s.objectByDataID(types.NewUInt64(first)); nexErr == nil {
		t.Error("the previous object in the slot was not retired")
	}

	target := datastore_types.NewDataStorePersistenceTarget()
	target.OwnerID = owner
	target.PersistenceSlotID = types.NewUInt16(3)

	object, nexErr := s.objectByPersistenceTarget(target)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if string(object.Name) != "second" {
		t.Errorf("persistence slot holds %q, want the newest object", string(object.Name))
	}
}

// searchQuery has to survive contact with Postgres, not just look right.
func TestSearchRunsAndPages(t *testing.T) {
	s := testStore(t)
	owner := types.NewPID(1800000100)

	for _, name := range []string{"a", "b", "c"} {
		if _, nexErr := s.initializeObject(owner, postParam(name, 42, "smash")); nexErr != nil {
			t.Fatal(nexErr)
		}
	}
	// A different data type, which the search must not return.
	if _, nexErr := s.initializeObject(owner, postParam("other", 99)); nexErr != nil {
		t.Fatal(nexErr)
	}

	param := datastore_types.NewDataStoreSearchParam()
	param.SearchTarget = constants.SearchTypePublic
	param.DataType = types.NewUInt16(42)
	param.Tags = types.NewList[types.String]()
	param.Tags = append(param.Tags, types.NewString("smash"))
	param.ResultOrderColumn = constants.SearchSortColumnCreatedTime
	param.ResultRange.Length = types.NewUInt32(2)

	objects, total, nexErr := s.search(param, owner)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if len(objects) != 2 {
		t.Errorf("page: got %d objects, want 2", len(objects))
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3 (the unpaged count)", total)
	}
	for _, object := range objects {
		if uint16(object.DataType) != 42 {
			t.Errorf("search returned data type %d", uint16(object.DataType))
		}
	}
}

// Ratings are created with the object and accumulate, and the totals come back.
func TestRatings(t *testing.T) {
	s := testStore(t)

	dataID, nexErr := s.initializeObject(types.NewPID(1800000200), postParam("rated", 5))
	if nexErr != nil {
		t.Fatal(nexErr)
	}

	init := datastore_types.NewDataStoreRatingInitParamWithSlot()
	init.Slot = types.NewInt8(0)
	init.Param.InitialValue = types.NewInt64(10)
	if nexErr := s.initializeRating(dataID, init); nexErr != nil {
		t.Fatal(nexErr)
	}

	rating, nexErr := s.rate(types.NewUInt64(dataID), 0, 5)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if int64(rating.TotalValue) != 15 || uint32(rating.Count) != 1 {
		t.Errorf("after one vote: total %d count %d, want 15 and 1", int64(rating.TotalValue), uint32(rating.Count))
	}

	if _, nexErr := s.rate(types.NewUInt64(dataID), 9, 1); nexErr == nil {
		t.Error("rating a slot that was never initialised should fail")
	}

	object, nexErr := s.objectByDataID(types.NewUInt64(dataID))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if len(object.Ratings) != 1 || int64(object.Ratings[0].Rating.TotalValue) != 15 {
		t.Errorf("ratings did not come back with the object: %v", object.Ratings)
	}
}
