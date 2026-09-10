package arms

import (
	"strings"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
)

// A search always excludes deleted and not-yet-uploaded objects, and always ends up with
// one argument per placeholder — the property that makes the whole thing injection-proof.
func TestSearchQueryIsFullyParameterised(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	param.SearchTarget = constants.SearchTypePublic
	param.DataType = types.NewUInt16(7)
	param.OwnerIDs = types.NewList[types.PID]()
	param.OwnerIDs = append(param.OwnerIDs, types.NewPID(1800000042))
	param.Tags = types.NewList[types.String]()
	param.Tags = append(param.Tags, types.NewString("party-crash"))

	query, args := searchQuery(param, 1800000001, nil)

	if !strings.Contains(query, "uploaded AND NOT deleted") {
		t.Errorf("search does not exclude incomplete or deleted objects: %s", query)
	}
	if want := strings.Count(query, "$"); want != len(args) {
		t.Errorf("query has %d placeholders but %d arguments", want, len(args))
	}
	// Nothing a console sent may appear as text in the query.
	for _, leaked := range []string{"party-crash", "1800000042", "1800000001"} {
		if strings.Contains(query, leaked) {
			t.Errorf("value %q was interpolated into the query instead of bound: %s", leaked, query)
		}
	}
}

// An unknown sort column must fall back, never reach the SQL.
func TestSearchQuerySortColumnIsWhitelisted(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	param.ResultOrderColumn = constants.SearchSortColumnRating0 // deliberately unsupported
	param.ResultOrder = constants.SearchSortOrderDesc

	query, _ := searchQuery(param, 1, nil)

	if !strings.Contains(query, "ORDER BY data_id DESC") {
		t.Errorf("unsupported sort column did not fall back to data_id: %s", query)
	}

	param.ResultOrderColumn = constants.SearchSortColumnCreatedTime
	param.ResultOrder = constants.SearchSortOrderAsc
	if query, _ := searchQuery(param, 1, nil); !strings.Contains(query, "ORDER BY created ASC") {
		t.Errorf("supported sort column was not used: %s", query)
	}
}

// A page size of zero means "the default", and an absurd one is capped rather than served.
func TestSearchQueryLimitIsCapped(t *testing.T) {
	for _, requested := range []uint32{0, searchLimit + 1, 1 << 20} {
		param := datastore_types.NewDataStoreSearchParam()
		param.ResultRange.Length = types.NewUInt32(requested)

		_, args := searchQuery(param, 1, nil)
		if limit := args[len(args)-1]; limit != int64(searchLimit) {
			t.Errorf("length %d produced limit %v, want %d", requested, limit, searchLimit)
		}
	}

	param := datastore_types.NewDataStoreSearchParam()
	param.ResultRange.Length = types.NewUInt32(10)
	if _, args := searchQuery(param, 1, nil); args[len(args)-1] != int64(10) {
		t.Errorf("a reasonable page size was not honoured: %v", args[len(args)-1])
	}
}

// Only the searches that actually need a friend graph pay for the nx-baas call.
func TestNeedsFriends(t *testing.T) {
	friendSearch := datastore_types.NewDataStoreSearchParam()
	friendSearch.SearchTarget = constants.SearchTypeFriend
	if !needsFriends(friendSearch) {
		t.Error("a friends-only search must fetch the friend graph")
	}

	publicSearch := datastore_types.NewDataStoreSearchParam()
	publicSearch.SearchTarget = constants.SearchTypePublic
	if needsFriends(publicSearch) {
		t.Error("a public search must not cost an nx-baas call")
	}
}
