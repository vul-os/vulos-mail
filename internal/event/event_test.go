package event_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/model"
)

// roundTrip encodes then decodes an event and asserts the decoded value is
// deeply equal to the original. This is the durability invariant for the
// append-only log: replaying an encoded event must reproduce identical state.
func roundTrip(t *testing.T, e event.Event) event.Event {
	t.Helper()
	b, err := event.Encode(e)
	if err != nil {
		t.Fatalf("Encode(%s) error: %v", e.Kind(), err)
	}
	got, err := event.Decode(b)
	if err != nil {
		t.Fatalf("Decode(%s) error: %v", e.Kind(), err)
	}
	if got.Kind() != e.Kind() {
		t.Fatalf("Kind mismatch: got %q want %q", got.Kind(), e.Kind())
	}
	if !reflect.DeepEqual(got, e) {
		t.Fatalf("round-trip mismatch for %s:\n got  %#v\n want %#v", e.Kind(), got, e)
	}
	return got
}

func TestRoundTrip(t *testing.T) {
	// A fully-populated envelope to exercise every nested field. Date must be a
	// concrete location to survive JSON round-trip equality; use UTC.
	env := model.Envelope{
		From:            []string{"alice@example.com", "alice2@example.com"},
		FromName:        "Alice Example",
		To:              []string{"bob@example.com"},
		Cc:              []string{"carol@example.com", "dan@example.com"},
		Subject:         "Hello ☃ unicode & special chars",
		Date:            time.Date(2026, 6, 19, 12, 34, 56, 0, time.UTC),
		MessageIDHeader: "<abc123@example.com>",
		InReplyTo:       "<prev@example.com>",
		References:      []string{"<r1@example.com>", "<r2@example.com>"},
	}

	tests := []struct {
		name string
		ev   event.Event
		kind string
	}{
		{
			name: "MessageIngested",
			kind: "MessageIngested",
			ev: event.MessageIngested{
				MessageID:     model.ID("01J0000000000000000000MSG1"),
				BlobRef:       model.BlobRef("sha256:deadbeef"),
				Envelope:      env,
				Size:          4096,
				ThreadID:      model.ID("01J0000000000000000000THR1"),
				InitialLabels: []model.LabelID{model.LabelInbox, model.LabelImportant},
				InitialFlags:  []model.Flag{model.FlagSeen, model.FlagFlagged},
			},
		},
		{
			name: "MessageIngested_minimal",
			kind: "MessageIngested",
			ev: event.MessageIngested{
				MessageID: model.ID("01J0000000000000000000MSG2"),
				BlobRef:   model.BlobRef("sha256:cafe"),
				Envelope:  model.Envelope{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		},
		{
			name: "Labeled",
			kind: "Labeled",
			ev: event.Labeled{
				MessageID: model.ID("01J0000000000000000000MSG1"),
				LabelID:   model.LabelStar,
			},
		},
		{
			name: "Unlabeled",
			kind: "Unlabeled",
			ev: event.Unlabeled{
				MessageID: model.ID("01J0000000000000000000MSG1"),
				LabelID:   model.LabelSpam,
			},
		},
		{
			name: "FlagSet_true",
			kind: "FlagSet",
			ev: event.FlagSet{
				MessageID: model.ID("01J0000000000000000000MSG1"),
				Flag:      model.FlagSeen,
				Value:     true,
			},
		},
		{
			name: "FlagSet_false",
			kind: "FlagSet",
			ev: event.FlagSet{
				MessageID: model.ID("01J0000000000000000000MSG1"),
				Flag:      model.FlagAnswered,
				Value:     false,
			},
		},
		{
			name: "LabelCreated",
			kind: "LabelCreated",
			ev: event.LabelCreated{
				LabelID:   model.LabelID("lbl-work"),
				Name:      "Work",
				LabelKind: model.LabelUser,
			},
		},
		{
			name: "LabelRenamed",
			kind: "LabelRenamed",
			ev: event.LabelRenamed{
				LabelID: model.LabelID("lbl-work"),
				Name:    "Work Stuff",
			},
		},
		{
			name: "LabelDeleted",
			kind: "LabelDeleted",
			ev: event.LabelDeleted{
				LabelID: model.LabelID("lbl-work"),
			},
		},
		{
			name: "MessageExpunged",
			kind: "MessageExpunged",
			ev: event.MessageExpunged{
				MessageID: model.ID("01J0000000000000000000MSG1"),
			},
		},
		{
			name: "ThreadMerged",
			kind: "ThreadMerged",
			ev: event.ThreadMerged{
				Into: model.ID("01J0000000000000000000THR1"),
				From: model.ID("01J0000000000000000000THR2"),
			},
		},
	}

	seen := map[string]bool{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ev.Kind() != tt.kind {
				t.Fatalf("Kind() = %q, want %q", tt.ev.Kind(), tt.kind)
			}
			roundTrip(t, tt.ev)
		})
		seen[tt.kind] = true
	}

	// Guard: every event Kind handled by Decode must be covered here. If a new
	// event type is added to the codec, this test should be extended.
	wantKinds := []string{
		"MessageIngested", "Labeled", "Unlabeled", "FlagSet",
		"LabelCreated", "LabelRenamed", "LabelDeleted",
		"MessageExpunged", "ThreadMerged",
	}
	for _, k := range wantKinds {
		if !seen[k] {
			t.Errorf("event kind %q is decodable but has no round-trip test case", k)
		}
	}
}

// TestEncodeIsTagged verifies the wire form is a tagged {kind,data} envelope so
// the log format stays stable and self-describing.
func TestEncodeIsTagged(t *testing.T) {
	b, err := event.Encode(event.LabelDeleted{LabelID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatalf("encoded form is not a tagged envelope: %v", err)
	}
	if w.Kind != "LabelDeleted" {
		t.Errorf("kind tag = %q, want LabelDeleted", w.Kind)
	}
	if len(w.Data) == 0 {
		t.Error("data payload is empty")
	}
}

// TestDecodeErrors ensures malformed / unknown input returns an error and never
// panics. The decoder runs over untrusted on-disk bytes; a panic would be fatal.
func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"unknown_tag", []byte(`{"kind":"Nonexistent","data":{}}`)},
		{"empty_tag", []byte(`{"kind":"","data":{}}`)},
		{"garbled_json", []byte(`{not valid json`)},
		{"not_an_object", []byte(`12345`)},
		{"empty_bytes", []byte{}},
		{"null", []byte(`null`)},
		{"bad_data_payload", []byte(`{"kind":"FlagSet","data":"not-an-object"}`)},
		{"wrong_data_type", []byte(`{"kind":"MessageIngested","data":{"Size":"not-a-number"}}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Decode panicked on %q: %v", tt.input, r)
				}
			}()
			ev, err := event.Decode(tt.input)
			if err == nil {
				t.Fatalf("Decode(%q) = %v, want error", tt.input, ev)
			}
			if ev != nil {
				t.Errorf("Decode(%q) returned non-nil event %v alongside error", tt.input, ev)
			}
		})
	}
}
