---
name: session-resume
description: ▶▶ RESUME HERE (session-12) — era-4 design certified; 4a CLEARED both blind reviewers (doc-only fix before merge); RegCap COUNTING RULE REFUTED (fresh-only misses renewals) → REOPENED, awaiting Andrew. ★ .claude now TRACKED after a checkout wiped it.
metadata:
  type: reference
---

# ▶▶ RESUME HERE — session-12 (2026-08-29)

**origin/main HEAD:** `0984db4` (VERIFIED; local main NOT stale this session). ALWAYS `git fetch` at session start.
Era-3 committed state-root format FROZEN (immutable, #632). Frozen consensus formats amendable only by a NEW ERA (new `BlockVersion`).

## ★★★ INFRA INCIDENT + FIX (session-12): `.claude/` was untracked → a seat checkout WIPED it → NOW TRACKED
Root cause: `.claude/` was untracked on `main`, but a Builder seat ran `git add .claude/` and committed agent defs + ALL seat memory into
feature commit `7241f82`. Checking back out to `main` (which didn't track them) DELETED the working-tree files — agent defs AND memory.
`claude --agent planner` then failed (no `.claude/agents/planner.md`). RECOVERED via `git checkout 7241f82 -- .claude/` (full 88-file tree).
Andrew's fix (ratified): **TRACK `.claude/` on main** so no checkout can wipe it again. Note: agent DEFS are a snapshot of source-of-truth
`../agent-orchestra/` (recoverable there); the MEMORY is the unique/irreplaceable part — tracking protects it. Two marginal 4a-review memos
live only in `git stash@{0}` (untracked stash internals) — reconstructable, low value; stash can be dropped once tracking lands.

## ★★★ TWO LIVE THREADS (unchanged by the infra incident)
### (1) RegCap COUNTING RULE REFUTED → RegCap value+rule REOPENED (escalated to Andrew, AWAITING authorization)
Blind Research verdict `.../research-outcome/era4-regcap-value-VERDICT-2026-08-29.md`: the FRESH-ONLY cap is UNSOUND.
- Buckets fill with EVERY BondReg, fresh AND renewal (`chain.go:2995-2996`); #506 bounds renewals PER IDENTITY not per block → O(registry)
  distinct ids can each renew once in ONE block → one bucket = O(registry) → TTL-firing `C_block = O(registry)×SProofMax` = the wall era-4 removes.
- GEOMETRY: all regs at height h land in bucket `D=h+ttl+1` → **one bucket = one block's regs → per-block TOTAL BondReg count cap == bucket-population cap.**
- CORRECTION PATH (needs Andrew's go — he AUTHORIZED it): (a) cap = per-block TOTAL BondReg count (fresh+renewal); (b) Tester re-measure honest
  ceiling = `2 MiB / min(fresh, RENEWAL) reg size` (renewals pack SMALLER `node.go:71-73` → ceiling >1, UN-measured → 256 may be too LOW);
  (c) re-set value; (d) re-cert (Research) + re-ratify (Andrew). #299 re-mint gate still applies. REST OF ERA-4 DESIGN STANDS CERTIFIED.

### (2) 4a (schema/version-const + reserved tags) CLEARED BOTH BLIND REVIEWERS — merge blocked on a doc-only fix + the RegCap correction
Commit `7241f82`, branch `era4-4a-schema-classification` (off `0984db4`). Reserves `BlockVersionWitnessable=5` (chain.go:357) + tag STRINGS
`tagDueBucket`/`tagQualified`/`tagEpochStart` (statehash.go:68-70). Inert-by-guard; NO stateRootTags/classification/leaf/predicate/version-widen.
- **Tester PROMOTED** (green, inert vs era-2/3 golden-hash guards, ablation RED-confirmed). **PE SHIP-WITH-FIXES**
  (`.../principle-engineer/RULING-era4-4a-schema-reservation-2026-08-29.md`) — code inert (all 4 Qs verified). ONE doc-only fix: the
  decomposition doc is cited in chain.go:346 + statehash.go:67 + CHANGELOG but not in 4a's tree (lives on #635's branch).
- Both reviewers INDEPENDENTLY confirmed the Builder's split (defer stateRootTags wiring to 4b; the doc's literal "add in 4a" is NON-inert).

## ★ MERGE SEQUENCING (the two threads meet)
Do the RegCap correction FIRST (fixes design doc + decomposition hazard-3 + #635 canon `docs/decisions.md`/`owned-residuals.md` + re-measured
value), THEN merge corrected #635 (puts the decomposition doc on main → resolves 4a's citation), THEN merge 4a. 4a's CODE is correct/cleared;
only the doc dep + the canon correction gate its merge. NOTHING is merged yet — nothing at risk.

## NEXT MOVES (need seats + Andrew)
1. Andrew: RegCap correction AUTHORIZED; 4b runs PARALLEL (cap-independent). Track-`.claude` PR to merge.
2. Builder: correct cap → per-block TOTAL count across design doc + decomposition hazard-3 + #635 canon.
3. Tester: measure min RENEWAL reg byte size → honest ceiling → re-derive RegCap.
4. Research: re-cert corrected rule+value. Andrew: re-ratify. Then merge #635 → merge 4a → build 4b → 4c.

## ★★ THREE HAZARDS from the decomposition (HOLD as gates)
1. New keyspaces MUST be v5-GATED in the leaf marshaller or committing them breaks the era-3 byte-identical freeze. 4b ablation owed.
2. **Rotate-LAST stale-capture (sharpest):** any 4b hook firing AFTER `rotateEpoch` reads `qualified` (`chain.go:3130`) freezes a STALE set =
   I3 divergence. Ordering ablation owed. PE adds: keep the coverage guard's `extra` branch STRICT in 4b/4c or the reservation stops being inert-by-guard.
3. The cap rule (REFUTED as fresh-only) must bound per-block TOTAL count. 4c ablation: honest renewal-heavy block ACCEPTS to ceiling; TOTAL>RegCap REJECTS.

## BUILD-TIME GATES (RECERT2 — each MUST ablate RED before its increment is trusted)
`qualified` maintenance drift-guard (per-site; **2989 reddens specifically**) · T-3 dual-source guard (ablate missed renew old-bucket delete) ·
T-3 byte-identical era-3 replay corpus · Q5 recovery-branch agreement · existing completeness guards force the new tags.

## OPEN DECISIONS FOR ANDREW (carried)
- Recovery-boundary DIRECTION (R2): commit `LivenessRecoveryHeight` (trustless, more scope) vs O-2 posture-bound. Separable follow-on.
- era-3 residual: `versionSupported` admits v3 = silent mis-validation — out of era-4 scope.

## Detail: [[era4-witnessable-transitions]], [[witness-floor-box-inc3-refuted]], [[witness-floor-box-track]].

## STANDING SCARS (carry forward)
- ★ NEW (session-12): `.claude/` MUST stay TRACKED — an untracked `.claude/` + a seat `git add .claude/` into a feature commit → checkout to
  main DELETES agent defs + memory. Seats must NOT sweep `.claude/` into unrelated feature commits; commit memory deliberately.
- ★ NEW (session-12): DON'T skip a "closing" blind confirm as "obviously composes" — the COMPOSITION PREMISE itself can be wrong. The parallel
  RegCap confirm I nearly skipped caught the fresh-only unsoundness. Run the blind gate even when the answer looks certain.
- ★ NEW (session-12): worktree-isolate MUTATING seats — a shared-tree seat that `git checkout`s a feature commit churns/deletes uncommitted memory.
- A Tester "all N pass" is a BASELINE not a verdict — require demonstrated RED + real-fixture/honest-verify.
- Blind PE/Research catches the hole the sizing seat misses; route "obvious" fixes/values through the gate anyway.
- Stale local `main` bit THREE seats — bake `git fetch origin` / read-at-named-commit into EVERY seat prompt.
- Researcher seat has NO Bash → verifies tree by artifact-presence. Planner seat has NO Edit/Bash → this recovery ran under plain `claude`.
