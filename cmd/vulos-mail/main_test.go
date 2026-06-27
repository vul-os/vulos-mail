package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	injectBrokerHeaders(r.Header, "trusted-secret", "alice@vulos.to", "real-pass", "imap.vulos.to", "993", "smtp.vulos.to", "587")

	// The DAV headers must be stripped and NOT re-added (IMAP-only proxy).
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
