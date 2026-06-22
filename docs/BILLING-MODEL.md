# Vulos suite billing model — topology, costs, and the no-holes audit

This documents how billing works across **Vulos (cloud/OS)**, **Vulos Office**,
and **Vulos Mail**, the **Fly + Hetzner** deployment topology it must account for,
and the rules that keep every billable action leak-proof.

> Independence note: this is the **commercial Vulos deployment** model. vulos-mail
> and vulos-office each run fully standalone with no billing (see each repo's
> `SELFHOST.md`). Billing is an optional layer wired through the seam adapters.

## 1. Where billing lives

The **vulos-cloud control plane (`cp`)** is the single source of truth: Paystack
(charged in **ZAR**, multi-currency *display* only, FX cached in cp), tiers,
subscriptions, `metered_events`, super-admin, and the quota API. Products consume
**entitlements** from cp and emit **usage** back — they never run their own
payments. (Mail: `integration/cloud`; Office: `backend/integration/cloud`.)

## 2. Deployment topology (and why it splits)

| Component | Host | Why |
|---|---|---|
| `cp` (control plane, identity, billing, admin) | **Fly** | stateless API + managed boxes; Fly is fine here |
| Vulos Office | **Fly** (or anywhere) | no outbound :25 needed |
| **Vulos Mail** | **Hetzner** | Fly blocks outbound :25; mail needs warm IPs + rDNS |
| Mail outbound MTA pool | **Hetzner, fixed warm IPs** | reputation lives on stable IPs — never autoscaled |
| Blobs | Backblaze B2 / Cloudflare R2 / Linode OS | cheapest durable object storage |

cp↔mail is loosely coupled (HTTPS + shared secret), so the cross-provider split
costs nothing operationally.

## 3. Infrastructure cost model (must account for Fly **and** Hetzner)

Rough monthly figures (verify against live quotes); scale point ≈ 10k users
(70% free @1GB, 30% Pro @~10GB, ~5 outbound msgs/user/day ≈ 1.5M msg/mo):

| Line | Provider | ~Monthly |
|---|---|---|
| `cp` API + control plane | Fly | $25–80 |
| Office (stateless) | Fly | $20–60 |
| Mail frontend (JMAP/IMAP/webmail/MX) — fixed fleet + LB | Hetzner | $30–70 |
| **Mail outbound warm-IP pool (2–4 IPs)** | Hetzner | ~$20 |
| Object storage (~37 TB blobs) | B2/R2 | $185–220 |
| DNS / misc | Cloudflare | ~$0–20 |
| **Total** | — | **~$300–470/mo → ~$0.03–0.05 / user** |

Notes that matter for the model:
- **Outbound is self-hosted** (~$20 fixed), not a per-email relay — marginal send
  cost ≈ 0, so send volume is cheap to grow once IPs are warm.
- Marginal cost is **dominated by storage** → cap free-tier storage tightly.
- Mail load is steady; **autoscaling is optional** (fixed fleet is cheaper and
  simpler). Only the Fly tiers autoscale naturally.
- Fly egress + Hetzner traffic are generous/included at this scale; revisit at
  100k+ users (storage ~370 TB → ~$2k/mo, same per-user rate).

## 4. Tier model (cp-owned) — the unified table

`free` · `pro` · `team` · `enterprise` (subscription prices in cp: free $0 / pro $9
/ team $12 / enterprise $99 per month — **see the pricing caveat below**). Every
billable surface across **every product** now maps onto these four tiers (no
surface is off-model), exposed via cp `Entitlements.For(account, product)` and
enforced by each product:

