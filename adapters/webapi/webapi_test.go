package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// submitCall records the arguments Submit was invoked with.
type submitCall struct {
	called bool
	from   string
	to     []string
	raw    []byte
}

// newBackend builds a Backend whose AuthKey accepts validKey and whose Submit
// records its arguments into call (returning submitErr).
func newBackend(validKey string, submitErr error, call *submitCall) *Backend {
	return &Backend{
		AuthKey: func(apiKey string) (string, bool) {
			if apiKey == validKey {
				return "acct-123", true
			}
			return "", false
		},
		Submit: func(_ context.Context, from string, to []string, raw []byte) error {
			call.called = true
			call.from = from
			call.to = to
			call.raw = raw
			return submitErr
		},
	}
}

func TestHandleSend(t *testing.T) {
	const validKey = "secret-key"

	tests := []struct {
		name       string
		authHeader string // raw Authorization header; "" means omit
		body       string
		wantStatus int
		wantSubmit bool
	}{
		{
			name:       "valid text+html",
			authHeader: "Bearer " + validKey,
			body:       `{"from":"alice@example.com","to":["bob@example.org"],"subject":"Hello","text":"plain body","html":"<p>html body</p>"}`,
			wantStatus: http.StatusOK,
			wantSubmit: true,
		},
		{
			name:       "valid text only",
			authHeader: "Bearer " + validKey,
			body:       `{"from":"alice@example.com","to":["bob@example.org"],"subject":"Hi","text":"only text"}`,
			wantStatus: http.StatusOK,
			wantSubmit: true,
		},
		{
			name:       "missing auth header",
			authHeader: "",
			body:       `{"from":"a@b.com","to":["c@d.com"],"subject":"s","text":"t"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme",
			authHeader: "Basic " + validKey,
			body:       `{"from":"a@b.com","to":["c@d.com"],"subject":"s","text":"t"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid key",
			authHeader: "Bearer wrong-key",
			body:       `{"from":"a@b.com","to":["c@d.com"],"subject":"s","text":"t"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed JSON",
			authHeader: "Bearer " + validKey,
			body:       `{not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing from",
			authHeader: "Bearer " + validKey,
			body:       `{"to":["c@d.com"],"subject":"s","text":"t"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing to",
			authHeader: "Bearer " + validKey,
			body:       `{"from":"a@b.com","to":[],"subject":"s","text":"t"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing subject",
			authHeader: "Bearer " + validKey,
			body:       `{"from":"a@b.com","to":["c@d.com"],"text":"t"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var call submitCall
			b := newBackend(validKey, nil, &call)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/send", strings.NewReader(tt.body))
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			b.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if call.called != tt.wantSubmit {
				t.Fatalf("Submit called = %v, want %v", call.called, tt.wantSubmit)
			}
		})
	}
}

func TestHandleSendComposesMessage(t *testing.T) {
	const validKey = "secret-key"
	var call submitCall
	b := newBackend(validKey, nil, &call)

	body := `{"from":"Alice <alice@example.com>","to":["bob@example.org"],"cc":["carol@example.net"],"subject":"Greetings Earth","text":"hello in plain","html":"<p>hello in html</p>"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+validKey)
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// Submit must receive the envelope sender and all recipients (To + Cc).
	if call.from != "Alice <alice@example.com>" {
		t.Errorf("Submit from = %q, want full from string", call.from)
	}
	wantTo := []string{"bob@example.org", "carol@example.net"}
	if len(call.to) != len(wantTo) {
		t.Fatalf("Submit to = %v, want %v", call.to, wantTo)
	}
	for i, r := range wantTo {
		if call.to[i] != r {
			t.Errorf("Submit to[%d] = %q, want %q", i, call.to[i], r)
		}
	}

	raw := string(call.raw)
	if !strings.Contains(raw, "Greetings Earth") {
		t.Errorf("composed message missing subject; got:\n%s", raw)
	}
	if !strings.Contains(raw, "multipart/alternative") {
		t.Errorf("composed message missing multipart/alternative; got:\n%s", raw)
	}
	// quoted-printable encodes "hello in plain" verbatim (no special chars).
	if !strings.Contains(raw, "hello in plain") {
		t.Errorf("composed message missing text body; got:\n%s", raw)
	}
	if !strings.Contains(raw, "Message-Id:") && !strings.Contains(raw, "Message-ID:") {
		t.Errorf("composed message missing Message-ID; got:\n%s", raw)
	}
	if !strings.Contains(raw, "Date:") {
		t.Errorf("composed message missing Date; got:\n%s", raw)
	}

	// Response JSON includes the message ID and accepted count.
	var resp sendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Accepted != 2 {
		t.Errorf("accepted = %d, want 2", resp.Accepted)
	}
	if resp.ID == "" {
		t.Error("response id is empty")
	}
	if !strings.Contains(raw, resp.ID) {
		t.Errorf("response id %q not found in composed Message-ID", resp.ID)
	}
}

func TestHandleSendSubmitError(t *testing.T) {
	const validKey = "secret-key"
	var call submitCall
	b := newBackend(validKey, errSubmit, &call)

	body := `{"from":"a@b.com","to":["c@d.com"],"subject":"s","text":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+validKey)
	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !call.called {
		t.Error("Submit should have been called")
	}
}

func TestMessageID(t *testing.T) {
	id1 := messageID("example.com")
	id2 := messageID("example.com")
	if id1 == id2 {
		t.Errorf("messageID returned duplicate values: %q", id1)
	}
	if !strings.HasSuffix(id1, "@example.com") {
		t.Errorf("messageID = %q, want @example.com suffix", id1)
	}
	if strings.ContainsAny(id1, "<>") {
		t.Errorf("messageID = %q should not contain angle brackets", id1)
	}
	if got := messageID(""); !strings.HasSuffix(got, "@localhost") {
		t.Errorf("messageID(\"\") = %q, want @localhost suffix", got)
	}
}

// errSubmit is a sentinel error used to simulate a delivery failure.
var errSubmit = errSubmitType("submit failed")

type errSubmitType string

func (e errSubmitType) Error() string { return string(e) }
