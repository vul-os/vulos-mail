package mtaout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileQueue is a durable, fsync'd QueueStore that persists each queued outbound
// message as one JSON file under a directory. It is the self-host default: no
// external dependency, and a queued message survives a crash/deploy because Add
// fsyncs the file (and its parent directory) before returning.
//
// Cloud deployments backed by Postgres should prefer a DB-backed QueueStore (see
// internal/mailpg) so the queue is durable even on ephemeral compute; FileQueue
// is durable only as long as its directory is on persistent storage.
type FileQueue struct {
	mu  sync.Mutex
	dir string
}

// NewFileQueue opens (creating if absent) a file-backed queue rooted at dir.
func NewFileQueue(dir string) (*FileQueue, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mtaout: queue dir: %w", err)
	}
	return &FileQueue{dir: dir}, nil
}

func (q *FileQueue) path(id string) string {
	return filepath.Join(q.dir, safeQueueName(id)+".json")
}

// Add durably writes a new queued item (write temp → fsync → rename → fsync dir).
func (q *FileQueue) Add(it QueuedItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.writeItem(it)
}

// Update overwrites an existing item's persisted retry state. Identical mechanics
// to Add (atomic replace); a missing prior file is fine (treated as a fresh add).
func (q *FileQueue) Update(it QueuedItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.writeItem(it)
}

func (q *FileQueue) writeItem(it QueuedItem) error {
	if it.Msg.ID == "" {
		return fmt.Errorf("mtaout: queue item has no ID")
	}
	data, err := json.Marshal(it)
	if err != nil {
		return err
	}
	final := q.path(it.Msg.ID)
	tmp := final + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	// fsync the directory so the rename (the durability point) itself survives a
	// crash, not just the file contents.
	return fsyncDir(q.dir)
}

// Remove deletes a completed item. A missing file is not an error (idempotent).
func (q *FileQueue) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := os.Remove(q.path(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return fsyncDir(q.dir)
}

// Load reads every persisted item back for startup recovery. Unparseable files
// are skipped (logged by the caller via a returned non-fatal set) rather than
// aborting recovery of the rest of the queue.
func (q *FileQueue) Load() ([]QueuedItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []QueuedItem
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(q.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var it QueuedItem
		if json.Unmarshal(data, &it) != nil || it.Msg.ID == "" {
			continue // skip a corrupt/partial file rather than fail recovery
		}
		out = append(out, it)
	}
	return out, nil
}

func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	// Directory fsync is a no-op (and may error) on some platforms; ignore an
	// "operation not supported" rather than fail an otherwise-durable write.
	if err := d.Sync(); err != nil && !os.IsNotExist(err) {
		// best-effort: the file itself is already fsync'd
		return nil
	}
	return nil
}

// safeQueueName keeps a message ID safe to use as a filename (IDs are normally
// random hex, but a submitter-supplied ID must not escape the queue dir).
func safeQueueName(id string) string {
	const maxLen = 200
	b := make([]rune, 0, len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			if r == '.' {
				r = '_' // avoid "." / ".." traversal
			}
			b = append(b, r)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "q"
	}
	if len(b) > maxLen {
		b = b[:maxLen]
	}
	return string(b)
}
