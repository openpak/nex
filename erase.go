package nexhost

// Account erasure (card openpak/prds#120).
//
//	DELETE /internal/accounts/{account_id}/data      Authorization: Bearer $ACCOUNT_ERASE_KEY
//
// is served on the title's /healthz listener (NEX_HTTP_ADDR, base+2: the container network, never
// routed by Traefik) by every container. One container is one title with one database, so the
// website calls every title container; each one removes what its own databases and buckets hold for
// the account. Which rows those are is decided by the schema (erase_sql.go), not by a list of
// titles: every table with a PID column is scrubbed, with two documented exceptions. docs/erasure.md
// has the per-table decisions.
//
// The account's PIDs come from the account core (links in the "wiiu", "3ds" and "switch"
// namespaces) and, for a Wii U / 3DS id that was never linked, from nn-account's resolution
// service. Without either configured the endpoint answers 502: erasing "nothing" for an unknown PID
// set would be a lie the website then acts on.

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	accountv1 "github.com/openpak/account-api/openpak/account/v1"
	resolutionv1 "github.com/openpak/nex/internal/resolutionpb"
)

var eraseUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// PIDSource names every NEX PID an account has, as decimal strings (a Switch PID does not fit a
// bigint, so no integer type is used until the database compares them as numeric).
type PIDSource interface {
	PIDs(ctx context.Context, account string) ([]string, error)
}

// corePIDs resolves an account through the account core and nn-account.
type corePIDs struct {
	links accountv1.LinksClient
	res   resolutionv1.ResolutionClient // nil: no nn-account
	coreK string
	resK  string
}

// NewPIDSourceFromEnv dials the account core (NEX_CORE_ADDR + NEX_CORE_KEY; ACCOUNT_GRPC_ADDR +
// ACCOUNT_INTERNAL_KEY are accepted too) and, when set, nn-account's gRPC (NEX_ACCOUNT_GRPC_ADDR +
// NEX_ACCOUNT_GRPC_KEY). nil when the core is not configured.
func NewPIDSourceFromEnv() PIDSource {
	addr, key := env("NEX_CORE_ADDR", os.Getenv("ACCOUNT_GRPC_ADDR")), env("NEX_CORE_KEY", os.Getenv("ACCOUNT_INTERNAL_KEY"))
	if addr == "" || key == "" {
		return nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil
	}
	c := &corePIDs{links: accountv1.NewLinksClient(conn), coreK: key}
	if a, k := os.Getenv("NEX_ACCOUNT_GRPC_ADDR"), os.Getenv("NEX_ACCOUNT_GRPC_KEY"); a != "" {
		if rc, err := grpc.NewClient(a, grpc.WithTransportCredentials(insecure.NewCredentials())); err == nil {
			c.res, c.resK = resolutionv1.NewResolutionClient(rc), k
		}
	}
	return c
}

func (c *corePIDs) PIDs(ctx context.Context, account string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && s != "0" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	lctx := metadata.AppendToOutgoingContext(ctx, "x-internal-key", c.coreK)
	r, err := c.links.ListAccountLinks(lctx, &accountv1.ListAccountLinksRequest{AccountId: account})
	if err != nil {
		return nil, err
	}
	for _, l := range r.GetLinks() {
		subject := strings.TrimSpace(l.GetSubjectId())
		switch l.GetNamespace() {
		case "wiiu", "3ds":
			if n, err := strconv.ParseUint(subject, 10, 64); err == nil {
				add(strconv.FormatUint(n, 10))
			}
		case "switch": // the PID as 16 hex digits
			if n, err := strconv.ParseUint(subject, 16, 64); err == nil {
				add(strconv.FormatUint(n, 10))
			}
		}
	}
	if c.res != nil {
		for _, ns := range []string{"wiiu", "3ds"} {
			rctx := metadata.AppendToOutgoingContext(ctx, "X-API-Key", c.resK)
			rr, err := c.res.ResolveAccount(rctx, &resolutionv1.ResolveAccountRequest{Namespace: ns, AccountId: account})
			if err != nil {
				return nil, err
			}
			if rr.GetFound() {
				add(strconv.FormatUint(uint64(rr.GetPid()), 10))
			}
		}
	}
	return out, nil
}

// Eraser removes an account's data from one title container.
type Eraser struct {
	Title string
	Key   string // ACCOUNT_ERASE_KEY; empty: the route answers 503
	PIDs  PIDSource
	DBs   func() ([]*sql.DB, error) // the title's databases
	S3    []S3Target                // the title's buckets
	// Forget drops in-memory state for the PIDs (optional).
	Forget func(pids []string)

	mu sync.Mutex
}

// Erase removes the account's rows and objects. A second run removes 0.
func (e *Eraser) Erase(ctx context.Context, account string) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.PIDs == nil {
		return 0, errors.New("account core is not configured (NEX_CORE_ADDR/NEX_CORE_KEY)")
	}
	pids, err := e.PIDs.PIDs(ctx, account)
	if err != nil {
		return 0, err
	}
	sort.Strings(pids)
	rows := 0
	if len(pids) > 0 {
		var dbs []*sql.DB
		if e.DBs != nil {
			if dbs, err = e.DBs(); err != nil {
				return 0, err
			}
		}
		for _, db := range dbs {
			res, err := scrubDatabase(ctx, db, e.Title, pids, e.removeBodies)
			if err != nil {
				return 0, err
			}
			rows += res.Rows
		}
		forgetLoginNames(pids)
		if e.Forget != nil {
			e.Forget(pids)
		}
	}
	return rows, nil
}

