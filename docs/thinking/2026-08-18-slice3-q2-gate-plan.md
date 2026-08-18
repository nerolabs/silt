# Slice 3 — the Q2 pruned-tolerance gate: plan before code

**Date:** 2026-08-18 · **Author:** builder · **Status:** DELIBERATION (plan; no code yet —
pace-before-code on a #6-gated consensus-adjacent change). **Basis:** the H2 design doc
`2026-08-18-serve-retain-from-checkpoint-oom-fix.md` (§SLICE 3 spec) + the two PE rulings
(`rolling-horizon-oom`, `pruned-block-representation`) + the safetyDepth research certification.
Slices 1–2 are landed (commit `6f3d39f`), dormant. This is the consensus-critical core — the
one slice where an attacker's input touches the space-time verification skip, so it is the
merge gate.

## The mechanism paragraph (build-immutable #6)

**The failure this prevents is X because Y; this change addresses Y by Z.** *X:* once nodes
payload-selectively prune (drop `BondReg.Answer` below the horizon), a replay path that accepts
a pruned block will **skip** the space-time proof re-verify for it (there is no `Answer` to check).
*Y:* if a **peer** can get a pruned block accepted at a height the receiving node has **not**
finalized below, the peer forges consensus standing for free — it strips `Answer` to dodge
`verifyBond`, and the node counts the bond. That is a **C1 (no-discount) break** — the exact M0
corner slice-3 exists to protect. *Z:* gate the skip on the node's **own** trust anchor: a pruned
block is accepted (proof re-verify skipped) **only strictly below `trustFloor`**, where
`trustFloor` is the receiving node's own finalized/checkpoint anchor — never a value derived from
the fork under replay. At or above the floor, a pruned block is **rejected**
(`ErrPrunedAboveHorizon`); the node re-verifies the real `Answer` for everything it has not itself
finalized.

## Invariants touched (consensus-invariants.md I1–I5)

- **I1 (quorum intersection) — preserved, untouched.** Slice 3 changes nothing about how a quorum
  is sized or filled. Fork-choice weight (`Weight`) and the finality gate (`ErrPreFinalityReorg`)
  are unchanged; a pruned block carries the same header + consensus sigs, so it counts identically.
- **I2 (never sign twice, persisted) — untouched.** No signing path changes.
- **I3 (set changes at a finalized boundary; weight not head-count) — untouched.** The horizon is
  epoch-aligned precisely so it never splits a validator-set snapshot (#357 Cond A), but slice 3
  reads the horizon; it does not change set admission.
- **I4 (commit ≠ final; liveness) — untouched.** Pruning is anchored *below* finality; it can never
  strand a still-reorgable or still-gathering block. Under a >⅓ stall, finality (and thus the
  horizon) stalls → no prune → self-correcting.
- **I5 (deterministic fork-choice; accountable safety — honest never slashed) — PRESERVED, and this
  is the one to prove.** Payload-selective pruning **keeps the header + consensus sigs**, so
  late-reveal equivocation evidence survives (a pruned block still yields the culprit's signature
  over its hash). `FindEquivocations` is purely intra-height and needs no `Answer`. The oracle must
  assert: a pruned-payload block still produces a valid late-reveal slash, and no honest node is
  slashed by the prune.

**PR-body invariant statement (required by the binding rule):** "Slice 3 touches **I5** (preserves
attributable late-reveal slashing across a pruned payload) and reads **I3/I4**'s finalized-boundary
anchor; it changes no quorum sizing (I1), no signing ledger (I2), and no fork-choice function (I5
determinism). The C1/M0 protection is the Q2 gate: proof-skip only strictly below the node's own
anchor."

## The load-bearing finding (evidence, sharpens the design doc)

The design doc says "replay tolerance in **BOTH** paths — Reconcile AND Reload." The code says
otherwise, and it matters:

- **Reconcile (peer fork)** is the ONLY path that re-verifies bonds:
  `Reconcile` → `tmp.Append(fork[i])` (chain.go:2497) → `Append` → `ValidateCommit` (1761) →
  `ValidateProposal` (1633) → `validateBondRegs` (1660) → `validateBondReg` → `verifyBond(r.Answer)`
  (1146). A pruned block (`Answer == nil`) fails `verifyBond` here today. **This is the single gate
  site**, and it is exactly the attacker-facing path (peer-supplied fork). ✓ gate belongs here.
- **Reload (own disk)** does NOT re-verify bonds: `Reload` (2073) → `appendStructural` (2100) →
  `validateStructural` (2108). It checks proposer + attester sigs against `b.Hash()` and the
  empty-block/takedown/slash rules, then applies — it **never calls `validateBondRegs`** (the comment
  at chain.go:2228 is explicit: "height>0 went through validateBondRegs" *at commit time*; Reload
  trusts that, same as it skips the reputation gate). For a pruned block, `b.Hash()` returns the
  **stored** pre-prune hash (slice 2), and the proposer/attester sigs were made over that exact hash
  → they still verify. `len(b.BondRegs)` is still > 0 (Prune drops the `Answer` field, keeps the reg
  slice), so the empty-block check passes. **⇒ Reload of a pruned own-disk block already works via
  slice 2. No gate change in `validateStructural`.**

**Conclusion:** slice 3 is a **one-site** change (`validateBondRegs`), plus threading the floor into
`tmp`, plus a decode-invariant belt — NOT a two-path change. I will **prove** the Reload-no-change
claim with a test (a pruned own-disk block round-trips through Reload) rather than assume it — but I
will not add machinery to a path the evidence says needs none (#6: grep/verify before building).

## The sharp edge — `trustFloor` is the receiver's anchor, threaded into `tmp`

`validateBondRegs` runs inside the throwaway `tmp` replica during Reconcile. If the gate computed
the floor from `tmp`'s own replayed state, an attacker could inflate it (feed a fork with a high
finalized head) to get pruned blocks accepted at heights the **real** node never finalized — the
C1 break reappears one level down. So:

- `trustFloor() uint64 = max(c.cfg.WSCheckpoint.Height, c.RetentionHorizon())` — computed on the
  **receiving** chain `c`.
- Thread it into `tmp` as an injected field, mirroring `tmp.verifyBond = c.verifyBond` (chain.go:2493):
  `tmp.trustFloor = c.trustFloor()` (a snapshot value, or a closure returning `c`'s floor — snapshot
  is safer: the floor must not move mid-replay). The gate in `validateBondRegs` reads `c.prunedFloor`
  (the injected value), which for a normally-operating node equals its own `trustFloor()` and for
  `tmp` equals the **receiver's** floor, never the fork's.

## The Q2 gate (the three branches, in `validateBondRegs`)

In `validateBondRegs(b *Block)` (chain.go:1115), before the per-reg loop:

```
if b.IsPruned() {
    if b.Height >= c.prunedFloor {         // at/above the node's own anchor
        return ErrPrunedAboveHorizon       // C1 gate — RED without this
    }
    return nil                             // below the anchor: trust, SKIP verifyBond
}
// !IsPruned → fall through to the existing per-reg verify loop, any height (unchanged)
```

- **Decode-invariant belt** (reject a smuggled hybrid): a block with `Pruned` set AND any
  `BondReg.Answer != nil` is malformed — a full block cannot carry a forged stored-hash. Add to the
  block-decode / Reconcile entry (candidate: `DecodeBlocks`, or a cheap check in `validateBondRegs`
  itself: `if b.IsPruned() && anyAnswerPresent(b) { return ErrMalformedPruned }`). Keep it adjacent
  to the gate so the two rules read together.
- **Structural + proposer-sig checks still run** for a pruned block (they verify against
  `Hash()` == stored value, proven in slice 2) — only the space-time re-verify is skipped.

## Failing-first oracles (the merge gate — write RED first)

Model on the existing objective fixtures (`matureWorld` / `matureWorld12`) — a real chain with
finality + a live `verifyBond`, the only setting that exercises the floor.

1. **Q2 security oracle (the C1 catch) — `TestModelCheck_Q2_PrunedAboveFloorRejected`.** Build a
   fork containing a pruned block at height **≥ the receiver's `trustFloor`**; Reconcile must
   REJECT with `ErrPrunedAboveHorizon`. RED without the gate (today `verifyBond(nil)` returns false
   → `ErrBadBondReg`, the *wrong* reason and, worse, in a variant where the attacker also omits the
   reg from the verify set it would slip — the test pins the explicit gate, not the incidental
   nil-fail). Below the floor: the same pruned block is ACCEPTED and Prev-linkage-authenticated.
2. **Attacker cannot inflate the floor — `TestModelCheck_Q2_ForkFloorIgnored`.** A fork whose own
   (replayed) finalized head is high must NOT raise the acceptance floor: a pruned block above the
   **receiver's** real floor is still rejected even though it is below the fork's. Pins the
   thread-the-receiver's-floor design.
3. **Reload round-trip — `TestReloadPrunedBlockRoundTrips`.** A pruned own-disk block replays
   through `Reload`/`validateStructural` identically to its full form (sigs verify vs stored hash;
   no bond re-verify attempted). Proves the one-site finding — no `validateStructural` change.
4. **Late-reveal slash survives the prune (I5) — `TestPrunedBlockStillSlashable`.** A pruned block
   (header + sigs kept, Answer dropped) still yields a valid equivocation proof; the culprit is
   slashed and no honest node is. Guards the accountable-safety corner.
5. **Decode invariant — `TestPrunedWithAnswerRejected`.** A block with `Pruned` set and any
   `Answer` present is rejected as malformed.

All five must be RED against the current tree (or against a recorded revert of the gate) and GREEN
with slice 3, per the merge-gate discipline (consensus-correctness rule 4: every security property
has a deterministic RED/GREEN home).

## Explicitly NOT in slice 3 (defer)

- The actual prune of the node's OWN chain (drop `Answer` in `c.blocks` + durable store) — **slice
  4**. Slice 3 only makes replay *tolerate* a pruned block; nothing prunes yet (stays dormant).
- Serving the light chain + the (A) lagging-peer checkpoint-redirect — slice 4.
- #466 `EncodeBlocksUpTo` buffer belt — slice 5.

## One-line PE ack to request at PR time (not a re-consult)

The design doc flagged a minor PE note: the payload-selective refinement borders the PE's in-scope
mechanism call. Slice 3 adds one refinement the doc did not: **Reload needs no gate** (evidence:
`validateStructural` never re-verifies bonds; slice 2's stored-hash covers the sig). This *reduces*
surface vs. the doc — a one-line PE ack that "single-site gate in `validateBondRegs`, Reload
unchanged, is sound" suffices; it does not touch the consensus rule the ruling already blessed.

## POST-BUILD FINDING (2026-08-18) — the threading is load-bearing for LIVENESS, and the plan's threat framing was slightly off

Building it surfaced a refinement the plan got subtly wrong. The gate runs *during* the
`tmp.Append` replay, when `tmp`'s head reaches only the current block — so `tmp`'s
self-computed `trustFloor()` is the **incremental** horizon `≈ height−safetyDepth`, which
is always **below** the current block's height. Consequence:

- **Without threading, `tmp` OVER-rejects, it does not over-trust.** A pruned block at
  height H sees an incremental floor `< H`, so `H ≥ floor` → **rejected**. So the naive
  failure mode is not "attacker inflates the floor to sneak a block in" — it is the
  opposite: a node would refuse *legitimately* pruned history from an honest peer, breaking
  sync. (Ablation proof: removing the two threading lines turns
  `TestQ2_ReconcileHonorsPositiveReceiverFloor` part (a) RED — "floor 1" at h3 instead of
  the receiver's fixed 5.)
- **The threading's real job is liveness:** pin `tmp` to the receiver's OWN fixed floor so a
  pruned block genuinely below the receiver's finalized anchor is *accepted*. Its safety is
  backstopped by the **finality gate's hash-chaining**: `ErrPreFinalityReorg` forces the fork
  to contain the receiver's finalized head, so every block below the floor hash-equals the
  receiver's already-verified history — trusting a pruned one there re-trusts what the node
  itself finalized, not a peer's claim. So "never a peer's claim" (the PE ruling) is enforced
  by the finality gate + the fixed receiver floor together, not by the floor value alone.

This does not change the code (the threading is correct and necessary); it corrects the
*why*. The C1 protection still holds: a pruned block at/above the floor — i.e. NOT in the
finalized, hash-locked prefix — is rejected (part (b) of the positive-floor test).

## What shipped (slice 3, all GREEN + ablation-verified)

- Gate in `validateBondRegs` (chain.go): pruned & `Height ≥ trustFloor()` → `ErrPrunedAboveHorizon`;
  pruned & below → skip re-verify (belt: pruned+Answer → `ErrMalformedPruned`); full → unchanged.
- `trustFloor()` in retention.go = `max(WSCheckpoint.Height, RetentionHorizon())`, with a
  `trustFloorOverride` the Reconcile replay pins to the receiver's snapshot.
- Threading in `Reconcile`: `floor := c.trustFloor(); tmp.trustFloorOverride = &floor`.
- Oracles (`q2_gate_test.go`): above/at-floor rejected, below-floor accepted, fresh-node
  trusts nothing, malformed-pruned rejected, full-block-unaffected, Reconcile-gates-in-replay,
  positive-receiver-floor (threading, ablation-load-bearing), pruned-still-slashable (I5),
  Reload-round-trips (the one-site finding). Reload needs no gate — confirmed by test.
- `slashingWindow` etc. untouched. Nothing prunes yet (slice 4).

## Open questions to settle before writing the gate

1. **Snapshot vs. closure for `tmp.prunedFloor`.** Lean snapshot (value frozen at Reconcile entry)
   so the floor cannot move mid-replay. Confirm no path needs it live.
2. **Where the decode-invariant belt lives** — `DecodeBlocks` (catches on the wire, earliest) vs.
   inside `validateBondRegs` (adjacent to the gate, one place to read). Lean: both cheap; put the
   authoritative check in `validateBondRegs` and a fast reject in decode if it's a one-liner.
3. **Error taxonomy** — `ErrPrunedAboveHorizon` (new sentinel) + `ErrMalformedPruned` (new). Confirm
   naming against the existing `Err*` set.
