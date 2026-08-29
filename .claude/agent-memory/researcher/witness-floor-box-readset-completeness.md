---
name: witness-floor-box-readset-completeness
description: REFUTED/GATED 2026-08-29 — increment-3 six-transition read-set is INCOMPLETE for v4 blocks; v4 read-set = full apply() touch-set (= cloneForDryRun copy-set); quorum-stack committed reads MUST be witnessed.
metadata:
  type: project
---

**Verdict (increment-3 Part A read-set, floor box):** Q1 completeness **REFUTED**, Q2 sound-closure **GATED**. Filed:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/witness-floor-box-readset-completeness-RESEARCH-CERTIFICATION-2026-08-29.md`

**The decisive artifact / correction:** The Builder doc (`docs/thinking/2026-08-29-witness-floor-box-delivery-increment3-options.md`) enumerated a per-transition six-row read-set and half-found the "apply() also reads" trap — then STOPPED at the 3 bond-reg displacement reads (slashed:2977, bondRootOwner:2980, bondRootProven:2986). It missed that for a **v4 block**, `validateEra3Roots` (era3validity.go:88) recomputes the post-state root via `postApplyRoots`→real `apply()` on a clone, so acceptance reads EVERY committed key apply() touches. Four omitted apply branches: TTL-expiry sweep (chain.go:3005, iterates bondRegHeight/bonded/regVersion as MAPS), validatorsSeen population (:3024 via attesterQualified → slashed/bonded/epochSet), maturity latch (:3034 Mature→C2Metric → validatorsSeen/bonded/bondDomain/slashed), and **rotateEpoch** (:3046, rewrites epochSet from liveQualifiedSet = full bonded/slashed read + reads everMature/regVersion/gateLockedIn/era3LockedIn).

**The sound v4 read-set is already in-tree:** `cloneForDryRun` (era3validity.go:142) deep-copies all 12 committed maps + 6 scalars = the 18 stateRootLeaves tags. That copy-set IS the v4 read-set. Its own drift-guard comment (:137) names the under-demand bug from the writer's side.

**Q2 sound closure = NO for v4.** Transition-validity read-set is NOT a sound closure. Quorum-stack committed reads (bonded/slashed/epochSet) enter acceptance via the root recompute and MUST be witnessed — omitting them lets a proposer commit a wrong consensus-WEIGHT leaf (the era3validity.go:19-23 safety attack). Doc conflated this (closable with roots today, mandatory) with the genuine #535 `epochStart`/`effectiveEpochSet` observable gap (no committed root, a bounded posture residual). Different problems; gap 2 does not excuse omission 1.

**The one place the doc is right:** era-2 (v2/v3) blocks — validateEra3Roots is a no-op, so acceptance reads only the validity predicates and the six-transition table holds. But the floor box is an era-3 artifact; its acceptance path IS the v4 path. See [[c7-witness-floor-box-validation]], [[era3-committed-state-root-format]], [[600-floor-box-direction-post-coexistence]], [[witness-floor-box-dos-bound]].

**Lift to CERTIFIED needs:** (1) re-scope Part A to the apply() committed touch-set (recommend recording-derivation A3, not the hand-written A2 table — apply's map-iterating TTL/rotate branches have no static per-transition key list); (2) recording drift-guard ablated, corpus MUST include a TTL-expiry block, a maturity-latch-tripping block, and an epoch-BOUNDARY block incl. young→mature handoff (else green-covering-nothing, session-7 era-boundary scar); (3) explicit R2 statement — box does not finalize a LivenessRecoveryHeight boundary on roots alone.

Residuals: R1 v4 read-set = full apply touch-set (OPEN, build target); R2 #535 epochStart observable gap (OPEN, bounded); R3 LogRoot verified by RFC-6962 shape not SMT witness (HELD); R4 increment-2 ReadEntry single-QueryKind can't model `map[k],ok` (CONFIRMED real, orthogonal).
