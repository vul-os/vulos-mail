// Package search is a placeholder for the ranked FTS + embeddings projection
// described in DESIGN §4. For now it does a case-insensitive substring match
// over envelope fields and (when a blob store is supplied) the extracted inline
// body text. The real implementation — per-account ranked full-text + attachment
// text + embeddings emitted at ingest — replaces the body of Search without
// changing its signature. It operates on a message slice (not a projection) so
// callers can snapshot under a lock and search outside it.
package search

import (
	"context"
	"sort"
	"strings"

	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/model"
)

// Search returns the messages matching query, newest first. If store is non-nil,
// the extracted inline body text is searched too; otherwise only the envelope.
func Search(ctx context.Context, msgs []*model.Message, store blob.Store, query string) ([]*model.Message, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []*model.Message
	for _, m := range msgs {
		if q == "" || matchEnvelope(m, q) {
			out = append(out, m)
			continue
		}
		if store != nil {
			if raw, err := store.Get(ctx, m.BlobRef); err == nil {
				if text, err := mime.ExtractText(raw); err == nil && strings.Contains(strings.ToLower(text), q) {
					out = append(out, m)
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID }) // ULID ≈ newest first
	return out, nil
}

func matchEnvelope(m *model.Message, q string) bool {
	e := m.Envelope
	if strings.Contains(strings.ToLower(e.Subject), q) {
		return true
	}
	for _, group := range [][]string{e.From, e.To, e.Cc} {
		for _, s := range group {
			if strings.Contains(strings.ToLower(s), q) {
				return true
			}
		}
	}
	return false
}
