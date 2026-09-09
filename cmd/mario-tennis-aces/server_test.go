package main

import (
	"strings"
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_constants "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	ranking_constants "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/constants"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
)

// A session this title creates arrives closed, and stays invisible to every search
// until it is opened. This is the fix for the double-host deadlock.
func TestCreatedSessionIsOpenedForSearch(t *testing.T) {
	session := match_making_types.NewMatchmakeSession()
	session.OpenParticipation = types.NewBool(false)

	holder := match_making_types.NewGatheringHolder()
	holder.Object = session

	opened, ok := openGathering(holder).Object.(match_making_types.MatchmakeSession)
	if !ok {
		t.Fatal("openGathering lost the MatchmakeSession")
	}
	if !bool(opened.OpenParticipation) {
		t.Error("session is still closed, searches would miss it")
	}

	other := match_making_types.NewPersistentGathering()
	holder.Object = other
	if _, isSession := openGathering(holder).Object.(match_making_types.MatchmakeSession); isSession {
		t.Error("a non-session gathering must be passed through untouched")
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
		{"forced overwrite ascending", ranking_constants.UpdateModeDeleteOld, ranking_constants.OrderByAscending, "true"},
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
	if !strings.Contains(ordinal, "WHERE category = $1 AND pid = ANY($4)") {
		t.Errorf("the cohort filter belongs inside the ranked set:\n%s", ordinal)
	}
	if !strings.Contains(ordinal, "WHERE r.pid = $4") {
		t.Errorf("the scope filter belongs outside the ranked set:\n%s", ordinal)
	}
}

func TestBoardLengthIsBounded(t *testing.T) {
	order := ranking_types.NewRankingOrderParam()

	order.Length = types.NewUInt8(0)
	if got := boardLength(order); got != 100 {
		t.Errorf("an unset length should fall back to 100, got %d", got)
	}

	order.Length = types.NewUInt8(20)
	if got := boardLength(order); got != 20 {
		t.Errorf("a stated length should be honoured, got %d", got)
	}
	if got := halfWindow(order); got != 10 {
		t.Errorf("a nearby window should sit half above the player, got %d", got)
	}
}

// An unset search filter must not become a filter on zero: a title searching for
// "any data type" would otherwise get only objects of type 0.
func TestSearchConditionsSkipUnsetFilters(t *testing.T) {
	param := datastore_types.NewDataStoreSearchParam()
	where, args := searchConditions(param)
	if where != "deleted = false" || len(args) != 0 {
		t.Errorf("an empty search should filter on nothing: %q with %d args", where, len(args))
	}

	param.DataType = types.NewUInt16(7)
	param.OwnerIDs = types.List[types.PID]{types.NewPID(1800000042)}
	param.Tags = types.List[types.String]{types.NewString("tournament")}
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
