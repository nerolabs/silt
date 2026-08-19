# Pace-before-build — the cloudtest harness: defects, gaps, and improvements

**Date:** 2026-08-19 · **Why:** billable field runs are long (~40 min base, ~90 min with the
maturing drill) and cost real money, so before spending more we audit the harness holistically —
all defects, all improvements, prioritized — rather than bolting on the economy run piecemeal.
Owner ask (2026-08-19): "think deeply on where we can make improvements." This is the plan; nothing
here is built yet.

## 0. The run's time anatomy (where the minutes actually go — measured from run 6a38d7b-42691)

| Phase | Cost | Note |
|---|---|---|
| `apply` (terraform provision 33 resources, 3 regions) | ~3–5 min | one-time |
| `wait_ready` (VM boot + GCS binary pull + systemd) | up to 600 s deadline; ~2–4 min real | binary pull is fast (GCS curl); cost is VM boot |
| **17 scenarios, STRICTLY SERIAL** | the bulk | independent flows run one-after-another |
| `flow_maturing_handoff` (opt-in, MATURING=1) | **LATCH 1100 s + HANDOFF 1980 s ≈ 51 min** | must cross real epoch boundaries at real cadence — intrinsically long |
| `report` + `teardown` (destroy 33 resources) | ~2–3 min | network delete is the slow tail |

**The two dominant levers: the serial scenario chain, and the ~51-min maturing drill.** The run I did
(~40 min) SKIPPED maturing; a full cert run is ~90 min.

## 1. TIME & COST (the owner's priority)

- **T1 — scenarios run strictly serially (`run_all_scenarios`, ~17 flows).** Many are independent and
  touch disjoint nodes (web-ui-guard, publisher-unlinkability, care-link, takedown, cross-nat) — they
  could run **concurrently** or in **tiers**. *Caveat:* flows that kill/restart/partition shared nodes
  (fault-tolerance, restart-survival, chaos, the #184 drills, maturing) MUST stay serial and isolated —
  a kill in one corrupts another. So the win is a **two-lane split**: a "read-only/independent" lane
  (publish, fetch, care-link, ui-guard, unlinkability, takedown) run concurrently, and a "destructive"
  lane run serially. Estimated saving: the independent lane collapses from sum-of-latencies to its
  slowest member — several minutes off a base run.
- **T2 — maturing-handoff (~51 min) should be its OWN dedicated run, not bundled.** It dominates any
  run it's in, and it stops validators (it runs LAST for that reason). Splitting it out means the base
  cert run is a tight ~30–40 min and the maturing cert is a separate, clearly-scoped ~55 min run. Also
  re-examine whether HANDOFF_BLOCKS_S=1980 (9 blocks × 220 s worst-height) can shrink with the #451
  synchronizer's real cadence (it was sized against a pre-#451 worst case; run e2fab4b measured
  80–170 s/height — so 9 × 170 ≈ 1530 s might be the real bound, saving ~7 min).
- **T3 — report.md / results.jsonl are OVERWRITTEN each run** (not per-RUN_ID, unlike the console /
  flow-evidence logs which ARE per-RUN_ID). **This is the exact cause of the fresh-eyes audit's "the
  committed report.md is a stale pre-fix run"** — each run clobbers the last, so a grade is lost unless
  force-committed immediately. **Fix: write `report-<RUN_ID>.md` / `results-<RUN_ID>.jsonl` and an
  evidence bundle per run**, so no grade is ever silently lost. Cheap, high-value, closes an audit
  finding permanently.
- **T4 — no fast "core-confirm" tier** between SMOKE (4-node shakeout) and the full cert. A mid-tier
  (core flows, no sybil cohort, no maturing, single-region) would give a ~10–15 min post-change
  confirmation without the full ~40–90 min spend. Add `TIER=core` alongside `SMOKE=1`.

## 2. COVERAGE GAPS (what silt properties the harness cannot test today)

- **C1 — the S7 repair economy is untestable.** `-economy` is not wired into `topology.py`; there is no
  fund→skim→kill→verify-bounty→measure-`g` scenario. The Phase 2 exit gate (**`g` measured on the
  wire**) has no home. [Slice 4]
- **C2 — the repair LOOP is never exercised end-to-end.** There is **no `-care` caretaker** in the
  topology. `durability-turnover` only proves survival by fetching from a *surviving replica*
  (`store-2`) — no shard is ever *reconstructed*. So the core "repair outruns failure" property (S2 /
  the whole reason the repair loop exists) is **unverified on the cloud.** A caretaker that reconstructs
  a killed stripe is the missing piece — and it's the SAME piece C1 needs.
- **C3 — production chunk size is never used.** Every publish is `-chunk-size 65536` (64 KiB sim size).
  This **hides the §0.1 repair-RAM spike ~1000×** (a 64 MiB-chunk stripe reconstructs 640 MiB–1 GiB;
  a 64 KiB one reconstructs 640 KiB), and means large-file publish, real striping, and realistic
  bandwidth are all untested.
- **C4 (future, Phase 4) — bandwidth / Proof-of-Delivery** has no test surface. Out of scope now; noted.

## 3. THE KEY STRATEGIC INSIGHT — §0.1 is a LOCAL measurement; don't buy a cloud run for it

