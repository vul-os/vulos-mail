package mailpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/vul-os/vulos-mail/services/mtaout"
)

// PGOutQueue is a Postgres-backed mtaout.QueueStore. It makes the outbound queue
// durable on cloud/Neon deployments where local disk is ephemeral: Add commits a
// row before returning, so a 250-acknowledged message survives a redeploy and is
// recovered into the scheduler on the next boot.
type PGOutQueue struct {
	db *sql.DB
}

// NewPGOutQueue returns a Postgres-backed outbound queue store.
func NewPGOutQueue(db *sql.DB) *PGOutQueue { return &PGOutQueue{db: db} }

// Add durably inserts (or replaces) a queued item. A synchronous COMMIT to
// Postgres is the durability point — it returns only once the row is on stable
// storage, which is the contract the scheduler relies on before sending 250.
func (q *PGOutQueue) Add(it mtaout.QueuedItem) error {
	data, err := json.Marshal(it)
	if err != nil {
		return err
	}
	_, err = q.db.ExecContext(context.Background(),
		`INSERT INTO mail.outqueue (id, item) VALUES ($1, $2)
		 ON CONFLICT (id) DO UPDATE SET item = EXCLUDED.item`,
		it.Msg.ID, data)
	if err != nil {
		return fmt.Errorf("mailpg: outqueue add: %w", err)
	}
	return nil
}

// Update persists a changed retry state (same upsert as Add).
func (q *PGOutQueue) Update(it mtaout.QueuedItem) error { return q.Add(it) }

// Remove deletes a completed item.
func (q *PGOutQueue) Remove(id string) error {
	if _, err := q.db.ExecContext(context.Background(),
		`DELETE FROM mail.outqueue WHERE id = $1`, id); err != nil {
		return fmt.Errorf("mailpg: outqueue remove: %w", err)
	}
	return nil
}

// Load returns every persisted item for startup recovery.
func (q *PGOutQueue) Load() ([]mtaout.QueuedItem, error) {
	rows, err := q.db.QueryContext(context.Background(), `SELECT item FROM mail.outqueue`)
	if err != nil {
		return nil, fmt.Errorf("mailpg: outqueue load: %w", err)
	}
	defer rows.Close()
	var out []mtaout.QueuedItem
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var it mtaout.QueuedItem
		if json.Unmarshal(data, &it) != nil || it.Msg.ID == "" {
			continue
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
