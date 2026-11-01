package mk8wiiu

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestYTConfig(t *testing.T) {
	if c, err := loadYTConfig(env(nil)); err != nil || c.Enabled {
		t.Fatalf("unset must be disabled and fine: %+v %v", c, err)
	}
	if _, err := loadYTConfig(env(map[string]string{"MK8_YT_ENABLED": "1"})); err == nil ||
		!strings.Contains(err.Error(), "MK8_YT_CLIENT_ID") {
		t.Fatalf("enabled without credentials must name what is missing: %v", err)
	}
	full := map[string]string{"MK8_YT_ENABLED": "1", "MK8_YT_CLIENT_ID": "a", "MK8_YT_CLIENT_SECRET": "b",
		"MK8_YT_REFRESH_TOKEN": "c", "MK8_YT_SPOOL_DIR": "/x"}
	c, err := loadYTConfig(env(full))
	if err != nil || c.Privacy != "unlisted" || c.DailyLimit != 5 {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	for k, v := range map[string]string{"MK8_YT_PRIVACY": "secret", "MK8_YT_DAILY_UPLOADS": "7"} {
		m := map[string]string{}
		for a, b := range full {
			m[a] = b
		}
		m[k] = v
		if _, err := loadYTConfig(env(m)); err == nil {
			t.Errorf("%s=%s accepted", k, v)
		}
	}
}

// stubGoogle serves the token endpoint and the resumable upload protocol.
func stubGoogle(t *testing.T, quota *atomic.Bool, uploads *atomic.Int32) (*httptest.Server, *ytClient) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Write([]byte(`{"access_token":"at","expires_in":3600}`))
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			w.WriteHeader(401)
			return
		}
		switch r.Method {
		case "POST":
			if quota.Load() {
				w.WriteHeader(403)
				w.Write([]byte(`{"error":{"errors":[{"reason":"quotaExceeded"}]}}`))
				return
			}
			var m struct {
				Status struct{ PrivacyStatus string }
			}
			json.NewDecoder(r.Body).Decode(&m)
			if m.Status.PrivacyStatus != "unlisted" {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Location", srv.URL+"/upload?id=1")
		case "PUT":
			io.Copy(io.Discard, r.Body)
			uploads.Add(1)
			w.Write([]byte(`{"id":"yt` + string(rune('A'+uploads.Load())) + `"}`))
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := ytConfig{ClientID: "c", ClientSecret: "s", RefreshToken: "rt", Privacy: "unlisted"}
	c := newYTClient(cfg)
	c.tokenURL, c.uploadURL = srv.URL+"/token", srv.URL+"/upload"
	return srv, c
}

func TestYTWorker(t *testing.T) {
	dsn := os.Getenv("MK8_TEST_DSN")
	if dsn == "" {
		t.Skip("MK8_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{datastoreSchema, youtubeSchema, `DELETE FROM mk8.mktv_videos`, `DELETE FROM mk8.mktv_consent`, `DELETE FROM mk8.datastore_objects`,
		`INSERT INTO mk8.datastore_objects (data_id, owner_pid) VALUES (101, 1800000001), (102, 1800000002), (103, 1800000001), (104, 1800000001)`,
		`INSERT INTO mk8.mktv_consent (pid, allowed) VALUES (1800000001, true), (1800000002, false)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	for _, n := range []string{"101.mp4", "102.mp4", "100.mp4", "103.mp4", "104.mp4", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("video"), 0o600)
	}
	var quota atomic.Bool
	var uploads atomic.Int32
	_, yt := stubGoogle(t, &quota, &uploads)
	cfg := ytConfig{SpoolDir: dir, Privacy: "unlisted", DailyLimit: 2, MaxBytes: 1 << 20, MaxAttempts: 3}
	w := &ytWorker{cfg: cfg, db: db, yt: yt, now: time.Now}

	if err := w.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := func(id int) (s string, y sql.NullString) {
		db.QueryRow(`SELECT status, youtube_id FROM mk8.mktv_videos WHERE data_id=$1`, id).Scan(&s, &y)
		return
	}
	if s, y := status(101); s != "uploaded" || !y.Valid {
		t.Fatalf("101: %s %v", s, y)
	}
	if s, y := status(102); s != "no_consent" || y.Valid {
		t.Fatalf("102 (no consent): %s %v", s, y)
	}
	if _, err := os.Stat(filepath.Join(dir, "102.mp4")); !os.IsNotExist(err) {
		t.Fatal("non-consenting video was kept")
	}
	if s, _ := status(100); s != "unknown_object" {
		t.Fatalf("100: %s", s)
	}
	// daily limit of 2: 101 and 103 upload, 104 waits
	if uploads.Load() != 2 {
		t.Fatalf("uploads = %d, want the daily limit 2", uploads.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "104.mp4")); err != nil {
		t.Fatal("104 should still be queued")
	}
	if _, err := os.Stat(filepath.Join(dir, "done", "101.mp4")); err != nil {
		t.Fatal("101 not moved to done")
	}

	// next day, quota refusal stops the worker without marking the video failed
	w.now = func() time.Time { return time.Now().Add(24 * time.Hour) }
	quota.Store(true)
	if err := w.runOnce(context.Background()); err != errQuota {
		t.Fatalf("want errQuota, got %v", err)
	}
	if s, _ := status(104); s != "" {
		t.Fatalf("quota refusal recorded as %q", s)
	}
}