**MEASURED 2026-08-19 (`core/erasure/reconstruct_mem_test.go`), for $0:** reconstructing one
`DefaultParams` (k=10,n=16) stripe holds, RESIDENT: **1.0 MiB at the 64 KiB sim chunk size vs
1.0 GiB at the 64 MiB production minimum** — the ~1000× the cert predicted (1024×), confirmed. On a
2 GB floor box that leaves ~1 GB, and the Run-A daemon baseline was 0.5–1.25 GiB — so **repair +
baseline can exceed 2 GB → OOM. Production-chunk-size repair does NOT comfortably fit the 2 GB box.**
Consequence for Run B: the economy grade must run on a **bigger box** (e2-medium/standard) OR land the
**streaming/column-wise decode** mitigation first, or it will OOM on the first reconstruction (an
outcome the sim's 64 KiB size would have hidden entirely). This is the §0.1 gate ANSWERED — no cloud
run spent. (Original reasoning below.)


The §0.1 gate ("measure repair RAM at production chunk size before the economy-ON grade") is a
**single-node property**: how much RAM does `erasure.ReconstructStripe` hold when rebuilding one
production-chunk-size stripe? `ReconstructStripe` holds the whole stripe as `[][]byte`
(`erasure.go:99`), so it is `k × shardBytes` resident — measurable by a **local Go benchmark** with
`-benchmem` at 64 MiB shards, for **$0**. Build-immutables #6/#7: reproduce locally first; the cloud
*confirms*, it never *discovers*. **So the §0.1 gate should be a local benchmark, not a billable run** —
and it should run BEFORE we build cloud repair, because if a production stripe can't be reconstructed
within a 2 GB box at all, that's a mechanism finding (streaming/column-wise decode) that changes what
cloud repair even looks like. **Recommend: build the local `BenchmarkReconstructStripe` first.** Only
the economy grade (`g` under real multi-node churn) genuinely needs the cloud.

## 4. ATTRIBUTION & ROBUSTNESS (false verdicts / capture gaps)

- **A1 — #482 (filed): the sybil-fork detector false-positives** on an unreadable/height-mismatched
  anchor hash. It cost real attribution time this run (a scary "#402 fork" on a provably clean chain).
  Fix: read the anchor's committed tip reliably, compare at the SAME height, or SKIP when the premise
  is unreadable (the verdict already prints "capture premise unmet" yet grades a fork).
- **A2 — the `184-partition` 120 s catch-up window is too tight under load.** val-c was one block behind
  on the same chain (not diverged) and the busiest node (RSS peak 1.25 GiB) — a slow-sync GAP, not a
  reconverge break. Make the window **load-adaptive** (scale with observed height cadence) or widen to
  match the #451 synchronizer's real catch-up time.
- **A3 — the new RSS telemetry is coarse** (30 s cgroup-total). High-value extensions: **per-flow
  correlation** (tag each sample with the running scenario so a peak is attributable to its cause),
  a **peak-triggered pprof heap snapshot** (when RSS crosses a threshold, pull `/debug/pprof/heap` so
  the *composition* of a spike is captured, not just its size — exactly what §0.1 wants), and **keep
  the sampler running during teardown** to catch a late OOM.

## 5. FIDELITY (harness ≠ production)

- **F1 — sim chunk size (64 KiB vs 64 MiB prod)** is the root of C3 and the §0.1 blind spot. The fix is
  a per-scenario chunk-size knob (default stays small for the fast consensus flows; the durability /
  economy / §0.1 flows publish at production size).
- **F2 — no caretaker** is the root of C2.

## 6. Recommended sequence (cost/benefit ordered)

1. **FREE, now — §0.1 local benchmark** (§3). Measures the gate for $0; may reshape cloud repair before
   we build it. *No billable run.*
2. **Cheap harness fixes** — T3 (per-RUN_ID report/results, closes the audit's evidence-loss
   permanently), A1 (#482 fork-detector), A2 (partition window). Small, high-leverage, no run needed.
3. **The one big build that unlocks Phase 2 — the reconstructing-caretaker + production-chunk-size +
   `-economy` + the economy scenario** (C1+C2+C3+F1+F2 are ONE coherent build: a caretaker that
   reconstructs a prod-chunk-size object after a kill, with the economy on, verifying the bounty pays
   and measuring `g`). Add the T1 two-lane split and T4 core tier alongside if cheap. Locally validate
   the shell before spending.
3a. **Gate the economy Run B on the §0.1 result** — if the local benchmark shows a prod stripe won't
   fit the 2 GB box, size the economy run's box up (e2-medium/standard) OR land the streaming-decode
   mitigation first, so the economy grade isn't invalidated by an OOM the benchmark already predicted.
4. **Then the billable economy Run B** — the Phase 2 exit gate (`g` on the wire), on a box the §0.1
   benchmark proved can hold repair, with A3's per-flow RSS + peak-pprof capturing the real footprint.
5. **T2 — split maturing into its own dedicated run** (opportunistic; it already only runs on
   MATURING=1).

**The through-line:** measure §0.1 for free first, fix the cheap harness defects, then make the ONE
coverage build that turns the harness from "tests survival at sim scale" into "tests the repair economy
at production scale" — and only then spend the economy run, on evidence.
