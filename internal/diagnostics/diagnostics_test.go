package diagnostics

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"
)

// fakeResolver is a fully in-memory Resolver: every lookup is served from the
// configured maps, so the check suite never touches the network.
type fakeResolver struct {
	mx   map[string][]*net.MX
	txt  map[string][]string
	host map[string][]string // forward A/AAAA (LookupHost) AND DNSBL answers
	addr map[string][]string // reverse (LookupAddr)
	ipa  map[string][]net.IPAddr
	err  map[string]error // per-name forced error (keyed by the queried name)
}

func nf() error { return &net.DNSError{Err: "no such host", IsNotFound: true} }

func (f *fakeResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if e, ok := f.err[name]; ok {
		return nil, e
	}
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, nf()
}
func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if e, ok := f.err[name]; ok {
		return nil, e
	}
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, nf()
}
func (f *fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	if e, ok := f.err[host]; ok {
		return nil, e
	}
	if v, ok := f.host[host]; ok {
		return v, nil
	}
	return nil, nf()
}
func (f *fakeResolver) LookupAddr(_ context.Context, addr string) ([]string, error) {
	if e, ok := f.err[addr]; ok {
		return nil, e
	}
	if v, ok := f.addr[addr]; ok {
		return v, nil
	}
	return nil, nf()
}
func (f *fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if e, ok := f.err[host]; ok {
		return nil, e
	}
	if v, ok := f.ipa[host]; ok {
		return v, nil
	}
	return nil, nf()
}

// fakeTLS returns a preset connection state (and/or error) per check.
type fakeTLS struct {
	smtp *tls.ConnectionState
	imap *tls.ConnectionState
	err  error
}

func (f *fakeTLS) ProbeTLS(_ context.Context, mode TLSMode, _, _ string) (*tls.ConnectionState, error) {
	if f.err != nil {
		return nil, f.err
	}
	if mode == TLSStartTLSSMTP {
		return f.smtp, nil
	}
	return f.imap, nil
}

// fakeProber returns a preset latency/error.
type fakeProber struct {
	lat    time.Duration
	err    error
	called int
}

func (p *fakeProber) Probe(context.Context, string) (time.Duration, error) {
	p.called++
	return p.lat, p.err
}

func find(t *testing.T, rep Report, id string) Check {
	t.Helper()
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %q not found in report; have %d checks", id, len(rep.Checks))
	return Check{}
}

func baseConfig() Config {
	return Config{
		Domain:        "example.test",
		DKIMSelectors: []string{"sel1"},
		DNSBLs:        []string{"bl.test"},
		SendingIPs:    []net.IP{net.ParseIP("192.0.2.10")},
		HELO:          "mx.example.test",
		Enabled:       true,
		Timeout:       time.Second,
	}
}

// healthyResolver returns a resolver where every DNS deliverability record is
// correct and the sending IP is clean.
func healthyResolver() *fakeResolver {
	return &fakeResolver{
		mx: map[string][]*net.MX{"example.test": {{Host: "mx.example.test.", Pref: 10}}},
		txt: map[string][]string{
			"example.test":                 {"v=spf1 mx -all"},
			"sel1._domainkey.example.test": {"v=DKIM1; k=rsa; p=MIGfMA0GCS"},
			"_dmarc.example.test":          {"v=DMARC1; p=reject; rua=mailto:d@example.test"},
		},
		host: map[string][]string{
			"mx.example.test": {"192.0.2.10"},
			// DNSBL query for 192.0.2.10 on bl.test -> NXDOMAIN (handled by default).
		},
		addr: map[string][]string{"192.0.2.10": {"mx.example.test."}},
		ipa:  map[string][]net.IPAddr{"example.test": {{IP: net.ParseIP("192.0.2.10")}}},
	}
}

func TestDisabledReport(t *testing.T) {
	r := New(Config{Domain: "example.test", Enabled: false})
	rep := r.Run(context.Background())
	if rep.Status != StatusWarn {
		t.Fatalf("disabled report status = %s, want warn", rep.Status)
	}
	if len(rep.Checks) != 1 || rep.Checks[0].ID != "diagnostics.enabled" {
		t.Fatalf("disabled report should carry a single diagnostics.enabled check, got %+v", rep.Checks)
	}
}

