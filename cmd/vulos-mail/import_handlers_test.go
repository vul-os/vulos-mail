package main

// import_handlers_test.go — Tests for the broker-gated bulk PIM import endpoints.
//
// Both handlers are exercised with:
//   - Missing / wrong broker auth → 401
//   - Malformed / missing account → 400
//   - Valid batch → 202 with counts
//   - Re-submit same UID (idempotent) → stored copy overwritten, same 202 result

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/caldav"
	"github.com/vul-os/vulos-mail/carddav"
)

const importTestSecret = "import-broker-secret-abc"

// testContactStore builds an in-memory CardDAV store for import handler tests.
func testContactStore() carddav.Store { return carddav.NewMemStore() }

// testCalStore builds an in-memory CalDAV store for import handler tests.
func testCalStore() caldav.Store { return caldav.NewMemStore() }

// doImport is a helper that sends a POST request to the given handler and
// returns the response recorder.
func doImport(h http.Handler, target string, body any, secret string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	if secret != "" {
		r.Header.Set("X-Vulos-Broker-Auth", secret)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// ── /api/admin/import/contacts ────────────────────────────────────────────────

func TestImportContactsAuth(t *testing.T) {
	h := importContactsHandler(testContactStore(), importTestSecret)

	// No auth header → 401.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/import/contacts", strings.NewReader(`{}`))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: got %d, want 401", w.Code)
	}

	// Wrong secret → 401.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	r2.Header.Set("X-Vulos-Broker-Auth", "wrong")
	h.ServeHTTP(w2, r2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("wrong auth: got %d, want 401", w2.Code)
	}

	// Wrong method (GET) → 405.
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.Header.Set("X-Vulos-Broker-Auth", importTestSecret)
	h.ServeHTTP(w3, r3)
	if w3.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: got %d, want 405", w3.Code)
	}
}

func TestImportContactsMissingAccount(t *testing.T) {
	h := importContactsHandler(testContactStore(), importTestSecret)
	w := doImport(h, "/", map[string]any{"vcards": []string{"BEGIN:VCARD\r\nEND:VCARD\r\n"}}, importTestSecret)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing account: got %d, want 400", w.Code)
	}
}

func TestImportContactsBulk(t *testing.T) {
	store := testContactStore()
	h := importContactsHandler(store, importTestSecret)

	vcard1 := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-alice\r\nFN:Alice\r\nEMAIL:alice@test.io\r\nEND:VCARD\r\n"
	vcard2 := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-bob\r\nFN:Bob\r\nEMAIL:bob@test.io\r\nEND:VCARD\r\n"
	noUID := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:NoUID\r\nEND:VCARD\r\n"

	w := doImport(h, "/", map[string]any{
		"account": "test@vulos.to",
		"vcards":  []string{vcard1, vcard2, noUID},
	}, importTestSecret)

	if w.Code != http.StatusAccepted {
		t.Fatalf("bulk: got %d, want 202; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]int
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["imported"] != 2 {
		t.Errorf("imported = %d, want 2", resp["imported"])
	}
	if resp["errors"] != 1 {
		t.Errorf("errors = %d, want 1 (no-UID vCard)", resp["errors"])
	}

	// Verify contacts are in the store.
	resources, err := store.List("test@vulos.to")
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("store has %d resources, want 2", len(resources))
	}
}

func TestImportContactsIdempotent(t *testing.T) {
	store := testContactStore()
	h := importContactsHandler(store, importTestSecret)

	vc := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:uid-same\r\nFN:Sam\r\nEND:VCARD\r\n"
	req := map[string]any{"account": "acc@vulos.to", "vcards": []string{vc}}

	// First POST.
	w1 := doImport(h, "/", req, importTestSecret)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first post: %d", w1.Code)
	}
	// Second POST with same UID: overwrite (idempotent) — still 202, no duplicate.
	w2 := doImport(h, "/", req, importTestSecret)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second post: %d", w2.Code)
	}
	// Store must still have exactly one resource for this UID.
	resources, _ := store.List("acc@vulos.to")
	if len(resources) != 1 {
		t.Errorf("idempotent: store has %d resources, want 1", len(resources))
	}
}

// ── /api/admin/import/events ──────────────────────────────────────────────────

func TestImportEventsAuth(t *testing.T) {
	h := importEventsHandler(testCalStore(), importTestSecret)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: got %d, want 401", w.Code)
	}
}

func TestImportEventsBulk(t *testing.T) {
	store := testCalStore()
	h := importEventsHandler(store, importTestSecret)

	ics1 := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:evt-001\r\nSUMMARY:Standup\r\nDTSTART:20260628T090000Z\r\nDTEND:20260628T093000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	ics2 := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:evt-002\r\nSUMMARY:Planning\r\nDTSTART:20260628T140000Z\r\nDTEND:20260628T150000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	noUID := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nSUMMARY:NoUID\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	w := doImport(h, "/", map[string]any{
		"account": "cal@vulos.to",
		"events":  []string{ics1, ics2, noUID},
	}, importTestSecret)

	if w.Code != http.StatusAccepted {
		t.Fatalf("bulk: got %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]int
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["imported"] != 2 {
		t.Errorf("imported = %d, want 2", resp["imported"])
	}
	if resp["errors"] != 1 {
		t.Errorf("errors = %d, want 1 (no-UID event)", resp["errors"])
	}

	// Verify events are in the CalDAV store.
	resources := store.List("cal@vulos.to")
	if len(resources) != 2 {
		t.Fatalf("store has %d resources, want 2", len(resources))
	}
}

func TestImportEventsIdempotent(t *testing.T) {
	store := testCalStore()
	h := importEventsHandler(store, importTestSecret)

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:same-ev\r\nSUMMARY:Recurring\r\nDTSTART:20260628T100000Z\r\nDTEND:20260628T110000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	req := map[string]any{"account": "u@vulos.to", "events": []string{ics}}

	_ = doImport(h, "/", req, importTestSecret)
	_ = doImport(h, "/", req, importTestSecret)

	// UID-keyed href → same resource overwritten, not duplicated.
	resources := store.List("u@vulos.to")
	if len(resources) != 1 {
		t.Errorf("idempotent: store has %d resources, want 1", len(resources))
	}
}
