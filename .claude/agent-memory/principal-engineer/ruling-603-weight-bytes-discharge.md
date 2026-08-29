---
name: ruling-603-weight-bytes-discharge
description: Ruling on #603 (epochSet committed weight-bytes load-bearing before era-3 freeze) — DISCHARGED by #606, NOT by the #623 matureEpoch probe the consult pointed at
metadata:
  type: project
---

Ruling 2026-08-28: **#603 DISCHARGED** — but NOT by the probe the consult named.

**Why:** era-3 freeze locks the SMT-committed field set. #603 owed a probe proving the
per-member WEIGHT BYTES of `epochSet` load-bearing (not just membership) via
`requireEpochWeightQuorum` (`chain.go:2489-2510`, reject at `3*support<=2*total`).

**The discharge:** `TestEpochWeightBytesAreLoadBearing`
(`core/chain/modelcheck_snapshot_equivalence_test.go:938`), shipped PR #606 / commit
`07ad6cd` — two PRs BEFORE #623. Holds membership byte-identical, flattens weights to a
constant (`flattenWeights`, line 921), asserts flip is `ErrNoQuorumWeight` specifically.

**The misdirect:** the consult framed #623's `matureEpoch` probe as the weight-bytes proof.
It is NOT. `matureEpoch` is a `bool` (`chain.go:920`); dropping it makes the weight-quorum
guard (`chain.go:2457 ... && c.matureEpoch`) not fire at all → path-entry flip, not
weight-bytes flip. The builder's own comment concedes this
(`modelcheck_snapshot_equivalence_test.go:598`: "changes whether the mature-epoch weight
rule APPLIES, never how the ⅔ is summed"). Its RED reason coincidentally being
`ErrNoQuorumWeight` is what made the framing tempting.

**I ablated to prove non-vacuous** (two-sided gate): (A) no-flatten/true-weights →
ablated accepts, test RED (`got <nil>`); (B) empty-membership instead of flatten → fails
via `ErrNoQuorum`, the `errors.Is(ErrNoQuorumWeight)` assert correctly rejects it, RED. So
the test refuses a membership-omission ablation dressed up as weight-bytes.

**The missed coupling (highest-value finding):** `bonded` weight has NO analogous
weight-bytes probe. `requireDeMatureSuperQuorum` (`chain.go:2471`) sums live `bonded`
weight the same way; its `everMature`/`deMatureWorld` probe is an entry-gate flip only
(comment at line 519 concedes it). If the era-3 freeze covers the `bonded` representation,
#603's logic implies a sibling obligation no current probe discharges.

**How to apply:** #603 is dischargeable-shippable for the epochSet axis. The one call for
Andrew: does the era-3 freeze scope include `bonded` weight? If yes, owe a
`requireDeMatureSuperQuorum` weight-bytes sibling probe first. My rec: build it regardless
(cheap clone of `weightBytesWorld` on the de-mature world). Note premise correction: the
consult said HEAD=`7101672`; local worktree HEAD was `4d1a1ec` (separate agent branch),
`7101672` is origin/main tip — code identical in both, verdict unaffected.

Related: [[ruling-keystone-probes-bonded-epochset]] (the membership-vs-weight split, era-3
freeze gate), [[ruling-keystone-loo-latch-gate-domain-tranche]] (the #623 tranche this
consult grew out of).

Ruling: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-603-weight-bytes-discharge-2026-08-28.md`
