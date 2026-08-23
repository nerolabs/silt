# #299 / Phase 3 — the answer-size evidence: it is a parameter, not an encoding

**Date:** 2026-08-23. **Context:** ROADMAP Phase 3 ("cheap heights") names the
near-term #299 tiers — Merkle multiproof compression, batch verification,
reg/entry batching — to shrink the ~1.5 MB per-reg height cost. Before
building, the terms were measured (build-immutable #7). The measurements
change the phase plan.

## The measurements (committed: `core/bond/answer_size_measure_test.go`,
`core/bond/verify_cpu_measure_test.go`)

Encoded answer decomposition at k=64 (`DefaultLabelSamples`), 4 KiB blocks:

| term | 8 MiB bond (n=2048) | 64 MiB bond (n=16384) |
|---|---|---|
| encoded total | 1497 KiB | 1513 KiB |
| possession blocks (20) | 80 KiB | 80 KiB |
| label-open blocks (64 × ~5) | 1280 KiB | 1264 KiB |
| — duplicated by leaf index | 160 KiB (12.5%) | **12 KiB (0.9%)** |
| Merkle proofs raw | 116 KiB | 147 KiB |
| — distinct-hash union floor | 34 KiB (70% saveable) | 58 KiB (60% saveable) |

Verify CPU (space-only, 64 MiB, k=64): **1.8 ms per answer.** The VDF verify
on top is Wesolowski (O(log T) group ops — milliseconds). Verification CPU is
not a cost center.

## What the numbers refute and what they leave

- **Parent-dedup (the #299 "cheap interim" hypothesis): refuted at scale.**
  Cross-open index collisions are a small-n artifact (birthday over
  64×5 draws): 12.5% at n=2048 falls to 0.9% at n=16384 and keeps falling.
- **Multiproof compression: real but marginal — ~90 KiB of 1.5 MB (~6%).**
  Proof bytes are only ~10% of the answer.
- **Batch verification: not worth building.** At 1.8 ms/answer the verify is
  already three orders below the loop-occupancy bar; #528's fix removed the
  O(height) replay that multiplied it.
- **The dominant term is structural: 64 label opens × (node + pred + 3
  DRSample parents) × 4 KiB ≈ 1.25 MB of raw plot bytes.** No encoding
  change removes it. The only 10× levers are `DefaultLabelSamples` (k) and
  `BlockSize` — both **soundness parameters** of the G2 labeling check
  (Evolving tier in TENETS Part IX; the exact Sybil-cost parameters are
  "held in tension and re-tuned"), and any change is **research-gated,
  always** (build-process rule 5; #299 itself says a k-reduction "is a
  soundness tradeoff — a deliberate design/parameter decision, not a safe
  unilateral change").

## Decision

1. **Do not build the multiproof/batch-verify tiers now.** 6% wire and a
   non-problem CPU term do not move the phase's exit gate (the publish bound
   re-derives from floor × payload; a 6% payload change does not re-derive
   it "downward" in any meaningful sense). Recorded as available if a later
   soundness-neutral sweep wants the 90 KiB.
2. **Route the 10× lever to research:** consult filed at
   `/Users/andrewedmond/Claude/claude/silt-reviews/research/299-label-samples-answer-size-CONSULT.md`
   asking for the soundness margin of k (64 → what floor?) and the
   BlockSize/parents trade, with these measurements attached. The knee
   (#528) is fixed and field-confirmed, so this is a bandwidth/N²-scale
   question now, not a liveness one — the consult can take its time.
3. **The Phase 3 exit gate (deep sheet h ≥ 128, prune field-exercised) does
   not need the payload change.** With the knee dead, heights accrue at
   steady cadence; the deep sheet is runnable on current parameters and is
   also the RSS-trend check (0.92 → 1.19 GiB run-over-run — watch it).
   Billable: needs the owner's explicit go.

## The honest Phase-3 reframe

"Cheap heights" had two faces: the liveness face (heights starve the run /
the loop) and the cost face (bytes × N² audits + chain weight). #528 closed
the liveness face by construction (O(delta) catch-up), field-confirmed by
the clean sheet. The cost face is a parameter decision that belongs to
research + the owner, not to encoding work. The build track's remaining
Phase 3 items are the deep-sheet gate and (pending consult) the parameter
change with its version-gate story.
