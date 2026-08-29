---
name: vision-research-parallel-lane
description: The VISION.md research lane — C-7 CERTIFIED, C-1→CERTIFIED-CONDITIONAL (CT-1), C-5 GATED, all Andrew-RATIFIED 2026-08-27
metadata:
  type: project
---

Andrew asked to get ahead on VISION.md's unresearched claims as a parallel path (2026-08-27).
Triage first (see researcher `vision-research-backlog-triage.md`), then a scoped lane. All five
suspected-(C) candidates were already tier (B) — don't re-open (A)/(B).

## Ratified research verdicts (all Andrew-ratified 2026-08-27)

**C-7 — witness-based floor-box validation: CERTIFIED.** Sound + complete for the set-valued state;
needs BOTH membership + non-membership proofs (SMT serves both); withheld witness → STALL, never
accept. Soundness no longer blocks #600. Cert: `…/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`.

**C-1 — "maturity before capture": GATED → CERTIFIED-CONDITIONAL (CT-1 lift).** Now a theorem under
three hypotheses (H honest-arrival floor, B adversary budget, P param constraint). The decisive move:
T_mature and honest weight both scale with arrival rate λ_H, so the rate CANCELS from the order —
leaving the falsifiable inequality **`W_A < 2·w_min·M_req`** (⅔-capture-safe). Operator dials:
MatureValidators + MinBond. Not unconditional (WS wall). Owed: the **λ_H measurement** (address-diverse
bonded arrival/height via shipped C2Metric + floor-exit alarm) — OPENED as an instrumentation lane.
NEW sharpest residual **R6: H⊥B independence break** (adversary's staged bonds count in both the arrival
floor AND capture weight → λ_H must be address-diverse). Certs:
`…/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md` + `…/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`.

**C-5 — composed operator-economics: GATED (ratified).** CLOSED: firewall (γ→1/N intact),
conservation (no money-pump), no-price-out. GATED: **G1** VISION line 54 is FACTUALLY WRONG — the repair
bounty pays the new HOLDER of the rebuilt shard, not the reconstructor (paid zero); **G2** floor-box
reconstruction RAM unmeasured at prod chunk (bundle with coexistence test); **G3** repair self-funds HOT
objects only. Cert: `…/C5-honest-operator-economics-composition-RESEARCH-CERTIFICATION-2026-08-27.md`.

## The era-3 format freeze obligation (C-7 Q3, ratified in PR #605)
era-3 `Block` must commit BOTH the state root AND the transparency-log root, over the
completeness/order-independence-proven field set, + verifier invariant "no witness → never accept
(stall)". Block commits NEITHER root today (`chain.go:311-390` verified). Gates:
#603-weight (epochSet PROVEN via #606; bonded-weight + spent/slashed still owed) + #600 (Andrew's
scope) + C-7 (certified). See [[keystone-era3-freeze-sequencing]].

## Execution status (2026-08-27)
- **#605 MERGED** (main af446d6) — C-7 + C-1(GATED) recorded. VISION honesty pass IN FLIGHT (Builder)
  supersedes the C-1 entry to CONDITIONAL + folds C-5 G1/G3 + C-2/C-3 + R6 — see [[vision-drift-honesty-pass]].
- **λ_H instrumentation** — lane OPENED (Builder).
- Parked: C-5 gate-lift, coexistence+G2 (billable, Andrew's go + #600 scope owed).
