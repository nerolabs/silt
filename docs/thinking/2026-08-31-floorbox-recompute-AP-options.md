# Floor-box v5 state-root recompute — Class A + Class P (the last two Path-1 classes)

Date: 2026-08-31
Author: Builder
Governing cert: `../../silt-reviews/research/research-outcome/floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md`
(both CERTIFIED-in-direction; A carries R-A-legacy / R-A-membership-source; P carries
R-P-boundary-scalars / R-P-tally-regversion / R-P-sameblock-order / R-P-recovery.)

This lands the final two apply() transition classes of the Path-1 O(payload)+O(registry-per-digest)
recompute spine. It reuses the proven primitive: derive the write-set → witness each touched leaf
against prevStateRoot → reconstruct each touched whole-set digest via `nodeSetMTH` → require
`postRoot == StateRoot`. The box STILL never-Accepts (R-scope).

## Measurement — the real leaf-diff apply() produces (ground truth, not a guess)

A throwaway harness (`zz_measure_ap_test.go`, deleted after) applied real blocks to a real chain
clone and diffed `stateRootLeavesV5()` before/after. Config: objective v5, MinBond=era4MinBond,
BondTTLBlocks=64.

### Class A (a non-proposer att that writes validatorsSeen), MatureValidators=0 (matureEpoch=true)

Pre-state: proposer + attester both bonded>MinBond, both qualified, in the frozen epochSet.
A block at h=1 with one entry + one non-proposer att by the attester produced:

```
A ADDED:   byRoot||<entry>        = 01          (E/R, already handled)
           validatorsSeen||<att>  = 01          (THE A-WRITE)
A CHANGED: validatorsSeenRoot     : <32b>-><32b> (the whole-set digest)
A DELETED: (none)
```

So the A class touches exactly: the per-member `validatorsSeen||attester` Present leaf (an ADD,
add-only — apply never deletes validatorsSeen) plus the `validatorsSeenRoot` digest. Screen inputs
(slashed, epochSet membership) are READ, not written — they ride the changed-leaf/point witnesses.

### Class P (epoch boundary), MatureValidators=0, EpochBlocks=2

A steady-state boundary (epochSet unchanged, all lock-ins already locked at genesis) at h=4:

```
P CHANGED: epochStart : 2 -> 4
```

Only `epochStart` changes — it advances EVERY boundary. This is the floor of P's write-set.

A boundary that ALSO carries a new bond reg (membership change at the freeze), h=4:

```
P(+bondreg) ADDED:   bondDomain||nv, bondRegHeight||nv, bondRootOwner||nv, bondRootProven||nv,
                     bonded||nv, dueBucket[due]||, regVersion||nv,     (class-B leaves)
                     epochSet||nv    = 0x600000                        (THE FREEZE per-member ADD)
                     qualified||nv                                     (class-B qualifiedMaintain)
P(+bondreg) CHANGED: bondedRoot, qualifiedRoot,                       (class-B digests)
                     epochSetRoot,                                    (THE FREEZE digest)
                     epochStart : 2 -> 4                              (P scalar)
```

Load-bearing observation: the frozen `epochSet` includes the validator this SAME block just bonded.
`epochSet = clone(qualified_POST)` runs LAST (rotate-LAST). This is R-P-sameblock-order made
concrete: the freeze source is the POST-apply qualified set, so P must be layered on top of the
same block's B/S/T qualified deltas, then freeze.

### Lock-in scalars

The three activation tallies (`gateLockedIn`/`gateHeight`, `era3LockedIn`/`era3Height`,
`era4LockedIn`/`era4Height`) write their scalar ONLY at the one boundary where the tally first
crosses `3*ready > 2*total` (monotonic guard). In the steady-state and never-mature fixtures they
did not change. `matureEpoch` flips false→true once, at the first mature rotation.

## The write-set / read-set derivation

### Class A — `floorbox_recompute_stateroot_atts_v5.go`

Write-set (per non-proposer att that passes the screen): one `validatorsSeen||id` ADD.
Digest: `validatorsSeenRoot` (reconstructed from the post-validatorsSeen id-set).

Screen (per attester, O(payload) point reads, box computes qualification itself from own-cfg over
witnessed inputs — never a witness verdict):
1. `slashed[id]` — F2 gate. Present ⇒ not qualified.
2. If objective + epochsEnabled + matureEpoch: `epochSet[id]` MEMBERSHIP (weight discarded). This is
   the FROZEN set, per R-A-membership-source — read epochSet, never live bonded.
