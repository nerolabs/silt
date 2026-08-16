# 2026-08-16 — the #432 branch confirm run (09fbe60-84613): wedge ESCAPED on WAN; latch + fresh-publisher still open; run interrupted

Overnight autonomous run (Andrew authorized: "you can do a cloud test too while
I sleep"), launched from `feat/432-two-phase-rounds` @ 09fbe60 —
`MATURING=1 SYBILS=8 LOOP_BUDGET=1`, ALL on-demand, #6 justification via
RUN_MECHANISM/RUN_REPRO (the pre-flight gate demanded it — good gate).

## What the run established (evidence: console-432-branch-confirm.log, flow-evidence-09fbe60-84613.log, evidence-432-drain-09fbe60-84613.log)

1. **The #432 wedge is escaped on real WAN.** Both pre-fix MATURING runs
   starved at tip ~6. This run: flow 5 convergence PASS at tip 16 advancing to
   17 mid-check (all four validators identical head hash), and the chain was
   still committing at **height 46+** when the run was interrupted. The round
   machinery ran live: `round-change: advancing/recorded` every stuck height,
   `new-view proposal height=44 round=1 forced=false`, and — the certified
   carry-forward, in the field — **`new-view proposal height=20 round=2
   forced=true`** (a locked value re-proposed at a higher round).
2. **16 PASS / 0 FAIL** through the flows that ran: first-run, publish-fetch,
   care-link, become-validator, convergence, fault-tolerance, restart-standing,
   restart-content, takedown, cross-nat, forged-block, low-bond,
   priv-unlinkability. 2 GAP = the known undrivable-on-WAN #345/#350 drills
   (their deterministic home — the local drills — went green tonight).
3. **The drain is NOT starved** (first live read was wrong; corrected against
   the pending trace): val-a's pending queue drained 12→6 by h25 (first-time
   cohort bonds committing — flow 4 PASS), then re-grew 6→10 — **TTL renewals
   refilling the queue**, i.e. steady-state renewal traffic, not the old
   staleness wedge. The new `drain blocked at own sign slot — awaiting round
   advance` line (shipped in #434) fired exactly as designed and named the
   per-sweep contention honestly.

## The two OPEN items (the run was killed before flows 10/C2 could grade them)

- **The everMature latch had NOT tripped by ~35 min** — `wheels shed
  permanently` absent from val-a's journal at h43+, far past the computed 630s
  bound, with the maturer cohort live. UNATTRIBUTED: could be drain order
  (renewals crowding first-time maturer regs out of the byte budget), a
  C2-premise gap, or something new. The drain-curve lines (per-commit C2, 64MiB
  jumps) are debug-level and the run was `-log info`, so the decisive evidence
  was not captured. **Next run: LOG_LEVEL=debug on val-a, or promote the
  drain-curve line to info for MATURING topologies.**
- **fetch-1 did not warm within the computed 240s** (strand-(a) unsettled
  again; #351-shaped). Its tail shows an idle event loop after the failure —
  the warm-failure diagnostics are earlier in its journal and were not pulled
  before teardown.

## Process notes (own them)

- **The run was killed ~40 min in** by the session harness's background-task
  lifecycle — NOT by the harness or the network failing. The destroy trap fired
  but was interrupted mid-terraform; 21 VMs kept running until I nuked by label
  (`nuke` exit 0; `gcloud instances list` empty, verified). **The
  detached-launch discipline in memory says double-fork + logfile precisely so
  the run outlives the session machinery — I deviated (used a session
  background task) and this is the cost. Next long run: `nohup setsid`
  double-fork, then Monitor the logfile.**
- Because the kill preceded flows 10-maturing-handoff and C2-no-capture and the
  report phase, **this run grades as an INTERRUPTED confirm run: strong
  evidence the wedge fix works under real WAN, no formal verdict on maturity or
  fresh-publisher bounds.** results.jsonl has no summary line; the console +
  flow-evidence + live-captured drain/round evidence are the record.
- Live evidence capture from a still-up network before teardown (head heights,
  pending trace, round-change/new-view lines) turned an interrupted run into
  usable evidence — worth keeping as a practice when a run dies mid-flight.
