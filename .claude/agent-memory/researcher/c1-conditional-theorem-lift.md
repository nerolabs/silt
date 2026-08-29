---
name: c1-conditional-theorem-lift
description: C-1 lift SUCCEEDS — "maturity precedes capture" is now a CONDITIONAL theorem (CT-1) under H,B,P; the arrival rate CANCELS, leaving crossing condition W_A < 2·w_min·M_req; new seam R6.
metadata:
  type: project
---

# C-1 lift: conditional crossing-height theorem (2026-08-27)

**Verdict: CERTIFIED-CONDITIONAL** (was GATED). Filed:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`
Ratification pending (human); one owed measurement (`λ_H` at launch).

**The lift (Theorem CT-1):** under honest-arrival floor H (`λ_H>0`, MEASURED), adversary-budget
cap B (`W_A`, DECLARED), parameter constraint P2 (`M_req > W_A/(2·w_min)`), maturity provably
precedes capture. **The decisive move: the honest arrival RATE `λ_H` CANCELS** — both T_mature and
honest weight at that height scale with λ_H — leaving a pure budget-vs-threshold inequality:
**`W_A < 2·w_min·M_req` (⅔-capture-safe), `W_A < ½·w_min·M_req` (⅓-stall-safe).** That converts
"assume a specific growth rate" (fragile parameterization) into "assume ANY positive floor"
(hypothesis of C1's class). This is what makes it a THEOREM not a re-labeled bet.

**Binds the SHIPPED trigger:** shed fires at `min(NakamotoOperators,NakamotoDomains) ≥ MatureValidators`
over the committed ledger (`chain.go:1819–1840`), latched one-way (`chain.go:2820`). Verified this round.

**Why still conditional (the WS wall, unremoved):** `λ_H` and `W_A` are UNVERIFIABLE from genesis.
silt cannot prove honest operators arrive; if λ_H→0 maturity never comes (anchors correctly stay
armed — theorem is vacuous, not violated). W_A too-low = certified vs the wrong adversary. Same form
every deployed PoS training-wheel retirement takes; unconditional theorem does not exist for anyone.

**Literature schema adopted (verified vs source):** Barrué/Piatek arXiv:2404.09627 Theorem 3 =
arrival-CDF → finite crossing time = the T_mature schema; it does NOT supply a budget-crossing
condition (the gap CT-1 fills). Buterin 2014 WS = social assumption. arXiv:2310.01546 =
young-network-cheapest-to-capture (why T_capture is EARLY).

**NEW sharpest #183 seam — R6 (H⊥B independence break, OPEN):** B's staged bonds count in BOTH the
honest-arrival floor `A(t)` (inflating apparent λ_H) AND the capture weight `w_A`. The arrival process
and the budget are NOT independent. This is why λ_H must be measured as ADDRESS-DIVERSE arrival and why
the A-axis declaration-cheap degradation (P3) matters most here. NOT closed.

**R1 re-priced:** the cold-start window is now the falsifiable inequality `W_A < 2·w_min·M_req` — red
team's job is sharp: falsify it for a plausibly-parameterized launch. R5 (adaptive param) target is now
nameable: region `W_A ≥ 2·w_min·M_req`. R2/R3 unchanged (closed). See [[c1-maturity-before-capture]].

**Owed to instantiate:** MEASURE `λ_H` (operator/domain-distinct bonds per height, from the shipped
`C2Metric` over `c.bonded` — instrumentation exists, recording the floor is owed); DECLARE `W_A`
(owner's call); set `M_req`,`w_min` to satisfy P2; add an arrival-rate-floor alarm (theorem-domain exit).
