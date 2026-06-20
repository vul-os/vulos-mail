package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/event"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/model"
)

// fixtureMessage describes one message to ingest into the test log + store.
type fixtureMessage struct {
	id     model.ID
	from   string
	date   time.Time
	body   string
	labels []model.LabelID
	flags  []model.Flag
}

// buildFixture creates an in-memory log + filesystem blob store and ingests the
// given messages, returning the log and store ready for export.
func buildFixture(t *testing.T, msgs []fixtureMessage) (eventlog.Log, blob.Store) {
	t.Helper()
	ctx := context.Background()
	clock := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	log := eventlog.NewMem(func() time.Time { return clock })
	store, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	for _, m := range msgs {
		ref, err := store.Put(ctx, []byte(m.body))
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		ev := event.MessageIngested{
			MessageID: m.id,
			BlobRef:   ref,
			Envelope: model.Envelope{
				From:    []string{m.from},
				Subject: "subject " + string(m.id),
				Date:    m.date,
			},
			Size:          int64(len(m.body)),
			ThreadID:      m.id,
			InitialLabels: m.labels,
			InitialFlags:  m.flags,
		}
		if _, err := log.Append(ctx, "test", ev); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	return log, store
}

func twoMessages() []fixtureMessage {
	return []fixtureMessage{
		{
			id:    "msg-1",
			from:  "alice@example.com",
			date:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			body:  "Subject: Hello\r\n\r\nFrom the desk of Alice.\r\nBody line one.\r\n",
			flags: []model.Flag{model.FlagSeen},
		},
		{
			id:    "msg-2",
			from:  "bob@example.com",
			date:  time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC),
			body:  "Subject: Hi\r\n\r\nReply body from Bob.\r\n",
			flags: []model.Flag{model.FlagSeen, model.FlagAnswered},
		},
	}
}

func TestExportMbox(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		msgs         []fixtureMessage
		wantContains []string
		wantSeps     int
	}{
		{
			name: "two messages with bodies and separators",
			msgs: twoMessages(),
			wantContains: []string{
				"From alice@example.com Fri Jan  2 03:04:05 2026\r\n",
				"From bob@example.com Tue Feb  3 04:05:06 2026\r\n",
				"Body line one.",
				"Reply body from Bob.",
				// "From the desk of Alice." starts with "From " and must be quoted.
				">From the desk of Alice.",
			},
			wantSeps: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log, store := buildFixture(t, tc.msgs)
			var buf bytes.Buffer
			if err := ExportMbox(ctx, log, store, &buf); err != nil {
				t.Fatalf("ExportMbox: %v", err)
			}
			out := buf.String()
			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("mbox output missing %q\n--- output ---\n%s", want, out)
				}
			}
			if got := strings.Count(out, "\r\nFrom ") + boolToInt(strings.HasPrefix(out, "From ")); got != tc.wantSeps {
				t.Errorf("separator count = %d, want %d", got, tc.wantSeps)
			}
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestExportMaildir(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		msgs      []fixtureMessage
		wantFiles int
		wantFlags []string // sorted flag suffixes expected across cur/ files
	}{
		{
			name:      "two messages produce two cur files with flags",
			msgs:      twoMessages(),
			wantFiles: 2,
			wantFlags: []string{":2,RS", ":2,S"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log, store := buildFixture(t, tc.msgs)
			dir := t.TempDir()
			if err := ExportMaildir(ctx, log, store, dir); err != nil {
				t.Fatalf("ExportMaildir: %v", err)
			}
			for _, sub := range []string{"tmp", "new", "cur"} {
				if fi, err := os.Stat(filepath.Join(dir, sub)); err != nil || !fi.IsDir() {
					t.Fatalf("expected maildir subdir %q: err=%v", sub, err)
				}
			}
			entries, err := os.ReadDir(filepath.Join(dir, "cur"))
			if err != nil {
				t.Fatalf("ReadDir cur: %v", err)
			}
			if len(entries) != tc.wantFiles {
				t.Fatalf("cur/ has %d files, want %d", len(entries), tc.wantFiles)
			}

			// File contents must match the original raw bodies.
			wantBodies := map[string]bool{}
			for _, m := range tc.msgs {
				wantBodies[m.body] = true
			}
			var gotFlags []string
			for _, e := range entries {
				data, err := os.ReadFile(filepath.Join(dir, "cur", e.Name()))
				if err != nil {
					t.Fatalf("ReadFile: %v", err)
				}
				if !wantBodies[string(data)] {
					t.Errorf("unexpected file body for %s:\n%s", e.Name(), data)
				}
				if i := strings.Index(e.Name(), ":2,"); i >= 0 {
					gotFlags = append(gotFlags, e.Name()[i:])
				}
			}
			sort.Strings(gotFlags)
			if strings.Join(gotFlags, ",") != strings.Join(tc.wantFlags, ",") {
				t.Errorf("flag suffixes = %v, want %v", gotFlags, tc.wantFlags)
			}
		})
	}
}
