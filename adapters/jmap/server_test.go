package jmap_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jmapadapter "github.com/vul-os/vmail/adapters/jmap"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
)

func newServer(t *testing.T) (*httptest.Server, *account.Runtime) {
	t.Helper()
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(ctx, log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	be := &jmapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) {
		if u == "alice" && p == "pw" {
			return rt, nil
		}
		return nil, http.ErrNoCookie
	}}
	return httptest.NewServer(be.Handler()), rt
}

func msg(id, subject, body string) []byte {
	return []byte("From: x@y.com\r\nTo: alice@vmail.test\r\nSubject: " + subject + "\r\nMessage-ID: <" + id + ">\r\nDate: Mon, 02 Jan 2006 15:04:05 -0700\r\n\r\n" + body + "\r\n")
}

func apiCall(t *testing.T, srv *httptest.Server, method string, args map[string]any) map[string]any {
	t.Helper()
	reqBody, _ := json.Marshal(map[string]any{
		"using":       []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
		"methodCalls": [][]any{{method, args, "c0"}},
	})
	req, _ := http.NewRequest("POST", srv.URL+"/jmap/api", strings.NewReader(string(reqBody)))
	req.SetBasicAuth("alice", "pw")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%s: status %d", method, resp.StatusCode)
	}
	var out struct {
		MethodResponses [][]json.RawMessage `json:"methodResponses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", method, err)
	}
	if len(out.MethodResponses) != 1 {
		t.Fatalf("%s: expected 1 response, got %d", method, len(out.MethodResponses))
	}
	var name string
	_ = json.Unmarshal(out.MethodResponses[0][0], &name)
	if name == "error" {
		t.Fatalf("%s returned error: %s", method, out.MethodResponses[0][1])
	}
	var result map[string]any
	_ = json.Unmarshal(out.MethodResponses[0][1], &result)
	return result
}

func TestJMAPSessionAndEmailFlow(t *testing.T) {
	ctx := context.Background()
	srv, rt := newServer(t)
	defer srv.Close()

	idA, _ := rt.Ingest(ctx, msg("a@x", "Hello JMAP", "first body"), []model.LabelID{model.LabelInbox}, nil)
	rt.Ingest(ctx, msg("b@x", "Second", "second body"), []model.LabelID{model.LabelInbox}, nil)

	// Session resource.
	req, _ := http.NewRequest("GET", srv.URL+"/jmap/session", nil)
	req.SetBasicAuth("alice", "pw")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("session: %v / %v", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Unauthorized without creds.
	if r, _ := http.Get(srv.URL + "/jmap/session"); r == nil || r.StatusCode != 401 {
		t.Fatalf("expected 401 without auth")
	}

	// Mailbox/get includes inbox with 2 emails.
	mb := apiCall(t, srv, "Mailbox/get", map[string]any{"accountId": "alice"})
	var inboxTotal float64
	for _, raw := range mb["list"].([]any) {
		m := raw.(map[string]any)
		if m["id"] == string(model.LabelInbox) {
			inboxTotal = m["totalEmails"].(float64)
		}
	}
	if inboxTotal != 2 {
		t.Errorf("inbox totalEmails = %v, want 2", inboxTotal)
	}

	// Email/query inbox -> 2 ids, newest first.
	q := apiCall(t, srv, "Email/query", map[string]any{"accountId": "alice", "filter": map[string]any{"inMailbox": string(model.LabelInbox)}})
	qids := q["ids"].([]any)
	if len(qids) != 2 {
		t.Fatalf("query returned %d ids, want 2", len(qids))
	}

	// Email/get the first id.
	g := apiCall(t, srv, "Email/get", map[string]any{"accountId": "alice", "ids": []string{string(idA)}})
	list := g["list"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["subject"] != "Hello JMAP" {
		t.Fatalf("Email/get wrong: %+v", list)
	}

	// Email/set: mark idA $seen and add it to a (created) label via mailboxIds patch.
	rt.CreateLabel(ctx, "work", "Work", model.LabelUser)
	s := apiCall(t, srv, "Email/set", map[string]any{
		"accountId": "alice",
		"update": map[string]any{
			string(idA): map[string]any{
				"keywords/$seen":      true,
				"mailboxIds/work":     true,
				"mailboxIds/" + string(model.LabelInbox): nil, // remove from inbox
			},
		},
	})
	if _, ok := s["updated"].(map[string]any)[string(idA)]; !ok {
		t.Fatalf("Email/set did not update idA: %+v", s)
	}

	// Verify through the runtime.
	m, _ := rt.Message(idA)
	if !m.Flags[model.FlagSeen] {
		t.Error("idA should be $seen after Email/set")
	}
	if m.Labels[model.LabelInbox] {
		t.Error("idA should have been removed from inbox")
	}
	if !m.Labels["work"] {
		t.Error("idA should have label work")
	}
}

func TestJMAPIdentityGet(t *testing.T) {
	srv, _ := newServer(t)
	defer srv.Close()
	res := apiCall(t, srv, "Identity/get", map[string]any{"accountId": "alice"})
	list, _ := res["list"].([]any)
	if len(list) != 1 {
		t.Fatalf("Identity/get list = %d, want 1", len(list))
	}
	id := list[0].(map[string]any)
	if id["email"] != "alice" || id["id"] != "i0" {
		t.Errorf("identity wrong: %+v", id)
	}
}
