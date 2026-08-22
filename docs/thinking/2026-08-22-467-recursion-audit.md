# #467 — the recursion/re-entrancy audit (PE flixz Finding 2, audit extension)

Date: 2026-08-22. Status: audit complete; five sibling fixes shipped with
failing-first tests (`core/node/recursion_audit_test.go`).

## Where #467 already stood

The crash itself — `fatal error: stack overflow` on the
`resolveProviders ⇄ probeShard ⇄ sweepProviders` cycle under a large fresh
root — was fixed by PR #471 (2026-08-18, merged hours after the issue was
filed): the DHT walk terminal is trampolined through the event loop
(`node.go` `walk.step`, `AfterFunc(0)`), with failing-first regressions
`TestAnnounceHeldTrampolinesWalkTerminal` and
`TestResolveProvidersTrampolinesWalkTerminal`. The complementary
"don't repair a still-publishing root" trigger fix arrived independently as
#517's two-consecutive-over-slack-sweeps confirmation gate (PR #519).

What #467 still owed was the PE's audit extension: *"no unbounded recursion /
no re-entrant cycle between subsystems, not just no unbounded heap — scan for
siblings of this path."* That scan had never been run. This document is it.

## Method

Every self-recursive continuation closure in `core/`, `adapters/`, and `cmd/`
(`var f func(...); f = func { ... f(...) }` and named recursion) was located by
grep and classified by one question: **can an iteration's continuation run on
the caller's stack, and if so, what bounds the iteration count?** A chain is
safe if every iteration crosses a deferred boundary (a trampolined walk
terminal, a real network round-trip, a timer) or its count is a small constant
(K, one provider set, one stripe's shards). It is a sibling of the flixz crash
if a synchronous fast path lets the chain advance inline over a collection
that scales with store, file, or cared-set size.

The audit also re-derived the one fact the #471 fix rests on: `request` was
NOT uniformly async — a synchronous transport send failure ran the callback
inline (`requestAttempt`), which silently re-armed every "safe because it
crosses a request" chain the moment a transport rejected all sends.

## Findings — unbounded siblings (all fixed)

| # | Path | Inline trigger | Depth |
|---|------|----------------|-------|
| S1 | `repairStripes` healthy-stripe walk (`repair.go`) | a stripe within slack continues synchronously — the common case, every sweep | O(stripes); plus O(stripes × refs) rescan CPU in one loop task |
| S2a | `FetchChunk` already-held fast path (`file.go`) | chunk in local store | O(ids) via `fetchAll`/`fetchColumn` chains over a fully-held list |
| S2b | `fetchFrom` no-provider / all-skipped exit (`file.go`) | empty or fully-gated provider set — the fresh-root condition | O(ids) via the same chains |
| S3 | `repairTick` root walk (`repair.go`) | a denied root or failed registry lookup completes synchronously | O(cared roots) |
| S4 | `request` synchronous send failure (`node.go`) | transport rejects the send (no route / dead adapter) | re-arms every request-crossing chain, O(items) |
| S5 | `distribute` member skip on convergent dedup (`file.go`) | source chunk already shipped | O(column members) = O(stripes) |

S1 was the sharpest: not just stack depth but a B2 liveness violation — the
per-stripe scan re-walked the whole `refs` slice each call, so one sweep of a
large healthy file did O(stripes × refs) work in a single loop task,
monopolizing the loop that also serves every other request.

**Fix idiom, uniform (the #471 contract):** a completion that can occur
synchronously posts through `clock.AfterFunc(0, …)` — depth O(1) across
items, one extra tick per item, loop stays live. S1 additionally groups
`refs` by stripe once (`repairStripesFrom`), taking the sweep's stripe-walk
CPU from O(stripes × refs) to O(refs).

## Bounded (no change; recorded so the next audit doesn't re-derive them)

- Skip chains — `try`/`send`/`next` over one provider set or K targets
  (`probeShard`, `fetchFrom` mid-sweep, `announceAll` per key,
  `sweepProviders`, `placeAt`, `fanOut`, `chainrole` ask): constant-bounded,
  and with S4 fixed every request crossing is deferred even on send failure.
- Per-item chains that cross a trampolined walk each iteration
  (`healManifest`, `announceAll` across keys, `ColumnHolders`, PoR
  `nextLeaf`): O(1) stack across items since #471.
- `walk.step` self-re-entry: bounded per walk by the lookup candidate set;
  terminal trampolined (#471); send-failure re-entry now deferred (S4).
- Timer-driven loops (`repairTick` reschedule, `StartReprovide`, daemon
  audit/`badPropose` retries): each iteration is its own timer event.
- `AcquireCredits`: caller-bounded count; async via S4.

## Test shape

`recursion_audit_test.go` pins each fix with the #471 assertion: the entry
call must return with `done` unfired (RED before the fix: every one of the
five completed inline), and the loop drain must complete it. The S3 test
observes yielding through the scheduler: processing N synchronously-skipped
roots must take N zero-time steps before the next event (the RepairInterval
reschedule) — inline, the sweep is already over when `repairTick` returns.

## Discipline extension (what to hold in review)

A continuation that *can* complete synchronously either (a) posts through the
loop, or (b) carries a visible constant bound on its inline depth. "It
crosses a request" is not an argument — it wasn't true on the send-failure
path for the first four months of this codebase.
