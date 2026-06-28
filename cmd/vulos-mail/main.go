// Command vulos-mail runs the mail system: MX (receive), submission (send), and IMAP
// (serve) listeners over a shared account manager, plus the outbound scheduler
// loop. Configuration is via environment variables; account provisioning here is
// a single demo account (real provisioning/auth is a later wave).
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	appsplatform "github.com/vul-os/vulos-apps/appsplatform"
	"github.com/vul-os/vulos-apps/mcp"
	imapadapter "github.com/vul-os/vulos-mail/adapters/imap"
	jmapadapter "github.com/vul-os/vulos-mail/adapters/jmap"
	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
	"github.com/vul-os/vulos-mail/adapters/webapi"
	"github.com/vul-os/vulos-mail/caldav"
	"github.com/vul-os/vulos-mail/carddav"
	cloud "github.com/vul-os/vulos-mail/integration/cloud"
	"github.com/vul-os/vulos-mail/internal/abuse"
	"github.com/vul-os/vulos-mail/internal/account"
	mailapps "github.com/vul-os/vulos-mail/internal/apps"
	"github.com/vul-os/vulos-mail/internal/authlimit"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/compose"
	"github.com/vul-os/vulos-mail/internal/emailauth"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/filter"
	"github.com/vul-os/vulos-mail/internal/llm"
	"github.com/vul-os/vulos-mail/internal/mailsettings"
	"github.com/vul-os/vulos-mail/internal/metrics"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/scan"
	"github.com/vul-os/vulos-mail/internal/seam"
	"github.com/vul-os/vulos-mail/internal/seam/altcha"
	seamlocal "github.com/vul-os/vulos-mail/internal/seam/local"
	"github.com/vul-os/vulos-mail/internal/server"
	"github.com/vul-os/vulos-mail/internal/signup"
	"github.com/vul-os/vulos-mail/internal/tenant"
	"github.com/vul-os/vulos-mail/internal/tlsconf"
	"github.com/vul-os/vulos-mail/services/mtaout"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadEnv loads KEY=VALUE pairs from a dotenv file into the process environment
