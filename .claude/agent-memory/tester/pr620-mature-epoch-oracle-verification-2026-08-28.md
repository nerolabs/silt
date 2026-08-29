---
name: pr620-mature-epoch-oracle-verification-2026-08-28
description: PR #620 matureOrderings fixture: all 3 ablations RED confirmed, genuine intermediate divergence verified (bonded@h1 4 vs 5), full core+sim -race PASS
metadata:
  type: project
---

## PR #620 — mature-epoch family coverage in the order-independence oracle

Branch: `test/order-mature-epoch-family` (worktree `agent-a7816ff4cd9318629`, HEAD 59bfd65).
Date: 2026-08-28.

## Baseline

`go test ./core/chain/...` (no -race): PASS, 3.089s.
`go test ./core/chain/... -run TestCommittedSetFieldsAreOrderIndependent|...|TestMatureEpochFamilyIsOrderIndependent -v`:
- TestCommittedSetFieldsAreOrderIndependent: PASS — "all 16 committedSet fields identical"
- TestMatureEpochFamilyIsOrderIndependent: PASS — "froze identical 4-member epochSet across two opposite slash orderings"
- TestBondRegG3DisplacementIsOrderIndependent: PASS
- TestBondedOrderFreeUnderSlashInteraction: PASS
- TestRevLogRootIsOrderDependent: PASS
- TestCommittedLogFieldsAreGenuinelyOrderDependent: PASS

## Ablation A — epochSet: delete one governor from epochSet in slashEarly chain

Injection: `if slashEarly { delete(c.epochSet, idOf(gov[0])) }` before return in matureOrderings.

RED output:
- `TestCommittedSetFieldsAreOrderIndependent`: "1 field(s) classified `committedSet` DIFFER ... [epochSet]"
- `TestMatureEpochFamilyIsOrderIndependent`: "[slash-early] epochSet froze 3 members, want 4"

Reverted via `git checkout --`. SHA256 restored: `ec5881e658eefd1e10ef2293dd99fe89e137870e522ff3ce1c72cb9255163794`.

## Ablation B — matureEpoch: force false in slashEarly chain

Injection: `if slashEarly { c.matureEpoch = false }`.

RED output:
- `TestCommittedSetFieldsAreOrderIndependent`: "1 field(s) ... [matureEpoch]"
- `TestMatureEpochFamilyIsOrderIndependent`: "[slash-early] matureEpoch did NOT set — the #357 Cond-B handoff never fired"

Reverted. SHA256 restored.

## Ablation C — everMature: force false in slashEarly chain

Injection: `if slashEarly { c.everMature = false }`.

RED output:
- `TestCommittedSetFieldsAreOrderIndependent`: "1 field(s) ... [everMature]"
- `TestMatureEpochFamilyIsOrderIndependent`: "[slash-early] everMature did NOT latch — the maturity path never fired"

Reverted. SHA256 restored.

## Stressing-vs-vacuous confirmation

Intermediate-state probe (package-level test, removed after run):
- slash-early: bonded@h1=4 slashed@h1=1
- slash-late:  bonded@h1=5 slashed@h1=0

Genuine divergence at h1 confirmed. Both chains reach identical final state:
- slash-early final: everMature=true matureEpoch=true epochSet=4
- slash-late  final: everMature=true matureEpoch=true epochSet=4

The maturity latch trips (everMature) and the epoch freeze fires (matureEpoch, epochSet=4 members) in both orderings. Not vacuous.

## Final clean green

`go test ./core/chain/... -race -count=1`: PASS, 21.599s.
`git status`: clean (only `.claude/` untracked worktree overhead).

## Full core+sim -race

All 22 packages PASS (background run, exit code 0):
- `core/chain`: 38.771s PASS
- `core/node`: 326.467s PASS
- `sim`: 478.400s PASS
- All others: PASS

`TestPerHeightCostLinear` under `-race` with 30s timeout: times out identically on PR branch AND main — pre-existing depth-war scar (`[[scar-depth-war-lineage]]`), not introduced by this PR.

## Verdict

PROMOTED. All three ablations went RED naming the correct field. Intermediate state genuinely differs (bonded@h1 4 vs 5). All three latches fire in both orderings. Full core+sim -race GREEN.
