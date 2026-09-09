package main

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// A search with nothing set must still be a valid query: uploaded objects only, oldest
// first, and a bounded number of them.
func TestSearchQueryEmpty(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	param.DataType = types.NewUInt16(anyDataType)

	where, order, args, limit, offset := searchQuery(param)

	if where != "uploaded" {
		t.Errorf("where = %q, want %q", where, "uploaded")
	}
	if order != "data_id ASC" {
		t.Errorf("order = %q, want %q", order, "data_id ASC")
	}
	if len(args) != 0 {
		t.Errorf("args = %v, want none", args)
	}
	if limit != searchLimit || offset != 0 {
		t.Errorf("range = %d..%d, want limit %d from 0", offset, limit, searchLimit)
	}
}

// Every filter that is set becomes one condition, and its placeholder must be the position
// of its own argument — an off-by-one here binds a tag against a date.
func TestSearchQueryFilters(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	param.OwnerIDs = types.List[types.PID]{types.NewPID(1800000001)}
	param.DataType = types.NewUInt16(7)
	param.Tags = types.List[types.String]{types.NewString("quest")}
	param.ReferDataID = types.NewUInt32(42)
	param.CreatedAfter = dateTime(time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC))
	param.ResultOrderColumn = constants.SearchSortColumnUpdatedTime
	param.ResultOrder = constants.SearchSortOrderDesc
	param.ResultRange.Offset = types.NewUInt32(20)
	param.ResultRange.Length = types.NewUInt32(5000)

	where, order, args, limit, offset := searchQuery(param)

	want := "uploaded AND owner_pid = ANY($1::numeric(20)[]) AND data_type = $2 AND " +
		"tags @> $3::text[] AND refer_data_id = $4 AND created_at >= $5"
	if where != want {
		t.Errorf("where =\n%q\nwant\n%q", where, want)
	}
	if len(args) != 5 {
		t.Fatalf("args = %v, want 5", args)
	}
	if owners, ok := args[0].(string); !ok || !strings.Contains(owners, "1800000001") {
		t.Errorf("first argument = %v, want the owner array literal", args[0])
	}
	if args[1] != uint16(7) {
		t.Errorf("data type argument = %v, want 7", args[1])
	}
	if order != "updated_at DESC" {
		t.Errorf("order = %q, want %q", order, "updated_at DESC")
	}
	if limit != searchLimit {
		t.Errorf("limit = %d, want it capped at %d", limit, searchLimit)
	}
	if offset != 20 {
		t.Errorf("offset = %d, want 20", offset)
	}
}

// A list of data types replaces the single one rather than adding to it: a query with both
// set must not exclude the very types it was asked for.
func TestSearchQueryDataTypeList(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	param.DataType = types.NewUInt16(7)
	param.DataTypes = types.List[types.UInt16]{types.NewUInt16(1), types.NewUInt16(2)}

	where, _, args, _, _ := searchQuery(param)

	if where != "uploaded AND data_type = ANY($1::integer[])" {
		t.Errorf("where = %q, want the list form", where)
	}
	if literal, ok := args[0].(string); !ok || literal != "{1,2}" {
		t.Errorf("data types argument = %v, want {1,2}", args[0])
	}
}

// An object with no period never expires; the wire has no way to say that, so it gets a
// date far enough out to mean the same thing.
func TestExpiry(t *testing.T) {
	created := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	if got := expiry(created, 7); !got.Equal(created.AddDate(0, 0, 7)) {
		t.Errorf("expiry with period 7 = %v, want seven days on", got)
	}
	if got := expiry(created, 0); got.Sub(created) < 50*365*24*time.Hour {
		t.Errorf("expiry with no period = %v, want far in the future", got)
	}
}

