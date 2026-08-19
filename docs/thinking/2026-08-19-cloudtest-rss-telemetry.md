# Phase 1.3 — RSS/heap telemetry in cloudtest (deliberation before code)

**Date:** 2026-08-19 · **Roadmap:** the ordered path Phase 1.3 (evidence hygiene) ·
**Discipline:** build-immutable #7 applied to our OWN headlines — a memory claim must
carry a citable, in-repo artifact.

## The gap (evidence, #7)

The 2026-08-19 fresh-eyes audit found **no committed RSS artifact backs "return-to-2GB"**
(the MATURING OOM headline, PRs #464/#465/#470). Verified in the harness today:
`integration/cloudtest` captures memory only as **binary crash detection** —
`scan_node_liveness` greps each node's journal for the kernel OOM-kill / Go-fatal
signature (`lib.sh:195`) — plus an **on-demand** heap profile (`./cloudtest.sh heap
<node>`, needs `DEBUG_PROFILE=1`). There is **no continuous RSS time-series**: nothing
records that RSS rose, plateaued, and stayed bounded. "Did it crash?" is captured;
"what was the envelope?" is not. So the return-to-2GB claim rests on the *absence* of a
crash, not on a *measured* ceiling — exactly the vibes-not-evidence gap #7/V4 forbid for
a shipped headline.

The other half of 1.3 — "commit the field-run artifacts" — is already satisfied: the
console / flow-evidence logs for prior runs are tracked (16 files), and the working
tree is clean. Only the RSS telemetry is missing.

## What the artifact must answer

The MATURING OOM claim is a *ceiling + steady-state* claim: RSS climbs during maturation,
peaks, and returns to a bounded plateau (~2 GB) rather than climbing to the VM limit and
OOM-killing. That is a **time-series** question — one number (peak) plus the shape
(does it plateau or climb). So the artifact is a per-node RSS series sampled across the
run, summarized to peak + final + plateau-vs-climb.

## Options (PACE)

- **A — controller-side background sampler, cgroup memory (CHOSEN).** A background loop
  during scenarios reads each node's `systemctl show silt.service -p MemoryCurrent`
  (the cgroup's live RSS-equivalent, one clean value, no PID lookup) every
  `MEM_SAMPLE_INTERVAL` (default 30 s) and appends `{node, rss_bytes, ts}` to a committed
  `rss-<RUN_ID>.jsonl`. At run end `scan_node_memory` computes peak / final / sample-count
  per node and records a `node-memory-envelope` finding carrying the numbers. Strictly
  additive, failure-tolerant (a missed read never fails the run), reuses `ssh_node` +
  `record` + `node_names`. ~40 lines.
  - *Cost:* 12 nodes × ~2 s SSH per sweep at a 30 s interval is comfortable; over a
    ~10 min run that is ~20 samples/node — enough for peak + plateau shape. Coarse by
    design: this is an envelope, not a profiler (that stays the on-demand pprof path).
- **B — per-node self-sampling + pull at report.** Each node samples its own cgroup into
  a local file; the controller pulls at teardown. Cheaper per sample (no per-sweep SSH
  fan-out) and denser, but needs a provisioning change (a sampler unit on every node) —
  more surface, more to break, and the density isn't needed for an envelope. Deferred; if
  a future claim needs sub-second resolution, this is the upgrade.
- **C — scrape `/debug/pprof/heap` inuse_space each interval.** Measures Go *heap*, not
  process *RSS* — and RSS (what the OOM-killer acts on) is the claim. pprof heap misses
  off-heap/mmap/fragmentation. The on-demand pprof path already covers attribution; the
  envelope wants RSS. Rejected as the primary signal (kept as the attribution tool).

## Why cgroup MemoryCurrent (not /proc VmRSS)

silt runs as a systemd service; `MemoryCurrent` is the cgroup's accounted memory — the
exact quantity `GOMEMLIMIT` and the OOM-killer act against, and a single value with no
PID discovery. `/proc/<pid>/status VmRSS` needs the PID and misses cgroup accounting
nuance. Same source the kernel OOM decision uses ⇒ the honest number for the claim.

## Scope honesty (#7)

This ships the **mechanism**, verified locally on the parse/summary logic (a synthetic
series → peak/final). The **artifact itself** lands on the next real run (Phase 1.4's
deep run), which will also carry the E5 floor-box drain rider. No billable run is spent
here to "produce evidence" — the harness is built and unit-checked; the run that needs it
produces it. Stating that split rather than implying the artifact already exists.

## The report

`gen_report.sh` surfaces `node-memory-envelope` like any finding; the raw
`rss-<RUN_ID>.jsonl` is committed alongside the console/flow logs, so "return-to-2GB"
becomes a citable file + a peak number per node, not a headline.
