# The concurrent-publish 502: attributed — a Care/NetGet event-loop self-deadlock, NOT the inbound cap

**Date:** 2026-08-19 · **Roadmap:** Phase 1.1 (the flixz publish-flood 502) ·
**Discipline:** build-immutables #6 (attribute before you ship) + #7 (evidence or
nothing) — this doc records the evidence chain and the deliberation before any code.

## The field finding (as reported)

flixz handoff #2: `--workers 4` concurrent segment publish → the daemon returns
**HTTP 502** mid-ingest (daemon survives), correlated with the
`inbound-cap: 256M …` startup banner. Folded into roadmap Phase 1.1 as "the
inbound-cap should backpressure gracefully, not 502 the caller."

## The evidence chain (local repro, all artifacts in-session)

Repro: one validator daemon (`-serve-registry`, `-quorum 0`, UI on), 4 workers ×
sequential 3 MB multipart publishes via `POST /api/publish`. Fails in ~2 min,
every run: 4×502 by segment 1–2, daemon alive.

1. **The 502 bodies name three faces** (never captured in the field):
   `manifest chunk … placed on no node after 4 attempts (network full or
   unreachable)` (`core/node/file.go:176`), `chainhost: consensus timed out`,
   and `chainhost: entry … not committed within 30s (submitted to the mempool;
   still pending)`.
2. **Control run, cap effectively unbounded (100G): identical failures.** The
   inbound cap is NOT the driver. (The field report pattern-matched the startup
   banner; the actual error bodies were never captured there.)
3. **The daemon's always-on saturation telemetry stayed silent** (250 ms
   slow-task / 2 s queue-wait warnings: zero lines) — not loop *saturation*.
4. **The `-log debug` run caught the mechanism red-handed** — the 15 s hang
   watchdog fired with a full stack (`eventloop task HANG — node thread stuck
   kind=ui seconds=17`):
   `uiServer.apiPublish` → `s.onLoop(nd.Care)` **on the daemon loop** →
   `Node.Care` (`core/node/repair.go:49`) synchronously calls `reg.Lookup` →
   `chainhost.Host.Lookup` → `h.onLoop` **posts back onto the same loop and
   blocks in a select** (`adapters/chainhost/chainhost.go:47`) waiting for a
   task the wedged loop can never run → 30 s self-deadlock ending in
   `chainhost: consensus timed out`.

## The mechanism paragraph (#6)

The failure is *concurrent UI publishes 502 while the daemon survives* **because**
every successful publish's auto-caretake (`-care-published`, `ui.go:497`) runs
`Node.Care` on the daemon's single event loop, and `Care` synchronously calls
`ports.Registry.Lookup`, which on a validator daemon is `chainhost` — an adapter
that marshals the call **back onto the same loop** and blocks awaiting its
execution: a reentrant post-and-wait self-deadlock that wedges the node's single
thread for the 30 s chainhost timeout. Behind the wedged thread, other workers'
placement requests exhaust their 4 × 2 s attempts ("placed on no node"), and
mempool entries outlive the 30 s commit poll ("not committed within 30s") — the
three observed 502 faces. **This change addresses it by** making core read the
committed chain directly when it has one (`n.chain.LookupRoot` — the very data
chainhost would have answered with), so no loop-context registry read ever
marshals through the adapter back onto its own loop.

## Blast radius (all five doorways, one class)

Synchronous `reg.Lookup` from core, reachable on the loop:

| Site | Loop context | Effect on a validator daemon |
|---|---|---|
| `repair.go:49` `Care` | UI publish auto-caretake (`ui.go:497`); `-care` startup (inside the `AnnounceHeld` callback, `daemon.go:1283`) | 30 s wedge per publish / per care root |
| `file.go:597` `NetGet` | `apiFetch` posts it (`ui.go:556`) | 30 s wedge per non-local fetch |
| `repair.go:221` `repairRoot` | `repairTick` clock callback | 30 s wedge per repair sweep root |
| `por.go:218` `Audit` | daemon audit sweep (`clk.AfterFunc`) | 30 s wedge per audit sweep |
| `repairclaim.go:60` claim verify | inbound bounty-claim message | 30 s wedge per claim judged |

Why the suites never caught it: e2e publishes via the CLI (`swarm add` — a
separate process whose registry is `httpregistry`, no self-marshal), and
cloudtest caretakers point at remote registries. Only the
**validator-that-also-serves-its-own-UI/caretakes** shape composes core-on-loop
with chainhost — exactly the flixz box, silt's first real single-box production
shape.

## Options considered (PACE)

- **A — chain-first lookup in core (CHOSEN).** A small `n.lookupEntry(reg, root)`
  helper: a node holding a chain replica answers from `n.chain.LookupRoot`
  (loop-safe, and *literally the same read* `chainhost.Lookup` performs); nodes
  without a chain fall through to `reg.Lookup` unchanged. Fixes all five
  doorways at the root; no new machinery. Note: a validator pointed at a
  *remote* registry now answers maintenance lookups from its own committed
  replica rather than the remote — the replica is what it would serve to anyone
  else, so this is the consistent source.
- **B — reentrancy-aware `chainhost.onLoop`** (run inline when already on the
  loop). Rejected: Go has no sound goroutine-identity primitive — the known
  tricks are goid stack-parsing hacks (B8: amateur tell), and the adapter
  contract is cleaner stated as "never call this from the loop" (now in its
  comment).
- **C — UI-layer patch (pass the just-registered entry into `Care`).** Rejected
  as the sole fix: closes one doorway of five, leaves the class.

**Owned residual (filed as an issue, not fixed here):** on a *chainless* node
(client / plain daemon with a remote registry), these same sites still make a
blocking HTTP `Lookup` on the loop — bounded by the HTTP timeout, no deadlock,
pre-existing — a B2 wart worth an async pass of its own.

## Corrections this forces (R2)

- **ROADMAP.md Phase 1.1** said the fairness/priority-lane work "also fixes the
  publish-flood 502." Corrected: the 502 was this deadlock, cap-independent. The
  v2b consensus-priority lane remains justified by its own adversarial case (the
  PE ruling's sybil-cohort flood), not by this finding.
- **Drive-by defect:** `-inbound-cap 0` (the documented "unbounded" sentinel)
  is rejected by `parseSize` ("bad size \"0\"") — fixed with a unit test.

## Tests (V5, failing-first)

1. **Integration (the deadlock):** a validator node + real eventloop + chainhost
   registry; run `Care` from loop context (as `ui.go:497` does) with a short
   chainhost timeout — RED before (times out), GREEN after (chain-first read).
2. **e2e (the field face):** one validator daemon with UI; 4 concurrent
   multipart publishes — all must return 200 with the daemon alive. RED before
   (502s by segment 1–2), GREEN after.
3. **Unit:** `parseSize("0")` → 0 (unbounded sentinel works as documented).
