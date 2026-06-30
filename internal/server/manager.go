// Package server assembles the pieces into a running multi-account mail system:
// it manages per-account runtimes (each its own durable log over a shared,
// deduplicated blob store) and exposes the callbacks the protocol adapters need
// — MX delivery, IMAP auth, submission auth. This is the wiring layer; cmd/vulos-mail
// turns it into a process.
//
// Auth here is a placeholder in-memory credential map; OAuth2/TOTP/passkeys are a
// later wave. Account addressing is exact-match (one address = one account).
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vul-os/vulos-mail/internal/abuse"
	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/blob"
	"github.com/vul-os/vulos-mail/internal/compose"
	"github.com/vul-os/vulos-mail/internal/dkim"
	"github.com/vul-os/vulos-mail/internal/dsn"
	"github.com/vul-os/vulos-mail/internal/eventlog"
	"github.com/vul-os/vulos-mail/internal/filter"
	"github.com/vul-os/vulos-mail/internal/ids"
	"github.com/vul-os/vulos-mail/internal/mailsettings"
	"github.com/vul-os/vulos-mail/internal/metrics"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/model"
	"github.com/vul-os/vulos-mail/internal/region"
	"github.com/vul-os/vulos-mail/internal/seam"
	"github.com/vul-os/vulos-mail/internal/tenant"
	"github.com/vul-os/vulos-mail/services/mtaout"
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
	// Settings is satisfied by either the in-memory *mailsettings.Store (default)
	// or the Postgres-backed *mailpg.PGSettings (cloud, when DATABASE_URL is set).
	Settings mailsettings.SettingsStore
	Vacation *mailsettings.Responder

	// Abuse, if set, gates every outbound send path (SMTP submission, JMAP, webapi)
	// with rate + recipient-burst checks. nil = no outbound abuse gating.
	Abuse *abuse.Filter

	// Identity, if set, replaces the built-in in-memory credential map as the
	// source of truth for authentication, account existence, and provisioning.
	// Standalone deployments set a file-backed local store; the optional
	// vulos-cloud adapter sets a cp-backed identity. nil = in-memory creds
	// (used by tests and ephemeral dev runs).
	Identity seam.Identity
	// Plans, if set, supplies per-account entitlements (tier/quota/suspension);
	// nil = unlimited self-hosted. Usage, if set, sinks metered events; nil =
	// no-op. Both are populated by the optional cloud adapter.
	Plans seam.Entitlements
	Usage seam.Usage
	// Regions resolves each mailbox to its home cell (region + internal endpoint).
	// nil = all mailboxes resolve to the EU default (Phase-0 single-cell).
	// Set to a configured *region.Resolver once multi-cell routing is wired.
	Regions *region.Resolver

	mu       sync.Mutex
	accounts map[string]*account.Runtime
	creds    map[string][]byte // address -> bcrypt password hash

	subMu  sync.Mutex
	subs   map[string]map[chan struct{}]bool // account -> live-update subscribers
	tokens map[string]pushTok                // opaque push token -> account (EventSource can't send Basic auth)

	sendMu sync.Mutex
	sends  map[string]*daySend // account -> today's send count (plan/tier enforcement)

	planMu    sync.Mutex
	planCache map[string]planEntry // account -> last-known entitlement (bounded fail-open)
}

type pushTok struct {
	account string
	expires time.Time
}

type daySend struct {
	day string
	n   int
}

type planEntry struct {
	plan seam.Plan
	at   time.Time
}

// planTTL bounds how long a cached entitlement is trusted after a cp error, so a
// brief control-plane outage preserves the last-known plan (incl. suspension)
// instead of silently failing fully open.
const planTTL = 90 * time.Second

// NewManager creates a manager rooted at dir, using blobs for bodies and sched
// for outbound (sched may be nil if sending is disabled).
func NewManager(dir string, blobs blob.Store, sched *mtaout.Scheduler) *Manager {
	return &Manager{
		dir: dir, blobs: blobs, gen: ids.NewGen(), sched: sched,
		LogOpen:   func(d string) (eventlog.Log, error) { return eventlog.OpenFile(filepath.Join(d, "log.jsonl"), nil) },
		Signer:    dkim.NewSigner(),
		accounts:  map[string]*account.Runtime{},
		creds:     map[string][]byte{},
		sends:     map[string]*daySend{},
		planCache: map[string]planEntry{},
		subs:      map[string]map[chan struct{}]bool{},
		tokens:    map[string]pushTok{},
	}
}

// --- live updates (SSE) ---

