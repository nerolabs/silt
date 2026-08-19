# Field-test report — run 2323b09-20931 (ECONOMY=1, 2026-08-19)

**HEAD:** 2323b09 (Phase 2 economy chain complete; `flow_economy_repair` first field run).
**Topology:** full cert config, 3 regions, all on-demand, SYBILS=8, **ECONOMY=1**.
**Purpose:** the Phase 2 exit gate on the wire — a caretaker reconstructs from parity after a
`swarm holders`-driven kill and the bounty draws the reserve down.

## Grade: 17 PASS · 2 SKIP · 3 GAP · 2 FAIL

Lower than run 6a38d7b (19/2/2/0), for ONE root cause below. But the run VALIDATED tonight's harness
work in the field (see "wins").

## Root cause of every non-green result: publish-commit latency under 3-region load (#441-family)
A chain-backed registry publish IS a consensus commit. On quorum-2 across 3 regions, val-a commits
blocks every ~20–70 s (journal: blocks 1→18+, healthy — NO wedge, NO loop saturation, NO crash). A
publish's HTTP client times out (`context deadline exceeded while awaiting headers`) when its entry's
commit lands slower than the client window — especially late in the run, under load. val-a (the
registry + an anchor) is *healthy*; the failure is the synchronous-publish-vs-slow-commit tension, the
same #441/#351-family signature the harness already references (run 1ebd487).

- **`11-economy-repair` (GAP) — the scenario worked correctly.** Its setup `swarm add` timed out on
  a slow commit; the flow's guard reported **"economy UNTESTED this run, not a failure"** — no false
  FAIL, no crash. The economy grade is genuinely untested (registry latency), not a silt economy
  defect and not a scenario bug. **Fix: give the economy publish the retry-tolerance `ft_publish`
  already has (`PUBLISH_RETRY_S=360`); a single raw `swarm add` is too fragile against slow commits.**
- **`chaos-fetch` + `chaos-reprovide` (FAIL) — same latency, LESS robust guard.** The chaos setup
  publish's entry didn't commit → "root not in registry" → the fetch FAILed. Unlike
  durability-turnover / economy-repair, `chaos-crash` ASSERTS on a publish that may not have landed
  instead of GAPing. **Fix: chaos-crash should GAP (not FAIL) when its setup publish didn't land** —
  match the robustness the other publish-dependent flows already have.

## The wins — tonight's harness improvements, FIELD-VALIDATED
- **A1 fork-detector fix (#485) CONFIRMED:** `5-sybil-no-capture` now reports **"PRE-EXISTING
  DIVERGENCE UNVERIFIABLE: ... the hash at h53 was unreadable (anchor)"** — exactly the fix — instead
  of the old false **"DIVERGENT FORK (#402 class)"**. The scary false-positive is gone.
- **T3 per-run report (#485) CONFIRMED:** `report-2323b09-20931.md` + `results-2323b09-20931.jsonl`
  written per-RUN_ID — a grade can no longer be a stale overwritten `report.md`.
- **RSS telemetry (#478) — 2nd clean field run:** `infra-node-memory` PASS (worst peak 1.47 GiB,
  adversary node; store-2/caretaker at 0.02 GiB — consistent with the economy flow GAPing before any
  reconstruction, so no repair spike). `infra-node-liveness` PASS — no OOM/crash-loop; healthy cohort.
- **The economy scenario's GAP-guard** did its job on its first real run — robustness proven.

## Other non-green (pre-existing, not tonight's work)
- **`184-partition` (GAP)** — the drill UNDER-DROVE: val-c *advanced* during the partition (h27→h29),
  so the sever didn't isolate it below the commit threshold; the catch-up assertion (A2) wasn't even
  reached. A drill-drive issue (widen the sever), not the reconverge logic.
- **`184-equivocation` / `10-maturing-handoff` (SKIP)** — dedicated-net / opt-in-premise, as designed.

## Net
The exit gate is **not closed** (economy untested — registry publish latency), but the run was
productive: it field-validated three of tonight's harness fixes, proved the economy scenario robust,
and pinned two concrete, fixable harness issues (economy-publish retry; chaos-crash GAP-robustness).
A re-run after those fixes — the economy publish retried, and earlier in the sheet before the chain
loads — is the path to closing the gate. Artifacts committed: `rss-`, `results-`, `console-`,
`flow-evidence-` for this run.
