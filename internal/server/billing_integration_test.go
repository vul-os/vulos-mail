package server_test

// TestBillingIntegration proves the gate→meter→suspension chain end-to-end
// through the REAL cloud adapter (integration/cloud), not a hand-rolled stub.
// It stands up an httptest server that mimics the vulos-cloud control-plane and
// wires cloud.NewEntitlements + cloud.NewUsage onto a real server.Manager backed
// by a real FS blob store.  Every assertion checks both the Manager's behaviour
// (allow/reject) and that the correct HTTP calls reached the cp stub.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	cloud "github.com/vul-os/vulos-mail/integration/cloud"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/server"
)

// cpStub is a minimal vulos-cloud control-plane stub.  It returns whatever plan
// is set via setPlan, records every call to /api/usage, and enforces the shared
// secret on every request.
type cpStub struct {
	mu      sync.Mutex
	plan    map[string]any // JSON-serialisable plan response
	usage   []map[string]any
	secret  string
	handler http.Handler
}

func newCPStub(secret string) *cpStub {
	s := &cpStub{secret: secret}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/mail/signup", s.handleSignup)
	mux.HandleFunc("/api/mail/auth", s.handleAuth)
	mux.HandleFunc("/api/mail/exists", s.handleExists)
	mux.HandleFunc("/api/entitlements", s.handleEntitlements)
	mux.HandleFunc("/api/usage", s.handleUsage)
	s.handler = mux
	return s
}

func (s *cpStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Relay-Auth") != s.secret {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	s.handler.ServeHTTP(w, r)
}

func (s *cpStub) setPlan(plan map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plan = plan
}

func (s *cpStub) usageEvents() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, len(s.usage))
	copy(out, s.usage)
	return out
}

func (s *cpStub) handleSignup(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *cpStub) handleAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	_ = json.NewEncoder(w).Encode(map[string]string{"account": body.Username})
}

func (s *cpStub) handleExists(w http.ResponseWriter, r *http.Request) {
	acct := r.URL.Query().Get("account")
	// Report every account as existing so Manager.account() doesn't reject them.
	_ = json.NewEncoder(w).Encode(map[string]bool{"exists": acct != ""})
}

func (s *cpStub) handleEntitlements(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	p := s.plan
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(p)
}

func (s *cpStub) handleUsage(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
		s.mu.Lock()
		s.usage = append(s.usage, body)
		s.mu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

// newBillingMgr creates a Manager wired to the cp stub via the real cloud
// adapter (cloud.NewEntitlements, cloud.NewUsage).
func newBillingMgr(t *testing.T, stub *cpStub, stubURL string) *server.Manager {
	t.Helper()
	dir := t.TempDir()
	blobs, err := blob.NewFS(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	mgr := server.NewManager(dir, blobs, nil)
	c := cloud.New(stubURL, stub.secret)
	mgr.Plans = cloud.NewEntitlements(c)
	mgr.Usage = cloud.NewUsage(c)
	return mgr
}

// usageKinds returns the "kind" field from every usage event captured by the stub.
func usageKinds(events []map[string]any) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		if k, ok := ev["kind"].(string); ok {
			out = append(out, k)
		}
	}
	return out
}

func hasUsageKind(events []map[string]any, kind string) bool {
	for _, ev := range events {
		if ev["kind"] == kind {
			return true
		}
	}
	return false
}

// proPlan builds a plan response with the given per-day cap and storage limit.
func proPlan(maxSendPerDay int, maxBytes int64, suspended bool) map[string]any {
	return map[string]any{
		"tier":            "pro",
		"max_send_per_day": maxSendPerDay,
		"max_bytes":       maxBytes,
		"max_addresses":   5,
		"suspended":       suspended,
	}
}

// validRaw returns a minimal RFC-5322 message with the given From/To.
func validRaw(from, to string) []byte {
	return []byte("From: " + from + "\r\nTo: " + to + "\r\nSubject: test\r\n\r\nhello\r\n")
}

