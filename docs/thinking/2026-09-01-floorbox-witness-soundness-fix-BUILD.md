# Floor-box witness-soundness fix (R1.2) — BUILD-DAY decisions

Date: 2026-09-01
Status: BUILD. Ships with the code. Companion to the DESIGN note
(`2026-09-01-floorbox-witness-soundness-fix-design.md`) and the PE ruling
(`silt-reviews/principle-engineer/RULING-floorbox-R1.2-invariant-pins-2026-09-01.md`).

This note records the build choices the design under-specifies, so the increment is
certifiable without a verbal hand-off. Everything here defers to the design and the ruling.

## Mechanism (attribution, confirmed against the tree)

The failure is a wrong-accept-by-recompute: recompute classes P / A / B read per-member
VALUES and SCREEN PREDICATES from the untrusted witness structs and use them as fold
`NewValue`s or branch predicates WITHOUT resolving them against `prevStateRoot`. Class M
(`RecomputeMatureNow`) inherits an inflated `validatorsSeen` set through the committed root
when class A admits a spurious member. Evidence: the 11 adversarial-root gates in
`floorbox_recompute_adversarialroot_v5_test.go`, each RED on main (the box wrong-accepts a
forged committed root the attacker also moves).

## The fix — one shape, applied per field

Thread `prevStateRoot` into the screen/write-set functions and re-anchor every untrusted
field-read with `statehash.Resolve(prevStateRoot, Key(tag, id), claimedValue, proof)`:

- a value/membership read requires `IsProvenPresent()` with the claimed value;
- an absence read requires `IsProvenAbsent()`;
- `MustStall` (NoWitness) returns a stall error. NEVER falls through to a false/absent read.

This mirrors the sound classes (S/T/M) that already route every witness value through a
VerifyProof'd `OldValue` or `Resolve`. The new proofs live on the witness carriers.

## Build decisions the design leaves to the builder

### D1 — Carrier proof fields, not a separate witness bundle

The design offered "one combined per-attester witness bundle" as an option. I add the proof
fields DIRECTLY to the existing carriers (`StateRootAttScreen`, `StateRootRotateMember`,
`StateRootBondRegScreen`). Reason: SIMPLEST diff, one proof per field-read, and the
reflection coverage meta-assertion keys on the carrier struct — a bundle would need its own
coverage story. Each new field is `X statehash.Witness` alongside the value it anchors.

### D2 — Class A anchors at SOURCE (`attesterQualifiedFromScreen`), per PE constraint 1

The screen predicate is anchored inside `attesterQualifiedFromScreen` (which now takes
`prevStateRoot`), NOT only at the fold-write. This is the PE's load-bearing constraint: a
forged class-A screen inflates `validatorsSeenRoot`, and class M inherits it. Anchoring the
screen forecloses the spurious `validatorsSeen||id` ADD at its source, which closes the
class-M inheritance (the `TestAdversarialRoot_ClassM_PoisonedBySpuriousAtt` gate proves it).

Per-field class-A anchors (all point Resolve against `prevStateRoot`):
- `Slashed=true`  → `Resolve(Key(tagSlashed, id), Present, proof).IsProvenPresent()`;
  `Slashed=false` → `.IsProvenAbsent()`.
- `InEpochSet=true`  → `Resolve(Key(tagEpochSet, id), EncodeInt64(BondedSize?), proof)` — the
  epochSet leaf is value-carrying (the frozen weight). Membership is all the screen needs
  (weight discarded, R-A-membership-source), so the box proves PRESENT with the committed
  weight value the witness carries and checks only presence. `InEpochSet=false` →
  `.IsProvenAbsent()`.
- `BondedPresent=true` → `Resolve(Key(tagBonded, id), EncodeInt64(BondedSize), proof).IsProvenPresent()`;
  `BondedPresent=false` → `.IsProvenAbsent()`.

Note: the class-A screen also serves the pre-maturity path. The InEpochSet value-anchor
requires the committed epochSet weight; the screen carries only membership intent, so the
InEpochSet proof is a membership proof whose value is the committed epochSet leaf value. The
test supplies that value. For `InEpochSet`, the box does not use the weight — it only
requires the presence/absence proof to verify.

### D3 — Class P: do NOT fork the quorum arithmetic (PE constraint 2, the #402 trap)

