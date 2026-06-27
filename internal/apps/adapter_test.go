package apps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsplatform "github.com/vul-os/vulos-apps/appsplatform"
)

// fakeEngine records the last /v1 request a brokered call made.
type capture struct {
	method string
	path   string
	query  string
	header http.Header
	body   string
}

func newEngine(t *testing.T, status int, resp string) (*httptest.Server, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
		}
		cap.method, cap.path, cap.query, cap.header, cap.body = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func testAdapter(engineURL string) *Adapter {
	return New(Config{
		EngineURL: engineURL, BrokerSecret: "s3cr3t",
		Mailbox: "bot@vulos.to", Password: "pw",
		IMAPHost: "imap.host", IMAPPort: "993", SMTPHost: "smtp.host", SMTPPort: "587",
	})
}

func TestProductAndScopes(t *testing.T) {
	a := testAdapter("http://x")
	if a.Product() != appsplatform.ProductMail {
		t.Fatalf("Product()=%q", a.Product())
	}
	if got := a.RequiredScope("messages"); got != appsplatform.ScopeAppsRead {
		t.Errorf("read scope=%q", got)
	}
	if got := a.RequiredScope("mail.send"); got != appsplatform.ScopeAppsWrite {
		t.Errorf("send scope=%q", got)
	}
	if got := a.RequiredScope("auth.test"); got != "" {
		t.Errorf("auth.test scope=%q want empty", got)
	}
}

func TestActSendBrokersHeaders(t *testing.T) {
	srv, cap := newEngine(t, 200, `{"id":"42"}`)
	a := testAdapter(srv.URL)
	app := &appsplatform.App{ID: "abc", Name: "Notifier"}
	payload := json.RawMessage(`{"to":["a@b.c"],"subject":"hi","text":"yo"}`)
	res, err := a.Act(context.Background(), app, appsplatform.ActionRequest{Action: "mail.send", Payload: payload}, nil)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if cap.method != http.MethodPost || cap.path != "/v1/messages" {
		t.Fatalf("got %s %s", cap.method, cap.path)
	}
	if cap.header.Get("X-Vulos-Broker-Auth") != "s3cr3t" || cap.header.Get("X-Vulos-Mail-Email") != "bot@vulos.to" {
		t.Errorf("broker headers missing: %v", cap.header)
	}
	if cap.header.Get("X-Vulos-Mail-Secret") != "pw" {
		t.Errorf("broker secret header missing")
	}
	if m, ok := res.(map[string]any); !ok || m["id"] != "42" {
		t.Errorf("result passthrough = %#v", res)
	}
}

func TestActFlagUsesUIDAndFolder(t *testing.T) {
	srv, cap := newEngine(t, 200, `{}`)
	a := testAdapter(srv.URL)
	payload := json.RawMessage(`{"uid":17,"flag":"\\Seen","add":true}`)
	_, err := a.Act(context.Background(), &appsplatform.App{ID: "x"}, appsplatform.ActionRequest{Action: "mail.flag", Target: "Archive", Payload: payload}, nil)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if cap.method != http.MethodPatch || cap.path != "/v1/messages/17/flags" {
		t.Fatalf("got %s %s", cap.method, cap.path)
	}
	if cap.query != "folder=Archive" {
		t.Errorf("query=%q", cap.query)
	}
}

func TestReadMessagesQuery(t *testing.T) {
	srv, cap := newEngine(t, 200, `{"messages":[]}`)
	a := testAdapter(srv.URL)
	_, err := a.Read(context.Background(), &appsplatform.App{}, appsplatform.ReadRequest{
		Kind: "messages", Target: "INBOX", Params: map[string]string{"limit": "5"},
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cap.path != "/v1/messages" || cap.query != "folder=INBOX&limit=5" {
		t.Fatalf("got %s ?%s", cap.path, cap.query)
	}
}

func TestUnsupportedAction(t *testing.T) {
	a := testAdapter("http://x")
	if _, err := a.Act(context.Background(), &appsplatform.App{}, appsplatform.ActionRequest{Action: "nope"}, nil); err == nil {
		t.Fatal("expected error for unsupported action")
	}
}

func TestEngineErrorSurfaced(t *testing.T) {
	srv, _ := newEngine(t, 502, `{"error":"mail server connection failed"}`)
	a := testAdapter(srv.URL)
	_, err := a.Read(context.Background(), &appsplatform.App{}, appsplatform.ReadRequest{Kind: "folders"})
	if err == nil {
		t.Fatal("expected engine error")
	}
}

func TestNotConfigured(t *testing.T) {
	a := New(Config{}) // no engine, no mailbox
	if _, err := a.Read(context.Background(), &appsplatform.App{}, appsplatform.ReadRequest{Kind: "folders"}); err == nil {
		t.Fatal("expected not-configured error")
	}
	if a.Configured() {
		t.Fatal("Configured() should be false")
	}
}

func TestIncomingWebhookToDefaultTarget(t *testing.T) {
	srv, cap := newEngine(t, 200, `{}`)
	a := testAdapter(srv.URL)
	app := &appsplatform.App{ID: "x", Name: "Deploy", DefaultTarget: "ops@vulos.to"}
	_, err := a.Act(context.Background(), app, appsplatform.ActionRequest{
		Action: "incoming_webhook", Payload: json.RawMessage(`{"text":"deploy ok"}`),
	}, nil)
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if cap.path != "/v1/messages" {
		t.Fatalf("path=%s", cap.path)
	}
	var sent struct {
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}
	if err := json.Unmarshal([]byte(cap.body), &sent); err != nil {
		t.Fatalf("body: %v (%s)", err, cap.body)
	}
	if len(sent.To) != 1 || sent.To[0] != "ops@vulos.to" || sent.Text != "deploy ok" {
		t.Errorf("composed = %#v", sent)
	}
}
