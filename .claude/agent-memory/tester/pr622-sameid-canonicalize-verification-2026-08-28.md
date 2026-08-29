---
name: pr622-sameid-canonicalize-verification-2026-08-28
description: PR #622 canonicalBondRegs fix verified 2026-08-28: all 3 ablations RED, negative control sensitive, genesis premise preserved, full core/chain+sim -race PASS
metadata:
  type: project
---

PR #622 — fix(chain): CONSENSUS-RULE — canonicalize same-id intra-block bond regs in apply()
Commit: 80ae77e. Branch: worktree-agent-afbcf8aa5246851d9. Verified 2026-08-28.

## Run sequence

**Baseline:** `go test ./core/chain/... -race` PASS (10.5s). `go test ./sim/... -race` PASS (295s).

**1. TestRegVersionIntraBlockOrderIndependent (covering probe)**
- As shipped (80ae77e): PASS — log: "same-id two-version intra-block reg is order-INDEPENDENT: both orderings commit regVersion=3 bondDomain=0x22 bonded=4194304 (the largest-Size winner)"
- Ablation (canonicalBondRegs → b.BondRegs): FAIL — log: "regVersion DIVERGED across intra-block orderings (2 vs 3) — the canonicalBondRegs fold in apply() is not order-independent."
- Reverted. Verified GREEN after revert.

**2. TestGateLockInSwingIsOrderIndependent (gate-swing trip-wire)**
- As shipped: PASS — log: "the #506 gate locked in identically across two opposite intra-block orderings (gateLockedIn=true gateHeight=4) with the same-id two-version validator as the ⅔ swing"
- Ablation (canonicalBondRegs → b.BondRegs): FAIL — log: "gateLockedIn is FALSE — the #506 lock-in tally did not fire (the swing validator's version did not clear the >⅔ ready-weight bar)"
- Reverted. Verified GREEN after revert.

**3. TestSameRootSameIDRenewAdmitted (negative control)**
- As shipped: PASS (bonded=4194304=minBond*2, the larger reg wins in both orderings)
- Winner-logic injection (inverted Size comparison: `a.Size > b.Size` instead of `<`): FAIL — log: "resize did not take: bonded=2097152 want 4194304" — test is NOT passing vacuously; it catches wrong winner selection
- Reverted. Verified GREEN after revert.

**4. TestGenesisSameRootApplyIsOrderDependent (genesis premise preserved — #619 tripwire)**
- PASS — log: "premise pinned: genesis apply() owner is slice-order-dependent (A-first→1a3675ec81f5, B-first→035cc4a43629) — safe only by genesis byte-identity"
- The fold (same-ID only) did NOT silently make distinct-id genesis cases order-independent. Premise intact.

**Final clean green:** `go test ./core/chain/... -race` PASS (28.2s). Working tree clean (only .claude/ untracked).

## Result: PROMOTED

All 6 verification steps pass. No anomalies. Scoped correctly — same-ID fold only, distinct-ID genesis case unchanged.
