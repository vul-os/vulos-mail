// Package eventlog is the per-account append-only log — the source of truth.
// Two implementations: Mem (tests) and File (durable JSONL). A SQLite-backed
// impl is a later drop-in behind the same Log interface; nothing above this
// package knows which backend is in use.
package eventlog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/vul-os/vulos-mail/internal/event"
)

// Record is one appended event with its assigned monotonic Seq (the account's
// MODSEQ basis). Seq starts at 1 and never has gaps.
type Record struct {
	Seq   uint64
	Time  time.Time
	Actor string
	Event event.Event
}

// Log is the append-only event store for one account.
type Log interface {
	// Append assigns the next Seq and durably stores the event.
	Append(ctx context.Context, actor string, e event.Event) (Record, error)
	// ReadFrom returns all records with Seq >= seq, in order.
	ReadFrom(ctx context.Context, seq uint64) ([]Record, error)
	// Len returns the highest assigned Seq (0 if empty).
	Len(ctx context.Context) (uint64, error)
}

// Snapshotter is an optional Log capability: persist a projection snapshot and
// truncate the consumed log prefix, so reopening doesn't replay the full history.
type Snapshotter interface {
	LoadSnapshot() (data []byte, throughSeq uint64, ok bool, err error)
	SaveSnapshot(throughSeq uint64, data []byte) error
}

// wireRecord is the on-disk form: the event is stored in its tagged codec form.
type wireRecord struct {
	Seq   uint64          `json:"seq"`
	Time  time.Time       `json:"time"`
	Actor string          `json:"actor"`
	Event json.RawMessage `json:"event"`
}

func toWire(r Record) (wireRecord, error) {
	eb, err := event.Encode(r.Event)
	if err != nil {
		return wireRecord{}, err
	}
	return wireRecord{Seq: r.Seq, Time: r.Time, Actor: r.Actor, Event: eb}, nil
}

func fromWire(w wireRecord) (Record, error) {
	e, err := event.Decode(w.Event)
	if err != nil {
		return Record{}, err
	}
	return Record{Seq: w.Seq, Time: w.Time, Actor: w.Actor, Event: e}, nil
}

// --- Mem: in-memory log for tests ---

type Mem struct {
	mu   sync.Mutex
	recs []Record
	now  func() time.Time
}

// NewMem returns an in-memory log. now defaults to time.Now; inject for tests.
func NewMem(now func() time.Time) *Mem {
	if now == nil {
		now = time.Now
	}
	return &Mem{now: now}
}

func (m *Mem) Append(_ context.Context, actor string, e event.Event) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := Record{Seq: uint64(len(m.recs)) + 1, Time: m.now().UTC(), Actor: actor, Event: e}
	m.recs = append(m.recs, r)
	return r, nil
}

func (m *Mem) ReadFrom(_ context.Context, seq uint64) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.recs))
	for _, r := range m.recs {
		if r.Seq >= seq {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *Mem) Len(_ context.Context) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return uint64(len(m.recs)), nil
}

// --- File: durable append-only JSONL log ---

type File struct {
	mu   sync.Mutex
	path string
	seq  uint64
	now  func() time.Time
}

// OpenFile opens (or creates on first append) a JSONL log at path, recovering
// the current Seq from the file tail.
func OpenFile(path string, now func() time.Time) (*File, error) {
	if now == nil {
		now = time.Now
	}
	f := &File{path: path, now: now}
	recs, err := f.readAll()
	if err != nil {
		return nil, err
	}
	if n := len(recs); n > 0 {
		f.seq = recs[n-1].Seq
	}
	// A compacted log may have an empty/short tail but a snapshot covering a higher
	// Seq; the high-water mark must come from the snapshot so new Appends continue
	// numbering past it (and aren't skipped as already-folded on replay).
	if _, through, ok, serr := f.LoadSnapshot(); serr == nil && ok && through > f.seq {
		f.seq = through
	}
	return f, nil
}

func (f *File) Append(_ context.Context, actor string, e event.Event) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	r := Record{Seq: f.seq + 1, Time: f.now().UTC(), Actor: actor, Event: e}
	w, err := toWire(r)
	if err != nil {
		return Record{}, err
	}
	line, err := json.Marshal(w)
	if err != nil {
		return Record{}, err
	}

	file, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return Record{}, err
	}
	if err := file.Sync(); err != nil {
		return Record{}, err
	}
	f.seq = r.Seq
	return r, nil
}

func (f *File) ReadFrom(_ context.Context, seq uint64) ([]Record, error) {
	recs, err := f.readAll()
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		if r.Seq >= seq {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *File) Len(_ context.Context) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq, nil
}

// LoadSnapshot returns the persisted projection snapshot and the Seq it covers,
// or ok=false if none exists.
func (f *File) LoadSnapshot() (data []byte, throughSeq uint64, ok bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := os.ReadFile(f.path + ".snap")
	if os.IsNotExist(err) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	var s snapWire
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, 0, false, err
	}
	return s.Data, s.Seq, true, nil
}

// SaveSnapshot durably writes the snapshot covering throughSeq, then truncates
// the log to records with Seq > throughSeq. The snapshot is written and fsynced
// before truncation, so a crash in between leaves the (untruncated) log intact —
// Open replays it and skips already-folded records by Seq.
func (f *File) SaveSnapshot(throughSeq uint64, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	wire, err := json.Marshal(snapWire{Seq: throughSeq, Data: data})
	if err != nil {
		return err
	}
	tmp := f.path + ".snap.tmp"
	if err := os.WriteFile(tmp, wire, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, f.path+".snap"); err != nil {
		return err
	}

	// Rewrite the log keeping only records after the snapshot.
	recs, err := f.readAll()
	if err != nil {
		return err
	}
	var buf []byte
	for _, r := range recs {
		if r.Seq <= throughSeq {
			continue
		}
		w, err := toWire(r)
		if err != nil {
			return err
		}
		line, err := json.Marshal(w)
		if err != nil {
			return err
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	logTmp := f.path + ".tmp"
	if err := os.WriteFile(logTmp, buf, 0o600); err != nil {
		return err
	}
	return os.Rename(logTmp, f.path)
}

type snapWire struct {
	Seq  uint64 `json:"seq"`
	Data []byte `json:"data"`
}

func (f *File) readAll() ([]Record, error) {
	file, err := os.Open(f.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var recs []Record
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large messages
	ln := 0
	for sc.Scan() {
		ln++
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var w wireRecord
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", f.path, ln, err)
		}
		r, err := fromWire(w)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", f.path, ln, err)
		}
		recs = append(recs, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}
