package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/vul-os/vulos-mail/internal/seam"
	"github.com/vul-os/vulos-mail/internal/server"
)

// provisionMailboxHandler serves POST /api/admin/provision-mailbox: the free-mail
// mailbox provisioning seam used by Vulos Cloud's free-org-mail feature. It is
// broker-gated with the same X-Vulos-Broker-Auth / LILMAIL_BROKER_SECRET pattern
// as the other admin/brokered routes.
//
// It creates/ensures a mailbox <localpart>@<domain> on the configured mail server
// and is idempotent: provisioning an address that already exists succeeds without
// changing it. When the active identity provider does not own account creation
// (it returns seam.ErrUnsupported — e.g. a deployment whose identity is managed by
// an external system), the endpoint returns 501 so the caller knows to provision
// through that system instead. This is the clear, documented self-provision seam.
func provisionMailboxHandler(mgr *server.Manager, defaultDomain, brokerSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !brokerAuthorized(r, brokerSecret) {
			writeJSONErr(w, http.StatusUnauthorized, "broker authentication required")
			return
		}
		var req struct {
			Localpart string `json:"localpart"`
			Domain    string `json:"domain"`
			Org       string `json:"org"`
			Password  string `json:"password"` // optional; generated when absent
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "bad request")
			return
		}
		localpart := strings.TrimSpace(strings.ToLower(req.Localpart))
		domain := strings.TrimSpace(strings.ToLower(req.Domain))
		if domain == "" {
			domain = defaultDomain
		}
		if localpart == "" || strings.ContainsAny(localpart, "@ \t") {
			writeJSONErr(w, http.StatusBadRequest, "invalid localpart")
			return
		}
		address := localpart + "@" + domain

		// Idempotent: an existing mailbox is a success (no change).
		if mgr.IsLocal(address) {
			writeProvisionOK(w, address, false)
			return
		}
		password := req.Password
		if password == "" {
			password = randomPassword()
		}
		err := mgr.AddAccount(address, password)
		switch {
		case err == nil:
			log.Printf("provisioned mailbox %s (org=%q)", address, req.Org)
			writeProvisionOK(w, address, true)
		case errors.Is(err, seam.ErrUnsupported):
			writeJSONErr(w, http.StatusNotImplemented,
				"this mail server does not self-provision mailboxes; create the account via the configured identity provider")
		case strings.Contains(strings.ToLower(err.Error()), "exists"):
			// Lost a race with a concurrent provision — still idempotently OK.
			writeProvisionOK(w, address, false)
		default:
			writeJSONErr(w, http.StatusBadGateway, "could not provision mailbox: "+err.Error())
		}
	}
}

func writeProvisionOK(w http.ResponseWriter, address string, created bool) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"address": address, "created": created})
}

// randomPassword returns a strong random password for a provisioned mailbox whose
// credential is owned/brokered out of band (the free-mail caller resets or brokers
// it). 32 bytes of entropy, URL-safe.
func randomPassword() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// healthzHandler serves GET /healthz: a basic, unauthenticated liveness signal for
// the status page / load balancer. It reports only that the HTTP server is up; it
// does not run the (broker-gated, expensive) diagnostics suite.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
