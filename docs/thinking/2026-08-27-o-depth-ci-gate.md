# The O(depth) CI gate — turning the memory-growth diagnostic into a pass/fail assertion

Date: 2026-08-27
Author: Builder seat
Status: built; PR open, pending blind PE review

## The decision

Add `TestPerHeightCostLinear` to the `sim` package: a depth-parameterized test that
drives the mature-epoch consensus network to increasing heights, measures per-height
cost, and **fails on super-linear growth**. It runs in the standard CI `go test`
job (no opt-in env gate), completes in ~6 s, and is the standing gate for the
depth-war failure class (#528/#535/#549/#555/#556/#558/#560/#561/#562/#563/#572 —
"per-height cost grows with chain depth").

The Tester specified this gate in `.claude/agent-memory/tester/class-gate-o-depth-review.md`
and Andrew chose the CI-check form (not a review checklist).

## Why a unit test cannot catch this class

A unit test at fixed height h is *always green* for an O(depth) bug. The canonical
incident is #555: `AllEntries` was O(n) per block at height n — green in every
constant-height test, catastrophic at real chain depth. Only a benchmark
parameterized on depth surfaces it. The gate must measure the *slope* of cost vs
depth, not cost at a point.

## The metric: baseline-subtracted HeapObjects

Two candidate metrics, both already sampled by the diagnostic
(`sim/oom_growth_diag_test.go`, `TestConsensusMemoryGrowth`):

| metric | linear? | noise | verdict |
|---|---|---|---|
| `runtime.MemStats.HeapObjects` | yes, 234 obj/height | **deterministic** across runs | **primary signal** |
| `runtime.MemStats.HeapInuse` | yes, ~26 KB/height | GC arena steps (granular) | too noisy for a tight bound |

Measured baseline (seed=11, 4 honest + 9 sybil, EpochBlocks=4), driving real wire
commits, GC before each sample:

```
h=250   HeapObjects=60541   (60529 on a second machine-warm run — see below)
h=500   HeapObjects=119047
h=1000  HeapObjects=236073
h=2000  HeapObjects=470119
h=0     HeapObjects=1757   (fixed overhead: identities, endpoints, genesis)
```

HeapObjects is **deterministic** across repeated runs (the seeded sim + a forced GC
gives the identical count at each height; only the live-object sample at the exact
same height varies by a handful of objects from GC timing). This is the clean signal.

**Baseline subtraction is load-bearing.** The h=0 count (1757) is fixed overhead that
does not grow with depth. If you take the raw ratio `HeapObjects(2000)/HeapObjects(1000)`
you dilute the depth signal with constant overhead and the ratio drifts below 2.0 even
for a purely linear system. Subtracting the h=0 baseline isolates the growth that is
*attributable to depth*:

```
growth(h) = HeapObjects(h) - HeapObjects(0)
growth(250)=58784  growth(500)=117290  growth(1000)=234316  growth(2000)=468362
per-height = 235.1, 234.6, 234.3, 234.2   → flat ⇒ linear
```

## The bound: a ratio (doubling) test

