package jmap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jmapadapter "github.com/vul-os/vulos-mail/adapters/jmap"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/model"
)

// Full JMAP send flow: create a draft, then EmailSubmission/set referencing it
// via #creationId, with onSuccessUpdateEmail moving it Drafts -> Sent.
func TestJMAPSubmissionFlow(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, _ := account.Open(ctx, log, store, ids.NewGen(), nil)

	var mu sync.Mutex
	var sent [][]byte
	be := &jmapadapter.Backend{
		Auth: func(u, p string) (*account.Runtime, error) { return rt, nil },
		Submit: func(_ context.Context, _ string, raw []byte) error {
			mu.Lock()
			sent = append(sent, raw)
			mu.Unlock()
			return nil
		},
	}
	srv := httptest.NewServer(be.Handler())
	defer srv.Close()

	body := map[string]any{
		"using": []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail", "urn:ietf:params:jmap:submission"},
		"methodCalls": []any{
			[]any{"Email/set", map[string]any{
				"accountId": "alice",
				"create": map[string]any{
					"draft1": map[string]any{
						"mailboxIds": map[string]bool{"drafts": true},
						"keywords":   map[string]bool{"$draft": true},
						"from":       []map[string]string{{"email": "alice@vulos.to"}},
						"to":         []map[string]string{{"email": "bob@example.com"}},
						"subject":    "Hello from JMAP",
						"textBody":   []map[string]string{{"partId": "t", "type": "text/plain"}},
						"bodyValues": map[string]any{"t": map[string]string{"value": "body text"}},
					},
				},
			}, "c0"},
			[]any{"EmailSubmission/set", map[string]any{
				"accountId": "alice",
				"create": map[string]any{
					"sub1": map[string]any{"emailId": "#draft1", "identityId": "i0"},
				},
				"onSuccessUpdateEmail": map[string]any{
					"#sub1": map[string]any{
						"mailboxIds/drafts": nil,
						"mailboxIds/sent":   true,
						"keywords/$draft":   nil,
					},
				},
			}, "c1"},
		},
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+"/jmap/api", bytes.NewReader(raw))
	req.SetBasicAuth("alice", "pw")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("request: %v / %v", err, resp.StatusCode)
	}
	resp.Body.Close()

	// The message was actually submitted.
	mu.Lock()
	n := len(sent)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("Submit called %d times, want 1", n)
	}

	// Exactly one message exists, now in Sent (not Drafts).
	if got := rt.MessagesWithLabel(model.LabelSent); len(got) != 1 {
		t.Fatalf("Sent has %d, want 1", len(got))
	}
	if got := rt.MessagesWithLabel(model.LabelDrafts); len(got) != 0 {
		t.Errorf("Drafts has %d, want 0 (should have moved to Sent)", len(got))
	}
}
