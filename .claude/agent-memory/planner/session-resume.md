---
name: session-resume
description: ▶▶ RESUME HERE (session-12 END) — era-4 RATIFIED + recorded (PR #635) + build-decomposed. NEXT session scoped goal = build 4a (schema/classification) through the blind loop. One owed Research confirm of RegCap=256.
metadata:
  type: reference
---

# ▶▶ RESUME HERE — session-12 END (2026-08-29)

**origin/main HEAD:** `0984db4` (VERIFIED; local main NOT stale this session). ALWAYS `git fetch` at session start.
Era-3 committed state-root format FROZEN (immutable, #632). Frozen consensus formats amendable only by a NEW ERA (new `BlockVersion`).

## ★★★ SESSION-12 BANKED: era-4 design RATIFIED + recorded + build-decomposed
Full arc this session: measurement → E-2 correction → re-cert CERTIFIED-WITH-CONDITIONS → RegCap measurement → VETO GATE → **Andrew
RATIFIED** (full format package + Option A / RegCap=256 + #299 re-mint gate; recovery boundary deferred). Zero mechanism code shipped
(correct — build starts next session). Zero billable resources. Certs: `.../research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`.

## STATE OF PLAY
- **PR #635 — canon record, docs-only, GREEN, NOT MERGED.** Records the era-4 ratification (`docs/decisions.md`), design-doc status →
  RATIFIED, and the RegCap #299 re-mint gate (`docs/design/owned-residuals.md` E6). Merge = Andrew's or route to a Bash seat (docs-only
  passes the self-merge classifier; Planner has no Bash). Branch `era4-ratification-and-build-decomposition`.
- **Build decomposition PACE:** `docs/thinking/2026-08-29-era4-build-decomposition-options.md`. Increments **4a** schema+classification →
  **4b** maintenance spine → **4c** v5 predicate + RegCap + version-widen (PREDICATE-FIRST) → **4d** activation + mint-flip.

## ★ OWED BEFORE 4c BUILDS THE RegCap RULE (not blocking 4a/4b): a brief Research confirm of the VALUE
RECERT2 Q2 rendered RegCap "measurement-required, cannot certify at desk." The measurement is now DONE (honest ceiling=1). 256 sits in
Research's certified bracket [honest-ceiling=1, 16,384] with 256× margin — so it COMPOSES, but no Research seat has closed Q2 at the
specific value + the FRESH-only counting + the #299 gate. Route a brief Research confirm to lift Q2 → CERTIFIED before 4c. Parallel-safe.

## ★ NEXT SESSION SCOPED GOAL: build increment 4a through the blind loop
**4a = schema + classification ONLY.** Mint `BlockVersion=5` const; add `tagDueBucket`/`tagQualified`/`tagEpochStart` to `statehash.go`
`stateRootTags`; wire classification. NO `Chain` map adds, NO `apply()` change, NO predicate, **NO `versionSupported<=5` widen** (held to
4c, predicate-first). Gate: ablate one tag→classification link, confirm `TestStateFieldsAreClassified` reddens. Blind loop: Builder →
blind PE + Tester → merge, verified on origin/main.

## ★★ THREE CONSENSUS-BREAKING HAZARDS the decomposition surfaced (each mapped to a red-able ablation — HOLD these gates)
1. **New keyspaces MUST be v5-GATED in the leaf marshaller** or committing them breaks the era-3 byte-identical freeze on live v4 nodes.
   Owed 4b ablation: remove the gate, confirm the era-3 replay root DIVERGES.
2. **Rotate-LAST stale-capture (the sharpest):** if any 4b maintenance hook fires AFTER `rotateEpoch` reads `qualified` (`chain.go:3130`),
   the boundary freezes a STALE set = I3-equivalent divergence. Couples intra-block ordering to the Q5 recovery agreement. Ordering ablation owed.
3. **RegCap counts FRESH regs only** (`chain.go:1587` `ok==false`), not renewals. Owed 4c ablation: 300 renewals + 200 fresh ACCEPT; 257 fresh REJECT.

## BUILD-TIME GATES (from RECERT2 — each MUST ablate RED before its increment is trusted)
- `qualified` maintenance drift-guard (`qualified == filter(bonded, slashed, MinBond)` every block; per-site ablate; **2989 reddens specifically**).
- T-3 dual-source drift-guard (`bucket ⟺ bondRegHeight+ttl+1==D AND bonded present`; byte-identical StateRoot vs era-3 replay; ablate missed renew old-bucket delete).
- T-3 byte-identical replay corpus (renew-reset, ttl==0, slash-before-due). Q5 recovery-branch agreement assertion. Existing completeness guards force the new tags.

## OPEN DECISIONS FOR ANDREW (carried)
- Recovery-boundary DIRECTION (commit `LivenessRecoveryHeight` = trustless, more scope; vs O-2 posture-bound) — separable follow-on (R2).
- era-3 residual: `versionSupported` admits v3 = silent mis-validation — out of era-4 scope, open elsewhere.

## Detail: [[era4-witnessable-transitions]], [[witness-floor-box-inc3-refuted]], [[witness-floor-box-track]].

## STANDING SCARS (carry forward)
- A Tester "all N pass" is a BASELINE not a verdict — require demonstrated RED + real-fixture/honest-verify.
- Blind PE catches the SYMMETRIC hole the Tester's injections miss; a Researcher can REFUTE a PE's proposed fix — route fixes back through the gate.
- Stale local `main` bit THREE seats last session — bake `git fetch origin` / read-at-named-commit into EVERY seat prompt.
- `gh pr merge` can print a local-worktree error that is NOT a merge refusal — verify origin/main directly.
- Researcher seat has NO Bash → verifies tree by artifact-presence, not `git fetch`. Planner seat has NO Edit → update memory via Write.
