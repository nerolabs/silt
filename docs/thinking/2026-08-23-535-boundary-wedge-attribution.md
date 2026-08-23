# #535 attribution — the h64 epoch-boundary liveness cliff (why no unilateral fix)

**Date:** 2026-08-23. **Issue:** #535. **Field run:** 45da13c-17686.

## The mechanism paragraph (build-immutable #6)

The failure is a permanent liveness stall at the epoch boundary h64 **because**
the mature-epoch finality quorum requires signers holding > ⅔ of the FROZEN
epoch weight (`requireEpochWeightQuorum` sums `c.epochSet`), and a member that
lapses or goes offline mid-epoch keeps its frozen weight in that denominator
for the whole epoch (`attesterQualified` is frozen membership) — so once
members holding > ⅓ of the frozen weight cannot sign, no block reaches the
super-quorum, including the boundary block whose commit is the only event that
rotates the snapshot to a lighter set; the #506 R-gate compounds the
non-recovery by refusing a returning member's re-registration within R blocks.
The field's `324 MiB bonded across 9` against a ~516 MiB frozen total is this
exactly. This is addressed **by** a research ruling on whether the frozen
denominator should exclude provably-lapsed members — NOT by a unilateral code
change, because it is a consensus-rule/quorum-arithmetic change (build-process
rule 5) and the design comment (`chain.go` :937-944) already weighed and
rejected the naive form ("a protocol-forced mid-epoch disqualification… could
stall the chain").

## How it was attributed (code-reading, then confirmed by deterministic repro)

No guessing (build-immutable #7). The attribution is a chain of read code:
`requireEpochWeightQuorum` (frozen `total`) → `attesterQualified` (frozen
membership, lapsed keeps its vote) → the design comment naming the exact
stall risk → the field C2 line (`324 MiB across 9`). The repro
(`core/chain/modelcheck_535_boundary_wedge_test.go`) then reproduces the
arithmetic deterministically: a 9-of-12 live coalition (324 MiB) is refused
with `ErrNoQuorumWeight`; a 10-of-12 control (388 MiB) commits. The cliff is
precisely "> ⅓ of frozen weight offline," and the fix space is the DENOMINATOR,
not the quorum rule.

## Why the repro is GREEN, not born-RED

This is a **characterization oracle**, the codebase's parked-pending-consult
pattern (the #451 lineage). > ⅓ offline denying a commit IS the BFT liveness
bound and is *correct* against a live set — so a born-RED "this must not
happen" assertion would be wrong. GREEN here means "the cliff is present and
sits exactly where characterized." The finding is that the denominator is the
FROZEN set, so the effective bound is measured against stale membership. What
to do about that is the consult's call. When the ruling lands, the fix's
merge gate becomes: this test's control still commits, and a new assertion
(objectively-lapsed weight excluded, or the boundary rotates) turns from
its current characterization to the ruled behavior.

## Scope discipline

- Consult: `/Users/andrewedmond/Claude/claude/silt-reviews/research/535-epoch-boundary-liveness-cliff-CONSULT.md`.
- The model-check gains this schedule now (D-CONSENSUS: confirm, never discover).
- NO cloud re-run of the Phase 3 gate until the ruling lands and the fix is
  local-green — the wedge is schedule-deterministic.
- The companion harness defects that mis-attributed the wedge (#536) are
  harness-only and fixed separately.
