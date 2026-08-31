# Floor-box recompute increment 3 — reproduce `requireDeMatureSuperQuorum` trustlessly

Date: 2026-08-31
Seat: Builder
Branch: `feat/floorbox-recompute-increment3-dematureQuorum` (off `origin/main` = `f29bd73`)

## The target predicate

`requireDeMatureSuperQuorum` (`core/chain/chain.go:2947-2964`) is the F-1 de-maturation
super-quorum. Once a chain has matured (`everMature`) but its live decentralization has since
dropped below the bar (`!matureNow()`), a commit must be carried by a real-bond SUPER-MAJORITY:
the committing coalition (proposer + the distinct qualified attesters `seen`) must control
`≥⌈2·total/3⌉` of the WHOLE live `bonded` weight — no anchor sign-off.

The fold, byte-for-byte:

```
total     = Σ over the WHOLE bonded map
committed = bonded[proposer] + Σ_{id∈seen} bonded[id]
need      = (2*total + 2) / 3      // ⌈2·total/3⌉
met       = committed >= need      // (total<=0 ⇒ trivially met: nothing to protect)
```

It is GATED behind `c.everMature && c.objective() && !c.matureNow()` (`chain.go:2827`) — the
de-mature transition. Increment 2's `RecomputeMatureNow` already reproduces `matureNow`
trustlessly, so the gate is reproducible.

## The read pattern (identical to increments 1/2)

This is the THIRD weighted validity predicate. The pattern from increments 1
(`RecomputeEpochWeightQuorum`, epochSet) and 2 (`RecomputeMatureNow`, validatorsSeen):

1. **Set-completeness.** Reconstruct `nodeSetMTH(witnessedIDs)` over the whole-`bonded`
   id-list; require it equals the committed `bondedRoot` digest leaf (itself proven present
   against the committed StateRoot). One omitted/injected member ⇒ different MTH ⇒ stall.
   This is the F1 `bondedRoot` digest FINALLY READ (F1 committed it inert; increment 3
   consumes it for `bonded`).
2. **Per-member weight (C-1).** For every id in the reconstructed set, `Resolve` its
   `bonded[id]` weight leaf against the committed root. A forged weight fails
   `smt.VerifyProof` ⇒ stall. The digest gives membership; the inclusion proofs give the
   values.
3. **Own config / threshold (C-6).** The ⅔ ratio is a fixed consensus constant, not a
   genesis knob — like increment 1's `requireEpochWeightQuorum`, this predicate's fold reads
   NO per-deployment config value from the witness. The threshold cannot be shifted via the
   witness. The C-6 obligation is exercised (the box takes NO threshold from `w`); the
   config-from-witness ablation asserts it (below).

Then the fold + threshold, byte-for-byte the full node's (`chain.go:2949-2963`).

## The maturity gate — the increment-3-specific piece

`requireDeMatureSuperQuorum` fires ONLY when `!matureNow()`. So the trustless reproduction
must not fold the super-quorum in the mature state. Two options:

