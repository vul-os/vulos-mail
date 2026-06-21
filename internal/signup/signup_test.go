package signup_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
