// Package diagnostics runs deliverability and health checks for a configured
// mail domain/server and returns structured, machine-readable results.
//
// It is a self-contained part of the OSS mail server: it has no dependency on
// Vulos Cloud. The same report drives two surfaces — an authenticated JSON API
// (GET /api/diagnostics, consumed by Cloud's status page) and a human-readable
// CLI report (`vulos-mail diagnostics`, for self-hosters).
//
// Every external dependency is injected behind an interface or function value
// (DNS via [Resolver], TLS via [TLSDialer], the round-trip self-test via
// [Prober]) so the whole check suite is unit-tested offline — the tests never
// touch the network.
package diagnostics

import (
	"context"
	"net"
	"sort"
	"sync"
	"time"
)

// Status is the outcome of a single check (and, aggregated, of the whole report).
type Status string

const (
	// StatusOK means the check passed.
	StatusOK Status = "ok"
	// StatusWarn means the check found a non-fatal issue worth attention (e.g. a
	// DMARC policy of p=none, or a self-test that is disabled).
	StatusWarn Status = "warn"
	// StatusFail means the check found a problem that will hurt deliverability or
	// break mail flow.
	StatusFail Status = "fail"
)

// rank orders statuses so the report can report the worst outcome.
func (s Status) rank() int {
	switch s {
	case StatusFail:
		return 2
	case StatusWarn:
		return 1
	default:
		return 0
	}
}

// Check is the structured result of one diagnostic.
type Check struct {
	// ID is a stable, dotted identifier (e.g. "dns.spf", "tls.smtp") for the
	// status page to key on across runs.
	ID string `json:"id"`
	// Title is a short human label.
	Title string `json:"title"`
	// Status is ok | warn | fail.
	Status Status `json:"status"`
	// Detail explains what was found.
	Detail string `json:"detail"`
	// Remediation is a hint on how to fix a warn/fail (omitted when ok).
	Remediation string `json:"remediation,omitempty"`
	// Value is the measured value the check is about (record contents, address,
	// certificate expiry, …), when there is one.
	Value string `json:"value,omitempty"`
	// LatencyMS is the wall-clock time the check took, in milliseconds.
	LatencyMS int64 `json:"latencyMs,omitempty"`
}

// Summary counts checks by status.
type Summary struct {
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Total int `json:"total"`
}

// Report is the full diagnostics result.
type Report struct {
	Domain      string    `json:"domain"`
	GeneratedAt time.Time `json:"generatedAt"`
	// Status is the worst status across all checks.
	Status  Status  `json:"status"`
	Summary Summary `json:"summary"`
	Checks  []Check `json:"checks"`
}

// Config controls which checks run and against what. It mirrors the
// [diagnostics] configuration section.
type Config struct {
	// Domain is the mail domain under test (MX/SPF/DKIM/DMARC are keyed on it).
	Domain string
	// DKIMSelectors are the DKIM selectors to verify (e.g. "vulos-mail").
	DKIMSelectors []string
	// SendingIPs are the public IP(s) outbound mail leaves from, used for the
	// PTR/reverse-DNS and DNSBL checks. When empty and AutoDetectIP is set the
	// runner resolves the domain's A/AAAA as a best-effort stand-in.
	SendingIPs []net.IP
	// AutoDetectIP allows the runner to fall back to the domain's A/AAAA records
	// when SendingIPs is empty.
	AutoDetectIP bool
	// DNSBLs are the blocklist zones to query (e.g. "zen.spamhaus.org").
	DNSBLs []string
	// HELO is the HELO/EHLO name the server announces; PTR results are matched
	// against it.
	HELO string
	// SMTPHost/SMTPPort is the submission endpoint probed for STARTTLS.
	SMTPHost string
	SMTPPort string
	// IMAPHost/IMAPPort is the IMAPS endpoint probed for implicit TLS.
	IMAPHost string
	IMAPPort string
	// TestMailbox is the address the round-trip self-test sends to and polls
	// (default test@<Domain>).
	TestMailbox string
	// Enabled gates the whole capability. When false, Run returns an empty,
	// disabled report.
	Enabled bool
	// Timeout bounds each individual check (and is the default round-trip poll
	// budget).
	Timeout time.Duration
	// RoundTripEnabled gates the send→deliver→receive self-test.
	RoundTripEnabled bool
	// RoundTripMinInterval rate-limits the self-test: if Run is called again
	// within this window the round-trip check is reported as warn (rate-limited)
	// rather than sending another probe.
	RoundTripMinInterval time.Duration
}

