# C3 / R2.2 — the full economy observability set: the calls this build made

**Date:** 2026-09-08 · **Seat:** Builder · **Type:** BUILD deliberation (code ships in the
same PR) · **Against:** main `97e3101`

**Sources this build answers to**
- `docs/thinking/2026-09-01-economy-observability-design.md` — §0 the honesty rule, §2 the
  four panels, §3 the tier table and its two named gaps, §4 how each series is computed.
- `/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md`
  §2 (the 17-row build list) and §2.1 (the correction that the design doc's own
  network-wide repair-Gini gate is vacuous).
- `.claude/CLAUDE.md` simplicity rules 4 / 7 / 8 and `docs/TENETS.md` Part VI.

This is observability. It changes no validity rule, no fork-choice, no epoch, no slashing,
no I1-I5 invariant, no published M0/C1/C2 claim, no economic mechanism and no security
parameter a proof depends on. The one new ledger method is a READ that deliberately does
not register (below), and it is classified `neutral` under the Invariant-A guard.

---

## 1. The rows whose spec no longer matches main

**Row 3 (prepay/skim split) was already built** as Lane C4's A4-1 and is untouched here.

**Row 5 (network aggregate `g`) is UNBUILDABLE as specified, and it ships as an ABSENCE
with its reason rather than as a substitute.** `g` is the annualized trend of
cost-per-repair, and cost-per-repair is `Paid / Repairs` (`core/credit/instruments.go`).
Rows 8-9 gossip served bytes and repairs done. Repairs done is the DENOMINATOR; nothing on
the wire carries the numerator — not credits paid out of an escrow, not credits earned as a
repairer — and row 10 is explicit that a third gossip field must not be added. So the
network aggregate is not computable from the surface this track is allowed to build.

The choice was between publishing a number derived from an input nobody sent and publishing
the absence. Row 13's own rule names the answer: a field absent is a legitimate rendering, a
guess is not. `GET /api/economy/g` therefore carries `networkNotKnowable` with that
derivation on the wire, and never a `network` block. **This is the one row I stopped on.**
Closing it needs an owner call on a third gossip field, which row 10 currently forbids.

**Row 1's cadence provenance shifted.** The advisory ties the sample to a consensus epoch
(≈5.9 min measured). Coupling a dashboard ring to `EpochBlocks` would make a presentation
cadence read chain config, so the ring uses a flat 6-minute wall-clock interval — the
measured epoch rounded up. Sampling slightly slower than an epoch is the safe direction: the
quantities accumulate, so a coarser sample can under-report the number of distinct windows
and can never invent one.

**Nothing else moved.** C1's flat-delivery-path retirement does not reach these rows; the
`funded` split that row 3 introduced is what rows 1-2 difference.

## 2. The tier-class bands — a published number, so here is where it came from

Row 10 forbids a third gossip field and derives the class from the `CapTotal` peers already
gossip. The two edges are read off the Economist's published tier table
(`silt-agent-memory/economist/reviews/2026-09-01-tiered-edge-economy-sustainability-audit.md`, the
Pony/Horse/Archival table), not invented:

| band | edge | where the number comes from |
|---|---|---|
| pony | `CapTotal < 16 GiB` | the horse row's stated disk floor, "16+ GB disk" |
| horse | `16 GiB <= CapTotal < 1 TiB` | above the horse floor, below the archival's stated "TBs" |
| archival | `CapTotal >= 1 TiB` | the archival row's stated order of magnitude |

The binary units are the code's: `CapTotal` is counted in bytes, so 16 GiB and 1 TiB are the
nearest binary magnitudes to the table's prose figures, and both sit at or above them.

**What this band is not.** Not a security parameter, not a role, not a standing input. A node
declares its actual role with `-serve-content` / `-validator` / `-archive` and the classifier
never reads those. All three gossiped numbers are self-reported; a node can lie its way into
any band and gain a wrong dashboard. The bands ship ON THE WIRE (`/api/economy/network`
`bands[]`, each with its source string) so an operator checks the classification instead of
trusting a label, and a test pins the wire edges to the classifier's constants.

