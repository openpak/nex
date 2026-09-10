package main

// SearchObject's WHERE clause. LIFT CANDIDATE -- see the header of datastore.go.
//
// Everything a console sends arrives here as a placeholder argument. The one part of the
// query a console influences textually is the ORDER BY column, and that comes out of a
// fixed map — an unknown sort column falls back to data_id rather than reaching the SQL.

import (
	"fmt"
	"strings"

	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	"github.com/lib/pq"
)

// sortColumns maps the sort column a client asks for to the column it means. Rating sorts
// (64+ and 96+) are absent on purpose.
//
// ponytail: no rating sorts — they need a join per slot and Strikers has never been seen
// asking for one. Add the join if a capture shows a search with SearchSortColumnRating*.
var sortColumns = map[constants.SearchSortColumn]string{
	constants.SearchSortColumnDataID:           "data_id",
	constants.SearchSortColumnSize:             "size",
	constants.SearchSortColumnNameAlphabetical: "name",
	constants.SearchSortColumnDataType:         "data_type",
	constants.SearchSortColumnReferredCount:    "referred_count",
	constants.SearchSortColumnCreatedTime:      "created",
	constants.SearchSortColumnUpdatedTime:      "updated",
}

// needsFriends reports whether answering this search means asking nx-baas who the caller's
// friends are. Most searches do not, and each one that does costs an HTTP call.
func needsFriends(param datastore_types.DataStoreSearchParam) bool {
	return param.OwnerType == constants.SearchTargetFriend ||
		param.SearchTarget == constants.SearchTypeFriend ||
		param.SearchTarget == constants.SearchTypeReceived ||
		param.SearchTarget == constants.SearchTypePublicExcludeOwnAndFriends
}

// searchQuery builds the SELECT for one DataStoreSearchParam. The trailing count(*) OVER ()
// is the unpaged total the client is told about, which is why this is one query and not two.
//
// The predicates below only narrow the result set; nex-protocols-common-go re-checks every
// returned object against the caller's permissions before it goes on the wire, so a search
// type we do not translate can cost a wasted row, never a leaked one.
func searchQuery(param datastore_types.DataStoreSearchParam, caller uint64, friends []uint32) (string, []any) {
	var where []string
	var args []any

	// placeholder records an argument and returns its $n.
	placeholder := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	where = append(where, "uploaded", "NOT deleted")

	switch param.SearchTarget {
	case constants.SearchTypePublic:
		where = append(where, "permission = "+placeholder(int64(constants.PermissionPublic)))
	case constants.SearchTypeOwn, constants.SearchTypeOwnAll, constants.SearchTypePrivate,
		constants.SearchTypeOwnPending, constants.SearchTypeOwnRejected, constants.SearchTypeSend,
		constants.SearchTypeSendFriend, constants.SearchTypeSendSpecified, constants.SearchTypeSendSpecifiedFriend:
		where = append(where, "owner_pid = "+placeholder(caller))
	case constants.SearchTypeFriend:
		where = append(where, "owner_pid = ANY("+placeholder(pidStrings(friends))+")")
	}

	switch param.OwnerType {
	case constants.SearchTargetFriend:
		where = append(where, "owner_pid = ANY("+placeholder(pidStrings(friends))+")")
	case constants.SearchTargetAnybodyExcludeSpecified:
		if len(param.OwnerIDs) > 0 {
			where = append(where, "NOT (owner_pid = ANY("+placeholder(pidArray(param.OwnerIDs))+"))")
		}
	default:
		if len(param.OwnerIDs) > 0 {
			where = append(where, "owner_pid = ANY("+placeholder(pidArray(param.OwnerIDs))+")")
		}
	}

	// DataTypes is the NEX 4 list form; DataType is the single-value one it replaced.
	if len(param.DataTypes) > 0 {
		dataTypes := make([]int64, 0, len(param.DataTypes))
		for _, dataType := range param.DataTypes {
			dataTypes = append(dataTypes, int64(dataType))
		}
		where = append(where, "data_type = ANY("+placeholder(pq.Array(dataTypes))+")")
	} else if param.DataType != 0 {
		where = append(where, "data_type = "+placeholder(int64(param.DataType)))
	}

	if param.ReferDataID != 0 {
		where = append(where, "refer_data_id = "+placeholder(int64(param.ReferDataID)))
	}
	if len(param.Tags) > 0 {
		where = append(where, "tags @> "+placeholder(pq.Array(stringSlice(param.Tags))))
	}
	for _, bound := range []struct {
		column   string
		operator string
		value    types.DateTime
	}{
		{"created", ">=", param.CreatedAfter},
		{"created", "<=", param.CreatedBefore},
		{"updated", ">=", param.UpdatedAfter},
		{"updated", "<=", param.UpdatedBefore},
	} {
		if bound.value != 0 {
			where = append(where, bound.column+" "+bound.operator+" "+placeholder(bound.value.Standard()))
		}
	}

	column, ok := sortColumns[param.ResultOrderColumn]
	if !ok {
		column = "data_id"
	}
	direction := "ASC"
	if param.ResultOrder == constants.SearchSortOrderDesc {
		direction = "DESC"
	}

	limit := uint32(param.ResultRange.Length)
	if limit == 0 || limit > searchLimit {
		limit = searchLimit
	}

	query := `SELECT ` + objectColumns + `, count(*) OVER () FROM strikers.datastore_objects WHERE ` +
		strings.Join(where, " AND ") +
		` ORDER BY ` + column + ` ` + direction +
		` OFFSET ` + placeholder(int64(param.ResultRange.Offset)) +
		` LIMIT ` + placeholder(int64(limit))
	return query, args
}

// pidStrings renders PIDs for a numeric(20)[] comparison. Postgres will not compare a
// numeric column against a bigint array, and a PID does not fit an int64 by contract.
func pidStrings(pids []uint32) any {
	values := make([]string, 0, len(pids))
	for _, pid := range pids {
		values = append(values, fmt.Sprint(pid))
	}
	return pq.Array(values)
}
