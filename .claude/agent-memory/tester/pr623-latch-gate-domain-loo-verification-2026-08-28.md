---
name: pr623-latch-gate-domain-loo-verification-2026-08-28
description: Latch/gate/domain leave-one-out tranche verification (2026-08-28): all 6 new probes adversarially confirmed RED, guard fires confirmed, mutating probes confirmed executing, full -race PASS
metadata:
  type: project
---

# Latch/gate/domain leave-one-out tranche — tester verification (2026-08-28)

**Branch:** `keystone-loo-tranche` at `c7c611a` (checkpoint of uncommitted Builder work before ablation)

**Files verified:** `core/chain/modelcheck_snapshot_equivalence_test.go` (422 lines added)

**Why:** Six committed fields were in `probeUncovered`; the Builder promoted them to real probes. The Tester independently confirms each probe is adversarial (goes RED under defect injection) rather than decoration green.

**How to apply:** This record is the ground truth for the adversarial confirmation of this tranche. Cite `c7c611a` and the per-field error strings below as evidence.

---

## Step 1 — Baseline

- Branch: `keystone-loo-tranche`, commit `c7c611a`
- `go test ./core/chain/`: PASS (7.104s)
- `-race` oracle tests: both PASS (TestLeaveOneOutProvesEachFieldLoadBearing + TestSnapshotBootMatchesReplayBoot)
- All 14 flip log lines emitted (8 prior fields + 6 new fields all confirmed flipping)

---

## Step 2 — Per-field adversarial ablation (all 6 new fields)

Each ablation was driven via a temporary test file (`zz_tester_ablation_test.go`, removed after use), calling the underlying predicate directly to capture the named error. No panics, no silent skips.

| Field | World | Injected defect | Full-snapshot error (RED) | Ablated result | Confirmed adversarial |
|-------|-------|----------------|--------------------------|---------------|-----------------------|
| `everMature` | `deMatureWorld` | `snapshotBoot(c, "everMature")` — zero bool | `chain: de-matured network requires a real-bond super-quorum (≥⅔ of live bonded weight): coalition holds 5 MiB of 105 MiB bonded (need ≥70 MiB)` (`ErrDeMatureQuorum`) | nil (accept, bar skipped) | YES |
| `matureEpoch` | `matureEpochWorld` | `snapshotBoot(c, "matureEpoch")` — zero bool | `chain: mature-epoch commit lacks the frozen-weight super-majority (>⅔ of epoch bonded weight — B2, research certification 2026-08-13): coalition holds 2097152 of 23068672 bonded weight (need >15379114)` (`ErrNoQuorumWeight`) | nil (accept on count floor, bftThreshold(2)=1) | YES |
| `gateLockedIn` | `gateWorld` | `snapshotBoot(c, "gateLockedIn")` — zero bool | `chain: bond registration violates the active reg-inclusion rate bound (#506 R-rule — slashed identity, or re-registered within R of its last committed reg): validator c1d0792e… re-registered 4 blocks after its last reg (R=10)` (`ErrRegGate`) | nil (accept, gate never armed) | YES |
| `gateHeight` | `gateWorld` | `snapshotBoot(c, "gateHeight")` — zero uint64 | nil (accept, reg is pre-gate at height gateHeight-1) | `chain: bond registration violates the active reg-inclusion rate bound (#506 R-rule — slashed identity, or re-registered within R of its last committed reg): validator c1d0792e… re-registered 2 blocks after its last reg (R=10)` (`ErrRegGate`, gate fires early) | YES (flip is opposite direction: full=accept, ablated=reject) |
| `regVersion` | `regVersionWorld` | `snapshotBoot(c, "regVersion")` — empty map | `chain: bond registration violates the active reg-inclusion rate bound (#506 R-rule — slashed identity, or re-registered within R of its last committed reg): validator 5b316f… re-registered 4 blocks after its last reg (R=10)` (`ErrRegGate`, via apply(boundary) locking gate) | nil (accept, gate never locked — zero ready weight in tally) | YES — mutating probe |
| `bondDomain` | `domainWorld` | `snapshotBoot(c, "bondDomain")` — empty map | accept (anchors eligible, bonds in one domain, matureNow()=false) | reject (anchors shed — coefficient rises, latch trips on apply, 0 qualified) | YES — mutating probe; flip is opposite: full=accept, ablated=reject |

All 6 errors are named consensus/gate rejections. None are nil-map panics or silent skips. The panic recovery path in `askSafely` was not invoked for any ablation.

---

## Step 3 — Guard fires confirmed

Injected: `everMatureProbe.ask` neutered to always return `"reject"` (field-blind).

Result at `modelcheck_snapshot_equivalence_test.go:1156`:
```
omitting committed field "everMature" changed NO verdict in any world.
Either the field is not actually load-bearing (so committing it is bloat on the snapshot
— revisit the Q2 enumeration and the Q3 growth analysis), or the probes are not adversarial
enough. This is a finding to route, not a test to relax.
```

Exit code 1. Guard fires precisely. Restored via `git checkout core/chain/modelcheck_snapshot_equivalence_test.go`.

---

## Step 4 — Mutating probes execute in leave-one-out

`regVersion` (`gate-tally` world) and `bondDomain` (`domain-latch` world) both appear in the verbose `-v` output with verdict flips:
- `[gate-tally] omitting regVersion → probe "regVersion carries the #506 readiness super-quorum...": full=reject ablated=accept`
- `[domain-latch] omitting bondDomain → probe "declared bondDomain is load-bearing via the maturity latch...": full=accept ablated=reject`

Both are correctly SKIPPED in `TestSnapshotBootMatchesReplayBoot` (mutates=true, the check() func skips them). That test PASSES.

---

## Step 5 — Final suite

`go test -race -count=1 -timeout 300s ./core/chain/`: PASS (26.756s)

**Verdict: all 6 new probes are adversarial. No decoration greens. No panics. Guard is live. Mutating probes execute. Full -race suite GREEN.**

`probeUncovered` now holds only `bondRegHeight` and `validatorsSeen` — 2 declared debts, down from 8.
