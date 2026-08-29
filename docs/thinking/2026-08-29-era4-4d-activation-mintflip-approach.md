# era-4 step 4d — height-gated activation + mint-flip to v5 (PACE approach + ablation plan)

**Date:** 2026-08-29
**Seat:** Builder
**Status:** approach + ablation plan. Implements the CERTIFIED era-4 design (RECERT2) and the
RATIFIED build decomposition (`docs/thinking/2026-08-29-era4-build-decomposition-options.md`, §2
increment 4d). 4a/4b/4c are already on `origin/main` @ `fdd6857`.
**Grounded against:** `origin/main` @ `fdd6857` (HEAD verified; every `file:line` below re-checked
against this commit).

## The answer up front

4d is a **faithful structural mirror of era-3 step 2c** (`3af40bc`, `#631`) one era level up. It
adds an era-4 (v5) activation latch, a mint-flip, and a version-boundary rule, reusing the exact
`#506`/era-3 lock-in machinery with the readiness threshold raised to `regVersion >= 5`. **No
deviation from the certified design is required** — every piece 4d needs already has a working
era-3 analog on main, and the one genuinely new question (where the era-4 activation scalars are
committed) resolves cleanly under the 4b v5-leaf-gating already in place.

## What is already on main (verified @ fdd6857 — 4d does NOT re-touch these)

- **v5 constant + tags:** `BlockVersionWitnessable = 5` (`chain.go:357`); `tagDueBucket`,
  `tagQualified`, `tagEpochStart` (`statehash.go:69-71`). — 4a
- **Maintenance spine, maintained on EVERY apply:** `qualified`/`dueBucket` hooks at
  `chain.go:3164,3179,3189,3193,3204`; committed as **v5-only leaves** via `stateRootLeavesV5`
  (`statehash.go:172-199`), gated by `StateRootForVersion` (`statehash.go:225-229`). The maps are
  maintained unconditionally; only the LEAF EMISSION is v5-gated. — 4b
- **v5 validity predicate on every disk-write path + `versionSupported <= 5`
  (`chain.go:790`) + `RegCap = 256`.** — 4c

So 4d flips MINTING only. The predicate, the leaf marshaller, and the decode ceiling are done.

## The era-3 analog 4d mirrors (the shape to copy, verified @ fdd6857)

| era-3 (2c, on main) | era-4 (4d, this PR) |
|---|---|
| `era3LockedIn bool` / `era3Height uint64` (`chain.go:1026-1027`) | `era4LockedIn bool` / `era4Height uint64` |
| lock-in tally, `regVersion >= BlockVersionStateRoot` (`chain.go:3378-3390`) | tally, `regVersion >= BlockVersionWitnessable` |
| `era3Active(h)` (`chain.go:3401-3406`) | `era4Active(h)` |
| `MintVersion` returns v4 at/above H_era3 (`chain.go:3414-3419`) | `MintVersion` returns v5 at/above H_era4 |
| `PopulateEra3Roots` stamps v4 + roots (`chain.go:3430-3439`) | `PopulateEra4Roots` stamps v5 + roots |
| `validateEra3Version`: v<4 rejected at/above H_era3 (`era3validity.go:69-74`) | `validateEra4Version`: v<5 rejected at/above H_era4 |
| `Era3ActivationHeight` pre-latch override (`chain.go:264`) | `Era4ActivationHeight` pre-latch override |
| `adopt` carries the scalars (`chain.go:3750-3751`) | `adopt` carries `era4LockedIn`/`era4Height` |
| `cloneForDryRun` copies the scalars (`era3validity.go:159-160`) | clone copies era-4 scalars |
| classified `committedSet` (`modelcheck_state_completeness_test.go:97-98`) | classify era-4 scalars `committedSet` |

## The one genuinely new decision: where the era-4 activation scalars are committed

era-3 committed `era3LockedIn`/`era3Height` as leaves so "a node that forged them would produce a
mismatching root and be rejected" (`docs/thinking/2026-08-29-era3-step2c-activation-mint-flip.md`
§reorg-stability). era-3 could commit them for EVERY v4 block (they live in `stateRootLeaves`,
`statehash.go:149-150`). **era-4 cannot** — committing them for v4 blocks would edit the frozen
era-3 leaf set and break the era-3 byte-identical freeze (immutable, `#632`).

**Resolution — commit the era-4 scalars as V5-ONLY leaves, exactly like the 4b spine maps.** Add
`tagEra4LockedIn`/`tagEra4Height` and emit them ONLY in `stateRootLeavesV5` (`statehash.go:172`),
adding both names to `stateRootTagsV5` (`statehash.go:79`). This is sound, and it is the SAME
pattern 4b already uses for `epochStart` (a scalar promoted to a v5-only committed leaf,
`statehash.go:196`):

- **The scalars are maintained on every rotation** (in the lock-in tally), the same as
  `qualified`/`dueBucket` are maintained on every apply. Maintenance is unconditional; only the
  leaf emission is v5-gated.
