---
name: era4-witnessable-transitions
description: era-4 new-era design (trustless witnessable transitions, Option B). Session-12: measurement DONE (boundary FITS 2GB, RegCap ≤ 16,384), Builder rev-2 DONE, Research re-cert round 2 IN FLIGHT.
metadata:
  type: project
---

# era-4 witnessable transitions — the new-era track (Option B, 2026-08-29)

Opened after Andrew ratified Option B ([[witness-floor-box-inc3-refuted]]): make the two whole-map `apply()` transitions
O(payload)/witnessable in a NEW era so the floor box stays fully trustless. origin/main `0984db4`.

## Design + review history
- PACE `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md`: T-3 (TTL → committed due-height BUCKET; "nothing else due"
  = ONE non-membership proof) + E-2 (rotation → committed `qualified`) + O-1 (commit `epochStart`). New `BlockVersion=5`. Load-bearing
  floor: pokt SMT has NO batch/range proof (leaves at `H(key)`).
- PE `RULING-era4-witnessable-transitions-2026-08-29.md` — SHIP-WITH-FIXES.
- Research `era4-witnessable-transitions-EQUIVALENCE-RESEARCH-2026-08-29.md` — GATED (caught the missed `chain.go:2989` maintenance site).
- Builder revision (folded 5 fixes; scoped recovery boundary out).
- Research RE-CERT `era4-witnessable-transitions-RECERT-2026-08-29.md` — STILL GATED (2 items).
- **Builder rev-2 (session-12): folded the RE-CERT Q2 correction + the measured numbers. Re-cert round 2 IN FLIGHT.**

## ★★ RE-CERT (round 1) OUTCOME — GATED. Two items held it; three closed.
**CLOSED by the revision:** (a) E-2 five-site maintenance enumeration (2989/2995/3008/3019/3020) grep-complete, the 2989 I1 fix listed
correctly; (b) canonical id-list pin forecloses MTH malleability; (c) O-1 commit `epochStart` CERTIFIED narrowly + recovery boundary
cleanly scoped out.

**★ ITEM 1 — REFUTED (load-bearing): the PE's POINTER-ADVANCE fix is UNSOUND.** Collapsing `qualified`+`epochSet` into ONE shared
live-mutating keyspace breaks frozen-epoch-set safety. Verified: `epochSet` is a SEPARATE committed keyspace, FROZEN mid-epoch (assigned
only at `rotateEpoch` `chain.go:3131` + `adopt` `:3546`); mid-epoch quorum reads the FROZEN set (`requireEpochWeightQuorum` `:2597`,
`RoundCatchupMet` `:2631`); the code calls a mid-epoch set change "I3 churning-set unsoundness ... impossible" (`:1239-1241`). CORRECT
DIRECTION (RE-CERT direction b): keep `epochSet` a FROZEN materialized keyspace; add a live `qualified` accelerator; the boundary COPIES
qualified→epochSet and is a DISTINCT, HEAVIER witness class = O(boundary-delta). (Structural tension worked: PE proposed, Research refuted.)

**★ ITEM 2 — the reg-cap VALUE.** Cap classification CERTIFIED (new validity rule; #506 first-reg exemption `chain.go:1587`;
`MaxBondRegBytesPerBlock` proposer-only `node.go:270`/`chainrole.go:798`). The NUMBER was "un-derivable at desk" per round 1 — session-12
measurement now brackets it (below); the OPEN question is whether the desk upper-bound closes it or a run is still needed.

## ★★★ THE HONEST META-SIGNAL — MEASURED: the boundary FITS
Option B keeps discovering epoch/weight state is EPOCH-SCOPED → witnessing it has an epoch-scoped cost. Whole-map recompute → O(registry)
→ Option B. The epoch boundary → O(boundary-delta), a HEAVIER witness class. **SESSION-12 MEASUREMENT SETTLED IT: the boundary FITS the
2 GB box.** SMT proofs are tiny (max 1,474 B at 1M leaves << 16 KiB SProofMax); the COUNT of proofs is the constraint.
`boundary_witness ≤ RegCap × EpochBlocks × SProofMax ≤ 2 GB` ⟹ **`RegCap ≤ 16,384 ids/block`** (EpochBlocks=8). No feasibility cliff.
Honest cohorts sit far below 16,384 (the 2 MiB proposer byte budget alone ceilings honest per-block regs).

## SESSION-12 PROGRESS — measurement DONE, Builder rev-2 DONE, re-cert round 2 IN FLIGHT
1. ✅ Tester MEASUREMENT (local, no billable): proof sizes + the RegCap bracket `λ_H ≤ RegCap ≤ 16,384`. λ_H (honest arrival rate) NOT
   pinned in canon (`m0.md:487`, `owned-residuals.md:392`; cancels out of maturity theorem `decisions.md:662`) — but likely UPPER-boundable
   at desk via the proposer byte budget. Tester call: do NOT burn a billable run on λ_H; the R1 correction is the first gate.
2. ✅ Builder rev-2 of the design doc: E-2 now = frozen materialized `tagEpochSet` + live `tagQualified` accelerator (direction b); boundary
   = COPY `epochSet := qualified`, stated as a distinct O(boundary-delta) witness class with the measured bound; two keyspaces settled;
   §1/§5/§7/§9/§10/§11 updated. Direction (a)-alone REJECTED (reintroduces O(registry) scan). Doc-only, no code (build-process #6).
3. ⏳ Research RE-CERT round 2 IN FLIGHT (blind): Q1 = is corrected E-2 sound (I1/I3 frozen-set immutability)? Q2 = can RegCap be pinned at
   DESK (λ_H upper-bound via proposer budget) or does it need a measured run? Q3 = lift GATED→CERTIFIED (guards + T-3 replay as build-time
   obligations, R2 recovery-boundary scoped out)?
4. ON CERTIFIED → **VETO GATE to Andrew:** BlockVersion=5 + new tags (`tagDueBucket`/`tagQualified`/`tagEpochStart`) + two-keyspace layout +
   the RegCap validity rule + its value. If Q2 says "needs measured λ_H" → escalate the billable-run decision to Andrew separately.

## Still-owed / decisions for Andrew
- Recovery-boundary DIRECTION (commit `LivenessRecoveryHeight` = trustless, more scope; vs posture-bound) — his call, separable follow-on (R2).
- era-3 residual: `versionSupported` admits v3 = silent mis-validation (prior era-3 ruling) — out of era-4 scope, open elsewhere.
- OPS: the Researcher seat has NO Bash → can't `git fetch`; it verifies the `0984db4` tree by artifact-presence (acceptable, disclosed).

## Path
PACE→PE→Research→revision→RE-CERT (GATED)→**measurement (DONE: boundary fits, RegCap≤16,384)→Builder rev-2 (DONE)→re-cert round 2 (IN
FLIGHT)→VETO GATE (BlockVersion=5 + cap rule + boundary witness class)→2a→2b→2c build behind BOTH drift-guards→new-era FREEZE.** Multi-session.

Related: [[witness-floor-box-inc3-refuted]], [[witness-floor-box-track]], [[session-resume]], [[keystone-era3-freeze-sequencing]].