Why not a self-declared class field: it is a third self-reported number that buys nothing a
band over the second one does not already give, and it adds a surface an adversary can shape
independently of its pledge.

## 3. The two gossip fields, and their bound

`ports.Message.ServedBytes` and `ports.Message.RepairsDone`, CBOR slots 29/30, `omitempty`.
Exactly two, as row 10 requires.

- **They ride WITH the capacity pledge, never without it.** The receiver files them under the
  existing `msg.CapTotal > 0` condition. That is not incidental: the tier band that
  classifies the sample is derived from `CapTotal`, so a work figure arriving with no pledge
  could not be classified. The consequence is a property worth naming — **the work sample IS
  the capacity sample**, so every member of it is classifiable by construction.
- **The bound is the existing one.** They land in `peerCaps`, under `evictPeerInfoIfFull` and
  `maxPeerInfo = 4096`, so rows 8-9 open no new peer-keyed map: two int64s per existing
  entry, and `peerinfo_bound_test.go`'s property restated on the new fields.
- **Additive, not a format break.** DHT gossip is not a frozen consensus format; the decoder
  skips keys it does not know, and a gate encodes a frame with the new keys and decodes it
  with a struct that predates them.

> **SUPERSEDED 2026-09-09 (the gossip half).** The Researcher ruled this GATED: the two
> fields are gossiped only under `-privacy=off`, non-reporting reads as unknown rather than
> zero, and the owner must ratify before it merges. See
> `docs/thinking/2026-09-09-c3-review-foldin.md` §3.

**Privacy, stated rather than buried.** Both are node-wide totals with no object axis and no
fetcher axis, so neither carries the `(fetcher x object)` access record Don't #3 forbids.
They are the same class of self-reported advisory figure as the capacity pledge already
beside them. They are NOT gated by `-privacy`: that flag governs what an unauthenticated HTTP
reader gets, and gating the gossip would make every network panel vacuous on the compiled
default. The residual an operator should know: a peer learns this node's lifetime served
bytes, which on a node caretaking one root is that root's serve volume — a SERVER-side
volume, not an access record. Flagged here for review rather than resolved by me.

## 4. The vacuous-gate correction (advisory §2.1), measured not asserted

The design doc's §6 asks for a network-wide repair-Gini threshold. Under D-TIERING's
coupling (b) transient ponies serve and relay but do no durability work, so most of the
network sits at structural zero and the network-wide Gini is near 1 on a *healthy* network.
`TestR22RepairGiniIsScopedToTheCapableSubset` measures both numbers on the vision shape
rather than quoting the advisory's:

```
healthy vision shape (60 ponies at 0 repairs, 6 horses repairing evenly):
  repairGini(repair-capable subset, n=6)  = 0.0000
  repairGini(whole sample,          n=66) = 0.9091
```

A threshold that passes the first fails the second, on the same healthy network. So
`EconomySample.RepairGini` is computed **within the repair-capable subset only**, the wire
carries that scope in the field's own `scope` string, and the test also drives total capture
(all repair on one capable node → 0.8571) so the scoped series is shown to have teeth.
Serve-work stays over the whole sample: every tier serves, and the vision is that the edge
does the majority of it.

I did NOT build the Tester's three assertions. They are the Tester's, RED-first, after this.

## 5. What each panel renders when it has nothing

> **AMENDED 2026-09-09.** Two renderings were added by the review round: the two work Ginis
> render "withheld by this node's privacy setting" on the shipped `-privacy` default, and a
> Gini whose sample reported no work renders "no work reported" rather than `0.0000`. See the
> fold-in doc §2 and §5.


The honesty rule (§0) allows exactly three renderings of a number we do not have: **absent
with a named reason**, **sample too small**, **not yet measurable**. A zero is none of them.

