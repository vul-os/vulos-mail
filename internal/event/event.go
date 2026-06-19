// Package event defines the account event types and a stable codec. Every
// mutation to an account is one event; the ordered sequence of events IS the
// account (see docs/DESIGN.md §3). Events are encoded deterministically so a log
// can be hashed, diffed, and replayed to rebuild identical state.
package event

import (
	"encoding/json"
	"fmt"

	"github.com/vul-os/vmail/internal/model"
)

// Event is one account mutation. Implementations are plain value structs.
type Event interface {
	Kind() string
}

// MessageIngested records a newly received/imported message.
type MessageIngested struct {
	MessageID     model.ID
	BlobRef       model.BlobRef
	Envelope      model.Envelope
	Size          int64
	ThreadID      model.ID
	InitialLabels []model.LabelID
	InitialFlags  []model.Flag
}

func (MessageIngested) Kind() string { return "MessageIngested" }

// Labeled / Unlabeled add or remove a label (the many-to-many edge).
type Labeled struct {
	MessageID model.ID
	LabelID   model.LabelID
}

func (Labeled) Kind() string { return "Labeled" }

type Unlabeled struct {
	MessageID model.ID
	LabelID   model.LabelID
}

func (Unlabeled) Kind() string { return "Unlabeled" }

// FlagSet sets or clears a per-message flag.
type FlagSet struct {
	MessageID model.ID
	Flag      model.Flag
	Value     bool
}

func (FlagSet) Kind() string { return "FlagSet" }

// LabelCreated / LabelRenamed / LabelDeleted manage user labels. (Field is named
// LabelKind to avoid colliding with the Kind() method.)
type LabelCreated struct {
	LabelID   model.LabelID
	Name      string
	LabelKind model.LabelKind
}

func (LabelCreated) Kind() string { return "LabelCreated" }

type LabelRenamed struct {
	LabelID model.LabelID
	Name    string
}

func (LabelRenamed) Kind() string { return "LabelRenamed" }

type LabelDeleted struct {
	LabelID model.LabelID
}

func (LabelDeleted) Kind() string { return "LabelDeleted" }

// MessageExpunged tombstones a message. Blob GC is async + refcounted elsewhere.
type MessageExpunged struct {
	MessageID model.ID
}

func (MessageExpunged) Kind() string { return "MessageExpunged" }

// ThreadMerged folds one thread into another (late-arriving references).
type ThreadMerged struct {
	Into model.ID
	From model.ID
}

func (ThreadMerged) Kind() string { return "ThreadMerged" }

// --- codec ---

type wire struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// Encode serializes an event to its tagged wire form.
func Encode(e Event) ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode event %s: %w", e.Kind(), err)
	}
	return json.Marshal(wire{Kind: e.Kind(), Data: data})
}

// Decode parses the tagged wire form back into a concrete value-typed event.
func Decode(b []byte) (Event, error) {
	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("decode event envelope: %w", err)
	}
	switch w.Kind {
	case "MessageIngested":
		return unmarshal[MessageIngested](w.Data)
	case "Labeled":
		return unmarshal[Labeled](w.Data)
	case "Unlabeled":
		return unmarshal[Unlabeled](w.Data)
	case "FlagSet":
		return unmarshal[FlagSet](w.Data)
	case "LabelCreated":
		return unmarshal[LabelCreated](w.Data)
	case "LabelRenamed":
		return unmarshal[LabelRenamed](w.Data)
	case "LabelDeleted":
		return unmarshal[LabelDeleted](w.Data)
	case "MessageExpunged":
		return unmarshal[MessageExpunged](w.Data)
	case "ThreadMerged":
		return unmarshal[ThreadMerged](w.Data)
	default:
		return nil, fmt.Errorf("unknown event kind %q", w.Kind)
	}
}

func unmarshal[T Event](data []byte) (Event, error) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("decode event data: %w", err)
	}
	return v, nil
}
