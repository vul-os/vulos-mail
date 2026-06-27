// Package apps implements the Vulos Mail ProductAdapter for the shared
// @vulos/apps Apps & Bots platform (github.com/vul-os/vulos-apps).
//
// It is the product seam that lets apps/bots act on and read mail through
// lilmail's /v1 JSON API. The platform owns auth, token hashing,
// product-targeting and scope enforcement; this adapter owns the mail-native
// semantics — translating the generic act/read envelopes into the concrete /v1
// calls lilmail actually supports.
//
// Credential custody (server-honest): apps do not hold a mailbox password. The
// adapter acts as a single configured "apps mailbox" (VULOS_MAIL_APPS_ACCOUNT /
// VULOS_MAIL_APPS_PASSWORD) and brokers each /v1 request to the lilmail engine
// using lilmail's CP-brokered credential mode (the same X-Vulos-Mail-* header
// model the webmail /v1 proxy and the Vulos Cloud control plane use). Because the
// adapter only ever sets these headers from trusted server config — never from
// client input — there is no header-forgery / SSRF surface here. The lilmail
// engine itself is untouched; this lives only in the vulos-mail product binary.
package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appsplatform "github.com/vul-os/vulos-apps/appsplatform"
)

// Config wires the adapter to the lilmail engine and the apps mailbox. It mirrors
// the values the webmail /v1 reverse proxy already uses (LILMAIL_ENGINE_URL,
// LILMAIL_BROKER_SECRET, IMAP/SMTP host+port) plus a dedicated service mailbox the
// apps act as.
type Config struct {
	// EngineURL is the lilmail engine base (LILMAIL_ENGINE_URL). Empty disables
	// runtime act/read with a clear error (the management surface still works).
	EngineURL string
	// BrokerSecret is lilmail's broker gate secret (LILMAIL_BROKER_SECRET). When
	// empty the engine ignores brokered credentials and /v1 calls 401.
	BrokerSecret string
	// Mailbox / Password are the apps service mailbox credentials
	// (VULOS_MAIL_APPS_ACCOUNT / VULOS_MAIL_APPS_PASSWORD). Apps act as this
	// mailbox; target selects an IMAP folder within it.
	Mailbox  string
	Password string
	// IMAP/SMTP connection settings lilmail dials with the brokered credential.
	IMAPHost string
	IMAPPort string
	SMTPHost string
	SMTPPort string
	// HTTPClient is optional; a 15s-timeout client is used when nil.
	HTTPClient *http.Client
}

// Adapter implements appsplatform.ProductAdapter for the Mail product.
type Adapter struct {
	cfg Config
	hc  *http.Client
}

var _ appsplatform.ProductAdapter = (*Adapter)(nil)

// New builds a Mail adapter from cfg.
func New(cfg Config) *Adapter {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Adapter{cfg: cfg, hc: hc}
}

// Configured reports whether the adapter can actually broker to an engine — i.e.
// an engine URL and apps-mailbox credentials are set. Used by the composition
// root to log a clear warning when the place is mounted but inert.
func (a *Adapter) Configured() bool {
	return a.cfg.EngineURL != "" && a.cfg.Mailbox != "" && a.cfg.Password != ""
}

// Product is the product this adapter serves.
func (a *Adapter) Product() string { return appsplatform.ProductMail }

// RequiredScope maps an action (Act) or kind (Read) to the scope it needs. Reads
// require apps:read; mail-changing actions require apps:write. auth.test needs no
// scope (it is handled by the platform, listed here for completeness).
func (a *Adapter) RequiredScope(actionOrKind string) string {
	switch actionOrKind {
	case "auth.test":
		return ""
	case "me", "folders", "messages", "message", "search":
		return appsplatform.ScopeAppsRead
	default:
		// All mail-changing actions (send, draft, flag, move, delete, the incoming
		// webhook). An unknown action falls here too and is rejected in Act with a
		// clear "unsupported" error after the scope check.
		return appsplatform.ScopeAppsWrite
	}
}

// CanAccessTarget gates an app's access to a target. For Mail the place is a
// single configured apps mailbox; a non-empty target is an IMAP folder within it,
// and every folder is in-scope for any app that targets mail (the platform has
// already enforced product-targeting + scope). lilmail returns a real error for a
// nonexistent folder, so we do not pre-check it here (no extra round-trip). An
// empty target is target-less and always accessible.
func (a *Adapter) CanAccessTarget(_ *appsplatform.App, _ string) (allowed, exists bool) {
	return true, true
}