For linear cost, doubling the height doubles the depth-attributable cost:
`growth(2h)/growth(h) → 2.0`. For the super-linear class we defend against
(O(n) per block ⇒ O(n²) total, the #555 shape), the ratio → **4.0**.

Measured baseline ratios:

```
growth(1000)/growth(500)  = 1.998
growth(2000)/growth(1000) = 1.999
```

The separation between linear (2.0) and super-linear (4.0) is a factor of two. The
gate asserts:

> `growth(2H)/growth(H) < 2.6` at two stages: H=500→1000 and H=1000→2000.

### Why 2.6

- The measured linear baseline is 1.998–1.999. A threshold of 2.6 sits **30 % above**
  the baseline — no legitimate linear growth reaches it.
- A super-linear O(n²) defect sits at ~4.0 — **54 % above** the threshold. Caught with
  wide margin.
- 2.6 also tolerates a *mildly* super-linear component (e.g., an O(log n) factor:
  n·log n total gives ratio ≈ 2·(log 2000/log 1000) ≈ 2.2 at these heights) without
  flagging — that is intentional. The gate targets the polynomial-degree regressions
  the lineage is made of, not a log factor. A log factor is not what OOM-looped the
  field cohort; a linear-per-block factor was.

### The two-stage requirement

Checking the ratio at **two** doublings (500→1000 and 1000→2000), both of which must
pass, guards against a single-point fluke and confirms the *slope* is stable across
the range, not just at one pair. A real O(n²) defect fails both stages; GC noise
cannot fail both at 2.6 (baseline is 1.998 at both).

## False-positive analysis (the flake question)

The gate must not flake on legitimate linear growth or GC noise. Sources of variance
and why each is contained:

1. **GC timing on HeapObjects.** Contained by `runtime.GC()` immediately before each
   sample (already in the diagnostic) — this forces a collection so the count reflects
   *reachable* objects, which the seeded sim makes deterministic. Measured run-to-run
   variance at a fixed height is < 20 objects out of 60k–470k — five orders of
   magnitude below the 30 % headroom to the 2.6 threshold.
2. **Legitimate linear growth.** Ratio is 1.998, threshold is 2.6. A 30 % headroom.
   For the gate to false-fire, per-height object growth would have to *increase* by
   30 % over the second half of the chain — that is itself a super-linear signal, which
   is exactly what the gate should catch. There is no linear regime that trips it.
3. **Go runtime / version drift.** The ratio is dimensionless and baseline-subtracted,
   so absolute allocation differences between Go versions cancel. Only a change in the
   *shape* of growth moves the ratio.
4. **HeapInuse excluded from the assertion.** HeapInuse steps by GC arena granularity
   (measured 2.057 at one doubling — above 2.0 from a step landing mid-range). It is
   logged for human attribution but NOT asserted on, precisely because its steps would
   flake a tight bound. HeapObjects carries the assertion.

Residual flake risk: effectively zero on the object-count metric at a 2.6 bound with
two-stage confirmation. If a future legitimate change adds a genuinely super-linear
(degree > 1) per-height cost, the gate SHOULD fail — that is the gate working, and the
fix is to make the cost linear (as every lineage fix did: #555's incremental update),
not to loosen the bound.

## Cost / CI budget

The diagnostic drives 2000 real wire commits in **~6 s** wall (measured: 5.66 s to
h=2000, 5.93 s total). It runs in the existing `go test` job. No new CI job, no opt-in
env gate. The gate is on by default so a regression is caught on the PR that introduces
it — the whole point of a standing gate.

To keep the default `-short` suite fast, the gate honors `testing.Short()`: under
`-short` it runs the reduced ladder (250→500→1000, ~1.5 s) which still spans two
doublings and catches the O(n²) class; the full 500→1000→2000 ladder runs in the
non-short CI `go test` job. Both ladders assert the same 2.6 two-stage bound.

## What this gate does NOT do

- It does not prove any *specific* function is O(1) per block. It proves the
  aggregate per-height memory-object cost of the consensus hot path is linear in
  depth. A regression in any hot-path structure that accumulates per-block super-
  linearly (the lineage shape) moves the aggregate ratio and trips the gate.
- It measures memory objects, not wall-time. Time-based super-linearity (an O(n)
  scan per block that allocates nothing) would not move HeapObjects. This is a known
  gap; the lineage incidents were memory-accumulation (#555 `AllEntries` built an
  O(n) slice per block ⇒ objects). A time-dimension gate is a possible follow-on but
  is out of scope here and would need its own noise analysis (wall-time is far noisier
  than a seeded object count in CI).

## Boundary check

This is a TEST addition proving an existing cost property. It touches no chain
hot-path code, no consensus rule (I1–I5), no published claim, no economic mechanism.
Not research-gated. It observes; it does not refactor.

## Failing-first proof (required — a gate is decoration until its defect goes red)

Injected a synthetic O(n)-per-block accumulator into the drive loop (a slice that
grows by `h` entries at height h ⇒ O(n²) total objects) behind a test-only env flag,
confirmed the ratio jumps to ~3–4 and the gate goes RED, then removed it and confirmed
GREEN. Evidence recorded in the PR body.