// The store itself needs a database, so this runs only when one is pointed at. It is the
// check that the SQL is real SQL: the array literals, the casts and the list scanning are
// the parts a compiler cannot see are wrong.
//
//	DATABASE_URL=postgres://…/mhgu_test go test ./...
func TestStoreRoundTrip(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	objects, err := newStore(db)
	if err != nil {
		t.Fatal(err)
	}
	// Clear the test owner up front rather than afterwards: a failed run leaves its rows
	// behind, and the next run should not inherit them.
	if _, err := db.Exec(`DELETE FROM datastore.objects WHERE owner_pid = 1800000009`); err != nil {
		t.Fatal(err)
	}

	param := datastore_types.NewDataStorePreparePostParam()
	param.Size = types.NewUInt32(1234)
	param.Name = types.NewString("guild card")
	param.DataType = types.NewUInt16(7)
	param.MetaBinary = types.NewQBuffer([]byte{1, 2, 3})
	param.Tags = types.List[types.String]{types.NewString("quest")}
	param.PersistenceInitParam.PersistenceSlotID = types.NewUInt16(3)
	param.RatingInitParams = types.List[datastore_types.DataStoreRatingInitParamWithSlot]{}

	owner := types.NewPID(1800000009)
	dataID, nexErr := objects.create(owner, param)
	if nexErr != nil {
		t.Fatal(nexErr)
	}

	// Not visible until the upload completes.
	if _, nexErr := objects.meta(types.NewUInt64(dataID)); nexErr == nil {
		t.Fatal("an object that was never uploaded is being served")
	}
	if nexErr := objects.setUploaded(types.NewUInt64(dataID), true); nexErr != nil {
		t.Fatal(nexErr)
	}

	meta, nexErr := objects.meta(types.NewUInt64(dataID))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if string(meta.Name) != "guild card" || uint16(meta.DataType) != 7 || uint64(meta.OwnerID) != 1800000009 {
		t.Errorf("meta = %q/%d/%d, want the object that was posted", meta.Name, meta.DataType, meta.OwnerID)
	}
	if len(meta.Tags) != 1 || string(meta.Tags[0]) != "quest" {
		t.Errorf("tags = %v, want [quest]", meta.Tags)
	}

	// The persistence slot is how the game asks for "my object" without knowing its ID.
	target := datastore_types.NewDataStorePersistenceTarget()
	target.OwnerID = owner
	target.PersistenceSlotID = types.NewUInt16(3)
	if _, nexErr := objects.metaByPersistence(target, types.NewUInt64(0)); nexErr != nil {
		t.Fatal(nexErr)
	}

	search := datastore_types.NewDataStoreSearchParam()
	search.OwnerIDs = types.List[types.PID]{owner}
	search.Tags = types.List[types.String]{types.NewString("quest")}
	search.DataType = types.NewUInt16(anyDataType)
	found, total, nexErr := objects.search(search, owner)
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if total != 1 || len(found) != 1 {
		t.Fatalf("search found %d of %d, want the one object", len(found), total)
	}

	// Ratings: a slot has to exist before it can be voted on, and a vote has to come back
	// as the slot's new totals.
	rating := datastore_types.NewDataStoreRatingInitParamWithSlot()
	rating.Slot = types.NewInt8(0)
	rating.Param.InitialValue = types.NewInt64(10)
	rating.Param.RangeMin = types.NewInt32(-5)
	rating.Param.RangeMax = types.NewInt32(5)
	if nexErr := objects.createRating(dataID, rating); nexErr != nil {
		t.Fatal(nexErr)
	}
	info, nexErr := objects.rate(types.NewUInt64(dataID), types.NewUInt8(0), types.NewInt32(3), types.NewUInt64(0))
	if nexErr != nil {
		t.Fatal(nexErr)
	}
	if int64(info.TotalValue) != 13 || uint32(info.Count) != 1 {
		t.Errorf("rating = %d over %d votes, want 13 over 1", info.TotalValue, info.Count)
	}
	if _, nexErr := objects.rate(types.NewUInt64(dataID), types.NewUInt8(0), types.NewInt32(99), types.NewUInt64(0)); nexErr == nil {
		t.Error("a vote outside the slot range was accepted")
	}
}
