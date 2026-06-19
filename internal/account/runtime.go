// Package account is the single-writer runtime for one account — the keystone
// every protocol adapter (SMTP/IMAP/JMAP/DAV) calls. It owns the event log, blob
// store, id/thread generators, and a *live* projection kept current by applying
// each event as it is appended (fold-as-you-go). The invariant tested here:
// after any sequence of operations, the live projection equals a fresh Rebuild
// from the log — i.e. the live service never drifts from the source of truth.
//
// All mutations serialize through one mutex (the single-writer model). Reads
// snapshot copies so callers can't race a later write through shared maps.
package account

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/event"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/mime"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/projection"
	"github.com/vul-os/vmail/internal/search"
	"github.com/vul-os/vmail/internal/threading"
)

// Runtime is a live, mutable view of one account.
type Runtime struct {
	log      eventlog.Log
	store    blob.Store
	ids      *ids.Gen
	threader *threading.Threader

	mu   sync.RWMutex
	proj *projection.Account

	onChange func(eventlog.Record) // optional push hook; set once via Open opts
}

// Open rebuilds the account from its log and returns a ready runtime. The
// threader is re-seeded from the projection so conversation grouping is stable
// across restarts.
func Open(ctx context.Context, log eventlog.Log, store blob.Store, gen *ids.Gen, onChange func(eventlog.Record)) (*Runtime, error) {
	proj, err := projection.Rebuild(ctx, log)
	if err != nil {
		return nil, err
	}
	th := threading.New(gen)
	seed := make(map[string]model.ID, len(proj.Messages))
	for _, m := range proj.Messages {
		if m.Envelope.MessageIDHeader != "" {
			seed[m.Envelope.MessageIDHeader] = m.ThreadID
		}
	}
	th.Seed(seed)
	return &Runtime{log: log, store: store, ids: gen, threader: th, proj: proj, onChange: onChange}, nil
}

func (r *Runtime) notify(rec eventlog.Record) {
	if r.onChange != nil {
		r.onChange(rec)
	}
}

// Ingest stores the body, decides the thread, appends MessageIngested, and folds
// it into the live projection — the single entry path for new mail.
func (r *Runtime) Ingest(ctx context.Context, raw []byte, labels []model.LabelID, flags []model.Flag) (model.ID, error) {
	env, err := mime.ParseEnvelope(raw)
	if err != nil {
		return "", err
	}
	ref, err := r.store.Put(ctx, raw) // idempotent; safe outside the lock
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	threadID := r.threader.Resolve(env)
	msgID := model.ID(r.ids.New())
	rec, err := r.log.Append(ctx, "ingest", event.MessageIngested{
		MessageID: msgID, BlobRef: ref, Envelope: env, Size: int64(len(raw)),
		ThreadID: threadID, InitialLabels: labels, InitialFlags: flags,
	})
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	r.proj.Apply(rec)
	r.mu.Unlock()

	r.notify(rec)
	return msgID, nil
}

// appendApply is the shared write path for non-ingest mutations.
func (r *Runtime) appendApply(ctx context.Context, actor string, e event.Event) error {
	r.mu.Lock()
	rec, err := r.log.Append(ctx, actor, e)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.proj.Apply(rec)
	r.mu.Unlock()
	r.notify(rec)
	return nil
}

func (r *Runtime) Label(ctx context.Context, msgID model.ID, labelID model.LabelID) error {
	return r.appendApply(ctx, "user", event.Labeled{MessageID: msgID, LabelID: labelID})
}

func (r *Runtime) Unlabel(ctx context.Context, msgID model.ID, labelID model.LabelID) error {
	return r.appendApply(ctx, "user", event.Unlabeled{MessageID: msgID, LabelID: labelID})
}

func (r *Runtime) SetFlag(ctx context.Context, msgID model.ID, flag model.Flag, value bool) error {
	return r.appendApply(ctx, "user", event.FlagSet{MessageID: msgID, Flag: flag, Value: value})
}

func (r *Runtime) CreateLabel(ctx context.Context, labelID model.LabelID, name string, kind model.LabelKind) error {
	return r.appendApply(ctx, "user", event.LabelCreated{LabelID: labelID, Name: name, LabelKind: kind})
}

func (r *Runtime) Expunge(ctx context.Context, msgID model.ID) error {
	return r.appendApply(ctx, "user", event.MessageExpunged{MessageID: msgID})
}

// --- reads (return deep copies; safe to retain after return) ---

// HighestSeq is the account's current MODSEQ basis.
func (r *Runtime) HighestSeq() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.proj.Seq
}

// MessagesWithLabel returns copies of messages carrying labelID, in ingest order.
func (r *Runtime) MessagesWithLabel(labelID model.LabelID) []*model.Message {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return copyAll(r.proj.MessagesWithLabel(labelID))
}

// AllMail returns copies of all non-trash/non-spam messages, in ingest order.
func (r *Runtime) AllMail() []*model.Message {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return copyAll(r.proj.AllMail())
}

// Labels returns copies of all labels.
func (r *Runtime) Labels() []*model.Label {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src := r.proj.LabelList()
	out := make([]*model.Label, len(src))
	for i, l := range src {
		lc := *l
		out[i] = &lc
	}
	return out
}

// Search snapshots the message set under the lock, then matches outside it (so
// blob IO doesn't hold the write lock).
func (r *Runtime) Search(ctx context.Context, query string) ([]*model.Message, error) {
	snap := r.AllMail()
	return search.Search(ctx, snap, r.store, query)
}

// Body returns the raw stored bytes for a message.
func (r *Runtime) Body(ctx context.Context, ref model.BlobRef) ([]byte, error) {
	return r.store.Get(ctx, ref)
}

func copyAll(src []*model.Message) []*model.Message {
	out := make([]*model.Message, len(src))
	for i, m := range src {
		out[i] = copyMessage(m)
	}
	return out
}

func copyMessage(m *model.Message) *model.Message {
	c := *m
	c.Labels = maps.Clone(m.Labels)
	c.Flags = maps.Clone(m.Flags)
	c.Envelope.From = slices.Clone(m.Envelope.From)
	c.Envelope.To = slices.Clone(m.Envelope.To)
	c.Envelope.Cc = slices.Clone(m.Envelope.Cc)
	c.Envelope.References = slices.Clone(m.Envelope.References)
	return &c
}
