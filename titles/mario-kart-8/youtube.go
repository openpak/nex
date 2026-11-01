package mk8wiiu

// Mario Kart TV videos to YouTube (the worker side).
//
// The console renders a highlight into a video itself and uploads it to YouTube through the
// Wii U system's own YouTube module (see docs/mktv-youtube.md): the NEX server only ever holds
// the replay data, never a video. So this worker does not sit in the game's path. It takes
// finished video files that someone placed in a spool directory (a capture from Quay or a
// console, or a future redirected-upload ingest), checks that the highlight's owner opted in,
// uploads the video with the YouTube Data API v3 to the one OpenPak channel, and records the
// YouTube video id so the Mario Kart TV site can embed it.
//
// It is disabled unless MK8_YT_ENABLED=1, and refuses to start without credentials.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const youtubeSchema = `
CREATE SCHEMA IF NOT EXISTS mk8;
-- A player's opt-in to having their highlights published on the OpenPak YouTube channel.
-- Absent or false means no: their videos are never uploaded.
CREATE TABLE IF NOT EXISTS mk8.mktv_consent (
	pid numeric(20) PRIMARY KEY,
	allowed boolean NOT NULL DEFAULT false,
	updated timestamptz NOT NULL DEFAULT now()
);
-- One row per highlight the worker has handled. youtube_id is set when status = 'uploaded'.
CREATE TABLE IF NOT EXISTS mk8.mktv_videos (
	data_id bigint PRIMARY KEY,
	owner_pid numeric(20) NOT NULL,
	status text NOT NULL CHECK (status IN ('uploaded','failed','no_consent','unknown_object')),
	youtube_id text,
	attempts int NOT NULL DEFAULT 0,
	error text NOT NULL DEFAULT '',
	updated timestamptz NOT NULL DEFAULT now()
);
`

// ytConfig is read from the environment (see the README for the variable list).
type ytConfig struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	RefreshToken string
	SpoolDir     string
	Privacy      string // public, unlisted or private
	DailyLimit   int    // uploads per UTC day; videos.insert costs 1600 of the default 10000 quota units
	MaxBytes     int64  // largest accepted video
	MaxAttempts  int
	SiteURL      string // shown in the description
}

func loadYTConfig(getenv func(string) string) (ytConfig, error) {
	c := ytConfig{
		Enabled:      getenv("MK8_YT_ENABLED") == "1",
		ClientID:     getenv("MK8_YT_CLIENT_ID"),
		ClientSecret: getenv("MK8_YT_CLIENT_SECRET"),
		RefreshToken: getenv("MK8_YT_REFRESH_TOKEN"),
		SpoolDir:     getenv("MK8_YT_SPOOL_DIR"),
		Privacy:      getenv("MK8_YT_PRIVACY"),
		SiteURL:      getenv("MK8_YT_SITE_URL"),
		DailyLimit:   5,
		MaxBytes:     512 << 20,
		MaxAttempts:  3,
	}
	if !c.Enabled {
		return c, nil
	}
	if c.Privacy == "" {
		c.Privacy = "unlisted"
	}
	if c.Privacy != "public" && c.Privacy != "unlisted" && c.Privacy != "private" {
		return c, errors.New("MK8_YT_PRIVACY must be public, unlisted or private")
	}
	if v := getenv("MK8_YT_DAILY_UPLOADS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 6 {
			return c, errors.New("MK8_YT_DAILY_UPLOADS must be 1-6 (the default quota allows 6 uploads a day)")
		}
		c.DailyLimit = n
	}
	var missing []string
	for name, v := range map[string]string{"MK8_YT_CLIENT_ID": c.ClientID, "MK8_YT_CLIENT_SECRET": c.ClientSecret,
		"MK8_YT_REFRESH_TOKEN": c.RefreshToken, "MK8_YT_SPOOL_DIR": c.SpoolDir} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return c, fmt.Errorf("YouTube upload enabled but %s unset", strings.Join(missing, ", "))
	}
	return c, nil
}

// ytClient talks to Google. The base URLs are fields so tests can point them at a stub.
type ytClient struct {
	cfg       ytConfig
	http      *http.Client
	tokenURL  string
	uploadURL string
	token     string
	expires   time.Time
}

func newYTClient(cfg ytConfig) *ytClient {
	return &ytClient{cfg: cfg, http: &http.Client{Timeout: 30 * time.Minute},
		tokenURL:  "https://oauth2.googleapis.com/token",
		uploadURL: "https://www.googleapis.com/upload/youtube/v3/videos"}
}

// accessToken exchanges the refresh token for a short-lived access token (cached).
func (c *ytClient) accessToken(ctx context.Context) (string, error) {
	if c.token != "" && time.Now().Before(c.expires.Add(-time.Minute)) {
		return c.token, nil
	}
	form := url.Values{"client_id": {c.cfg.ClientID}, "client_secret": {c.cfg.ClientSecret},
		"refresh_token": {c.cfg.RefreshToken}, "grant_type": {"refresh_token"}}
	req, _ := http.NewRequestWithContext(ctx, "POST", c.tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&tr); err != nil {
		return "", fmt.Errorf("token response: %w", err)
	}
	if resp.StatusCode != 200 || tr.AccessToken == "" {
		return "", fmt.Errorf("token refresh refused: %d %s", resp.StatusCode, tr.Error)
	}
	c.token, c.expires = tr.AccessToken, time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second)
	return c.token, nil
}

// quotaExceeded marks a Data API quota or rate refusal, after which the worker stops for the day.
var errQuota = errors.New("youtube quota exceeded")

// upload sends one video with the resumable protocol (a metadata POST, then the bytes) and
// returns the new video's id.
func (c *ytClient) upload(ctx context.Context, f *os.File, size int64, title, description string) (string, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}
	meta, _ := json.Marshal(map[string]any{
		"snippet": map[string]any{"title": title, "description": description, "categoryId": "20"},
		"status":  map[string]any{"privacyStatus": c.cfg.Privacy, "selfDeclaredMadeForKids": false, "embeddable": true},
	})
	q := c.uploadURL + "?uploadType=resumable&part=snippet,status"
	req, _ := http.NewRequestWithContext(ctx, "POST", q, bytes.NewReader(meta))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/mp4")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	loc := resp.Header.Get("Location")
	if err := ytStatus(resp); err != nil {
		return "", err
	}
	if loc == "" {
		return "", errors.New("youtube gave no upload location")
	}
	put, _ := http.NewRequestWithContext(ctx, "PUT", loc, f)
	put.ContentLength = size
	put.Header.Set("Authorization", "Bearer "+tok)
	put.Header.Set("Content-Type", "video/mp4")
	resp, err = c.http.Do(put)
	if err != nil {
		return "", err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err := ytStatus(resp); err != nil {
		return "", err
	}
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.ID == "" {
		return "", errors.New("youtube returned no video id")
	}
	return v.ID, nil
}

// ytStatus turns a non-2xx Google response into an error, recognising quota refusals.
func ytStatus(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == 403 && (bytes.Contains(b, []byte("quotaExceeded")) || bytes.Contains(b, []byte("uploadLimitExceeded"))) {
		return errQuota
	}
	return fmt.Errorf("youtube answered %d", resp.StatusCode)
}

// ytWorker moves videos from the spool to YouTube.
type ytWorker struct {
	cfg ytConfig
	db  *sql.DB
	yt  *ytClient
	now func() time.Time
}

var spoolName = regexp.MustCompile(`^([0-9]{1,18})\.mp4$`)

func (w *ytWorker) moveTo(dir, name string) {
	_ = os.MkdirAll(filepath.Join(w.cfg.SpoolDir, dir), 0o750)
	_ = os.Rename(filepath.Join(w.cfg.SpoolDir, name), filepath.Join(w.cfg.SpoolDir, dir, name))
}

func (w *ytWorker) record(id uint64, owner, status, ytID, errText string) {
	_, err := w.db.Exec(`INSERT INTO mk8.mktv_videos (data_id, owner_pid, status, youtube_id, attempts, error)
		VALUES ($1,$2,$3,NULLIF($4,''),1,$5)
		ON CONFLICT (data_id) DO UPDATE SET status=$3, youtube_id=COALESCE(NULLIF($4,''), mk8.mktv_videos.youtube_id),
			attempts=mk8.mktv_videos.attempts+1, error=$5, updated=now()`, int64(id), owner, status, ytID, errText)
	if err != nil {
		log.Printf("[mario-kart-8] youtube: recording %d: %v", id, err)
	}
}

// uploadsToday counts uploads since 00:00 UTC (the quota resets at midnight Pacific; UTC is the
// conservative reading, so a day may allow a few more than the limit says).
func (w *ytWorker) uploadsToday() (int, error) {
	var n int
	start := w.now().UTC().Truncate(24 * time.Hour)
	err := w.db.QueryRow(`SELECT count(*) FROM mk8.mktv_videos WHERE status='uploaded' AND updated >= $1`, start).Scan(&n)
	return n, err
}

// runOnce handles the spooled files in id order. It returns errQuota or the daily limit stop.
func (w *ytWorker) runOnce(ctx context.Context) error {
	entries, err := os.ReadDir(w.cfg.SpoolDir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && spoolName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if n, err := w.uploadsToday(); err != nil {
			return err
		} else if n >= w.cfg.DailyLimit {
			return nil
		}
		id, _ := strconv.ParseUint(spoolName.FindStringSubmatch(name)[1], 10, 64)
		if err := w.handle(ctx, id, name); err != nil {
			if errors.Is(err, errQuota) {
				log.Printf("[mario-kart-8] youtube: quota exhausted, pausing")
			}
			return err
		}
	}
	return nil
}

func (w *ytWorker) handle(ctx context.Context, id uint64, name string) error {
	var owner string
	err := w.db.QueryRow(`SELECT owner_pid FROM mk8.datastore_objects WHERE data_id=$1 AND NOT deleted`, int64(id)).Scan(&owner)
	if err == sql.ErrNoRows {
		w.record(id, "0", "unknown_object", "", "no such highlight")
		w.moveTo("rejected", name)
		return nil
	}
	if err != nil {
		return err
	}
	var allowed bool
	if err := w.db.QueryRow(`SELECT coalesce((SELECT allowed FROM mk8.mktv_consent WHERE pid=$1), false)`, owner).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		// Never keep or publish a non-consenting player's video.
		w.record(id, owner, "no_consent", "", "owner has not opted in")
		_ = os.Remove(filepath.Join(w.cfg.SpoolDir, name))
		return nil
	}
	var done bool
	var attempts int
	_ = w.db.QueryRow(`SELECT status='uploaded', attempts FROM mk8.mktv_videos WHERE data_id=$1`, int64(id)).Scan(&done, &attempts)
	if done {
		w.moveTo("done", name)
		return nil
	}
	if attempts >= w.cfg.MaxAttempts {
		w.moveTo("rejected", name)
		return nil
	}
	path := filepath.Join(w.cfg.SpoolDir, name)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > w.cfg.MaxBytes {
		w.record(id, owner, "failed", "", "video missing, empty or over the size limit")
		w.moveTo("rejected", name)
		return nil
	}
	title := fmt.Sprintf("Mario Kart 8 highlight #%d", id)
	desc := "A Mario Kart TV highlight from OpenPak, the Mario Kart 8 online service."
	if w.cfg.SiteURL != "" {
		desc += "\n" + w.cfg.SiteURL
	}
	ytID, err := w.yt.upload(ctx, f, st.Size(), title, desc)
	if err != nil {
		if !errors.Is(err, errQuota) {
			w.record(id, owner, "failed", "", err.Error())
		}
		return err
	}
	w.record(id, owner, "uploaded", ytID, "")
	w.moveTo("done", name)
	log.Printf("[mario-kart-8] youtube: highlight %d uploaded", id)
	return nil
}

// startYouTube runs the worker when it is enabled. Called once from the secure server's start.
func (s *Server) startYouTube() {
	// The tables exist even while the worker is off, so the website can always join them.
	if _, err := s.db.Exec(youtubeSchema); err != nil {
		log.Printf("[mario-kart-8] youtube: schema: %v", err)
		return
	}
	cfg, err := loadYTConfig(os.Getenv)
	if err != nil {
		log.Printf("[mario-kart-8] youtube: disabled: %v", err)
		return
	}
	if !cfg.Enabled {
		return
	}
	w := &ytWorker{cfg: cfg, db: s.db, yt: newYTClient(cfg), now: time.Now}
	log.Printf("[mario-kart-8] youtube: worker on, spool %s, %d uploads a day, privacy %s", cfg.SpoolDir, cfg.DailyLimit, cfg.Privacy)
	go func() {
		for {
			if err := w.runOnce(context.Background()); err != nil {
				log.Printf("[mario-kart-8] youtube: %v", err)
				if errors.Is(err, errQuota) {
					time.Sleep(6 * time.Hour)
					continue
				}
			}
			time.Sleep(time.Minute)
		}
	}()
}