- **Before activation, blocks are v4 and the scalars are at their zero value.** `era4Active(h)`
  first returns true at `era4Height`, which by construction is the FIRST v5 height. So on every
  block where `era4LockedIn == true` / `era4Height > 0` matters, the block is v5 and the scalars
  ARE committed. A snapshot-booted node that boots on a v4 prefix reconstructs the scalars at zero
  (their genuine value there); a node that boots past H_era4 gets them from the committed v5 root.
  There is no height at which a non-zero era-4 activation scalar is uncommitted.
- **The completeness guards force this and check it.** Adding the two fields to `Chain` reddens
  `TestStateFieldsAreClassified` until classified `committedSet`; classifying them `committedSet`
  reddens `TestStateRootCoversExactlyTheCommittedSetFields` until they appear in `stateRootTagsV5`
  (or `stateRootTags`); putting them in `stateRootTagsV5` and forgetting the leaf loop reddens
  `TestStateRootEmitsALeafForEveryV5CommittedField`. The DISJOINTNESS guard
  (`modelcheck_stateroot_determinism_test.go:62-71`) reddens if they leak into the era-3 set.

**Why NOT commit them in `stateRootTags` (the era-3 set):** that breaks the era-3 freeze (a v4
block's root changes) — the same hazard-1 the 4b spine maps avoid. Refuted.

**Why NOT leave them uncommitted (classify observable/transient):** then a forged activation
boundary would ride through a v5 root unchecked — the exact "activation state is itself
committed-root-protected" property era-3 established. The scalars are consensus-critical derived
state; they must be committed. Refuted.

## The era-3 → era-4 layering constraint (a real ordering guard, not a deviation)

A v5 block commits a SUPERSET of the v4 leaves. So era-4 can only mint v5 where era-3 is already
active. Two facts make this automatic under the mirror, but 4d must not break them:

1. **The v5 predicate already requires the v5 root shape** (4c). A v5 block whose root omits the
   spine leaves is rejected. So minting v5 is only correct once the spine is live — which it always
   is (4b maintains it on every apply from genesis).
2. **The era-4 readiness threshold `regVersion >= 5` is strictly higher than era-3's `>= 4`.** A
   node signals 5 only when it can mint AND validate v5, which entails it can already do v4. In the
   post-latch path the era-4 tally therefore cannot lock in before the era-3 tally has the weight to
   (a `>= 5` signaller is also a `>= 4` signaller). In the pre-latch (genesis-declared) path, a
   deployment that sets `Era4ActivationHeight` MUST set it `>= Era3ActivationHeight`; **the mirror
   makes H_era4 the first v5 height and v5 ⊇ v4, so H_era4 < H_era3 would mint a v5 block below the
   era-3 boundary — a config error.** 4d adds a genesis-config assertion (`Era4ActivationHeight`
   must be 0 or `>= Era3ActivationHeight` when both are set) so a misconfigured launch fails loudly
   rather than minting an ill-formed block. This is the ONE piece era-3's 2c did not need (it was the
   first mint-flip era); it is not a design deviation, it is the layering invariant the mirror
   creates, guarded.

## Scope — exactly what 4d changes

1. `chain.go`: add `era4LockedIn`/`era4Height` fields + doc comment (mirror `chain.go:1010-1027`);
   add `Era4ActivationHeight` config field + doc (mirror `chain.go:247-264`); add the era-4 lock-in
   tally in `rotateEpoch` (mirror `chain.go:3378-3390`); add `era4Active` (mirror `3401-3406`);
   extend `MintVersion` to return v5 at/above H_era4 (mirror `3414-3419`); add `PopulateEra4Roots`
   (mirror `3430-3439`); carry the scalars in `adopt` (mirror `3750-3751`); add the layering config
   assertion in `New`.
2. `era3validity.go`: add `validateEra4Version` (mirror `validateEra3Version`, `69-74`) — v<5
   rejected at/above H_era4 — and call it on the SAME write paths era-3's runs (ValidateProposal +
   the own-disk Reload path). Copy the era-4 scalars in `cloneForDryRun` (mirror `159-160`).
3. `statehash.go`: add `tagEra4LockedIn`/`tagEra4Height`; add both to `stateRootTagsV5`; emit them
   as scalar leaves in `stateRootLeavesV5` (mirror the `epochStart` scalar leaf, `196`).
4. `chainrole.go`: extend the two propose/pre-check mint sites (`chainrole.go:707,863`) so at/above
   H_era4 they call `PopulateEra4Roots` (which stamps v5) instead of `PopulateEra3Roots`. The gate
   becomes: v5 if `MintVersion == 5`, else v4 if `>= 4`, else v2.
5. `modelcheck_state_completeness_test.go`: classify `era4LockedIn`/`era4Height` `committedSet`.
6. A new `ErrEra4VersionRequired` error (mirror `ErrEra3VersionRequired`).

**NOT touched:** `MaxBondRegBytesPerBlock`, `RegCap` (256), the v5 predicate, the RegCap rule, the
spine maps, the leaf marshaller for the spine maps. 4d flips minting; 4c/4b own the rest.

## The activation height is the HUMAN's call

