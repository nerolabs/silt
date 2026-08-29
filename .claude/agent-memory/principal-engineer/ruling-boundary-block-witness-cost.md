---
name: ruling-boundary-block-witness-cost
description: Boundary-block witness cost for the semi-stateless witness floor box — both branches O(registry), not bounded-able as self-recompute, posture decision needed
metadata:
  type: project
---

# Ruling: boundary-block witness cost (2026-08-29, at origin/main 0984db4)

Verdict: both `apply()` branches are **O(registry)**, NOT O(payload). Not
bounded-able as a self-recompute. The floor box needs a **posture decision**
(accept attested root under a bounded check), which touches NEITHER the frozen
era-3 format NOR I1-I5.

**Why:** the era-3 post-state root recompute (`postApplyRoots`,
`era3validity.go:118-130`) calls `StateRoot()` -> `stateRootLeaves()`
(`statehash.go:70-128`), which rebuilds the SMT by iterating the WHOLE
committed maps. A root-only box cannot reproduce the root without every leaf
pre-image.
- TTL sweep `chain.go:3005-3013`: bare `for id,regH := range c.bondRegHeight`
  over the WHOLE map, EVERY block whenever `BondTTLBlocks>0`. No expiry heap
  (verified grep). O(registry), and NOT confined to boundaries.
- rotateEpoch `chain.go:3124-3181` via liveQualifiedSet `chain.go:1198-1210`:
  whole `bonded` map scan at each `h%EpochBlocks==0` boundary. O(registry).
- All 6 maps (bonded/slashed/epochSet/bondRegHeight/regVersion/bondDomain) are
  under the SMT (18-field committedSet, `statehash.go:34-45,70-128`), so their
  reads ARE the witness set.
- At C_block = |read-set|*16KiB, 2GB/16KiB ~= 128K leaves. Past ~128K
  validators an HONEST boundary witness exceeds the box's own DoS cap.

**Missed coupling (highest value):** under TTL the O(registry) cost is EVERY
block, not just boundaries. So the "self-recompute the ordinary block" story
only holds if `Config.BondTTLBlocks==0` in the era-3 production config — a
config fact NOT settled in code, load-bearing for the whole floor-box thesis.
Pin it before the direction decision.

**How to apply:** cleanest sound path is Option A — accept the attester-signed
StateRoot at boundary/TTL heights under a bounded check (finality QC verifies
as a threshold sig, payload-tied leaf deltas verify O(payload)), self-recompute
only where bounded. This matches the #600 validate-by-proof direction.
- Research-gated: the SOUNDNESS of the bounded check (can a Byzantine >2/3
  super-quorum sign a root whose whole-map TTL/rotation transition is falsified
  while every payload-delta proof still verifies?). I shaped the question, did
  not certify.
- The one call for Andrew: Option A (accept-attested-under-check) vs Option B
  (self-recompute-only, refuse above the ceiling). Trust-model/scope trade tied
  to [[ruling-witness-floor-box-mechanism]] and #600. I recommend A, gated on
  the research question, TTL-config pinned first.

Full ruling:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-boundary-block-witness-cost-2026-08-29.md`
Related: [[ruling-witness-floor-box-mechanism]], [[ruling-era3-committed-state-root-format]],
[[ruling-r4a-witness-accessor-spine]], [[ruling-r3-witness-bound-review]].
