package mps

import (
	"testing"

	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_constants "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	match_making_constants "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/constants"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
)

func TestParseIntegerSettings(t *testing.T) {
	settings := parseIntegerSettings("0:0=60, 0:1=30 10:7=-1, nonsense, 0:bad=1, 1:2=notanumber")

	if len(settings) != 2 {
		t.Fatalf("want settings for two indexes, got %d: %v", len(settings), settings)
	}
	if got := settings[0][types.NewUInt16(0)]; got != types.NewUInt32(60) {
		t.Errorf("index 0 key 0: want 60, got %d", got)
	}
	if got := settings[0][types.NewUInt16(1)]; got != types.NewUInt32(30) {
		t.Errorf("index 0 key 1: want 30, got %d", got)
	}
	// Signed on the wire, so -1 has to survive as the same 32 bits.
	if got := settings[10][types.NewUInt16(7)]; int32(got) != -1 {
		t.Errorf("index 10 key 7: want -1, got %d", int32(got))
	}
	if _, ok := settings[1]; ok {
		t.Error("an entry with an unparsable value must be skipped, not stored")
	}
}

func TestParseIntegerSettingsEmpty(t *testing.T) {
	if settings := parseIntegerSettings(""); len(settings) != 0 {
		t.Errorf("unset MPS_INTEGER_SETTINGS must configure nothing, got %v", settings)
	}
}

func TestApplyResultOptionDropsUnrequestedFields(t *testing.T) {
	session := match_making_types.NewMatchmakeSession()
	session.ApplicationBuffer = types.NewBuffer([]byte{1, 2, 3})
	session.MatchmakeParam.Params = types.NewMap[types.String, types.Variant]()
	session.MatchmakeParam.Params[types.NewString("x")] = types.NewVariant()

	kept := applyResultOption(session, match_making_constants.FindMatchmakeSessionResultOptionApplicationBuffer)
	if len(kept.ApplicationBuffer) != 3 {
		t.Errorf("a requested application buffer must survive, got %v", kept.ApplicationBuffer)
	}
	if len(kept.MatchmakeParam.Params) != 0 {
		t.Errorf("an unrequested matchmake param must be cleared, got %v", kept.MatchmakeParam.Params)
	}

	dropped := applyResultOption(session, match_making_constants.FindMatchmakeSessionResultOptionNone)
	if len(dropped.ApplicationBuffer) != 0 {
		t.Errorf("an unrequested application buffer must be cleared, got %v", dropped.ApplicationBuffer)
	}
}

func TestSearchOrderBy(t *testing.T) {
	cases := []struct {
		column datastore_constants.SearchSortColumn
		order  datastore_constants.SearchSortOrder
		want   string
	}{
		{datastore_constants.SearchSortColumnDataID, datastore_constants.SearchSortOrderAsc, "data_id ASC"},
		{datastore_constants.SearchSortColumnUpdatedTime, datastore_constants.SearchSortOrderAsc, "updated_time ASC"},
		{datastore_constants.SearchSortColumnSize, datastore_constants.SearchSortOrderAsc + 1, "size DESC"},
		// A rating slot is not a column of this table; sorting must still be stable.
		{datastore_constants.SearchSortColumnRating0, datastore_constants.SearchSortOrderAsc, "data_id ASC"},
	}

	for _, test := range cases {
		param := datastore_types.NewDataStoreSearchParam()
		param.ResultOrderColumn = test.column
		param.ResultOrder = test.order

		if got := orderBy(param); got != test.want {
			t.Errorf("column %v order %v: want %q, got %q", test.column, test.order, test.want, got)
		}
	}
}

// The ORDER BY clause is interpolated into the query rather than bound, so it must never
// contain anything a caller controls.
func TestSearchOrderByIsAlwaysAKnownColumn(t *testing.T) {
	known := map[string]bool{
		"data_id": true, "size": true, "name": true,
		"data_type": true, "created_time": true, "updated_time": true,
	}

	for column := 0; column < 256; column++ {
		param := datastore_types.NewDataStoreSearchParam()
		param.ResultOrderColumn = datastore_constants.SearchSortColumn(column)

		clause := orderBy(param)
		name, direction, _ := cut(clause)
		if !known[name] || (direction != "ASC" && direction != "DESC") {
			t.Fatalf("sort column %d produced %q, which is not a known column and direction", column, clause)
		}
	}
}

func cut(clause string) (string, string, bool) {
	for i := range clause {
		if clause[i] == ' ' {
			return clause[:i], clause[i+1:], true
		}
	}
	return clause, "", false
}

func TestFromEnvDefaultsToThisTitle(t *testing.T) {
	t.Setenv("NEX_ACCESS_KEY", "e915510f")
	t.Setenv("NX_INTERNAL_KEY", "test")
	t.Setenv("MPS_DATABASE_URL", "postgres://localhost/test")

	cfg := FromEnv()

	if cfg.Host.PortBase != PortBase {
		t.Errorf("want the title's own block %d, got %d", PortBase, cfg.Host.PortBase)
	}
	if cfg.Host.Version != NEXVersion {
		t.Errorf("want NEX %s, got %s", NEXVersion, cfg.Host.Version)
	}
	if cfg.Host.HTTPAddr != ":22112" {
		t.Errorf("want healthz on the block's +2, got %s", cfg.Host.HTTPAddr)
	}
}

func TestFromEnvHonoursOverrides(t *testing.T) {
	t.Setenv("NEX_PORT_BASE", "22300")
	t.Setenv("NEX_VERSION", "4.4.0")
	t.Setenv("NEX_ACCESS_KEY", "e915510f")
	t.Setenv("NX_INTERNAL_KEY", "test")

	cfg := FromEnv()

	if cfg.Host.PortBase != 22300 {
		t.Errorf("NEX_PORT_BASE must win, got %d", cfg.Host.PortBase)
	}
	if cfg.Host.Version != "4.4.0" {
		t.Errorf("NEX_VERSION must win, got %s", cfg.Host.Version)
	}
}
