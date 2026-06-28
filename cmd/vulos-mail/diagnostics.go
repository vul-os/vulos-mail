package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vul-os/vulos-mail/internal/diagnostics"
)

// splitComma splits a comma-separated config value, trimming and dropping empties.
func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// diagDuration parses a Go duration env value, falling back to def on error/empty.
func diagDuration(key string, def time.Duration) time.Duration {
	if v := env(key, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// diagConfigFromEnv builds the diagnostics Config and any wired Options (the live
// round-trip prober) from the [diagnostics] config section / environment. It is
// shared by the `vulos-mail diagnostics` CLI and the GET /api/diagnostics handler
// so both surfaces run an identical check suite.
func diagConfigFromEnv() (diagnostics.Config, []diagnostics.Option) {
	domain := env("VULOS_DOMAIN", "vulos.to")
	testMailbox := env("VULOS_DIAG_TEST_MAILBOX", "test@"+domain)
	cfg := diagnostics.Config{
		Domain:               domain,
		Enabled:              env("VULOS_DIAG_ENABLED", "") != "",
		HELO:                 env("VULOS_DIAG_HELO", domain),
		SMTPHost:             env("VULOS_MAIL_SMTP_HOST", domain),
		SMTPPort:             env("VULOS_MAIL_SMTP_PORT", "587"),
		IMAPHost:             env("VULOS_MAIL_IMAP_HOST", domain),
		IMAPPort:             env("VULOS_MAIL_IMAP_PORT", "993"),
		AutoDetectIP:         env("VULOS_DIAG_AUTODETECT_IP", "") != "",
		TestMailbox:          testMailbox,
		RoundTripEnabled:     env("VULOS_DIAG_ROUNDTRIP", "") != "",
		Timeout:              diagDuration("VULOS_DIAG_TIMEOUT", 10*time.Second),
		RoundTripMinInterval: diagDuration("VULOS_DIAG_ROUNDTRIP_INTERVAL", time.Minute),
	}
	if v := splitComma(env("VULOS_DIAG_DKIM_SELECTORS", "vulos-mail")); len(v) > 0 {
		cfg.DKIMSelectors = v
	}
	if v := splitComma(env("VULOS_DIAG_DNSBLS", "")); len(v) > 0 {
		cfg.DNSBLs = v
	}
	for _, s := range splitComma(env("VULOS_DIAG_SENDING_IPS", "")) {
		if ip := net.ParseIP(s); ip != nil {
			cfg.SendingIPs = append(cfg.SendingIPs, ip)
		}
	}

	var opts []diagnostics.Option
	// The live round-trip prober is wired only when a test-mailbox password is
	// configured; otherwise the self-test reports "not configured" rather than
	// failing — the same open-core seam pattern used elsewhere.
	if cfg.RoundTripEnabled {
		if pass := env("VULOS_DIAG_TEST_PASSWORD", ""); pass != "" {
			user := env("VULOS_DIAG_TEST_USER", testMailbox)
			opts = append(opts, diagnostics.WithProber(diagnostics.NewSMTPIMAPProber(diagnostics.ProberConfig{
				From:        testMailbox,
				To:          testMailbox,
				SMTPHost:    cfg.SMTPHost,
				SMTPPort:    cfg.SMTPPort,
				SMTPUser:    user,
				SMTPPass:    pass,
				IMAPHost:    cfg.IMAPHost,
				IMAPPort:    cfg.IMAPPort,
				IMAPUser:    user,
				IMAPPass:    pass,
				InsecureTLS: env("VULOS_DIAG_INSECURE_TLS", "") != "",
			})))
		}
	}
	return cfg, opts
}

// newDiagRunner builds a diagnostics Runner from the environment configuration.
func newDiagRunner() *diagnostics.Runner {
	cfg, opts := diagConfigFromEnv()
	return diagnostics.New(cfg, opts...)
}

// diagnosticsHandler serves GET /api/diagnostics. It is broker-gated with the
// same X-Vulos-Broker-Auth / LILMAIL_BROKER_SECRET pattern as the other
// admin/brokered routes: without a configured secret the endpoint is closed, and
// a request must present the matching secret. The body is the full JSON Report
// consumed by Cloud's status page.
func diagnosticsHandler(runner *diagnostics.Runner, brokerSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !brokerAuthorized(r, brokerSecret) {
			writeJSONErr(w, http.StatusUnauthorized, "broker authentication required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		rep := runner.Run(ctx)
		w.Header().Set("Content-Type", "application/json")
		// The report carries its own ok|warn|fail status, so 200 is always correct;
		// the status page reads the body, not the HTTP code.
		_ = json.NewEncoder(w).Encode(rep)
	}
}

// brokerAuthorized reports whether the request carries the broker secret in the
// X-Vulos-Broker-Auth header (constant-time compared). An empty configured secret
// closes the endpoint entirely.
func brokerAuthorized(r *http.Request, secret string) bool {
	if secret == "" {
		return false
	}
	got := r.Header.Get("X-Vulos-Broker-Auth")
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

// runDiagnosticsCLI runs the diagnostics suite and prints a human-readable report
// (or JSON with --json). It exits non-zero when the overall status is fail.
func runDiagnosticsCLI(args []string) int {
	jsonOut := false
	for _, a := range args {
		if a == "--json" || a == "-json" {
			jsonOut = true
		}
	}
	runner := newDiagRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	rep := runner.Run(ctx)

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printDiagReport(rep)
	}
	if rep.Status == diagnostics.StatusFail {
		return 1
	}
	return 0
}

// printDiagReport renders the report as an aligned, human-readable table.
func printDiagReport(rep diagnostics.Report) {
	mark := func(s diagnostics.Status) string {
		switch s {
		case diagnostics.StatusOK:
			return "[ OK ]"
		case diagnostics.StatusWarn:
			return "[WARN]"
		default:
			return "[FAIL]"
		}
	}
	fmt.Printf("vulos-mail diagnostics for %s  (%s)\n", rep.Domain, rep.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("overall: %s   ok=%d warn=%d fail=%d total=%d\n\n",
		strings.ToUpper(string(rep.Status)), rep.Summary.OK, rep.Summary.Warn, rep.Summary.Fail, rep.Summary.Total)
	for _, c := range rep.Checks {
		lat := ""
		if c.LatencyMS > 0 {
			lat = fmt.Sprintf(" (%dms)", c.LatencyMS)
		}
		fmt.Printf("%s %-28s %s%s\n", mark(c.Status), c.ID, c.Title, lat)
		if c.Detail != "" {
			fmt.Printf("        %s\n", c.Detail)
		}
		if c.Value != "" {
			fmt.Printf("        value: %s\n", c.Value)
		}
		if c.Remediation != "" {
			fmt.Printf("        fix:   %s\n", c.Remediation)
		}
	}
}
