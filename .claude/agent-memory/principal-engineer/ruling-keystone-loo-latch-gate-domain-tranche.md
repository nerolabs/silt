---
name: ruling-keystone-loo-latch-gate-domain-tranche
description: Ruling on the keystone leave-one-out latch/gate/domain tranche (6 committed fields) — SHIP; freeze axis still open on bondRegHeight+validatorsSeen
metadata:
  type: project
---

Ruling filed 2026-08-28: `keystone-loo-tranche` @ c7c611a (local, not pushed) — 6
new leave-one-out probes for `everMature`, `matureEpoch`, `gateLockedIn`, `gateHeight`,
`regVersion`, `bondDomain` in `TestLeaveOneOutProvesEachFieldLoadBearing`.

Verdict: SHIP. All six flips are genuine and isolated to the field.

Premises I verified myself (chain.go):
- de-mature bar `c.everMature && c.objective() && !c.matureNow()` at :2471.
- mature-epoch weight quorum gated at :2457; qualification frozen branch at :1043.
- regGateActive = `gateLockedIn && h > gateHeight` at :3030; R-rule at :1497 with
  restoresHeldStanding exemption (false when bonded>=MinBond at :3057 — this is WHY
  the gate probes' live-bonded x is not exempted and the R-rule fires).
- rotateEpoch #506 tally reads regVersion at :3007, sets gateLockedIn :3012 / gateHeight :3013.
- bondDomain → C2Metric grouping :1985 → NakamotoDomains :2023 → MatureCoefficient :1880
  → matureNow :1867. NOT metric-only.

Independent audit probes I wrote/ran/deleted (zz_pe_audit*.go):
- bondDomain coefficient 1→3 on drop (immature→mature). Direction only works as
  merge-lowers because ablation models drop as EMPTY (empty domains only RAISE the count).
- everMature: matureNow IDENTICAL both sides — flip isolated to the guard.
- regVersion two-step: full locks gate (reject), dropped doesn't (accept).
- Named reject reasons: matureEpoch→ErrNoQuorumWeight; bondDomain-dropped→anchor shed,
  seen 2→0, count floor "0 qualified need 4".

STOP boundaries held: every setField target is committed STATE, never a cfg threshold.
weight-sum seam 2450-2456, epochSet freeze/rotateEpoch I3, ⌈A/2⌉ all untouched. Diff is
test-only (zero source change).

THE COUPLING THE CONSULT MISSED (same shape as prior keystone rulings): the era-3
FREEZE GATE. This tranche moves completeness 4/12→10/12 committedSet fields, which reads
like "almost done" but is NOT "safe to freeze." Two residuals for the freeze checklist:
1. probeUncovered's bondRegHeight blocker-reason is STALE — gateWorld (added here) IS a
   gate-active world, so bondRegHeight is now cheaply probable (drop → skip R-rule at
   :1497 → flip). Fix the reason.
2. Completeness axis NOT closed: bondRegHeight (cert field 9) + validatorsSeen (cert
   field 12) still unprobed. Do not declare closed.

The one call that is Andrew's: freeze with these two still in probeUncovered, or wait.
My recommendation: WAIT — both probes are cheap, a frozen unproven field is the exact
bloat-or-unsound finding this oracle exists to force.

Full path: /Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-keystone-loo-latch-gate-domain-tranche-2026-08-28.md

Related: [[ruling-keystone-probes-bonded-epochset]] (epochSet membership vs weight bytes
split), [[ruling-620-mature-epoch-order-independence]] (matureEpoch/epochSet regime).
