---
name: scar-depth-war-lineage
description: SCAR (10+ occurrences, THIRD-TIME RULE LONG EXCEEDED): depth-war lineage #528/#535/#549/#555/#558/#560/#561/#562/#563/#572 — "cost grows with depth," one bug per deep-run cycle, impossible to find by inspection. O(depth) gate proposed.
metadata:
  type: project
---

# Scar: Depth-war lineage — "cost grows with depth"

**Failure class:** Per-height cost that grows with chain depth. Only a hammered deep run
flushes these bugs; inspection fails every time.

**Source citations:**
- MEMORY.md session-7 entry: "THE DEPTH-WAR LINEAGE — ALL CLOSED, field-confirmed (detail
  in the issues + docs/thinking/ + evidence PRs #564/#575/#579/#584/#585): #528, #535,
  #549, #555, #556, #558, #560, #561, #562, #563, #572."
- `docs/decisions.md` D-M1-PIVOT: "field runs stall short of depth (h64) partly on M1
  costs — heavy per-reg proofs, round durations, the 360 s publish bound — so cheaper
  heights are the path TO the M0 field confirmation."
- MEMORY.md "#572 is the shape to remember" entry (see below).

**Occurrence count: 10+ confirmed deep-run bugs across the named issues.**

Third-time threshold was crossed many times over. This is the class-level gate candidate
(see [[class-gate-o-depth-review]]).

## The issues (all closed, field-confirmed)

| Issue | Shape |
|---|---|
| #528 | First h56 liveness knee; node cost growth at depth |
| #535 | Churn-stall at depth (persistent-node cost) |
| #549 | Cost growth in chain replay path |
| #555 | AllEntries O(n) per-block at height n; incremental update needed |
| #556 | Per-height bond proof cost scaling |
| #558 | Round-duration growing with height |
| #560 | Per-height proof size growing (N² bandwidth) |
| #561 | Depth-related GC pressure |
| #562 | Publish-starvation at depth (entry mempool, #441) |
| #563 | Snapshot-sync structural gap (O(all-time state) replay) |
| #572 | Daemon replayed chain.cbor BEFORE wiring bond verifier |

## The canonical shape (#572)

From MEMORY.md: "The daemon replayed chain.cbor BEFORE wiring the bond verifier;
`objective()` = MinBond>0 && verifyBond!=nil, so replay ran LEGACY rep-gated qualification
with an empty boot ledger → validatorsSeen rebuilt EMPTY → everMature latch lost →
restored seats mute forever. Fixed PR #582."

This is not a pure O(depth) cost bug but belongs to the family: a structural coupling
between startup order and replay that only manifests at real chain depth. Inspection of the
code shows no bug; only a replay from real depth exposed it.

## Why inspection fails

These bugs share one property: the per-height operation looks constant in a unit test or a
short sim. The cost or coupling only emerges when the chain has real depth. The model-check
tier catches pure invariant violations; it does not catch cost growth unless the test is
explicitly parameterized on depth.

## The proposed gate

**O(depth) review is a standing check for any hot-path function touching the chain.**
Before merging any PR that adds or modifies a per-height operation:

1. Ask: does per-height cost grow with chain length?
2. If yes, or if unknown: require a measured benchmark parameterized on depth (not a
   constant-height unit test).
3. The `#555 lesson` is the canonical example: AllEntries was O(n) per block at height n —
   green in unit tests, catastrophic at real depth.

This gate is proposed for encoding into PE review checklist and CI. See [[class-gate-o-depth-review]].

## The save/restore regime instrumentation (permanent)

From MEMORY.md: "The save/restore regime lines (`chain.Regime()`) are PERMANENT
instrumentation: a healthy restore is everMature=true(post-latch) + seen>0, ALWAYS."
This is the current in-code scar from #572.

**Links:** [[scar-one-defect-four-costumes]], [[scar-oom-memory-failure-class]], [[class-gate-o-depth-review]]