| surface | the empty state | why not a zero |
|---|---|---|
| Panel 1, horizon | "not yet measurable", and NOT green | `credit.Horizon` returns `finite=false` when it has observed no burn. An unmeasured burn is not a proven-safe one, and it is certainly not "perpetual" — silt funds a finite renewable horizon (D-S7) |
| Panel 2, margin | "cost not supplied", revenue only | the operating cost is off-ledger and private. It is a query parameter, never a persisted flag, and the node never stores it |
| Panel 3, flows | "not yet measured", pooled row ABSENT | a delta needs two ring samples. "net 0" on a draining node is the silent-loss shape Don't #4 forbids |
| Panel 4, wash | "no wash shape", `authenticityKnowable: false` | the shape is local-exact; authenticity is not-knowable (Douceur). The word is "suspected" and the panel never claims a finding |
| `g` per object | `known: false` with a `reason` | `credit.G` returns 0 for BOTH "cost did not move" and "cannot be computed". The second must read as unknown, never as flat |
| `g` network | absent, with `networkNotKnowable` | §1 above |
| Ginis / tier mix | "sample too small", no value at all | §6 below |
| C2 | absent with `c2Absent` | a zeroed C2 block reads as a decentralised network |
| a tier class with no members | the row is ABSENT | "none in my sample" is not "none exist" |
| the observed tier ratio with no archival node | absent with its reason | a ratio with a zero denominator is unknown, not infinite |

## 6. The gossip sample floor — `minGossipSample = 3`

Row 13 says a gossip-estimated field never renders without its sample size. That leaves the
question of how small a sample may still publish, and the answer is derived, not chosen:

- at n = 1 a Gini is 0 by construction — the "number" carries no information and reads as
  perfect equality;
- at n = 2 a Gini **inverts**: `G = |a-b| / (2(a+b))`, so publishing it beside the sample size
  republishes the ratio of two named peers' work counters. An aggregate that resolves to one
  other node's value is not an aggregate;
- 3 is the smallest sample where neither holds.

So the floor is a privacy floor and an honesty floor at once, and it is what makes
`/api/economy/concentration` and `/api/economy/network` safe to leave open on the
unauthenticated wire (§7). The repair series carries the floor against its OWN subset size,
not the sample's — a 20-node sample routinely holds a 2-node capable subset.

## 7. The whole-surface route constant: 7 → 11, and why each route is safe

> **SUPERSEDED 2026-09-09 — the argument below for leaving `/api/economy/concentration` and
> `/api/economy/network` OPEN is FALSE and was refuted by measurement, twice and
> independently.** A published Gini plus its sample size is one equation, and
> `minGossipSample` bounds the sample SIZE, not the number of terms the reader does not
> already know; an adversary with free identities supplies n−1 of them and solves for the
> last. It is kept here unedited because the fold-in doc reasons about the error itself. The
> correction, the shape chosen and why, and the gate that pins it:
> `docs/thinking/2026-09-09-c3-review-foldin.md`.


Adding a GET route reddens BOTH whole-surface scans, which is the gate working. Each new
route was examined against each scan's property before the count moved; the examination is
written next to the constant in `cmd/silt/r29a_status_surface_test.go`.

- **`/api/economy/flows`, `/api/economy/g` — TOKEN-GATED IN FULL.** Every figure on either
  is an escrow delta of a named cared root: `objects[].skimIn` is the delta of exactly the
  counter the F2 scan protects, and `costPerRepair` is one root's `Paid/Repairs`. The
  **pooled row goes with the array**, not open beside it: on a node caretaking one object the
  pooled window delta IS that object's delta. That is the mistake `selfFunding` shipped with,
  and it is the ablation that matters (D2 below).
