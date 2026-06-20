package eventlog

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/vul-os/vulos-mail/internal/event"

	_ "modernc.org/sqlite" // pure-Go, CGO-free driver, registered as "sqlite"
)

// SQLite is a durable, concurrency-safe event log backed by a SQLite database.
// It is a drop-in alternative to the File (JSONL) log, sharing the same event
// codec so records written by either backend decode identically. It implements
// the Log interface.
type SQLite struct {
	mu  sync.Mutex
	db  *sql.DB
	now func() time.Time
}

// OpenSQLite opens (creating if absent) a SQLite-backed event log at path. The
// connection is configured with WAL journaling and a busy timeout for
// concurrency, and the events table is created if it does not yet exist. now
// defaults to time.Now; inject it for deterministic tests.
func OpenSQLite(path string, now func() time.Time) (*SQLite, error) {
	if now == nil {
		now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite %q: %w", p, err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS events (
		seq   INTEGER PRIMARY KEY,
		ts    INTEGER,
		actor TEXT,
		event BLOB
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite create table: %w", err)
	}
	return &SQLite{db: db, now: now}, nil
}

// Append assigns the next Seq (one past the current maximum, gapless and
// monotonic), durably stores the event in its shared codec form, and returns
// the resulting Record.
func (s *SQLite) Append(ctx context.Context, actor string, e event.Event) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var maxSeq uint64
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq), 0) FROM events").Scan(&maxSeq); err != nil {
		return Record{}, fmt.Errorf("sqlite max seq: %w", err)
	}

	r := Record{Seq: maxSeq + 1, Time: s.now().UTC(), Actor: actor, Event: e}
	w, err := toWire(r)
	if err != nil {
		return Record{}, err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO events (seq, ts, actor, event) VALUES (?, ?, ?, ?)",
		r.Seq, r.Time.UnixNano(), r.Actor, []byte(w.Event),
	); err != nil {
		return Record{}, fmt.Errorf("sqlite insert: %w", err)
	}
	return r, nil
}

// ReadFrom returns all records with Seq >= seq, ordered by Seq.
func (s *SQLite) ReadFrom(ctx context.Context, seq uint64) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		"SELECT seq, ts, actor, event FROM events WHERE seq >= ? ORDER BY seq", seq)
	if err != nil {
		return nil, fmt.Errorf("sqlite read: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var (
			rec   uint64
			ts    int64
			actor string
			blob  []byte
		)
		if err := rows.Scan(&rec, &ts, &actor, &blob); err != nil {
			return nil, fmt.Errorf("sqlite scan: %w", err)
		}
		r, err := fromWire(wireRecord{
			Seq:   rec,
			Time:  time.Unix(0, ts).UTC(),
			Actor: actor,
			Event: blob,
		})
		if err != nil {
			return nil, fmt.Errorf("sqlite decode seq %d: %w", rec, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite rows: %w", err)
	}
	return out, nil
}

// Len returns the highest assigned Seq (0 if empty).
func (s *SQLite) Len(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var maxSeq uint64
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq), 0) FROM events").Scan(&maxSeq); err != nil {
		return 0, fmt.Errorf("sqlite len: %w", err)
	}
	return maxSeq, nil
}

// Close releases the underlying database handle.
func (s *SQLite) Close() error {
	return s.db.Close()
}
