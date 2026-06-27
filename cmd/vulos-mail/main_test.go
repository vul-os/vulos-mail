package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

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
