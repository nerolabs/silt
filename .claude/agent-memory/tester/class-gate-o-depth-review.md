---
name: class-gate-o-depth-review
description: CLASS-LEVEL GATE CANDIDATE (10+ occurrences, third-time rule long exceeded): any hot-path function touching the chain gets an O(depth) review before merge. Proposed for PE review checklist + CI.
metadata:
  type: project
---

# Class-level gate: O(depth) review for chain hot-path functions

**Status: PROPOSED — not yet encoded. Hand to Builder + Planner.**

**Basis:** The depth-war lineage (#528, #535, #549, #555, #556, #558, #560, #561, #562,
#563, #572 — 10+ incidents, all field-confirmed) is a single failure class: per-height cost
that grows with chain depth. The third-time rule threshold was exceeded many times over.
This must be encoded as a standing gate, not documented again. See [[scar-depth-war-lineage]]
for the full citation and incident table.

**The proposed standing check (for PE review checklist and CI):**

> **O(depth) review:** Any PR adding or modifying a function on the per-height hot path must
> answer: does per-height cost (time, memory, or I/O) grow with chain length?
>
> - If NO: state the bound in the PR body (e.g., "O(1) per block, cost is constant with depth").
> - If YES or UNKNOWN: require a measured benchmark parameterized on chain depth — not a
>   constant-height unit test — before merge.

**The canonical example (#555):**

`AllEntries` was O(n) per block at height n — perfectly green in all unit tests and short
sims, catastrophic at real chain depth. The fix (incremental update) was O(changed × log n)
per block. No inspection of the function's correctness would have caught this; only a
depth-parameterized benchmark would have.

**Why CI alone is insufficient:**

A unit test at fixed height h=10 is always green for an O(depth) bug. The gate must be a
benchmark, not a correctness assertion.

**Proposed CI artifact:**

A benchmark suite `BenchmarkChain/height=100`, `BenchmarkChain/height=1000`,
`BenchmarkChain/height=10000` on the critical per-height functions. Failure signal: per-
iteration cost grows super-linearly across the height dimension.

**Where to encode it:**

1. PE review checklist (the six-question quorum checklist in `consensus-invariants.md` is
   the model — a parallel "O(depth) checklist" for hot-path PRs).
2. A CI benchmark job that reports per-height cost ratios.
3. A code comment at the entry points most at risk (replay loop, per-block `apply`,
   `AllEntries`, the bond-proof path).

**This is a BUILDER + PLANNER task.** The Tester flags it; the gate is built by the Builder
and enforced by the Planner in the roadmap sequence. Do not close this entry until the
gate exists and has been confirmed to catch at least one synthetic O(depth) regression.

**Links:** [[scar-depth-war-lineage]], [[scar-one-defect-four-costumes]]