// Act performs a mail action via lilmail /v1. Supported actions map 1:1 to /v1
// endpoints lilmail actually implements:
//
//	mail.send / message.post  -> POST   /v1/messages              (compose + send)
//	mail.draft                -> POST   /v1/drafts                (save a draft)
//	mail.flag                 -> PATCH  /v1/messages/{uid}/flags  (label/flag)
//	mail.move                 -> POST   /v1/messages/{uid}/move   (file to a folder)
//	mail.delete               -> DELETE /v1/messages/{uid}        (trash / hard-delete)
//	incoming_webhook          -> POST   /v1/messages              (send a notification)
//
// target (when present) is the IMAP folder the message lives in.
func (a *Adapter) Act(ctx context.Context, app *appsplatform.App, req appsplatform.ActionRequest, emit appsplatform.EmitFunc) (any, error) {
	switch req.Action {
	case "mail.send", "message.post", "mail.compose":
		res, err := a.do(ctx, http.MethodPost, "/v1/messages", nil, req.Payload)
		if err == nil && emit != nil {
			emit("message.sent", map[string]any{"app_id": app.ID, "by": app.AccountID()}, nil)
		}
		return res, err

	case "mail.draft":
		return a.do(ctx, http.MethodPost, "/v1/drafts", nil, req.Payload)

	case "mail.flag":
		uid, err := uidOf(req.Payload)
		if err != nil {
			return nil, err
		}
		// lilmail reads {flag, add} from the body; extra fields (uid) are ignored,
		// so the app's payload is forwarded as-is.
		return a.do(ctx, http.MethodPatch, "/v1/messages/"+url.PathEscape(uid)+"/flags", folderQuery(req.Target), req.Payload)

	case "mail.move":
		uid, err := uidOf(req.Payload)
		if err != nil {
			return nil, err
		}
		// lilmail reads {toFolder} from the body; uid is ignored there.
		return a.do(ctx, http.MethodPost, "/v1/messages/"+url.PathEscape(uid)+"/move", folderQuery(req.Target), req.Payload)

	case "mail.delete":
		uid, err := uidOf(req.Payload)
		if err != nil {
			return nil, err
		}
		q := folderQuery(req.Target)
		if hardOf(req.Payload) {
			if q == nil {
				q = url.Values{}
			}
			q.Set("hard", "true")
		}
		return a.do(ctx, http.MethodDelete, "/v1/messages/"+url.PathEscape(uid), q, nil)

	case "incoming_webhook":
		body, err := a.webhookToMessage(app, req.Payload)
		if err != nil {
			return nil, err
		}
		res, err := a.do(ctx, http.MethodPost, "/v1/messages", nil, body)
		if err == nil && emit != nil {
			emit("message.sent", map[string]any{"app_id": app.ID, "by": app.AccountID()}, nil)
		}
		return res, err

	default:
		return nil, fmt.Errorf("unsupported mail action %q", req.Action)
	}
}

// Read returns mail content via lilmail /v1. Supported kinds:
//
//	me        -> GET /v1/me                              (the apps mailbox identity)
//	folders   -> GET /v1/folders                         (folder list)
//	messages  -> GET /v1/messages?folder=&limit=         (a folder listing)
//	message   -> GET /v1/messages/{uid}?folder=          (one message; params.uid)
//	search    -> GET /v1/search?folder=&q=&limit=        (search; params.q)
func (a *Adapter) Read(ctx context.Context, _ *appsplatform.App, req appsplatform.ReadRequest) (any, error) {
	switch req.Kind {
	case "me":
		return a.do(ctx, http.MethodGet, "/v1/me", nil, nil)

	case "folders":
		return a.do(ctx, http.MethodGet, "/v1/folders", nil, nil)

	case "messages":
		q := url.Values{}
		if req.Target != "" {
			q.Set("folder", req.Target)
		}
		if l := req.Params["limit"]; l != "" {
			q.Set("limit", l)
		}
		return a.do(ctx, http.MethodGet, "/v1/messages", q, nil)

	case "message":
		uid := strings.TrimSpace(req.Params["uid"])
		if uid == "" {
			return nil, errors.New("read message: params.uid required")
		}
		return a.do(ctx, http.MethodGet, "/v1/messages/"+url.PathEscape(uid), folderQuery(req.Target), nil)

	case "search":
		q := url.Values{}
		if req.Target != "" {
			q.Set("folder", req.Target)
		}
		q.Set("q", req.Params["q"])
		if l := req.Params["limit"]; l != "" {
			q.Set("limit", l)
		}
		return a.do(ctx, http.MethodGet, "/v1/search", q, nil)

	default:
		return nil, fmt.Errorf("unsupported mail read kind %q", req.Kind)
	}
}

