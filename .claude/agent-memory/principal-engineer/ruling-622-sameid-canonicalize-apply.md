---
name: ruling-622-sameid-canonicalize-apply
description: PR #622 SHIP — canonicalBondRegs fold in apply() is a genuine total order; I ablated RED, verified distinct-id identity, found the same-id-two-DIFFERENT-root ownership side-effect (safe, order-fixing)
metadata:
  type: project
---

# Ruling: #622 same-id intra-block bond-reg canonicalization — SHIP

Verdict: **SHIP.** The certified fix for the same-id two-version intra-block fork.
`apply()` folds `b.BondRegs` through `canonicalBondRegs` (chain.go:2828) before the bond loop;
one winner per id by a total order (Size, Version, Domain, Sig), all six same-id-slot writes
(`bondRootOwner`, `bondRootProven`, `bonded`, `bondRegHeight`, `regVersion`, `bondDomain`)
take from the ONE winner `r`.

**Why:** matches the cert (`sameid-twoversion...-2026-08-28`) §3(a1) exactly — canonicalize not reject,
one winner takes all fields, covers regVersion+bondDomain+bonded, legal resize stays admitted.

## Premises I verified myself (worktree agent-a616a5675fd76f854 @ 80ae77e)
- Total order is genuine: Size int64, Version uint8, Domain uint64, Sig []byte via bytes.Compare
  (chain.go:417-452, bondRegLess). Sig binds root+size+nonce+domain+version (signingBytes:457).
  So a full tie on Size/Version/Domain ⇒ same signed message ⇒ if Sig also ties the regs are
  BYTE-IDENTICAL (identical writes, order irrelevant). NO residual order-dependence.
- Ablation RED (I ran it): removing the fold flips regVersion 2↔3, gate fails to lock in one
  ordering, TestCommittedSetFieldsAreOrderIndependent reports gateLockedIn/gateHeight DIFFER.
  Green is EARNED, not vacuous.
- Distinct-id is IDENTITY: canonicalBondRegs on all-distinct ids returns input unchanged,
  same order (I ran it). TestGenesisSameRootApplyIsOrderDependent stays green — distinct-id
  order-dependence (residual R-G) preserved, not silently fixed. Builder's own doc records
  they hit the id-sort trap and corrected to first-appearance order.
- Only apply-path iterator that writes committed state is the folded one (2828). Other
  `.BondRegs` uses are validation (1445/1483, no commit), digest (523/560-576, order-preserving),
  empty-checks, proposer fold (chainrole, builds not applies).

## The coupling the consult framing missed (the crux I found)
Same-id, TWO DIFFERENT roots in one block: pre-fold the id claimed BOTH roots
(bondRootOwner[A]=bondRootOwner[B]=id) with an ORDER-DEPENDENT bonded (4194304 vs 2097152).
Post-fold the id claims ONLY the winner's root (B), bonded order-INDEPENDENT.
So the fold DROPS a root-ownership write that the old loop made. Verdict: SAFE and correct —
(1) old bonded was the fork the cert targets, now fixed; (2) F1 is "one plot, one standing," a
single id squatting two roots gained nothing legitimate; (3) new outcome is order-independent;
(4) honest proposer emits ≤1 reg/id (chainrole:748 + embedded dedup), so this is Byzantine/genesis
only; (5) at height>0 validateBondRegs rejects below-floor regs before the fold, so winner
selection can't be corrupted. NOT a distinct-id change (the fold is identity on distinct ids).

## Residuals (all non-blocking)
- Stale docs in the PR: `docs/thinking/2026-08-28-orderVacuous-506-gate.md` is the OLD
  "report-don't-fix" deliberation; the correct one is the -sameid-twoversion- file. PR body text
  is also the stale finding-era text. Docs only; code is the fix.
- gateLockedIn/gateHeight clear the order-independence list only; still owed a leave-one-out
  (verdict-flip) probe on probeUncovered — correctly disclosed, not gold-plated.
- bondDomain floor edge and displacement-branch interaction reasoned closed (below-floor can't
  win largest-Size; winner selection is per-id content-pure, independent of other ids/slice).
