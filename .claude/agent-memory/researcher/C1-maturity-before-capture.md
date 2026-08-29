---
name: c1-maturity-before-capture
description: Consult C-1 verdict — "maturity is reached before capture" is a safe-parameterization (certified net), NOT a theorem; VISION-vs-canon register gap; the cold-start window is the live #183 seam.
metadata:
  type: project
---

# C-1: "maturity is reached before the scaffolding can be captured" (2026-08-27)

**Verdict: GATED — CERTIFIED-as-safe-parameterization, REFUTED-as-theorem.** Filed:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md`

**Why:** the claim is an M0 published-claim gating the #183 red team. It cannot be
published as a theorem — no bound exists, and by the same wall (weak subjectivity is a
SOCIAL trust assumption, not math: Buterin 2014, ethereum.org) that makes every deployed
PoS system's training-wheel retirement a governance act, never a proof (Ethereum/Cosmos/
Bitcoin — silt's own canon names this precedent, m0.md §10 F-1 residual 1).

**How to apply:**
- The canon (`m0.md` §10 "Reachability of maturity" 476–479; `owned-residuals.md` E3;
  `TENETS.md` immutable #3) states it CORRECTLY every time: "No proof; a safe-parameterization,
  not a theorem." VISION.md line 108 states it more strongly ("maturity IS reached before
  capture") — defensible because it says "bet," but one clause short of canon precision. That
  is a documentation residual (R6), held-in-tension, not a blocker. Don't re-open the design.
- What IS certified (shipped + regression-locked): the one-way `everMature` latch is a genuine
  ratchet (monotone `chain.go:2820`, reorg-reconstructed 3189, #572 replay-guard 2590); anchors
  gated on `handedOff()` not live Mature() (`launchAnchor` 998); de-maturation liveness = real-bond
  ≥⅔ super-quorum, no re-arm (2425); F-1 PoC INVERTED + locked (`f1_latch_test.go`
  TestMaturityLatchDoesNotRearmAnchorsOnDemature — both horns HALT + PERMANENT-CENTER unreachable).
- What is NOT closed: the temporal RACE. C1 is a no-DISCOUNT bound, not a no-CAPTURE bound; on a
  young network honest `C_honest` is small so buying a controlling fraction is cheap in ABSOLUTE
  terms, and the adversary's own real bond then TRIPS Mature(). The latch bounds the CONSEQUENCE
  (no permanent center, no re-arm), never the REACHABILITY of pre-maturity acquisition. That is
  the live #183 seam (R1 = brief seam 2). C2 concentration alarm is the only defense there, and
  C2 is held-not-closed by Kwon (operator-counting heuristic w/o TTP).
- **Gate lifts** when someone supplies an explicit (honest-arrival model H, adversary-budget model
  B) parameterization + a crossing-height derivation `T_mature < T_capture(B)` — a CONDITIONAL
  theorem, same shape as C1's "theorem under 3 hypotheses." Achievable, zero code dependency,
  parallel long-lead lane. Produce that → re-certify GATED→CERTIFIED-conditional.
- R2 (handoff-instant 8×/9×MinBond head-count capture) is CLOSED by weight-counting
  (`requireEpochWeightQuorum`, `quorum_weight_test.go`) — disclose to red team as closed, don't
  re-litigate. It's the mature-handoff seam, NOT the cold-start hole.