// removeBodies deletes the bodies of the objects about to be removed from every bucket the
// container knows. It runs before the rows go: a failure leaves them in place for the retry.
func (e *Eraser) removeBodies(ids []int64) error {
	for _, t := range e.S3 {
		if err := t.deleteObjects(context.Background(), ids); err != nil {
			return fmt.Errorf("bucket %s: %w", t.Bucket, err)
		}
	}
	return nil
}

// forgetLoginNames drops the in-memory NSA-id -> PID map entries of the PIDs (the durable rows are
// in nexhost.login_names and go with the other tables).
func forgetLoginNames(pids []string) {
	set := map[uint64]bool{}
	for _, p := range pids {
		if n, err := strconv.ParseUint(p, 10, 64); err == nil {
			set[n] = true
		}
	}
	loginNames.Range(func(k, v any) bool {
		if set[v.(uint64)] {
			loginNames.Delete(k)
		}
		return true
	})
}

// Handler serves the erase route.
func (e *Eraser) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if e == nil || e.Key == "" {
			http.Error(w, "erasure is not configured", http.StatusServiceUnavailable)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), []byte(e.Key)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("account_id")
		if !eraseUUID.MatchString(id) {
			http.Error(w, "account_id must be a UUID", http.StatusBadRequest)
			return
		}
		id = strings.ToLower(id)
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		n, err := e.Erase(ctx, id)
		if err != nil {
			slog.Error("account erase failed", "service", "nex-"+e.Title, "err", err.Error())
			http.Error(w, "erase failed", http.StatusBadGateway)
			return
		}
		slog.Info("account erased", "account", id, "rows", n, "service", "nex-"+e.Title)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"service": "nex-" + e.Title, "rows": n}) //nolint:errcheck
	}
}

// NewEraserFromEnv builds the title's eraser from its environment: ACCOUNT_ERASE_KEY, the core and
// nn-account settings above, every database URL the container carries and every S3 bucket.
func NewEraserFromEnv(title string, dbURLs ...string) *Eraser {
	urls := ownDatabaseURLs(dbURLs)
	e := &Eraser{Title: title, Key: os.Getenv("ACCOUNT_ERASE_KEY"), S3: s3TargetsFromEnv()}
	if e.Key != "" {
		e.PIDs = NewPIDSourceFromEnv()
	}
	var once sync.Once
	var dbs []*sql.DB
	var derr error
	e.DBs = func() ([]*sql.DB, error) {
		once.Do(func() { dbs, derr = openAll(urls) })
		return dbs, derr
	}
	return e
}

// mountErase adds the route to a health mux.
func mountErase(mux *http.ServeMux, e *Eraser) {
	mux.HandleFunc("DELETE /internal/accounts/{account_id}/data", e.Handler())
}

// ownDatabaseURLs is the set of databases one title container erases from: the title's own URL
// (the host's configured one, else NEX_DATABASE_URL, else DATABASE_URL). Only a title that names none
// falls back to scanning the environment. The production containers load the shared deploy .env, which
// carries every other service's URL (account, website, demonware on loopback, ...): scanning that
// reached databases that are not the title's, with no sslmode, and failed every erasure with 502.
func ownDatabaseURLs(configured []string) []string {
	var out []string
	for _, u := range configured {
		if u = strings.TrimSpace(os.ExpandEnv(u)); u != "" {
			out = append(out, u)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, k := range []string{"NEX_DATABASE_URL", "DATABASE_URL"} {
		if u := strings.TrimSpace(os.ExpandEnv(os.Getenv(k))); u != "" {
			return []string{u}
		}
	}
	return databaseURLsFromEnv()
}

var dbEnvName = regexp.MustCompile(`(^|_)(DATABASE_URL|POSTGRES_URI|POSTGRES_URL|DB_URL)$`)

// databaseURLsFromEnv lists the distinct Postgres URLs in the environment, whatever the title calls
// its variable (NEX_DATABASE_URL, ACNH_DATABASE_URL, PN_*_POSTGRES_URI, DATABASE_URL). The rewards
// outbox and test URLs are not the title's data.
func databaseURLsFromEnv() []string {
	var out []string
	seen := map[string]bool{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if !dbEnvName.MatchString(k) || strings.HasPrefix(k, "REWARDS_") || strings.Contains(k, "_TEST_") {
			continue
		}
		v = os.ExpandEnv(v)
		if !strings.HasPrefix(v, "postgres://") && !strings.HasPrefix(v, "postgresql://") && !strings.Contains(v, "host=") {
			continue
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func openAll(urls []string) ([]*sql.DB, error) {
	var out []*sql.DB
	for _, u := range urls {
		db, err := sql.Open("postgres", u)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(0)
		out = append(out, db)
	}
	return out, nil
}
