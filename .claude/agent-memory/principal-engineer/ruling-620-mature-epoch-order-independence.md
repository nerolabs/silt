---
name: ruling-620-mature-epoch-order-independence
description: PR #620 mature-epoch family order-independence — SHIP-WITH-CHANGES; epochSet is order-INVARIANT by construction (deterministic read of covered maps), NOT stress-proven; claim overstated
metadata:
  type: project
---

PR #620 covers everMature/matureEpoch/epochSet in the order-independence oracle via a
`matureOrderings` fixture (anchorless, epochs on, MatureValidators=2; victim slashed at
height 1 vs 3 across two orderings; freeze at height 4).

**Verdict: SHIP-WITH-CHANGES.** Mechanism sound, scope clean (chain.go diff = 0 lines),
guard is a real trip-wire. No consensus finding to escalate.

**The finding (framing hides it):** the PR claims order-INDEPENDENT. What it proves is
order-INVARIANT BY CONSTRUCTION. `epochSet = liveQualifiedSet(bonded,slashed)`
(chain.go:2908, 1108-1116), and rotateEpoch runs LAST in apply (chain.go:2882-2893), so
the freeze reads only FINAL post-block state. I measured: both fixture orderings feed the
freeze identical `bonded=4, slashed-victim=true`. The slash-timing variable (1 vs 3) is
washed out before height 4 — even a slash IN the boundary block (height 4) converges. So
`epochSet` cannot diverge under any slash ordering in this world; it is a deterministic
read of two maps whose order-independence #617/#618 already cover. Unlike #618 (which
found a real fork), nothing here COULD diverge.

**Also not stressed:** latch HEIGHT — all bonds at genesis, latch trips at height 1 in
BOTH (measured); the spread-bond latch-height stress the PR's own deliberation describes
(lines 448-452) is not built. Same for matureEpoch handoff height. Neither can flip a
one-way final-state bool, so not a defect — but the residual must be named.

**I verified myself:** guard is real (made 4-member vs 5-member frozen set, DeepEqual
distinguishes); reroute loses no coverage (3 fields empty in twoOrderings, populated in
matureOrderings); full ./core/chain green.

**Change requested (wording, no code):** soften "order-INDEPENDENT" to "order-invariant
read of covered inputs"; name latch/handoff-height as un-stressed residual.

**Coupling flagged:** epochSet order-independence leans on rotateEpoch being LAST in
apply — pin that as load-bearing. The #506-gate family (next increment, reuses this
fixture) has a genuinely order-exposed regVersion weight tally (rotateEpoch:2922-2934) —
must NOT inherit this PR's convergent-by-construction assumption.

**The human's call:** whether "deterministic read of covered maps" clears the era-3
freeze bar for epochSet, or whether the freeze should wait for a latch/handoff-height
fixture. My rec: sound for epochSet now; height dimensions are follow-on, don't block.

Ruling: /Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-620-mature-epoch-order-independence-2026-08-28.md
