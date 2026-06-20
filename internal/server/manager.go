// Package server assembles the pieces into a running multi-account mail system:
// it manages per-account runtimes (each its own durable log over a shared,
// deduplicated blob store) and exposes the callbacks the protocol adapters need
// — MX delivery, IMAP auth, submission auth. This is the wiring layer; cmd/vmail
// turns it into a process.
//
// Auth here is a placeholder in-memory credential map; OAuth2/TOTP/passkeys are a
// later wave. Account addressing is exact-match (one address = one account).
package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/compose"
	"github.com/vul-os/vmail/internal/dkim"
	"github.com/vul-os/vmail/internal/dsn"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/filter"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/mailsettings"
	"github.com/vul-os/vmail/internal/metrics"
	"github.com/vul-os/vmail/internal/mime"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/tenant"
	"github.com/vul-os/vmail/services/mtaout"
)

// Manager owns all accounts and the shared infrastructure.
type Manager struct {
	dir   string
	blobs blob.Store
	gen   *ids.Gen
	sched *mtaout.Scheduler

	// LogOpen opens an account's event log given its directory. Defaults to the
	// JSONL File log; set to a SQLite opener for the durable DB backend.
	LogOpen func(dir string) (eventlog.Log, error)
	// Signer holds outbound DKIM keys (shared with the submission backend).
	Signer *dkim.Signer
	// Inbound, if set, scans received mail to route inbox/junk/reject.
	Inbound *filter.Chain
	// Registry + Quota, if set, enforce per-tenant daily send limits.
	Registry *tenant.Registry
	Quota    *tenant.Quota
	// Settings + Vacation, if set, drive the vacation auto-responder on delivery.
	Settings *mailsettings.Store
	Vacation *mailsettings.Responder

	mu       sync.Mutex
	accounts map[string]*account.Runtime
	creds    map[string][]byte // address -> bcrypt password hash
}

// NewManager creates a manager rooted at dir, using blobs for bodies and sched
// for outbound (sched may be nil if sending is disabled).
func NewManager(dir string, blobs blob.Store, sched *mtaout.Scheduler) *Manager {
	return &Manager{
		dir: dir, blobs: blobs, gen: ids.NewGen(), sched: sched,
		LogOpen:  func(d string) (eventlog.Log, error) { return eventlog.OpenFile(filepath.Join(d, "log.jsonl"), nil) },
		Signer:   dkim.NewSigner(),
		accounts: map[string]*account.Runtime{},
		creds:    map[string][]byte{},
	}
}

// EnsureDKIM loads (or generates + persists) the outbound DKIM key for domain and
// returns the DNS TXT record to publish at <selector>._domainkey.<domain>. Keys
// persist to dataDir/dkim/<domain>.pem so the published record stays valid across
// restarts.
func (m *Manager) EnsureDKIM(domain, selector string) (string, error) {
	if m.Signer.Has(domain) {
		return "", nil
	}
	keyPath := filepath.Join(m.dir, "dkim", safeName(domain)+".pem")
	// Load a persisted key if present (keys MUST be stable: the published DNS TXT
	// is derived from them, so regenerating on each boot would break DKIM).
	if data, err := os.ReadFile(keyPath); err == nil {
		if key, perr := dkim.ParsePrivateKey(data); perr == nil {
			m.Signer.AddDomain(domain, selector, key)
			return dkim.PublicTXT(key)
		}
	}
	key, txt, err := dkim.GenerateRSAKey(2048)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err == nil {
		_ = os.WriteFile(keyPath, dkim.MarshalPrivateKey(key), 0o600)
	}
	m.Signer.AddDomain(domain, selector, key)
	return txt, nil
}

// AddAccount registers an address with a password (stored bcrypt-hashed).
func (m *Manager) AddAccount(address, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds[strings.ToLower(address)] = hash
	return nil
}

// account returns the runtime for address, opening (and caching) it on first use.
func (m *Manager) account(ctx context.Context, address string) (*account.Runtime, error) {
	address = strings.ToLower(address)
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.accounts[address]; ok {
		return rt, nil
	}
	if _, ok := m.creds[address]; !ok {
		return nil, errors.New("no such account")
	}
	acctDir := filepath.Join(m.dir, "accounts", safeName(address))
	if err := os.MkdirAll(acctDir, 0o700); err != nil {
		return nil, err
	}
	log, err := m.LogOpen(acctDir)
	if err != nil {
		return nil, err
	}
	rt, err := account.Open(ctx, log, m.blobs, m.gen, nil)
	if err != nil {
		return nil, err
	}
	m.accounts[address] = rt
	return rt, nil
}

