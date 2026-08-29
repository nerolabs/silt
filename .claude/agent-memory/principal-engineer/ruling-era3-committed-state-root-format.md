---
name: ruling-era3-committed-state-root-format
description: era-3 block format 7-question ruling — right shape WITH CHANGES; mint v4 not v3 (versionSupported already admits 3 = silent mis-validation); Q2 value-encoding is SAFETY not read-correctness
metadata:
  type: project
---

# Ruling: era-3 committed state-root block format (2026-08-28)

**Verdict:** right shape to take into Researcher certification, WITH two changes.
Ruling filed at `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era3-committed-state-root-format-2026-08-28.md`.

**Why / the two changes I forced (both verified against source):**

1. **MINT v4, NOT v3 (Q7) — I DISAGREED with the doc's 2A/v3 lean.**
   `versionSupported(v) = v>=1 && v<=BlockVersionRegGate` at `chain.go:668`, and
   `BlockVersionRegGate=3` (`chain.go:299`). So a current binary ACCEPTS a v3 block at
   decode and validates it under era-2 rules — with no era-3 root predicate. Minting 3 =
   silent mis-validation window, the exact thing `BlockVersion` exists to prevent
   (`chain.go:260-263`). v4 dissolves the const collision (3 stays #506 reg-readiness) and
   forces tightening `versionSupported`. The doc's "this binary already accepts v3" is the
   TRAP not the feature — that clause was written for a REJECT-only rule (#506), doesn't
   transfer to a schema+validity change.

2. **Q2 value-encoding is a CONSENSUS-SAFETY property, not read-correctness — HIGHEST SEV.**
   I verified THREE super-quorum predicates that SUM weights: `chain.go:2527-2536`
   (de-mature, sums epochSet), `chain.go:2593-2604` (bonded super-quorum, `⌈2·total/3⌉`),
   `chain.go:3150-3152` (mixed). A true-presence/wrong-weight witness forges a super-quorum.
   Presence-only encoding is UNSOUND for bonded/epochSet/bondRootOwner/bondRootProven/etc.

**Other calls:** Q1 (two-root composition) sound as proposed — every history-derived field is
committedSet→StateRoot, revLog→LogRoot, or the observable epochStart (under no root, safe only
because no predicate reads it). Q3 (keyspace) sound in shape but CONTINGENT on Researcher proving
prefix-freeness across 16 tags + scalar reserved keys. Q4 (verifier invariant) correct but
INCOMPLETE — needs a 2nd clause: witness pins (value,present), predicate must branch on both,
never Go map-zero default (`bondDomain=0` = present-zero vs proven-absent must be explicit). Q5
(activation) sound, #506 mechanism transfers, strictly-greater boundary is clean. Q6 no NEW
security param but sha256 + value-widths are inherited load-bearing.

**Missed couplings I named:** (a) value-encoding = safety not completeness; (b) v3 silent-
mis-validation window; (c) Q4 needs 2nd clause; (d) `TestStateFieldsAreClassified`
(`completeness_test.go:146`) catches a new FIELD but NOT a new READER of epochStart.

**The one call that's Andrew's:** mint boundary (v4 per my rec) + soft-vs-flag-day. Research
certifies the soundness; I recommend + settle severity/sequencing.

Related: [[ruling-keystone-probes-bonded-epochset]] (weight bytes unprobed — feeds #603 gate),
[[ruling-603-weight-bytes-discharge]], [[ruling-600-floor-box-direction]] (the witness end-state
this format is the precondition for), [[ruling-keystone-node-store-backend]].
