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

## 4. Tier model (cp-owned)

`free` · `pro` · `team` · `enterprise`. Each product maps tier → its own limits
via `Entitlements.For(account)`:

| Surface | free | pro | team/ent |
|---|---|---|---|
| Mail storage | ~1 GB | ~25 GB | higher |
| Mail send/day | low (warmup-ramped) | higher | custom |
| Mail addresses/aliases | 1 | many | many + domains |
| Office storage / seats | small / 1 | larger / few | larger / many |
| Cloud compute / GPU / relay GB | gated/0 | included | scaled |

## 5. The no-holes audit

Every billable action must satisfy **three guarantees**:

> **Gated** — checked against a freshly-validated entitlement *before* the action ·
> **Metered** — recorded to `metered_events` on the same call ·
> **Bypass-proof** — server-side, pre-issuance, race-safe.

### Per-product status

| Product | Billable surface | State | Hole to close |
|---|---|---|---|
| Cloud | compute, storage, relay GB, GPU, meet | gated + metered (audited) | real-time vs last-sample storage |
| **Office** | storage bytes, seats, office-access | **seam wired, not yet enforcing** | enforce storage/seat caps on upload/invite; gate office access by tier; emit usage |
| **Mail** | mailbox bytes, send/day, #addresses | local `tenant.Quota`+`abuse`; seam wired; honors cp suspension | map cp tier→`tenant.Quota`; meter bytes+sends; gate alias/address count |

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
