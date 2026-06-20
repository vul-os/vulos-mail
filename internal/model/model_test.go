package model_test

import (
	"reflect"
	"testing"

	"github.com/vul-os/vulos-mail/internal/model"
)

// TestSystemLabelsStable pins the exact set, order, ids, names, and kinds of the
// built-in labels every account starts with. Folders are projections of these,
// so this list is part of the on-disk/wire contract.
func TestSystemLabelsStable(t *testing.T) {
	want := []model.Label{
		{ID: model.LabelInbox, Name: "Inbox", Kind: model.LabelSystem},
		{ID: model.LabelArchive, Name: "Archive", Kind: model.LabelSystem},
		{ID: model.LabelSent, Name: "Sent", Kind: model.LabelSystem},
		{ID: model.LabelDrafts, Name: "Drafts", Kind: model.LabelSystem},
		{ID: model.LabelTrash, Name: "Trash", Kind: model.LabelSystem},
		{ID: model.LabelSpam, Name: "Spam", Kind: model.LabelSystem},
		{ID: model.LabelStar, Name: "Starred", Kind: model.LabelSystem},
		{ID: model.LabelImportant, Name: "Important", Kind: model.LabelSystem},
		{ID: model.LabelSnoozed, Name: "Snoozed", Kind: model.LabelSystem},
	}
	got := model.SystemLabels()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SystemLabels() mismatch:\n got  %#v\n want %#v", got, want)
	}
}

// TestSystemLabelsAllSystemKind ensures none of the built-ins are mislabeled as
// user/category, and that every one carries a non-empty id and name.
func TestSystemLabelsAllSystemKind(t *testing.T) {
	for _, l := range model.SystemLabels() {
		if l.Kind != model.LabelSystem {
			t.Errorf("label %q has kind %q, want %q", l.ID, l.Kind, model.LabelSystem)
		}
		if l.ID == "" {
			t.Errorf("label %q has empty id", l.Name)
		}
		if l.Name == "" {
			t.Errorf("label %q has empty name", l.ID)
		}
	}
}

// TestSystemLabelsUniqueIDs guards against accidental id collisions, which would
// silently merge two distinct system labels.
func TestSystemLabelsUniqueIDs(t *testing.T) {
	seen := map[model.LabelID]bool{}
	for _, l := range model.SystemLabels() {
		if seen[l.ID] {
			t.Errorf("duplicate system label id %q", l.ID)
		}
		seen[l.ID] = true
	}
}

// TestSystemLabelsReturnsFreshSlice verifies callers cannot mutate shared state:
// each call must return an independent slice/values.
func TestSystemLabelsReturnsFreshSlice(t *testing.T) {
	a := model.SystemLabels()
	a[0].Name = "MUTATED"
	b := model.SystemLabels()
	if b[0].Name != "Inbox" {
		t.Fatalf("SystemLabels() leaked shared state: second call sees %q", b[0].Name)
	}
}

// TestFlagConstants pins the wire string values of the per-message flags.
func TestFlagConstants(t *testing.T) {
	cases := map[model.Flag]string{
		model.FlagSeen:     "seen",
		model.FlagAnswered: "answered",
		model.FlagFlagged:  "flagged",
		model.FlagDraft:    "draft",
		model.FlagMDNSent:  "mdn-sent",
	}
	for f, want := range cases {
		if string(f) != want {
			t.Errorf("flag %v = %q, want %q", f, string(f), want)
		}
	}
}

// TestLabelKindConstants pins the LabelKind enum values.
func TestLabelKindConstants(t *testing.T) {
	cases := map[model.LabelKind]string{
		model.LabelSystem:   "system",
		model.LabelUser:     "user",
		model.LabelCategory: "category",
	}
	for k, want := range cases {
		if string(k) != want {
			t.Errorf("label kind %v = %q, want %q", k, string(k), want)
		}
	}
}

// TestLabelIDConstants pins the well-known system label id strings.
func TestLabelIDConstants(t *testing.T) {
	cases := map[model.LabelID]string{
		model.LabelInbox:     "inbox",
		model.LabelArchive:   "archive",
		model.LabelSent:      "sent",
		model.LabelDrafts:    "drafts",
		model.LabelTrash:     "trash",
		model.LabelSpam:      "spam",
		model.LabelStar:      "star",
		model.LabelImportant: "important",
		model.LabelSnoozed:   "snoozed",
	}
	for id, want := range cases {
		if string(id) != want {
			t.Errorf("label id %v = %q, want %q", id, string(id), want)
		}
	}
}

// TestCoreTypeConversions documents that the opaque id/ref types are string
// kinds, convertible to/from string without surprise.
func TestCoreTypeConversions(t *testing.T) {
	if got := model.ID("01J"); string(got) != "01J" {
		t.Errorf("ID conversion = %q", got)
	}
	if got := model.BlobRef("sha256:abc"); string(got) != "sha256:abc" {
		t.Errorf("BlobRef conversion = %q", got)
	}
	if got := model.LabelID("custom"); string(got) != "custom" {
		t.Errorf("LabelID conversion = %q", got)
	}
}