// notifyAccount wakes every live-update subscriber for an account (non-blocking).
func (m *Manager) notifyAccount(account string) {
	account = strings.ToLower(account)
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs[account] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Subscribe returns a channel that ticks on each change to the account, plus a
// cancel func to unsubscribe.
func (m *Manager) Subscribe(account string) (<-chan struct{}, func()) {
	account = strings.ToLower(account)
	ch := make(chan struct{}, 1)
	m.subMu.Lock()
	if m.subs[account] == nil {
		m.subs[account] = map[chan struct{}]bool{}
	}
	m.subs[account][ch] = true
	m.subMu.Unlock()
	return ch, func() {
		m.subMu.Lock()
		delete(m.subs[account], ch)
		m.subMu.Unlock()
	}
}

// PushToken mints an opaque short-lived token bound to an account (so an
// EventSource — which can't send Authorization headers — can authenticate).
func (m *Manager) PushToken(account string) string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "" // fail closed: no token rather than a low-entropy one
	}
	tok := hex.EncodeToString(b[:])
	m.subMu.Lock()
	now := time.Now()
	for k, t := range m.tokens { // prune expired so the map can't grow unbounded
		if now.After(t.expires) {
			delete(m.tokens, k)
		}
	}
	m.tokens[tok] = pushTok{account: strings.ToLower(account), expires: now.Add(24 * time.Hour)}
	m.subMu.Unlock()
	return tok
}

// AccountForToken resolves a push token to its account.
func (m *Manager) AccountForToken(tok string) (string, bool) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	t, ok := m.tokens[tok]
	if !ok || time.Now().After(t.expires) {
		delete(m.tokens, tok)
		return "", false
	}
	return t.account, true
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