// TestBillingIntegration is the cross-product billing chain test.
func TestBillingIntegration(t *testing.T) {
	const secret = "relay-secret"
	stub := newCPStub(secret)
	srv := httptest.NewServer(stub)
	defer srv.Close()

	ctx := context.Background()

	// ── (a) Pro plan, MaxSendPerDay=2: within cap allows send and meters it ──────

	t.Run("send_within_cap_succeeds_and_metered", func(t *testing.T) {
		stub.setPlan(proPlan(2, 0, false)) // no storage cap
		mgr := newBillingMgr(t, stub, srv.URL)
		// AddAccount with no Identity → in-memory creds (no cp roundtrip needed).
		if err := mgr.AddAccount("alice@vulos.to", "pw"); err != nil {
			t.Fatal(err)
		}
		stub.usage = nil // reset recorder

		// First send: within cap → should succeed.
		raw := validRaw("alice@vulos.to", "bob@remote.example")
		if err := mgr.SendRaw(ctx, "alice@vulos.to", []string{"bob@remote.example"}, raw); err != nil {
			t.Fatalf("first send within cap rejected: %v", err)
		}

		// The stub must have received a {product:mail, kind:send} POST.
		events := stub.usageEvents()
		if !hasUsageKind(events, "send") {
			t.Errorf("expected a 'send' usage event; got kinds=%v", usageKinds(events))
		}
		for _, ev := range events {
			if ev["kind"] == "send" {
				if p, ok := ev["product"].(string); !ok || p != "mail" {
					t.Errorf("usage event product=%q, want 'mail'", p)
				}
				if acct, ok := ev["account_id"].(string); !ok || !strings.EqualFold(acct, "alice@vulos.to") {
					t.Errorf("usage event account_id=%q, want alice@vulos.to", acct)
				}
				break
			}
		}
	})

	// ── (b) Over daily send cap → refused ────────────────────────────────────────

	t.Run("send_over_daily_cap_refused", func(t *testing.T) {
		stub.setPlan(proPlan(2, 0, false))
		mgr := newBillingMgr(t, stub, srv.URL)
		_ = mgr.AddAccount("carol@vulos.to", "pw")
		stub.usage = nil

		raw := validRaw("carol@vulos.to", "bob@remote.example")
		for i := 0; i < 2; i++ {
			if err := mgr.SendRaw(ctx, "carol@vulos.to", []string{"bob@remote.example"}, raw); err != nil {
				t.Fatalf("send %d within cap rejected: %v", i+1, err)
			}
		}
		// Third send is over the per-day cap.
		if err := mgr.SendRaw(ctx, "carol@vulos.to", []string{"bob@remote.example"}, raw); err == nil {
			t.Fatal("3rd send should be refused (over daily cap), but was accepted")
		}
		// CheckQuota alone should also refuse.
		if err := mgr.CheckQuota("carol@vulos.to", 100); err == nil {
			t.Fatal("CheckQuota should refuse when daily cap is exhausted")
		}
	})

	// ── (c) Suspended account → send refused ─────────────────────────────────────

	t.Run("suspended_account_send_refused", func(t *testing.T) {
		stub.setPlan(proPlan(100, 0, true)) // suspended=true
		mgr := newBillingMgr(t, stub, srv.URL)
		_ = mgr.AddAccount("dan@vulos.to", "pw")

		raw := validRaw("dan@vulos.to", "eve@remote.example")
		if err := mgr.SendRaw(ctx, "dan@vulos.to", []string{"eve@remote.example"}, raw); err == nil {
			t.Fatal("suspended account must not be able to send")
		}
		if err := mgr.CheckQuota("dan@vulos.to", 1); err == nil {
			t.Fatal("CheckQuota must refuse for a suspended account")
		}
	})

	// ── (d) MaxBytes storage cap ──────────────────────────────────────────────────
	// A delivery that pushes the mailbox past the cap is refused with a 452.
	// A delivery under the cap succeeds and a {kind:storage} event reaches the stub.

	t.Run("storage_cap_enforced_and_usage_metered", func(t *testing.T) {
		msg := []byte("From: sender@out.example\r\nTo: eve@vulos.to\r\nSubject: s\r\n\r\n" +
			strings.Repeat("X", 500) + "\r\n")
		// Allow exactly one message (cap is slightly above one message size).
		cap := int64(len(msg)) + 100
		stub.setPlan(map[string]any{
			"tier":            "pro",
			"max_send_per_day": 100,
			"max_bytes":       cap,
			"max_addresses":   5,
			"suspended":       false,
		})
		mgr := newBillingMgr(t, stub, srv.URL)
		_ = mgr.AddAccount("eve@vulos.to", "pw")
		stub.usage = nil

		// First delivery: within cap → must succeed.
		if err := mgr.Deliver(ctx, "eve@vulos.to", msg); err != nil {
			t.Fatalf("first delivery within storage cap rejected: %v", err)
		}

		// The stub must have received a {kind:storage} usage POST.
		events := stub.usageEvents()
		if !hasUsageKind(events, "storage") {
			t.Errorf("expected a 'storage' usage event; got kinds=%v", usageKinds(events))
		}
		for _, ev := range events {
			if ev["kind"] == "storage" {
				if p, ok := ev["product"].(string); !ok || p != "mail" {
					t.Errorf("storage usage event product=%q, want 'mail'", p)
				}
				break
			}
		}

		// Second delivery: would exceed the storage cap → must fail (452).
		if err := mgr.Deliver(ctx, "eve@vulos.to", msg); err == nil {
			t.Fatal("second delivery should be refused (over storage cap)")
		} else if !strings.Contains(err.Error(), "452") {
			t.Errorf("over-quota error should mention 452 temp rejection; got: %v", err)
		}
	})

	// ── (e) Assert cp stub received correct auth header and paths ─────────────────

	t.Run("cp_stub_rejects_wrong_auth", func(t *testing.T) {
		c := cloud.New(srv.URL, "WRONG-SECRET")
		ent := cloud.NewEntitlements(c)
		if _, err := ent.For(ctx, "alice@vulos.to"); err == nil {
			t.Fatal("entitlements call with wrong X-Relay-Auth must be rejected by cp stub")
		}
	})
}
