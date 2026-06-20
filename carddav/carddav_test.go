package carddav

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sampleVCard is a minimal valid vCard 3.0 used across the tests.
const sampleVCard = "BEGIN:VCARD\r\n" +
	"VERSION:3.0\r\n" +
	"FN:Alice Example\r\n" +
	"N:Example;Alice;;;\r\n" +
	"EMAIL:alice@example.com\r\n" +
	"END:VCARD\r\n"

// testBackend returns a Backend whose Auth accepts user "alice"/pass "secret"
// mapping to account "alice", backed by a fresh MemStore.
func testBackend() *Backend {
	return &Backend{
		Store: NewMemStore(),
		Auth: func(user, pass string) (string, bool) {
			if user == "alice" && pass == "secret" {
				return "alice", true
			}
			return "", false
		},
	}
}

// do issues an authenticated request against the handler and returns the
// recorder. When withAuth is false the Basic header is omitted.
func do(t *testing.T, b *Backend, method, path, body string, withAuth bool, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if withAuth {
		req.SetBasicAuth("alice", "secret")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, req)
	return rec
}

func TestPutThenGet(t *testing.T) {
	b := testBackend()
	const path = "/dav/addressbooks/alice/alice.vcf"

	put := do(t, b, http.MethodPut, path, sampleVCard, true, nil)
	if put.Code != http.StatusCreated {
		t.Fatalf("PUT status = %d, want %d", put.Code, http.StatusCreated)
	}
	etag := put.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("PUT did not return an ETag")
	}

	get := do(t, b, http.MethodGet, path, "", true, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", get.Code, http.StatusOK)
	}
	if got := get.Body.String(); got != sampleVCard {
		t.Fatalf("GET body = %q, want %q", got, sampleVCard)
	}
	if ct := get.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/vcard") {
		t.Fatalf("GET Content-Type = %q, want text/vcard...", ct)
	}
	if got := get.Header().Get("ETag"); got != etag {
		t.Fatalf("GET ETag = %q, want %q", got, etag)
	}
}

func TestPropfindListsResource(t *testing.T) {
	b := testBackend()
	do(t, b, http.MethodPut, "/dav/addressbooks/alice/alice.vcf", sampleVCard, true, nil)

	rec := do(t, b, "PROPFIND", "/dav/addressbooks/alice/", "", true, map[string]string{"Depth": "1"})
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND status = %d, want %d", rec.Code, http.StatusMultiStatus)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"multistatus",
		"/dav/addressbooks/alice/alice.vcf",
		"getcontenttype",
		"text/vcard",
		"getetag",
		"resourcetype",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("PROPFIND body missing %q\nbody:\n%s", want, body)
		}
	}
}

func TestReportAddressbookQuery(t *testing.T) {
	b := testBackend()
	do(t, b, http.MethodPut, "/dav/addressbooks/alice/alice.vcf", sampleVCard, true, nil)

	const reportBody = `<?xml version="1.0" encoding="utf-8"?>
<C:addressbook-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:prop><D:getetag/><C:address-data/></D:prop>
</C:addressbook-query>`

	rec := do(t, b, "REPORT", "/dav/addressbooks/alice/", reportBody, true,
		map[string]string{"Depth": "1", "Content-Type": "application/xml"})
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("REPORT status = %d, want %d", rec.Code, http.StatusMultiStatus)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/dav/addressbooks/alice/alice.vcf") {
		t.Errorf("REPORT body missing resource href\nbody:\n%s", body)
	}
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("REPORT body missing address-data\nbody:\n%s", body)
	}
}

func TestDeleteRemoves(t *testing.T) {
	b := testBackend()
	const path = "/dav/addressbooks/alice/alice.vcf"
	do(t, b, http.MethodPut, path, sampleVCard, true, nil)

	del := do(t, b, http.MethodDelete, path, "", true, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d", del.Code, http.StatusNoContent)
	}

	get := do(t, b, http.MethodGet, path, "", true, nil)
	if get.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE status = %d, want %d", get.Code, http.StatusNotFound)
	}

	delAgain := do(t, b, http.MethodDelete, path, "", true, nil)
	if delAgain.Code != http.StatusNotFound {
		t.Fatalf("DELETE missing status = %d, want %d", delAgain.Code, http.StatusNotFound)
	}
}

func TestAuth(t *testing.T) {
	b := testBackend()
	tests := []struct {
		name     string
		method   string
		path     string
		withAuth bool
		user     string
		pass     string
		want     int
	}{
		{name: "no credentials", method: "PROPFIND", path: "/dav/addressbooks/alice/", withAuth: false, want: http.StatusUnauthorized},
		{name: "get no credentials", method: http.MethodGet, path: "/dav/addressbooks/alice/alice.vcf", withAuth: false, want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, b, tt.method, tt.path, "", tt.withAuth, nil)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if rec.Code == http.StatusUnauthorized {
				if rec.Header().Get("WWW-Authenticate") == "" {
					t.Errorf("401 response missing WWW-Authenticate challenge")
				}
			}
		})
	}
}

func TestBadCredentialsAndCrossAccount(t *testing.T) {
	b := testBackend()
	tests := []struct {
		name string
		user string
		pass string
		path string
		want int
	}{
		{name: "wrong password", user: "alice", pass: "nope", path: "/dav/addressbooks/alice/", want: http.StatusUnauthorized},
		{name: "cross account", user: "alice", pass: "secret", path: "/dav/addressbooks/bob/", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PROPFIND", tt.path, nil)
			req.SetBasicAuth(tt.user, tt.pass)
			rec := httptest.NewRecorder()
			b.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestPutInvalidVCard(t *testing.T) {
	b := testBackend()
	rec := do(t, b, http.MethodPut, "/dav/addressbooks/alice/bad.vcf", "not a vcard at all", true, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPutReplaceReturnsNoContent(t *testing.T) {
	b := testBackend()
	const path = "/dav/addressbooks/alice/alice.vcf"
	do(t, b, http.MethodPut, path, sampleVCard, true, nil)
	rec := do(t, b, http.MethodPut, path, sampleVCard, true, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT replace status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestMemStore(t *testing.T) {
	s := NewMemStore()
	if _, err := s.Get("alice", "missing.vcf"); err != ErrNotFound {
		t.Fatalf("Get missing err = %v, want ErrNotFound", err)
	}
	if err := s.Delete("alice", "missing.vcf"); err != ErrNotFound {
		t.Fatalf("Delete missing err = %v, want ErrNotFound", err)
	}
	r, err := s.Put("alice", "a.vcf", []byte(sampleVCard))
	if err != nil {
		t.Fatalf("Put err = %v", err)
	}
	if r.ETag == "" {
		t.Fatalf("Put returned empty ETag")
	}
	got, err := s.Get("alice", "a.vcf")
	if err != nil || string(got.Data) != sampleVCard {
		t.Fatalf("Get = %q, %v", got.Data, err)
	}
	// Mutating the input must not change stored bytes.
	in := []byte(sampleVCard)
	s.Put("alice", "b.vcf", in)
	in[0] = 'X'
	got2, _ := s.Get("alice", "b.vcf")
	if string(got2.Data) != sampleVCard {
		t.Fatalf("stored data mutated via caller slice: %q", got2.Data)
	}
	list, _ := s.List("alice")
	if len(list) != 2 || list[0].Href != "a.vcf" || list[1].Href != "b.vcf" {
		t.Fatalf("List = %+v, want sorted [a.vcf b.vcf]", list)
	}
}