// AddAccount registers an address with a password (stored bcrypt-hashed). When
// an Identity provider is configured it owns provisioning; otherwise the address
// goes into the in-memory credential map.
func (m *Manager) AddAccount(address, password string) error {
	if m.Identity != nil {
		return m.Identity.Provision(context.Background(), strings.ToLower(address), password)
	}
	hash, err := bcrypt.GenerateFromPassword(prehash(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds[strings.ToLower(address)] = hash
	return nil
}

// prehash maps a password to a fixed-length base64(sha256) digest before bcrypt,
// so passwords longer than bcrypt's 72-byte limit aren't silently truncated (a
// collision surface). base64 of a sha256 is 44 bytes and contains no NUL.
func prehash(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return []byte(base64.StdEncoding.EncodeToString(sum[:]))
}

// IsLocal reports whether rcpt is a provisioned local account (used by the MX to
// reject unknown recipients at RCPT time).
func (m *Manager) IsLocal(rcpt string) bool {
	if m.Identity != nil {
		return m.Identity.Exists(strings.ToLower(rcpt))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.creds[strings.ToLower(rcpt)]
	return ok
}

// account returns the runtime for address, opening (and caching) it on first use.
func (m *Manager) account(ctx context.Context, address string) (*account.Runtime, error) {
	address = strings.ToLower(address)
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.accounts[address]; ok {
		return rt, nil
	}
	if m.Identity != nil {
		if !m.Identity.Exists(address) {
			return nil, errors.New("no such account")
		}
	} else if _, ok := m.creds[address]; !ok {
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
	rt, err := account.Open(ctx, log, m.blobs, m.gen, func(eventlog.Record) { m.notifyAccount(address) })
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
	// Storage cap (when an entitlement source is configured): refuse delivery that
	// would push the mailbox past its plan's storage allowance. A temporary 452 so
	// the sender retries rather than hard-bounces.
	if plan, ok := m.planFor(rcpt); ok && plan.MaxBytes > 0 {
		if rt.MailboxBytes()+int64(len(raw)) > plan.MaxBytes {
			metrics.MessagesReceived.WithLabelValues("overquota").Inc()
			return errors.New("452 4.2.2 mailbox is over its storage quota")
		}
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
	// Meter stored bytes so the control plane can bill/track mailbox storage.
	if m.Usage != nil {
		m.Usage.Report(ctx, seam.Event{Kind: "storage", Account: rcpt, Bytes: int64(len(raw))})
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
	// Best-effort: a vacation auto-reply that can't be persisted is dropped (it is
	// not acknowledged mail), but record it so a persistently failing queue is
	// visible rather than silent.
	if err := m.sched.Enqueue(mtaout.OutMessage{
		Tenant:     tenantOf(account),
		FromDomain: tenantOf(account),
		RcptDomain: tenantOf(sender),
		From:       account,
		Rcpts:      []string{sender},
		Raw:        reply,
		Class:      mtaout.Transactional,
	}); err != nil {
		metrics.MessagesReceived.WithLabelValues("vacation_enqueue_error").Inc()
	}
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
// backend's Enqueue hook). It returns an error when the message could not be made
// durable, so the caller can defer (4xx) rather than falsely acknowledge (250)
// mail that would be lost on a crash.
func (m *Manager) Enqueue(msg mtaout.OutMessage) error {
	if m.sched == nil {
		return nil
	}
	if err := m.sched.Enqueue(msg); err != nil {
		return err
	}
	metrics.SubmissionsAccepted.Inc()
	return nil
}

// GCBlobs deletes blobs not referenced by any account's live messages and older
// than grace (the grace window avoids racing a just-Put blob whose referencing
// event hasn't committed yet). No-op if the blob store isn't GC-capable.
//
// The blob store is global + content-addressed (bodies dedup across accounts), so
// the live set MUST cover EVERY account on disk — not just the ones currently
// open or in the in-memory creds map — or a shared body could be wrongly swept.
// It is fail-closed: if any account's log can't be read, GC aborts rather than
// delete with an incomplete live set.
func (m *Manager) GCBlobs(ctx context.Context, grace time.Duration) (int, error) {
	gc, ok := m.blobs.(blob.GCStore)
	if !ok {
		return 0, nil
	}
	live := map[model.BlobRef]bool{}

	// (a) Currently-open accounts: read live refs from the live runtime (consistent
	// under its own lock), and remember their dirs so we don't re-read them.
	m.mu.Lock()
	openRts := make([]*account.Runtime, 0, len(m.accounts))
	openDirs := map[string]bool{}
	for a, rt := range m.accounts {
		openRts = append(openRts, rt)
		openDirs[safeName(a)] = true
	}
	m.mu.Unlock()
	for _, rt := range openRts {
		for _, ref := range rt.LiveBlobRefs() {
			live[ref] = true
		}
	}

	// (b) Every other account dir on disk (not open → static log, safe to read).
	entries, err := os.ReadDir(filepath.Join(m.dir, "accounts"))
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() || openDirs[e.Name()] {
			continue
		}
		lg, err := m.LogOpen(filepath.Join(m.dir, "accounts", e.Name()))
		if err != nil {
			return 0, err // fail-closed: never GC with an incomplete live set
		}
		rt, err := account.Open(ctx, lg, m.blobs, m.gen, nil)
		if err != nil {
			return 0, err
		}
		for _, ref := range rt.LiveBlobRefs() {
			live[ref] = true
		}
		_ = rt.Close() // throwaway: release the log handle (no fd/db leak)
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

// CompactAll snapshots + truncates the log of every open account (cheap if the
// log backend doesn't support snapshots). Returns the number compacted.
func (m *Manager) CompactAll(ctx context.Context) int {
	m.mu.Lock()
	rts := make([]*account.Runtime, 0, len(m.accounts))
	for _, rt := range m.accounts {
		rts = append(rts, rt)
	}
	m.mu.Unlock()
	n := 0
	for _, rt := range rts {
		if err := rt.Compact(ctx); err == nil {
			n++
		}
	}
	return n
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

// HomeRegion returns the region that owns mailbox's data.
//
// Phase-0: always region.EU unless m.Regions is set and carries an override.
// The method is the single chokepoint so future callers (IMAP capability
// advertisement, routing proxies) can branch on region without knowing the
// resolver internals.
func (m *Manager) HomeRegion(mailbox string) region.Region {
	return m.Regions.Resolve(mailbox).Region
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
	// Bind the visible From: header to the sending account. This is the shared
	// chokepoint for every programmatic send path (JMAP EmailSubmission, webapi,
	// webmail), so no caller can emit DKIM-aligned mail "From" another address.
	if env, perr := mime.ParseEnvelope(raw); perr != nil || len(env.From) == 0 || !addrEqual(env.From[0], from) {
		return errors.New("From header must match the sending account")
	}
	if err := m.CheckQuota(from, len(raw)); err != nil {
		return err
	}
	// Abuse gate: enforce outbound rate + recipient-burst limits across all send
	// paths (JMAP EmailSubmission, webapi, webmail) — mirrors the check the SMTP
	// submission backend runs on authenticated submissions.
	if m.Abuse != nil {
		if act, reason := m.Abuse.Check(from, len(to)); act != abuse.Allow {
			return fmt.Errorf("send rejected by outbound abuse filter: %s", reason)
		}
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
		// Durability: surface a persistence failure to the caller (JMAP/webapi)
		// so it returns an error rather than reporting a send that wasn't queued.
		if err := m.Enqueue(mtaout.OutMessage{
			Tenant: tenantOf(from), FromDomain: tenantOf(from), RcptDomain: d,
			From: from, Rcpts: rcpts, Raw: raw, Class: mtaout.Transactional,
		}); err != nil {
			return fmt.Errorf("could not queue message for %s: %w", d, err)
		}
	}
	if m.Usage != nil {
		// One send == one message (matches the daily send-cap gate in allowPlanSend,
		// which counts messages, not recipients).
		m.Usage.Report(context.Background(), seam.Event{Kind: "send", Account: from, Count: 1, Bytes: int64(len(raw))})
	}
	return nil
}

// CheckQuota enforces the sending account's tenant daily quota (no-op if unset).
func (m *Manager) CheckQuota(account string, bytes int) error {
	// Plan/tier enforcement (when an entitlement source is configured): honor
	// suspension and the per-account daily send cap. This is the shared chokepoint
	// for every send path (submission, JMAP, webapi, webmail).
	if err := m.allowPlanSend(account); err != nil {
		return err
	}
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
	// Login only authenticates — a billing lapse blocks sending (see
	// allowPlanSend), not reading, so a lapsed user can still reach their mail. A
	// hard (abuse/admin) suspension is enforced upstream by the identity provider
	// (cp's /api/mail/auth refuses the credential).
	if m.Identity != nil {
		_, err := m.Identity.Authenticate(context.Background(), strings.ToLower(username), password)
		return err == nil
	}
	m.mu.Lock()
	hash, ok := m.creds[strings.ToLower(username)]
	m.mu.Unlock()
	if !ok {
		// Compare against a dummy hash so a non-existent account costs the same as a
		// wrong password (no username-enumeration timing oracle).
		_ = bcrypt.CompareHashAndPassword(dummyCredHash, prehash(password))
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, prehash(password)) == nil
}

// dummyCredHash is a valid bcrypt hash used to equalise the timing of the
// no-such-account path with a real wrong-password comparison. Plaintext is
// irrelevant; it only needs to be a well-formed bcrypt hash.
var dummyCredHash, _ = bcrypt.GenerateFromPassword(prehash("vulos-dummy-password"), bcrypt.DefaultCost)

// allowPlanSend enforces the account's entitlement at send time: a suspended
// account cannot send, and an account with a daily send cap is held to it. With
// no entitlement source (standalone) this is a no-op. Entitlement-lookup errors
// fail OPEN — a cp blip must never wedge the send path.
func (m *Manager) allowPlanSend(account string) error {
	plan, ok := m.planFor(account)
	if !ok {
		return nil // no entitlement source (standalone) or cold cp error → allow
	}
	if plan.Suspended {
		return errors.New("account suspended")
	}
	if plan.MaxSendPerDay <= 0 {
		return nil // unlimited
	}
	today := time.Now().UTC().Format("2006-01-02")
	key := strings.ToLower(account)
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	ds := m.sends[key]
	if ds == nil || ds.day != today {
		ds = &daySend{day: today}
		m.sends[key] = ds
	}
	if ds.n >= plan.MaxSendPerDay {
		return errors.New("daily send limit reached")
	}
	ds.n++
	return nil
}

// planFor returns the account's entitlement and whether one is known. It caches
// the last-known plan; on a cp error it serves the cached value while still fresh
// (so suspension/caps survive a brief outage), and only when there's no fresh
// cache does it report "unknown" (ok=false → callers fail open for availability).
// nil Plans (standalone) always reports unknown.
func (m *Manager) planFor(account string) (seam.Plan, bool) {
	if m.Plans == nil {
		return seam.Plan{}, false
	}
	key := strings.ToLower(account)
	plan, err := m.Plans.For(context.Background(), key)
	if err == nil {
		m.planMu.Lock()
		m.planCache[key] = planEntry{plan: plan, at: time.Now()}
		m.planMu.Unlock()
		return plan, true
	}
	m.planMu.Lock()
	e, cached := m.planCache[key]
	m.planMu.Unlock()
	if cached && time.Since(e.at) < planTTL {
		return e.plan, true // bounded fail-open: trust last-known entitlement
	}
	return seam.Plan{}, false // cold: fail open for availability
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

// addrEqual compares two addresses for identity, tolerating display-name forms
// ("Name <a@b>") on either side.
func addrEqual(a, b string) bool {
	pa, pb := parseAddr(a), parseAddr(b)
	return pa != "" && strings.EqualFold(pa, pb)
}

func parseAddr(s string) string {
	if m, err := mail.ParseAddress(s); err == nil {
		return m.Address
	}
	return strings.TrimSpace(s)
}
