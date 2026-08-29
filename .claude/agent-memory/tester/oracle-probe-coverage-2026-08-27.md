---
name: oracle-probe-coverage-2026-08-27
description: Keystone oracle leave-one-out probe coverage — verified on PR #617 branch ee1bcf3 (2026-08-28), spent+slashed PROMOTED
metadata:
  type: project
---

## Run 1 — 2026-08-27, session-7, branch main @ 9bfe8e2

All 10 oracle tests green. Coverage state: 3/16 committedSet fields probed (byRoot, revoked, bondRootOwner).
`bonded` and `epochSet` explicitly listed in probeUncovered as NEXT priority.

## Run 2 — 2026-08-27, session-8, branch keystone-probes-bonded-epochset (local, off main @ 9bfe8e2)

Coverage promoted to 5/16 (byRoot, revoked, bondRootOwner, epochSet, bonded). Ablation RED confirmed.

## TRUE-HEAD VERIFICATION — 2026-08-28, main @ 7d2a292

Verified by reading `core/chain/modelcheck_snapshot_equivalence_test.go` on the ff-updated tree.
At this point: `spent` and `slashed` were STILL in probeUncovered (unprobed debt).

## PR #617 ABLATION VERIFICATION — 2026-08-28, branch keystone-order-independence-spent-slashed @ ee1bcf3

All 4 ablations go RED; all reverted; final run GREEN. Working tree clean.

### Baseline (7 tests)

```
TestCommittedSetFieldsAreOrderIndependent   PASS (0.18s) — 16 fields identical across orderings
TestCommittedLogFieldsAreGenuinelyOrderDependent  PASS
TestRevLogRootIsOrderDependent              PASS
TestBondedOrderFreeUnderSlashInteraction    PASS
TestEpochWeightBytesAreLoadBearing          PASS
TestSnapshotBootMatchesReplayBoot           PASS
TestLeaveOneOutProvesEachFieldLoadBearing   PASS (0.19s)
```

Leave-one-out verdict flips observed on baseline:
- [token-spent] omitting spent: full=reject ablated=accept (double-spend allowed — real flip)
- [slashed-anchor] omitting slashed: full=disqualified ablated=qualified (re-admitted via launchAnchor — real flip)

### Ablation A — spent key order-sensitive (chain.go:2745)

Injection: `c.spent[string(e.Token.Serial)+fmt.Sprintf(":%d", b.Height)] = true`
Result: FAIL — `1 field(s) classified 'committedSet' DIFFER between two histories: [spent]`
Exit code: 1. Reverted.

### Ablation B — slashed key order-sensitive (chain.go:2819)

Injection: `taintedCulprit := culprit; taintedCulprit[0] ^= byte(b.Height); c.slashed[taintedCulprit] = true`
Result: FAIL — `1 field(s) classified 'committedSet' DIFFER between two histories: [slashed]`
Exit code: 1. Reverted.

### Ablation C — committedSet field empty in both orderings without orderVacuous declaration

Injection: clear `validatorsSeen` at end of each apply() call, making it ∅ in both orderings; not in orderVacuous.
Result: FAIL — `1 committedSet field(s) are EMPTY in both orderings and NOT declared in orderVacuous: [validatorsSeen]`
Exit code: 1. Reverted.

### Ablation D — spent leave-one-out probe non-adversarial

Injection: changed `mint(serial)` to `mint([]byte("never-spent-serial"))` in spentProbe — probes a serial never committed.
Result: FAIL — `omitting committed field "spent" changed NO verdict in any world.`
Exit code: 1. Reverted.

### Final clean green run

All 7 tests PASS. `git diff --stat` = empty (no leftover injection). Working tree clean.

### Coverage state on PR #617 (ee1bcf3)

probeUncovered (from test file): bondRootProven, bondRegHeight, regVersion, bondDomain, validatorsSeen, gateLockedIn, gateHeight, everMature, matureEpoch (9 fields).

Probed and ablation-confirmed:
- byRoot (launch world, dup-publish)
- revoked (launch world, un-revoke)
- bondRootOwner (launch world, F1 dedup)
- epochSet (mature-epoch world, membership flip → ErrNoQuorum)
- epochSet weight-bytes (weightBytesWorld, ⅔-weight flip → ErrNoQuorumWeight)
- bonded (objective-bonded world, qualification flip)
- spent (token-spent world, double-spend flip) — PROMOTED by PR #617
- slashed (slashed-anchor world, launchAnchor re-admission flip) — PROMOTED by PR #617

Total probed: 7 named fields (8 probes counting the weight-bytes discriminator separately).
Total probeUncovered debt: 9 fields.