func TestDNSChecksHealthy(t *testing.T) {
	r := New(baseConfig(), WithResolver(healthyResolver()))
	rep := r.Run(context.Background())

	for _, id := range []string{"dns.mx", "dns.spf", "dns.dkim.sel1", "dns.dmarc", "dns.a", "dns.ptr"} {
		if c := find(t, rep, id); c.Status != StatusOK {
			t.Errorf("%s status = %s (%s), want ok", id, c.Status, c.Detail)
		}
	}
	// DNSBL: NXDOMAIN means not listed -> ok.
	if c := find(t, rep, "dnsbl.bl.test"); c.Status != StatusOK {
		t.Errorf("dnsbl status = %s (%s), want ok", c.Status, c.Detail)
	}
}

func TestDNSChecksFailures(t *testing.T) {
	res := &fakeResolver{
		// MX missing entirely (NXDOMAIN), SPF too permissive, DKIM revoked,
		// DMARC p=none, A present, PTR mismatch.
		txt: map[string][]string{
			"example.test":                 {"v=spf1 +all"},
			"sel1._domainkey.example.test": {"v=DKIM1; p="},
			"_dmarc.example.test":          {"v=DMARC1; p=none"},
		},
		addr: map[string][]string{"192.0.2.10": {"other.host."}},
		host: map[string][]string{"other.host": {"192.0.2.10"}},
		ipa:  map[string][]net.IPAddr{"example.test": {{IP: net.ParseIP("192.0.2.10")}}},
	}
	r := New(baseConfig(), WithResolver(res))
	rep := r.Run(context.Background())

	if c := find(t, rep, "dns.mx"); c.Status != StatusFail {
		t.Errorf("mx = %s, want fail", c.Status)
	}
	if c := find(t, rep, "dns.spf"); c.Status != StatusWarn {
		t.Errorf("spf (+all) = %s, want warn", c.Status)
	}
	if c := find(t, rep, "dns.dkim.sel1"); c.Status != StatusWarn {
		t.Errorf("dkim (revoked) = %s, want warn", c.Status)
	}
	if c := find(t, rep, "dns.dmarc"); c.Status != StatusWarn {
		t.Errorf("dmarc (p=none) = %s, want warn", c.Status)
	}
	if c := find(t, rep, "dns.ptr"); c.Status != StatusWarn {
		t.Errorf("ptr (mismatch) = %s, want warn", c.Status)
	}
}

func TestDNSBLListed(t *testing.T) {
	res := healthyResolver()
	// 192.0.2.10 reversed is 10.2.0.192; a listing returns 127.0.0.2.
	res.host["10.2.0.192.bl.test"] = []string{"127.0.0.2"}
	r := New(baseConfig(), WithResolver(res))
	rep := r.Run(context.Background())
	c := find(t, rep, "dnsbl.bl.test")
	if c.Status != StatusFail {
		t.Fatalf("listed IP dnsbl = %s, want fail", c.Status)
	}
}

func TestNoSendingIPWarns(t *testing.T) {
	cfg := baseConfig()
	cfg.SendingIPs = nil
	cfg.AutoDetectIP = false
	r := New(cfg, WithResolver(healthyResolver()))
	rep := r.Run(context.Background())
	if c := find(t, rep, "dns.ptr"); c.Status != StatusWarn {
		t.Errorf("ptr without IP = %s, want warn", c.Status)
	}
	if c := find(t, rep, "dnsbl"); c.Status != StatusWarn {
		t.Errorf("dnsbl without IP = %s, want warn", c.Status)
	}
}

func TestAutoDetectIP(t *testing.T) {
	cfg := baseConfig()
	cfg.SendingIPs = nil
	cfg.AutoDetectIP = true
	r := New(cfg, WithResolver(healthyResolver()))
	rep := r.Run(context.Background())
	// With auto-detect, the domain's A record (192.0.2.10) is used for PTR/DNSBL.
	if c := find(t, rep, "dns.ptr"); c.Status != StatusOK {
		t.Errorf("ptr with auto-detected IP = %s (%s), want ok", c.Status, c.Detail)
	}
}

// --- TLS ---

type certSpec struct {
	host      string
	notBefore time.Time
	notAfter  time.Time
}

