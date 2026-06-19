package projection_test

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/event"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/projection"
)

func fixedNow() time.Time { return time.Unix(0, 0).UTC() }

// buildLog appends evs to a fresh Mem log while incrementally folding them into
// an Account, returning both so callers can compare fold-as-you-go vs Rebuild.
func buildLog(t *testing.T, evs []event.Event) (*eventlog.Mem, *projection.Account) {
	t.Helper()
	log := eventlog.NewMem(fixedNow)
	a := projection.New()
	ctx := context.Background()
	for _, e := range evs {
		r, err := log.Append(ctx, "test", e)
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		a.Apply(r)
	}
	return log, a
}

// The central invariant: folding events as they're appended yields exactly the
// same state as rebuilding from the log afterwards.
func TestFoldEqualsRebuild(t *testing.T) {
	evs := []event.Event{
		event.MessageIngested{MessageID: "m1", BlobRef: "sha256:a", ThreadID: "t1", InitialLabels: []model.LabelID{model.LabelInbox}},
		event.MessageIngested{MessageID: "m2", BlobRef: "sha256:b", ThreadID: "t1"},
		event.Labeled{MessageID: "m2", LabelID: model.LabelStar},
		event.FlagSet{MessageID: "m1", Flag: model.FlagSeen, Value: true},
		event.LabelCreated{LabelID: "work", Name: "Work", LabelKind: model.LabelUser},
		event.Labeled{MessageID: "m1", LabelID: "work"},
		event.Unlabeled{MessageID: "m1", LabelID: model.LabelInbox},
	}
	log, folded := buildLog(t, evs)

	rebuilt, err := projection.Rebuild(context.Background(), log)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if !reflect.DeepEqual(folded, rebuilt) {
		t.Fatalf("fold != rebuild\n folded=%+v\nrebuilt=%+v", folded, rebuilt)
	}

	if folded.Seq != uint64(len(evs)) {
		t.Errorf("Seq = %d, want %d", folded.Seq, len(evs))
	}
	m1 := folded.Messages["m1"]
	if m1 == nil {
		t.Fatal("m1 missing")
	}
	if m1.Labels[model.LabelInbox] {
		t.Error("m1 should have been unlabeled from inbox")
	}
	if !m1.Labels["work"] {
		t.Error("m1 should have label work")
	}
	if !m1.Flags[model.FlagSeen] {
		t.Error("m1 should be seen")
	}
	if got := folded.Threads["t1"]; got == nil || len(got.MessageIDs) != 2 {
		t.Errorf("thread t1 should hold 2 messages, got %+v", got)
	}
}

func TestReplayDeterministic(t *testing.T) {
	evs := []event.Event{
		event.MessageIngested{MessageID: "m1", BlobRef: "sha256:a", ThreadID: "t1"},
		event.Labeled{MessageID: "m1", LabelID: model.LabelStar},
		event.MessageIngested{MessageID: "m2", BlobRef: "sha256:b", ThreadID: "t2"},
	}
	log, _ := buildLog(t, evs)
	a, err := projection.Rebuild(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}
	b, err := projection.Rebuild(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("rebuild is non-deterministic")
	}
}

func TestExpungeAndThreadMerge(t *testing.T) {
	evs := []event.Event{
		event.MessageIngested{MessageID: "m1", BlobRef: "sha256:a", ThreadID: "t1"},
		event.MessageIngested{MessageID: "m2", BlobRef: "sha256:b", ThreadID: "t2"},
		event.ThreadMerged{Into: "t1", From: "t2"},
		event.MessageExpunged{MessageID: "m1"},
	}
	_, a := buildLog(t, evs)

	if _, exists := a.Messages["m1"]; exists {
		t.Error("m1 should be expunged")
	}
	if _, exists := a.Threads["t2"]; exists {
		t.Error("t2 should have merged into t1")
	}
	t1 := a.Threads["t1"]
	if t1 == nil || len(t1.MessageIDs) != 1 || t1.MessageIDs[0] != "m2" {
		t.Errorf("t1 should hold only m2 after merge+expunge, got %+v", t1)
	}
	if m2 := a.Messages["m2"]; m2 == nil || m2.ThreadID != "t1" {
		t.Errorf("m2.ThreadID should be t1, got %+v", m2)
	}
	if got := a.MessagesWithLabel(model.LabelInbox); len(got) != 0 {
		t.Errorf("no inbox messages expected, got %d", len(got))
	}
}

// Property test: for many random valid event sequences, fold == rebuild.
func TestRandomSequencesFoldEqualsRebuild(t *testing.T) {
	for seed := int64(0); seed < 64; seed++ {
		r := rand.New(rand.NewSource(seed))
		evs := genEvents(r, 250)
		log, folded := buildLog(t, evs)
		rebuilt, err := projection.Rebuild(context.Background(), log)
		if err != nil {
			t.Fatalf("seed %d: rebuild: %v", seed, err)
		}
		if !reflect.DeepEqual(folded, rebuilt) {
			t.Fatalf("seed %d: fold != rebuild", seed)
		}
	}
}

// genEvents produces a random but mostly-valid event stream, referencing live
// message/label ids so real state is exercised. Operations against missing ids
// are harmless (Apply is defensive and identical across fold/rebuild).
func genEvents(r *rand.Rand, n int) []event.Event {
	var (
		evs     []event.Event
		msgs    []model.ID
		userLbl []model.LabelID
		mc, lc  int
	)
	sysLabels := []model.LabelID{model.LabelInbox, model.LabelStar, model.LabelImportant, model.LabelArchive, model.LabelTrash}

	pickMsg := func() model.ID {
		if len(msgs) == 0 {
			return ""
		}
		return msgs[r.Intn(len(msgs))]
	}
	pickLabel := func() model.LabelID {
		all := append([]model.LabelID{}, sysLabels...)
		all = append(all, userLbl...)
		return all[r.Intn(len(all))]
	}

	for i := 0; i < n; i++ {
		switch r.Intn(10) {
		case 0, 1, 2: // ingest
			mc++
			id := model.ID(fmt.Sprintf("m%d", mc))
			thread := model.ID(fmt.Sprintf("t%d", r.Intn(mc/3+1)))
			evs = append(evs, event.MessageIngested{
				MessageID: id, BlobRef: model.BlobRef(fmt.Sprintf("sha256:%d", mc)),
				ThreadID: thread, InitialLabels: []model.LabelID{model.LabelInbox},
			})
			msgs = append(msgs, id)
		case 3, 4: // label
			evs = append(evs, event.Labeled{MessageID: pickMsg(), LabelID: pickLabel()})
		case 5: // unlabel
			evs = append(evs, event.Unlabeled{MessageID: pickMsg(), LabelID: pickLabel()})
		case 6: // flag
			evs = append(evs, event.FlagSet{MessageID: pickMsg(), Flag: model.FlagSeen, Value: r.Intn(2) == 0})
		case 7: // create user label
			lc++
			id := model.LabelID(fmt.Sprintf("u%d", lc))
			userLbl = append(userLbl, id)
			evs = append(evs, event.LabelCreated{LabelID: id, Name: fmt.Sprintf("L%d", lc), LabelKind: model.LabelUser})
		case 8: // expunge
			evs = append(evs, event.MessageExpunged{MessageID: pickMsg()})
		case 9: // thread merge
			evs = append(evs, event.ThreadMerged{
				Into: model.ID(fmt.Sprintf("t%d", r.Intn(mc+1))),
				From: model.ID(fmt.Sprintf("t%d", r.Intn(mc+1))),
			})
		}
	}
	return evs
}
