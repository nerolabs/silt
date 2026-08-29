---
name: same-root-intrablock-bondreg-contention
description: CERTIFIED (lifted from GATED 2026-08-28) — same-root distinct-ID BondReg collision was order-dependent in apply(); validity-layer per-root dedup (seenRoot / ErrSharedRootInBlock) shipped PR #618; R1+R2 closed; bonded/bondRootOwner/bondRootProven order-independent for admissible blocks.
metadata:
  type: project
---

# Same-root intra-block bond-registration contention (era-3 freeze gate)

**Verdict: CERTIFIED** (lifted from GATED 2026-08-28). Addendum:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/same-root-intrablock-bondreg-contention-ADDENDUM-R2-CLOSE-2026-08-28.md`
Original (GATED) cert:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/same-root-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`
Routed by PE ruling #618 (`RULING-618-bond-registration-order-independence-2026-08-28.md`).

**The finding (hand-traced at line level, pure map algebra — verifier-independent):**
`apply()` chain.go:2769-2799. TWO distinct proven BondRegs (id A, id B) on the SAME
root in ONE block → slice order [A,B] gives owner=A/bonded={A}; [B,A] gives
owner=B/bonded={B}. The `!(proven && !bondRootProven[root])` guard at 2780-2790 makes
whoever is FIRST in slice order win. Old validity layer had NO per-root dedup —
`seenReg` is per-ValidatorID and gate-gated. So the block was admissible and `apply()`
resolved it order-dependently → different SMT state root from identical block contents =
latent fork if frozen.

**Why the crypto does NOT close it (the subtle part):** the REAL verifier
(`bond.go:VerifySpaceTime`→`verifyLabels` :486) key-binds the root via
`plotSeedN(pk,n)` — one valid proven claim per root, so two HONEST holders can't race.
BUT (1) the model-check oracle that DISCHARGES the freeze runs `objectiveVerify` (a
stub accepting any pk/root), under which the case IS reachable; and (2) #508 proposer
filtering is POLICY not validity (chainrole.go:768 says so) — a Byzantine proposer builds
the block regardless. Unreachability held ONLY for the honest threat model AND only under
the real verifier — the freeze was proven in neither regime, so the fix was required.

**LIFT — fix (a) shipped as certified (PR #618, branch
order-independence-bond-registration-family, head 4c10525). Verified at line level:**
`seenRoot map[ports.Hash]ports.NodeID` in `validateBondRegs` (chain.go:1482-1488), rejects
distinct-ID same-root in one block with `ErrSharedRootInBlock` (sentinel @778). Runs
UNCONDITIONALLY (sits OUTSIDE `if gate`; gate @1463; closes the pre-gate seam). Fires only
on `prev != id` → same-ID renew/resize ADMITTED (F1). apply() UNTOUCHED — this is the (a)
validity tightening, NOT the (b) apply() tie-break (which would be a new consensus rule).
Call path: Append→ValidateCommit (validateBondRegs @2199)→apply only on nil (2612-2618), so
a rejected block never commits. Types match (Root/bondRootOwner/seenRoot all ports.Hash-keyed).

**Covering probe (`redteam_verify_sameroot-intrablock_test.go`) — closes R2:** two
distinct-ID PROVEN claims (Answer="valid", stub-accepted) on one root; BOTH orderings
rejected with ErrSharedRootInBlock + `bonded` empty in both chains (no divergent commit) +
same-ID-renew negative control ADMITTED. Regime correct: no RegGateActivationHeight so
regGateActive=false (proves the unconditional caveat). Ablation RED-then-GREEN recorded in
thinking doc 2026-08-28-order-independence-bond-registration-family.md (guard ablated →
admitted → owner/bonded diverge by slice order → revert green) = the execution proof (R1).

**Residuals now:** R1 (execution proof) CLOSED (the ablation RED). R2 (covering probe)
CLOSED. R3 (stub-vs-real verifier, standing/broader) OPEN unchanged — oracle proves under
the stub, a superset of reachable inputs; safe for a soundness gate, but any liveness/cost
claim off the same oracle inherits the OPPOSITE bias. R4 (snapshotBoot carried maps by
REFERENCE) fixed as a test-helper deep-copy (no research gate). N1 (new, minor): this lift
certifies the ORDER-INDEPENDENCE axis ONLY; snapshot-equivalence two-list-union
freeze-readiness for these 3 fields not re-certified here (keystone certs + #617/#618 debt
govern it).

**Corrected (from prior verdict):** GATE lifted — its single lifting condition (covering
probe, RED-then-GREEN, two distinct-ID same-root proven, both orderings) is met exactly.
Also confirmed the fix took (a) not (b) at the line level. See
[[c7-witness-floor-box-validation]] for the era-3 freeze context and
[[c1-conditional-theorem-lift]] for the same key-binding labeling soundness.
