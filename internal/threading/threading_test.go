package threading_test

import (
	"testing"

	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/model"
	"github.com/vul-os/vulos-mail/internal/threading"
)

func newThreader() *threading.Threader {
	return threading.New(ids.NewGen())
}

// A reply (In-Reply-To pointing at the parent's Message-ID) joins the parent's
// thread.
func TestReplyJoinsParentThread(t *testing.T) {
	th := newThreader()

	parent := th.Resolve(model.Envelope{MessageIDHeader: "<parent@x>"})
	reply := th.Resolve(model.Envelope{
		MessageIDHeader: "<reply@x>",
		InReplyTo:       "<parent@x>",
	})

	if parent == "" {
		t.Fatal("parent thread id is empty")
	}
	if reply != parent {
		t.Errorf("reply thread = %q, want parent %q", reply, parent)
	}
}

// Two messages with no shared references get distinct thread ids.
func TestUnrelatedMessagesDifferentThreads(t *testing.T) {
	th := newThreader()

	a := th.Resolve(model.Envelope{MessageIDHeader: "<a@x>"})
	b := th.Resolve(model.Envelope{MessageIDHeader: "<b@x>"})

	if a == "" || b == "" {
		t.Fatalf("empty thread id: a=%q b=%q", a, b)
	}
	if a == b {
		t.Errorf("unrelated messages share thread %q", a)
	}
}

// A References chain A <- B <- C groups all three into one thread, even though C
// only names A and B via References (and B via In-Reply-To).
func TestReferencesChainGroupsAll(t *testing.T) {
	th := newThreader()

	a := th.Resolve(model.Envelope{MessageIDHeader: "<a@x>"})
	b := th.Resolve(model.Envelope{
		MessageIDHeader: "<b@x>",
		InReplyTo:       "<a@x>",
		References:      []string{"<a@x>"},
	})
	c := th.Resolve(model.Envelope{
		MessageIDHeader: "<c@x>",
		InReplyTo:       "<b@x>",
		References:      []string{"<a@x>", "<b@x>"},
	})

	if a != b || b != c {
		t.Errorf("chain not grouped: a=%q b=%q c=%q", a, b, c)
	}
}

// A message that references a still-unknown parent but shares a known earlier
// reference still lands in the right thread (References scanned before In-Reply-To
// is irrelevant — any known ref wins).
func TestKnownReferenceWinsOverUnknownInReplyTo(t *testing.T) {
	th := newThreader()

	root := th.Resolve(model.Envelope{MessageIDHeader: "<root@x>"})
	// Reply names a parent we've never seen via In-Reply-To, but References the
	// known root. It should still join root's thread.
	reply := th.Resolve(model.Envelope{
		MessageIDHeader: "<late@x>",
		InReplyTo:       "<never-seen@x>",
		References:      []string{"<root@x>"},
	})

	if reply != root {
		t.Errorf("reply thread = %q, want root %q", reply, root)
	}
}

// Seed restores known Message-ID->ThreadID mappings so threading is stable
// across restarts: a reply ingested after a fresh start joins the seeded thread.
func TestSeedRestoresMappings(t *testing.T) {
	known := model.ID("THREAD-PERSISTED")

	th := newThreader()
	th.Seed(map[string]model.ID{"<parent@x>": known})

	reply := th.Resolve(model.Envelope{
		MessageIDHeader: "<reply@x>",
		InReplyTo:       "<parent@x>",
	})
	if reply != known {
		t.Errorf("reply thread = %q, want seeded %q", reply, known)
	}

	// A subsequent reply to the just-ingested reply also stays in the thread.
	reply2 := th.Resolve(model.Envelope{
		MessageIDHeader: "<reply2@x>",
		InReplyTo:       "<reply@x>",
	})
	if reply2 != known {
		t.Errorf("reply2 thread = %q, want seeded %q", reply2, known)
	}
}

// Seed ignores empty header keys (defensive: a projection row with no
// Message-ID must not poison the map).
func TestSeedIgnoresEmptyKey(t *testing.T) {
	th := newThreader()
	th.Seed(map[string]model.ID{"": model.ID("SHOULD-NOT-MATCH")})

	// An envelope with an empty In-Reply-To/References must not match the empty
	// seed entry; it mints a fresh thread.
	got := th.Resolve(model.Envelope{MessageIDHeader: "<m@x>"})
	if got == "SHOULD-NOT-MATCH" {
		t.Errorf("empty seed key was matched: %q", got)
	}
	if got == "" {
		t.Error("expected a freshly minted thread id, got empty")
	}
}

// Resolving the same Message-ID twice returns the same thread id (the second
// resolve sees the recorded mapping; it does not mint a new thread).
func TestResolveStableForSameMessageID(t *testing.T) {
	th := newThreader()

	first := th.Resolve(model.Envelope{MessageIDHeader: "<dup@x>"})
	// Re-resolve referencing itself so the recorded mapping is consulted.
	second := th.Resolve(model.Envelope{
		MessageIDHeader: "<dup@x>",
		References:      []string{"<dup@x>"},
	})

	if first != second {
		t.Errorf("same Message-ID resolved to different threads: %q vs %q", first, second)
	}
}

// A message with no Message-ID header still gets a thread id (it just isn't
// recorded for future replies to find).
func TestNoMessageIDStillMintsThread(t *testing.T) {
	th := newThreader()
	got := th.Resolve(model.Envelope{})
	if got == "" {
		t.Error("expected a minted thread id for header-less message")
	}
}