- **Option A (chosen): gate on the reproduced `matureNow`.** `RecomputeDeMatureSuperQuorum`
  takes a `SeenSetWitness` (increment 2's input) alongside the `BondedSetWitness`, calls
  `RecomputeMatureNow` first, and reproduces the de-mature verdict ONLY when the maturity
  recompute returns `mature == false`. When `mature == true` the predicate is a no-op
  (a full node does not run it), so the recompute returns `(met=true, nil)` — matching the
  full node, which skips the check. This keeps the box's verdict equal to the full node's:
  the de-mature bar binds exactly when `!matureNow()`, and is vacuous otherwise.
  - Cost: the box witnesses BOTH the whole-`bonded` set (for the super-quorum fold) AND the
    whole-`validatorsSeen` set (for the maturity gate). Both are already tracked
    (R-membership budget; whole `bonded` fits ~1-2M members per the disk-backed-store
    measurement). No new cost class.
  - Benefit: the gate is REPRODUCED, not assumed. A box cannot be tricked into enforcing (or
    skipping) the de-mature bar in the wrong maturity state, because the maturity state is
    itself proven from the committed root.

- **Option B (rejected): take `matureNow` as a trusted boolean input.** Simpler signature,
  but it lets an untrusted producer hand the box the WRONG maturity state and thereby suppress
  the de-mature bar (claim mature when the chain is not) — a soundness hole. Increment 2
  exists precisely to make `matureNow` trustless; consuming its output here is the point.

**Decision: Option A.** Gate on the reproduced `matureNow` (reuse `RecomputeMatureNow`). The
de-mature predicate only folds in the reproduced `!matureNow` state; in the mature state the
recompute is a no-op that matches the full node's skip.

## The proposer/seen coalition

The full node reads `bonded[b.ProposerID()]` and `bonded[id]` for each `id∈seen`. The
recompute takes `proposer` and `seen` as caller-supplied inputs (exactly as increment 1's
`RecomputeEpochWeightQuorum` does), and folds `committed` from the per-member weights it has
ALREADY proven against the committed root. A proposer/seen id NOT in the witnessed bonded set
contributes 0 (it is not in the whole-`bonded` fold) — matching the full node, where
`c.bonded[id]` is 0 for a non-bonded id. No extra proof is needed for coalition membership:
the coalition weight is a subset-sum of the already-proven whole-`bonded` weights.

## The #535 recovery-boundary carve-out (flag, do NOT reproduce)

`requireEpochWeightQuorum` (increment 1) governs its set by `effectiveEpochSet(h)`, which the
#535 recovery boundary swaps to `liveQualifiedSet`. `requireDeMatureSuperQuorum` folds the
WHOLE `bonded` map directly — it does NOT consult `effectiveEpochSet` or `liveQualifiedSet`,
so the #535 boundary does NOT change this fold. There is no boundary case to carve out for the
de-mature predicate itself. The boundary remains the ratified trust-the-directive carve-out
(accept-flip gate 1) for the sets it DOES touch (epochSet); this increment reproduces the
non-boundary path and does not reproduce any boundary case. Flagged here for the record.

## STOP boundary

Reproduce ONE predicate. Do NOT flip `WitnessValidateV5` to Accept — that is the final
increment, only after ALL predicates are reproduced. The box STILL never-Accepts.

## The hard ablations (C-5, red-before-green)

- **Forged per-member `bonded` weight ⇒ REJECT (C-1).** Forge one member's claimed weight
  while keeping its original inclusion proof (built for the true weight). `Resolve` against
  the forged `EncodeInt64(weight)` fails ⇒ `ErrRecomputeBondedMemberWeightUnproven` ⇒ stall.
- **Omitted / injected `bonded` member ⇒ REJECT (set-completeness).** Drop (or pad) a member
  from the witnessed id-list ⇒ reconstructed MTH ≠ committed `bondedRoot` ⇒
  `ErrRecomputeBondedSetIncomplete` ⇒ stall.
- **Config-from-witness threshold ⇒ REJECT (C-6, failing-first).** A negative-control fold
  that read the ⅔ threshold (or the coalition `need`) from the witness would let an attacker
  shift the bar. The real fold reads the fixed ⅔ constant from code and takes NO threshold
  from `w`; the ablation demonstrates a witness-carried threshold cannot move the verdict.

## Producer changes

- Remove `bondedRoot` from `inertDigestRootTags` (so `isDigestRootLeaf` no longer excludes it
  from the ground truth), and add a real red-on-drop ablation
  (`TestBondedRootReadReddensOnDrop`), replacing the skip-guarded placeholder for `bondedRoot`
  in `TestInertDigestRootsAwaitRecompute`.
- Emit the whole-`bonded` id-list + per-member `bonded` reads + the `bondedRoot` digest leaf
  in the producer (`readSetBondedRoot`), fired whenever the de-mature gate can run (mature
  latch set and objective). The remaining still-inert roots (`qualifiedRoot`, `slashedRoot`)
  keep their placeholders.
