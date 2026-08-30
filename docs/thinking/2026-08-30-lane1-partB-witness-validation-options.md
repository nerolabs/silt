# lane-1 Part B — the pure-core trustless witness-validation path: options + decision

Date: 2026-08-30
Author: Builder
Ships with: the B1 code in this same PR.

## What this increment is

Part A (merged, #656) built the v5 **witness read-set producer** `Chain.WitnessReadSetV5`:
for a v5 block it emits the exact committed-state keys the witnessable recompute reads,
proven complete and bounded by the execution-derived drift guard. Part A produces the
read-set; it does NOT consume witnesses or validate a block.

Part B is the **consumer**: a floor-box validation entry point that, given a v5 block plus
a set of witnesses for the read-set keys, trustlessly decides accept/reject — matching what
a full node would decide — while holding only the two committed roots, never the tree.

## B1 / B2 decomposition (the boundary)

- **B1 (this increment): the pure-core validation path + #535 policy, witnesses fed in
  tests.** A `core/chain` entry point that takes a v5 block, the parent committed StateRoot,
  and an already-in-hand witness bundle (pre-state proofs against the parent root, post-state
  proofs against the block's committed root), verifies them via the existing
  `IngestBlockWitnesses` (#634) + `Resolve` (#633) spine, recomputes the write-set by running
  the real `apply()` on a witness-seeded clone, and accepts iff every written leaf's
  post-value verifies against the block's committed StateRoot. Plus the ratified #535
  cold-auditor recovery-boundary policy. No live networking; witnesses are constructed
  directly in tests.
- **B2 (later): any-of-N witness delivery / networking / daemon mode.** Sourcing the witness
  bundle from ≥1-honest providers over the wire, the fetch fan-out, the slow-loris read
  deadline, the daemon wiring that runs the floor box against a live feed. B1 assumes the
  bundle is already in hand (the same assumption `IngestBlockWitnesses` documents); B2 is how
  it gets there.

The boundary is exactly the DoS-bound cert's own scope line: "this layer assumes the witness
bundle is already in hand (in-block carry or a completed fetch) and gates it" (witness_bound.go
scope). B1 = validate a bundle in hand. B2 = obtain the bundle.

## The constraint: B1 must be an ADDITIVE floor-box validation MODE

B1 adds a SEPARATE validation path. It does not modify `apply()`, `validateEra3Roots`,
`postApplyRoots`, any validity predicate, or any consensus invariant I1–I5. A full node's
acceptance path is byte-for-byte unchanged. The floor-box mode is a new function that a
witness-only client calls INSTEAD of holding the tree; a full node never calls it.

**Confirmed additive.** B1 reuses the real `apply()` and the real committed-leaf encoders
read-only on a throwaway clone (the same shape `postApplyRoots` already uses). It introduces
no new block field, no encoding change, no BlockVersion change, and no branch in the
full-node accept path. It changes how a *root-only client* gains confidence in the block; it
changes no rule. This matches the certified posture: "the witness path is a *different way to
check the same transition*, not a new rule" (C-7 §102; I1/I2/I4/I5 untouched, I3 read-only).

## The design problem: how does a root-only box verify `b.StateRoot`?

A full node accepts a v5 block iff `b.StateRoot == postApplyRoots(b).state`, where
`postApplyRoots` clones the FULL committed state, runs `apply(b)`, and re-hashes the ENTIRE
committed leaf set via `StateRootForVersion(5)` (`statehash.Root` — a full rebuild from all
leaves). A floor box does not hold the full state, so it cannot run that full rebuild.

### Options considered

**Option 1 — stateless SMT batch root-update from the parent root.** Reconstruct the parent
tree from the read-set witnesses (`ImportSparseMerkleTrie` at `R_parent`), replay the
write-set as `Update`/`Delete`, read the new root, compare to `b.StateRoot`.

- REJECTED. `pokt-network/smt@v1.0.0` exposes single-key `Prove`/`VerifyProof` only — no
  batch-diff, no range proof, no "apply-updates-against-witness" verifier
  (witness-floor-box-boundary-block-cost-RESEARCH §2.1). A single-key proof's `SideNodes` are
  sibling *hashes*, not retrievable nodes; the update traversal (`smt.nodes.Get(digest)`)
  cannot resolve intermediate nodes from hashes alone, so `ImportSparseMerkleTrie` + `Update`
  from proofs does not compose across keys. Building a stateless multi-key SMT-update
  primitive is a new, soundness-critical, uncertified mechanism. Out of scope; would be a
  research-gated build.

**Option 2 — trust the quorum-attested root (Option A of the boundary-cost research).** Accept
`b.StateRoot` on finality, witness-validate only the O(payload) validity predicates.

- REJECTED for B1. That was the posture for the v4 era where the TTL sweep made every block's
  read-set O(registry). Era-4 (v5) is exactly the transition redesign that fixes it: the
  `dueBucket` expiry index + the bounded read-set producer make the read-set O(payload) /
  O(RegCap). The whole point of lane-1 is to reclaim TRUSTLESS root validation for v5. Falling
  back to quorum-trust would discard the era-4 win.

**Option 3 (initially chosen, then BLOCKED) — seed a partial clone from the read-set, re-run
`apply()`, verify the write-set post-values against `b.StateRoot`.** This is the natural
additive recompute: verify the pre-state read-set against `R_parent`, seed a clone, run the
real `apply(b)`, diff to get the write-set, verify each write-set post-value against
`b.StateRoot`. Completeness would rest on the certified read-set identity (`apply` writes only
read-set keys), soundness on the two-root witness checks.

- **BLOCKED — the seeded-clone recompute is incompatible with the BOUNDED read-set, and the
  correct bounded recompute does not yet exist.** Two concrete, verified obstructions:

  1. **`apply()` iterates WHOLE committed maps the bounded read-set deliberately does not
     witness in full.** The TTL sweep ranges the entire `bondRegHeight` map (`chain.go:3272`);
     the maturity latch ranges the entire `validatorsSeen`/`bonded`; the boundary tallies
     range the whole frozen set. The era-4 win is that the read-set replaces the
     `bondRegHeight` scan with ONE `dueBucket[h]` leaf. So a clone seeded only from the bounded
     read-set has an INCOMPLETE `bondRegHeight`, and re-running `apply` on it would compute the
     WRONG write-set (it would miss expiries outside the witnessed slice, or wrongly conclude
     nothing expired). Re-running `apply` therefore requires the O(registry) state — which is
     exactly what the floor box does not hold. This is independently confirmed by the PE
     ruling: "The producer targets a DIFFERENT, bounded 'witnessable recompute' **that does not
     yet exist in the tree (Part B)**" (RULING-lane1-partA-readset-v5-producer-2026-08-30,
     premise 1).

  2. **Some read-set leaves are DIGESTS, not the typed data `apply` iterates.** `dueBucket[h]`
     is committed as an RFC-6962 MTH over the id SET (`statehash.go:224` `dueBucketMTH`), and
     the producer emits that MTH as the witnessed leaf (`readSetTTLCompleteness`,
     `readset_v5.go:479`). A floor box that holds the MTH cannot enumerate the expiring member
     ids the TTL sweep needs. So even for the occupied-bucket case, the seeded clone cannot
     reconstruct the typed `dueBucket` membership from the witness alone.

  The bounded witnessable recompute must therefore be a NEW computation — one that uses the
  `dueBucket[h]` accelerator instead of the `bondRegHeight` scan, reconstructs the expiring
  members from witnessed data, and provably yields the SAME post-state root as `apply` for
  every v5 block. Building that recompute correctly is soundness-critical (a divergence =
  accept-a-forgery) and it is not certified as an ALGORITHM (the amended cert certified the
  read-set IDENTITY, not a recompute procedure). It also couples back to the Part A read-set
  identity: to reconstruct `dueBucket`'s expiring members trustlessly, the recompute needs
  either the bucket membership witnessed as typed data (a read-set / witness-format change) or
  a per-member expiry proof scheme — an open identity question.

## Decision: STOP and report (research gate)

B1's core deliverable — a bounded witnessable recompute that accepts iff a full node would —
cannot be built additively AND soundly with the current SMT library + Part A outputs. The
recompute:

- is soundness-critical (it stands in for the consensus root check on a root-only client);
- does not exist and is explicitly named a Part B build the PE ruling did not certify an
  algorithm for;
- couples to the frozen v5 committed format and the read-set identity (the `dueBucket`
  membership-vs-digest question), which is a **research-gated consensus/published-claim
  surface** under `silt/.claude/CLAUDE.md` (consensus-rule + the C-7/#600 witness soundness).

Per the task's explicit STOP condition ("If a production consensus-rule change is unavoidable,
STOP and report") and the research gate, I am NOT guessing this recompute. I ship only the
parts of B1 that are sound, additive, and do NOT pre-empt the gated decision (below), and I
route the recompute-algorithm question to the Researcher/PE.

### What is soundly buildable now, and ships in this PR

- **The witness-verification spine is already built and certified** (`IngestBlockWitnesses`
  #634, `Resolve` #633). B1 does not need to rebuild it.
- **The #535 cold-auditor recovery-boundary policy** is a pure box-local config + outcome
  decision that does NOT depend on the recompute. It is ratified (decisions.md 2026-08-30,
  item 3) and buildable additively now. This PR ships it as `RecoveryDirective` +
  the `Indeterminate` outcome, wired so a floor-box caller sources the directive ONLY from
  local config and emits a loud `indeterminate-trustlessly` when absent at an ambiguous
  boundary. It is a standalone, testable policy unit that B2's recompute will consume.

### What is routed to the gate (NOT built here)

- The bounded witnessable recompute algorithm (the accept/reject core). Needs a certified
  procedure that provably matches `apply`'s v5 post-state root using only the bounded witnessed
  read-set — including how `dueBucket`'s expiring members are reconstructed trustlessly.

This keeps B1 additive (no `apply`/`validateEra3Roots`/I1–I5 change) and ships the ratified
#535 policy, while refusing to guess the soundness-critical recompute.

## The #535 cold-auditor recovery-boundary policy

Ratified (decisions.md 2026-08-30, item 3; RECONCILIATION-floorbox-livenessrecovery-boundary):
the recovery directive is sourced ONLY from the box's own local `-ws-checkpoint`-class config,
NEVER the proposer or the block.

- **Directive present for height h** ⇒ validate trustlessly against the recomputed witnessable
  set (the normal Option-3 path).
- **Directive ABSENT at an ambiguous recovery boundary** ⇒ emit a LOUD `indeterminate-trustlessly`
  outcome. Default (cold-auditor) = do NOT accept; never trust the proposer.
- **live-follower** (proceed on the full node's existing weak-subjectivity residual) is an
  OPT-IN flip of the same flag, never the default.

The "ambiguous recovery boundary" is the height where `effectiveEpochSet`'s recovery branch
keys on `cfg.LivenessRecoveryHeight` — a non-committed operator config the box cannot witness
(amended cert R2). B1 encodes: box-local directive drives it; absent ⇒ loud indeterminate;
the live-follower behavior is opt-in.

**GATE (not claimed closed):** the #535 residual's certifiable CLOSURE is gated on the #603
`bonded`/`epochSet` keystone probes. B1 does not mark it closed. A code comment notes the gate.

## What this PR ships (the sound, non-gated slice of B1)

- `core/chain/floorbox_v5.go` — the additive floor-box scaffolding that is buildable without
  the gated recompute:
  - `RecoveryDirective` — a box-LOCAL `-ws-checkpoint`-class config: the set of heights the
    box's own operator has a recovery directive for, plus the `LiveFollower` opt-in flag.
    Sourced only from local config; never from the proposer or the block.
  - `FloorBoxOutcome` — the outcome enum: `Accept`, `Reject`, `IndeterminateTrustlessly`.
  - `RecoveryBoundaryDecision` — the #535 policy unit: given a height, whether it is an
    ambiguous recovery boundary, and the box-local directive, returns whether the box may
    proceed to trustless validation, or must emit `IndeterminateTrustlessly` (default
    cold-auditor: do not accept), with the live-follower opt-in flip.
  - `WitnessValidateV5` — the additive entry point, wired to: (a) reject a non-v5 block, (b)
    apply the #535 recovery-boundary decision FIRST (loud indeterminate when the directive is
    absent at an ambiguous boundary), and (c) return `IndeterminateTrustlessly` with a clear
    `ErrRecomputeGated` reason on the trustless path, because the bounded witnessable recompute
    is research-gated and not yet built. It NEVER accepts a block on a guessed recompute — the
    safe default holds. When the recompute lands, its result slots into step (c).
- `core/chain/floorbox_v5_test.go` — tests for the shipped slice, each ablated:
  - #535 directive-absent at an ambiguous boundary ⇒ loud `IndeterminateTrustlessly`, NOT
    accept, NOT proposer-trust (ablate: flip to directive-present and confirm it no longer
    short-circuits to indeterminate on the #535 arm);
  - #535 directive-present ⇒ proceeds past the recovery gate (then hits `ErrRecomputeGated`);
  - live-follower opt-in flips the default (ablate: default vs opt-in differ);
  - a non-v5 block ⇒ `Reject` (the mode is v5-only);
  - the safety invariant: `WitnessValidateV5` NEVER returns `Accept` in this PR (the recompute
    is gated), proven by construction and by test — a green `Accept` here would be a guessed
    recompute, the banned move.

## What this PR does NOT ship (B2 / gated)

- **The bounded witnessable recompute (the accept core) — ROUTED TO THE GATE.** See the STOP
  decision above. No `Accept` verdict is produced until a certified recompute lands.
- No live networking, no any-of-N delivery, no daemon mode (B2).
- No stateless multi-key SMT-update primitive (Option 1 — out of scope, would be gated).
- No claim that #535 is closed (gated on #603).
- No change to `apply`, `validateEra3Roots`, any validity predicate, or I1–I5.
