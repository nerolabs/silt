---
name: ruling-613-odepth-ci-gate
description: Blind review of PR #613 (sim/TestPerHeightCostLinear O(depth) gate) — SHIP-WITH-CHANGES; failing-first proven by my own run; scope is memory-only
metadata:
  type: project
---

PR #613 adds `sim/TestPerHeightCostLinear`, a standing CI gate for the depth-war
failure class. Blind PE review 2026-08-28. Verdict: **SHIP-WITH-CHANGES** (high
confidence). Full ruling:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-613-odepth-ci-gate-2026-08-28.md`

**What I verified by running it myself (not on faith):**
- Failing-first is REAL: `SILT_ODEPTH_INJECT=1` drives both doublings red
  (ratios 3.030/3.361), non-zero exit, error names the class + fix. Green = 1.998/1.999.
  Matches PR body byte-for-byte.
- Bound derived from measurement (2.6 vs measured 1.998 baseline). NOT the prior 7×-loose
  scar — a 1.5× O(n²) already trips at 3.0.
- Deterministic: 3 repeat runs byte-identical growth values. Rides HeapObjects after
  forced GC on a seeded sim. HeapInuse logged not asserted.

**The two framing corrections owed (non-blocking):**
1. PR body says "full CI 500→1000→2000" but `.github/workflows/ci.yml:44,65` run
   `go test -short` on every PR/push — the standing gate is the **250→500→1000** ladder.
   Wide ladder runs only on release.yml. Correctness fine (short spans two doublings);
   wording overstates what guards main.
2. Gate is MEMORY-shaped (HeapObjects). #555 (`AllEntries`, chain.go:3216) is pure
   memory → caught. But #528's own doc says its knee "burns a CPU per sync" — a CPU-time
   O(depth) cost that would slip past. Author scopes the time-gate as an honest follow-on;
   the TITLE/CHANGELOG overclaim "the depth-war class." Reframe to the memory subset.

**The one call that's Andrew's:** ship the memory-only gate now vs hold for both
dimensions. My rec: ship now — the memory subset is the OOM-crash-loop subset; a
CPU-time gate needs its own noise analysis.

**Coupling the consult missed:** CI runs -short only, so the gate that actually protects
main is not the ladder the PR foregrounds. That is the highest-value finding here.
