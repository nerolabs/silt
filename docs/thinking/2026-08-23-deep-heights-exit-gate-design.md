# The Phase 3 deep-sheet exit gate: DEEP=1 flow design

**Date:** 2026-08-23. **Context:** ROADMAP Phase 3's exit gate — "a deep green
sheet (h ≥ 128) with the prune field-exercised at production parameters" (also
delivering the deferred Phase 1.4 deep run). Prereqs now hold: the #528 knee
is fixed and field-confirmed (clean sheet `585c82a-58990`, h65 by run end),
and the #299 payload question is measured and routed to research — heights
are affordable at current parameters.

## What the gate must observe (real evidence only)

1. **Depth:** the honest ceiling reaches h ≥ 128 on the wire.
2. **Prune engaged:** at fast-TTL 32 (the shipped default — TTL 32 IS the
   production default per the daemon's own narration), safetyDepth = 2·TTL =
   64, so at h ≥ 128 the retention horizon is ≈ h−64 epoch-floored — roughly
   half the chain payload-stripped. Evidence must be on-disk state, not a
   debug log line (the prune narration is LogDebug): `chain-status` grows a
   `pruned:` line counting `Block.IsPruned()` blocks — a real observable an
   operator also benefits from (the same acceptance rationale chain-status
   already documents for flows 5–7).
3. **Steady-state sync still converges over a pruned chain:** post-drive,
   the flow re-runs the flow-5 convergence probe (all validators within 2 of
   tip, identical head hash at tip). This field-exercises the slice-5
   suffix-sync-around-the-pruned-gap property at depth, on the #528 fast
   path.
4. **RSS trend:** the existing 30s sampler covers it; the deep run is the
   trend read (0.92 → 1.19 GiB run-over-run so far; bound is the 2 GB floor).

## Design decisions

- **A DEEP=1 opt-in flow (`flow_deep_heights`), registered after
  `flow_maturing_handoff`** (the destructive-last section — deep continues
  from the post-drills matured chain at ~h65, 12-seat rotation, and the
  maturing flow ends with the cohort restored (the RC run's liveness scan
  passed after it). Entry precondition: every validator active, else GAP
  (premise-degraded, property untested — the require_live rule).
- **Reuse the flow-10 drive machinery verbatim:** publish-driven blocks over
  the organic renewal treadmill, ceiling polling, the #451 topology-aware
  per-height bound (610s at 12 seats), and the #525 freeze early-exit (a
  stalled height grades the stall; the window never burns out idle).
- **Bounds:** per-height 610s (worst case); expected cadence is the observed
  ~40–90s/block, so ~63 blocks ≈ 45–95 min real. The overall window is
  (target − start) × 610s on paper but the early-exit makes the real bound
  the first frozen height. The fleet TTL backstop rises to
  **TTL_MINUTES=300** for DEEP runs (the 180 default would race a slow
  drive; the TTL is the no-leak cost backstop, not a grading bound).
- **Rows:** `12-deep-heights` (ceiling ≥ target in window),
  `12b-deep-prune` (every validator's chain-status pruned-count ≥ the
  arithmetic floor expectation, evidence carries count + on-disk
  chain.cbor bytes), `12c-deep-converge` (flow-5 probe on the pruned
  chain). DEEP=0 records a skip row naming the opt-in, mirroring SOAK.
- **Target:** `DEEP_TARGET` env, default 128 (the gate's number; overridable
  for a cheaper shakeout).

## Options considered and rejected

- **A standalone deep sheet (launch regime, no MATURING):** loses the
  matured/post-shed regime — the regime the external red team targets and
  the one whose weight builds fastest (12 bonded renewing). The full-sheet
  continuation reaches depth for one fleet's money and grades the whole
  surface en route (the "deep green sheet" reading).
- **Asserting prune via journal debug lines:** rejected — harness evidence
  reads real state (field-test immutable #3); LogDebug may not be captured.
- **Coupling the persistent-VPC switch (`net-up`) to this run:** rejected —
  one variable at a time; the per-run VPC path is the proven one. Apply
  net-up on a cheap shakeout run later.
- **Raising drive cadence (parallel publishers):** rejected for now — the
  organic treadmill + one-at-a-time drive matched real cadence in three
  runs; parallel publishers change the load shape the bounds were priced on.

## Local proof (the billable gate)

The wall-clock-at-depth leg has no local analogue (the sanctioned n/a case),
but the integrations the flow grades are locally green and cited as the
RUN_LOCAL_PROOF: `core/node` prune/suffix-sync suite
(`TestSuffixSync_CatchUpAroundPrunedPeer`, `TestSuffixSync_DeepCold…`,
`TestSuffixAppend_*`) + the new chain-status pruned-count unit test.
