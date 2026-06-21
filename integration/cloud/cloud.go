// Package cloud is the OPTIONAL vulos-cloud (control-plane) adapter for
// vulos-mail. It implements the seam interfaces against the cp HTTP API so a
// Vulos-hosted deployment can centralize identity, entitlements (Paystack/ZAR
// billing), and usage metering.
//
// This package is NOT imported by the mail core (internal/* or adapters/*) —
// only by the command's composition root, and only when VULOS_CP_URL is set. The
// OSS server runs entirely without it. Service-to-service calls authenticate with
// a shared secret (cp's X-Relay-Auth), matching how cp's relay PoPs call it.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vul-os/vulos-mail/internal/seam"
)

// Client talks to the vulos-cloud control plane.
type Client struct {
	base   string
	secret string
	hc     *http.Client
}

// New returns a cp client for baseURL authenticating with sharedSecret. Returns
// nil if baseURL is empty (cloud integration disabled — the default).
func New(baseURL, sharedSecret string) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{
		base:   strings.TrimRight(baseURL, "/"),
		secret: sharedSecret,
		hc:     &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-Relay-Auth", c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("cp %s %s: status %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// --- seam.Identity ---

// Identity authenticates against cp and treats cp as the source of truth for
// account existence. Account creation is owned by cp (Provision is unsupported).
type Identity struct{ c *Client }

// NewIdentity returns a cp-backed identity (nil if cloud is disabled).
func NewIdentity(c *Client) *Identity {
	if c == nil {
		return nil
	}
	return &Identity{c: c}
}

// Authenticate verifies credentials via cp's mail-auth endpoint.
func (i *Identity) Authenticate(ctx context.Context, username, password string) (string, error) {
	var out struct {
		Account string `json:"account"`
	}
	err := i.c.do(ctx, http.MethodPost, "/api/mail/auth",
		map[string]string{"username": username, "password": password}, &out)
	if err != nil || out.Account == "" {
		return "", errors.New("cloud: invalid credentials")
	}
	return strings.ToLower(out.Account), nil
}

// Exists asks cp whether the account is provisioned.
func (i *Identity) Exists(account string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out struct {
		Exists bool `json:"exists"`
	}
	if err := i.c.do(ctx, http.MethodGet, "/api/mail/exists?account="+url.QueryEscape(strings.ToLower(account)), nil, &out); err != nil {
		return false
	}
	return out.Exists
}

// Provision creates a new FREE account in cp (the "sign up via Vulos Mail" flow):
// the @vulos.to address becomes the cp identity. cp assigns the free tier and
// owns reserved-handle/abuse policy; mail's signup gate has already run.
func (i *Identity) Provision(ctx context.Context, account, password string) error {
	return i.c.do(ctx, http.MethodPost, "/api/mail/signup",
		map[string]string{"account": strings.ToLower(account), "password": password}, nil)
}

// --- seam.Entitlements ---

// Entitlements maps cp's quota response to a seam.Plan.
type Entitlements struct{ c *Client }

// NewEntitlements returns a cp-backed entitlement source (nil if disabled).
func NewEntitlements(c *Client) *Entitlements {
	if c == nil {
		return nil
	}
	return &Entitlements{c: c}
}

// For fetches the account's tier/quota from cp's quota endpoint.
func (e *Entitlements) For(ctx context.Context, account string) (seam.Plan, error) {
	var out struct {
		Tier          string `json:"tier"`
		MaxSendPerDay int    `json:"max_send_per_day"`
		MaxBytes      int64  `json:"max_bytes"`
		MaxAddresses  int    `json:"max_addresses"`
		Suspended     bool   `json:"suspended"`
	}
	if err := e.c.do(ctx, http.MethodGet, "/api/entitlements?product=mail&account_id="+url.QueryEscape(strings.ToLower(account)), nil, &out); err != nil {
		return seam.Plan{}, err
	}
	return seam.Plan{
		Tier: out.Tier, MaxSendPerDay: out.MaxSendPerDay, MaxBytes: out.MaxBytes,
		MaxAddresses: out.MaxAddresses, Suspended: out.Suspended,
	}, nil
}

// --- seam.Usage ---

// Usage pushes metered events to cp (fire-and-forget; metering must never block
// or fail the mail path).
type Usage struct{ c *Client }

// NewUsage returns a cp-backed usage sink (nil if disabled).
func NewUsage(c *Client) *Usage {
	if c == nil {
		return nil
	}
	return &Usage{c: c}
}

// Report posts the event to cp's usage ingest, best-effort.
func (u *Usage) Report(ctx context.Context, ev seam.Event) {
	_ = u.c.do(ctx, http.MethodPost, "/api/usage", map[string]any{
		"product": "mail", "kind": ev.Kind, "account_id": ev.Account,
		"count": ev.Count, "bytes": ev.Bytes,
	}, nil)
}

// Compile-time checks that the adapter satisfies the seam interfaces.
var (
	_ seam.Identity     = (*Identity)(nil)
	_ seam.Entitlements = (*Entitlements)(nil)
	_ seam.Usage        = (*Usage)(nil)
)