// Deliver is the MX delivery callback: route an accepted message to a recipient's
// account inbox. Unknown recipients are rejected (the MX returns an error).
func (m *Manager) Deliver(ctx context.Context, rcpt string, raw []byte) error {
	rt, err := m.account(ctx, rcpt)
	if err != nil {
		return err
	}
	label := model.LabelInbox
	if m.Inbound != nil {
		switch v := m.Inbound.Scan(ctx, raw); v.Action {
		case filter.Reject:
			metrics.MessagesReceived.WithLabelValues("reject").Inc()
			return errors.New("message rejected: " + v.Reason)
		case filter.Junk:
			label = model.LabelSpam
		}
	}
	if _, err = rt.Ingest(ctx, raw, []model.LabelID{label}, nil); err != nil {
		return err
	}
	if label == model.LabelInbox {
		metrics.MessagesReceived.WithLabelValues("inbox").Inc()
		m.maybeVacation(rcpt, raw)
	} else {
		metrics.MessagesReceived.WithLabelValues("spam").Inc()
	}
	return nil
}

// maybeVacation sends an out-of-office auto-reply if the recipient has vacation
// enabled and the incoming message isn't automated/from a daemon, rate-limited
// per sender.
func (m *Manager) maybeVacation(account string, raw []byte) {
	if m.Settings == nil || m.Vacation == nil || m.sched == nil {
		return
	}
	st := m.Settings.Get(account)
	if !st.Vacation.Enabled {
		return
	}
	env, err := mime.ParseEnvelope(raw)
	if err != nil || len(env.From) == 0 {
		return
	}
	sender := env.From[0]
	if sender == "" || strings.EqualFold(sender, account) ||
		mailsettings.IsAutomated(raw) || mailsettings.IsDaemonAddress(sender) {
		return
	}
	if !m.Vacation.ShouldReply(account, sender) {
		return
	}
	reply := mailsettings.BuildReply(account, sender, st.Vacation.Subject, st.Vacation.Body)
	m.sched.Enqueue(mtaout.OutMessage{
		Tenant:     tenantOf(account),
		FromDomain: tenantOf(account),
		RcptDomain: tenantOf(sender),
		From:       account,
		Rcpts:      []string{sender},
		Raw:        reply,
		Class:      mtaout.Transactional,
	})
}

// AuthIMAP validates IMAP credentials and returns the account runtime.
func (m *Manager) AuthIMAP(username, password string) (*account.Runtime, error) {
	if !m.checkCred(username, password) {
		return nil, errors.New("invalid credentials")
	}
	return m.account(context.Background(), username)
}

// AuthSubmit validates submission credentials and returns the runtime + tenant.
func (m *Manager) AuthSubmit(username, password string) (*account.Runtime, string, error) {
	if !m.checkCred(username, password) {
		return nil, "", errors.New("invalid credentials")
	}
	rt, err := m.account(context.Background(), username)
	if err != nil {
		return nil, "", err
	}
	return rt, tenantOf(username), nil
}

// Enqueue hands an outbound message to the scheduler (used by the submission
// backend's Enqueue hook).
func (m *Manager) Enqueue(msg mtaout.OutMessage) {
	if m.sched != nil {
		m.sched.Enqueue(msg)
		metrics.SubmissionsAccepted.Inc()
	}
}

// GCBlobs deletes blobs not referenced by any account's live messages and older
// than grace (the grace window avoids racing a just-Put blob whose referencing
// event hasn't committed yet). No-op if the blob store isn't GC-capable.
func (m *Manager) GCBlobs(ctx context.Context, grace time.Duration) (int, error) {
	gc, ok := m.blobs.(blob.GCStore)
	if !ok {
		return 0, nil
	}
	m.mu.Lock()
	addrs := make([]string, 0, len(m.creds))
	for a := range m.creds {
		addrs = append(addrs, a)
	}
	m.mu.Unlock()

	live := map[model.BlobRef]bool{}
	for _, a := range addrs {
		rt, err := m.account(ctx, a)
		if err != nil {
			continue
		}
		for _, ref := range rt.LiveBlobRefs() {
			live[ref] = true
		}
	}

	infos, err := gc.ListBlobs(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-grace)
	n := 0
	for _, bi := range infos {
		if live[bi.Ref] || bi.ModTime.After(cutoff) {
			continue
		}
		if gc.Delete(ctx, bi.Ref) == nil {
			n++
		}
	}
	return n, nil
}

