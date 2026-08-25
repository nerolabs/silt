# #572 — the val-c catch-up stall: attribution state and what shipped

**Date:** 2026-08-26. **Status:** mechanism NOT yet named — candidates narrowed, wrong
premises retired, and the instrumentation that names it on the next occurrence shipped.
No guess-fix (build-immutable #7).

## What the evidence established (all cited on the issue)

1. **Not a fork.** The majority committed the SAME h24 block val-c held
   (`checkpoint: 24:b04e6fb7…` on val-b, maturer-4, sybil-2). The issue's original
   fork-heal framing was corrected on the record.
2. **The chain-behind-mark shape is real** — `mark_height=33 mark_round=1` against a
   h24 chain (the markstore is atomic; chain.cbor save cadence lagged). Consequence:
   the I2 guard correctly muted val-c's consensus voice (no signable round-changes at
   h25–h33 — its ID is absent from every round-change sender list). Correct behavior,
   real system-level cost: a seat whose store lags its mark is consensus-mute until it
   syncs past the mark.
3. **Sweeps ran and silently failed for 100+ minutes.** The next-tick is scheduled
   outside the walk callback (chainrole.go), targets existed (3 persistent peers), and
   every failure branch of the walk logs at debug or below — the info-level field
   capture was structurally incapable of naming the branch.

## What the repro exonerated

`core/node/syncstall_572_test.go`, on the exact 12-seat governed-epoch topology
(matureWorld12), majority 12 ahead across an epoch rotation, era-2 certified blocks:

- **Control** (plain behind seat): catches up in one sweep. GREEN.
- **Chain-behind-ahead-mark** (the field shape, mark inside the gap past the
  rotation): catches up in one sweep; the mark neither blocks adoption nor moves. GREEN.

So the mark→sync coupling is NOT the defect, and the plain heal mechanics are sound in
this schedule. The field wedge needs a dimension this schedule lacks — candidates that
remain live: peers serving while deep in the h37 round ladder / mid-drill restarts;
real-TCP request timeout behavior; reg-heavy (1.5 MiB) windows and event-loop
occupancy interleaving; probe replies from peers whose head kept moving. Adding them
all blind would be a fishing expedition (#7) — instrumentation instead.

## What shipped

1. **The guard tests** — the two repro shapes stay as regression guards (the heal and
   the mark shape must keep working), plus the observability oracle below.
2. **Per-sweep sync diagnostic (`SyncChain`)**: when a sweep ends with zero adopted
   blocks while a probe showed a peer ahead (or every probe failed), ONE warn line
   reports our-next, max-peer-head, probe-fails, head-matches, windows,
   suffix-appends, reconciles, and the last branch error. Branch errors (window
   abort, undecodable reply, reconstruct failure, reconcile-refused, suffix stop,
   probe failure) are captured into the diagnostic as they occur.
   `TestSyncStall_572_NoProgressSweepWarns` asserts the warn fires (RED-proven by
   neutering the condition). The next field occurrence carries its mechanism in the
   info-level capture.

## Sequencing note

The deep re-run stays blocked on #572 per the session plan — but "blocked" now means:
either a further-dimensioned local repro goes RED first, or the next run's
instrumentation names the branch in one occurrence. The #574 harness fixes ride
before any re-run either way.
