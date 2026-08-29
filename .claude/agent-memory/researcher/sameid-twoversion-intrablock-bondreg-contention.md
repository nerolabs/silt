---
name: sameid-twoversion-intrablock-bondreg-contention
description: CERTIFIED (order-independence axis, lifted from GATED 2026-08-28) — PR #622 canonicalBondRegs/bondRegLess folds same-id BondRegs to a total-order winner; R2+R3 closed; residuals R4-LOO (completeness) + R6-MIXED (same-id multi-root, shown-safe).
metadata:
  type: project
---

Sibling of [[same-root-intrablock-bondreg-contention]] (#618). Certified GATED 2026-08-28,
LIFTED to CERTIFIED (order-independence axis) same day via PR #622.

**The finding (was CERTIFIED-real):** a block with two BondRegs for the SAME id on its own
root, distinct Version/Domain/Size, committed order-dependent `regVersion`/`bondDomain`/`bonded`
via last-writer-wins (chain.go:2840-2843). `regVersion` feeds the rotateEpoch #506 lock-in
tally (2922-2934), so `gateLockedIn`/`gateHeight` inherit the split when the two-version
validator is the >⅔ swing. DISTINCT from #618 (distinct-id, no legit form → REJECT); this HAS
a legit form (F1 renew/RESIZE), so REJECT was the wrong tool — the mirror-#618
seenReg-unconditional fix was REFUTED (breaks TestSameRootSameIDRenewAdmitted).

**The fix (CERTIFIED), PR #622 commit 80ae77e, branch worktree-agent-afbcf8aa5246851d9:**
`canonicalBondRegs(b.BondRegs)` at the apply() bond loop folds same-id regs to ONE winner by
`bondRegLess` = strict total order **largest Size, then Version, then Domain, then Sig-bytes**
(replace incumbent when bondRegLess(incumbent,r); MAX wins). Winner-takes-all-fields (one whole
BondReg → the loop sets bonded/regVersion/bondDomain from the same r). Sig tie-break makes it a
pure function of block content. Matches the certified direction exactly.

**Why scope is clean (distinct-id UNCHANGED):** the fold is a no-op on any block with no same-id
duplicate (first-appearance order preserved, no reg dropped/reordered). Genesis premise holds
(TestGenesisSameRootApplyIsOrderDependent uses distinct ids keyA/keyB → no fold; prod genesis
carries no BondRegs). F1 negative control TestSameRootSameIDRenewAdmitted keeps `bonded==2*minBond`
with NO change (larger reg wins in both orders — resize now order-monotone, not slice-accidental).

**Verification method:** commit not checked out in any local worktree; read canonicalBondRegs/
bondRegLess + test source VERBATIM from the commit patch on origin, cross-checked every invariant
against main-tree code it must preserve. WebFetch summary was a lead, not the fact.

**R2 CLOSED (both axes):** `TestRegVersionIntraBlockOrderIndependent` (same-id, same root rootV,
distinct Size/Ver/Dom, asserts byte-identical committed + fixed winner bonded==2*twoMiB/
regVersion==BlockVersionRegGate/bondDomain==0x22 — ablation-shaped). `TestGateLockInSwingIsOrderIndependent`
+ `gateSwingOrderings` (3 equal-weight, swing validator two regs on rootX v2+v3, exercises >⅔
lock-in tally, vacuity trip-wire matureEpoch+gateLockedIn must fire, DeepEqual across orders).
gateLockedIn/gateHeight moved out of orderVacuous with real fixtures.
**R3 CLOSED:** winner rule named + implemented + shipped through the gate.

**Residuals forward (both NON-blocking for the order-indep axis):**
- **R4-LOO (open, low):** leave-one-out/completeness axis for the 4 fields is a SEPARATE probe
  on the completeness ratchet — still owed before the era-3 freeze. This addendum certifies the
  ORDER-INDEPENDENCE axis only.
- **R6-MIXED (shown-safe by analysis):** same-id can sign two regs on DIFFERENT roots (admissible
  pre-gate); fold picks largest-Size winner, dropping the other root's claim. bondRootOwner stays
  order-independent: fold winner is content-deterministic, AND the only contender for a dropped
  root is a distinct-id same-root reg = REJECTED by #618 seenRoot (ErrSharedRootInBlock,
  unconditional). A dedicated fixture would make it probe-proven (recommended, not required).

Cert: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/sameid-twoversion-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`
Addendum (lift): `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/sameid-twoversion-intrablock-bondreg-contention-CERT-ADDENDUM-2026-08-28.md`
