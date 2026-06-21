// Command vulos-mail runs the mail system: MX (receive), submission (send), and IMAP
// (serve) listeners over a shared account manager, plus the outbound scheduler
// loop. Configuration is via environment variables; account provisioning here is
// a single demo account (real provisioning/auth is a later wave).
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/acme/autocert"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	imapadapter "github.com/vul-os/vulos-mail/adapters/imap"
	jmapadapter "github.com/vul-os/vulos-mail/adapters/jmap"
	smtpin "github.com/vul-os/vulos-mail/adapters/smtp"
	"github.com/vul-os/vulos-mail/adapters/webapi"
	"github.com/vul-os/vulos-mail/caldav"
	"github.com/vul-os/vulos-mail/carddav"
	"github.com/vul-os/vulos-mail/internal/abuse"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/compose"
	"github.com/vul-os/vulos-mail/internal/emailauth"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/filter"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/mailsettings"
	"github.com/vul-os/vulos-mail/internal/metrics"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/model"
	"github.com/vul-os/vulos-mail/internal/scan"
	"github.com/vul-os/vulos-mail/internal/server"
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
	if acct != "" && pass != "" {
		mgr.AddAccount(acct, pass)
		log.Printf("provisioned account %s", acct)
	} else {
		log.Printf("no VULOS_ACCOUNT/VULOS_PASSWORD set; no accounts provisioned")
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

	// Listeners.
	authn := &emailauth.Authenticator{} // real DNS
	mx := smtpin.NewServer(&smtpin.Backend{
		Deliver:    mgr.Deliver,
		AuthServID: domain,
		KnownRcpt:  mgr.IsLocal, // reject unknown recipients at RCPT (550 5.1.1)
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

	jmapBe := &jmapadapter.Backend{
		Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) },
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
	cgen := ids.NewGen()
	apiKey := env("VULOS_API_KEY", "")
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
	// Attachment download from a received message: ?id=<msgID>&n=<index>.
	httpMux.HandleFunc("/api/webmail/attachment", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="vulos-mail"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		rt, err := mgr.AuthIMAP(u, p)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		msg, ok := rt.Message(model.ID(r.URL.Query().Get("id")))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		raw, err := rt.Body(r.Context(), msg.BlobRef)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		a, ok := mime.AttachmentAt(raw, n)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		ct := a.Type
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		name := a.Name
		if name == "" {
			name = "attachment"
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
		_, _ = w.Write(a.Data)
	})

	// Calendar (persistent, shared with the CalDAV server), Basic auth.
	httpMux.HandleFunc("/api/webmail/calendar", func(w http.ResponseWriter, r *http.Request) {
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
		switch r.Method {
		case http.MethodGet:
			out := []map[string]any{}
			for _, rsc := range calStore.List(u) {
				for _, ev := range caldav.ParseEvents(rsc.Data) {
					out = append(out, map[string]any{"id": rsc.Href, "summary": ev.Summary, "start": ev.Start, "end": ev.End})
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			var d struct {
				Summary string `json:"summary"`
				Start   string `json:"start"`
				End     string `json:"end"`
			}
			if err := json.NewDecoder(r.Body).Decode(&d); err != nil || d.Summary == "" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			start, _ := time.Parse(time.RFC3339, d.Start)
			if start.IsZero() {
				start = time.Now()
			}
			end, _ := time.Parse(time.RFC3339, d.End)
			uid := cgen.New()
			ics, err := caldav.BuildEvent(caldav.Event{UID: uid, Summary: d.Summary, Start: start, End: end})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			href := uid + ".ics"
			calStore.Put(u, href, ics)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"id":"` + href + `"}`))
		case http.MethodDelete:
			calStore.Delete(u, r.URL.Query().Get("id"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Contacts (persistent, shared with the CardDAV server), Basic auth.
	httpMux.HandleFunc("/api/webmail/contacts", func(w http.ResponseWriter, r *http.Request) {
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
		switch r.Method {
		case http.MethodGet:
			res, _ := contactStore.List(u)
			out := make([]map[string]any, 0, len(res))
			for _, rsc := range res {
				c := carddav.ParseContact(rsc.Data)
				out = append(out, map[string]any{"id": rsc.Href, "name": c.Name, "email": c.Email})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			var c carddav.Contact
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.Email == "" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			vcf, err := carddav.BuildContact(c)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			href := cgen.New() + ".vcf"
			if _, err := contactStore.Put(u, href, vcf); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"id":"` + href + `"}`))
		case http.MethodDelete:
			_ = contactStore.Delete(u, r.URL.Query().Get("id"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Webmail settings (signature + vacation), Basic auth.
	httpMux.HandleFunc("/api/webmail/settings", func(w http.ResponseWriter, r *http.Request) {
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
	// Live updates: mint a push token (Basic auth), then stream changes over SSE
	// (EventSource can't send Authorization headers, hence the token).
	httpMux.HandleFunc("/api/webmail/pushtoken", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || func() bool { _, e := mgr.AuthIMAP(u, p); return e != nil }() {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"` + mgr.PushToken(u) + `"}`))
	})
	httpMux.HandleFunc("/api/webmail/changes", func(w http.ResponseWriter, r *http.Request) {
		acct, ok := mgr.AccountForToken(r.URL.Query().Get("token"))
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		ch, cancel := mgr.Subscribe(acct)
		defer cancel()
		_, _ = w.Write([]byte("retry: 3000\n\n"))
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				_, _ = w.Write([]byte("data: change\n\n"))
				fl.Flush()
			case <-time.After(25 * time.Second):
				_, _ = w.Write([]byte(": ping\n\n"))
				fl.Flush()
			}
		}
	})

	// Webmail static UI at the root (registered last; longest-prefix routing keeps
	// the API/DAV/JMAP handlers above taking precedence).
	if dir := env("VULOS_WEBMAIL_DIR", "./webmail"); dir != "" {
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
