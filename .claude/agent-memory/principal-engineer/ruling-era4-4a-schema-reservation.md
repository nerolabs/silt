---
name: ruling-era4-4a-schema-reservation
description: Blind review of commit 7241f82 (era-4 4a inert schema reservation) — SHIP-WITH-FIXES; source genuinely inert, but two comments + CHANGELOG cite a doc that does not exist in the commit
metadata:
  type: project
---

# Ruling: era-4 4a inert schema reservation (commit 7241f82)

**Verdict: SHIP-WITH-FIXES.** The consensus-relevant claim (inert on the live v4 chain) is TRUE and I verified it three ways. The fixes are documentation-integrity, not consensus.

**Why:** First code increment of ratified era-4. Adds only `BlockVersionWitnessable=5` (chain.go:357) and three reserved tag strings `tagDueBucket/tagQualified/tagEpochStart` (statehash.go:68-70). Blind review off origin/main @ 0984db4.

**How to apply:** era-4 is a multi-increment build (4a schema → 4b maintenance spine + v5 leaves → 4c predicate + versionSupported widen atomically → 4d activation). 4a is the inert base. The safety of the split rests on the two-way coverage guard, verified below.

## Verified premises (my own reading + execution)
- All 4 new identifiers have EXACTLY ONE reference each = their own declaration (grep, no consumer). Genuinely inert at reference level.
- `stateRootTags` var (statehash.go:77-83) and `stateRootLeaves()` (statehash.go:90-145) are BYTE-UNCHANGED vs base. Committed-root construction untouched.
- `versionSupported = v>=1 && v<=BlockVersionStateRoot` (chain.go:758) UNCHANGED = `<=4`. A v5 block still fails decode (chain.go:765). No mint flip (still `BlockVersionStateRoot`, chain.go:3236).
- No `qualified` / `dueBucket` Chain field exists; `epochStart` field exists but stays observable/excluded (statehash.go:25), NOT wired to `tagEpochStart`.
- Tag injectivity holds across the full 21-name set: no proper-prefix pairs, no dups (Python check). `epochSet` vs `epochStart` disambiguated by NUL after `epochSet`. Well-formed, collision-free, safe to freeze.
- Ran core/chain + core/statehash fresh: GREEN. `TestStateRootCoversExactlyTheCommittedSetFields` PASS proves the 3 tags did NOT enter the committed set.
- Coverage guard (modelcheck_stateroot_determinism_test.go:38-60) is a genuine TWO-WAY check: fails on `missing` (classified-uncommitted) AND `extra` (committed-unclassified). THIS is what makes the split safe — 4b cannot leak a reserved tag into the root without turning it red both ways.

## The fix (should-fix, not consensus)
`docs/thinking/2026-08-29-era4-build-decomposition-options.md` is cited in chain.go:346, statehash.go:67, and CHANGELOG — but DOES NOT EXIST in the commit tree. Two dangling doc refs in committed source + a PACE-BEFORE-CODE miss (deliberation ships in same PR). Note: the untracked `...witnessable-transitions-options.md` is a DIFFERENT file; the decomposition doc cited by name is genuinely absent.

## Coupling the split must honor (flag for 4b/4c)
The whole safety of shipping a reserved tag with no backing field rests on the two-way coverage guard staying wired. If 4b ever relaxes the `extra` branch to admit tags ahead of their fields, the reservation stops being inert-by-guard.
