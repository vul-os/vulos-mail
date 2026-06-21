// Package llm lets vulos-mail offer LLM features (summaries, smart replies, …)
// by routing OpenAI-compatible requests through the suite's llmux gateway — so
// provider routing, per-account budgets, and token-cost metering are handled
// centrally (and, in a Vulos deployment, billed via the control plane) rather
// than mail calling providers directly.
//
// It is OPTIONAL and off by default: with no llmux URL configured, mail exposes
// no LLM surface and stays a pure mail server. The user's own credentials gate
// access; mail injects the account's llmux key (the client never sees it) and
// only the chat/embeddings paths are proxied.
package llm

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// KeyFunc returns the llmux bearer key to use for an authenticated account.
// Standalone deployments return a single configured key for every account; a
// cloud deployment returns the account's cp-issued key. ok=false → no LLM access.
type KeyFunc func(account string) (key string, ok bool)

// AuthFunc authenticates a request and returns the account it acts as.
type AuthFunc func(r *http.Request) (account string, ok bool)

// Proxy forwards allowlisted OpenAI-compatible calls to an llmux gateway,
// substituting the account's key. Budget/suspension are enforced downstream by
// llmux + cp (a 402 simply flows back to the caller).
type Proxy struct {
	base   string // llmux base URL, e.g. http://llmux:4000
	keyFor KeyFunc
	hc     *http.Client
}

// allowed are the only upstream paths a client may reach through the proxy.
var allowed = map[string]bool{
	"/v1/chat/completions": true,
	"/v1/embeddings":       true,
	"/v1/models":           true,
}

// New returns a Proxy, or nil if llmux isn't configured (base == "").
func New(base string, keyFor KeyFunc) *Proxy {
	if base == "" || keyFor == nil {
		return nil
	}
	return &Proxy{
		base:   strings.TrimRight(base, "/"),
		keyFor: keyFor,
		hc:     &http.Client{Timeout: 120 * time.Second},
	}
}

// Handler authenticates with auth, then proxies POSTs to the allowlisted llmux
// paths under prefix (e.g. "/api/llm"). It strips any client Authorization and
// injects the account's key, so a caller can never spend another account's
// budget or reach an arbitrary upstream path.
func (p *Proxy) Handler(prefix string, auth AuthFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="vulos-mail"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		key, ok := p.keyFor(account)
		if !ok {
			http.Error(w, "LLM features not enabled for this account", http.StatusForbidden)
			return
		}
		upstream := strings.TrimPrefix(r.URL.Path, prefix)
		if !strings.HasPrefix(upstream, "/") {
			upstream = "/" + upstream
		}
		if !allowed[upstream] {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, p.base+upstream, r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		if a := r.Header.Get("Accept"); a != "" {
			req.Header.Set("Accept", a)
		}
		req.Header.Set("Authorization", "Bearer "+key) // account-scoped; client never sets this
		resp, err := p.hc.Do(req)
		if err != nil {
			http.Error(w, "llm gateway unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(resp.StatusCode)
		// Stream the response through (supports SSE streaming completions).
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if rerr == io.EOF {
				return
			}
			if rerr != nil {
				return
			}
		}
	})
}