`Era4ActivationHeight` ships as a config field with NO default mainnet value. Any concrete height
in TEST code is a test fixture. The post-latch path (readiness tally) needs no hardcoded height at
all — it derives H_era4 from committed history. A mainnet deployment that wants a genesis-declared
boundary sets `Era4ActivationHeight` out-of-band; that number is **ratifiable, not builder-picked**.
The PR body flags this explicitly. 4d does NOT write a mainnet activation height anywhere in
non-test code.

## Ablation plan — each gate injected → RED → restored (session-7 rule)

All four live in a new `modelcheck_era4_activation_test.go`, mirroring
`modelcheck_era3_activation_test.go`. Each carries its demonstrated-RED inline.

| Gate | Property | Injected defect that reddens it |
|---|---|---|
| **Before activation: v4, era-3 freeze holds** | Below H_era4 the chain mints v4; the v4 committed root is byte-identical to era-3 (spine + era-4 scalars absent from the v4 leaf set). | Emit `tagEra4LockedIn`/`tagEra4Height` (or a spine leaf) in `stateRootLeaves` (the era-3 set) instead of `stateRootLeavesV5` → `TestEra3RootByteIdenticalWithV5KeyspacesPresent` reddens: the v4 root diverges from the era-3 baseline. Its `withoutV5` baseline zeroes the era-4 latch scalars, so the leak reddens whether it reuses a REGISTERED tag OR emits under a FRESH UNREGISTERED tag (an unregistered leak does not cancel across the two baselines). The DISJOINTNESS guard (`TestStateRootCoversExactlyTheCommittedSetFields`) additionally reddens ONLY on a registered-tag reuse — the byte-identical gate is the backstop that also catches the unregistered case. |
| **At/after activation: v5, first v5 block accepted** | At/above H_era4 the chain mints v5; the first v5 block commits the spine keyspaces (dueBucket/qualified/epochStart) + the era-4 scalars and is ACCEPTED (RegCap ≤ 256 enforced). | Make `MintVersion` return v4 at/above H_era4 → the boundary rule (`validateEra4Version`) rejects the minted v4 block with `ErrEra4VersionRequired`; the append fails. |
| **Boundary determinism (no off-by-one)** | The flip is EXACTLY at H_era4: `era4Active(H-1)==false`, `era4Active(H)==true`; a fresh replica replaying the identical history derives the identical H_era4. | Change `era4Active` to `h > c.era4Height` (strict, wrong — era-4 is a `>=` mint boundary) → the at-boundary v5 assertion reddens (the boundary block mints v4 and is rejected). Also: replay-divergence RED via the reorg-stable test (mirror `2c` `TestEra3ActivationIsReorgStableAndReplayDerived2c`). |
| **Cross-activation continuity (4b/4c guards still hold)** | Epoch-boundary / recovery behavior is correct across H_era4: the spine maps stay consistent (`qualified == filter(bonded,slashed,MinBond)`), the Q5 recovery agreement holds, and the byte-identical-vs-era3-replay holds for v4 blocks below the boundary while v5 blocks above commit the spine. | The v5 predicate on the first post-activation block recomputes the spine root; any pre-existing spine drift (a 4b hook defect) surfaces as a StateRoot mismatch on the first v5 block. Demonstrated by a scoped drift injection in the continuity test (skip one `qualifiedMaintain` → the first v5 block's `validateEra4Roots` rejects). |

Plus the standing completeness guards, which redden automatically as the fields are added:
`TestStateFieldsAreClassified`, `TestStateRootCoversExactlyTheCommittedSetFields`,
`TestStateRootEmitsALeafForEveryV5CommittedField` (a new EMIT guard for the v5 scalar leaves,
mirroring the era-3 EMIT guard), `TestAdoptCopiesEveryCommittedField`,
`TestDryRunCloneCopiesEveryAppliedField`.

## Invariants (I1–I5)

Identical to era-3 2c (§Invariants of the 2c doc), one era up:
- **I1/I2/I4:** untouched. No quorum size/threshold/weight-seam change; no sign-ledger change; no
  finality-gate change. The era-4 lock-in READS the frozen `epochSet` weights and applies the SAME
  `>⅔` super-quorum the finality rule uses.
- **I3:** relied on. Lock-in tallies frozen-epoch weight at a rotation — rule change integrates only
  at a finalized boundary, by weight, exactly as I3 mandates. No change to `epochSet`/`rotateEpoch`.
- **I5:** preserved. `era4Active`/`MintVersion`/`validateEra4Version` are pure functions of
  committed state — every honest replica computes the identical verdict and mints the identical
  version.

## STOP boundaries honored

- Implements the CERTIFIED shape only: `regVersion >= 5` supermajority, height-gated, mirroring era-3
  which mirrored #506. No new activation condition invented.
- Does NOT touch the weight-sum seam, `epochSet` freeze / Condition-A freeze, `RegCap`, or the
  proposer byte budget.
- The consensus-rule surface (a new activation gate + version-boundary validity rule) is inside the
  ALREADY-CERTIFIED era-4 design; 4d is the certified 4d increment, not a new rule.

## Verdict: NO deviation. Proceed to build.
