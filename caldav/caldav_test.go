package caldav_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vul-os/vmail/caldav"
)

// testAuth accepts a single hard-coded credential and maps it to account
// "alice".
func testAuth(user, pass string) (string, bool) {
	if user == "alice" && pass == "secret" {
		return "alice", true
	}
	return "", false
}

// newServer returns a running httptest.Server backed by a fresh MemStore.
func newServer(t *testing.T) (*httptest.Server, *caldav.MemStore) {
	t.Helper()
	store := caldav.NewMemStore()
	b := caldav.New(testAuth, store)
	srv := httptest.NewServer(b.Handler())
	t.Cleanup(srv.Close)
	return srv, store
}

// do issues an authenticated request and returns the response.
func do(t *testing.T, srv *httptest.Server, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.SetBasicAuth("alice", "secret")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

const simpleEvent = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//vmail//test//EN\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:simple-1\r\n" +
	"DTSTART:20260615T090000Z\r\n" +
	"DTEND:20260615T100000Z\r\n" +
	"SUMMARY:Standup\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

const recurringEvent = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//vmail//test//EN\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:daily-1\r\n" +
	"DTSTART:20260601T120000Z\r\n" +
	"DTEND:20260601T123000Z\r\n" +
	"RRULE:FREQ=DAILY;COUNT=3\r\n" +
	"SUMMARY:Daily sync\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// reportBody builds a calendar-query REPORT body with a VEVENT time-range.
func reportBody(start, end string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:getetag/><C:calendar-data/></D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="` + start + `" end="` + end + `"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>`
}

// parseReport decodes a 207 multistatus body and returns the hrefs present.
func parseReport(t *testing.T, body []byte) []string {
	t.Helper()
	var ms struct {
		Responses []struct {
			Href string `xml:"href"`
		} `xml:"response"`
	}
	if err := xml.Unmarshal(body, &ms); err != nil {
		t.Fatalf("unmarshal multistatus: %v\nbody:\n%s", err, body)
	}
	hrefs := make([]string, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		hrefs = append(hrefs, r.Href)
	}
	return hrefs
}

func TestPutThenReportReturnsEvent(t *testing.T) {
	srv, _ := newServer(t)

	resp := do(t, srv, http.MethodPut, "/dav/calendars/alice/standup.ics", simpleEvent, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Range covering the event.
	resp = do(t, srv, "REPORT", "/dav/calendars/alice/", reportBody("20260615T000000Z", "20260616T000000Z"), nil)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("REPORT status = %d, want 207", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	hrefs := parseReport(t, body)
	if len(hrefs) != 1 || !strings.HasSuffix(hrefs[0], "/standup.ics") {
		t.Fatalf("REPORT hrefs = %v, want one ending in /standup.ics", hrefs)
	}

	// Range not covering the event.
	resp = do(t, srv, "REPORT", "/dav/calendars/alice/", reportBody("20260101T000000Z", "20260102T000000Z"), nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if hrefs := parseReport(t, body); len(hrefs) != 0 {
		t.Fatalf("REPORT outside range hrefs = %v, want none", hrefs)
	}
}

func TestRecurringExpansionWithinRange(t *testing.T) {
	srv, _ := newServer(t)

	resp := do(t, srv, http.MethodPut, "/dav/calendars/alice/daily.ics", recurringEvent, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	tests := []struct {
		name       string
		start, end string
		wantMatch  bool
	}{
		// Master instance (2026-06-01).
		{"first instance", "20260601T000000Z", "20260602T000000Z", true},
		// Third (last) instance, 2026-06-03 — only reachable via RRULE expansion.
		{"third instance via rrule", "20260603T000000Z", "20260604T000000Z", true},
		// Fourth day, 2026-06-04 — beyond COUNT=3, must NOT match.
		{"beyond count", "20260604T000000Z", "20260605T000000Z", false},
		// Whole window spanning all instances.
		{"whole span", "20260601T000000Z", "20260605T000000Z", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := do(t, srv, "REPORT", "/dav/calendars/alice/", reportBody(tt.start, tt.end), nil)
			if resp.StatusCode != http.StatusMultiStatus {
				t.Fatalf("REPORT status = %d, want 207", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			hrefs := parseReport(t, body)
			got := len(hrefs) == 1
			if got != tt.wantMatch {
				t.Fatalf("range [%s,%s): matched=%v (hrefs=%v), want matched=%v",
					tt.start, tt.end, got, hrefs, tt.wantMatch)
			}
		})
	}
}

func TestGetReturnsStoredBytes(t *testing.T) {
	srv, _ := newServer(t)

	resp := do(t, srv, http.MethodPut, "/dav/calendars/alice/g.ics", simpleEvent, nil)
	resp.Body.Close()

	resp = do(t, srv, http.MethodGet, "/dav/calendars/alice/g.ics", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(body, []byte(simpleEvent)) {
		t.Fatalf("GET body mismatch:\ngot:  %q\nwant: %q", body, simpleEvent)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Fatalf("Content-Type = %q, want text/calendar", ct)
	}
	if resp.Header.Get("ETag") == "" {
		t.Fatal("missing ETag header")
	}
}

func TestDeleteRemoves(t *testing.T) {
	srv, store := newServer(t)

	resp := do(t, srv, http.MethodPut, "/dav/calendars/alice/d.ics", simpleEvent, nil)
	resp.Body.Close()

	resp = do(t, srv, http.MethodDelete, "/dav/calendars/alice/d.ics", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	if _, ok := store.Get("alice", "d.ics"); ok {
		t.Fatal("resource still present after DELETE")
	}

	resp = do(t, srv, http.MethodGet, "/dav/calendars/alice/d.ics", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after DELETE = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	// Deleting a missing resource is a 404.
	resp = do(t, srv, http.MethodDelete, "/dav/calendars/alice/missing.ics", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE missing = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAuthRequired(t *testing.T) {
	srv, _ := newServer(t)

	tests := []struct {
		name   string
		setReq func(*http.Request)
	}{
		{"no credentials", func(*http.Request) {}},
		{"bad password", func(r *http.Request) { r.SetBasicAuth("alice", "wrong") }},
		{"unknown user", func(r *http.Request) { r.SetBasicAuth("bob", "secret") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PROPFIND", srv.URL+"/dav/calendars/alice/", nil)
			tt.setReq(req)
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if resp.Header.Get("WWW-Authenticate") == "" {
				t.Error("missing WWW-Authenticate challenge")
			}
		})
	}
}

func TestPropfindListsResources(t *testing.T) {
	srv, _ := newServer(t)

	for _, name := range []string{"a.ics", "b.ics"} {
		resp := do(t, srv, http.MethodPut, "/dav/calendars/alice/"+name, simpleEvent, nil)
		resp.Body.Close()
	}

	resp := do(t, srv, "PROPFIND", "/dav/calendars/alice/", "", map[string]string{"Depth": "1"})
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("PROPFIND status = %d, want 207", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	hrefs := parseReport(t, body)
	// Collection + two resources.
	if len(hrefs) != 3 {
		t.Fatalf("PROPFIND hrefs = %v, want 3 (collection + 2)", hrefs)
	}
	joined := strings.Join(hrefs, " ")
	for _, want := range []string{"/a.ics", "/b.ics"} {
		if !strings.Contains(joined, want) {
			t.Errorf("PROPFIND missing %s in %v", want, hrefs)
		}
	}
}

func TestPutRejectsInvalidICS(t *testing.T) {
	srv, _ := newServer(t)
	resp := do(t, srv, http.MethodPut, "/dav/calendars/alice/bad.ics", "not an ical", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT invalid status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestForbiddenForOtherAccount(t *testing.T) {
	srv, _ := newServer(t)
	// alice authenticates but addresses bob's collection.
	resp := do(t, srv, http.MethodGet, "/dav/calendars/bob/x.ics", "", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-account status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}
