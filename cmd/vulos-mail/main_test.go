package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/authlimit"
	"github.com/vul-os/vulos-mail/internal/diagnostics"
	"github.com/vul-os/vulos-mail/internal/server"
)

// A session-holding client must not be able to forge any broker/credential
// header on the /v1 → lilmail proxy. injectBrokerHeaders strips the full inbound
// set first, so forged values (notably the CalDAV/CardDAV base URLs, which this
// IMAP-only proxy does not re-add) never reach the engine.
func TestInjectBrokerHeadersStripsForgedInbound(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)

	// Forge the entire credential set, including the broker gate and the DAV
	// base URLs an attacker would point at an internal/exfil target.
	r.Header.Set("X-Vulos-Broker-Auth", "forged-secret")
	r.Header.Set("X-Vulos-Mail-Provider", "evil")
	r.Header.Set("X-Vulos-Mail-Email", "attacker@evil.test")
	r.Header.Set("X-Vulos-Mail-Username", "attacker@evil.test")
	r.Header.Set("X-Vulos-Mail-Auth", "oauthbearer")
	r.Header.Set("X-Vulos-Mail-Secret", "forged-token")
	r.Header.Set("X-Vulos-Mail-Imap-Host", "evil.test")
	r.Header.Set("X-Vulos-Mail-Imap-Port", "1234")
	r.Header.Set("X-Vulos-Mail-Smtp-Host", "evil.test")
	r.Header.Set("X-Vulos-Mail-Smtp-Port", "5678")
	r.Header.Set("X-Vulos-Mail-Caldav-Url", "http://169.254.169.254/latest/meta-data/")
	r.Header.Set("X-Vulos-Mail-Carddav-Url", "http://internal.svc/contacts")

	// No trusted DAV URLs configured (caldavURL/carddavURL empty).
	injectBrokerHeaders(r.Header, "trusted-secret", "alice@vulos.to", "real-pass", "imap.vulos.to", "993", "smtp.vulos.to", "587", "", "")

	// The forged DAV headers must be stripped and NOT re-added when no trusted
	// DAV URL is configured.
	for _, h := range []string{"X-Vulos-Mail-Caldav-Url", "X-Vulos-Mail-Carddav-Url"} {
		if got := r.Header.Get(h); got != "" {
			t.Fatalf("forged %s was forwarded: %q (want stripped)", h, got)
		}
	}

	// The remaining credential headers must reflect the trusted session, not the
	// forged inbound values.
	want := map[string]string{
		"X-Vulos-Broker-Auth":    "trusted-secret",
		"X-Vulos-Mail-Provider":  "imap",
		"X-Vulos-Mail-Email":     "alice@vulos.to",
		"X-Vulos-Mail-Username":  "alice@vulos.to",
		"X-Vulos-Mail-Auth":      "plain",
		"X-Vulos-Mail-Secret":    "real-pass",
		"X-Vulos-Mail-Imap-Host": "imap.vulos.to",
		"X-Vulos-Mail-Imap-Port": "993",
		"X-Vulos-Mail-Smtp-Host": "smtp.vulos.to",
		"X-Vulos-Mail-Smtp-Port": "587",
	}
	for h, exp := range want {
		if got := r.Header.Get(h); got != exp {
			t.Fatalf("%s = %q, want %q (forged value leaked or not overwritten)", h, got, exp)
		}
	}

	// A forged header must not survive as a duplicate value either.
	for h := range want {
		if vs := r.Header.Values(h); len(vs) != 1 {
			t.Fatalf("%s has %d values %v, want exactly 1 (forged duplicate not stripped)", h, len(vs), vs)
		}
	}
}

func TestWebSessionStoreSetPassRotatesCredential(t *testing.T) {
	s := newWebSessionStore(time.Hour)
	tok := s.create("alice@vulos.to", "old-pass")

	sess, ok := s.get(tok)
	if !ok || sess.user != "alice@vulos.to" || sess.pass != "old-pass" {
		t.Fatalf("get after create = %+v, ok=%v", sess, ok)
	}

	// Rotating the password must preserve the token and user but update the
	// brokered credential in place (so /v1 keeps working without re-login).
	s.setPass(tok, "new-pass")
	sess, ok = s.get(tok)
	if !ok || sess.user != "alice@vulos.to" || sess.pass != "new-pass" {
		t.Fatalf("get after setPass = %+v, ok=%v", sess, ok)
	}

	// setPass on an unknown token is a no-op (must not panic or create a session).
	s.setPass("nope", "x")
	if _, ok := s.get("nope"); ok {
		t.Fatal("setPass created a session for an unknown token")
	}

	s.delete(tok)
	if _, ok := s.get(tok); ok {
		t.Fatal("session still present after delete")
	}
}

