---
name: ruling-keystone-probes-bonded-epochset
description: PE ruling on the keystone leave-one-out probes for bonded and epochSet — sound-with-condition; the epochSet weight-role is still unprobed
metadata:
  type: project
---

Ruling filed 2026-08-27 on branch `keystone-probes-bonded-epochset` (blind review).
Verdict: **sound-with-one-condition**. Full ruling:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-keystone-probes-bonded-epochset-2026-08-27.md`.

**Why:** The two new leave-one-out probes (`bonded`, `epochSet`) are genuine finality-verdict
flips — verified by injection, not by trusting the green. Omitting either empties the map and
disqualifies the attesters via `attesterQualifiedAt` (`chain.go:1046` bonded; `1043`/`1068`
epochSet membership), so `collectQuorumSigs` returns `seen=0` and `requireQuorumStack` rejects
with `ErrNoQuorum`. Field-blind injection confirmed the leave-one-out goes RED. No consensus
rule moved; the #402 summation seam (`chain.go:2448-2462`) and `rotateEpoch` freeze are untouched.
World-aware harness does not weaken coverage — launch fields still run all launch probes.

**The coupling the consult missed (load-bearing):** the `epochSet` flip is carried by
frozen-set MEMBERSHIP, not by the ⅔-WEIGHT quorum the CHANGELOG claims. With epochSet empty,
`requireEpochWeightQuorum` short-circuits at `total<=0` (`chain.go:2452`) and never fires.
So the oracle does NOT yet prove the weight BYTES of epochSet are load-bearing — only membership.
For the era-3 format freeze (which turns on committing per-member weight), that gap matters.
Condition: reword the claim to "membership" for merge, and add a weight-discriminator probe
(coalition clearing the count floor but below ⅔ frozen weight → `ErrNoQuorumWeight`) before era-3
freezes.

**How to apply:** When the era-3 format-freeze decision comes up, do not let the weight role of
epochSet be treated as leave-one-out-proven. It is proven only by the `requireEpochWeightQuorum`
unit tests. This is engineering judgment (test-claim accuracy), mine to settle — no human veto-gate
and no research gate applied. See [[ruling-keystone-node-store-backend]] for the related keystone
storage ruling.
