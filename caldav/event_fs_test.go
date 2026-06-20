package caldav_test

import (
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/caldav"
)

func TestEventRoundTrip(t *testing.T) {
	start := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	ics, err := caldav.BuildEvent(caldav.Event{UID: "e1", Summary: "Standup", Start: start})
	if err != nil {
		t.Fatal(err)
	}
	evs := caldav.ParseEvents(ics)
	if len(evs) != 1 || evs[0].Summary != "Standup" || !evs[0].Start.Equal(start) {
		t.Fatalf("round-trip wrong: %+v", evs)
	}
}

func TestCalFSStorePersists(t *testing.T) {
	dir := t.TempDir()
	s, err := caldav.NewFSStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ics, _ := caldav.BuildEvent(caldav.Event{UID: "e1", Summary: "Lunch", Start: time.Now().UTC()})
	if et := s.Put("alice@vulos.to", "e1.ics", ics); et == "" {
		t.Fatal("Put returned empty etag")
	}
	// Reopen from disk.
	s2, _ := caldav.NewFSStore(dir)
	res := s2.List("alice@vulos.to")
	if len(res) != 1 {
		t.Fatalf("List = %d, want 1", len(res))
	}
	if evs := caldav.ParseEvents(res[0].Data); len(evs) != 1 || evs[0].Summary != "Lunch" {
		t.Errorf("persisted event wrong: %+v", evs)
	}
	if !s2.Delete("alice@vulos.to", "e1.ics") {
		t.Error("Delete should report existed")
	}
}
