# era-3 step 2c — height-gated activation + mint-flip to v4

**Date:** 2026-08-29
**Seat:** Builder
**Step:** 2c (FINAL) of the certified era-3 committed-state-root sequence.
**Certification:** `silt-reviews/research/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md` (Q5 activation, Q7 mint-4).
**Base:** main `5951a76` (2a schema + Hash + versionSupported<=4; 2b `validateEra3Roots` on every disk-write path).

## What 2c must do (the requirements, restated)

1. Derive an activation height `H_era3` from committed history: activate when a
   frozen-epoch-weight supermajority signals `regVersion >= 4` (= `BlockVersionStateRoot`).
2. Mint-flip: at/above `H_era3` the propose path mints **v4** with populated roots; below,
   mint v2 unchanged.
3. Validity at the boundary: at/above `H_era3` a block MUST be v4 with valid roots; a v2
   block at height >= `H_era3` is REJECTED. Below, v2 validation unchanged. era-1/era-2
   history stays valid (no re-interpretation).
4. Boundary reorg-stability: `H_era3` keys on finalized epoch boundaries so a reorg cannot
   move it to un-enforce it (the #357 Condition-A argument).
5. Honor the 2b write-path guard: no mint/append path writes a v4 block without the predicate.

## The certified shape — reuse #506, do not invent

Q5 is explicit: the #506 activation SHAPE transfers (height-gated, epoch-final boundary,
monotonic lock-in, `regVersion >= threshold` supermajority over frozen weight). The ONLY
thing that does not transfer from #506 is its "no mint bump / soft-fork acceptance" clause —
era-3 IS a hard fork, so it mints a new version (v4) that a pre-era-3 binary rejects at
decode (Q7). The activation MECHANISM is `rotateEpoch`'s lock-in, mirrored one level up.

