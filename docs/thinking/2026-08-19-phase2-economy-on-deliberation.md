# Phase 2 — Economy-ON (the S7 keystone): evidence map, slicing, and the one decision

**Date:** 2026-08-19 · **Roadmap:** the ordered path Phase 2 (D-M1-PIVOT) ·
**Discipline:** pace-before-code + build-immutable #6/#7. The audit's one-liner —
"the S7 economy is built + adversarially tested + default-OFF with no enable path" —
is the premise; this doc verifies it precisely and slices the enablement.

## Evidence map (verified in code, not taken from the audit)

The economy is **more wired than "default-OFF" implies** — the earning and skim paths
already run on a live daemon:

| Mechanism | State on a running daemon today | Site |
|---|---|---|
| Credit ledger created + wired | **YES** — `credit.New(50_000, 500_000)` (starter grant), `nd.SetLedger` | `daemon.go:562, :1036` |
| Serve earns credits + auto-skims 1/8 to the object escrow | **YES, live** — the MsgStoreChunk/serve path calls `RecordServeToObject` | `node.go:1209`; skim `escrow.go:49` (1/8) |
| Escrow accumulates per object | **YES** — `FundEscrow`/`RecordServeToObject` fill it | `escrow.go` |
| Repair verified (correctness + Shacham–Waters + quorum) | **YES** — H7 built | `repairclaim.go` |
| **Bounty PAYS the verified repairer** | **NO — the off switch.** `if n.cfg.RepairBountyBase <= 0 { return }` and `RepairBountyBase` is set only in tests | `repairclaim.go:240`; assigned only in 3 `_test.go` |
| Publisher/operator prepays a reserve (`FundDurability`) | **built, zero non-test callers** — no flag/API | `node.go:679` |
| `credit.G` / `Horizon` / `CostPerRepair` instruments | **built, never surfaced** — computed only inside repairclaim for a local decision, never logged or exposed | `instruments.go`; snapshot at `repairclaim.go:153` |
| Demand banking (`EnableDemandBank`) | built, zero non-test callers | `demandrole.go:106` |

**So the precise gap:** escrows *fill* (popular data self-skims) but never *disburse*
(the bounty is a no-op at `RepairBountyBase=0`); the funded horizon and `g` are computed
but **invisible**; and there is no publisher endowment path. Enablement is a **flag + a
telemetry surface + a fund path**, not construction — the audit is right, with the nuance
that half the loop (earn+skim) is already turning.

## The firewall that must survive (Invariant A — the M0 tie)

Credits fund **durability only**; they **never confer standing**. Verified: the bounty
`PayBounty` moves escrow→repairer *balance* (`escrow.go:118`), and `Reputation`
(`credit.go:282`) reads **bonded bytes − slashes**, never balance. Every slice below must
keep that: no economy enablement may make a credit balance readable by the standing/quorum
path. The telemetry slice is safe by construction (read-only); the bounty slice must be
tested to confirm enabling payouts moves no standing (the failing-first guard).

## The one decision that gates the keystone (Slice 1)

**Is `RepairBountyBase` a protocol constant or an operator flag?** `SkimNum/SkimDen` are
protocol-fixed constants (`escrow.go:49`); `RepairBountyBase` is a zero-default `Config`
field. The bounty is settled into **each node's LOCAL ledger** (`settleRepairVerdict`,
repairclaim.go), and the ledger is firewalled from standing — so nodes disagreeing on the
base is an *economic inconsistency* (different credit amounts for the same repair), **not a
consensus/safety break**. But a coherent market wants it network-agreed. Two shapes:

- **(A) Protocol constant** (like Skim): one blessed value, enabled by a single
  `-economy` / `-repair-bounty` **on/off** flag that flips it to the constant. Coherent
  by construction; the operator chooses participation, not price. Matches how Skim is
  already fixed. **Leaning here.**
- **(B) Operator-tunable** `-repair-bounty-base N`: flexible, but invites cross-node
  disagreement on price and a "who sets the market rate" question with no coordinator.

This touches the **economic mechanism** (S7 / a published durability claim), so per
build-immutable #6 it is worth an owner/PE call **before** Slice 1 codes it — not a
build-alone decision. Flagged; Slice 2 (telemetry) needs no such decision and goes first.

## Slices (in build order)

1. **Slice 2 first — durability telemetry (decision-free, build now).** Surface the
   built-but-invisible instruments: add a `durability` block to `/api/status` (per cared
   root: reserve / funded / paid / repairs / horizon; plus the node's credit balance) and
   an `-log info` line on a bounty payout / skim. Pure observability (S5); the prerequisite
   for *watching* `g` once the economy is on. No economic-parameter decision.
2. **Slice 1 — the keystone enable flag** (after the A/B decision): turn on bounty payout.
   Failing-first: a verified repair on an enabled node pays the repairer's balance and
   moves **zero** standing (the Invariant-A guard); off (default) stays a no-op.
3. **Slice 3 — the fund path**: an authenticated `/api/fund` (or daemon flag) calling
   `FundDurability`, so a publisher endows a reserve before popularity self-funds it.
4. **Slice 4 — the cloudtest economy-ON churn drill**: fund → serve-skim → kill holders →
   verify bounties pay verified repairs → **`g` measured on the wire for the first time**
   (the Phase 2 exit gate; uses Slice 2's telemetry + the 1.3 RSS envelope).

## This session

Ship Slice 2 (telemetry) — decision-free, on-roadmap, and it makes the economy observable
before it is switched on. Surface the Slice-1 A/B decision to the owner. Slices 1/3/4 follow
the decision.
