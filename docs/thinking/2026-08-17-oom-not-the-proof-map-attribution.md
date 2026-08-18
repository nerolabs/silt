# The MATURING OOM is not the proof map — attribution restart + flixz unblock

**Date:** 2026-08-17 (eve) · **Branch:** `diag/consensus-oom-heap-profile` ·
**Companion:** `silt-reviews/research/silt-oom-NOT-the-proof-map-FINDING-2026-08-17.md`
(full evidence), `silt-reviews/principle-engineer/proof-oom-fix-review-PE-2026-08-17.md`
(PE concurrence on #464).

## What happened

The proof-map fix (#464) shipped and its CONFIRM run (`38bb475-oomfix`, MATURING=1
SYBILS=8) **still FAILed `infra-node-liveness`: 61 kernel OOM-kills** — same
magnitude as pre-fix. So the run FALSIFIED the triage's root cause. This doc
records the deliberation for the restart, so the next builder doesn't re-blame the
proof map.

## Why it's not the proof map (structural, not a hunch)

Kills concentrate on CONSENSUS/cohort nodes (adversary×8, maturers×20, sybils×18,
validators×14) — the STORAGE nodes that actually hold chunks barely died
(store-2×1, store-1×0). The proof map is O(total held chunks); a node holding ~no
chunks has a ~empty proof map, so bounding it cannot save it. Same distribution was
in the triage-prompting run (76f654d). The triage reasoned from a code read — **the
daemon had no heap profiling**, so the real hog was never measured.

## What I measured (both RULE OUT small-scale causes)

- **In-process consensus core** (`sim/oom_growth_diag_test.go`, `SILT_OOM_DIAG=1`,
  300 heights): HeapInuse grows ~23 KiB/height (chain retains full block history,
  but blocks are tiny) → ~2.5 MiB at field h~60. Core ruled out.
- **2-validator bonded daemon** under continuous bond audits: stable ~55 MiB RSS,
  13 goroutines, no growth, no goroutine leak.

**Inference:** no hard leak (stable at small scale). The OOM is a full-scale
MATURING phenomenon — a large-but-bounded working set (21 nodes, big blocks, bond
churn) colliding with Go's default 2×-heap GC target on a 2 GB box. That is a
GC-pacing OOM, whose cheap, well-motivated mitigation is a soft heap limit.

## Decisions

1. **Ship `-mem-limit` (GOMEMLIMIT) now** — unblocks flixz (blocked on a
   memory-performant head) with a memory-bounded daemon. Honest caveat: if the LIVE
   set genuinely exceeds the box it thrashes instead of fixing — which is itself the
   discriminator (survives ⇒ GC-pacing; still OOMs ⇒ working set > box).
2. **Instrument for real attribution, don't guess the hog** (build-immutable #7).
   `-debug-addr` (pprof + SIGUSR1 heap dump) on the daemon; `DEBUG_PROFILE=1` +
   `./cloudtest.sh heap <node>` on the harness. The next step is a `DEBUG_PROFILE=1`
   MATURING run → heap-profile a consensus node before OOM → attribute → fix with a
   memory-wall regression.
3. **Keep #464** — it is a correct O(held) bound with the full proof set (PE
   concurred); it matters at storage scale, it's just not THIS OOM. Not reverted.
4. **Harness honesty:** `infra-node-liveness` hard-codes the now-falsified
   `"Root: the resident PoR proof map"`. Fast-follow: report the crash-loop as a
   blocker WITHOUT asserting an unverified root cause.

## Status of the larger arc

Red-team #183 is GATED behind a real memory-performant head. The path is now:
`-mem-limit` unblock (flixz) + instrumented run → attribute → real fix → clean
MATURING → #183.
