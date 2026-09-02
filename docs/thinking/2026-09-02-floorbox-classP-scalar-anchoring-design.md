# Floor-box class-P scalar-anchoring — Direction A + B (the R1.4 partial-retract fix)

Seat: Builder — 2026-09-02. Branch `builder/floorbox-classP-scalar-anchoring`.
Governs: ROADMAP Boulder-1; the owner-ratified class-P fix.
Certification (the spec, followed exactly):
`../silt-reviews/research/research-outcome/floorbox-classP-activation-rotate-anchoring-RESEARCH-CERTIFICATION-2026-09-02.md`.

## The mechanism (attribute before you patch)

The failure is a wrong-accept-by-recompute (×4) plus a false-stall (×1), because
`scalarFoldOp` (`floorbox_recompute_stateroot_rotate_v5.go:472`) verifies the pre-state
proof ONLY when it emits an op (`wit.OldValue != newValue`). On the suppression path
(`OldValue == newValue`, no emit) the pre-value is never fold-verified. That one line is
read through three call shapes:

1. the three lock-in BRANCH predicates `!decodeBoolLeaf(rw.X.OldValue)` at `:442/:450/:458`;
2. the `MatureEpoch` emit-decision at `:264`;
3. cross-class, the `everMature` branch read `preEverMature` at `maturitylatch_v5.go:80`.

A forged `OldValue` equal to the value the box would write suppresses the emit/skips the
branch; the committed change is silently omitted; the attacker commits a root that also
omits it; `postRoot == StateRoot` holds by construction. This is the R1.3 structure (fold
catches inconsistent, never consistently-forged) reborn at the scalar-predicate layer.

Separately, `anchorRotateMember` (`:355-367`) anchors `RegVersion` only against pre-state.
A fresh in-block bond has no pre-state `regVersion` leaf, so its honest witness sets
`RegVersionKnown=false` → the tally counts it as 0 → the box under-counts ready weight →
stalls where `apply()` (reading the just-written `c.regVersion[id]`, `chain.go:3444`)
finalizes.

## Why the certified anchor is sound (evidence)

Every scalar leaf is committed UNCONDITIONALLY: `statehash.go:196-253` calls `add(...)` for
each scalar with no presence guard, and `EncodeBool(false) = []byte{0}` is a non-empty
value (`statehash.go:71`). So a scalar leaf is ALWAYS present pre-state, even when the bool
is false. Therefore `statehash.Resolve(prevStateRoot, Key(T,nil), wit.OldValue, wit.Proof)`
`.IsProvenPresent()` is true iff `wit.OldValue` byte-matches the committed leaf. A forged
`OldValue` mismatches → `VerifyProof` fails → `NoWitness` (`witness.go:216-221`) → the
anchor stalls. Never `ProvenAbsent`, never a false present: the C-7 banned move stays
unrepresentable at the primitive.

## Direction A — unconditional pre-state anchor (P-s2..P-s8 + everMature)

Add `anchorRotateScalar(prevStateRoot, tag, wit)` that requires `Resolve(...).IsProvenPresent()`.
Call it UNCONDITIONALLY, before the emit/branch decision, for:

- `MatureEpoch` (`:264`) — anchor before the emit.
- the three lock-in bools — anchor in `rotateTallyOps` BEFORE the `:442/:450/:458` branch
  read, so the branch predicate is trusted. The height scalars (`GateHeight` etc.) ride the
  bool (cert §1b): suppressing the bool suppresses the pair, so anchoring the bool closes
  them; they keep their existing emit-time fold anchor.
- `everMature` (`maturitylatch_v5.go:80`) — anchor `preEverMature` before the branch.

`epochStart` (P-s1) is left as-is: its newValue is `b.Height`, which strictly advances every
boundary, so the emit ALWAYS fires and the OldValue is ALWAYS fold-verified (cert §1a). No
suppression path exists.

The tally arithmetic (`3*ready > 2*total`, the 3/4/5 levels, the `*ActivationHeight`
guards) is byte-for-byte untouched (#402 non-fork / PE constraint 2). The fix anchors the
INPUT bool, never the threshold.

## Direction B — in-block RegVersion cross-check

Extend `bondRegOpsWithQualWrites` to also return `regVerWrites map[ports.NodeID]uint8`,
derived from the same canonical winners it already folds (the `regVersion||id` changed leaf,
already fold-anchored). Thread it through `reconstructPostQualifiedWithWrites` → `rotateOps`
→ `anchorRotateMember`. In `anchorRotateMember`: if `id ∈ regVerWrites`, cross-check the
frozen member's tally `RegVersion` against the B-derived post-write value (fold-anchored)
and DO NOT read `RegVersionProof`; else keep the pre-state present/absent Resolve. The tally
then counts the fresh in-block member at its true post-write regVersion, matching `apply()`.
Never a wrong-accept (the value is fold-anchored, not witness-trusted).

Reconcile the contradicting doc-comments: `rotate_v5.go:105-106` (claimed an in-block
cross-check exists — it did not) is corrected to describe the built behavior; `:330-332`
already described the actual pre-state path and stays.

## R-COVERAGE-SCALAR-SPLIT

Split the `StateRootRotateScalar` coverage classification so the OldValue disposition
records the emit-vs-suppress distinction: `epochStart` OldValue is emit-anchored; the
suppressible scalars (matureEpoch, the three lock-in bools) are suppress-anchored (Direction
A). A future suppressible scalar cannot be wholesale-classified "anchored" — the split forces
a per-scalar disposition. The FIX-OPEN row flips to already-anchored (Direction A) with the
suppress-path gate named. This is the teeth against a third recurrence.

## What must NOT change (hard constraints)

- The box still NEVER Accepts: `WitnessValidateV5 → IndeterminateTrustlessly, ErrRecomputeGated`.
  `TestWitnessValidateV5_NeverAccepts*` stays green. STALL-ADDING ONLY.
- No committed v5 format field changes (the scalars are already committed; only the RECOMPUTE
  reads them differently).
- The #402 tally arithmetic is untouched.

## Gates flipped (assert-stall)

- R1.6: `TestOpenBreak_{Gate,Era3,Era4}LockedInOldValuePredicate` → assert `err != nil`.
- R1.5: `TestScheduleOracle_OpenBreak_A_*` (lock suppression) and `_OpenBreak_B_*` (in-block
  RegVersion) → assert stall; the I1 fork diagnostic is updated to assert the forged witness
  is now REFUSED.
- New suppress-path gates for `MatureEpoch` and `EverMature`.

## R-ROTATE brittleness riders (folded into rotate_epoch_last_drift_test.go)

- (D) allowlist the sanctioned `idQualifies` predicate in the `liveQualifiedSet` read-set
  guard, so the refactor `chain.go:1359-1361` invites (re-expressing `liveQualifiedSet` over
  `idQualifies`) does not false-RED the structural guard.
- (B) assert rotate-is-last-in-the-gate-body, not exactly-one-statement — a benign extra
  statement in the boundary gate should not RED the guard; rotate being last is the invariant.
