package nexhost

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const (
	eraseA = "11111111-1111-4111-8111-111111111111"
	eraseB = "22222222-2222-4222-8222-222222222222"
)

type fakePIDs map[string][]string

func (f fakePIDs) PIDs(_ context.Context, a string) ([]string, error) { return f[a], nil }

func eraseDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("NEX_ERASE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("NEX_ERASE_TEST_DATABASE_URL not set (a throwaway Postgres)")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func eraseExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func eraseCount(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// The pids: A is 1700000001 (Wii U) and a Switch id above 2^63, B is 1700000002.
const (
	pidA1 = "1700000001"
	pidA2 = "12297829382472990721"
	pidB  = "1700000002"
)

func buildErasureSchemas(t *testing.T, db *sql.DB) {
	for _, q := range []string{
		`DROP SCHEMA IF EXISTS datastore, ranking, mk8, messaging, matchmaking, tracking, nexhost, utility CASCADE`,
		`CREATE SCHEMA datastore; CREATE SCHEMA ranking; CREATE SCHEMA mk8; CREATE SCHEMA messaging;
		 CREATE SCHEMA matchmaking; CREATE SCHEMA tracking; CREATE SCHEMA nexhost; CREATE SCHEMA utility`,
		// A DataStore like Super Mario Maker 2's: ratings reference the object without a cascade.
		`CREATE TABLE datastore.objects (data_id bigserial PRIMARY KEY, owner_pid numeric(20) NOT NULL,
		   permission int NOT NULL DEFAULT 0, permission_recipients bigint[] NOT NULL DEFAULT '{}',
		   upload_completed boolean NOT NULL DEFAULT false, deleted boolean NOT NULL DEFAULT false)`,
		`CREATE TABLE datastore.ratings (data_id bigint NOT NULL REFERENCES datastore.objects (data_id), slot smallint NOT NULL, total bigint NOT NULL DEFAULT 0, PRIMARY KEY (data_id, slot))`,
		`CREATE TABLE datastore.course_records (data_id bigint, slot int, first_pid int, best_pid int, best_score int)`,
		`CREATE TABLE ranking.scores (unique_id bigint, owner_pid numeric(20), category int, score bigint)`,
		`CREATE TABLE ranking.categories (category int PRIMARY KEY, golf_scoring boolean)`,
		`CREATE TABLE mk8.tournaments (id bigserial PRIMARY KEY, owner_pid numeric(20) NOT NULL, name text)`,
		`CREATE TABLE mk8.tournament_scores (tournament_id bigint NOT NULL REFERENCES mk8.tournaments(id) ON DELETE CASCADE, pid numeric(20) NOT NULL, score bigint)`,
		`CREATE TABLE messaging.messages (id bigserial PRIMARY KEY, recipient_id numeric(20), recipient_type bigint, sender_pid numeric(20))`,
		`CREATE TABLE messaging.text_messages (id bigint PRIMARY KEY REFERENCES messaging.messages(id), body text)`,
		`CREATE TABLE matchmaking.gatherings (id bigserial PRIMARY KEY, owner_pid numeric(20), host_pid numeric(20), participants numeric(20)[] NOT NULL DEFAULT '{}')`,
		`CREATE TABLE matchmaking.matchmake_sessions (id bigint PRIMARY KEY REFERENCES matchmaking.gatherings(id), game_mode int)`,
		`CREATE TABLE tracking.join_gathering (date timestamp, source_pid numeric(20), gathering_id bigint)`,
		`CREATE TABLE nexhost.login_names (name text PRIMARY KEY, pid numeric(20,0) NOT NULL)`,
		`CREATE TABLE utility.unique_ids (unique_id bigint, password bigint, associated_pid numeric(20))`,
	} {
		eraseExec(t, db, q)
	}
	// Account A: a public finished course (kept), a private one, an unfinished one, with ratings.
	eraseExec(t, db, `INSERT INTO datastore.objects (data_id, owner_pid, permission, upload_completed) VALUES
		(1, $1, 0, true), (2, $1, 3, true), (3, $2, 0, false), (4, $3, 0, true)`, pidA1, pidA2, pidB)
	eraseExec(t, db, `INSERT INTO datastore.ratings VALUES (1, 0, 5), (2, 0, 6), (3, 0, 7), (4, 0, 8)`)
	// B's object lets A (and B) in by a recipient list.
	eraseExec(t, db, `UPDATE datastore.objects SET permission_recipients = ARRAY[1700000001, 1700000002] WHERE data_id = 4`)
	eraseExec(t, db, `INSERT INTO datastore.course_records VALUES (4, 0, 1700000001, 1700000002, 90), (1, 0, 1700000002, 1700000001, 80)`)
	eraseExec(t, db, `INSERT INTO ranking.scores VALUES (1, $1, 1, 10), (2, $2, 1, 20), (3, $3, 1, 30)`, pidA1, pidA2, pidB)
	eraseExec(t, db, `INSERT INTO ranking.categories VALUES (1, false)`)
	eraseExec(t, db, `INSERT INTO mk8.tournaments (id, owner_pid, name) VALUES (1, $1, 'A cup'), (2, $2, 'B cup')`, pidA1, pidB)
	eraseExec(t, db, `INSERT INTO mk8.tournament_scores VALUES (1, $1, 5), (1, $2, 6), (2, $1, 7), (2, $2, 8)`, pidA1, pidB)
	// A sends B a message; B sends A one; a gathering (type 2) whose id equals A's pid must not match.
	eraseExec(t, db, `INSERT INTO messaging.messages (id, recipient_id, recipient_type, sender_pid) VALUES
		(1, $2, 1, $1), (2, $1, 1, $2), (3, 1700000001, 2, $2), (4, $2, 1, $2)`, pidA1, pidB)
	eraseExec(t, db, `INSERT INTO messaging.text_messages VALUES (1, 'hi'), (2, 'yo'), (3, 'gathering'), (4, 'other')`)
	eraseExec(t, db, `INSERT INTO matchmaking.gatherings (id, owner_pid, host_pid, participants) VALUES
		(1, $1, $1, ARRAY[1700000001, 1700000002]), (2, $2, $2, ARRAY[1700000002, 12297829382472990721])`, pidA1, pidB)
	eraseExec(t, db, `INSERT INTO matchmaking.matchmake_sessions VALUES (1, 3), (2, 3)`)
	eraseExec(t, db, `INSERT INTO tracking.join_gathering VALUES (now(), $1, 1), (now(), $2, 2)`, pidA1, pidB)
	eraseExec(t, db, `INSERT INTO nexhost.login_names VALUES ('a-nsa', $1), ('b-nsa', $2)`, pidA2, pidB)
	eraseExec(t, db, `INSERT INTO utility.unique_ids VALUES (1, 1, $1), (2, 2, $2)`, pidA1, pidB)
}

func TestScrubDatabaseSMM2(t *testing.T) {
	db := eraseDB(t)
	buildErasureSchemas(t, db)
	ctx := context.Background()
	var released []int64
	res, err := scrubDatabase(ctx, db, "super-mario-maker-2", []string{pidA1, pidA2}, func(ids []int64) error {
		released = ids
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows == 0 || len(res.DataIDs) != 2 || len(released) != 2 {
		t.Fatalf("result %+v released %v", res, released)
	}
	// Objects: the public finished course stays, credited to nobody; the rest of A's is gone with its ratings.
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.objects WHERE data_id = 1 AND owner_pid = 0`); n != 1 {
		t.Fatalf("published course must stay, owner 0: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.objects WHERE data_id IN (2,3)`); n != 0 {
		t.Fatalf("private and unfinished courses stay: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.ratings WHERE data_id IN (2,3)`); n != 0 {
		t.Fatalf("ratings of removed courses stay: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.ratings WHERE data_id IN (1,4)`); n != 2 {
		t.Fatalf("ratings of kept courses lost: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.objects WHERE data_id = 4 AND owner_pid = 1700000002 AND permission_recipients = ARRAY[1700000002]::bigint[]`); n != 1 {
		t.Fatal("B's object must lose A from its recipient list and keep its owner")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.course_records WHERE first_pid = 0 AND best_pid = 1700000002 AND data_id = 4`); n != 1 {
		t.Fatal("record holder not anonymised")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM ranking.scores`); n != 1 {
		t.Fatalf("scores: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM ranking.categories`); n != 1 {
		t.Fatal("categories carry no player")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM mk8.tournaments WHERE id = 1 AND owner_pid = 0 AND name = 'A cup'`); n != 1 {
		t.Fatal("tournament must stay, owner 0")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM mk8.tournament_scores WHERE pid = 1700000002`); n != 2 {
		t.Fatalf("B's tournament scores: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM mk8.tournament_scores WHERE tournament_id = 1`); n != 1 {
		t.Fatal("A's tournament score stays")
	}
	// Messages: both directions of A's are gone with their bodies; the gathering message to a gathering id
	// that equals A's pid and B's own message stay.
	if n := eraseCount(t, db, `SELECT count(*) FROM messaging.messages`); n != 2 {
		t.Fatalf("messages: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM messaging.text_messages WHERE id IN (3,4)`); n != 2 {
		t.Fatal("other messages lost their bodies")
	}
	// Matchmaking: A's gathering (and its session) is gone, A leaves B's participants.
	if n := eraseCount(t, db, `SELECT count(*) FROM matchmaking.gatherings`); n != 1 {
		t.Fatalf("gatherings: %d", n)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM matchmaking.gatherings WHERE participants = ARRAY[1700000002]::numeric[]`); n != 1 {
		t.Fatal("A still a participant")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM matchmaking.matchmake_sessions`); n != 1 {
		t.Fatal("session of the removed gathering stays")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM tracking.join_gathering`); n != 1 {
		t.Fatal("tracking log keeps A")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM nexhost.login_names WHERE name = 'a-nsa'`); n != 0 {
		t.Fatal("login name kept")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM utility.unique_ids`); n != 1 {
		t.Fatal("unique id kept")
	}
	// Idempotent.
	res, err = scrubDatabase(ctx, db, "super-mario-maker-2", []string{pidA1, pidA2}, nil)
	if err != nil || res.Rows != 0 || len(res.DataIDs) != 0 {
		t.Fatalf("second run: %+v %v", res, err)
	}
}

func TestScrubDatabaseOtherTitleDeletesPublicObjects(t *testing.T) {
	db := eraseDB(t)
	buildErasureSchemas(t, db)
	res, err := scrubDatabase(context.Background(), db, "mario-kart-8", []string{pidA1, pidA2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DataIDs) != 3 {
		t.Fatalf("ids %v", res.DataIDs)
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM datastore.objects WHERE data_id IN (1,2,3)`); n != 0 {
		t.Fatalf("a title that is not a course maker keeps nothing: %d", n)
	}
}

func TestEraserEndpoint(t *testing.T) {
	e := &Eraser{Title: "t", Key: "secret", PIDs: fakePIDs{eraseA: {}}}
	mux := http.NewServeMux()
	mountErase(mux, e)
	call := func(auth, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/internal/accounts/"+id+"/data", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	if w := call("", eraseA); w.Code != 401 {
		t.Fatalf("no key: %d", w.Code)
	}
	if w := call("Bearer nope", eraseA); w.Code != 401 {
		t.Fatalf("wrong key: %d", w.Code)
	}
	if w := call("Bearer secret", "nope"); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
	w := call("Bearer secret", strings.ToUpper(eraseA))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"service":"nex-t"`) || !strings.Contains(w.Body.String(), `"rows":0`) {
		t.Fatalf("erase: %d %s", w.Code, w.Body.String())
	}
	e.PIDs = nil
	if w := call("Bearer secret", eraseA); w.Code != 502 {
		t.Fatalf("no core must not look like success: %d", w.Code)
	}
	e.Key = ""
	if w := call("Bearer secret", eraseA); w.Code != 503 {
		t.Fatalf("unset key: %d", w.Code)
	}
}

func TestEraserForgetsLoginNamesAndRunsDatabases(t *testing.T) {
	db := eraseDB(t)
	buildErasureSchemas(t, db)
	loginNames.Store("a-nsa", uint64(12297829382472990721))
	loginNames.Store("b-nsa", uint64(1700000002))
	e := &Eraser{Title: "mario-kart-8", PIDs: fakePIDs{eraseA: {pidA1, pidA2}, eraseB: {pidB}},
		DBs: func() ([]*sql.DB, error) { return []*sql.DB{db}, nil }}
	n, err := e.Erase(context.Background(), eraseA)
	if err != nil || n == 0 {
		t.Fatalf("rows=%d err=%v", n, err)
	}
	if _, ok := loginNames.Load("a-nsa"); ok {
		t.Fatal("login name still in memory")
	}
	if _, ok := loginNames.Load("b-nsa"); !ok {
		t.Fatal("other account's login name lost")
	}
	if n := eraseCount(t, db, `SELECT count(*) FROM ranking.scores WHERE owner_pid = 1700000002`); n != 1 {
		t.Fatal("second account's rows touched")
	}
	if n, err := e.Erase(context.Background(), eraseA); err != nil || n != 0 {
		t.Fatalf("second run: rows=%d err=%v", n, err)
	}
}

func TestS3TargetsFromEnv(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "minio:9000")
	t.Setenv("S3_ACCESS_KEY", "ak")
	t.Setenv("S3_SECRET_KEY", "sk")
	t.Setenv("ACNH_S3_BUCKET", "nex-acnh")
	t.Setenv("PN_SSBWIIU_DATASTORE_S3BUCKET", "ssb")
	t.Setenv("PN_SSBWIIU_DATASTORE_S3URL", "https://s3.example")
	t.Setenv("PN_SSBWIIU_DATASTORE_S3KEY", "k2")
	t.Setenv("PN_SSBWIIU_DATASTORE_S3SECRET", "s2")
	got := map[string]S3Target{}
	for _, tg := range s3TargetsFromEnv() {
		got[tg.Bucket] = tg
	}
	if a := got["nex-acnh"]; a.Endpoint != "minio:9000" || a.Access != "ak" || a.Secret != "sk" {
		t.Fatalf("shared credentials not applied: %+v", a)
	}
	if s := got["ssb"]; s.Endpoint != "https://s3.example" || s.Access != "k2" || s.Secret != "s2" {
		t.Fatalf("own credentials lost: %+v", s)
	}
}

func TestDatabaseURLsFromEnv(t *testing.T) {
	t.Setenv("NEX_DATABASE_URL", "postgres://a/x")
	t.Setenv("ACNH_DATABASE_URL", "postgres://a/x")
	t.Setenv("PN_SPLATOON_POSTGRES_URI", "postgres://b/y")
	t.Setenv("REWARDS_EMIT_DATABASE_URL", "postgres://rewards/z")
	t.Setenv("ACNH_TEST_DATABASE_URL", "postgres://test/t")
	t.Setenv("SOMETHING_DATABASE_URL", "not-a-url")
	got := databaseURLsFromEnv()
	if len(got) != 2 || got[0] != "postgres://a/x" || got[1] != "postgres://b/y" {
		t.Fatalf("urls %v", got)
	}
}
