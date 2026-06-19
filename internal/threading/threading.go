// Package threading decides which conversation a message belongs to, using the
// RFC 5322 Message-ID / In-Reply-To / References chain. The decision is made
// once at ingest and recorded in the MessageIngested event (ThreadID), so it is
// part of the log/source-of-truth; the in-memory map here only needs seeding
// from a rebuilt projection on restart.
package threading

import (
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
)

// Threader resolves thread ids. Not safe for concurrent use; the single-writer
// model means one Threader per account-owner.
type Threader struct {
	gen      *ids.Gen
	byHeader map[string]model.ID // Message-ID header -> ThreadID
}

// New returns an empty Threader.
func New(gen *ids.Gen) *Threader {
	return &Threader{gen: gen, byHeader: map[string]model.ID{}}
}

// Seed restores the Message-ID→ThreadID map (e.g. from a rebuilt projection) so
// threading is stable across restarts.
func (t *Threader) Seed(headerToThread map[string]model.ID) {
	for h, tid := range headerToThread {
		if h != "" {
			t.byHeader[h] = tid
		}
	}
}

// Resolve returns the thread id for a message and records its Message-ID.
// A message whose In-Reply-To/References point at a known message joins that
// thread; otherwise a new thread id is minted.
func (t *Threader) Resolve(env model.Envelope) model.ID {
	var tid model.ID
	refs := make([]string, 0, len(env.References)+1)
	refs = append(refs, env.References...)
	if env.InReplyTo != "" {
		refs = append(refs, env.InReplyTo)
	}
	for _, r := range refs {
		if r == "" {
			continue
		}
		if existing, ok := t.byHeader[r]; ok {
			tid = existing
			break
		}
	}
	if tid == "" {
		tid = model.ID(t.gen.New())
	}
	if env.MessageIDHeader != "" {
		t.byHeader[env.MessageIDHeader] = tid
	}
	return tid
}
