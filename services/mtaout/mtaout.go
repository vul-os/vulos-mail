// Package mtaout is the outbound deliverability subsystem — the part every mail
// service gets wrong and the answer to "serve many senders on few IPs"
// (DESIGN §6). It is pure, testable scheduling logic behind a Sender interface:
//
//   - Pool: a shared warm-IP pool, consistent-hashed per tenant, segregated by
//     traffic class (transactional vs bulk) so reputation stays coherent.
//   - Warmup: per-sending-domain daily volume ramp.
//   - Reputation: per-tenant bounce/complaint gating (the abuse-containment moat
//     that makes shared IPs safe).
//   - Scheduler: per-destination-domain concurrency cap + adaptive backoff on
//     temporary failures, with permanent failures bounced.
//
// The actual SMTP dialing lives behind Sender (see smtpsender.go); tests inject
// a fake so the scheduler is verified deterministically without a network.
package mtaout

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

// TrafficClass segregates IP pools so transactional and bulk reputations don't
// contaminate each other.
type TrafficClass int

const (
	Transactional TrafficClass = iota
	Bulk
)

// OutMessage is one queued outbound message (one destination domain).
type OutMessage struct {
	ID         string
	Tenant     string
	FromDomain string // DKIM-aligned sending domain; reputation + warmup key
	RcptDomain string // destination domain; throttle key
	From       string
	Rcpts      []string
	Raw        []byte
	Class      TrafficClass
}

// SendStatus is the outcome of a delivery attempt.
type SendStatus int

const (
	Delivered SendStatus = iota
	TempFail             // 4xx / transient: retry with backoff
	PermFail             // 5xx / permanent: bounce
)

// SendResult is what a Sender returns.
type SendResult struct {
	Status SendStatus
	Err    error
}

// Sender performs one delivery attempt from a chosen source IP.
type Sender interface {
	Send(ctx context.Context, msg OutMessage, sourceIP string) SendResult
}

// --- Pool ---

// Pool assigns a stable source IP per (tenant, class).
type Pool struct {
	byClass map[TrafficClass][]string
}

// NewPool builds a pool with separate transactional and bulk IP sets.
func NewPool(transactional, bulk []string) *Pool {
	return &Pool{byClass: map[TrafficClass][]string{
		Transactional: transactional,
		Bulk:          bulk,
	}}
}

// IPFor returns the source IP for a tenant in a class (consistent hash, so a
// tenant's reputation stays pinned to a stable subset of the pool).
func (p *Pool) IPFor(tenant string, class TrafficClass) string {
	ips := p.byClass[class]
	if len(ips) == 0 {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tenant))
	return ips[h.Sum32()%uint32(len(ips))]
}

// --- Warmup ---

// Warmup enforces a per-sending-domain daily volume ramp.
type Warmup struct {
	schedule  []int // schedule[d] = max sends on day d since first send
	firstSeen map[string]time.Time
	used      map[string]int // "domain|dayIndex" -> count
}

// NewWarmup builds a warmup with the given per-day cap curve (last value is the
// steady-state cap once warm).
func NewWarmup(schedule []int) *Warmup {
	return &Warmup{schedule: schedule, firstSeen: map[string]time.Time{}, used: map[string]int{}}
}

func (w *Warmup) dayIndex(domain string, now time.Time) int {
	fs, ok := w.firstSeen[domain]
	if !ok {
		return 0
	}
	return int(now.Sub(fs) / (24 * time.Hour))
}

func (w *Warmup) capFor(day int) int {
	if len(w.schedule) == 0 {
		return 1 << 30
	}
	if day >= len(w.schedule) {
		day = len(w.schedule) - 1
	}
	if day < 0 { // clock moved backward (NTP/VM migration) → never index < 0
		day = 0
	}
	return w.schedule[day]
}

// Allow reports whether another send for domain fits today's cap.
func (w *Warmup) Allow(domain string, now time.Time) bool {
	if _, ok := w.firstSeen[domain]; !ok {
		return w.capFor(0) > 0
	}
	d := w.dayIndex(domain, now)
	return w.used[wkey(domain, d)] < w.capFor(d)
}

