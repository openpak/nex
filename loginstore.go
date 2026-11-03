package nexhost

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// Login names (the NSA id a console logs in under -> its PID) are kept in Postgres, in the
// title's own database, so a restart does not forget them. Pool: one connection, none idle.

var (
	loginMu sync.RWMutex
	loginDB *sql.DB
)

// UseLoginStore points the login-name store at db (a title whose database is not
// cfg.DatabaseURL calls this once). It creates the table if needed.
func UseLoginStore(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, q := range []string{
		`CREATE SCHEMA IF NOT EXISTS nexhost`,
		`CREATE TABLE IF NOT EXISTS nexhost.login_names (name text PRIMARY KEY, pid numeric(20,0) NOT NULL, updated timestamptz NOT NULL DEFAULT now())`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	loginMu.Lock()
	loginDB = db
	loginMu.Unlock()
	return nil
}

func openLoginStore(url string) error {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	if err := UseLoginStore(db); err != nil {
		db.Close()
		return err
	}
	return nil
}

func saveLoginName(name string, pid uint64) {
	loginMu.RLock()
	db := loginDB
	loginMu.RUnlock()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `INSERT INTO nexhost.login_names (name, pid) VALUES ($1, $2::numeric)
		ON CONFLICT (name) DO UPDATE SET pid = EXCLUDED.pid, updated = now()`, name, strconv.FormatUint(pid, 10)); err != nil {
		log.Printf("login name store: %v", err)
	}
}

func loadLoginName(name string) (uint64, bool) {
	loginMu.RLock()
	db := loginDB
	loginMu.RUnlock()
	if db == nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var s string
	if err := db.QueryRowContext(ctx, `SELECT pid::text FROM nexhost.login_names WHERE name = $1`, name).Scan(&s); err != nil {
		if err != sql.ErrNoRows {
			log.Printf("login name lookup: %v", err)
		}
		return 0, false
	}
	pid, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return pid, true
}
