# 2026-08-27 — Re-derive the publish bound DOWNWARD from measured field cadence

**Owed Phase-3 gate clause** (ROADMAP.md "The ordered path" §3, and docs/decisions.md
D-M1-PIVOT): the original Phase-3 exit gate asked for the 360 s publish bound to be
re-derived **downward** once heights got cheap. This is that derivation. Discipline:
derive from the measured number, do not guess (#549-Q3 / build-immutable #7).

## The measured cadence (artifact citation)

`integration/cloudtest/results-fe2376a-deep.jsonl:29` (flow `12-deep-heights`,
Phase-3 exit gate, silt commit `fe2376a`, generated 2026-08-26T15:39:17Z):

> DEEP drive (Phase 3 exit gate): honest ceiling reached h132 (target h128, from h78)
> within 2615s of the 7200s wall (**~48s/height measured**)

Arithmetic check on the artifact's own numbers: (h132 − h78) = 54 heights in 2615 s =
**48.4 s/height**. The figure is not taken as gospel — it is the report's own division,
confirmed. Same value is mirrored in `report-fe2376a-deep.md:18`.

Historical context (the arc): the same steady cadence was ~390 s/height at the start of
the depth war and 80–170 s/height by run e2fab4b-9589 (cited in the H_ESCAPE_S
derivation, scenarios.sh:1831). ~48 s/height is the floor the depth-war fixes reached.

## What the publish bound is actually made of (mechanism, from the source)

`PUBLISH_RETRY_S = 360` is a **derived** window (scenarios.sh:11-29), not a magic
constant. It is the sum of two terms:

1. **Gather legs — 136 s.** 4 sequential legs × 34 s/leg. The 34 s/leg is
   `-request-timeout 8s × (1 + 3 -request-retries) + backoff (0.25+0.5+1)` ≈ 34 s.
   This is request-timeout arithmetic. **It is NOT coupled to height cadence.**
2. **Commit-wait leg — 220 s (`H_ESCAPE_S`).** The amount of wall-clock a single
   contested height can legitimately consume before its 2-round synchronizer escape
   completes. Derivation (scenarios.sh:1827-1835), verified against source this session:
   - `roundAdvanceSweeps = 2` (core/node/rounds.go:51)
   - `sweepsForRound(r) = roundAdvanceSweeps + r(r+1)/2` (core/node/rounds.go:67-68)
   - `ChainSyncInterval = 30 s` (core/node/node.go:288)
   - 2-round escape = `sweepsForRound(0) + sweepsForRound(1)` = `2 + 3` = **5 sweeps ×
     30 s = 150 s**, plus one ~34 s gather leg for the submit→poll→confirm ≈ **184 s**,
     historically rounded up to 220 s (a 36 s / ~20 % cushion).

Sum: `136 + 220 ≈ 356 → 360` (scenarios.sh:20-21).

## The finding that governs the re-derivation

**The 220 s commit-wait leg is a synchronizer ROUND bound, not a wall-clock cadence
quantity.** It is counted in fixed 30 s `ChainSyncInterval` sweeps and does NOT scale
down with the cheap ~48 s/height steady cadence. A contested height that pays a 2-round
escape still costs `dur(0)+dur(1) = 150 s` regardless of how cheap uncontested heights
have become. The measured cadence (390 → 170 → 48 s/height) is the *observed height
duration this 150 s escape allowance must cover* — and at ~48 s/height it covers it with
enormous margin.

That is the load-bearing distinction for this clause: **the escape floor is a
consensus-liveness parameter (the #451 synchronizer certification depends on it). It is
research-gated and I do NOT tighten it here.** Tightening the escape toward the 48 s
cadence would be the exact "a timing knob that is also a security parameter" trap
(build-process.md; CLAUDE.md research gate).

So the honest downward move is confined to the parts of the 360 s budget that were
padding for a slow *height straddle* — padding the cheap cadence retires — while leaving
the synchronizer escape floor intact.

## The re-derivation (downward)

The publish must survive, worst case, **one contested height** (submit → the height its
entry lands in pays a 2-round escape) plus its gather legs. It does not need to budget
for straddling multiple slow heights: at 48 s/height, even three uncontested heights
(~144 s) fit well inside the escape allowance already counted.

New derived bound:

```
  gather legs      : 4 × 34 s                       = 136 s   (unchanged; not cadence-coupled)
  commit-wait leg  : 2-round escape 150 s + 34 s gather = 184 s   (the H_ESCAPE_S FLOOR, un-rounded)
  ─────────────────────────────────────────────────────────────
  sum                                               = 320 s
  round down to a clean figure, keeping the floor   → 300 s
```

`PUBLISH_RETRY_S`: **360 → 300 s**. This is the downward re-derivation. The 60 s it sheds
is precisely the historical padding: the 36 s escape-rounding cushion (220 → 184, now
that the measured cadence proves the un-rounded floor holds) plus ~24 s of stale
slow-height straddle allowance the cheap cadence retires.

### The safety margin, explicit and justified

`PUBLISH_RETRY_S` is a **retry-loop deadline**, not a hard "commit-or-FAIL" SLO — the
flow keeps retrying the publish until this deadline, and a slow-but-eventual commit is
picked up idempotently (scenarios.sh:1870-1880). So the mirror-risk (a window too tight
re-introduces flake) is a GAP-sooner risk under transient WAN churn / spot preemption,
not a false-FAIL risk. The margin held is:

- The full 150 s synchronizer 2-round escape floor stays inside the bound — a genuinely
  contested height cannot exhaust it.
- 300 s = **6.25× the measured 48 s steady cadence** and **1.76× the 170 s worst-case
  per-height** seen at e2fab4b. A publish that lands in a contested height still has the
  whole escape window plus a full gather retry inside 300 s.
- 300 s is NOT tightened to the escape floor (184 s) — it keeps 116 s of retry headroom
  above the floor for exactly the transient-churn case the retry loop exists to absorb.

I deliberately did **not** go below 300 s. 240 s (the pre-#453 figure) assumed a flat
64 s drain cycle and was the value run 82bcd2b-39478 GAPped a real publish against
(scenarios.sh:22-24) — that is the documented too-tight failure this clause must not
re-create. 300 s sits comfortably above that scar with the escape floor intact.

## Scope: what changes and what does NOT

**Changes (the publish bound and its sibling):**
- `PUBLISH_RETRY_S: 360 → 300` (scenarios.sh:29).
- `ECONOMY_PUBLISH_RETRY_S` default `360 → 300` (scenarios.sh:1882 etc.) — it is the same
  publish operation's retry budget for the economy setup publish; it mirrors
  `PUBLISH_RETRY_S` by design (comment at scenarios.sh:1867), so it re-derives identically.

**Does NOT change (out of scope, and consensus-gated):**
- `H_ESCAPE_S` / the 220 s escape floor itself — a #451 synchronizer-liveness parameter.
- `LATCH_S` (1100), `HANDOFF_BLOCKS_S` (9×220), `STALL_S`, the maturing/deep per-height
  terms (`mh_height_s`/`dh_height_s`=220). These are maturity/handoff/stall bounds built
  on the escape floor, not the publish bound. They are a separate clause; re-deriving
  them belongs to their own evidence, not this one. Touching them here would be scope
  creep past the owed clause.

## Scenarios affected by the tighter publish bound

Every publish-dependent flow reads `PUBLISH_RETRY_S` as its publish retry deadline:
`2-publish-fetch`, `9-cross-nat`, durability-turnover, and the economy setup publish
(`11-economy-repair`, via `ECONOMY_PUBLISH_RETRY_S`). All of them still carry margin:
each still has the full 150 s synchronizer escape window inside the 300 s bound, so a
publish that pays a contested-height escape completes with room to spare at the measured
48 s cadence. None of these flows asserts a publish must commit *faster* than the bound;
the bound is the give-up point, and 300 s is 6.25× the measured steady cadence.

No billable cloud run is taken for this change — it is a derivation + config change,
reasoned locally from the fe2376a-deep artifact (task constraint; and the miss-inside-a-
computed-window-is-a-finding rule means the next scheduled field run will surface any
mis-derivation as a real signal, not a re-grade).

## Decision

1. `PUBLISH_RETRY_S: 360 → 300`; `ECONOMY_PUBLISH_RETRY_S` default `360 → 300`. Comment
   updated to cite the fe2376a-deep ~48 s/height cadence and this arithmetic.
2. The escape floor and all maturity/handoff/stall windows stay — retuning them is
   research-gated (consensus liveness) and out of this clause's scope.
3. ROADMAP.md and docs/decisions.md true-up: the owed "360 → downward" clause is
   discharged to 300 s with this derivation cited.
