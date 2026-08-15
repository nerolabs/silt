# Instrument the event loop for latency evidence — name the slow (or hung) function from real load

**Date:** 2026-08-15
**Idea:** Andrew — *"Can we not instrument function latency in logging, and use evidence to find functions that take unreasonable time, or worse case hang completely?"* This is his call, and it's the better one; recorded here as the deliberation.
**Context:** the MATURING attribution investigation (`2026-08-15-maturing-attribution-corrected-anchor-throughput.md`) landed on an anchor-side throughput wall on the single event-loop goroutine, but couldn't name the dominant atom — the per-issuer diagnostics were `LogDebug` and the run was `-log info`. The PE asked for a decomposition of the goroutine budget by term.

## The fork: synthetic decomposition harness vs. instrument the real loop

I had proposed a synthetic in-process harness that reconstructs the 12-validator load and attributes CPU-ms per term. Andrew's alternative — **instrument the real event loop and let real execution produce the evidence** — is better on every axis that matters here:

| | synthetic harness | instrument the loop (Andrew) |
|---|---|---|
| evidence basis | reconstructed load (guess-shaped) | the real system under real load (#7) |
| catches a **hang** | no (measures CPU, not stuck-ness) | yes — watchdog + stack dump of where it's stuck |
| reusability | throwaway | permanent observability; closes the gap that let me mis-attribute twice |
| works in the field | no | yes — same instrument, local *or* WAN |
| risk | new synthetic code paths | adapter-only, additive, zero core change |

The synthetic harness's only edge is deterministic isolation of a single atom — but for *naming* the dominant atom under the real 12-validator regime, real instrumentation is the more honest evidence. So: build the instrument, run it under load, read the truth.

## Why the event loop is the right (and only) place

The core is lock-free by construction — every entry point runs one-at-a-time on a single goroutine (`adapters/eventloop`), which is *why* consensus is deterministic. That single goroutine is therefore the one serialization point: whatever eats it, blocks everything. Timing each task there names the culprit. And it's an **adapter**, so real wall-clock is legitimate — the deterministic core stays free of ambient time (it must, or the sim diverges). Putting the timer anywhere in core would have broken that; the loop is exactly where it belongs.

## What shipped

`adapters/eventloop` gains optional instrumentation (zero value = off, so the sim — which uses its own scheduler, not this loop — is untouched):

- **Labeled tasks.** `Post(label, fn)`; inbound deliveries labeled by `msg.Kind` (tcpnet), timers/commit/api by a short constant. The label is the attribution key.
- **Slow-task log** (`SlowThreshold`/`OnSlow`): any single task ≥ threshold → one line, the "unreasonable time" evidence.
- **Hang watchdog** (`HangThreshold`/`OnHang`): a background goroutine reports a task still in-flight past the threshold **once**, with an all-goroutine `runtime.Stack` dump (the loop goroutine shows exactly where it's blocked) — the "hangs completely" case.
- **Per-window budget summary** (`SummaryEvery`/`OnSummary`): count/total/max per label per window — *this is the PE's goroutine-budget decomposition*, straight from real load. `BondChallenge` dominating names VDF+label-opens; `TokenRequest` names the RSA blind-signs; a commit/sync term names those.

Wired in the daemon at info/warn/error via the existing logger. Thresholds: slow 250ms, hang 15s, summary 30s (matched to the bond-audit cycle) — conservative first cuts, flagged to the PE for tuning.

## What this does NOT do (and the open questions → PE consult)

Attribution is at **handler-kind** granularity, not atom-within-handler. If `BondChallenge` dominates, that names the *handler* (VDF-eval + label-opens together); splitting VDF vs label-opens needs finer spans inside `answerBondChallenge` — a staged step, only if the coarse cut points there. Open questions (thresholds, label completeness, whether to run this under a local 12-validator load vs a field run, filing the confirmed no-rate-limit griefing seam) are in the PE consult:
`silt-reviews/principle-engineer/builder-consult-eventloop-instrumentation-2026-08-15.md`.

Discipline note: this is a **measurement tool, not a conclusion.** It does not name the atom until it runs under load. After correcting the attribution twice, the rule holds — no naming the residual until the evidence is on the table.
