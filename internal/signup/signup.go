// Package signup provides the self-serve account-creation HTTP surface for the
// standalone server: an anti-abuse challenge endpoint and a gated signup
// endpoint that provisions a new handle@domain mailbox. It depends only on the
// seam interfaces, so the same handler works with the local Altcha gate or a
// cloud-backed gate.
package signup

import (
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/vul-os/vulos-mail/internal/seam"
)

// ProvisionFunc creates an account (typically Manager.AddAccount).
type ProvisionFunc func(address, password string) error

// Issuer optionally mints an anti-abuse challenge (e.g. *altcha.Gate).
type Issuer interface{ IssueJSON() ([]byte, error) }

// handle: 3–32 chars, starts alphanumeric, then [a-z0-9._-].
var handleRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

// defaultReserved are handles that must never be self-registered.
var defaultReserved = map[string]bool{
	"admin": true, "administrator": true, "postmaster": true, "abuse": true,
	"root": true, "hostmaster": true, "webmaster": true, "noreply": true,
	"no-reply": true, "support": true, "security": true, "mailer-daemon": true,
	"info": true, "help": true, "billing": true, "dmarc": true, "spam": true,
}

// Config wires the signup handler.
type Config struct {
	Domain    string
	Gate      seam.SignupGate // anti-abuse verification (required)
	Issuer    Issuer          // optional challenge minting (nil → no challenge endpoint)
	Provision ProvisionFunc   // create the account (required)
	// Reserved, if set, additionally reports handles that may not be registered.
	Reserved func(handle string) bool
}

// Handler returns an http.Handler serving:
//
//	GET  /api/signup/challenge  -> anti-abuse challenge JSON (if an Issuer is set)
//	POST /api/signup            -> { handle, password, solution } create mailbox
func Handler(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/signup/challenge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || cfg.Issuer == nil {
			http.NotFound(w, r)
			return
		}
		b, err := cfg.Issuer.IssueJSON()
		if err != nil {
			http.Error(w, "challenge unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/api/signup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Handle   string `json:"handle"`
			Password string `json:"password"`
			Solution string `json:"solution"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		handle := strings.ToLower(strings.TrimSpace(req.Handle))
		if !handleRE.MatchString(handle) {
			httpJSONError(w, http.StatusBadRequest, "invalid handle")
			return
		}
		if defaultReserved[handle] || (cfg.Reserved != nil && cfg.Reserved(handle)) {
			httpJSONError(w, http.StatusConflict, "handle reserved")
			return
		}
		if len(req.Password) < 8 {
			httpJSONError(w, http.StatusBadRequest, "password too short (min 8)")
			return
		}
		// Anti-abuse gate before any state change.
		if err := cfg.Gate.Verify(r.Context(), req.Solution, clientIP(r)); err != nil {
			httpJSONError(w, http.StatusForbidden, "anti-abuse check failed")
			return
		}
		addr := handle + "@" + cfg.Domain
		if err := cfg.Provision(addr, req.Password); err != nil {
			httpJSONError(w, http.StatusConflict, "address unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"address": addr})
	})
	return mux
}

func httpJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