// (without overriding values already set). The file is chosen by VULOS_ENV_FILE,
// else .env.<VULOS_ENV> (e.g. VULOS_ENV=main -> .env.main), else .env if present.
func loadEnv() {
	path := os.Getenv("VULOS_ENV_FILE")
	if path == "" {
		if name := os.Getenv("VULOS_ENV"); name != "" {
			path = ".env." + name
		} else if _, err := os.Stat(".env"); err == nil {
			path = ".env"
		} else {
			return
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("env file %s: %v (skipping)", path, err)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
	log.Printf("loaded config from %s", path)
}

func main() {
	loadEnv()
	// Subcommands. `vulos-mail diagnostics` runs the deliverability/health check
	// suite and prints a report (add --json for machine output); no subcommand runs
	// the mail server.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "diagnostics", "diag":
			os.Exit(runDiagnosticsCLI(os.Args[2:]))
		}
	}
	var (
		dataDir = env("VULOS_DATA_DIR", "./data")
		domain  = env("VULOS_DOMAIN", "vulos.to")
		mxAddr  = env("VULOS_MX_ADDR", ":2525")
		subAddr = env("VULOS_SUBMIT_ADDR", ":2587")
		imapddr = env("VULOS_IMAP_ADDR", ":2143")
		jmapddr = env("VULOS_JMAP_ADDR", ":2080")
		acct    = env("VULOS_ACCOUNT", "")
		pass    = env("VULOS_PASSWORD", "")
	)

	// Blob store: S3-compatible if configured, else local FS (both zstd + dedup).
	var blobs blob.Store
	if ep := env("VULOS_S3_ENDPOINT", ""); ep != "" {
		s3, serr := blob.NewS3(context.Background(), ep, env("VULOS_S3_KEY", ""), env("VULOS_S3_SECRET", ""), env("VULOS_S3_BUCKET", "vulos-mail"), env("VULOS_S3_SSL", "") != "")
		if serr != nil {
			log.Fatalf("s3 blob store: %v", serr)
		}
		blobs = s3
		log.Printf("blob store: S3 %s/%s", ep, env("VULOS_S3_BUCKET", "vulos-mail"))
	} else {
		fs, ferr := blob.NewFS(dataDir + "/blobs")
		if ferr != nil {
			log.Fatalf("blob store: %v", ferr)
		}
		blobs = fs
	}

	// TLS: bring-your-own cert/key, or self-signed for dev (VULOS_TLS_SELFSIGNED=1).
	var selfSigned []string
	if env("VULOS_TLS_SELFSIGNED", "") != "" {
		selfSigned = []string{domain, "localhost"}
	}
	tlsCfg, err := tlsconf.Config(env("VULOS_TLS_CERT", ""), env("VULOS_TLS_KEY", ""), selfSigned...)
	if err != nil {
		log.Fatalf("tls: %v", err)
	}
	// ACME (Let's Encrypt) for real certs, if configured. Overrides the cert/key
	// or self-signed config above. Serves HTTP-01 challenges on :80.
	if domains := env("VULOS_ACME_DOMAINS", ""); domains != "" {
		am := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(dataDir + "/acme"),
			HostPolicy: autocert.HostWhitelist(strings.Split(domains, ",")...),
			Email:      env("VULOS_ACME_EMAIL", ""),
		}
		// Optional: point at a custom ACME directory (e.g. a local Pebble/step-ca
		// for testing). VULOS_ACME_INSECURE skips TLS verification of that endpoint.
		if dir := env("VULOS_ACME_DIRECTORY", ""); dir != "" {
			ac := &acme.Client{DirectoryURL: dir}
			if env("VULOS_ACME_INSECURE", "") != "" {
				ac.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			}
			am.Client = ac
		}
		tlsCfg = am.TLSConfig()
		go func() {
			if err := http.ListenAndServe(":80", am.HTTPHandler(nil)); err != nil {
				log.Printf("acme http-01: %v", err)
			}
		}()
		log.Printf("ACME enabled for %s (HTTP-01 on :80)", domains)
	}
	if tlsCfg != nil {
		log.Printf("TLS enabled (STARTTLS on SMTP/IMAP, HTTPS on API)")
	}

	// Outbound: warm-IP pool + reputation + scheduler behind a pluggable Sender.
	//
	// Backend selection (DELIVER_BACKEND env):
	//   ""    (default) → built-in direct-SMTP sender; no external deps, self-host friendly.
	//   "ses"           → vulos-deliver SES backend (opt-in, cloud/SaaS deployments).
	//   "smtp"          → vulos-deliver managed-SMTP relay (opt-in).
	//
	// SES credentials: DELIVER_SES_KEY / DELIVER_SES_SECRET / DELIVER_SES_REGION.
	// When key/secret are empty, the SES backend falls back to the standard AWS
	// credential chain (IAM role, ~/.aws/credentials, etc.).
	pool := mtaout.NewPool([]string{}, []string{}) // configure source IPs in prod
	rep := mtaout.NewReputation(100, 0.10, 0.02)
	warm := mtaout.NewWarmup([]int{50, 100, 500, 1000, 5000, 10000, 50000})
	outSender, err := mtaout.NewSender(mtaout.SenderConfig{
		HELO:           domain,
		DeliverBackend: env("DELIVER_BACKEND", ""),
		SESRegion:      env("DELIVER_SES_REGION", ""),
		SESKey:         env("DELIVER_SES_KEY", ""),
		SESSecret:      env("DELIVER_SES_SECRET", ""),
		SESConfigSet:   env("DELIVER_SES_CONFIG_SET", ""),
	})
	if err != nil {
		log.Fatalf("outbound sender: %v", err)
	}
	if env("DELIVER_BACKEND", "") != "" {
		log.Printf("outbound sender: vulos-deliver/%s", env("DELIVER_BACKEND", ""))
	} else {
		log.Printf("outbound sender: built-in SMTP (direct MX delivery, self-host default)")
	}
	sched := mtaout.NewScheduler(mtaout.Config{
		Sender: outSender, Pool: pool, Warmup: warm, Reputation: rep,
		MaxPerDomain: 10,
	})

	mgr := server.NewManager(dataDir, blobs, sched)
	if env("VULOS_DB", "") == "sqlite" {
		mgr.LogOpen = func(d string) (eventlog.Log, error) { return eventlog.OpenSQLite(filepath.Join(d, "log.db"), nil) }
		log.Printf("event log backend: sqlite")
	}
	sched.SetOnBounce(func(msg mtaout.OutMessage, reason string) { mgr.HandleBounce(domain, msg, reason) })
	if txt, err := mgr.EnsureDKIM(domain, "vulos-mail"); err == nil && txt != "" {
		log.Printf("DKIM: publish TXT at vulos-mail._domainkey.%s :  %s", domain, txt)
	}
	// Identity: standalone by default (a persistent, file-backed local account
	// store — the OSS self-hosted path), with the optional vulos-cloud adapter
	// taking over only when VULOS_CP_URL is set. The mail core depends solely on
	// the seam interfaces; nothing here couples it to vulos-cloud.
	localID, err := seamlocal.NewIdentity(dataDir)
	if err != nil {
		log.Fatalf("account store: %v", err)
	}
	mgr.Identity = localID
	if cp := env("VULOS_CP_URL", ""); cp != "" {
		c := cloud.New(cp, env("VULOS_CP_SECRET", ""))
		mgr.Identity = cloud.NewIdentity(c)
		mgr.Plans = cloud.NewEntitlements(c)
		mgr.Usage = cloud.NewUsage(c)
		log.Printf("identity/billing: vulos-cloud control plane (%s)", cp)
	} else {
		log.Printf("identity/billing: standalone (local account store, no cloud)")
	}
	// Seed the configured account (config is authoritative across restarts). Skip
	// when cloud owns identity.
	if acct != "" && pass != "" {
		if env("VULOS_CP_URL", "") == "" {
			if err := localID.Upsert(acct, pass); err != nil {
				log.Fatalf("seed account: %v", err)
			}
		}
		log.Printf("provisioned account %s", acct)
	} else if localID.Count() == 0 && env("VULOS_CP_URL", "") == "" {
		log.Printf("no VULOS_ACCOUNT set and no accounts on disk; use self-serve signup at /api/signup")
	}

	// Self-serve signup: gated by an Altcha proof-of-work challenge (self-hosted,
	// no external service). Set VULOS_SIGNUP=off to disable public signup.
	var signupGate seam.SignupGate = seam.OpenGate{}
	var signupIssuer signup.Issuer
	if env("VULOS_SIGNUP", "") != "off" {
		secret := []byte(env("VULOS_ALTCHA_SECRET", ""))
		if len(secret) == 0 {
			secret = make([]byte, 32)
			_, _ = rand.Read(secret)
		}
		// Difficulty: ~50k worst-case hashes — a meaningful per-signup deterrent,
		// ~1–2s for a real browser to solve, tunable via VULOS_ALTCHA_DIFFICULTY.
		difficulty := 50_000
		if d := env("VULOS_ALTCHA_DIFFICULTY", ""); d != "" {
			if n, e := strconv.Atoi(d); e == nil && n > 0 {
				difficulty = n
			}
		}
		gate := altcha.New(secret, difficulty)
		signupGate, signupIssuer = gate, gate
	}

	// Inbound anti-abuse chain (rspamd if configured).
	chain := filter.NewChain()
	if rs := env("RSPAMD_URL", ""); rs != "" {
		chain.Add(scan.NewRspamd(rs, 8.0))
		log.Printf("rspamd spam scanning enabled: %s", rs)
	}
	mgr.Inbound = chain

	// Account settings + vacation responder.
	mgr.Settings = mailsettings.NewStore()
	mgr.Vacation = mailsettings.NewResponder(0, nil)

	// Multi-tenancy: registry + optional per-tenant daily message quota.
	mgr.Registry = tenant.NewRegistry()
	if q := env("VULOS_DAILY_MSG_QUOTA", ""); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			mgr.Quota = tenant.NewQuota(n, 0, nil)
			log.Printf("per-tenant daily message quota: %d", n)
		}
	}

	// Outbound abuse filter (rate + recipient-burst auto-suspend).
	abuseFilter := abuse.New(abuse.Config{})

	// Inbound credential-check brute-force limiter, shared across IMAP, SMTP
	// submission, JMAP Basic auth, AND the webmail HTTP auth endpoints (login,
	// in-place password change, the /api/webmail/send Basic-auth gate, and the
	// /api/llm gate) — keyed per client IP and per account.
	authLimiter := authlimit.New(authlimit.Config{})

	// Trusted fronting-proxy allowlist (CIDR/IP). Honoured for X-Forwarded-For
	// (rate-limit keying) and X-Forwarded-Proto (Secure-cookie decision) — only
	// from a peer in this list, so a direct client can't spoof either header.
	var trustedProxies []string
	if v := env("VULOS_TRUSTED_PROXIES", ""); v != "" {
		trustedProxies = strings.Split(v, ",")
	}
	trustedNets := parseTrustedProxies(trustedProxies)
	// guard wraps the brute-force limiter for the HTTP auth endpoints.
	guard := &authGuard{lim: authLimiter, trusted: trustedNets}

	// Listeners.
	authn := &emailauth.Authenticator{} // real DNS
	mx := smtpin.NewServer(&smtpin.Backend{
		Deliver:    mgr.Deliver,
		AuthServID: domain,
		KnownRcpt:  mgr.IsLocal, // reject unknown recipients at RCPT (550 5.1.1)
		VerifyVerdict: func(raw []byte, ip net.IP, helo, mailFrom string) smtpin.AuthVerdict {
			// Bound DNS-backed auth (SPF/DMARC) so a slow/dead resolver can never
			// stall the delivery path.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			res := authn.Verify(ctx, raw, ip, helo, mailFrom)
			// Enforce DMARC only at p=reject; quarantine/none stay annotate-only.
			reject := res.DMARC == "fail" && res.DMARCPolicy == "reject"
			return smtpin.AuthVerdict{AuthResults: res.AuthResults(), Reject: reject}
		},
	}, mxAddr, domain)
	mx.TLSConfig = tlsCfg
	sub := smtpin.NewSubmitServer(&smtpin.SubmitBackend{Auth: mgr.AuthSubmit, Enqueue: mgr.Enqueue, Signer: mgr.Signer, Abuse: abuseFilter, Quota: mgr.CheckQuota, Limiter: authLimiter}, subAddr, domain)
	sub.TLSConfig = tlsCfg
	imapBe := &imapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }, Limiter: authLimiter}
	imapSrv := imapadapter.NewServer(imapBe, tlsCfg)

	jmapBe := &jmapadapter.Backend{
		Auth:    func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) },
		Limiter: authLimiter,
		Submit: func(ctx context.Context, account string, raw []byte) error {
			env, err := mime.ParseEnvelope(raw)
			if err != nil {
				return err
			}
			return mgr.SendRaw(ctx, account, append(append([]string{}, env.To...), env.Cc...), raw)
		},
	}

	// Shared HTTP auth for DAV: validate IMAP creds, return the account id.
	davAuth := func(u, p string) (string, bool) {
		if _, err := mgr.AuthIMAP(u, p); err == nil {
			return u, true
		}
		return "", false
	}
	calStore, err := caldav.NewFSStore(dataDir + "/dav/calendar")
	if err != nil {
		log.Fatalf("calendar store: %v", err)
	}
	caldavBe := caldav.New(davAuth, calStore)
	contactStore, err := carddav.NewFSStore(dataDir + "/dav/contacts")
	if err != nil {
		log.Fatalf("contacts store: %v", err)
	}
	carddavBe := &carddav.Backend{Auth: carddav.Auth(davAuth), Store: contactStore}
	apiKey := env("VULOS_API_KEY", "")
	webapiBe := &webapi.Backend{
		AuthKey: func(k string) (string, bool) { return acct, apiKey != "" && k == apiKey },
		Submit:  mgr.SendRaw,
	}

	// One HTTP server multiplexing the webmail UI, JMAP, CalDAV, CardDAV, and APIs.
	httpMux := http.NewServeMux()
	// Self-serve signup (anti-abuse gated) — provisions handle@domain via the
	// active identity provider. Disabled when VULOS_SIGNUP=off.
	if env("VULOS_SIGNUP", "") != "off" {
		signupRate := 10
		if v := env("VULOS_SIGNUP_RATE_PER_HOUR", ""); v != "" {
			if n, e := strconv.Atoi(v); e == nil && n > 0 {
				signupRate = n
			}
		}
		signupH := signup.Handler(signup.Config{
			Domain: domain, Gate: signupGate, Issuer: signupIssuer, Provision: mgr.AddAccount,
			RatePerHour: signupRate, TrustedProxies: trustedProxies,
		})
		httpMux.Handle("/api/signup", signupH)  // exact: create account
		httpMux.Handle("/api/signup/", signupH) // subtree: /challenge
		log.Printf("self-serve signup enabled at /api/signup (anti-abuse: altcha PoW)")
	}
	// Optional LLM features (summaries, smart replies, …) routed through the
	// suite's llmux gateway — provider routing + per-account budget + token-cost
	// metering are handled centrally (billed via cp in a Vulos deployment). Off
	// unless VULOS_LLMUX_URL is set, so mail stays a pure mail server by default.
	if lurl := env("VULOS_LLMUX_URL", ""); lurl != "" {
		staticKey := env("VULOS_LLMUX_KEY", "") // cloud deployments resolve a per-account key from cp instead
		keyFor := func(string) (string, bool) { return staticKey, staticKey != "" }
		if px := llm.New(lurl, keyFor); px != nil {
			llmAuth := func(r *http.Request) (string, bool) {
				u, p, ok := r.BasicAuth()
				if !ok {
					return "", false
				}
				// Per-IP/per-account brute-force throttle (shared limiter). A
				// locked key is refused (401) without reaching the credential
				// check; the Handler has no way to surface a 429 here.
				ip := clientIP(r, guard.trusted)
				if guard.lim.AnyLocked(ip, u) {
					return "", false
				}
				if _, err := mgr.AuthIMAP(u, p); err != nil {
					guard.lim.Fail(ip, u)
					return "", false
				}
				guard.lim.Success(ip, u)
				return u, true
			}
			httpMux.Handle("/api/llm/", px.Handler("/api/llm", llmAuth))
			log.Printf("LLM features enabled via llmux at %s", lurl)
		}
	}
	httpMux.Handle("/jmap/", jmapBe.Handler())
	httpMux.Handle("/dav/calendars/", caldavBe.Handler())
	httpMux.Handle("/dav/addressbooks/", carddavBe.Handler())
	httpMux.Handle("/api/", webapiBe.Handler())

	// ── Webmail: lilmail mail-engine wiring ──────────────────────────────────
	// The bundled webmail (@vulos/mail-ui <MailApp/>) talks exclusively to the
	// lilmail /v1 JSON API for all mail data (identity, folders, messages, search,
	// flags, delete, send). In the standalone deployment that engine is a lilmail
	// process pointed at THIS server's IMAP/SMTP; vulos-mail brokers each /v1
	// request to it using the signed-in user's credentials (lilmail's CP-brokered
	// credential mode, gated by LILMAIL_BROKER_SECRET).
	//
	// Auth model: the webmail signs in via POST /api/webmail/login, which validates
	// the mailbox credentials and mints an HttpOnly session cookie. The password is
	// held server-side in the session (never in the browser) so the /v1 proxy can
	// present it to the engine as broker headers on every request. The mail-ui's
	// own /v1 calls ride that cookie (credentials: 'include').
	webSessions := newWebSessionStore(12 * time.Hour)
	// The session cookie must be Secure whenever the browser↔server hop is HTTPS.
	// That hop is HTTPS when we terminate TLS ourselves (tlsCfg != nil), when TLS
	// is terminated by a trusted upstream proxy (X-Forwarded-Proto: https from a
	// trusted peer), or when the operator forces it (VULOS_FORCE_SECURE_COOKIE)
	// for a deployment fronted by TLS that doesn't set XFP.
	forceSecureCookie := env("VULOS_FORCE_SECURE_COOKIE", "") != ""
	secureCookieFor := func(r *http.Request) bool {
		if tlsCfg != nil || forceSecureCookie {
			return true
		}
		return trustedPeer(r, trustedNets) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
	setSessionCookie := func(w http.ResponseWriter, r *http.Request, tok string, maxAge int) {
		http.SetCookie(w, &http.Cookie{
			Name: webSessionCookie, Value: tok, Path: "/",
			HttpOnly: true, Secure: secureCookieFor(r), SameSite: http.SameSiteLaxMode,
			MaxAge: maxAge,
		})
	}
	// Sign in: validate mailbox credentials (Basic auth or JSON body), then mint a
	// webmail session cookie holding them server-side for the /v1 proxy to broker.
	authIMAP := func(u, p string) error { _, err := mgr.AuthIMAP(u, p); return err }
	httpMux.HandleFunc("/api/webmail/login", webmailLoginHandler(webSessions, authIMAP, guard, setSessionCookie))
	// Sign out: drop the server-side session and clear the cookie.
	httpMux.HandleFunc("/api/webmail/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(webSessionCookie); err == nil {
			webSessions.delete(c.Value)
		}
		setSessionCookie(w, r, "", -1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	// Mailbox connection settings (host/port) — both brokered to the lilmail
	// engine below AND surfaced to the self-hoster so they can configure a desktop
	// or mobile mail client (Thunderbird, Apple Mail, K-9…) against this server.
	imapHost := env("VULOS_MAIL_IMAP_HOST", domain)
	imapPort := env("VULOS_MAIL_IMAP_PORT", "993")
	smtpHost := env("VULOS_MAIL_SMTP_HOST", domain)
	smtpPort := env("VULOS_MAIL_SMTP_PORT", "587")
	// Standalone (local identity) owns passwords, so the webmail can offer an
	// in-place change-password control. Under the cloud control plane, identity is
	// owned elsewhere, so the control is hidden — the UI only ever offers what the
	// backend can actually do.
	passwordChangeable := env("VULOS_CP_URL", "") == ""
	signupEnabled := env("VULOS_SIGNUP", "") != "off"
	engineConfigured := env("LILMAIL_ENGINE_URL", "") != ""
	// appsEnabled is set once the Apps & Bots place mounts (below). Declared here
	// so the account-capabilities surface and the /apps SPA deep-link can read it.
	appsEnabled := false

	// Calendar/Contacts standalone: the /v1 proxy strips any client-supplied
	// CalDAV/CardDAV base URLs (SSRF guard) and otherwise never re-adds them, so
	// the mounted Calendar/Contacts surfaces would be non-functional. When the
	// operator configures TRUSTED DAV base URLs here, the proxy injects them on
	// every request (after the strip) so lilmail dials them with the brokered
	// credential — making cal/contacts work standalone while forged values stay
	// stripped. Left unset → the surfaces stay hidden.
	//
	// NOTE: lilmail's brokered DAV dial presents X-Vulos-Mail-Secret as an HTTP
	// Bearer token (oauth2 mode), so the configured endpoints must accept Bearer
	// auth. vulos-mail's own /dav backend is Basic-auth (IMAP creds) only, so we
	// do NOT auto-derive these from the engine; they are explicit, config-gated,
	// and default off. See SELFHOST.md.
	caldavURL := strings.TrimSpace(env("LILMAIL_CALDAV_URL", ""))
	carddavURL := strings.TrimSpace(env("LILMAIL_CARDDAV_URL", ""))

	// Account surface for the standalone self-hoster: the signed-in identity, the
	// exact IMAP/SMTP connection settings for an external client, and which
	// account operations this deployment supports. Session-authenticated.
	httpMux.HandleFunc("/api/webmail/account", func(w http.ResponseWriter, r *http.Request) {
		sess, ok := webSessions.get(cookieValue(r, webSessionCookie))
		if !ok {
			writeJSONErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":  sess.user,
			"domain": domain,
			"imap":   map[string]string{"host": imapHost, "port": imapPort, "security": "SSL/TLS"},
			"smtp":   map[string]string{"host": smtpHost, "port": smtpPort, "security": "STARTTLS"},
			"capabilities": map[string]bool{
				"changePassword": passwordChangeable,
				"signup":         signupEnabled,
				"engine":         engineConfigured,
				// Calendar/Contacts only function standalone when a trusted DAV
				// base URL is configured (the proxy injects it); else hidden.
				"calendar": engineConfigured && caldavURL != "",
				"contacts": engineConfigured && carddavURL != "",
				// Apps & Bots manage surface (mail.vulos.org/apps) is available when
				// the place is mounted (read by reference at request time).
				"apps": appsEnabled,
			},
		})
	})

	// Change the signed-in mailbox password in place (local identity only). The
	// current password is re-verified, the new one is persisted, and the live
	// session's brokered credential is rotated so /v1 keeps working without a
	// forced re-login.
	httpMux.HandleFunc("/api/webmail/account/password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		tok := cookieValue(r, webSessionCookie)
		sess, ok := webSessions.get(tok)
		if !ok {
			writeJSONErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !passwordChangeable {
			writeJSONErr(w, http.StatusNotImplemented, "password changes are managed by the control plane")
			return
		}
		var req struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "bad request")
			return
		}
		if len(req.NewPassword) < 8 {
			writeJSONErr(w, http.StatusBadRequest, "new password must be at least 8 characters")
			return
		}
		ip, ok := guard.begin(w, r, sess.user)
		if !ok {
			return
		}
		if _, err := mgr.AuthIMAP(sess.user, req.CurrentPassword); err != nil {
			guard.fail(ip, sess.user)
			writeJSONErr(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
		guard.success(ip, sess.user)
		if err := localID.Upsert(sess.user, req.NewPassword); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, "could not update password")
			return
		}
		// Rotate the brokered credential in the live session so subsequent /v1
		// requests authenticate with the new password.
		webSessions.setPass(tok, req.NewPassword)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	// /v1/* — the lilmail mail-engine surface the webmail reads from. With
	// LILMAIL_ENGINE_URL set, proxy to that engine and inject the signed-in user's
	// mailbox credentials as lilmail broker headers. Unset → a clear "engine not
	// configured" response (503) rather than a confusing 404.
	if engineURL := env("LILMAIL_ENGINE_URL", ""); engineURL == "" {
		httpMux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"mail engine not configured (set LILMAIL_ENGINE_URL to a lilmail engine)"}`))
		})
		log.Printf("webmail: LILMAIL_ENGINE_URL unset — /v1 mail data disabled; the bundled webmail will show \"mail engine not configured\"")
	} else {
		target, perr := url.Parse(engineURL)
		if perr != nil {
			log.Fatalf("LILMAIL_ENGINE_URL %q: %v", engineURL, perr)
		}
		// TrimSpace to match lilmail's verifier, which trims its own configured
		// secret (lilmail/handlers/jsonapi/broker.go) but compares the presented
		// header value as-is — so any stray whitespace here would fail the gate.
		brokerSecret := strings.TrimSpace(env("LILMAIL_BROKER_SECRET", ""))
		if brokerSecret == "" {
			log.Printf("WARNING: LILMAIL_ENGINE_URL set but LILMAIL_BROKER_SECRET empty — the engine will ignore brokered credentials and /v1 reads will 401")
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		baseDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			baseDirector(req)
			req.Host = target.Host
		}
		httpMux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
			sess, ok := webSessions.get(cookieValue(r, webSessionCookie))
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"not authenticated"}`))
				return
			}
			// Never forward the browser's cookies/auth, and never let a client
			// spoof the broker headers — strip the full inbound credential set
			// (incl. the broker gate and the CalDAV/CardDAV base URLs) and then
			// set them fresh from the validated session.
			r.Header.Del("Cookie")
			r.Header.Del("Authorization")
			injectBrokerHeaders(r.Header, brokerSecret, sess.user, sess.pass, imapHost, imapPort, smtpHost, smtpPort, caldavURL, carddavURL)
			proxy.ServeHTTP(w, r)
		})
		if caldavURL != "" || carddavURL != "" {
			log.Printf("webmail: brokering DAV base URLs to engine (caldav=%q carddav=%q)", caldavURL, carddavURL)
		}
		log.Printf("webmail: proxying /v1 → lilmail engine %s (broker → imap %s:%s, smtp %s:%s)", engineURL, imapHost, imapPort, smtpHost, smtpPort)
	}

	// ── Apps & Bots place (shared @vulos/apps platform) ──────────────────────
	// The Mail product hosts an apps & bots place via the product-agnostic
	// appsplatform handler set + a Mail ProductAdapter. Apps act on / read mail
	// through lilmail's /v1 (the same engine the webmail proxies to), acting as a
	// configured service mailbox brokered with the validated server credential.
	//
	// Open-core seam: the standalone SQLite registry is the default. A Vulos Cloud
	// developer-console Registry would be wired here when selected (it implements
	// the SAME appsplatform.Registry in a CLOSED package this OSS core never
	// imports — see appsplatform/seam.go), so removing it never breaks this build.
	if env("VULOS_APPS", "") != "off" {
		if cp := env("VULOS_APPS_CP_URL", ""); cp != "" {
			// Cloud hook (env-gated): the cloud apps Registry is a closed package not
			// built into this OSS binary; fall back to the standalone default rather
			// than fail. A Vulos Cloud build wires cloudapps.New(cp) at this seam.
			log.Printf("apps: VULOS_APPS_CP_URL set but the cloud apps registry is not built into this OSS binary; using the standalone registry")
		}
		appsDB := env("VULOS_APPS_DB", filepath.Join(dataDir, "apps.db"))
		appsReg, aerr := appsplatform.NewStandaloneRegistry(appsDB,
			appsplatform.WithScopeSet(appsplatform.NewScopeSet(appsplatform.ScopeAppsRead, appsplatform.ScopeAppsWrite)))
		if aerr != nil {
			log.Printf("apps: registry unavailable (%v); apps & bots place disabled", aerr)
		} else {
			appsDisp := appsplatform.NewDispatcher(appsReg, appsplatform.ProductMail)
			mailAdapter := mailapps.New(mailapps.Config{
				EngineURL:    env("LILMAIL_ENGINE_URL", ""),
				BrokerSecret: strings.TrimSpace(env("LILMAIL_BROKER_SECRET", "")),
				Mailbox:      env("VULOS_MAIL_APPS_ACCOUNT", ""),
				Password:     env("VULOS_MAIL_APPS_PASSWORD", ""),
				IMAPHost:     imapHost, IMAPPort: imapPort, SMTPHost: smtpHost, SMTPPort: smtpPort,
			})
			// Management API auth: reuse the webmail session. The signed-in mailbox
			// is the owner; the seeded VULOS_ACCOUNT (if any) is treated as admin so
			// it can manage every app in a single-operator deployment.
			appsAdmin := func(r *http.Request) (string, bool, bool) {
				sess, ok := webSessions.get(cookieValue(r, webSessionCookie))
				if !ok {
					return "", false, false
				}
				return sess.user, acct != "" && sess.user == acct, true
			}
			appsH, herr := appsplatform.NewHandler(appsplatform.MountConfig{
				Adapter:    mailAdapter,
				Registry:   appsReg,
				Dispatcher: appsDisp,
				Admin:      appsAdmin,
				BasePath:   "/api/apps",
			})
			if herr != nil {
				log.Printf("apps: handler init failed (%v); apps & bots place disabled", herr)
			} else {
				httpMux.Handle("/api/apps", appsH)  // base
				httpMux.Handle("/api/apps/", appsH) // subtree (runtime, hooks, rotate, …)
				appsEnabled = true
				if mailAdapter.Configured() {
					log.Printf("apps & bots: mounted at /api/apps (acting as mailbox %s via engine %s)", env("VULOS_MAIL_APPS_ACCOUNT", ""), env("LILMAIL_ENGINE_URL", ""))
				} else {
					log.Printf("apps & bots: mounted at /api/apps (management only — set LILMAIL_ENGINE_URL + VULOS_MAIL_APPS_ACCOUNT/PASSWORD to let apps act on mail)")
				}

				// ── MCP server (same seam, agent shape) ──────────────────────
				// The MCP layer is a different shape over the SAME ProductAdapter,
				// Registry and vat_ token auth: the adapter's Act actions become MCP
				// tools and its Read kinds become MCP resources (described precisely
				// via the mcp.Descriptor the adapter implements). It ships standalone
				// in this OSS binary — point any MCP agent at /mcp with an apps token.
				//
				// Open-core seam: MCPConfig.Gateway (cloud MCP aggregation) is left
				// nil here; a Vulos Cloud composition root wires it env-gated. The OSS
				// core never imports a Gateway implementation.
				mcpH, merr := mcp.NewHandler(mcp.MCPConfig{
					Adapter:  mailAdapter,
					Registry: appsReg,
					Emit:     appsDisp.EmitFunc(),
					BasePath: "/mcp",
				})
				if merr != nil {
					log.Printf("mcp: handler init failed (%v); MCP server disabled", merr)
				} else {
					httpMux.Handle("/mcp", mcpH)  // base
					httpMux.Handle("/mcp/", mcpH) // subtree
					log.Printf("mcp: mounted at /mcp (agents operate Mail via the apps token + adapter)")
				}
			}
		}
	}

	// Webmail compose endpoint (Basic auth via the user's IMAP credentials). The
	// bundled webmail now sends through the /v1 proxy (POST /v1/messages → lilmail);
	// this endpoint remains as a simple authenticated send API for scripts/clients.
	httpMux.HandleFunc("/api/webmail/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="vulos-mail"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ip, ok := guard.begin(w, r, u)
		if !ok {
			return
		}
		if _, err := mgr.AuthIMAP(u, p); err != nil {
			guard.fail(ip, u)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		guard.success(ip, u)
		var req struct {
			To          []string `json:"to"`
			Cc          []string `json:"cc"`
			Subject     string   `json:"subject"`
			Text        string   `json:"text"`
			HTML        string   `json:"html"`
			Attachments []struct {
				Name string `json:"name"`
				Type string `json:"type"`
				Data string `json:"data"` // base64
			} `json:"attachments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.To)+len(req.Cc) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var atts []compose.Attachment
		for _, a := range req.Attachments {
			data, derr := base64.StdEncoding.DecodeString(a.Data)
			if derr != nil {
				http.Error(w, "bad attachment encoding", http.StatusBadRequest)
				return
			}
			atts = append(atts, compose.Attachment{Name: a.Name, Type: a.Type, Data: data})
		}
		if err := mgr.WebSendMsg(r.Context(), u, req.To, req.Cc, req.Subject, req.Text, req.HTML, atts); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	// ── Operations endpoints ─────────────────────────────────────────────────
	// Broker-gated admin + diagnostics, plus an unauthenticated liveness probe.
	// These use exact patterns so they take precedence over the "/api/" subtree
	// handler registered above regardless of ordering.
	adminBrokerSecret := strings.TrimSpace(env("LILMAIL_BROKER_SECRET", ""))
	// Free-mail mailbox provisioning seam (used by Cloud's free-org-mail feature).
	httpMux.HandleFunc("/api/admin/provision-mailbox", provisionMailboxHandler(mgr, domain, adminBrokerSecret))
	// Bulk PIM import: OS import engine writes owned contact/event copies here.
	registerImportHandlers(httpMux, contactStore, calStore, adminBrokerSecret)
	// Liveness for the status page / load balancer.
	httpMux.HandleFunc("/healthz", healthzHandler)
	// Advanced deliverability/health diagnostics (broker-gated JSON report).
	diagRunner := newDiagRunner()
	httpMux.HandleFunc("/api/diagnostics", diagnosticsHandler(diagRunner, adminBrokerSecret))
	if adminBrokerSecret == "" {
		log.Printf("ops: /api/diagnostics, /api/admin/provision-mailbox, and /api/admin/import/* are CLOSED (set LILMAIL_BROKER_SECRET to enable); /healthz is open")
	} else {
		log.Printf("ops: /healthz (open), /api/diagnostics + /api/admin/provision-mailbox + /api/admin/import/* (broker-gated)")
	}

	// Webmail static UI at the root (registered last; longest-prefix routing keeps
	// the API/DAV/JMAP handlers above taking precedence). The webmail is a
	// React+Vite SPA built into ./webmail/dist (run `cd webmail && npm run build`).
	if dir := env("VULOS_WEBMAIL_DIR", "./webmail/dist"); dir != "" {
		// The Mail product exposes Calendar and Contacts as standalone surfaces
		// (mail.vulos.org/calendar and /contacts). They are rendered by the same
		// single-page webmail bundle (App.jsx switches on the path), so these
		// routes must serve the SPA's index.html rather than 404 in the plain
		// FileServer. The client then mounts <Calendar/> / <Contacts/> against
		// the same /v1 session the mailbox uses. They are only mounted when a
		// trusted DAV base URL is configured (LILMAIL_CALDAV_URL /
		// LILMAIL_CARDDAV_URL) so the surfaces are actually functional; otherwise
		// the deep-link routes are left to 404 (surface hidden).
		serveSPA := func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		}
		if engineConfigured && caldavURL != "" {
			httpMux.HandleFunc("/calendar", serveSPA)
		}
		if engineConfigured && carddavURL != "" {
			httpMux.HandleFunc("/contacts", serveSPA)
		}
		// Apps & Bots manage surface (mail.vulos.org/apps), reachable from Settings.
		// Served as the SPA deep-link only when the place is actually mounted.
		if appsEnabled {
			httpMux.HandleFunc("/apps", serveSPA)
		}
		// /inbox is the explicit sign-in entry point: the marketing landing's
		// "Sign in" CTA targets it. It serves the SPA, which shows <Login/> while
		// unauthenticated and the mailbox once signed in (currentSurface() maps any
		// non calendar/contacts/apps path to the default mail surface).
		httpMux.HandleFunc("/inbox", serveSPA)

		// Standalone marketing landing for the Vulos Mail origin, mounted at /site/*
		// so the page's relative ./assets/... refs resolve once we inject a
		// <base href="/site/">. Public; it never shadows the SPA, which owns "/".
		if siteSub, subErr := fs.Sub(siteFS, "site"); subErr == nil {
			httpMux.Handle("/site/", http.StripPrefix("/site/", http.FileServer(http.FS(siteSub))))
		} else {
			log.Printf("marketing site unavailable: %v", subErr)
		}

		// Precompute the landing HTML once, injecting <base href="/site/"> into the
		// <head> so the embedded page's relative asset URLs resolve under /site/.
		var landingHTML []byte
		if data, readErr := siteFS.ReadFile("site/index.html"); readErr == nil {
			landingHTML = []byte(strings.Replace(string(data), "<head>", `<head><base href="/site/">`, 1))
		} else {
			log.Printf("marketing landing unavailable: %v", readErr)
		}

		// Auth-gated root. http.ServeMux routes "/" as a catch-all, so this handler
		// also receives every path the more specific handlers above did not claim
		// (SPA assets, unregistered deep-links). Only the EXACT "/" path branches to
		// the landing; everything else keeps the original FileServer behavior, and
		// signed-in visitors get the SPA at "/" too.
		fileServer := http.FileServer(http.Dir(dir))
		httpMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				fileServer.ServeHTTP(w, r)
				return
			}
			if _, ok := webSessions.get(cookieValue(r, webSessionCookie)); ok {
				fileServer.ServeHTTP(w, r)
				return
			}
			if landingHTML == nil {
				// No landing embedded — fall back to the SPA so "/" still works.
				fileServer.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(landingHTML)
		})
	}

	go serve("mx", mxAddr, mx.ListenAndServe)
	go serve("submission", subAddr, sub.ListenAndServe)
	go serve("imap", imapddr, func() error {
		ln, err := net.Listen("tcp", imapddr)
		if err != nil {
			return err
		}
		return imapSrv.Serve(ln)
	})
	go serve("http (jmap/dav/api)", jmapddr, func() error {
		srv := &http.Server{Addr: jmapddr, Handler: httpMux, TLSConfig: tlsCfg}
		if tlsCfg != nil {
			return srv.ListenAndServeTLS("", "") // certs come from TLSConfig
		}
		return srv.ListenAndServe()
	})

	// Outbound scheduler loop (+ metrics).
	go func() {
		ctx := context.Background()
		for {
			st := sched.Tick(ctx, time.Now())
			metrics.Outbound.WithLabelValues("delivered").Add(float64(st.Delivered))
			metrics.Outbound.WithLabelValues("deferred").Add(float64(st.Deferred))
			metrics.Outbound.WithLabelValues("bounced").Add(float64(st.Bounced))
			metrics.QueueDepth.Set(float64(sched.Pending()))
			time.Sleep(5 * time.Second)
		}
	}()

	// Periodic blob garbage collection (sweep unreferenced bodies; 1h grace).
	go func() {
		for {
			time.Sleep(6 * time.Hour)
			if n, err := mgr.GCBlobs(context.Background(), time.Hour); err == nil && n > 0 {
				log.Printf("blob GC: removed %d unreferenced blobs", n)
			}
		}
	}()

	// Periodic log compaction (snapshot + truncate so reopen stays fast).
	go func() {
		for {
			time.Sleep(12 * time.Hour)
			if n := mgr.CompactAll(context.Background()); n > 0 {
				log.Printf("log compaction: snapshotted %d accounts", n)
			}
		}
	}()

	// Metrics endpoint.
	if metricsAddr := env("VULOS_METRICS_ADDR", ":2090"); metricsAddr != "" {
		go serve("metrics", metricsAddr, func() error {
			mux := http.NewServeMux()
			mux.Handle("/metrics", metrics.Handler())
			return http.ListenAndServe(metricsAddr, mux)
		})
	}

	log.Printf("vulos-mail up: domain=%s mx=%s submit=%s imap=%s jmap=%s data=%s", domain, mxAddr, subAddr, imapddr, jmapddr, dataDir)
	select {} // block forever
}

func serve(name, addr string, fn func() error) {
	log.Printf("%s listening on %s", name, addr)
	if err := fn(); err != nil {
		log.Fatalf("%s: %v", name, err)
	}
}

// webSessionCookie is the name of the HttpOnly cookie that ties a browser to a
// server-side webmail session.
const webSessionCookie = "vm_session"

// webSession holds an authenticated webmail user's mailbox credentials. The
// plaintext password lives only in memory, for the lifetime of the session, so
// the /v1 reverse proxy can broker it to the lilmail engine on each request
// (lilmail dials this server's IMAP/SMTP with it) — the same credential-custody
// model the Vulos Cloud control plane uses.
type webSession struct {
	user    string
	pass    string
	expires time.Time
}

// webSessionStore is a small in-memory, TTL'd store of webmail sessions keyed by
// an opaque cookie token.
type webSessionStore struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]webSession
}

func newWebSessionStore(ttl time.Duration) *webSessionStore {
	return &webSessionStore{ttl: ttl, m: map[string]webSession{}}
}

// create mints a new random token bound to the given credentials and returns it.
func (s *webSessionStore) create(user, pass string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[tok] = webSession{user: user, pass: pass, expires: time.Now().Add(s.ttl)}
	return tok
}

// get returns the session for a token, dropping and rejecting expired ones.
func (s *webSessionStore) get(tok string) (webSession, bool) {
	if tok == "" {
		return webSession{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[tok]
	if !ok {
		return webSession{}, false
	}
	if time.Now().After(sess.expires) {
		delete(s.m, tok)
		return webSession{}, false
	}
	return sess, true
}

func (s *webSessionStore) delete(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, tok)
}

// setPass rotates the brokered password held in an existing session (after an
// in-place password change), preserving the token and expiry so the browser
// stays signed in.
func (s *webSessionStore) setPass(tok, pass string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.m[tok]; ok {
		sess.pass = pass
		s.m[tok] = sess
	}
}

// writeJSONErr writes a JSON {"error": msg} body with the given status code.
func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// mailBrokerHeaders is the complete set of broker/credential headers the lilmail
// engine honors. The /v1 reverse proxy strips every one of these from the inbound
// request before injecting its own trusted values, so a session-holding client
// can't forge any of them (notably X-Vulos-Mail-Caldav-Url / -Carddav-Url, which
// lilmail would otherwise dial with the user's brokered credential — SSRF /
// credential-exfil). Mirrors vulos-cloud's CP strip set (routes_mail.go).
var mailBrokerHeaders = []string{
	// Broker gate secret — must be stripped inbound so a client can't forge it.
	"X-Vulos-Broker-Auth",
	"X-Vulos-Mail-Provider", "X-Vulos-Mail-Email", "X-Vulos-Mail-Username",
	"X-Vulos-Mail-Auth", "X-Vulos-Mail-Secret",
	"X-Vulos-Mail-Imap-Host", "X-Vulos-Mail-Imap-Port",
	"X-Vulos-Mail-Smtp-Host", "X-Vulos-Mail-Smtp-Port",
	// CalDAV/CardDAV brokered base URLs. Always stripped inbound; re-added by
	// injectBrokerHeaders ONLY from trusted operator config (LILMAIL_CALDAV_URL /
	// LILMAIL_CARDDAV_URL), never from a client-supplied value.
	"X-Vulos-Mail-Caldav-Url", "X-Vulos-Mail-Carddav-Url",
}

// injectBrokerHeaders strips the full inbound broker/credential header set from h
// and then sets the trusted values for the validated IMAP session. Stripping
// first (rather than relying on Set's overwrite) guarantees that headers a client
// forges cannot be smuggled through to the lilmail engine. The CalDAV/CardDAV
// base URLs are re-added ONLY from the operator-configured (trusted) caldavURL /
// carddavURL — an empty value leaves the header stripped, so a forged inbound
// value can never survive and the cal/contacts surfaces simply stay inert.
func injectBrokerHeaders(h http.Header, brokerSecret, user, pass, imapHost, imapPort, smtpHost, smtpPort, caldavURL, carddavURL string) {
	for _, hdr := range mailBrokerHeaders {
		h.Del(hdr)
	}
	h.Set("X-Vulos-Broker-Auth", brokerSecret)
	h.Set("X-Vulos-Mail-Provider", "imap")
	h.Set("X-Vulos-Mail-Email", user)
	h.Set("X-Vulos-Mail-Username", user)
	h.Set("X-Vulos-Mail-Auth", "plain")
	h.Set("X-Vulos-Mail-Secret", pass)
	h.Set("X-Vulos-Mail-Imap-Host", imapHost)
	h.Set("X-Vulos-Mail-Imap-Port", imapPort)
	h.Set("X-Vulos-Mail-Smtp-Host", smtpHost)
	h.Set("X-Vulos-Mail-Smtp-Port", smtpPort)
	// Trusted, operator-configured DAV base URLs only (config-gated). Stripped
	// above when unset so client-forged values never reach the engine.
	if caldavURL != "" {
		h.Set("X-Vulos-Mail-Caldav-Url", caldavURL)
	}
	if carddavURL != "" {
		h.Set("X-Vulos-Mail-Carddav-Url", carddavURL)
	}
}

// cookieValue returns the value of the named cookie, or "" when absent.
func cookieValue(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

// authGuard applies the shared brute-force limiter (internal/authlimit) to the
// webmail HTTP auth endpoints, keyed per client IP and per account so an
// attacker can neither spray one password across accounts from one host nor
// grind one account from many hosts. It is the HTTP counterpart to the limiter
// already wired into the IMAP/SMTP/JMAP adapters.
type authGuard struct {
	lim     *authlimit.Limiter
	trusted []*net.IPNet
}

// begin resolves the request's client IP (honouring trusted-proxy XFF) and
// reports whether the request may proceed to the real credential check. When the
// IP or account is currently locked out it writes a 429 JSON error and returns
// ok=false; the caller must return. The returned ip is passed to fail/success.
func (g *authGuard) begin(w http.ResponseWriter, r *http.Request, account string) (ip string, ok bool) {
	ip = clientIP(r, g.trusted)
	if g.lim != nil && g.lim.AnyLocked(ip, account) {
		writeJSONErr(w, http.StatusTooManyRequests, "too many failed attempts; try again later")
		return ip, false
	}
	return ip, true
}

// fail records a failed credential check for the IP and account.
func (g *authGuard) fail(ip, account string) {
	if g.lim != nil {
		g.lim.Fail(ip, account)
	}
}

// success clears the failure history after a correct credential check.
func (g *authGuard) success(ip, account string) {
	if g.lim != nil {
		g.lim.Success(ip, account)
	}
}

// webmailLoginHandler builds the POST /api/webmail/login handler: it validates
// mailbox credentials (Basic auth or JSON body) under the brute-force guard,
// then mints a session cookie holding them server-side for the /v1 proxy. It is a
// standalone constructor so the rate-limited auth path is unit-testable.
func webmailLoginHandler(sessions *webSessionStore, authIMAP func(user, pass string) error, guard *authGuard, setCookie func(http.ResponseWriter, *http.Request, string, int)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var u, p string
		if bu, bp, ok := r.BasicAuth(); ok {
			u, p = bu, bp
		} else {
			var req struct {
				User     string `json:"user"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
				return
			}
			u, p = req.User, req.Password
		}
		ip, ok := guard.begin(w, r, u)
		if !ok {
			return
		}
		if err := authIMAP(u, p); err != nil {
			guard.fail(ip, u)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
			return
		}
		guard.success(ip, u)
		tok := sessions.create(u, p)
		setCookie(w, r, tok, int((12 * time.Hour).Seconds()))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"email": u, "username": u})
	}
}

