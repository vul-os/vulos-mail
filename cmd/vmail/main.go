// Command vmail runs the mail system: MX (receive), submission (send), and IMAP
// (serve) listeners over a shared account manager, plus the outbound scheduler
// loop. Configuration is via environment variables; account provisioning here is
// a single demo account (real provisioning/auth is a later wave).
package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	imapadapter "github.com/vul-os/vmail/adapters/imap"
	jmapadapter "github.com/vul-os/vmail/adapters/jmap"
	smtpin "github.com/vul-os/vmail/adapters/smtp"
	"github.com/vul-os/vmail/adapters/webapi"
	"github.com/vul-os/vmail/caldav"
	"github.com/vul-os/vmail/carddav"
	"github.com/vul-os/vmail/internal/abuse"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/emailauth"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/filter"
	"github.com/vul-os/vmail/internal/mailsettings"
	"github.com/vul-os/vmail/internal/metrics"
	"github.com/vul-os/vmail/internal/scan"
	"github.com/vul-os/vmail/internal/server"
	"github.com/vul-os/vmail/internal/tenant"
	"github.com/vul-os/vmail/internal/tlsconf"
	"github.com/vul-os/vmail/services/mtaout"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	var (
		dataDir = env("VMAIL_DATA_DIR", "./data")
		domain  = env("VMAIL_DOMAIN", "vmail.test")
		mxAddr  = env("VMAIL_MX_ADDR", ":2525")
		subAddr = env("VMAIL_SUBMIT_ADDR", ":2587")
		imapddr = env("VMAIL_IMAP_ADDR", ":2143")
		jmapddr = env("VMAIL_JMAP_ADDR", ":2080")
		acct    = env("VMAIL_ACCOUNT", "")
		pass    = env("VMAIL_PASSWORD", "")
	)

	// Blob store: S3-compatible if configured, else local FS (both zstd + dedup).
	var blobs blob.Store
	if ep := env("VMAIL_S3_ENDPOINT", ""); ep != "" {
		s3, serr := blob.NewS3(context.Background(), ep, env("VMAIL_S3_KEY", ""), env("VMAIL_S3_SECRET", ""), env("VMAIL_S3_BUCKET", "vmail"), env("VMAIL_S3_SSL", "") != "")
		if serr != nil {
			log.Fatalf("s3 blob store: %v", serr)
		}
		blobs = s3
		log.Printf("blob store: S3 %s/%s", ep, env("VMAIL_S3_BUCKET", "vmail"))
	} else {
		fs, ferr := blob.NewFS(dataDir + "/blobs")
		if ferr != nil {
			log.Fatalf("blob store: %v", ferr)
		}
		blobs = fs
	}

	// TLS: bring-your-own cert/key, or self-signed for dev (VMAIL_TLS_SELFSIGNED=1).
	var selfSigned []string
	if env("VMAIL_TLS_SELFSIGNED", "") != "" {
		selfSigned = []string{domain, "localhost"}
	}
	tlsCfg, err := tlsconf.Config(env("VMAIL_TLS_CERT", ""), env("VMAIL_TLS_KEY", ""), selfSigned...)
	if err != nil {
		log.Fatalf("tls: %v", err)
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
	if env("VMAIL_DB", "") == "sqlite" {
		mgr.LogOpen = func(d string) (eventlog.Log, error) { return eventlog.OpenSQLite(filepath.Join(d, "log.db"), nil) }
		log.Printf("event log backend: sqlite")
	}
	sched.SetOnBounce(func(msg mtaout.OutMessage, reason string) { mgr.HandleBounce(domain, msg, reason) })
	if txt, err := mgr.EnsureDKIM(domain, "vmail"); err == nil && txt != "" {
		log.Printf("DKIM: publish TXT at vmail._domainkey.%s :  %s", domain, txt)
	}
	if acct != "" && pass != "" {
		mgr.AddAccount(acct, pass)
		log.Printf("provisioned account %s", acct)
	} else {
		log.Printf("no VMAIL_ACCOUNT/VMAIL_PASSWORD set; no accounts provisioned")
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
	if q := env("VMAIL_DAILY_MSG_QUOTA", ""); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			mgr.Quota = tenant.NewQuota(n, 0, nil)
			log.Printf("per-tenant daily message quota: %d", n)
		}
	}

	// Outbound abuse filter (rate + recipient-burst auto-suspend).
	abuseFilter := abuse.New(abuse.Config{})

	// Listeners.
	authn := &emailauth.Authenticator{} // real DNS
	mx := smtpin.NewServer(&smtpin.Backend{
		Deliver:    mgr.Deliver,
		AuthServID: domain,
		Verify: func(raw []byte, ip net.IP, helo, mailFrom string) string {
			// Bound DNS-backed auth (SPF/DMARC) so a slow/dead resolver can never
			// stall the delivery path.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return authn.Verify(ctx, raw, ip, helo, mailFrom).AuthResults()
		},
	}, mxAddr, domain)
	mx.TLSConfig = tlsCfg
	sub := smtpin.NewSubmitServer(&smtpin.SubmitBackend{Auth: mgr.AuthSubmit, Enqueue: mgr.Enqueue, Signer: mgr.Signer, Abuse: abuseFilter, Quota: mgr.CheckQuota}, subAddr, domain)
	sub.TLSConfig = tlsCfg
	imapBe := &imapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}
	imapSrv := imapadapter.NewServer(imapBe, tlsCfg)

	jmapBe := &jmapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}

	// Shared HTTP auth for DAV: validate IMAP creds, return the account id.
	davAuth := func(u, p string) (string, bool) {
		if _, err := mgr.AuthIMAP(u, p); err == nil {
			return u, true
		}
		return "", false
	}
	caldavBe := caldav.New(davAuth, caldav.NewMemStore())
	carddavBe := &carddav.Backend{Auth: carddav.Auth(davAuth), Store: carddav.NewMemStore()}
	apiKey := env("VMAIL_API_KEY", "")
	webapiBe := &webapi.Backend{
		AuthKey: func(k string) (string, bool) { return acct, apiKey != "" && k == apiKey },
		Submit:  mgr.SendRaw,
	}

	// One HTTP server multiplexing the webmail UI, JMAP, CalDAV, CardDAV, and APIs.
	httpMux := http.NewServeMux()
	httpMux.Handle("/jmap/", jmapBe.Handler())
	httpMux.Handle("/dav/calendars/", caldavBe.Handler())
	httpMux.Handle("/dav/addressbooks/", carddavBe.Handler())
	httpMux.Handle("/api/", webapiBe.Handler())
	// Webmail compose endpoint (Basic auth via the user's IMAP credentials).
	httpMux.HandleFunc("/api/webmail/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="vmail"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := mgr.AuthIMAP(u, p); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			To      []string `json:"to"`
			Subject string   `json:"subject"`
			Text    string   `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.To) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := mgr.WebSend(r.Context(), u, req.To, req.Subject, req.Text); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	// Webmail settings (signature + vacation), Basic auth.
	httpMux.HandleFunc("/api/webmail/settings", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="vmail"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := mgr.AuthIMAP(u, p); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		type vac struct {
			Enabled bool   `json:"enabled"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		type dto struct {
			Signature string `json:"signature"`
			Vacation  vac    `json:"vacation"`
		}
		if r.Method == http.MethodPost {
			var d dto
			if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			cur := mgr.GetSettings(u)
			cur.Signature = d.Signature
			cur.Vacation = mailsettings.Vacation{Enabled: d.Vacation.Enabled, Subject: d.Vacation.Subject, Body: d.Vacation.Body}
			mgr.SetSettings(u, cur)
		}
		s := mgr.GetSettings(u)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dto{Signature: s.Signature, Vacation: vac{s.Vacation.Enabled, s.Vacation.Subject, s.Vacation.Body}})
	})
	// Webmail static UI at the root (registered last; longest-prefix routing keeps
	// the API/DAV/JMAP handlers above taking precedence).
	if dir := env("VMAIL_WEBMAIL_DIR", "./webmail"); dir != "" {
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

	// Metrics endpoint.
	if metricsAddr := env("VMAIL_METRICS_ADDR", ":2090"); metricsAddr != "" {
		go serve("metrics", metricsAddr, func() error {
			mux := http.NewServeMux()
			mux.Handle("/metrics", metrics.Handler())
			return http.ListenAndServe(metricsAddr, mux)
		})
	}

	log.Printf("vmail up: domain=%s mx=%s submit=%s imap=%s jmap=%s data=%s", domain, mxAddr, subAddr, imapddr, jmapddr, dataDir)
	select {} // block forever
}

func serve(name, addr string, fn func() error) {
	log.Printf("%s listening on %s", name, addr)
	if err := fn(); err != nil {
		log.Fatalf("%s: %v", name, err)
	}
}
