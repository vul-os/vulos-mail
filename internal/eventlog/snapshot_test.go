package eventlog_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/model"
)

// Directly exercise the File log's snapshot + truncate + reopen path (the durable
// compaction surface): SaveSnapshot persists + truncates, LoadSnapshot reads it
// back, the Seq high-water survives a reopen, and Close releases the handle.
func TestFileSnapshotRoundTrip(t *testing.T) {
	ctx := context.Background()
	clk := func() time.Time { return time.Unix(0, 0).UTC() }
	path := filepath.Join(t.TempDir(), "log.jsonl")

	f, err := eventlog.OpenFile(path, clk)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.Append(ctx, "u", event.Labeled{MessageID: model.ID("m"), LabelID: "inbox"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, ok, _ := f.LoadSnapshot(); ok {
		t.Fatal("no snapshot should exist yet")
	}
	if err := f.SaveSnapshot(5, []byte("SNAPDATA")); err != nil {
		t.Fatal(err)
	}
	data, through, ok, err := f.LoadSnapshot()
	if err != nil || !ok || through != 5 || string(data) != "SNAPDATA" {
		t.Fatalf("LoadSnapshot = %q,%d,%v,%v", data, through, ok, err)
	}
	// Log truncated to nothing past the snapshot.
	if recs, _ := f.ReadFrom(ctx, 1); len(recs) != 0 {
		t.Fatalf("after snapshot, log has %d records, want 0", len(recs))
	}
	// Appends continue numbering past the snapshot's Seq, through a reopen.
	r, err := f.Append(ctx, "u", event.Labeled{MessageID: model.ID("m2"), LabelID: "inbox"})
	if err != nil || r.Seq != 6 {
		t.Fatalf("append after snapshot: seq=%d err=%v, want 6", r.Seq, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f2, err := eventlog.OpenFile(path, clk)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	if n, _ := f2.Len(ctx); n != 6 {
		t.Fatalf("reopened Len = %d, want 6 (high-water from snapshot)", n)
	}
	r2, _ := f2.Append(ctx, "u", event.Labeled{MessageID: model.ID("m3"), LabelID: "inbox"})
	if r2.Seq != 7 {
		t.Fatalf("append after reopen: seq=%d, want 7", r2.Seq)
	}
}
