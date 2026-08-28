# Keystone leave-one-out: closing the latch/gate/domain tranche

Date: 2026-08-28
Seat: Builder
Scope: `core/chain/modelcheck_snapshot_equivalence_test.go` —
`TestLeaveOneOutProvesEachFieldLoadBearing`. Six fields to move out of
`probeUncovered` OR report as findings: `everMature`, `matureEpoch`, `regVersion`,
`gateLockedIn`, `gateHeight`, `bondDomain`.

This is model-check/unit tier test work. It probes thresholds; it moves none. STOP
boundaries (weight-sum seam `chain.go:2450-2456`, epochSet freeze / `rotateEpoch` I3,
the `⌈A/2⌉` threshold, #603's weight-discriminator probe) are untouched.

## The one rule this oracle enforces

Ablate a committed field from a snapshot-booted replica (no block history); if no
verdict flips, the field is either bloat under the committed root or the probe is not
adversarial. A field is proven load-bearing once ANY world's probe flips on its
omission. The snapshot-boot constraint is load-bearing: no `ValidateCommit` (Prev==head
fails); drive the exact predicate the field feeds via `collectQuorumSigs →
requireQuorumStack`, `ValidateEntry`, or `validateBondRegs`.

## Where each field is actually read (evidence, chain.go)

- `everMature` — `requireQuorumStack:2471`: `c.everMature && c.objective() &&
  !c.matureNow()` gates `requireDeMatureSuperQuorum`. A VALIDITY predicate. Drop it →
  the de-mature ⅔-live-weight bar is skipped → a sub-⅔ coalition the network should
  refuse is ACCEPTED.
- `matureEpoch` — `requireQuorumStack:2457`: `...&& c.matureEpoch` gates
  `requireEpochWeightQuorum`; and `attesterQualifiedAt:1043` switches qualification to
  frozen-`epochSet` membership when `matureEpoch`. Drop it → the mature-epoch weight
  quorum is skipped.
- `gateLockedIn` — `regGateActive:3030`: `c.gateLockedIn && h > c.gateHeight`. Drop it
  (→ false) → the #506 R-rule (`validateBondRegs:1497`) never fires → a within-R
  re-registration that must be REJECTED (`ErrRegGate`) is ACCEPTED.
- `gateHeight` — `regGateActive:3030`: same predicate. Drop it (→ 0) → with the gate
  locked, `h > 0` is true for any real height, so a reg at a height BELOW the true
  gateHeight (which should be pre-gate, accepted) is now post-gate → the R-rule fires →
  REJECTED. The flip runs the OPPOSITE way from `gateLockedIn`.
- `regVersion` — read at EXACTLY ONE verdict-relevant site: `rotateEpoch:3007`, the #506
  lock-in tally (`ready += w` iff `regVersion[id] >= BlockVersionRegGate`). It is NOT
  read by any Validate/quorum path. So its leave-one-out flip requires `rotateEpoch` to
  RUN on the snapshot replica — i.e. `apply()` a boundary block that crosses the gate
  lock-in, then observe the downstream gate verdict.
- `bondDomain` — read by `C2Metric:1985` → `MatureCoefficient:1883` → `matureNow:1867`.
  And `matureNow()` IS consumed by a validity predicate: the same de-mature bar at
  `requireQuorumStack:2471`. So `bondDomain` is NOT metric-only — it feeds a verdict
  transitively through the maturity coefficient. This overturns the standing
  probeUncovered claim ("read by C2Metric, a metric rather than a validity predicate").

## Options considered

### everMature / matureEpoch

Option A (chosen): two separate worlds, one per field, because the two fields live in
mutually-exclusive regimes exactly as `bonded`/`epochSet` do (the two-regime trap).

- `deMatureWorld` (everMature): epochs DISABLED (`EpochBlocks: 0`), no anchors.
  `handedOff` = raw `everMature`; `requireEpochWeightQuorum` is skipped
  (needs `epochsEnabled()`), so the de-mature bar is the ONLY regime gate. Build a
  chain that latches `everMature` while decentralized, then lapses bonds so
  `!matureNow()` holds live. The probed block is a coalition BELOW ⅔ of live bonded
  weight. Full snapshot → `ErrDeMatureQuorum`; everMature-dropped → accept.
- `matureEpochWorld` (matureEpoch): epochs ON, `matureEpoch` set. The probed block is
  carried by frozen members but BELOW ⅔ frozen weight. Full snapshot →
  `ErrNoQuorumWeight` (the weight quorum fires because `matureEpoch`). matureEpoch-
  dropped → the weight quorum is skipped AND qualification leaves the frozen-set branch;
  the block clears the count floor → accept.

Rejected Option B: one combined world. It cannot exist — `epochsEnabled() &&
matureEpoch` and `!epochsEnabled()` are exclusive, and the de-mature bar needs the
former off to be the sole discriminator.

### gateLockedIn / gateHeight

Chosen: ONE `gateWorld` (adapted from `gateSwingOrderings`) that latches maturity and
LOCKS the #506 gate at a boundary, carrying a member whose `bondRegHeight` is recent.
Two probes on this world:

- `gateLockedInProbe`: validate a within-R re-reg for the recent member at a height >
  gateHeight. Full → `ErrRegGate`; gateLockedIn-dropped → accept (gate inert).
- `gateHeightProbe`: validate a within-R re-reg at a height STRICTLY BETWEEN 0 and the
  true gateHeight. Full → accept (pre-gate; R-rule not yet active). gateHeight-dropped
  (→ 0) → `ErrRegGate` (the reg now sits at `h > 0`, gate active).

The reg-window (`recentBondRegNonces`) yields `BondRegNonce(prev)` as its first nonce
regardless of block history (`chain.go:1385-1386` appends before the blockByHash break),
so a snapshot replica CAN validate a reg signed against any chosen `prev` hash. This is
the mechanism that lets `validateBondRegs` run on a history-less node.

### regVersion

Chosen: `regVersionWorld` — a gate-lockABLE world where the snapshot is captured BEFORE
the lock-in boundary (`gateLockedIn` still false, `regVersion` carries a ⅔-ready
super-quorum). The probe APPLIES the boundary block (drives `rotateEpoch` → the tally),
then validates a within-R re-reg past the resulting gateHeight:

- Full snapshot: tally sees ready ≥ ⅔ → gate locks → the within-R reg is REJECTED
  (`ErrRegGate`).
- regVersion-dropped snapshot: the tally sees an EMPTY regVersion map → ready = 0 → gate
  does NOT lock → the same reg is ACCEPTED.

This is a two-step probe (apply-then-validate), the only way to reach `regVersion`'s
sole verdict-relevant read on a snapshot node. It is `mutates: true` (it applies a
block), so it runs against throwaway replicas exactly like the F1/G3 probes.

### bondDomain — the finding call

bondDomain is NOT metric-only. It feeds `matureNow()` through the address-diverse
Nakamoto coefficient (`NakamotoDomains`), and `matureNow()` gates the de-mature validity
bar. So in principle a leave-one-out probe exists: a world matured ONLY because its
declared domains split a whale into address-diverse groups, where dropping `bondDomain`
collapses the coefficient below `MatureValidators` → `matureNow()` flips → the de-mature
bar changes.

But on a SNAPSHOT-booted node the direction is the problem. Ablation models a dropped
field as EMPTY, so `bondDomain`-dropped → all domains 0 (unset) → `NakamotoDomains`
RISES or holds (domain-0 is counted as each its own independent group,
`chain.go:2013-2014: "a chain with no domains set yields NakamotoDomains ==
NakamotoBonds"`). Dropping domains can only LOWER the min coefficient by REMOVING a
grouping that MERGED keys — i.e. `bondDomain` is load-bearing only when it MERGES several
keys into ONE failure domain, LOWERING the coefficient below what the raw bond count
suggests. That is the C2 direction that matters: a splitter declaring the same domain to
be honestly counted as one. So the covering world is: a chain whose keys share ONE
declared domain, matured=false live because the shared domain caps the coefficient; drop
bondDomain → the keys count as independent → coefficient RISES → `matureNow()` flips
true → the de-mature bar (which only fires when `!matureNow()`) STOPS firing. Verdict
flip: full → `ErrDeMatureQuorum`; dropped → accept.

Decision: BUILD IT (a real probe), do not report a finding. The evidence says bondDomain
DOES buy a verdict, so leaving it in probeUncovered with the "metric-only" excuse is the
quietly-kept-with-a-fresh-excuse anti-pattern the task forbids. The probe world is
`domainWorld`: it latches everMature with the shared-domain coefficient below
MatureValidators, epochs disabled, and the de-mature bar is the discriminator — the same
skeleton as `deMatureWorld` but the maturity flip is carried by the DOMAIN merge, not the
bond count.

## Ablation discipline (the session-7 rule)

Every probe is proven by injecting its defect and watching RED with the NAMED error, not
a nil-map panic and not a decoration all-identical pass. The per-field REDs and their
exact error values are recorded in the CHANGELOG entry and the final report. A probe is
not shipped until its defect has been injected and gone red.

## STOP boundaries — restated, verified untouched

- No change to the weight-sum seam (`requireEpochWeightQuorum` arithmetic).
- No change to `epochSet` freeze / `rotateEpoch` semantics — `regVersionWorld` DRIVES
  the existing tally, it does not alter it.
- No change to the `⌈A/2⌉`/`⌊A/2⌋+1` threshold.
- #603 (weight-discriminator) is out of scope; `epochSet` weight bytes are not probed
  here.
- Every flip changes which identities are ADMITTED (qualification) or whether a reg is a
  valid PAYLOAD — never how total/support are summed.
</content>
</invoke>
