// Command vmail runs the mail system: MX (receive), submission (send), and IMAP
// (serve) listeners over a shared account manager, plus the outbound scheduler
// loop. Configuration is via environment variables; account provisioning here is
// a single demo account (real provisioning/auth is a later wave).
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	imapadapter "github.com/vul-os/vmail/adapters/imap"
	jmapadapter "github.com/vul-os/vmail/adapters/jmap"
	smtpin "github.com/vul-os/vmail/adapters/smtp"
	"github.com/vul-os/vmail/internal/abuse"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/emailauth"
	"github.com/vul-os/vmail/internal/filter"
	"github.com/vul-os/vmail/internal/scan"
	"github.com/vul-os/vmail/internal/server"
	"github.com/vul-os/vmail/internal/tenant"
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

	blobs, err := blob.NewFS(dataDir + "/blobs")
	if err != nil {
		log.Fatalf("blob store: %v", err)
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
			return authn.Verify(context.Background(), raw, ip, helo, mailFrom).AuthResults()
		},
	}, mxAddr, domain)
	sub := smtpin.NewSubmitServer(&smtpin.SubmitBackend{Auth: mgr.AuthSubmit, Enqueue: mgr.Enqueue, Signer: mgr.Signer, Abuse: abuseFilter, Quota: mgr.CheckQuota}, subAddr, domain)
	imapBe := &imapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}
	imapSrv := imapadapter.NewServer(imapBe, nil)

	jmapBe := &jmapadapter.Backend{Auth: func(u, p string) (*account.Runtime, error) { return mgr.AuthIMAP(u, p) }}

	go serve("mx", mxAddr, mx.ListenAndServe)
	go serve("submission", subAddr, sub.ListenAndServe)
	go serve("imap", imapddr, func() error {
		ln, err := net.Listen("tcp", imapddr)
		if err != nil {
			return err
		}
		return imapSrv.Serve(ln)
	})
	go serve("jmap", jmapddr, func() error { return http.ListenAndServe(jmapddr, jmapBe.Handler()) })

	// Outbound scheduler loop.
	go func() {
		ctx := context.Background()
		for {
			sched.Tick(ctx, time.Now())
			time.Sleep(5 * time.Second)
		}
	}()

	log.Printf("vmail up: domain=%s mx=%s submit=%s imap=%s jmap=%s data=%s", domain, mxAddr, subAddr, imapddr, jmapddr, dataDir)
	select {} // block forever
}

func serve(name, addr string, fn func() error) {
	log.Printf("%s listening on %s", name, addr)
	if err := fn(); err != nil {
		log.Fatalf("%s: %v", name, err)
	}
}
