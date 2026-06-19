package imap_test

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	imapadapter "github.com/vul-os/vmail/adapters/imap"
	"github.com/vul-os/vmail/internal/event"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/model"
)

func newLog() *eventlog.Mem {
	return eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
}

func foldLive(t *testing.T, evs []event.Event) (*eventlog.Mem, *imapadapter.View) {
	t.Helper()
	log := newLog()
	v := imapadapter.NewView()
	ctx := context.Background()
	for _, e := range evs {
		r, err := log.Append(ctx, "t", e)
		if err != nil {
			t.Fatal(err)
		}
		v.Apply(r)
	}
	return log, v
}

// UIDs assigned in order, never reused after retire, UIDNEXT monotonic.
func TestUIDAssignmentAndNoReuse(t *testing.T) {
	evs := []event.Event{
		event.MessageIngested{MessageID: "m1", InitialLabels: []model.LabelID{model.LabelInbox}},
		event.MessageIngested{MessageID: "m2", InitialLabels: []model.LabelID{model.LabelInbox}},
		event.MessageIngested{MessageID: "m3", InitialLabels: []model.LabelID{model.LabelInbox}},
		event.Unlabeled{MessageID: "m2", LabelID: model.LabelInbox}, // retire UID 2
		event.MessageIngested{MessageID: "m4", InitialLabels: []model.LabelID{model.LabelInbox}},
	}
	_, v := foldLive(t, evs)

	u1, _ := v.UIDOf(model.LabelInbox, "m1")
	u3, _ := v.UIDOf(model.LabelInbox, "m3")
	u4, _ := v.UIDOf(model.LabelInbox, "m4")
	if u1 != 1 || u3 != 3 || u4 != 4 {
		t.Fatalf("UIDs = m1:%d m3:%d m4:%d, want 1,3,4", u1, u3, u4)
	}
	if _, ok := v.UIDOf(model.LabelInbox, "m2"); ok {
		t.Error("m2 should be retired from inbox")
	}
	if v.UIDNext(model.LabelInbox) != 5 {
		t.Errorf("UIDNEXT = %d, want 5 (retired UIDs never reused)", v.UIDNext(model.LabelInbox))
	}
	if v.Count(model.LabelInbox) != 3 {
		t.Errorf("Count = %d, want 3", v.Count(model.LabelInbox))
	}
	// Entries must be UID-ascending.
	ents := v.Entries(model.LabelInbox)
	for i := 1; i < len(ents); i++ {
		if ents[i].UID <= ents[i-1].UID {
			t.Fatalf("entries not UID-ascending: %+v", ents)
		}
	}
}

// A message in two labels has independent UIDs per mailbox.
func TestPerMailboxUIDSpaces(t *testing.T) {
	evs := []event.Event{
		event.MessageIngested{MessageID: "a", InitialLabels: []model.LabelID{model.LabelInbox}},
		event.LabelCreated{LabelID: "work", Name: "Work", LabelKind: model.LabelUser},
		event.MessageIngested{MessageID: "b", InitialLabels: []model.LabelID{"work"}},
		event.Labeled{MessageID: "a", LabelID: "work"}, // a enters work after b
	}
	_, v := foldLive(t, evs)

	if uid, _ := v.UIDOf(model.LabelInbox, "a"); uid != 1 {
		t.Errorf("a in inbox UID = %d, want 1", uid)
	}
	if uid, _ := v.UIDOf("work", "b"); uid != 1 {
		t.Errorf("b in work UID = %d, want 1", uid)
	}
	if uid, _ := v.UIDOf("work", "a"); uid != 2 {
		t.Errorf("a in work UID = %d, want 2", uid)
	}
}

// Determinism: folding live == rebuilding from the log, for random sequences.
func TestViewFoldEqualsRebuild(t *testing.T) {
	for seed := int64(0); seed < 48; seed++ {
		r := rand.New(rand.NewSource(seed))
		evs := genEvents(r, 200)
		log, live := foldLive(t, evs)
		rebuilt, err := imapadapter.RebuildView(context.Background(), log)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if !reflect.DeepEqual(live, rebuilt) {
			t.Fatalf("seed %d: live view != rebuild", seed)
		}
	}
}

func genEvents(r *rand.Rand, n int) []event.Event {
	var (
		evs    []event.Event
		msgs   []model.ID
		labels = []model.LabelID{model.LabelInbox, model.LabelStar, model.LabelArchive}
		mc     int
	)
	pick := func() model.ID {
		if len(msgs) == 0 {
			return ""
		}
		return msgs[r.Intn(len(msgs))]
	}
	for i := 0; i < n; i++ {
		switch r.Intn(8) {
		case 0, 1, 2:
			mc++
			id := model.ID(fmt.Sprintf("m%d", mc))
			evs = append(evs, event.MessageIngested{MessageID: id, InitialLabels: []model.LabelID{labels[r.Intn(len(labels))]}})
			msgs = append(msgs, id)
		case 3, 4:
			evs = append(evs, event.Labeled{MessageID: pick(), LabelID: labels[r.Intn(len(labels))]})
		case 5:
			evs = append(evs, event.Unlabeled{MessageID: pick(), LabelID: labels[r.Intn(len(labels))]})
		case 6:
			evs = append(evs, event.MessageExpunged{MessageID: pick()})
		case 7:
			evs = append(evs, event.Labeled{MessageID: pick(), LabelID: labels[r.Intn(len(labels))]})
		}
	}
	return evs
}