// do brokers a single /v1 request to the lilmail engine with the apps-mailbox
// credential and returns the decoded JSON response (or the raw text when the body
// is not JSON). A non-2xx status becomes an error carrying the engine's message.
func (a *Adapter) do(ctx context.Context, method, path string, query url.Values, body json.RawMessage) (any, error) {
	if a.cfg.EngineURL == "" {
		return nil, errors.New("mail engine not configured (set LILMAIL_ENGINE_URL)")
	}
	if a.cfg.Mailbox == "" || a.cfg.Password == "" {
		return nil, errors.New("apps mailbox not configured (set VULOS_MAIL_APPS_ACCOUNT and VULOS_MAIL_APPS_PASSWORD)")
	}
	u := strings.TrimRight(a.cfg.EngineURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	a.setBrokerHeaders(req.Header)

	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mail engine request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("mail engine %s: %s", resp.Status, msg)
	}
	if len(bytes.TrimSpace(respBody)) == 0 {
		return map[string]any{"ok": true}, nil
	}
	var out any
	if err := json.Unmarshal(respBody, &out); err != nil {
		return string(respBody), nil
	}
	return out, nil
}

// setBrokerHeaders sets lilmail's CP-brokered credential headers for the apps
// mailbox. These are always derived from trusted server config (never client
// input), so there is no forgery surface; the CalDAV/CardDAV base-URL headers are
// deliberately NOT set (the adapter exposes only mail, not calendar/contacts).
func (a *Adapter) setBrokerHeaders(h http.Header) {
	h.Set("X-Vulos-Broker-Auth", a.cfg.BrokerSecret)
	h.Set("X-Vulos-Mail-Provider", "imap")
	h.Set("X-Vulos-Mail-Email", a.cfg.Mailbox)
	h.Set("X-Vulos-Mail-Username", a.cfg.Mailbox)
	h.Set("X-Vulos-Mail-Auth", "plain")
	h.Set("X-Vulos-Mail-Secret", a.cfg.Password)
	h.Set("X-Vulos-Mail-Imap-Host", a.cfg.IMAPHost)
	h.Set("X-Vulos-Mail-Imap-Port", a.cfg.IMAPPort)
	h.Set("X-Vulos-Mail-Smtp-Host", a.cfg.SMTPHost)
	h.Set("X-Vulos-Mail-Smtp-Port", a.cfg.SMTPPort)
}

// webhookToMessage turns an incoming-webhook body into a /v1/messages compose
// payload. The webhook is the simplest integration: a script POSTs JSON and the
// app sends an email. Recipients come from the body's "to", falling back to the
// app's default_target when that is an email address.
func (a *Adapter) webhookToMessage(app *appsplatform.App, payload json.RawMessage) (json.RawMessage, error) {
	var p struct {
		To      []string `json:"to"`
		Cc      []string `json:"cc"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
		HTML    string   `json:"html"`
	}
	_ = json.Unmarshal(payload, &p)
	if len(p.To) == 0 && strings.Contains(app.DefaultTarget, "@") {
		p.To = []string{app.DefaultTarget}
	}
	if len(p.To) == 0 {
		return nil, errors.New("incoming webhook: no recipient (set the app's default_target to an email address, or POST {\"to\":[...]})")
	}
	if strings.TrimSpace(p.Subject) == "" {
		name := app.Name
		if name == "" {
			name = "Vulos Mail"
		}
		p.Subject = name + " notification"
	}
	if strings.TrimSpace(p.Text) == "" && strings.TrimSpace(p.HTML) == "" {
		p.Text = string(payload)
	}
	return json.Marshal(map[string]any{
		"to": p.To, "cc": p.Cc, "subject": p.Subject, "text": p.Text, "html": p.HTML,
	})
}

// folderQuery builds a ?folder= query for a non-empty target, else nil.
func folderQuery(target string) url.Values {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	return url.Values{"folder": {target}}
}

// uidOf extracts a message UID from a payload, accepting either a JSON string or
// number (IMAP UIDs are numeric, but tolerate string ids).
func uidOf(payload json.RawMessage) (string, error) {
	var p struct {
		UID any `json:"uid"`
	}
	if err := json.Unmarshal(payload, &p); err == nil {
		switch v := p.UID.(type) {
		case string:
			if v != "" {
				return v, nil
			}
		case float64:
			return strconv.FormatInt(int64(v), 10), nil
		case json.Number:
			return v.String(), nil
		}
	}
	return "", errors.New("payload.uid required")
}

// hardOf reports whether a delete payload requested a hard (permanent) delete.
func hardOf(payload json.RawMessage) bool {
	var p struct {
		Hard bool `json:"hard"`
	}
	_ = json.Unmarshal(payload, &p)
	return p.Hard
}
