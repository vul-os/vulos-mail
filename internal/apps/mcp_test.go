package apps

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	appsplatform "github.com/vul-os/vulos-apps/appsplatform"
	"github.com/vul-os/vulos-apps/mcp"
)

// TestMCPInitializeAndToolsList mounts the Mail adapter behind an MCP handler and
// drives the initialize → tools/list handshake with an apps token, asserting the
// adapter's Act actions surface as MCP tools and its Read kinds as resources.
func TestMCPInitializeAndToolsList(t *testing.T) {
	reg := appsplatform.NewMemoryRegistry()
	created, err := reg.Create(appsplatform.CreateParams{
		Name:     "agent",
		OwnerID:  "owner",
		Products: []string{appsplatform.ProductMail},
		Scopes:   []string{appsplatform.ScopeAppsRead, appsplatform.ScopeAppsWrite},
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	h, err := mcp.NewHandler(mcp.MCPConfig{Adapter: testAdapter("http://engine.invalid"), Registry: reg})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	call := func(method string, params any) mcp.Response {
		body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(b)))
		req.Header.Set("Authorization", "Bearer "+created.Token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var resp mcp.Response
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s response: %v (%s)", method, err, w.Body.String())
		}
		return resp
	}

	if init := call("initialize", map[string]any{"protocolVersion": mcp.ProtocolVersion}); init.Error != nil {
		t.Fatalf("initialize error: %+v", init.Error)
	}

	tools := call("tools/list", nil)
	if tools.Error != nil {
		t.Fatalf("tools/list error: %+v", tools.Error)
	}
	tb, _ := json.Marshal(tools.Result)
	for _, want := range []string{"mail.send", "mail.draft", "mail.flag", "mail.move", "mail.delete"} {
		if !strings.Contains(string(tb), want) {
			t.Errorf("tools/list missing %q: %s", want, tb)
		}
	}

	resources := call("resources/list", nil)
	if resources.Error != nil {
		t.Fatalf("resources/list error: %+v", resources.Error)
	}
	rb, _ := json.Marshal(resources.Result)
	for _, want := range []string{"me", "folders", "messages", "message", "search"} {
		if !strings.Contains(string(rb), want) {
			t.Errorf("resources/list missing %q: %s", want, rb)
		}
	}
}