So 2c is: add a second lock-in tally in `rotateEpoch` keyed on `regVersion >= 4` (era-3's
readiness level, distinct from #506's `>= 3`), producing `era3LockedIn`/`era3Height`; add an
`era3Active(h)` predicate mirroring `regGateActive(h)`; wire the mint-flip and the boundary
validity rule to it.

## Options considered

### Option A — reuse the #506 lock-in machinery verbatim, new fields (CHOSEN)

Add `era3LockedIn bool` / `era3Height uint64` to `Chain`, tallied in the SAME `rotateEpoch`
block that tallies the #506 gate, but against a `regVersion >= BlockVersionStateRoot` (== 4)
threshold. Add `era3Active(h) = era3LockedIn && h >= era3Height` (plus a genesis override
`Era3ActivationHeight`, mirroring `RegGateActivationHeight`, for trusted-launch coordination
and deterministic model-check schedules). Mint-flip in `proposeBlock` (line 844): if
`era3Active(nextHeight)`, stamp v4 and populate roots via the existing 2a helper; else stamp
v2. Boundary validity in `ValidateProposal`: if `era3Active(b.Height)` and `b.Version < v4`,
reject.

- **Cost:** two new scalar fields (must be classified + folded into the state root + copied in
  the dry-run clone + carried in adopt/snapshot). One new config knob.
- **Benefit:** it IS the certified shape. Every soundness argument (monotonic, epoch-final,
  replay-identical, reorg-stable) transfers by construction from the #506 certification
  (Q5 "the boundary argument TRANSFERS"). Byzantine signal-inflation is absorbed by the shared
  >⅔ threshold, identical to #506. No new research surface.

### Option B — key era-3 activation off the #506 gate (`gateLockedIn`) directly

Reuse `gateHeight` as `H_era3` — activate era-3 when the #506 gate activates.

- **Rejected.** The two readinesses are DIFFERENT software states (cert Q7): a node signalling
  `regVersion >= 3` knows the R-rule but does NOT necessarily have the SMT root computation and
  the era-3 predicate. Gating era-3 on the #506 signal would mint v4 blocks that nodes without
  the era-3 software cannot validate — the exact silent-mis-validation Q7 refutes. The cert is
  explicit: era-3 gates on a DISTINCT readiness level (`regVersion >= 4`). Deviating here is a
  new research gate. Refused.

### Option C — a separate era-3 signalling field on BondReg (a new `Era3Ready` bool)

Add a dedicated readiness flag rather than reusing the `regVersion` scalar.

- **Rejected.** `regVersion` is already the monotone software-level signal (`BondReg.Version`,
  the #506 mechanism the cert names in Q7: "reused at a higher level — no new machinery"). A
  validator sets `regVersion = 4` only when it can enforce the R-rule AND validate committed
  roots. Adding a parallel field duplicates the mechanism and touches the value encoding /
  BondReg schema — a bigger change for no gain. The cert's chosen mechanism is `regVersion >= 4`.
  Refused as gold-plating that also widens the format surface.

## The chosen design in detail

### Activation derivation (mirror `rotateEpoch` #506 lock-in)

In `rotateEpoch`, alongside the #506 tally, add:

```
if !c.era3LockedIn && c.cfg.Era3ActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
    var total, ready int64
    for id, w := range set {                 // `set` = liveQualifiedSet(), the frozen weights
        total += w
        if c.regVersion[id] >= BlockVersionStateRoot { ready += w }
    }
    if total > 0 && 3*ready > 2*total {
        c.era3LockedIn = true
        c.era3Height = h + c.cfg.EpochBlocks   // enforce from the NEXT boundary
    }
}
```

Identical to the #506 tally except the threshold is `>= BlockVersionStateRoot` (4). One
finalized epoch of notice (`H_era3 = h + EpochBlocks`); monotonic by the `!era3LockedIn` guard;
weight not heads (a cheap-bond cohort cannot fake-signal); Byzantine inflation absorbed by the
shared >⅔ threshold. This runs on `set = liveQualifiedSet()` — the frozen epoch weights — so it
is a pure function of committed history: every replica, live or replaying, derives the same
`H_era3`. Epoch boundaries are super-quorum-final (#357 Condition A), so `H_era3` cannot be
reorged out (Q5 "the boundary argument transfers").

### The `era3Active` predicate (mirror `regGateActive`)

```
func (c *Chain) era3Active(h uint64) bool {
    if c.cfg.Era3ActivationHeight > 0 { return h >= c.cfg.Era3ActivationHeight }
    return c.era3LockedIn && h >= c.era3Height
}
```

**One deliberate difference from `regGateActive`: `>=`, not `>`.** The #506 R-rule applies to
every block of height > H_act because the boundary block is "the last old-rules block" (a
validity-payload rule). era-3 is a MINT/FORMAT boundary: `H_era3` is defined as "the height at
and above which v4 is the minted format". Using `>=` makes `H_era3` itself the first v4 height,
which is the requirement's phrasing ("at/above `H_era3`, a block MUST be v4"). This is a naming
choice, not a soundness deviation — the certified property is monotonic height-gated activation
keyed on finalized history, satisfied identically by `>=` at `H_era3` or `>` at `H_era3 - 1`.
The genesis override uses the same `>=` for consistency, so a trusted launch declares "v4 from
height N" directly.

### Mint-flip (`proposeBlock`, chainrole.go:844)

The single mint-version stamp is line 844 (`b.Version = chain.BlockVersionRounds`). Replace it
with a version decision + root population, placed AFTER all apply-affecting content (BondRegs,
entries, slashes) is folded in — because the roots are over the POST-APPLY committed state, and
the folded content changes that state. The chain exposes `MintVersion(height)` and a
`PopulateEra3Roots(block)` helper so the node asks the chain (which owns the activation state)
rather than re-deriving it. If `era3Active(height)`: stamp v4, compute `StateRoot()`/`LogRoot()`
over the post-apply state of THIS block (the existing 2a `postApplyRoots` dry-run), attach them.
Else: stamp v2. Below `H_era3` the path is byte-for-byte the old behavior.

### Boundary validity (`ValidateProposal`)

Add, before the 2b root predicate: if `c.era3Active(b.Height)` and `b.Version < BlockVersionStateRoot`,
reject with `ErrEra3VersionRequired`. This makes a v2 block at/above `H_era3` invalid. Below
`H_era3`, no era-3 version requirement fires — era-2 validation unchanged. The 2b predicate then
enforces the roots on the v4 block. Together: at/above `H_era3` a block must be v4 AND carry
valid roots; a wrong root is already rejected by 2b.

### Reorg-stability / snapshot / clone plumbing

`era3LockedIn`/`era3Height` are derived committed state, exactly like `gateLockedIn`/`gateHeight`:
- classified `committedSet` in the completeness oracle → folded into the state root;
- copied by value in `cloneForDryRun` (the dry-run apply must see the same activation state);
- carried in `adopt` (a reorg replays every rotation, re-deriving them);
- carried on snapshot boot via their `committedSet` classification. Snapshot-boot equivalence is
  a model-check property today (`snapshotBoot` in `modelcheck_snapshot_equivalence_test.go`
  reflects over the committedSet classification); there is no production chain-state
  snapshot-boot path yet, and no `transferState` function. The mechanism that carries these
  fields is the committedSet classification itself, not a named transfer routine.

Because they enter the state root, a node that forged them would produce a mismatching root and
be rejected by 2b — the activation state is itself committed-root-protected.

## Invariants (I1–I5)

- **I1 (quorum intersection):** untouched. The mint-flip and version rule do not change any
  quorum size, threshold, or the weight-sum seam. The era-3 lock-in READS the frozen `epochSet`
  weights (the same set I3 freezes) and applies the SAME >⅔ super-quorum the finality rule uses —
  it does not create a new quorum.
- **I2 (never sign twice):** untouched. No change to the sign ledger or watermark.
- **I3 (set changes only at finalized boundary; by weight):** PRESERVED and RELIED ON. The
  lock-in tallies frozen-epoch weight at a rotation, integrating the rule change only at a
  finalized boundary — the same discipline I3 mandates for set changes. It does NOT modify
  `epochSet`, `rotateEpoch`'s freeze, or the `⌈A/2⌉` threshold (STOP boundary honored).
- **I4 (commit ≠ final):** untouched. No change to the finality gate, fork-choice, or the
  rounds/locking machinery.
- **I5 (deterministic fork-choice; attributable safety):** PRESERVED. `era3Active`,
  `MintVersion`, and the boundary rule are pure functions of committed state — every honest
  replica computes the identical verdict and mints the identical version. No new slash path.

## STOP boundaries honored

- Implements the CERTIFIED shape only: `regVersion >= 4` supermajority, height-gated, mirroring
  #506. No new activation condition invented.
- Does NOT touch the weight-sum seam, `epochSet` freeze / `rotateEpoch`'s Condition-A freeze,
  the `⌈A/2⌉` threshold, or the value encoding.
- Does NOT add a mint/append path that writes a v4 block without the 2b predicate — the mint
  path goes through `ValidateProposal` (which runs 2b) before the local Append, and the write-set
  guard stays green.

## Proof plan (model-check tier — every green needs a demonstrated RED)

`core/chain/modelcheck_era3_activation_test.go`:
1. Below `H_era3`: nodes mint v2 (`MintVersion(h) == v2`); a v4 requirement does NOT fire
   (a v2 block at h < H_era3 validates); era-2 unchanged. RED if the version rule fires early.
2. At/above `H_era3`: `MintVersion(h) == v4`; a correctly-rooted v4 block is ACCEPTED; a v2 block
   at h >= H_era3 is REJECTED (`ErrEra3VersionRequired`); a v4 block with a wrong root is REJECTED
   (2b, `ErrEra3StateRootMismatch`). RED if any of these flips.
3. Readiness gates activation: below the >⅔ ready-weight threshold → no lock-in (era3Active stays
   false at every height); at/above → lock-in, and `H_era3` == the derived `boundary + EpochBlocks`.
   RED against a tally that ignores the threshold or counts heads.
4. Reorg-stability: a reorg that replays the ready history re-derives the SAME `H_era3`; a fork
   WITHOUT the ready signal does not carry activation — the boundary cannot be moved to un-enforce.
   RED against activation state that survives a reorg it was not earned on.

Each assertion pairs with an ablation (a demonstrated RED) recorded in the test comments, per the
session-7 rule: a green check is not shipped until its defect has been injected and watched go red.
