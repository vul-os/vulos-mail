// Package mailpg is the Postgres backing store for the mail index/metadata layer.
//
// When DATABASE_URL (or VULOS_DATABASE_URL) is set the binary uses this package
// to back the event log, per-account settings, and web sessions, all in the
// dedicated "mail" schema of the shared Neon database.  When neither variable
// is set the binary falls back to the existing JSONL/SQLite defaults unchanged —
// single-binary self-hosting stays zero-dependency.
//
// Layout inside Postgres schema "mail":
//
//	mail.events    — per-account event log (source of truth for message index)
//	mail.snapshots — projection checkpoints (compact on demand)
//	mail.settings  — per-account settings incl. home_region, vacation, signature
//	mail.sessions  — webmail session tokens (broker IMAP creds server-side)
//
// Message bodies are NOT moved here; they remain in object storage (Tigris/S3/FS).
package mailpg

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
)

// DSN returns the Postgres data source name from the environment, checking
// VULOS_DATABASE_URL first, then DATABASE_URL.  Returns "" when neither is set.
func DSN() string {
	if v := os.Getenv("VULOS_DATABASE_URL"); v != "" {
		return v
	}
	return os.Getenv("DATABASE_URL")
}

// Open connects to Postgres using dsn (a postgres:// connection string) and
// configures a pool suitable for a mail server: up to 20 open connections with
// a 5-minute idle timeout.  The caller owns the *sql.DB and must Close it.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("mailpg: open: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mailpg: ping: %w", err)
	}
	return db, nil
}