func makeCert(t *testing.T, spec certSpec, signer *x509.Certificate, signerKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: spec.host},
		DNSNames:              []string{spec.host},
		NotBefore:             spec.notBefore,
		NotAfter:              spec.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	parent := tmpl
	pkey := key
	if signer != nil {
		parent = signer
		pkey = signerKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, pkey)
	if err != nil {
		t.Fatal(err)
	}
	crt, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return crt, key
}

func state(certs ...*x509.Certificate) *tls.ConnectionState {
	return &tls.ConnectionState{PeerCertificates: certs}
}

func tlsConfigWith(smtp, imap *tls.ConnectionState) *fakeTLS { return &fakeTLS{smtp: smtp, imap: imap} }

func TestTLSValidTrusted(t *testing.T) {
	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	ca, caKey := makeCert(t, certSpec{host: "ca.test", notBefore: now.Add(-time.Hour), notAfter: now.Add(365 * 24 * time.Hour)}, nil, nil)
	leaf, _ := makeCert(t, certSpec{host: "mail.example.test", notBefore: now.Add(-time.Hour), notAfter: now.Add(90 * 24 * time.Hour)}, ca, caKey)
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	cfg := baseConfig()
	cfg.SMTPHost, cfg.IMAPHost = "mail.example.test", "mail.example.test"
	r := New(cfg,
		WithResolver(healthyResolver()),
		WithTLSDialer(tlsConfigWith(state(leaf, ca), state(leaf, ca))),
		WithRootCAs(pool),
		WithClock(func() time.Time { return now }),
	)
	rep := r.Run(context.Background())
	if c := find(t, rep, "tls.smtp"); c.Status != StatusOK {
		t.Errorf("tls.smtp = %s (%s), want ok", c.Status, c.Detail)
	}
	if c := find(t, rep, "tls.imap"); c.Status != StatusOK {
		t.Errorf("tls.imap = %s (%s), want ok", c.Status, c.Detail)
	}
}

func TestTLSExpired(t *testing.T) {
	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	leaf, _ := makeCert(t, certSpec{host: "mail.example.test", notBefore: now.Add(-48 * time.Hour), notAfter: now.Add(-time.Hour)}, nil, nil)
	cfg := baseConfig()
	cfg.SMTPHost, cfg.IMAPHost = "mail.example.test", "mail.example.test"
	r := New(cfg,
		WithResolver(healthyResolver()),
		WithTLSDialer(tlsConfigWith(state(leaf), state(leaf))),
		WithClock(func() time.Time { return now }),
	)
	rep := r.Run(context.Background())
	if c := find(t, rep, "tls.smtp"); c.Status != StatusFail {
		t.Errorf("expired tls.smtp = %s, want fail", c.Status)
	}
}

func TestTLSHostnameMismatch(t *testing.T) {
	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	leaf, _ := makeCert(t, certSpec{host: "wrong.test", notBefore: now.Add(-time.Hour), notAfter: now.Add(90 * 24 * time.Hour)}, nil, nil)
	cfg := baseConfig()
	cfg.SMTPHost, cfg.IMAPHost = "mail.example.test", "mail.example.test"
	r := New(cfg,
		WithResolver(healthyResolver()),
		WithTLSDialer(tlsConfigWith(state(leaf), state(leaf))),
		WithClock(func() time.Time { return now }),
	)
	rep := r.Run(context.Background())
	if c := find(t, rep, "tls.imap"); c.Status != StatusFail {
		t.Errorf("hostname-mismatch tls.imap = %s, want fail", c.Status)
	}
}

func TestTLSSelfSignedWarns(t *testing.T) {
	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	leaf, _ := makeCert(t, certSpec{host: "mail.example.test", notBefore: now.Add(-time.Hour), notAfter: now.Add(90 * 24 * time.Hour)}, nil, nil)
	cfg := baseConfig()
	cfg.SMTPHost, cfg.IMAPHost = "mail.example.test", "mail.example.test"
	r := New(cfg,
		WithResolver(healthyResolver()),
		WithTLSDialer(tlsConfigWith(state(leaf), state(leaf))),
		WithRootCAs(x509.NewCertPool()), // empty pool -> chain not trusted
		WithClock(func() time.Time { return now }),
	)
	rep := r.Run(context.Background())
	if c := find(t, rep, "tls.smtp"); c.Status != StatusWarn {
		t.Errorf("self-signed tls.smtp = %s, want warn", c.Status)
	}
}

