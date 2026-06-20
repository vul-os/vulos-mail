package account_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/model"
)

func TestCompactPreservesStateAndShrinksLog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, _ := blob.NewFS(filepath.Join(dir, "blobs"))
	logPath := filepath.Join(dir, "log.jsonl")

	open := func() *account.Runtime {
		lg, err := eventlog.OpenFile(logPath, nil)
		if err != nil {
			t.Fatal(err)
		}
		rt, err := account.Open(ctx, lg, store, ids.NewGen(), nil)
		if err != nil {
			t.Fatal(err)
		}
		return rt
	}

	rt := open()
	for i := 0; i < 20; i++ {
		if _, err := rt.Ingest(ctx, []byte("From: a@b\r\nTo: me@x\r\nSubject: m\r\n\r\nbody\r\n"), []model.LabelID{model.LabelInbox}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Label + flag mutations after ingest, to exercise non-ingest events too.
	inbox := rt.MessagesWithLabel(model.LabelInbox)
	if len(inbox) != 20 {
		t.Fatalf("inbox=%d want 20", len(inbox))
	}
	_ = rt.SetFlag(ctx, inbox[0].ID, model.FlagSeen, true)
	_ = rt.CreateLabel(ctx, "work", "Work", model.LabelUser)
	_ = rt.Label(ctx, inbox[1].ID, "work")

	beforeSeq := rt.HighestSeq()
	if err := rt.Compact(ctx); err != nil {
		t.Fatal(err)
	}

	// The on-disk log is now truncated to nothing past the snapshot.
	lg, _ := eventlog.OpenFile(logPath, nil)
	if n, _ := lg.Len(ctx); n != beforeSeq {
		// Len still reports the high-water seq, but ReadFrom(1) should be empty.
	}
	recs, _ := lg.ReadFrom(ctx, 1)
	if len(recs) != 0 {
		t.Fatalf("after compact, log still has %d records, want 0 (all in snapshot)", len(recs))
	}

	// Reopen from the snapshot (+ empty tail): state must be identical.
	rt2 := open()
	if got := rt2.MessagesWithLabel(model.LabelInbox); len(got) != 20 {
		t.Fatalf("after compact+reopen, inbox=%d want 20", len(got))
	}
	if m, _ := rt2.Message(inbox[0].ID); m == nil || !m.Flags[model.FlagSeen] {
		t.Error("flag $seen lost across compaction")
	}
	if got := rt2.MessagesWithLabel("work"); len(got) != 1 {
		t.Errorf("work label lost across compaction: %d", len(got))
	}

	// Further ingests after compaction continue to work (tail on top of snapshot).
	if _, err := rt2.Ingest(ctx, []byte("From: a@b\r\nTo: me@x\r\nSubject: after\r\n\r\nx\r\n"), []model.LabelID{model.LabelInbox}, nil); err != nil {
		t.Fatal(err)
	}
	rt3 := open()
	if got := rt3.MessagesWithLabel(model.LabelInbox); len(got) != 21 {
		t.Fatalf("after post-compaction ingest+reopen, inbox=%d want 21", len(got))
	}
}
