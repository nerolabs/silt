---
name: era4-4a-branch-prep
description: era-4 4a PR #637 MERGED to main 2026-08-29 (squash, origin/main HEAD effe115). What the merge conflicted on and how it resolved; the anticipated-but-absent CHANGELOG collision.
metadata:
  type: project
---

# era-4 4a branch-prep — PR #637, MERGED 2026-08-29

**MERGED (squash) 2026-08-29 17:31 UTC. `origin/main` HEAD = `effe115`** — "feat(chain):
era-4 4a — mint BlockVersion=5 + reserve the three v5 field tags (inert) (#637)". All CI green
at merge (head `4a6c05d`, mergeStateStatus CLEAN). era-4 4a schema+classification is now on main.
Next in the locked order: 4b (see [[era4-ratification-and-build-order]]).

Prepped `era4-4a-schema-classification` (blind-reviewed at `7241f82`, off old base `0984db4`)
onto current `origin/main` `5ea1401` (#635). Opened PR #637 (base main). Merge HEAD `4a6c05d`.

**Why:** 4a's reviewed code must reach main CI-green without rewriting the reviewed commit —
human gives the merge go, this is prep only.

**How to apply:** when re-prepping a blind-reviewed branch onto moved main, MERGE (not rebase)
to preserve the reviewed commit; verify the code diff vs new main equals what was reviewed.

## What 4a's reviewed code is (must stay byte-identical)
- `core/chain/chain.go:357` — `const BlockVersionWitnessable = 5` (inert; does NOT widen versionSupported).
- `core/chain/statehash.go:68-70` — `tagDueBucket`/`tagQualified`/`tagEpochStart` strings, NOT in
  stateRootTags, NOT classified, NO leaf. Reserves on-wire byte layout only.
- CHANGELOG entry for 4a.
Verified: `git diff origin/main...branch -- core/ cmd/ adapters/` = ONLY those two hunks. Builds + vets clean.

## The merge — what conflicted, how resolved
- **NO conflict in code/CHANGELOG.** #635 touched only docs + `.claude/`; it never edited CHANGELOG,
  chain.go, or statehash.go. So the anticipated CHANGELOG-vs-#635 collision did NOT materialize —
  4a's CHANGELOG entry appended cleanly above the #634 line.
- **3 add/add conflicts, all in `.claude/agent-memory/planner/`** (MEMORY.md, era4-witnessable-transitions.md,
  session-resume.md). Cause: 4a's `7241f82` independently committed a `.claude/` snapshot; #636 tracked
  `.claude/` on main first, later sessions updated it. Resolved by taking **origin/main's side** (`--theirs`):
  main's copies are strictly newer (session-12 state) and superset the stale 4a snapshots. No memory authored,
  just picked the canonical side.

## Citation resolves post-merge
`docs/thinking/2026-08-29-era4-build-decomposition-options.md` (cited in chain.go:346, statehash.go:67,
CHANGELOG) was ABSENT in 4a's old-base tree (dangling citation) and landed via the merge from main.
Now present. Companion `...era4-witnessable-transitions-options.md` also present.

## CI (PR #637) — all pass
Docs-ship-with-code, Go vet/fmt/test, race detector, multi-process e2e, NAT hole-punch, cross-NAT relay,
website changelog+links, netlify deploy-preview — all pass. READY for human merge go. Do NOT self-merge (code PR).

Related: [[era4-ratification-and-build-order]], [[era4-witnessable-transitions-track]].
