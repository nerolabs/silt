# 2026-08-16 — the clean re-run (ce15a80-89365): best MATURING scorecard yet; 10a exposes a forced-lock re-proposal stall

19 pass / 3 gap / 1 fail — the strongest MATURING run to date, and the #441+#448
fixes both field-confirmed:

- **Latch at h15 with the WHOLE cohort seated (260 MiB across 8)** — vs h26 (run 3)
  and the 22-minute starvation (confirm run). The #448 reg FIFO didn't just close
  the starvation; the drain got fair and fast.
- **Flow-10 handoff PASS**: post-latch publishes drove commits across the epoch
  boundary (h45→49) — the #441 mature face under a formal grade.
- **Flow-6 PASS within its computed 200s bound** (#449) — fault tolerance holds
  under the certified submit-then-poll design.
- 10c WS cold-sync PASS (F-1 held); 10b's M0 no-capture half HELD.

## The FAIL: 10a — a carried lock re-proposed for 14+ rounds without completing

With the 4 sybils stopped (8 of 12 epoch members live, 512 of 516 MiB weight), no
commit landed within the computed 370s STALL_S. The captured evidence:

- 388 round-change lines; a node at h50 with `mark_round=14` — fourteen rounds
  burned at one height.
- `new-view proposal height=50 round={4,6,9,11,13,15} forced=true` — the SAME
  carried lock re-proposed round after round, its gather never completing.
- **The head-count-floor hypothesis is REFUTED in code**: mature-epoch
  `RequiredQuorum` returns `cfg.Quorum` (=2) — "weight rule carries the Byzantine
  bar". 8 live holding ~99% weight satisfies every stated quorum.
- 10b's resume then ALSO overran its 240s window — but 10c's checkpoint at h54
  proves the chain recovered after the drills: slow escape, not a wedge.

**Leading mechanism candidate (UNPROVEN — do not build on it):** the locked value's
AUTHOR may be a stopped sybil (its drain proposal locked at r0 exactly as the drill
killed it). An era-2 commit requires the author's round-scoped prepare, lifted from
the carried lock QC for a dead author (the #432 certification's liveness half) —
proven in fixtures only at r1 over a 4-member epoch. Whether the lift (or something
else in the carried-lock path: lock-QC verification at high rounds, round-change
quorum composition with 4 of 12 silent, per-round designee rotation hitting dead
sybils 1/3 of the time) fails under the field shape (12-member epoch, 4 down, many
carries) is exactly what the deterministic repro must pin.

## Next (the tier order, strictly)

1. **Deterministic repro in `matureWorld`**, extended to the field shape: a
   12-member epoch (4 bonded anchors + 4 maturers + 4 sybils), lock a
   sybil-authored value at r0, stop the sybils, drive sweeps — does the forced
   re-proposal ever complete? Born-RED expected if the field mechanism is real.
2. **Research consult with the repro** — this is inside the certified #432
   machinery; no unilateral change.
3. The soak re-run and red-team #183 stay queued BEHIND this finding: a mature
   network that pays 370s+ (observed; eventually recovered) whenever a locked
   value's author cohort goes silent is a liveness bound the red team must not
   discover first.

The scorecard's honest summary: every fixed defect stayed fixed (publishes,
drain fairness, fault tolerance, handoff, latch); the drills advanced one
doorway deeper into the machinery and found the next thing there.
