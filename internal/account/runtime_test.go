package account

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/projection"
)

func msg(messageID, inReplyTo, subject, body string) []byte {
	h := fmt.Sprintf("From: alice@example.com\r\nTo: bob@example.com\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: Mon, 02 Jan 2006 15:04:05 -0700\r\n", subject, messageID)
	if inReplyTo != "" {
		h += fmt.Sprintf("In-Reply-To: <%s>\r\nReferences: <%s>\r\n", inReplyTo, inReplyTo)
	}
	return []byte(h + "\r\n" + body + "\r\n")
}

func newRuntime(t *testing.T) *Runtime {
	t.Helper()
	store, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := Open(context.Background(), log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

// The keystone invariant: the live projection the runtime maintains by folding
// events as they're appended is byte-identical to a fresh rebuild from the log.
func TestLiveProjectionEqualsRebuild(t *testing.T) {
	ctx := context.Background()
	rt := newRuntime(t)

	idA, _ := rt.Ingest(ctx, msg("a@x", "", "Kickoff", "alpha"), []model.LabelID{model.LabelInbox}, nil)
	rt.Ingest(ctx, msg("b@x", "a@x", "Re: Kickoff", "ok"), []model.LabelID{model.LabelInbox}, nil)
	idC, _ := rt.Ingest(ctx, msg("c@x", "", "Invoice", "pay up"), []model.LabelID{model.LabelInbox}, nil)

	if err := rt.CreateLabel(ctx, "work", "Work", model.LabelUser); err != nil {
		t.Fatal(err)
	}
	if err := rt.Label(ctx, idA, "work"); err != nil {
		t.Fatal(err)
	}
	if err := rt.SetFlag(ctx, idA, model.FlagSeen, true); err != nil {
		t.Fatal(err)
	}
	if err := rt.Unlabel(ctx, idA, model.LabelInbox); err != nil {
		t.Fatal(err)
	}
	if err := rt.Expunge(ctx, idC); err != nil {
		t.Fatal(err)
	}

	rebuilt, err := projection.Rebuild(ctx, rt.log)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rt.proj, rebuilt) {
		t.Fatal("live projection drifted from rebuild")
	}
}

// Conversation grouping must survive a restart (re-seed from the rebuilt log).
func TestThreadingSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })

	rt1, _ := Open(ctx, log, store, ids.NewGen(), nil)
	idA, _ := rt1.Ingest(ctx, msg("a@x", "", "Topic", "first"), nil, nil)

	// Reopen from the same log (fresh runtime, threader re-seeded internally).
	rt2, err := Open(ctx, log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	idB, _ := rt2.Ingest(ctx, msg("b@x", "a@x", "Re: Topic", "second"), nil, nil)

	rebuilt, _ := projection.Rebuild(ctx, log)
	if rebuilt.Messages[idA].ThreadID != rebuilt.Messages[idB].ThreadID {
		t.Error("reply after reopen should join the original thread")
	}
}
