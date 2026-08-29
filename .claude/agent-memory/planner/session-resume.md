---
name: session-resume
description: ▶▶ RESUME HERE (session-13 END) — RegCap DONE+MERGED (#635 @ 5ea1401, N=256 total-count count-cap) AND 4a schema/tag reservation MERGED (#637 @ main effe115). NEXT SCOPED GOAL: build 4b (maintenance spine) — the two sharpest hazards come due. Memory pending a periodic PR.
metadata:
  type: reference
---

# ▶▶ RESUME HERE — session-13 (2026-08-29)

**origin/main HEAD:** `effe115` (#637 / 4a merged). Lineage this session: `0076337` → `5ea1401` (#635 RegCap) → `effe115` (#637 4a). ALWAYS `git fetch` at session start.
Era-3 committed state-root format FROZEN (immutable, #632). Frozen consensus formats amendable only by a NEW ERA (new `BlockVersion`).
⚠ SESSION-13 seat memory is UNCOMMITTED in the working tree — Andrew chose a PERIODIC memory PR ([[feedback-periodic-memory-pr]]); batch it at this milestone.

## ★★★ SESSION-13 SHIPPED: RegCap correction (#635) + era-4 step 4a (#637) — both on main, verified
- **#635 @ `5ea1401`:** the RegCap correction on record. RULE = per-block TOTAL BondReg count (fresh+renewal, after `canonicalBondRegs`), v5 validity
  on receipt. INSTRUMENT = COUNT cap (not byte — PE decisive: byte cap inflates witness ~13–18× as evolving knob k drops). VALUE = **N=256**
  (floor 18 @ k=1, ceiling 16,384; ~363 MiB worst block Sybil-cost-bounded; witness 32 MiB fits 2 GiB box 64×). Ablation SPEC = ">256 TOTAL any-mix
  REJECT; ≤256 ACCEPT". Re-derivation gate = all 7 determinants (B, k, Samples, BlockSize, BondVDFDelay, MinBond, proof scheme) at next mint.
  Full lineage: recert → instrument ruling → value-derivation (GATED on doc correction) → Builder wrote canon → PE conformance CONFIRM → merged. Certs:
  `.../research-outcome/era4-regcap-recert-VERDICT-2026-08-29.md`, `.../principle-engineer/RULING-era4-regcap-instrument-A-vs-B-2026-08-29.md`,
  `.../research-outcome/era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`, `.../principle-engineer/CONFIRM-era4-regcap-record-conformance-2026-08-29.md`.
- **#637 @ `effe115`:** era-4 step 4a. Reserves `BlockVersionWitnessable=5` (chain.go:357) + inert tag STRINGS `tagDueBucket`/`tagQualified`/`tagEpochStart`
  (statehash.go:68-70), inert-by-guard (NOT in stateRootTags, not classified, no leaf, no versionSupported widen). PE SHIP-WITH-FIXES + Tester PROMOTED
  (`.../principle-engineer/RULING-era4-4a-schema-reservation-2026-08-29.md`). CI 8/8 green.

## ★ NEXT SCOPED GOAL: build 4b — the maintenance spine (NOT yet started; needs PACE → build → blind PE+Tester)
4b maintains the LIVE `tagQualified` keyspace at ALL FIVE bonded/slashed sites (`chain.go` 2989/2995/3008/3019/3020) + the T-3 due-bucket machinery;
`epochSet` stays its OWN FROZEN materialized keyspace, `epochSet := qualified` (copy) at the boundary. This is where the two sharpest hazards come due:
- **v5-GATE every new keyspace in the leaf marshaller** or committing them breaks the era-3 byte-identical freeze. 4b ablation owed (byte-identical era-3 replay corpus).
- **Rotate-LAST stale-capture (sharpest):** any 4b hook firing AFTER `rotateEpoch` reads `qualified` (`chain.go:3130`) freezes a STALE set = I3 divergence.
  Ordering ablation owed. Keep the coverage guard's `extra` branch STRICT or the 4a reservation stops being inert-by-guard.
Then 4c (v5 predicate + RegCap total-count enforcement + the flipped ablation; honest block ACCEPTS to ceiling, TOTAL>256 any-mix REJECTS, watch I4) →
4d (activation + mint-flip — the IRREVERSIBLE gate; Andrew ratifies). Also land the instrument thinking-doc (`era4-regcap-instrument-options` @ `0b02854`).

## BUILD-TIME GATES (RECERT2 — each MUST ablate RED before its increment is trusted)
`qualified` maintenance drift-guard (per-site; **2989 reddens specifically**) · T-3 dual-source guard (ablate missed renew old-bucket delete) ·
T-3 byte-identical era-3 replay corpus · Q5 recovery-branch agreement · existing completeness guards force the new tags · RegCap total-count any-mix ablation.

## OPEN DECISIONS FOR ANDREW (carried)
- Recovery-boundary DIRECTION (R2): commit `LivenessRecoveryHeight` (trustless, more scope) vs O-2 posture-bound. Separable follow-on.
- era-3 residual: `versionSupported` admits v3 = silent mis-validation — out of era-4 scope.
- Periodic memory PR: batch session-13 seat memory to main (route a Bash seat, `.claude/agent-memory/**` only).

## Detail: [[era4-witnessable-transitions]], [[witness-floor-box-inc3-refuted]], [[witness-floor-box-track]].

## STANDING SCARS (carry forward)
- ★★ (session-13): a CERTIFIED correction that is never WRITTEN INTO the design-of-record is not done — docs (+#635) kept the REFUTED fresh-only rule
  and the Builder's new draft re-introduced it. Propagate a certified rule change into EVERY doc-of-record + the ablation before sizing/shipping on it.
- ★★ (session-13): measure the VALIDITY minimum, not the ENCODER minimum (phantom 225 B). And SWEEP the evolving knob (k) a consensus value depends on
  — at k=64 byte-cap and count-cap coincide and hid the instrument choice; the difference only appears across k. Both caught by blind PE/Research gates.
- ★ (session-13): a consensus VALUE from a proposer-POLICY constant is not a validity bound. `MaxBondRegBytesPerBlock` was policy; RegCap is its own v5 validity rule.
- ★ (session-13): the Researcher seat has NO Bash → cannot read files at a branch commit; route branch-content confirms to PE/Tester (Bash), or check out first.
- ★ (session-13): VERIFY a merge directly (`gh pr view --json state,mergedAt` + `git log origin/main`) — the merge command echo can lie; never trust it.
- ★ (session-12): `.claude/` MUST stay TRACKED — untracked + a seat `git add .claude/` into a feature commit → checkout to main DELETES agent defs + memory.
- ★ (session-12): DON'T skip a "closing" blind confirm as "obviously composes" — the COMPOSITION PREMISE itself can be wrong (fresh-only x3 + phantom-225 all caught).
- ★ (session-12): worktree-isolate MUTATING seats — a shared-tree seat that `git checkout`s a feature commit churns/deletes uncommitted memory.
- A Tester "all N pass" is a BASELINE not a verdict — require demonstrated RED + real-fixture/honest-verify.
- Stale local `main` bit THREE seats — bake `git fetch origin` / read-at-named-commit into EVERY seat prompt.
- Planner seat has NO Edit/Bash → runs under plain `claude` when it must edit code/tree; drives merges via a Bash seat.
