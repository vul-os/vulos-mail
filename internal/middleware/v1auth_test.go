package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vul-os/vulos-mail/internal/apikey"
	"github.com/vul-os/vulos-mail/internal/middleware"
)

// stubIntrospector is a mocked apikey.Introspector for middleware tests.
type stubIntrospector struct {
	res apikey.Result
	err error
}

func (s stubIntrospector) Introspect(_ context.Context, _ string) (apikey.Result, error) {
	return s.res, s.err
}

// testSessionAuth is a simple session-auth callback used in tests: the special
// cookie "valid-session" resolves to "user@example.com"; any other value fails.
func testSessionAuth(r *http.Request) (account, secret string, ok bool) {
	c, err := r.Cookie("vm_session")
	if err != nil {
		return "", "", false
	}
	if c.Value == "valid-session" {
		return "user@example.com", c.Value, true
	}
	return "", "", false
}

// echoHandler is the downstream handler used in tests. It reads the AuthInfo
// from context and returns it as JSON so tests can inspect the resolved identity.
var echoHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	info, ok := middleware.GetV1Auth(r.Context())
	if !ok {
		http.Error(w, "no auth in context", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"method":  info.Method,
		"account": info.Account,
	})
})

// do sends a request through the middleware and returns the response recorder.
func do(intro apikey.Introspector, authHeader, sessionCookie string) *httptest.ResponseRecorder {
	h := middleware.V1Auth(intro, testSessionAuth, echoHandler)
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "vm_session", Value: sessionCookie})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestV1Auth_ValidAPIKey(t *testing.T) {
	intro := stubIntrospector{res: apikey.Result{
		Valid: true, Account: "alice@vulos.org", Products: []string{"mail"},
	}}
	w := do(intro, "Bearer vk_live_good", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "alice@vulos.org") || !contains(w.Body.String(), "apikey") {
		t.Fatalf("expected identity from key, got %s", w.Body.String())
	}
}

func TestV1Auth_InvalidAPIKey(t *testing.T) {
	intro := stubIntrospector{res: apikey.Result{Valid: false}}
	w := do(intro, "Bearer vk_bad", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestV1Auth_KeyMissingMailProduct(t *testing.T) {
	intro := stubIntrospector{res: apikey.Result{
		Valid: true, Account: "x@vulos.org", Products: []string{"office"},
	}}
	w := do(intro, "Bearer vk_wrongproduct", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestV1Auth_IntrospectionUnavailable(t *testing.T) {
	intro := stubIntrospector{err: errors.New("cp down")}
	w := do(intro, "Bearer vk_anything", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestV1Auth_ValidSession(t *testing.T) {
	// No introspector configured; valid session cookie → 200.
	w := do(nil, "", "valid-session")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "user@example.com") || !contains(w.Body.String(), "session") {
		t.Fatalf("expected session identity, got %s", w.Body.String())
	}
}

func TestV1Auth_InvalidSession(t *testing.T) {
	w := do(nil, "", "bad-session")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestV1Auth_NoCredentials(t *testing.T) {
	w := do(nil, "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestV1Auth_KeyIgnoredWhenIntrospectorNil(t *testing.T) {
	// vk_ key presented but intro nil → falls through to session path → 401
	// (no valid session cookie either).
	w := do(nil, "Bearer vk_ignored", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (session fallback, no cookie), got %d (%s)", w.Code, w.Body.String())
	}
}

func TestV1Auth_KeyIgnoredWhenIntrospectorNil_ValidSession(t *testing.T) {
	// vk_ key presented but intro nil; valid session cookie is present →
	// session path wins and the request succeeds.
	w := do(nil, "Bearer vk_ignored", "valid-session")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (session wins), got %d (%s)", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "session") {
		t.Fatalf("expected session method, got %s", w.Body.String())
	}
}

func TestV1Auth_APIKeyAndSessionBothPresent(t *testing.T) {
	// When both a vk_ key and a session cookie are present, the key wins.
	intro := stubIntrospector{res: apikey.Result{
		Valid: true, Account: "key-account@vulos.org", Products: []string{"mail"},
	}}
	w := do(intro, "Bearer vk_live_priority", "valid-session")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "apikey") {
		t.Fatalf("expected apikey method to win, got %s", w.Body.String())
	}
}

// contains is a simple substring check that avoids importing strings.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