func TestWebSessionStoreExpires(t *testing.T) {
	s := newWebSessionStore(-time.Second) // already expired
	tok := s.create("bob@vulos.to", "pw")
	if _, ok := s.get(tok); ok {
		t.Fatal("expired session should not be returned")
	}
}

func TestWriteJSONErr(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONErr(rec, 401, "not authenticated")
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error != "not authenticated" {
		t.Fatalf("error = %q", body.Error)
	}
}

// TestInjectBrokerHeadersTrustedDAVInjected proves that operator-configured
// (trusted) DAV base URLs are injected after the inbound strip, replacing any
// client-forged values, so cal/contacts work standalone without letting a client
// point lilmail at an arbitrary DAV target.
func TestInjectBrokerHeadersTrustedDAVInjected(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/calendar/events", nil)
	// Client tries to forge DAV targets (SSRF).
	r.Header.Set("X-Vulos-Mail-Caldav-Url", "http://169.254.169.254/latest/")
	r.Header.Set("X-Vulos-Mail-Carddav-Url", "http://internal.svc/contacts")

	injectBrokerHeaders(r.Header, "trusted-secret", "alice@vulos.to", "real-pass",
		"imap.vulos.to", "993", "smtp.vulos.to", "587",
		"https://dav.vulos.to/cal/", "https://dav.vulos.to/card/")

	if got := r.Header.Get("X-Vulos-Mail-Caldav-Url"); got != "https://dav.vulos.to/cal/" {
		t.Fatalf("caldav url = %q, want the trusted configured value", got)
	}
	if got := r.Header.Get("X-Vulos-Mail-Carddav-Url"); got != "https://dav.vulos.to/card/" {
		t.Fatalf("carddav url = %q, want the trusted configured value", got)
	}
	// Exactly one value each — the forged inbound value must not survive as a dup.
	for _, h := range []string{"X-Vulos-Mail-Caldav-Url", "X-Vulos-Mail-Carddav-Url"} {
		if vs := r.Header.Values(h); len(vs) != 1 {
			t.Fatalf("%s has %d values %v, want exactly 1", h, len(vs), vs)
		}
	}
}

// TestWebmailLoginRateLimit proves the login handler refuses an IP after enough
// failed credential checks (429), recovers on a correct password, and never
// throttles an unrelated IP.
func TestWebmailLoginRateLimit(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	lim := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Hour, Lockout: time.Hour, Now: func() time.Time { return clock }})
	guard := &authGuard{lim: lim}
	sessions := newWebSessionStore(time.Hour)

	authIMAP := func(u, p string) error {
		if p == "correct" {
			return nil
		}
		return errors.New("bad creds")
	}
	cookie := func(http.ResponseWriter, *http.Request, string, int) {}
	h := webmailLoginHandler(sessions, authIMAP, guard, cookie)

	post := func(remote, user, pass string) int {
		body := `{"user":"` + user + `","password":"` + pass + `"}`
		r := httptest.NewRequest(http.MethodPost, "/api/webmail/login", strings.NewReader(body))
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	// 3 bad attempts lock the IP; the 4th is throttled BEFORE the credential check.
	for i := 0; i < 3; i++ {
		if code := post("203.0.113.5:1", "alice", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i, code)
		}
	}
	if code := post("203.0.113.5:1", "alice", "correct"); code != http.StatusTooManyRequests {
		t.Fatalf("locked IP must be 429 even with a correct password, got %d", code)
	}
	// A different IP and account is unaffected and can sign in (the account key
	// "alice" is also locked cross-host by design, so use a distinct account to
	// isolate the per-IP dimension).
	if code := post("198.51.100.9:1", "carol", "correct"); code != http.StatusOK {
		t.Fatalf("distinct IP/account should sign in, got %d", code)
	}
	// After the lockout elapses, the first IP can sign in again.
	clock = clock.Add(2 * time.Hour)
	if code := post("203.0.113.5:1", "alice", "correct"); code != http.StatusOK {
		t.Fatalf("after lockout elapses: want 200, got %d", code)
	}
}