- **`/api/economy/concentration`, `/api/economy/network` — OPEN.** Neither carries a root, a
  per-object figure or a per-peer figure. The Ginis and the mix are aggregates over at least
  `minGossipSample` nodes and below that publish nothing; C2 is committed-global (every node
  holds that chain); the crowd estimate is the number `/api/status` already publishes in
  `network`. Self's own served bytes are inside the serve-Gini sample, so it is the FLOOR,
  not the aggregation, that makes these safe unauthenticated.

Both existing scans are pinned at ONE instant, so the ring holds one sample and the two
token-gated routes answer `windowNotYetMeasured` there whatever the token — a clean walk over
them in those fixtures is vacuous. `TestR22FlowsAndGAreTokenGatedAcrossAMeasuredWindow`
drives a real four-sample window first, proves the figures are on the tokened wire, and only
then walks the untokened surface.

## 8. The one new ledger method, and the defect it exists to prevent

`Node.send` stamps every outbound message. The obvious readers — `ServedBytes`,
`RepairsDone` — go through `acct()`, which calls `Register`, which **creates an account**: on
an unconfigured ledger it hands out the starter grant, and on an R2.12 faucet-configured one
it sets `grantPending` and increments `grantsPending`. Reading a counter to fill in a
FindNode's gossip fields would therefore mint an account and move faucet accounting as a side
effect of sending a packet.

`credit.Ledger.WorkSample` is the non-registering read that closes it, returning `ok=false`
for a node with no account so "has done no work" and "is unknown to my ledger" stay different
facts. Classified `neutral` under the Invariant-A guard. The optional interface is resolved
ONCE in `SetLedger` rather than asserted per message. Measured under controlled revert: the
registering version grows the census to 1 and `grantsPending` to 1 on a pure read.

## 9. Controlled reverts run (each RED before this shipped)

| # | revert | reddens |
|---|---|---|
| A1 | `WorkSample` uses `acct()` | census 1, `grantsPending` 1 on a READ |
| B1 | `handle` drops the two work fields from `capInfo` | sample carries 0/0; scoped repair Gini becomes decoration |
| B2 | `repairCapable` returns true for the pony | repair sample 66 instead of 6 |
| B3 | `SetLedger` does not resolve the work reader | self gossips zeros |
| C1 | `toWire` drops the two fields | round-trip loses them (they decode as a LEGAL 0) |
| C2 | slot 29 loses `omitempty` | a work-free sender still emits key 29 |
| D1 | `withheldEconomyFlows` returns the full document | the pooled delta on the untokened wire |
| **D2** | **pooled left OPEN, only the array withheld** | **still RED — the exact `selfFunding` defect class** |
| D3 | `withheldEconomyG` returns the full document | `costPerRepair` on the untokened wire |
| E1 | the sample-floor guard removed | a 0-node Gini published |
| F1 | "not yet measurable" → "perpetual" | Panel 1 |
| F2 | the gossip cell renders below the floor | row 13 |
| F3 | no-window renders a 0 net | Panel 3 |

**One vacuous gate caught in the act.** The first version of the `omitempty` assertion
compared encoded frame LENGTHS; under revert C2 a non-`omitempty` key grows both frames
equally, the inequality still held, and the gate stayed green. It was replaced with an exact
scan for the presence of keys 29/30 in the raw CBOR map, which reddens.

## 10. Simplicity: what I did not build

- No new `R-*` name (rule 4).
- No new timer and no goroutine: the ring is appended from the status snapshot that already
  recomputes on its own cadence, so `/api/economy/flows`, `/api/economy/g` and `/api/status`
  all difference ONE reading of the ledger.
- No new estimator: `EconomySample` calls `EstimateNetwork` for the crowd count so the two
  surfaces cannot drift onto different numbers.
- No third gossip field, no self-declared tier label, no `(fetcher x object)` join.
- No A5 cold-vs-exhausted refusal split and nothing for A6 — C7's scope.
- No Tester assertions — the Tester encodes those RED-first, after this.