// withDefaults returns a copy of cfg with zero-valued fields filled in.
func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	if c.RoundTripMinInterval <= 0 {
		c.RoundTripMinInterval = time.Minute
	}
	if len(c.DKIMSelectors) == 0 {
		c.DKIMSelectors = []string{"vulos-mail"}
	}
	if len(c.DNSBLs) == 0 {
		c.DNSBLs = []string{"zen.spamhaus.org", "bl.spamcop.net", "b.barracudacentral.org"}
	}
	if c.TestMailbox == "" && c.Domain != "" {
		c.TestMailbox = "test@" + c.Domain
	}
	if c.HELO == "" {
		c.HELO = c.Domain
	}
	if c.SMTPHost == "" {
		c.SMTPHost = c.Domain
	}
	if c.SMTPPort == "" {
		c.SMTPPort = "587"
	}
	if c.IMAPHost == "" {
		c.IMAPHost = c.Domain
	}
	if c.IMAPPort == "" {
		c.IMAPPort = "993"
	}
	return c
}

// Resolver is the subset of *net.Resolver the DNS/DNSBL checks use. *net.Resolver
// satisfies it; tests inject a fake.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Runner orchestrates the checks. The zero value is not usable; build one with
// [New].
type Runner struct {
	cfg      Config
	resolver Resolver
	dialTLS  TLSDialer
	prober   Prober
	now      func() time.Time

	mu        sync.Mutex
	lastProbe time.Time
}

// New builds a Runner from cfg. Optional dependencies are injected via [Option];
// unset ones default to real implementations (net.DefaultResolver, a live TLS
// dialer). The round-trip [Prober] has no live default — without [WithProber]
// the self-test reports as not-configured.
func New(cfg Config, opts ...Option) *Runner {
	r := &Runner{
		cfg:      cfg.withDefaults(),
		resolver: net.DefaultResolver,
		dialTLS:  defaultTLSDialer,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Option customizes a Runner.
type Option func(*Runner)

// WithResolver injects a DNS resolver (default net.DefaultResolver).
func WithResolver(res Resolver) Option { return func(r *Runner) { r.resolver = res } }

// WithTLSDialer injects the TLS probe (default a live dialer).
func WithTLSDialer(d TLSDialer) Option { return func(r *Runner) { r.dialTLS = d } }

// WithProber injects the round-trip self-test prober.
func WithProber(p Prober) Option { return func(r *Runner) { r.prober = p } }

// WithClock injects the clock (for tests).
func WithClock(now func() time.Time) Option { return func(r *Runner) { r.now = now } }

// Config returns the effective (defaulted) configuration.
func (r *Runner) Config() Config { return r.cfg }

// Run executes every enabled check and returns the assembled report.
func (r *Runner) Run(ctx context.Context) Report {
	rep := Report{
		Domain:      r.cfg.Domain,
		GeneratedAt: r.now().UTC(),
	}
	if !r.cfg.Enabled {
		rep.Status = StatusWarn
		rep.Checks = []Check{{
			ID:          "diagnostics.enabled",
			Title:       "Diagnostics enabled",
			Status:      StatusWarn,
			Detail:      "diagnostics are disabled by configuration",
			Remediation: "set the [diagnostics] enabled flag (VULOS_DIAG_ENABLED=1) to run checks",
		}}
		rep.finalize()
		return rep
	}

	rep.Checks = append(rep.Checks, r.dnsChecks(ctx)...)
	rep.Checks = append(rep.Checks, r.dnsblChecks(ctx)...)
	rep.Checks = append(rep.Checks, r.tlsChecks(ctx)...)
	rep.Checks = append(rep.Checks, r.roundTripCheck(ctx))

	rep.finalize()
	return rep
}

// finalize computes the summary and overall status.
func (rep *Report) finalize() {
	rep.Status = StatusOK
	rep.Summary = Summary{}
	for _, c := range rep.Checks {
		rep.Summary.Total++
		switch c.Status {
		case StatusFail:
			rep.Summary.Fail++
		case StatusWarn:
			rep.Summary.Warn++
		default:
			rep.Summary.OK++
		}
		if c.Status.rank() > rep.Status.rank() {
			rep.Status = c.Status
		}
	}
}

// sendingIPs returns the configured sending IPs, falling back to the domain's
// A/AAAA records when AutoDetectIP is set and none are configured.
func (r *Runner) sendingIPs(ctx context.Context) []net.IP {
	if len(r.cfg.SendingIPs) > 0 {
		return r.cfg.SendingIPs
	}
	if !r.cfg.AutoDetectIP || r.cfg.Domain == "" {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()
	addrs, err := r.resolver.LookupIPAddr(cctx, r.cfg.Domain)
	if err != nil {
		return nil
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.IP)
	}
	return out
}

// measure runs fn and returns its elapsed time in milliseconds.
func (r *Runner) measure(fn func()) int64 {
	start := r.now()
	fn()
	return r.now().Sub(start).Milliseconds()
}

// sortedTXT returns TXT records joined and sorted for stable reporting.
func sortedTXT(records []string) []string {
	out := append([]string(nil), records...)
	sort.Strings(out)
	return out
}