// TestWebmailLoginSuccessClearsFailures proves a correct password resets the
// failure history so a user's own earlier typos don't lock them out.
func TestWebmailLoginSuccessClearsFailures(t *testing.T) {
	lim := authlimit.New(authlimit.Config{MaxFailures: 3, Window: time.Hour, Lockout: time.Hour})
	guard := &authGuard{lim: lim}
	h := webmailLoginHandler(newWebSessionStore(time.Hour),
		func(_, p string) error {
			if p == "correct" {
				return nil
			}
			return errors.New("bad")
		}, guard, func(http.ResponseWriter, *http.Request, string, int) {})

	post := func(pass string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/webmail/login",
			strings.NewReader(`{"user":"bob","password":"`+pass+`"}`))
		r.RemoteAddr = "192.0.2.1:1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	post("wrong")
	post("wrong")
	if code := post("correct"); code != http.StatusOK { // resets history
		t.Fatalf("want 200, got %d", code)
	}
	// Two fresh failures must not lock yet (history was cleared).
	post("wrong")
	if code := post("wrong"); code != http.StatusUnauthorized {
		t.Fatalf("want 401 (not locked yet after reset), got %d", code)
	}
}

// TestClientIPXFFTrustedOnly proves X-Forwarded-For is honoured only from a
// trusted peer; an untrusted client cannot spoof its rate-limit key.
func TestClientIPXFFTrustedOnly(t *testing.T) {
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})

	// Untrusted peer: XFF ignored, RemoteAddr used.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:1"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := clientIP(r, trusted); got != "203.0.113.7" {
		t.Fatalf("untrusted peer: clientIP = %q, want 203.0.113.7", got)
	}

	// Trusted peer: rightmost untrusted XFF hop used.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.1.2.3:1"
	r.Header.Set("X-Forwarded-For", "9.9.9.9, 10.9.9.9")
	if got := clientIP(r, trusted); got != "9.9.9.9" {
		t.Fatalf("trusted peer: clientIP = %q, want 9.9.9.9", got)
	}
}

// TestTrustedPeer proves the Secure-cookie peer check only trusts allowlisted
// fronting proxies (so XFP can't be believed from a direct client).
func TestTrustedPeer(t *testing.T) {
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})
	mk := func(remote string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		return r
	}
	if !trustedPeer(mk("10.5.5.5:1"), trusted) {
		t.Fatal("10.5.5.5 should be a trusted peer")
	}
	if trustedPeer(mk("203.0.113.1:1"), trusted) {
		t.Fatal("203.0.113.1 must not be trusted")
	}
}

// --- ops endpoints: provisioning, healthz, diagnostics ---

const testBrokerSecret = "s3cret-broker"

func brokerReq(method, path, body, secret string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if secret != "" {
		r.Header.Set("X-Vulos-Broker-Auth", secret)
	}
	return r
}

