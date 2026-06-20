package jmap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jmapadapter "github.com/vul-os/vulos-mail/adapters/jmap"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/model"
)

func mkRuntime(t *testing.T) *account.Runtime {
	t.Helper()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(context.Background(), log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

// TestJMAPIgnoresRequestAccountId is the JMAP authorization regression: the
// server must operate on the *authenticated* runtime, never on the accountId the
// client puts in the request. We authenticate as bob (whose runtime is empty)
// but ask for alice's data with `accountId:"alice"` and alice's real message id.
// Bob must get nothing back.
func TestJMAPIgnoresRequestAccountId(t *testing.T) {
	ctx := context.Background()

	alice := mkRuntime(t)
	aliceID, err := alice.Ingest(ctx,
		[]byte("From: x@out.example\r\nTo: alice@vulos.to\r\nSubject: secret\r\n\r\nbody\r\n"),
		[]model.LabelID{model.LabelInbox}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bob := mkRuntime(t) // empty mailbox

	// Auth always resolves to bob, regardless of the Basic user supplied.
	be := &jmapadapter.Backend{
		Auth: func(_, _ string) (*account.Runtime, error) { return bob, nil },
	}
	srv := httptest.NewServer(be.Handler())
	defer srv.Close()

	// Email/get with alice's accountId AND alice's real id, authenticated as bob.
	reqBody := map[string]any{
		"using": []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
		"methodCalls": []any{
			[]any{"Email/get", map[string]any{
				"accountId":  "alice@vulos.to",
				"ids":        []string{string(aliceID)},
				"properties": []string{"subject", "preview", "bodyValues"},
			}, "c0"},
			[]any{"Email/query", map[string]any{"accountId": "alice@vulos.to"}, "c1"},
		},
	}
	buf, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", srv.URL+"/jmap/api", bytes.NewReader(buf))
	req.SetBasicAuth("bob@vulos.to", "irrelevant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var out struct {
		MethodResponses []json.RawMessage `json:"methodResponses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.MethodResponses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(out.MethodResponses))
	}

	// Email/get: the list must be empty and alice's id reported notFound.
	var get []json.RawMessage
	_ = json.Unmarshal(out.MethodResponses[0], &get)
	var getArgs struct {
		AccountID string   `json:"accountId"`
		List      []any    `json:"list"`
		NotFound  []string `json:"notFound"`
	}
	_ = json.Unmarshal(get[1], &getArgs)
	if len(getArgs.List) != 0 {
		t.Fatalf("CROSS-ACCOUNT LEAK via Email/get: bob received %d of alice's emails", len(getArgs.List))
	}
	if getArgs.AccountID == "alice@vulos.to" {
		t.Fatalf("server echoed the client accountId instead of the authenticated account: %q", getArgs.AccountID)
	}

	// Email/query: bob's mailbox is empty, so total must be 0.
	var query []json.RawMessage
	_ = json.Unmarshal(out.MethodResponses[1], &query)
	var queryArgs struct {
		IDs   []string `json:"ids"`
		Total int      `json:"total"`
	}
	_ = json.Unmarshal(query[1], &queryArgs)
	if queryArgs.Total != 0 || len(queryArgs.IDs) != 0 {
		t.Fatalf("CROSS-ACCOUNT LEAK via Email/query: bob saw %d ids", len(queryArgs.IDs))
	}
}

// TestJMAPRejectsBadCredentials verifies the JMAP edge 401s when auth fails and
// when no Authorization header is present.
func TestJMAPRejectsBadCredentials(t *testing.T) {
	be := &jmapadapter.Backend{
		Auth: func(string, string) (*account.Runtime, error) { return nil, errInvalid },
	}
	srv := httptest.NewServer(be.Handler())
	defer srv.Close()

	// No auth header.
	resp, err := http.Get(srv.URL + "/jmap/session")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing auth: want 401, got %d", resp.StatusCode)
	}

	// Bad creds.
	req, _ := http.NewRequest("GET", srv.URL+"/jmap/session", nil)
	req.SetBasicAuth("alice@vulos.to", "wrong")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad creds: want 401, got %d", resp2.StatusCode)
	}
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

const errInvalid = sentinelErr("invalid credentials")
