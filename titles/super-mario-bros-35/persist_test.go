package smb35

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking2_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking2/types"
)

// Common data survives a restart: a new store on the same database still answers it. Needs a
// Postgres (SMB35_TEST_DATABASE_URL); skipped without one.
func TestCommonDataSurvivesRestart(t *testing.T) {
	dsn := os.Getenv("SMB35_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SMB35_TEST_DATABASE_URL not set")
	}
	open := func() *ranking2Store {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		st, err := newRanking2Store(db)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	a := open()
	d := ranking2_types.NewRanking2CommonData()
	d.UserName = types.NewString("ann")
	d.Mii = types.NewQBuffer([]byte{1, 2, 3})
	d.BinaryData = types.NewQBuffer([]byte{9})
	const uid = 0xfedcba9876543210 // above int64: must round-trip
	a.put(77, uid, d)

	b := open()
	got, ok := b.get(77, uid)
	if !ok || string(got.UserName) != "ann" || len(got.Mii) != 3 || got.BinaryData[0] != 9 {
		t.Fatalf("after restart: %+v %v", got, ok)
	}
	d.UserName = types.NewString("bea")
	b.put(77, uid, d)
	if got, _ := open().get(77, uid); string(got.UserName) != "bea" {
		t.Fatalf("update lost: %q", got.UserName)
	}
	if _, ok := b.get(77, 1); ok {
		t.Fatal("unknown unique id answered")
	}
	b.db.Exec(`DELETE FROM smb35.common_data WHERE pid = 77`)
}
