package mailpg

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/eventlog"
)

// PGLog is a Postgres-backed event log implementing eventlog.Log and
// eventlog.Snapshotter.  All events for all accounts share the mail.events
// table; rows are scoped by the account column.
//
// Concurrency: a per-instance mutex serialises Append calls so seq assignment
// is gapless within a single process.  In multi-replica deployments each
// replica carries its own lock; the (account, seq) primary key prevents
// duplicates and a BEGIN … SELECT MAX … FOR UPDATE transaction prevents seq
// races across replicas.
type PGLog struct {
	db      *sql.DB
	account string
	now     func() time.Time
	mu      sync.Mutex
}

// NewPGLog returns a PGLog for the given account.  now defaults to time.Now.
func NewPGLog(db *sql.DB, account string, now func() time.Time) *PGLog {
	if now == nil {
		now = time.Now
	}
	return &PGLog{db: db, account: strings.ToLower(account), now: now}
}

// LogOpener returns a func(dir string) (eventlog.Log, error) that satisfies
// Manager.LogOpen and creates a PGLog for each account directory.  The account
// address is derived from the directory name (the Manager stores accounts under
// data/accounts/<safeName(address)>, where safeName replaces "@" with "_at_").
func LogOpener(db *sql.DB, now func() time.Time) func(string) (eventlog.Log, error) {
	return func(dir string) (eventlog.Log, error) {
		acct := unSafeName(filepath.Base(dir))
		return NewPGLog(db, acct, now), nil
	}
}

// unSafeName reverses the safeName transformation used by the server.Manager
// to derive a directory name from an email address (@ → _at_).  It is correct
// for well-formed email addresses; pathological addresses containing "_at_"
// literally are not supported.
func unSafeName(safe string) string {
	return strings.ReplaceAll(safe, "_at_", "@")
}

// Append assigns the next seq (gapless, monotonically increasing per account),
// durably stores the event, and returns the resulting Record.
func (l *PGLog) Append(ctx context.Context, actor string, e event.Event) (eventlog.Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	encoded, err := event.Encode(e)
	if err != nil {
		return eventlog.Record{}, fmt.Errorf("mailpg eventlog encode: %w", err)
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return eventlog.Record{}, fmt.Errorf("mailpg eventlog begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var maxSeq uint64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM mail.events WHERE account = $1 FOR UPDATE`,
		l.account,
	).Scan(&maxSeq); err != nil {
		return eventlog.Record{}, fmt.Errorf("mailpg eventlog max seq: %w", err)
	}

	r := eventlog.Record{
		Seq:   maxSeq + 1,
		Time:  l.now().UTC(),
		Actor: actor,
		Event: e,
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO mail.events (account, seq, ts, actor, event) VALUES ($1, $2, $3, $4, $5)`,
		l.account, r.Seq, r.Time.UnixNano(), r.Actor, encoded,
	); err != nil {
		return eventlog.Record{}, fmt.Errorf("mailpg eventlog insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return eventlog.Record{}, fmt.Errorf("mailpg eventlog commit: %w", err)
	}
	return r, nil
}

// ReadFrom returns all records with Seq >= seq, ordered by Seq.
func (l *PGLog) ReadFrom(ctx context.Context, seq uint64) ([]eventlog.Record, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT seq, ts, actor, event FROM mail.events WHERE account = $1 AND seq >= $2 ORDER BY seq`,
		l.account, seq,
	)
	if err != nil {
		return nil, fmt.Errorf("mailpg eventlog read: %w", err)
	}
	defer rows.Close()

	var out []eventlog.Record
	for rows.Next() {
		var (
			s     uint64
			ts    int64
			actor string
			blob  []byte
		)
		if err := rows.Scan(&s, &ts, &actor, &blob); err != nil {
			return nil, fmt.Errorf("mailpg eventlog scan: %w", err)
		}
		ev, err := event.Decode(blob)
		if err != nil {
			return nil, fmt.Errorf("mailpg eventlog decode seq %d: %w", s, err)
		}
		out = append(out, eventlog.Record{
			Seq:   s,
			Time:  time.Unix(0, ts).UTC(),
			Actor: actor,
			Event: ev,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mailpg eventlog rows: %w", err)
	}
	return out, nil
}

// Len returns the highest assigned Seq (0 if empty).
func (l *PGLog) Len(ctx context.Context) (uint64, error) {
	var maxSeq uint64
	if err := l.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM mail.events WHERE account = $1`,
		l.account,
	).Scan(&maxSeq); err != nil {
		return 0, fmt.Errorf("mailpg eventlog len: %w", err)
	}
	return maxSeq, nil
}

// LoadSnapshot returns the persisted projection snapshot and the seq it covers,
// or ok=false if none exists.  Implements eventlog.Snapshotter.
func (l *PGLog) LoadSnapshot() (data []byte, throughSeq uint64, ok bool, err error) {
	row := l.db.QueryRowContext(context.Background(),
		`SELECT through_seq, data FROM mail.snapshots WHERE account = $1`,
		l.account,
	)
	var seq uint64
	var b []byte
	if err := row.Scan(&seq, &b); err == sql.ErrNoRows {
		return nil, 0, false, nil
	} else if err != nil {
		return nil, 0, false, fmt.Errorf("mailpg snapshot load: %w", err)
	}
	return b, seq, true, nil
}

// SaveSnapshot durably writes the snapshot covering throughSeq, then deletes
// the log rows with seq <= throughSeq (same compaction semantics as the File
// backend).  Implements eventlog.Snapshotter.
func (l *PGLog) SaveSnapshot(throughSeq uint64, data []byte) error {
	ctx := context.Background()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mailpg snapshot begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO mail.snapshots (account, through_seq, data) VALUES ($1, $2, $3)
		 ON CONFLICT (account) DO UPDATE SET through_seq = EXCLUDED.through_seq, data = EXCLUDED.data`,
		l.account, throughSeq, data,
	); err != nil {
		return fmt.Errorf("mailpg snapshot upsert: %w", err)
	}
	// Compact the log: keep only events that follow the snapshot.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM mail.events WHERE account = $1 AND seq <= $2`,
		l.account, throughSeq,
	); err != nil {
		return fmt.Errorf("mailpg snapshot compact: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mailpg snapshot commit: %w", err)
	}
	return nil
}
