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

	imapadapter "github.com/vul-os/vulos-mail/adapters/imap"
	jmapadapter "github.com/vul-os/vulos-mail/adapters/jmap"
	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
	"github.com/vul-os/vulos-mail/adapters/webapi"
	"github.com/vul-os/vulos-mail/caldav"
	"github.com/vul-os/vulos-mail/carddav"
	cloud "github.com/vul-os/vulos-mail/integration/cloud"
	"github.com/vul-os/vulos-mail/internal/abuse"
	"github.com/vul-os/vulos-mail/internal/account"
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

	// Outbound: warm-IP pool + reputation + scheduler over a real SMTP sender.
	pool := mtaout.NewPool([]string{}, []string{}) // configure source IPs in prod
	rep := mtaout.NewReputation(100, 0.10, 0.02)
	warm := mtaout.NewWarmup([]int{50, 100, 500, 1000, 5000, 10000, 50000})
	sched := mtaout.NewScheduler(mtaout.Config{
		Sender: &mtaout.SMTPSender{HELO: domain}, Pool: pool, Warmup: warm, Reputation: rep,
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
	// submission, and JMAP Basic auth (keyed per client IP and per account).
	authLimiter := authlimit.New(authlimit.Config{})

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
		var trustedProxies []string
		if v := env("VULOS_TRUSTED_PROXIES", ""); v != "" {
			trustedProxies = strings.Split(v, ",")
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
				if _, err := mgr.AuthIMAP(u, p); err != nil {
					return "", false
				}
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
	secureCookie := tlsCfg != nil
	setSessionCookie := func(w http.ResponseWriter, tok string, maxAge int) {
		http.SetCookie(w, &http.Cookie{
			Name: webSessionCookie, Value: tok, Path: "/",
			HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode,
			MaxAge: maxAge,
		})
	}
	// Sign in: validate mailbox credentials (Basic auth or JSON body), then mint a
	// webmail session cookie holding them server-side for the /v1 proxy to broker.
	httpMux.HandleFunc("/api/webmail/login", func(w http.ResponseWriter, r *http.Request) {
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
		if _, err := mgr.AuthIMAP(u, p); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
			return
		}
		tok := webSessions.create(u, p)
		setSessionCookie(w, tok, int((12 * time.Hour).Seconds()))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"email": u, "username": u})
	})
	// Sign out: drop the server-side session and clear the cookie.
	httpMux.HandleFunc("/api/webmail/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(webSessionCookie); err == nil {
			webSessions.delete(c.Value)
		}
		setSessionCookie(w, "", -1)
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
		brokerSecret := env("LILMAIL_BROKER_SECRET", "")
		if brokerSecret == "" {
			log.Printf("WARNING: LILMAIL_ENGINE_URL set but LILMAIL_BROKER_SECRET empty — the engine will ignore brokered credentials and /v1 reads will 401")
		}
		imapHost := env("VULOS_MAIL_IMAP_HOST", domain)
		imapPort := env("VULOS_MAIL_IMAP_PORT", "993")
		smtpHost := env("VULOS_MAIL_SMTP_HOST", domain)
		smtpPort := env("VULOS_MAIL_SMTP_PORT", "587")
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
			// spoof the broker headers — set them fresh from the validated session.
			r.Header.Del("Cookie")
			r.Header.Del("Authorization")
			r.Header.Set("X-Vulos-Broker-Auth", brokerSecret)
			r.Header.Set("X-Vulos-Mail-Provider", "imap")
			r.Header.Set("X-Vulos-Mail-Email", sess.user)
			r.Header.Set("X-Vulos-Mail-Username", sess.user)
			r.Header.Set("X-Vulos-Mail-Auth", "plain")
			r.Header.Set("X-Vulos-Mail-Secret", sess.pass)
			r.Header.Set("X-Vulos-Mail-Imap-Host", imapHost)
			r.Header.Set("X-Vulos-Mail-Imap-Port", imapPort)
			r.Header.Set("X-Vulos-Mail-Smtp-Host", smtpHost)
			r.Header.Set("X-Vulos-Mail-Smtp-Port", smtpPort)
			proxy.ServeHTTP(w, r)
		})
		log.Printf("webmail: proxying /v1 → lilmail engine %s (broker → imap %s:%s, smtp %s:%s)", engineURL, imapHost, imapPort, smtpHost, smtpPort)
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
		if _, err := mgr.AuthIMAP(u, p); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
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
	// Webmail static UI at the root (registered last; longest-prefix routing keeps
	// the API/DAV/JMAP handlers above taking precedence). The webmail is a
	// React+Vite SPA built into ./webmail/dist (run `cd webmail && npm run build`).
	if dir := env("VULOS_WEBMAIL_DIR", "./webmail/dist"); dir != "" {
		// The Mail product exposes Calendar and Contacts as standalone surfaces
		// (mail.vulos.org/calendar and /contacts). They are rendered by the same
		// single-page webmail bundle (App.jsx switches on the path), so these
		// routes must serve the SPA's index.html rather than 404 in the plain
		// FileServer. The client then mounts <Calendar/> / <Contacts/> against
		// the same /v1 session the mailbox uses.
		serveSPA := func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		}
		httpMux.HandleFunc("/calendar", serveSPA)
		httpMux.HandleFunc("/contacts", serveSPA)
		httpMux.Handle("/", http.FileServer(http.Dir(dir)))
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

// cookieValue returns the value of the named cookie, or "" when absent.
func cookieValue(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}
