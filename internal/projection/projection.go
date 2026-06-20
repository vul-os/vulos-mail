// Package projection folds an account's event log into queryable state
// (messages, labels, threads, flags). The central invariant — tested in
// projection_test.go — is that state == fold(log): applying events in Seq order
// to a fresh Account yields the canonical state, and Rebuild reproduces it
// identically. Projections are therefore disposable and re-shapeable: drop them,
// replay the log, get the same answer.
package projection

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/model"
)

// Account is the folded state of one account.
type Account struct {
	Seq      uint64
	Messages map[model.ID]*model.Message
	Labels   map[model.LabelID]*model.Label
	Threads  map[model.ID]*model.Thread

	order []model.ID // message ingest order, for deterministic listing
}

// New returns a fresh account seeded with the system labels.
func New() *Account {
	a := &Account{
		Messages: map[model.ID]*model.Message{},
		Labels:   map[model.LabelID]*model.Label{},
		Threads:  map[model.ID]*model.Thread{},
	}
	for _, l := range model.SystemLabels() {
		lc := l
		a.Labels[l.ID] = &lc
	}
	return a
}

// snapshot is the serializable form of an Account (includes the unexported
// ingest order). Used by log compaction to avoid replaying the full log.
type snapshot struct {
	Seq      uint64                         `json:"seq"`
	Messages map[model.ID]*model.Message    `json:"messages"`
	Labels   map[model.LabelID]*model.Label `json:"labels"`
	Threads  map[model.ID]*model.Thread     `json:"threads"`
	Order    []model.ID                     `json:"order"`
}

// Snapshot serializes the folded state (so it can be restored without replaying
// the whole log).
func (a *Account) Snapshot() ([]byte, error) {
	return json.Marshal(snapshot{
		Seq: a.Seq, Messages: a.Messages, Labels: a.Labels, Threads: a.Threads, Order: a.order,
	})
}

// Restore rebuilds an Account from a Snapshot. Apply subsequent (Seq > a.Seq)
// records to bring it current.
func Restore(data []byte) (*Account, error) {
	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	a := New()
	a.Seq = s.Seq
	if s.Messages != nil {
		a.Messages = s.Messages
	}
	if s.Labels != nil {
		a.Labels = s.Labels
	}
	if s.Threads != nil {
		a.Threads = s.Threads
	}
	a.order = s.Order
	return a, nil
}

// Apply folds one record into the account. Unknown/duplicate operations are
// handled defensively so replay of any well-formed log converges.
func (a *Account) Apply(r eventlog.Record) {
	// Idempotent on Seq: skip records already folded (e.g. when applying the log
	// tail on top of a restored snapshot, or replaying a not-yet-truncated log).
	if r.Seq != 0 && r.Seq <= a.Seq {
		return
	}
	switch e := r.Event.(type) {
	case event.MessageIngested:
		if _, exists := a.Messages[e.MessageID]; !exists {
			m := &model.Message{
				ID:       e.MessageID,
				BlobRef:  e.BlobRef,
				Envelope: e.Envelope,
				Size:     e.Size,
				ThreadID: e.ThreadID,
				Labels:   map[model.LabelID]bool{},
				Flags:    map[model.Flag]bool{},
			}
			for _, l := range e.InitialLabels {
				m.Labels[l] = true
			}
			for _, f := range e.InitialFlags {
				m.Flags[f] = true
			}
			a.Messages[e.MessageID] = m
			a.order = append(a.order, e.MessageID)
			a.attachToThread(e.ThreadID, e.MessageID)
		}

	case event.Labeled:
		if m := a.Messages[e.MessageID]; m != nil {
			m.Labels[e.LabelID] = true
		}

	case event.Unlabeled:
		if m := a.Messages[e.MessageID]; m != nil {
			delete(m.Labels, e.LabelID)
		}

	case event.FlagSet:
		if m := a.Messages[e.MessageID]; m != nil {
			if e.Value {
				m.Flags[e.Flag] = true
			} else {
				delete(m.Flags, e.Flag)
			}
		}

	case event.LabelCreated:
		if _, exists := a.Labels[e.LabelID]; !exists {
			a.Labels[e.LabelID] = &model.Label{ID: e.LabelID, Name: e.Name, Kind: e.LabelKind}
		}

	case event.LabelRenamed:
		if l := a.Labels[e.LabelID]; l != nil {
			l.Name = e.Name
		}

	case event.LabelDeleted:
		delete(a.Labels, e.LabelID)
		for _, m := range a.Messages {
			delete(m.Labels, e.LabelID)
		}

	case event.MessageExpunged:
		if _, exists := a.Messages[e.MessageID]; exists {
			delete(a.Messages, e.MessageID)
			a.removeFromOrder(e.MessageID)
			a.detachFromThreads(e.MessageID)
		}

	case event.ThreadMerged:
		a.mergeThreads(e.Into, e.From)
	}

	a.Seq = r.Seq
}

func (a *Account) attachToThread(threadID, msgID model.ID) {
	if threadID == "" {
		return
	}
	t := a.Threads[threadID]
	if t == nil {
		t = &model.Thread{ID: threadID}
		a.Threads[threadID] = t
	}
	t.MessageIDs = append(t.MessageIDs, msgID)
}

func (a *Account) detachFromThreads(msgID model.ID) {
	for tid, t := range a.Threads {
		t.MessageIDs = removeID(t.MessageIDs, msgID)
		if len(t.MessageIDs) == 0 {
			delete(a.Threads, tid)
		}
	}
}

func (a *Account) mergeThreads(into, from model.ID) {
	if into == from || into == "" || from == "" {
		return
	}
	src := a.Threads[from]
	if src == nil {
		return
	}
	dst := a.Threads[into]
	if dst == nil {
		dst = &model.Thread{ID: into}
		a.Threads[into] = dst
	}
	for _, mid := range src.MessageIDs {
		dst.MessageIDs = append(dst.MessageIDs, mid)
		if m := a.Messages[mid]; m != nil {
			m.ThreadID = into
		}
	}
	delete(a.Threads, from)
}

func (a *Account) removeFromOrder(id model.ID) {
	for i, x := range a.order {
		if x == id {
			a.order = append(a.order[:i], a.order[i+1:]...)
			return
		}
	}
}

func removeID(ids []model.ID, id model.ID) []model.ID {
	out := ids[:0]
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// Rebuild folds the entire log into a fresh account.
func Rebuild(ctx context.Context, log eventlog.Log) (*Account, error) {
	a := New()
	recs, err := log.ReadFrom(ctx, 1)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		a.Apply(r)
	}
	return a, nil
}

// --- queries (the read API the adapters project onto the wire) ---

// MessagesWithLabel returns messages carrying labelID, in ingest order.
func (a *Account) MessagesWithLabel(labelID model.LabelID) []*model.Message {
	var out []*model.Message
	for _, id := range a.order {
		if m := a.Messages[id]; m != nil && m.Labels[labelID] {
			out = append(out, m)
		}
	}
	return out
}

// AllMail returns every non-trash, non-spam message in ingest order
// (the IMAP "\All Mail" projection).
func (a *Account) AllMail() []*model.Message {
	var out []*model.Message
	for _, id := range a.order {
		m := a.Messages[id]
		if m == nil || m.Labels[model.LabelTrash] || m.Labels[model.LabelSpam] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// LabelList returns labels sorted by id (stable for tests/UI).
func (a *Account) LabelList() []*model.Label {
	out := make([]*model.Label, 0, len(a.Labels))
	for _, l := range a.Labels {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