// Record counts a dispatched send against the domain's daily budget.
func (w *Warmup) Record(domain string, now time.Time) {
	if _, ok := w.firstSeen[domain]; !ok {
		w.firstSeen[domain] = now
	}
	d := w.dayIndex(domain, now)
	w.used[wkey(domain, d)]++
}

func wkey(domain string, day int) string { return fmt.Sprintf("%s|%d", domain, day) }

// WarmupState is the serializable, recoverable state of a Warmup (the per-domain
// first-send time and daily counters). The ramp schedule is config and is not
// part of the snapshot.
type WarmupState struct {
	FirstSeen map[string]time.Time `json:"firstSeen"`
	Used      map[string]int       `json:"used"`
}

// Snapshot returns the warmup's recoverable state so it can be persisted and
// restored across restarts (otherwise a redeploy would reset every domain's ramp
// to day 0, hurting deliverability).
func (w *Warmup) Snapshot() WarmupState {
	fs := make(map[string]time.Time, len(w.firstSeen))
	for k, v := range w.firstSeen {
		fs[k] = v
	}
	used := make(map[string]int, len(w.used))
	for k, v := range w.used {
		used[k] = v
	}
	return WarmupState{FirstSeen: fs, Used: used}
}

// Restore loads a previously-snapshotted warmup state.
func (w *Warmup) Restore(st WarmupState) {
	if st.FirstSeen != nil {
		w.firstSeen = st.FirstSeen
	}
	if st.Used != nil {
		w.used = st.Used
	}
}

// --- Reputation ---

// Reputation tracks per-tenant outcomes and gates senders whose bounce/complaint
// rates exceed thresholds (containment so one bad tenant can't burn shared IPs).
type Reputation struct {
	minVolume        int
	maxBounceRate    float64
	maxComplaintRate float64
	stats            map[string]*repStat
}

type repStat struct{ delivered, bounced, complaints int }

// NewReputation builds a reputation gate.
func NewReputation(minVolume int, maxBounceRate, maxComplaintRate float64) *Reputation {
	return &Reputation{minVolume: minVolume, maxBounceRate: maxBounceRate, maxComplaintRate: maxComplaintRate, stats: map[string]*repStat{}}
}

func (r *Reputation) get(tenant string) *repStat {
	st := r.stats[tenant]
	if st == nil {
		st = &repStat{}
		r.stats[tenant] = st
	}
	return st
}

func (r *Reputation) RecordDelivered(tenant string) { r.get(tenant).delivered++ }
func (r *Reputation) RecordBounced(tenant string)   { r.get(tenant).bounced++ }

// RecordComplaint is driven by feedback loops (FBL).
func (r *Reputation) RecordComplaint(tenant string) { r.get(tenant).complaints++ }

// RepStatState is one tenant's recoverable reputation counters.
type RepStatState struct {
	Delivered  int `json:"delivered"`
	Bounced    int `json:"bounced"`
	Complaints int `json:"complaints"`
}

// Snapshot returns the reputation gate's recoverable per-tenant counters so they
// can be persisted and restored across restarts (otherwise a redeploy would let
// a previously-gated abusive tenant immediately resume on the shared IPs).
func (r *Reputation) Snapshot() map[string]RepStatState {
	out := make(map[string]RepStatState, len(r.stats))
	for t, st := range r.stats {
		out[t] = RepStatState{Delivered: st.delivered, Bounced: st.bounced, Complaints: st.complaints}
	}
	return out
}

// Restore loads previously-snapshotted reputation counters.
func (r *Reputation) Restore(state map[string]RepStatState) {
	for t, s := range state {
		r.stats[t] = &repStat{delivered: s.Delivered, bounced: s.Bounced, complaints: s.Complaints}
	}
}

// Throttled reports whether a tenant is currently gated.
func (r *Reputation) Throttled(tenant string) bool {
	st := r.stats[tenant]
	if st == nil {
		return false
	}
	total := st.delivered + st.bounced
	if total < r.minVolume {
		return false
	}
	bounceRate := float64(st.bounced) / float64(total)
	complaintRate := float64(st.complaints) / float64(total)
	return bounceRate > r.maxBounceRate || complaintRate > r.maxComplaintRate
}

