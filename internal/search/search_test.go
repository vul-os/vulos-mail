package search_test

import (
	"context"
	"testing"

	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/model"
	"github.com/vul-os/vulos-mail/internal/search"
)

// newStore returns an FS blob store rooted at a temp dir.
func newStore(t *testing.T) *blob.FS {
	t.Helper()
	s, err := blob.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("blob.NewFS: %v", err)
	}
	return s
}

// putMsg stores raw as a blob and returns a Message whose BlobRef lines up with
// the stored content. The envelope is supplied by the caller.
func putMsg(t *testing.T, s *blob.FS, id model.ID, env model.Envelope, raw string) *model.Message {
	t.Helper()
	ref, err := s.Put(context.Background(), []byte(raw))
	if err != nil {
		t.Fatalf("blob.Put: %v", err)
	}
	return &model.Message{
		ID:       id,
		BlobRef:  ref,
		Envelope: env,
		Size:     int64(len(raw)),
	}
}

func ids(msgs []*model.Message) []model.ID {
	out := make([]model.ID, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}

func contains(msgs []*model.Message, id model.ID) bool {
	for _, m := range msgs {
		if m.ID == id {
			return true
		}
	}
	return false
}

func TestSearchBySubject(t *testing.T) {
	s := newStore(t)
	body := "From: a@x\r\nSubject: s\r\n\r\nbody\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{Subject: "Quarterly Report"}, body)
	m2 := putMsg(t, s, "B", model.Envelope{Subject: "Lunch plans"}, body)

	got, err := search.Search(context.Background(), []*model.Message{m1, m2}, s, "report")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "A" {
		t.Errorf("subject search = %v, want [A]", ids(got))
	}
}

func TestSearchByFrom(t *testing.T) {
	s := newStore(t)
	body := "From: a@x\r\nSubject: s\r\n\r\nbody\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{From: []string{"alice@example.com"}}, body)
	m2 := putMsg(t, s, "B", model.Envelope{From: []string{"bob@example.com"}}, body)

	got, err := search.Search(context.Background(), []*model.Message{m1, m2}, s, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "A" {
		t.Errorf("from search = %v, want [A]", ids(got))
	}
}

func TestSearchByToAndCc(t *testing.T) {
	s := newStore(t)
	body := "From: a@x\r\nSubject: s\r\n\r\nbody\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{To: []string{"team@example.com"}}, body)
	m2 := putMsg(t, s, "B", model.Envelope{Cc: []string{"team@example.com"}}, body)
	m3 := putMsg(t, s, "C", model.Envelope{To: []string{"other@example.com"}}, body)

	got, err := search.Search(context.Background(), []*model.Message{m1, m2, m3}, s, "team@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !contains(got, "A") || !contains(got, "B") {
		t.Errorf("to/cc search = %v, want A and B", ids(got))
	}
}

// Body search only triggers when the envelope does NOT match and a store is
// supplied. The query word must appear in the inline body but not the envelope.
func TestSearchByBody(t *testing.T) {
	s := newStore(t)
	raw := "From: a@x\r\nTo: b@x\r\nSubject: hello\r\n\r\nthe quick brown fox\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{Subject: "hello"}, raw)

	got, err := search.Search(context.Background(), []*model.Message{m1}, s, "brown fox")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "A" {
		t.Errorf("body search = %v, want [A]", ids(got))
	}
}

// Without a store, body text is not searched: a query that only matches the body
// returns nothing.
func TestSearchBodyNotMatchedWithoutStore(t *testing.T) {
	s := newStore(t)
	raw := "From: a@x\r\nSubject: hello\r\n\r\nsecret payload\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{Subject: "hello"}, raw)

	got, err := search.Search(context.Background(), []*model.Message{m1}, nil, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("body matched without store: %v", ids(got))
	}
}

func TestSearchNoMatchReturnsEmpty(t *testing.T) {
	s := newStore(t)
	raw := "From: a@x\r\nSubject: hello\r\n\r\nbody text\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{Subject: "hello", From: []string{"a@x"}}, raw)

	got, err := search.Search(context.Background(), []*model.Message{m1}, s, "nonexistentzzz")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", ids(got))
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	s := newStore(t)
	body := "From: a@x\r\nSubject: s\r\n\r\nbody\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{Subject: "Important MEETING Notes"}, body)

	for _, q := range []string{"meeting", "MEETING", "MeEtInG"} {
		got, err := search.Search(context.Background(), []*model.Message{m1}, s, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "A" {
			t.Errorf("query %q = %v, want [A]", q, ids(got))
		}
	}
}

// An empty/whitespace query matches everything (the lister fast-path).
func TestSearchEmptyQueryMatchesAll(t *testing.T) {
	s := newStore(t)
	body := "From: a@x\r\nSubject: s\r\n\r\nbody\r\n"
	m1 := putMsg(t, s, "A", model.Envelope{Subject: "one"}, body)
	m2 := putMsg(t, s, "B", model.Envelope{Subject: "two"}, body)

	for _, q := range []string{"", "   "} {
		got, err := search.Search(context.Background(), []*model.Message{m1, m2}, s, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("query %q matched %d, want 2", q, len(got))
		}
	}
}

// Results are ordered newest-first, i.e. descending by message ID (ULIDs are
// time-sortable).
func TestSearchResultsNewestFirst(t *testing.T) {
	s := newStore(t)
	body := "From: a@x\r\nSubject: s\r\n\r\nbody\r\n"
	// IDs chosen so lexical order is unambiguous: C > B > A.
	mA := putMsg(t, s, "A", model.Envelope{Subject: "meeting"}, body)
	mB := putMsg(t, s, "B", model.Envelope{Subject: "meeting"}, body)
	mC := putMsg(t, s, "C", model.Envelope{Subject: "meeting"}, body)

	// Pass in a deliberately non-sorted order.
	got, err := search.Search(context.Background(), []*model.Message{mA, mC, mB}, s, "meeting")
	if err != nil {
		t.Fatal(err)
	}
	want := []model.ID{"C", "B", "A"}
	if g := ids(got); len(g) != 3 || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] {
		t.Errorf("order = %v, want %v", g, want)
	}
}

// A missing blob (Get returns ErrNotFound) is tolerated: the message is simply
// not matched on body, and Search does not error.
func TestSearchToleratesMissingBlob(t *testing.T) {
	s := newStore(t)
	// Message refers to a blob never stored.
	m1 := &model.Message{
		ID:       "A",
		BlobRef:  blob.Ref([]byte("never stored content")),
		Envelope: model.Envelope{Subject: "subject only"},
	}

	got, err := search.Search(context.Background(), []*model.Message{m1}, s, "content")
	if err != nil {
		t.Fatalf("Search errored on missing blob: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no match, got %v", ids(got))
	}
}
