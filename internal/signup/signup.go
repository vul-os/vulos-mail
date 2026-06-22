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
	"strconv"
	"strings"
	"sync"
	"time"

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
	// RatePerHour caps signups per client IP per hour (token bucket). <=0 uses a
	// sensible default; the Altcha PoW is the primary cost gate, this stops a
	// single source from grinding many accounts cheaply.
	RatePerHour int
	// TrustedProxies is the CIDR allowlist of fronting proxies whose
	// X-Forwarded-For header is honoured. Requests from any other RemoteAddr use
	// RemoteAddr directly, so a client can't spoof its rate-limit / abuse key by
	// sending its own XFF. Empty => XFF is never trusted.
	TrustedProxies []string
	// Now overrides the clock (tests).
	Now func() time.Time
}

// ipBucket is a per-IP token bucket (signups/hour) for the signup rate limit.
type ipBucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	perHour float64
	burst   float64
	now     func() time.Time

	mu      sync.Mutex
	buckets map[string]*ipBucket
}

func newRateLimiter(perHour int, now func() time.Time) *rateLimiter {
	if perHour <= 0 {
		perHour = 10
	}
	if now == nil {
		now = time.Now
	}
	burst := float64(perHour)
	if burst > 5 {
		burst = 5 // cap the initial burst regardless of the sustained rate
	}
	return &rateLimiter{perHour: float64(perHour), burst: burst, now: now, buckets: map[string]*ipBucket{}}
}

// allow consumes one token for ip, refilling at perHour. Returns false when the
// bucket is empty (over the limit).
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	b := rl.buckets[ip]
	if b == nil {
		b = &ipBucket{tokens: rl.burst, last: now}
		rl.buckets[ip] = b
	}
	// Refill since last seen.
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * (rl.perHour / 3600.0)
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	// Opportunistic GC of idle buckets keeps the map bounded.
	if len(rl.buckets) > 4096 {
		for k, v := range rl.buckets {
			if v.tokens >= rl.burst && k != ip {
				delete(rl.buckets, k)
			}
		}
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Handler returns an http.Handler serving:
//
//	GET  /api/signup/challenge  -> anti-abuse challenge JSON (if an Issuer is set)
//	POST /api/signup            -> { handle, password, solution } create mailbox
func Handler(cfg Config) http.Handler {
	limiter := newRateLimiter(cfg.RatePerHour, cfg.Now)
	trusted := parseCIDRs(cfg.TrustedProxies)
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
		ip := clientIP(r, trusted)
		// Per-IP rate limit before any work (parsing, PoW verify, provisioning).
		if !limiter.allow(ip) {
			httpJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
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
		if err := cfg.Gate.Verify(r.Context(), req.Solution, ip); err != nil {
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

func parseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		// Accept a bare IP as a /32 or /128.
		if !strings.Contains(c, "/") {
			if ip := net.ParseIP(c); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				c = c + "/" + strconv.Itoa(bits)
			}
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func isTrusted(ip string, trusted []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// clientIP returns the request's client IP. X-Forwarded-For is honoured ONLY
// when the immediate peer (RemoteAddr) is in the trusted-proxy allowlist;
// otherwise an untrusted client could spoof its rate-limit / abuse key with a
// crafted XFF header. The rightmost untrusted address in the XFF chain is used.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	}
	if !isTrusted(peer, trusted) {
		return peer
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}
	// Walk the chain right-to-left, skipping addresses we ourselves trust, and
	// return the first untrusted hop (the real client as our proxy saw it).
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		if !isTrusted(hop, trusted) {
			return hop
		}
	}
	return peer
}
