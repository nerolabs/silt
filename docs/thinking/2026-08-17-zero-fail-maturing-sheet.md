# 2026-08-17 — run 82bcd2b-39478: the first ZERO-FAIL MATURING sheet (20 pass / 3 gap / 0 fail)

The 10a re-grade under the complete fix set — and the first MATURING run in the
project's history with no failures:

- **10a-stall-drill PASS inside its computed 430s bound** — the drill that failed at
  90s (window artifact), 370s (view-sync missing), and 430s (gather serialization)
  across three runs. With #453 (rounds converge) + #457 (gathers complete), the
  honest >⅔-weight coalition commits on the wire with a third of the epoch silent.
- **10b PASS including the resume clincher** that GAPped twice (restarting members no
  longer tax every gather their timeout budget).
- Handoff PASS in bound (h59 ≥ target h57); 10c F-1 held; fourth consecutive
  fast full-cohort latch (h22).
- The 3 GAPs: the two known #345/#350 undrivable drills (deterministic homes green),
  and durability-turnover's setup publish on the STALE `PUBLISH_RETRY_S=240` — the
  one bound not yet re-derived for the #453 durations (its commit-wait leg assumes
  the old 64s cycle; honest arithmetic ≈ 34×5 legs + escape-aware commit wait ≈
  ~360). That re-derivation is the remaining harness item.

## The day's mechanism chain, closed

#441 (entries had no seat — mempool content, certified) → #448 (reg-queue ID
priority — FIFO) → #451/#453 (locking without a synchronizer — increasing durations
+ catch-up, field-confirmed by telemetry) → #456/#457 (sequential gathers taxing
dead peers — broadcast-and-collect, coarsely-timed oracle). Each found in the
field, pinned at the cheapest tier that could see it (two required NEW model-check
dimensions: per-node round-advance skew, then the coarsely-timed cost model),
research-certified before build, failing-first proven, and finally graded green on
the wire together.

## Remaining before red-team #183 (PE §7)

1. PUBLISH_RETRY_S re-derivation (harness, small).
2. The soak re-run (launch-regime publish/drain interleave — the PE gate's other
   half; the drill FAILed its first execution on the pre-fix code and has not run
   since the fixes landed).
