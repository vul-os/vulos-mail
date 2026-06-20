// Package imap is the IMAP protocol edge. This file implements the UID view: an
// edge projection that folds the account log into per-mailbox IMAP UID spaces.
//
// IMAP requires per-mailbox UIDs that are assigned in increasing order and never
// reused, plus a UIDVALIDITY that is stable across sessions. None of that belongs
// in the domain model (DESIGN §2: "UID/MODSEQ computed at the edge, never
// stored"), so it lives here as a deterministic projection of the same log: given
// the log, the UID assignment is fully determined, so a rebuild reproduces it
// exactly (tested). MODSEQ is the account-wide event Seq, which is monotonic and
// valid for CONDSTORE/QRESYNC.
package imap

import (
	"context"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/model"
)

// DefaultUIDValidity is constant per mailbox here: UIDs are never reassigned, so
// validity never needs to change.
const DefaultUIDValidity uint32 = 1

// Entry is one message's slot in a mailbox's UID space.
type Entry struct {
	UID uint32
	Msg model.ID
}

type mailbox struct {
	validity uint32
	uidNext  uint32 // next UID to assign
	byMsg    map[model.ID]uint32
	order    []Entry // UID-ascending (== assignment order)
}

func newMailbox() *mailbox {
	return &mailbox{validity: DefaultUIDValidity, uidNext: 1, byMsg: map[model.ID]uint32{}}
}

func (mb *mailbox) assign(msg model.ID) {
	if _, ok := mb.byMsg[msg]; ok {
		return
	}
	uid := mb.uidNext
	mb.uidNext++
	mb.byMsg[msg] = uid
	mb.order = append(mb.order, Entry{UID: uid, Msg: msg})
}

func (mb *mailbox) retire(msg model.ID) {
	if _, ok := mb.byMsg[msg]; !ok {
		return
	}
	delete(mb.byMsg, msg)
	for i, e := range mb.order {
		if e.Msg == msg {
			mb.order = append(mb.order[:i], mb.order[i+1:]...)
			break
		}
	}
	// uidNext is NOT decremented: retired UIDs are never reused.
}

// View is the folded UID state for an account, keyed by label (= IMAP mailbox).
type View struct {
	Seq      uint64
	mailboxes map[model.LabelID]*mailbox
}

// NewView returns an empty view.
func NewView() *View {
	return &View{mailboxes: map[model.LabelID]*mailbox{}}
}

func (v *View) mbox(label model.LabelID) *mailbox {
	mb := v.mailboxes[label]
	if mb == nil {
		mb = newMailbox()
		v.mailboxes[label] = mb
	}
	return mb
}

// Apply folds one record into the UID view.
func (v *View) Apply(rec eventlog.Record) {
	switch e := rec.Event.(type) {
	case event.MessageIngested:
		for _, l := range e.InitialLabels {
			v.mbox(l).assign(e.MessageID)
		}
	case event.Labeled:
		v.mbox(e.LabelID).assign(e.MessageID)
	case event.Unlabeled:
		if mb := v.mailboxes[e.LabelID]; mb != nil {
			mb.retire(e.MessageID)
		}
	case event.MessageExpunged:
		for _, mb := range v.mailboxes {
			mb.retire(e.MessageID)
		}
	case event.LabelDeleted:
		delete(v.mailboxes, e.LabelID)
	}
	v.Seq = rec.Seq
}

// RebuildView folds an entire log into a fresh view.
func RebuildView(ctx context.Context, log eventlog.Log) (*View, error) {
	v := NewView()
	recs, err := log.ReadFrom(ctx, 1)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		v.Apply(r)
	}
	return v, nil
}

// ViewFrom folds an already-read slice of records into a fresh view.
func ViewFrom(recs []eventlog.Record) *View {
	v := NewView()
	for _, r := range recs {
		v.Apply(r)
	}
	return v
}

// --- queries the IMAP Session adapter uses ---

// UIDValidity returns the mailbox's UIDVALIDITY (DefaultUIDValidity, or the
// constant for an as-yet-unseen mailbox).
func (v *View) UIDValidity(label model.LabelID) uint32 {
	if mb := v.mailboxes[label]; mb != nil {
		return mb.validity
	}
	return DefaultUIDValidity
}

// UIDNext returns the mailbox's UIDNEXT.
func (v *View) UIDNext(label model.LabelID) uint32 {
	if mb := v.mailboxes[label]; mb != nil {
		return mb.uidNext
	}
	return 1
}

// Count returns the number of messages currently in the mailbox.
func (v *View) Count(label model.LabelID) int {
	if mb := v.mailboxes[label]; mb != nil {
		return len(mb.order)
	}
	return 0
}

// Entries returns the mailbox's (UID, message) slots in UID-ascending order.
func (v *View) Entries(label model.LabelID) []Entry {
	if mb := v.mailboxes[label]; mb != nil {
		out := make([]Entry, len(mb.order))
		copy(out, mb.order)
		return out
	}
	return nil
}

// UIDOf returns the message's UID in the mailbox (0, false if absent).
func (v *View) UIDOf(label model.LabelID, msg model.ID) (uint32, bool) {
	if mb := v.mailboxes[label]; mb != nil {
		uid, ok := mb.byMsg[msg]
		return uid, ok
	}
	return 0, false
}