// parseTrustedProxies parses CIDR / bare-IP strings into networks. Mirrors the
// signup handler's parsing so the webmail HTTP endpoints key their rate limit on
// the same trusted-proxy model. Invalid entries are skipped.
func parseTrustedProxies(list []string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range list {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			if ip := net.ParseIP(c); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				c = c + "/" + strconv.Itoa(bits)
			}
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ipTrusted reports whether ip (a string) is within any trusted network.
func ipTrusted(ip string, trusted []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// trustedPeer reports whether the request's immediate peer (RemoteAddr) is a
// trusted fronting proxy — used to decide whether X-Forwarded-Proto may be
// believed for the Secure-cookie decision.
func trustedPeer(r *http.Request, trusted []*net.IPNet) bool {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	}
	return ipTrusted(peer, trusted)
}

// clientIP returns the request's client IP. X-Forwarded-For is honoured ONLY
// when the immediate peer is in the trusted-proxy allowlist; otherwise an
// untrusted client could spoof its rate-limit key with a crafted XFF header. The
// rightmost untrusted address in the XFF chain is used. Mirrors signup.clientIP.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	}
	if !ipTrusted(peer, trusted) {
		return peer
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		if !ipTrusted(hop, trusted) {
			return hop
		}
	}
	return peer
}
