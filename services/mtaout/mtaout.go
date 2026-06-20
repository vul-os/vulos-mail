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

// --- Scheduler ---

// Scheduler drains an outbound queue respecting per-destination caps, warmup, and
// reputation, retrying TempFail with backoff and bouncing PermFail.
type Scheduler struct {
	sender      Sender
	pool        *Pool
	warmup      *Warmup
	rep         *Reputation
	maxPerDomain int // max dispatches to one destination domain per tick
	maxAttempts  int
	backoff     func(attempt int) time.Duration

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
	Sender       Sender
	Pool         *Pool
	Warmup       *Warmup
	Reputation   *Reputation
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
		maxPerDomain: cfg.MaxPerDomain, maxAttempts: cfg.MaxAttempts, backoff: cfg.Backoff,
	}
}

// Enqueue adds a message to the outbound queue.
func (s *Scheduler) Enqueue(msg OutMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, &queued{msg: msg})
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
		case TempFail:
			q.attempts++
			if q.attempts >= s.maxAttempts {
				st.Bounced++
				bounces = append(bounces, bounceEv{q.msg, "too many delivery attempts"})
				if s.rep != nil {
					s.rep.RecordBounced(q.msg.Tenant)
				}
			} else {
				q.nextAttempt = now.Add(s.backoff(q.attempts))
				remaining = append(remaining, q)
				st.Deferred++
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