// HandleBounce generates a DSN for a permanently-failed message and delivers it
// to the original sender if the sender is local. Wire this to
// Scheduler.SetOnBounce.
func (m *Manager) HandleBounce(reportingDomain string, msg mtaout.OutMessage, reason string) {
	if msg.From == "" {
		return
	}
	bounce := dsn.Build(reportingDomain, msg.From, msg.Rcpts, reason)
	// Best-effort local delivery; if the sender isn't local, nothing to do.
	_ = m.Deliver(context.Background(), msg.From, bounce)
}

// GetSettings returns an account's settings (zero value if none/unset).
func (m *Manager) GetSettings(account string) mailsettings.Settings {
	if m.Settings == nil {
		return mailsettings.Settings{}
	}
	return m.Settings.Get(account)
}

// SetSettings stores an account's settings.
func (m *Manager) SetSettings(account string, s mailsettings.Settings) {
	if m.Settings != nil {
		m.Settings.Set(account, s)
	}
}

// WebSend sends a plain-text message (back-compat wrapper around WebSendMsg).
func (m *Manager) WebSend(ctx context.Context, account string, to []string, subject, text string) error {
	return m.WebSendMsg(ctx, account, to, nil, subject, text, "", nil)
}

// WebSendMsg composes a message (text + optional HTML + attachments) from the
// authenticated account, stores a Sent copy, DKIM-signs, and sends it — the
// webmail compose path.
func (m *Manager) WebSendMsg(ctx context.Context, account string, to, cc []string, subject, text, html string, atts []compose.Attachment) error {
	if len(to)+len(cc) == 0 {
		return errors.New("no recipients")
	}
	raw, err := compose.Build(compose.Message{
		From: account, To: to, Cc: cc, Subject: subject, Text: text, HTML: html,
		MessageID: m.gen.New() + "@" + tenantOf(account), Attachments: atts,
	})
	if err != nil {
		return err
	}
	if rt, err := m.account(ctx, account); err == nil {
		_, _ = rt.Ingest(ctx, raw, []model.LabelID{model.LabelSent}, []model.Flag{model.FlagSeen})
	}
	return m.SendRaw(ctx, account, append(append([]string{}, to...), cc...), raw)
}

// SendRaw accepts a fully-composed message for outbound delivery (used by the
// transactional webapi): DKIM-signs with the From domain's key and enqueues one
// message per destination domain. Quota is enforced when configured.
func (m *Manager) SendRaw(_ context.Context, from string, to []string, raw []byte) error {
	if err := m.CheckQuota(from, len(raw)); err != nil {
		return err
	}
	if m.Signer != nil {
		if signed, err := m.Signer.Sign(tenantOf(from), raw); err == nil {
			raw = signed
		}
	}
	byDomain := map[string][]string{}
	for _, r := range to {
		byDomain[tenantOf(r)] = append(byDomain[tenantOf(r)], r)
	}
	for d, rcpts := range byDomain {
		m.Enqueue(mtaout.OutMessage{
			Tenant: tenantOf(from), FromDomain: tenantOf(from), RcptDomain: d,
			From: from, Rcpts: rcpts, Raw: raw, Class: mtaout.Transactional,
		})
	}
	return nil
}

// CheckQuota enforces the sending account's tenant daily quota (no-op if unset).
func (m *Manager) CheckQuota(account string, bytes int) error {
	if m.Quota == nil {
		return nil
	}
	tenantID := account
	if m.Registry != nil {
		tenantID = m.Registry.TenantFor(account)
	}
	if ok, reason := m.Quota.Allow(tenantID, int64(bytes)); !ok {
		return errors.New(reason)
	}
	return nil
}

func (m *Manager) checkCred(username, password string) bool {
	m.mu.Lock()
	hash, ok := m.creds[strings.ToLower(username)]
	m.mu.Unlock()
	if !ok {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

func tenantOf(address string) string {
	if i := strings.LastIndex(address, "@"); i >= 0 {
		return address[i+1:]
	}
	return address
}

func safeName(address string) string {
	return strings.NewReplacer("@", "_at_", "/", "_", "\\", "_", "..", "_").Replace(address)
}
