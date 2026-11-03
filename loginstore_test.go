package nexhost

import (
	"database/sql"
	"os"
	"testing"
)

// A restart forgets the in-process map; the next process must still resolve the name.
// Needs a throwaway Postgres: NEX_SESSIONS_TEST_DATABASE_URL.
func TestLoginNamesSurviveRestart(t *testing.T) {
	url := os.Getenv("NEX_SESSIONS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("NEX_SESSIONS_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`DROP TABLE IF EXISTS nexhost.login_names`)
	if err := UseLoginStore(db); err != nil {
		t.Fatal(err)
	}
	const name = "11389664994428258486"
	const pid = uint64(1800000042)
	(&Host{}).rememberLoginName(name, pid)

	// "restart": drop the in-process map.
	loginNames.Delete(name)
	got, ok := PIDForLoginName(name)
	if !ok || got != pid {
		t.Fatalf("after restart: got %d, %v", got, ok)
	}
	if _, ok := PIDForLoginName("nobody"); ok {
		t.Fatal("unknown name resolved")
	}
}
