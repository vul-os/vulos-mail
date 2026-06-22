package signup_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/seam"
	"github.com/vul-os/vulos-mail/internal/signup"
)

type failGate struct{}

func (failGate) Verify(context.Context, string, string) error { return errors.New("blocked") }

type stubIssuer struct{}

func (stubIssuer) IssueJSON() ([]byte, error) { return []byte(`{"challenge":"abc"}`), nil }

func newHandler(gate seam.SignupGate, provisioned *[]string) http.Handler {
	var mu sync.Mutex
	return signup.Handler(signup.Config{
		Domain: "vulos.to",
		Gate:   gate,
		Issuer: stubIssuer{},
		Provision: func(addr, _ string) error {
			mu.Lock()
			defer mu.Unlock()
			*provisioned = append(*provisioned, addr)
			return nil
		},
	})
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/signup", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSignupValid(t *testing.T) {
	var got []string
	h := newHandler(seam.OpenGate{}, &got)
	w := post(h, `{"handle":"alice","password":"longenough","solution":"x"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(got) != 1 || got[0] != "alice@vulos.to" {
		t.Fatalf("provisioned = %v", got)
	}
}

func TestSignupRejections(t *testing.T) {
	cases := []struct {
		name, body string
		gate       seam.SignupGate
		wantCode   int
	}{
		{"invalid handle", `{"handle":"a b!","password":"longenough","solution":"x"}`, seam.OpenGate{}, http.StatusBadRequest},
		{"reserved handle", `{"handle":"postmaster","password":"longenough","solution":"x"}`, seam.OpenGate{}, http.StatusConflict},
		{"short password", `{"handle":"bob","password":"short","solution":"x"}`, seam.OpenGate{}, http.StatusBadRequest},
		{"gate blocks", `{"handle":"carol","password":"longenough","solution":"x"}`, failGate{}, http.StatusForbidden},
		{"bad json", `{`, seam.OpenGate{}, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			h := newHandler(c.gate, &got)
			w := post(h, c.body)
			if w.Code != c.wantCode {
				t.Fatalf("status %d, want %d (%s)", w.Code, c.wantCode, w.Body.String())
			}
			if len(got) != 0 {
				t.Fatalf("must not provision on rejection, got %v", got)
			}
		})
	}
}

// TestSignupRateLimit proves a single IP is throttled (429) after exhausting
// its per-hour token bucket, and that the bucket refills as the clock advances.
func TestSignupRateLimit(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	var mu sync.Mutex
	var got []string
	h := signup.Handler(signup.Config{
		Domain: "vulos.to", Gate: seam.OpenGate{}, Issuer: stubIssuer{},
		Provision: func(addr, _ string) error {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, addr)
			return nil
		},
		RatePerHour: 5, // burst capped at 5
		Now:         func() time.Time { return clock },
	})

	post := func(handle string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/signup",
			strings.NewReader(`{"handle":"`+handle+`","password":"longenough","solution":"x"}`))
		r.RemoteAddr = "203.0.113.7:5000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	// 5 allowed (burst), 6th throttled.
	for i := 0; i < 5; i++ {
		if code := post("user" + string(rune('a'+i))); code != http.StatusOK {
			t.Fatalf("signup %d: want 200, got %d", i, code)
		}
	}
	if code := post("userx"); code != http.StatusTooManyRequests {
		t.Fatalf("over-limit signup: want 429, got %d", code)
	}
	// A different IP is unaffected.
	r := httptest.NewRequest(http.MethodPost, "/api/signup",
		strings.NewReader(`{"handle":"other","password":"longenough","solution":"x"}`))
	r.RemoteAddr = "198.51.100.9:5000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("distinct IP should be allowed, got %d", w.Code)
	}
	// Advance ~1h so the first IP's bucket refills, then it succeeds again.
	clock = clock.Add(time.Hour)
	if code := post("userz"); code != http.StatusOK {
		t.Fatalf("after refill: want 200, got %d", code)
	}
}

// TestSignupXFFOnlyTrustedFromProxy proves X-Forwarded-For is honoured only when
// the peer is in the trusted-proxy allowlist; an untrusted client cannot spoof
// its rate-limit key via XFF.
func TestSignupXFFOnlyTrustedFromProxy(t *testing.T) {
	clock := time.Unix(0, 0).UTC()
	h := signup.Handler(signup.Config{
		Domain: "vulos.to", Gate: seam.OpenGate{}, Issuer: stubIssuer{},
		Provision:      func(string, string) error { return nil },
		RatePerHour:    2,
		TrustedProxies: []string{"10.0.0.0/8"},
		Now:            func() time.Time { return clock },
	})

	post := func(remote, xff string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/signup",
			strings.NewReader(`{"handle":"newuser","password":"longenough","solution":"x"}`))
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	// Untrusted peer rotating XFF: the spoofed header is ignored, so all requests
	// share the same RemoteAddr bucket and the 3rd is throttled.
	post("203.0.113.1:1", "1.1.1.1")
	post("203.0.113.1:1", "2.2.2.2")
	if code := post("203.0.113.1:1", "3.3.3.3"); code != http.StatusTooManyRequests {
		t.Fatalf("untrusted XFF must not bypass the limit: want 429, got %d", code)
	}

	// Trusted proxy: distinct client IPs in XFF get distinct buckets.
	if code := post("10.1.2.3:1", "4.4.4.4"); code != http.StatusOK {
		t.Fatalf("trusted proxy, client 4.4.4.4: want 200, got %d", code)
	}
	if code := post("10.1.2.3:1", "5.5.5.5"); code != http.StatusOK {
		t.Fatalf("trusted proxy, client 5.5.5.5: want 200, got %d", code)
	}
}

func TestSignupChallengeEndpoint(t *testing.T) {
	var got []string
	h := newHandler(seam.OpenGate{}, &got)
	r := httptest.NewRequest(http.MethodGet, "/api/signup/challenge", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "challenge") {
		t.Fatalf("challenge endpoint: status %d body %s", w.Code, w.Body.String())
	}
}
