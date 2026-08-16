# 2026-08-16 — the #441 confirm run (54003f7-91159): publishes FIXED on the wire; the latch trips LATE — a drain-pace finding

16 pass / 3 gap / 1 fail. The run's primary question — do publishes commit with the
#441 entry mempool — is answered YES across every publish-dependent flow: re-warm in
71s (40 consecutive failures pre-fix), durability-turnover PASS (GAPped on run 3),
priv-unlinkability, cross-NAT (a relay-path publish), chaos flows all green, entries
visible landing in the drain curve (h43: `1 entries, 1 bond-regs`).

## The FAIL: flow-10's latch missed the 630s bound — then tripped 5 minutes later

`wheels shed permanently` at 11:44:09 (sybil-3's journal; nakamoto 2, 195 MiB across
6) — ~54 min from network start vs run 3's ~25 min (h26, whole cohort). The chain's
block cadence was NORMAL (h59 by the trip, exactly 1 bond-reg draining in every
block h40–h59): the regression is WHICH regs — **first-time maturer regs banked ~2×
slower**, and flow-10's computed window caught it honestly (PE §4: a miss inside a
principled bound is a finding, never re-graded).

## The census that names the mechanism space

132+ `bond-reg submit REFUSED … signature` lines (vs 54 on pre-fix run
09fbe60-84613), heavily concentrated: one validator (f9008cef…, a maturer) refused
**49 times** ≈ 25 min of failed banking at the 30s resubmit cadence — the observed
maturity delay almost exactly. The refusal is the known AHEAD-skew face of the #427
K-window (mechanism pinned by TestBondRegAheadOfReceiverWindow_refusedThenHeals):
a reg signed over the submitter's tip fails at a receiver that hasn't committed
that head. It healed in ~1 resubmit pre-fix; this run it repeated ~50× for one
submitter.

**Leading hypothesis (UNPROVEN — flagged, not assumed):** the #441 entry-armed
sweeps increase round churn (entries continuously arm maybeAdvanceRound and the
designee sweep), which increases head-view skew between a submitting maturer and
the receiving designee — the reg is re-signed over a tip the designee persistently
trails. The deterministic home is the matureWorld fixture: reproduce a first-time
reg submitted under entry-churn and measure refusal repetition. Do NOT touch K or
the acceptance rule without that repro + research (both are C1-certified surfaces).

## Also found: flow-6 fault-tolerance GAP — the O(f+1) ladder's wall-clock cost

With val-d down, no commit landed in the flow window (passed on every pre-fix run).
The submit-then-poll design routes every publish through the designee rotation; a
DOWN designee inserts the staggered-takeover ladder ((3+dist)×30s sweeps) the old
direct-propose path never paid. This is the certification §4 fairness bound's real
latency: correct (the entry commits within O(f+1) HEIGHTS) but heights lengthen
when a designee is down. The flow's window predates the certified design — it needs
the same computed-bound treatment 10a got (STALL_S), sized to the ladder. Harness
change, next PR.

## Where this leaves the sequence

The #441 mechanism is field-confirmed for its purpose (publishes commit; the
mature-regime steady state was not reached this run, so the post-latch face reruns
next time). Before the next billable run: (1) fixture repro of the reg-refusal
repetition under entry churn (or its refutation); (2) flow-6's window gets its
computed ladder bound; (3) then ONE re-run grades latch-in-bound + post-latch
publishes together. Red-team #183 stays gated on that clean run per PE §7.