| Surface | free | pro | team | enterprise |
|---|---|---|---|---|
| **Mail** send/day | 200 | 2,000 | 10,000 | 100,000 |
| Mail mailbox | 1 GiB | 25 GiB | 100 GiB | 500 GiB |
| Mail addresses | 1 | 5 | 25 | 500 |
| **Office** storage | 2 GiB | 50 GiB | 200 GiB | 1 TiB |
| Office seats | 1 | 3 | 25 | 500 |
| **Meet** recording min/mo | 60 | 600 | 3,000 | 9,000 |
| **LLM** budget/mo | $0.50 | $10 | $50 | $500 |
| **Compute** boxes / storage | 1 / 0.1 GB | 3 / 50 GB | 10 / 200 GB | 50 / 500 GB |
| **GPU** | none | PAYG | PAYG | PAYG |
| **Relay** GB | 5 | 25 | 30 | 60 |
| TURN sessions | — | 100 | 200 | 1,000 |
| Tier **storage** GB | 5 | 50 | 100 | 500 |
| Public circuits | — | 50 | 200 | 1,000 |

Every ladder is **monotonic** (regression-guarded by tests) and **suspension is
authoritative on all of them** — an admin hard-suspend or billing lapse collapses
the account to free across mail/office/meet/llm/compute/relay/gpu at once.

> **Pricing caveat (a business decision, not a code one):** the *resource ladders*
> above are coherent, but the *prices* ($0/$9/$12/$99) predate them and the
> $/resource curve is off — team ($12) grants far more than pro ($9) for +$3,
> while enterprise jumps to $99. Set prices that reflect the ladders (likely:
> raise team, and/or make team per-seat). The technical model doesn't care; the
> P&L does.

## 5. The no-holes audit

Every billable action must satisfy **three guarantees**:

> **Gated** — checked against a freshly-validated entitlement *before* the action ·
> **Metered** — recorded to `metered_events` on the same call ·
> **Bypass-proof** — server-side, pre-issuance, race-safe.

### Per-product status (post-audit fixes)

| Product | Billable surface | State |
|---|---|---|
| **cp** (control plane) | tier tables, entitlements, usage, dunning/suspension | reference impl; all surfaces on the 4-tier model; hard-suspend authoritative everywhere |
| **Mail** | send/day, mailbox bytes, addresses | ✅ gated (send cap + storage cap) + metered (send + storage) + bounded-cache fail-open; suspension blocks send not read |
| **Office** | storage, seats, office-access, recordings | ✅ gated + metered; unauthenticated-recordings hole closed; atomic check-and-reserve (TOCTOU closed); seats count real members |
| **llmux** | LLM token spend | ✅ every route metered (chat/embeddings/forward/streaming); race-safe budget reservations; retry-queued usage |
| **vulos OS** | compute, relay, GPU, meet, LLM | ✅ cp billing client gates + meters + honors suspension on every surface (was: none) |
| **Meet** | participant-minutes, rooms | 🔄 per-tier minutes on-model + gate-at-mint; minutes-metering → cp **in progress** |

### Cross-cutting holes (the subtle, suite-wide ones)

1. **Lapse/refund/chargeback must bite at *action* time, not just login** — short
   entitlement-cache TTL (+ optional cp→product invalidation). Mail already
   re-checks suspension on every auth via `Entitlements.For`.
2. **Identity-level caps** — limit free product instances per *identity*, not just
   per product (stop one account farming every product free).
3. **Metering integrity** — products self-report, so for billing-grade surfaces
   (storage) cp should **independently sample** the object store rather than trust
   the product node. Defense-in-depth.
4. **Race-safety** — quota check immediately before resource issuance, serialized
   (cp's SQLite single-writer + WAL).

### The rule that closes most holes

> Enforce at the point of resource issuance, against a freshly-validated
> entitlement, and meter the same call.

## 6. Anti-abuse ties into billing

Free-tier reputation is an asset because **outbound is self-hosted**. Signup is
gated by Altcha PoW; new free accounts start low-trust (small send caps, ramped
with age) via `abuse.Filter` + `tenant.Quota`; power users get headroom via
invites, verified identity, or upgrading — so abuse control is on *capability*,
not *existence*. See each repo's `SELFHOST.md` and the seam docs.
