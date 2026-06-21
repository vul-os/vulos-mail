package llm_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vul-os/vulos-mail/internal/llm"
)

func TestProxyDisabledWhenNoBase(t *testing.T) {
	if llm.New("", func(string) (string, bool) { return "k", true }) != nil {
		t.Fatal("proxy must be nil when no llmux URL is configured")
	}
}

func TestProxyInjectsAccountKeyAndForwards(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	p := llm.New(upstream.URL, func(account string) (string, bool) {
		if account == "alice@vulos.to" {
			return "sk-alice", true
		}
		return "", false
	})
	auth := func(r *http.Request) (string, bool) { return "alice@vulos.to", true }
	h := p.Handler("/api/llm", auth)

	// allowed path: key injected, body + path forwarded
	r := httptest.NewRequest(http.MethodPost, "/api/llm/v1/chat/completions", strings.NewReader(`{"model":"smart"}`))
	r.Header.Set("Authorization", "Bearer client-should-be-ignored")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if gotAuth != "Bearer sk-alice" {
		t.Fatalf("upstream auth = %q, want the account key (client auth must be stripped)", gotAuth)
	}
	if gotPath != "/v1/chat/completions" || gotBody != `{"model":"smart"}` {
		t.Fatalf("forwarded path=%q body=%q", gotPath, gotBody)
	}
}

func TestProxyRejectsUnauthAndDisallowedPaths(t *testing.T) {
	p := llm.New("http://example.invalid", func(string) (string, bool) { return "k", true })

	// unauthenticated → 401
	w := httptest.NewRecorder()
	p.Handler("/api/llm", func(*http.Request) (string, bool) { return "", false }).
		ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/llm/v1/chat/completions", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want 401", w.Code)
	}

	// authenticated but disallowed upstream path → 404 (no SSRF to arbitrary paths)
	w = httptest.NewRecorder()
	p.Handler("/api/llm", func(*http.Request) (string, bool) { return "alice@vulos.to", true }).
		ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/llm/v1/../admin/keys", nil))
	if w.Code == http.StatusOK {
		t.Fatal("disallowed upstream path must not be proxied")
	}

	// account with no LLM access → 403
	w = httptest.NewRecorder()
	llm.New("http://example.invalid", func(string) (string, bool) { return "", false }).
		Handler("/api/llm", func(*http.Request) (string, bool) { return "x@vulos.to", true }).
		ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/llm/v1/chat/completions", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("no-LLM-access status = %d, want 403", w.Code)
	}
}
