# Field-test report — run 6a38d7b-42691 (2026-08-19)

**HEAD:** 6a38d7b (post-Phase-1 + Phase-2 Slices 1–3; economy **default OFF**).
**Topology:** full cert config — 4 anchors + 4 maturers + 8 sybils + store/fetch/relay/registry/NAT,
3 regions (us-west1 / us-east1 / europe-west1), ALL on-demand (preemption-safe graded run), SYBILS=8.
**Purpose (build-immutable #6 justification, recorded in run-justification.log):** validate the new
Phase-1.3 RSS/memory telemetry on real infra + a fresh graded pass (the prior committed report was a
stale pre-fix run, per the fresh-eyes audit). D-CONSENSUS gate green locally first (30 model-check tests).

## Grade: 19 PASS · 2 SKIP · 2 GAP · **0 FAIL**

A clean graded pass on a **healthy** network (`infra-node-liveness` PASS — no OOM-kill or crash-loop).

### The headline: memory-safety field-confirmed with a committed artifact
`infra-node-memory` **PASS** — the Phase-1.3 cgroup RSS sampler ran on real infra and produced
`rss-6a38d7b-42691.jsonl` (382 samples, every 30 s). **Worst peak across the whole cohort: 1.25 GiB**
(val-c, the busiest node), every other node well below; finals ≈ peaks (flat — no leak/climb). This is
the first committed RSS envelope backing the "return-to-2GB" memory claim the audit flagged as
resting on uncommitted prose. The OOM arc (Phases #464/#465/#470 + 1.3) is now field-evidenced, not asserted.

### The two GAPs — both attributed to the HARNESS, not silt (build-immutable #7, from the captured journals)

- **`5-sybil-no-capture` (GAP, major) — a capture false-positive, NOT a #402-class fork.** The verdict
  flagged sybil-4 at h48 as a "divergent fork," but its own data shows `anchor=unreadable` and a
  height mismatch (sybil h48 vs the anchor's last-*readable* h47) — it compared a sybil head against a
  hash it could not read. Attribution from the anchor journals: **the chain was clean.** All four
  anchors held ONE consistent chain to h49 with real quorums (val-b committed h48 with **11
  attestations**); the #402 strict-anchor-majority fix was active ("strict majority 3 required
  (objective; derived #402)"); sybils are excluded from the finality quorum; and **zero**
  equivocations / slashes / pre-finality-reorgs fired across any node. A sybil (min-bond, non-anchor)
  sitting on a different local head cannot finalize a fork — this is expected skew, misread as a fork
  because the anchor hash was uncaptured. **Fix belongs in the harness fork-detector** (read the
  anchor's committed tip reliably and compare at the SAME height, or compare the sybil against the
  committed-anchor tip), not in silt. Filed: the harness-robustness issue.
- **`184-partition` (GAP, major) — slow catch-up, not a reconvergence break.** val-c was one block
  behind on the SAME chain at heal (committed h45 while the tip was h47) and was the busiest node (RSS
  peak 1.25 GiB, cohort max). One block behind on one chain = catch-up latency vs the tight 120 s
  window, not divergence. Window-tuning / a known catch-up characteristic, not a safety break.

### The two SKIPs — both justified, not gaps in coverage
- **`184-equivocation` (SKIP)** — the destructive double-sign drill runs on its own dedicated ephemeral
  net (mid-sheet eviction would pin 3-of-4 against 3 live anchors = zero fault tolerance); certified by
  the e2e `TestEquivocatorSlashedOverTCP` + integration/adversarial, merge-gated by the in-process
  oracle (PE ruling 2026-08-17).
- **`10-maturing-handoff` (SKIP)** — the maturing/handoff regime drill; skipped this sheet (its
  premise/setup was not met on this run).

## What this run did NOT test (needs harness prep — the economy/§0.1 run, "Run B")
Every publish used `-chunk-size 65536` (64 KiB **sim** size), there is no `-care` caretaker in the
topology, and `-economy` is not wired into `topology.py`. So this run exercised neither the S7 repair
economy (default OFF) nor the §0.1 repair-RAM at production chunk size (the 64 KiB size hides the
~640 MiB–1 GiB reconstruction spike ~1000×). Those require the Run-B harness prep (a reconstructing
caretaker at production chunk size + the `-economy` wiring + a fund→kill→verify-`g` scenario).

## Artifacts committed with this report
`rss-6a38d7b-42691.jsonl` (the RSS envelope), `results.jsonl` (the graded verdicts),
`console-6a38d7b-42691.log`, `flow-evidence-6a38d7b-42691.log` (the captured GAP journals).