func TestTLSDialError(t *testing.T) {
	cfg := baseConfig()
	r := New(cfg,
		WithResolver(healthyResolver()),
		WithTLSDialer(&fakeTLS{err: errors.New("connection refused")}),
	)
	rep := r.Run(context.Background())
	if c := find(t, rep, "tls.smtp"); c.Status != StatusFail {
		t.Errorf("dial-error tls.smtp = %s, want fail", c.Status)
	}
}

// --- round trip ---

func TestRoundTripDisabled(t *testing.T) {
	r := New(baseConfig(), WithResolver(healthyResolver()))
	c := find(t, r.Run(context.Background()), "roundtrip")
	if c.Status != StatusWarn {
		t.Fatalf("roundtrip (disabled) = %s, want warn", c.Status)
	}
}

func TestRoundTripNoProber(t *testing.T) {
	cfg := baseConfig()
	cfg.RoundTripEnabled = true
	r := New(cfg, WithResolver(healthyResolver()))
	c := find(t, r.Run(context.Background()), "roundtrip")
	if c.Status != StatusWarn {
		t.Fatalf("roundtrip (no prober) = %s, want warn", c.Status)
	}
}

func TestRoundTripSuccess(t *testing.T) {
	cfg := baseConfig()
	cfg.RoundTripEnabled = true
	p := &fakeProber{lat: 250 * time.Millisecond}
	r := New(cfg, WithResolver(healthyResolver()), WithProber(p))
	c := find(t, r.Run(context.Background()), "roundtrip")
	if c.Status != StatusOK {
		t.Fatalf("roundtrip = %s (%s), want ok", c.Status, c.Detail)
	}
	if c.LatencyMS != 250 {
		t.Errorf("latency = %dms, want 250", c.LatencyMS)
	}
}

func TestRoundTripFailure(t *testing.T) {
	cfg := baseConfig()
	cfg.RoundTripEnabled = true
	p := &fakeProber{err: errors.New("timed out")}
	r := New(cfg, WithResolver(healthyResolver()), WithProber(p))
	c := find(t, r.Run(context.Background()), "roundtrip")
	if c.Status != StatusFail {
		t.Fatalf("roundtrip = %s, want fail", c.Status)
	}
}

func TestRoundTripRateLimited(t *testing.T) {
	cfg := baseConfig()
	cfg.RoundTripEnabled = true
	cfg.RoundTripMinInterval = time.Hour
	clock := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	p := &fakeProber{lat: 10 * time.Millisecond}
	r := New(cfg, WithResolver(healthyResolver()), WithProber(p), WithClock(func() time.Time { return clock }))

	// First run probes.
	if c := find(t, r.Run(context.Background()), "roundtrip"); c.Status != StatusOK {
		t.Fatalf("first roundtrip = %s, want ok", c.Status)
	}
	// Second run within the interval is rate-limited (no new probe sent).
	if c := find(t, r.Run(context.Background()), "roundtrip"); c.Status != StatusWarn {
		t.Fatalf("second roundtrip = %s, want warn (rate-limited)", c.Status)
	}
	if p.called != 1 {
		t.Fatalf("prober called %d times, want exactly 1 (rate-limited)", p.called)
	}
}

func TestSummaryAndOverallStatus(t *testing.T) {
	r := New(baseConfig(), WithResolver(healthyResolver()))
	rep := r.Run(context.Background())
	if rep.Summary.Total != len(rep.Checks) {
		t.Errorf("summary total %d != %d checks", rep.Summary.Total, len(rep.Checks))
	}
	// Overall status must be the worst of the individual checks.
	worst := StatusOK
	for _, c := range rep.Checks {
		worst = mergeStatus(worst, c.Status)
	}
	if rep.Status != worst {
		t.Errorf("overall status = %s, want %s", rep.Status, worst)
	}
}

func TestReverseIP(t *testing.T) {
	if got := reverseIP(net.ParseIP("1.2.3.4")); got != "4.3.2.1" {
		t.Errorf("reverseIP v4 = %q, want 4.3.2.1", got)
	}
	if got := reverseIP(net.ParseIP("2001:db8::1")); got == "" {
		t.Error("reverseIP v6 returned empty")
	}
}