// TestProvisionMailboxBrokerGate proves the endpoint is closed without a matching
// broker secret and creates a mailbox idempotently with one.
func TestProvisionMailboxBrokerGate(t *testing.T) {
	mgr := server.NewManager(t.TempDir(), nil, nil) // in-memory creds (Identity nil)
	h := provisionMailboxHandler(mgr, "vulos.to", testBrokerSecret)

	body := `{"localpart":"alice","domain":"vulos.to","org":"acme"}`

	// No secret -> 401.
	w := httptest.NewRecorder()
	h(w, brokerReq(http.MethodPost, "/api/admin/provision-mailbox", body, ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no secret: want 401, got %d", w.Code)
	}

	// Wrong secret -> 401.
	w = httptest.NewRecorder()
	h(w, brokerReq(http.MethodPost, "/api/admin/provision-mailbox", body, "nope"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret: want 401, got %d", w.Code)
	}

	// Correct secret -> 200, created:true.
	w = httptest.NewRecorder()
	h(w, brokerReq(http.MethodPost, "/api/admin/provision-mailbox", body, testBrokerSecret))
	if w.Code != http.StatusOK {
		t.Fatalf("provision: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Address string `json:"address"`
		Created bool   `json:"created"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Address != "alice@vulos.to" || !resp.Created {
		t.Fatalf("provision resp = %+v, want alice@vulos.to created:true", resp)
	}
	if !mgr.IsLocal("alice@vulos.to") {
		t.Fatal("mailbox not actually provisioned")
	}

	// Idempotent: second call -> 200, created:false.
	w = httptest.NewRecorder()
	h(w, brokerReq(http.MethodPost, "/api/admin/provision-mailbox", body, testBrokerSecret))
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || resp.Created {
		t.Fatalf("idempotent re-provision: want 200 created:false, got %d %+v", w.Code, resp)
	}
}

func TestProvisionMailboxValidation(t *testing.T) {
	mgr := server.NewManager(t.TempDir(), nil, nil)
	h := provisionMailboxHandler(mgr, "vulos.to", testBrokerSecret)

	for _, bad := range []string{
		`{"localpart":"","domain":"vulos.to"}`,
		`{"localpart":"a@b","domain":"vulos.to"}`,
		`{"localpart":"has space","domain":"vulos.to"}`,
	} {
		w := httptest.NewRecorder()
		h(w, brokerReq(http.MethodPost, "/api/admin/provision-mailbox", bad, testBrokerSecret))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %q: want 400, got %d", bad, w.Code)
		}
	}

	// GET is rejected.
	w := httptest.NewRecorder()
	h(w, brokerReq(http.MethodGet, "/api/admin/provision-mailbox", "", testBrokerSecret))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: want 405, got %d", w.Code)
	}
}

func TestProvisionMailboxDefaultDomain(t *testing.T) {
	mgr := server.NewManager(t.TempDir(), nil, nil)
	h := provisionMailboxHandler(mgr, "fallback.test", testBrokerSecret)
	w := httptest.NewRecorder()
	h(w, brokerReq(http.MethodPost, "/api/admin/provision-mailbox", `{"localpart":"bob"}`, testBrokerSecret))
	var resp struct {
		Address string `json:"address"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Address != "bob@fallback.test" {
		t.Fatalf("default-domain address = %q, want bob@fallback.test", resp.Address)
	}
}

func TestHealthz(t *testing.T) {
	w := httptest.NewRecorder()
	healthzHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", w.Code)
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Fatalf("healthz body status = %q, want ok", resp.Status)
	}
}

// TestDiagnosticsHandlerBrokerGate proves the diagnostics endpoint is broker-gated
// and returns a JSON Report. A disabled runner keeps the test fully offline.
func TestDiagnosticsHandlerBrokerGate(t *testing.T) {
	runner := diagnostics.New(diagnostics.Config{Domain: "example.test", Enabled: false})
	h := diagnosticsHandler(runner, testBrokerSecret)

	// Closed without a matching secret.
	w := httptest.NewRecorder()
	h(w, brokerReq(http.MethodGet, "/api/diagnostics", "", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no secret: want 401, got %d", w.Code)
	}

	// With the secret, returns a JSON report.
	w = httptest.NewRecorder()
	h(w, brokerReq(http.MethodGet, "/api/diagnostics", "", testBrokerSecret))
	if w.Code != http.StatusOK {
		t.Fatalf("diagnostics: want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var rep diagnostics.Report
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatalf("report not valid JSON: %v", err)
	}
	if rep.Domain != "example.test" || rep.Summary.Total != len(rep.Checks) {
		t.Fatalf("unexpected report shape: %+v", rep)
	}
}

// TestDiagnosticsHandlerClosedWhenNoSecret proves an empty configured secret
// closes the endpoint entirely (no request can pass).
func TestDiagnosticsHandlerClosedWhenNoSecret(t *testing.T) {
	runner := diagnostics.New(diagnostics.Config{Domain: "example.test", Enabled: false})
	h := diagnosticsHandler(runner, "")
	w := httptest.NewRecorder()
	h(w, brokerReq(http.MethodGet, "/api/diagnostics", "", "anything"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("closed endpoint: want 401, got %d", w.Code)
	}
	_ = context.Background()
}