// --- durable queue persistence ---

// QueuedItem is the persisted form of one queued outbound message together with
// its retry state. It is what a QueueStore writes and reloads so the outbound
// queue survives a crash/deploy.
type QueuedItem struct {
	Msg         OutMessage
	Attempts    int
	NextAttempt time.Time
}

// QueueStore is durable storage for the outbound queue. It is the seam that turns
// the otherwise in-memory scheduler crash-safe: a mail server must never lose
// acknowledged mail, so submission only returns 250 after Add has committed to
// stable storage.
//
// Implementations MUST guarantee the record is durable (fsync'd / committed)
// before Add returns. Update persists a changed retry state (after a TempFail
// defer); Remove deletes a completed (delivered or bounced) item; Load returns
// every persisted item for recovery at startup.
//
// A nil store keeps the queue in memory only (tests / ephemeral dev runs).
type QueueStore interface {
	Add(it QueuedItem) error
	Update(it QueuedItem) error
	Remove(id string) error
	Load() ([]QueuedItem, error)
}

// newQueueID mints a random id used to key a queued message in the store when the
// submitter did not supply one.
func newQueueID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a time-based id; collision risk is negligible and this only
		// triggers if the OS RNG is unavailable.
		return fmt.Sprintf("q-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// --- Scheduler ---

// Scheduler drains an outbound queue respecting per-destination caps, warmup, and
// reputation, retrying TempFail with backoff and bouncing PermFail.
type Scheduler struct {
	sender       Sender
	pool         *Pool
	warmup       *Warmup
	rep          *Reputation
	store        QueueStore
	maxPerDomain int // max dispatches to one destination domain per tick
	maxAttempts  int
	backoff      func(attempt int) time.Duration

	mu       sync.Mutex
	queue    []*queued
	onBounce func(msg OutMessage, reason string)
}

// SetOnBounce registers a callback invoked when a message is permanently failed
// (5xx) or exhausts its retries — used to generate a DSN back to the sender.
func (s *Scheduler) SetOnBounce(fn func(msg OutMessage, reason string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBounce = fn
}

type queued struct {
	msg         OutMessage
	attempts    int
	nextAttempt time.Time
}

// Config configures a Scheduler.
type Config struct {
	Sender     Sender
	Pool       *Pool
	Warmup     *Warmup
	Reputation *Reputation
	// Store, if set, makes the outbound queue durable: Enqueue persists before it
	// returns and the queue is rebuilt from it on startup via Recover. nil keeps
	// the queue in memory only.
	Store        QueueStore
	MaxPerDomain int
	MaxAttempts  int
	Backoff      func(attempt int) time.Duration
}

// NewScheduler builds a scheduler. Sensible defaults fill zero fields.
func NewScheduler(cfg Config) *Scheduler {
	if cfg.MaxPerDomain <= 0 {
		cfg.MaxPerDomain = 10
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Backoff == nil {
		cfg.Backoff = func(a int) time.Duration { return time.Duration(1<<uint(a)) * time.Minute }
	}
	return &Scheduler{
		sender: cfg.Sender, pool: cfg.Pool, warmup: cfg.Warmup, rep: cfg.Reputation,
		store:        cfg.Store,
		maxPerDomain: cfg.MaxPerDomain, maxAttempts: cfg.MaxAttempts, backoff: cfg.Backoff,
	}
}

// Recover rebuilds the in-memory queue from the durable store (call once at
// startup, before the Tick loop). It is a no-op when no store is configured.
// Recovered messages keep their attempt count and next-attempt time so retries
// and the eventual bounce/DSN survive a restart.
func (s *Scheduler) Recover() error {
	if s.store == nil {
		return nil
	}
	items, err := s.store.Load()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		s.queue = append(s.queue, &queued{msg: it.Msg, attempts: it.Attempts, nextAttempt: it.NextAttempt})
	}
	return nil
}

// Enqueue adds a message to the outbound queue. When a durable store is
// configured the message is persisted (and fsync'd) BEFORE Enqueue returns, so a
// crash immediately after acknowledgement cannot lose it; the error is returned
// to the caller so submission can defer (4xx) rather than falsely accept (250)
// mail it failed to persist. Without a store it is an in-memory append.
func (s *Scheduler) Enqueue(msg OutMessage) error {
	if msg.ID == "" {
		msg.ID = newQueueID()
	}
	if s.store != nil {
		if err := s.store.Add(QueuedItem{Msg: msg}); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.queue = append(s.queue, &queued{msg: msg})
	s.mu.Unlock()
	return nil
}

// removeDurable deletes a completed (delivered/bounced) item from the store. It
// is best-effort: a failed delete can at worst re-deliver on restart (a
// duplicate), never lose mail, so the scheduling pass is not aborted on error.
func (s *Scheduler) removeDurable(id string) {
	if s.store != nil {
		_ = s.store.Remove(id)
	}
}

// updateDurable persists a deferred item's new retry state. Best-effort for the
// same reason as removeDurable: losing an update only resets the backoff window.
func (s *Scheduler) updateDurable(q *queued) {
	if s.store != nil {
		_ = s.store.Update(QueuedItem{Msg: q.msg, Attempts: q.attempts, NextAttempt: q.nextAttempt})
	}
}

// Pending returns the current queue depth.
func (s *Scheduler) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// TickStats summarizes one scheduling pass.
type TickStats struct {
	Delivered, Deferred, Bounced, Throttled, WarmupHeld, RateHeld int
}

// Tick performs one scheduling pass at time now: dispatch every ready message
// that passes the gates, up to the per-domain cap, handling each result.
func (s *Scheduler) Tick(ctx context.Context, now time.Time) TickStats {
	s.mu.Lock()

	type bounceEv struct {
		msg    OutMessage
		reason string
	}
	var bounces []bounceEv
	perDomain := map[string]int{}
	var remaining []*queued
	var st TickStats

	for _, q := range s.queue {
		switch {
		case now.Before(q.nextAttempt):
			remaining = append(remaining, q)
			continue
		case s.rep != nil && s.rep.Throttled(q.msg.Tenant):
			st.Throttled++
			remaining = append(remaining, q)
			continue
		case s.warmup != nil && !s.warmup.Allow(q.msg.FromDomain, now):
			st.WarmupHeld++
			remaining = append(remaining, q)
			continue
		case perDomain[q.msg.RcptDomain] >= s.maxPerDomain:
			st.RateHeld++
			remaining = append(remaining, q)
			continue
		}

		perDomain[q.msg.RcptDomain]++
		if s.warmup != nil {
			s.warmup.Record(q.msg.FromDomain, now)
		}
		ip := ""
		if s.pool != nil {
			ip = s.pool.IPFor(q.msg.Tenant, q.msg.Class)
		}

		res := s.sender.Send(ctx, q.msg, ip)
		switch res.Status {
		case Delivered:
			st.Delivered++
			if s.rep != nil {
				s.rep.RecordDelivered(q.msg.Tenant)
			}
			s.removeDurable(q.msg.ID)
		case TempFail:
			q.attempts++
			if q.attempts >= s.maxAttempts {
				st.Bounced++
				bounces = append(bounces, bounceEv{q.msg, "too many delivery attempts"})
				if s.rep != nil {
					s.rep.RecordBounced(q.msg.Tenant)
				}
				s.removeDurable(q.msg.ID)
			} else {
				q.nextAttempt = now.Add(s.backoff(q.attempts))
				remaining = append(remaining, q)
				st.Deferred++
				// Persist the new retry state so a restart resumes the backoff and
				// the attempt count keeps climbing toward the eventual bounce/DSN.
				s.updateDurable(q)
			}
		case PermFail:
			st.Bounced++
			reason := "permanent delivery failure"
			if res.Err != nil {
				reason = res.Err.Error()
			}
			bounces = append(bounces, bounceEv{q.msg, reason})
			if s.rep != nil {
				s.rep.RecordBounced(q.msg.Tenant)
			}
			s.removeDurable(q.msg.ID)
		}
	}

	s.queue = remaining
	cb := s.onBounce
	s.mu.Unlock()

	for _, b := range bounces {
		if cb != nil {
			cb(b.msg, b.reason)
		}
	}
	return st
}