3. Else (pre-maturity objective): `bonded[id] >= MinBond || launchAnchor(id)` (own-cfg MinBond +
   own-cfg Anchors/handedOff).
4. Legacy mode: `rep(id)` — NOT a committed leaf. R-A-legacy: assert objective-mode; STALL if legacy.

The add-set = payload non-proposer attesters passing the screen. Fold the adds into pre-validatorsSeen
→ reconstruct `validatorsSeenRoot`.

### Class P — `floorbox_recompute_stateroot_rotate_v5.go`

Every rotateEpoch write (chain.go:3393-3500), reproduced:
- `epochStart = h` — ALWAYS (scalar). Folded every boundary.
- early-return if `!everMature` (witness the everMature pre-scalar). If not everMature, ONLY
  epochStart changed — reconstruct just that.
- `matureEpoch = true` — scalar, folded iff pre was false.
- `epochSet = clone(qualified_POST)` [normal] or `liveQualifiedSet()` [#535 recovery] — per-member
  ADD/DELETE leaves + `epochSetRoot` digest.
- three tallies over the frozen set, each reading `regVersion[id]` per member, `3*ready > 2*total`,
  each gated on own-cfg `*ActivationHeight == 0`, each writing a lock-in bool + height scalar.

The freeze SOURCE = POST-apply qualified. Reconstruction: anchor pre-qualified against prevStateRoot
(`qualifiedRoot` pre-digest), apply the same block's B/S/T qualified deltas FIRST (R-P-sameblock-order),
freeze = clone(post-qualified). Recovery boundary re-bases from the box's OWN LivenessRecoveryHeight
config (C-2, R-P-recovery), never a witness.

Tallies: box computes `3*ready > 2*total` from per-member `regVersion` witnesses over the frozen set,
own-cfg thresholds (3/4/5) + own-cfg activation-height guards. Fold a lock-in scalar iff the box's
computed post differs from the witnessed pre.

## Options considered

1. **Always-emit every rotate scalar as a FoldOp (OldValue==NewValue for unchanged).** Rejected:
   forces a witness for every scalar every boundary and pollutes the fold with no-ops. The existing
   E/R/S/B/T pattern emits ONLY changed leaves.
2. **Emit a scalar FoldOp only when box-computed post != witnessed pre (CHOSEN).** Matches the
   existing pattern; the fold's terminal equality still catches an omitted-but-should-have-changed
   scalar (post-root diverges ⇒ stall). Box computes each post from its own reconstruction + own-cfg;
   pre comes from a changed-leaf witness the fold verifies against prevStateRoot.
3. **Reconstruct the freeze from a standalone qualifiedRoot witness, ignore same-block deltas.**
   REFUTED by the measurement: the frozen epochSet includes this block's just-bonded validator. Must
   apply B/S/T deltas first, then freeze (R-P-sameblock-order).

## Scope gate change

Currently the scope gate stalls unconditionally on an epoch boundary and on any non-proposer att.
Widen it: A blocks (non-proposer atts) and P blocks (boundaries) are now IN scope, dispatched to the
new reconstructions. Legacy-mode A and #535-recovery-without-config still stall.

## Ablations (C-5, red-before-green, each driving the REAL recompute entry)

- A forged qualification screen ⇒ stall
- A legacy-mode ⇒ stall
- A omitted validatorsSeenRoot ⇒ stall
- P stale-freeze (freeze pre-delta qualified) ⇒ wrong epochSetRoot ⇒ stall
- P flipped weight-tally / wrong regVersion ⇒ stall
- P short/forged qualified-set witness ⇒ stall
- P missing rotate write (matureEpoch/epochStart/lock-in scalar) ⇒ stall

Each ablated RED-when-injected, GREEN-when-restored, through a real call to
`RecomputeStateRootEntriesRevocations`. Byte-exact vs real `apply()` + `StateRootForVersion(5)`.

## Cost (honest)

A: screen O(payload); `validatorsSeenRoot` reconstruction O(|validatorsSeen|). P: O(|qualified|) for
the freeze + tallies. Both O(registry), riding R-membership (OPEN, load-bearing for the #657
accept-flip). No new format element; both digests + all scalars already exist as committed v5 leaves.