`rotateTallyOps` keeps `3*ready > 2*total` byte-for-byte. The fix anchors the INPUTS
(`Weight`, `RegVersion`, `RegVersionKnown`) BEFORE they enter the tally, inside `rotateOps`
(where `prevStateRoot` is threaded). Anchors:
- `Weight` → `Resolve(Key(tagQualified, id), EncodeInt64(Weight), proof).IsProvenPresent()`.
  The freeze copies the post-qualified weight; for a steady-state member the frozen weight IS
  the committed `qualified||id` leaf. (An in-block-bonded member's qualified leaf is written
  by class B in THIS block; its weight is cross-checked against the B-derived `qualWrites`
  rather than a pre-state proof — see D4.)
- `RegVersion` (RegVersionKnown=true) → `Resolve(Key(tagRegVersion, id), EncodeUint8(RegVersion), proof).IsProvenPresent()`.
- `RegVersionKnown=false` → `Resolve(Key(tagRegVersion, id), nil, proof).IsProvenAbsent()`.

`TestActivationQuorumNonFork` stays GREEN — the arithmetic is untouched.

### D4 — Class P Weight: steady-state vs in-block-bonded

The frozen `Weight` for a member NOT touched by this block's class B is anchored against the
committed `qualified||id` leaf under `prevStateRoot` (D3). For a member bonded/resized in THIS
block, the pre-state `qualified||id` is stale (or absent), so the pre-state anchor is wrong.
For those, the box cross-checks the frozen `Weight` against the class-B-derived
`qualWrites[id]` (the post value class B computed and will fold), which is itself anchored by
the class-B fold. The rotate member's `QualifiedProof` is therefore OPTIONAL for an
in-block-bonded id (the cross-check replaces it) and REQUIRED for a steady-state id.

Simplest correct rule: the box is handed the entry-threaded `postQualified` set AND the
class-B `qualWrites`. For each frozen member: if `qualWrites[id]` exists, require
`Weight == decodeInt64(qualWrites[id])` (or, for a delete, the member cannot be frozen); else
require the `qualified||id` present-proof at `Weight`.

### D5 — Class B: anchor the displacement inputs (PriorOwner / Claimed / PriorProven)

The displacement branch reads `owner[root]`, `claimed[root]`, `provenRoot[root]` from the
raw screen. Anchor each against `prevStateRoot`:
- `Claimed=true` → `Resolve(Key(tagBondRootOwner, root), EncodeID(PriorOwner), proof).IsProvenPresent()`;
  `Claimed=false` → `.IsProvenAbsent()` of `bondRootOwner||root`.
- `PriorProven` → `Resolve(Key(tagBondRootProven, root), EncodeBool(PriorProven), proof)`:
  present-true / present-false each proven present; the honest absent case (unclaimed root)
  is covered by `Claimed=false` (no proven leaf exists for an unclaimed root).

The `bondRootOwner`/`bondRootProven` leaves are committed set leaves. `PriorProven=false`
for a CLAIMED-but-unproven root is a present leaf `EncodeBool(false)` (apply writes proven
only when true — so an unproven claimed root has NO `bondRootProven` leaf → absent). Rule:
`PriorProven=true` → present-proof `EncodeBool(true)`; `PriorProven=false` → absent-proof of
`bondRootProven||root`.

### D6 — The 11 gates flip from "assert wrong-accept" to "assert stall"

The gates ship RED on main by ASSERTING the box wrong-accepts (`err == nil`). After the fix
the box stalls, so each gate's terminal assertion inverts to require `err != nil` AND that the
error is a stall from the anchoring Resolve (`ErrRecompute...`). This is the RED-before /
green-after the design and ruling require. Each gate carries the honest-witness AGREE check
already; that must stay green (the fix must not break honest agreement). The gates get the new
proof fields populated from their existing pre-state provers for the honest witness, and a
NIL/absent proof for the forged field (which is exactly what an attacker who cannot forge a
`prevStateRoot` proof faces) — so the forged read stalls.

### D7 — Coverage meta-assertion (Tier C)

A reflection-pinned test over the three carrier structs asserting each field is classified
FIX (has a driven gate) or already-anchored (stated reason), with a teeth companion. Modeled
on `TestLeafDiffGuardCoversEveryEmittableTag`.

## Never-Accept preserved

Every new anchor ADDS a stall path (`MustStall` → error). No new Accept path. The STOP
boundary (`RecomputeStateRootEntriesRevocations:212-216`) is untouched.
`TestWitnessValidateV5_NeverAcceptsWhileRecomputeGated` stays green.

## Consult status

Builder-tier witness-soundness fix as scoped (adds stalls, never accepts). Does NOT touch
apply(), the block/validity rules, or I1–I5. Routes to the Researcher only if the fix would
flip the box toward Accept (it does not) or if a Weight branch reveals an existing accept path
(the gates show wrong-accept-by-recompute behind the STOP boundary, not a live accept).
