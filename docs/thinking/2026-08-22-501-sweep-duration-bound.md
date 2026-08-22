# 2026-08-22 — #501: bounding the repair sweep under dead holders

**Status: deliberation (pace-before-code). Decision at the bottom; fix direction is
gated on the measurement this entry commissions.**

## The question

Issue #501: a repair sweep that completes in ~2.1s with all holders alive takes
~3–4 minutes after 2–3 holders die (field runs f58d599-17479 / 86dd6a4-9492;
kill 08:00:50 → `reachable=22/29` verdicts 08:03:34 and 08:04:08). The
`-repair-interval` knob bounds only the idle gap. Where do the minutes go, and
what bounds the sweep without violating build-immutable #5 (retry, don't evict)
or #3 (a transport number is not a correctness signal)?

## Code-reading attribution (evidence: the cited sites, not yet a measurement)

The per-RPC dead-holder cost is split by message class
(`core/node/node.go:1092` `requestTimeoutFor`, `:1110` `requestAttempt`):

| Path | Deadline | Retries | Cost to give up on a corpse | Stamps `deadUntil`? |
|---|---|---|---|---|
| Probe/fetch (`MsgHasChunk`/`MsgFetchChunk`) | `HolderDialTimeout` 2s | none | ~2s | yes (after 1 attempt) |
| Walk (`MsgFindNode`/`MsgGetProviders`) | `RequestTimeout` 5s (fleet: 8s) | 3, backoff 250ms→1s | **~21.75s (fleet: ~33.75s)** | yes (after 4 attempts) |

Three compounding mechanisms, all in the walk layer:

1. **The walk ladder is the unit cost.** The `deadUntil` gates added for #226/#277
   (walk `core/node/node.go:1506`, probe `repair.go:403`, fetch `file.go:433`,
   announce `repair.go:154`, diversity sweep `dht_diversity.go:67`) all skip a peer
   *already* in the cache. A **freshly** killed holder is discovered dead only by
   paying one full ladder — and the killed holders are the NodeID-closest to their
   column keys (that is why they held the columns), so nearly every
   `resolveProviders`/`IterativeFindNode` toward those keys walks into them.
2. **The cooldown is shorter than the sweep (and than the ladder itself at fleet
   settings).** `HolderCooldown` 30s (`node.go:284`) was sized as "~½ a repair
   interval" against the 60s default; the fleet runs `-repair-interval 2s` and a
   dead-holder sweep runs minutes. Every 30s the corpses re-admit, and the next
   phase of the *same sweep* re-pays the full ladder.
3. **Live peers resurrect the corpses.** `walk.step()`'s `OnReply` calls
   `table.Observe()` on every contact in a reply (`node.go:1521`), and provider
   records for the dead survive via `RemoveIfNotSole` (sole-holder keep, #69). So
   eviction never sticks while any live peer still lists the corpse — by design
   (#69 recovery), but it makes mechanism 2 perpetual.

Phase structure multiplies the unit cost: manifest heal is serial per chunk
(`repair.go:319`), shard probes are concurrent at 32 (`repair.go:30`), but the
per-stripe fetch resolution walks and per-shard placement walks
(`repairStripe`) serialize — each later phase can re-encounter a re-admitted
corpse.

**What this predicts** (to be confirmed, not assumed): the 3–4 minutes are
dominated by walk-ladder stalls, in waves — one per phase per cooldown lapse —
not by the probe RPCs, which fail in 2s and are correctly cached.

## What the existing tier already proves (and doesn't)

`core/node/repair_deadcache_test.go` proves every leg skips an
*already-cooled* peer and that recovery un-gates on proof-of-life. Nothing
measures sweep **duration** with freshly dead holders — the CI signature
(`TestRepairBountyPaysOnTheWire` flaking at ~181.8s before #511 widened the
window to the measured 600s) was the only duration oracle, and it lives at the
e2e tier with a wall clock.

## Options

**O1 — measure first: a deterministic sim rig that runs a full sweep against
freshly killed holders under daemon-like config, and phase-attributed sweep
narration in the product.**
Cost: a test file + a few `logf` lines. Benefit: turns the prediction above into
a named, reproducible number on a laptop in sim-time (wall-instant); the
narration makes every future LOCAL/cloud run self-attributing (the issue asks
for exactly this). Risk: none — it is the #7-mandated step.

**O2 — classify walk dials as fast-fail (tight deadline / no retry), like holder
dials.**
Cost: small code change. Benefit: caps the unit cost. Risk: **unsound as
stated** — walk queries also maintain the mesh; failing a *live but jittery*
peer fast and evicting it re-opens #288 (evict-on-one-miss starved consensus).
Any version of this must split "give up for this lookup" from "evict +
negative-cache" — the lookup may move on quickly (Kademlia's α-parallelism
already tolerates unresponsive contacts), but eviction must still require the
full patient ladder.

**O3 — sweep-scoped dead set: once any dial in a sweep exhausts on a peer, skip
that peer for the remainder of the sweep, independent of `HolderCooldown`.**
Cost: small; a per-sweep map threaded through (or a sweep-epoch tag on
`deadUntil`). Benefit: kills mechanism 2 (mid-sweep re-ladders) without touching
any transport/eviction semantics — a peer is re-probed next sweep, so #69
recovery is intact. Risk: low; it is a cache-scope fix, not a timeout change.

**O4 — retune `HolderCooldown` (e.g. ≥ sweep duration or ≥ ladder cost).**
Cost: a constant. Risk: this is the knob-not-the-cause move (build-process rule
7) — the cooldown's *value* can't be right while the sweep's duration is
unbounded (circular), and per the read-research rule no such number gets
invented without the measurement.

**O5 — restructure phases for full concurrency (probe ∥ fetch ∥ place).**
Cost: real restructuring of `repairStripe`'s continuation chain. Benefit:
overlaps residual stalls. Risk: highest-touch option; premature before the unit
cost is bounded — if O2'/O3 shrink the stalls to seconds, serial phases are fine
at repair's background cadence.

## Decision

**O1 now** — the measurement rig + sweep narration. The mechanism paragraph
above is code-read, not measured; #7 says the measurement is the next task, and
the narration is deliverable observability regardless of which fix ships.
**O3 is the leading fix candidate** (scope fix, no transport semantics touched)
with **O2-split** (lookup-moves-on vs evict) as the structural companion if the
measurement shows first-discovery ladders dominate even within a single phase.
O4 stands rejected as primary; O5 deferred unless the measurement says the
serial phase structure, not the ladders, is the dominator. The fix decision gets
appended here once the rig produces numbers.

## The measurement (same day — `TestMeasure_501_SweepDurationUnderDeadHolders`)

Deterministic sim, daemon-faithful transport (5s timeout / 3 retries / 250ms
backoff / 2s holder-dial / 30s cooldown / DHTDomainCap 2), field-faithful
placement (replication 1 — a killed node takes all its columns), field-scale
object (32 shards + manifest over 24 nodes), 2 holders killed carrying 13/39
shards. Seed 42, fully reproducible:

| sweep | duration | timeouts | reachable | phase split (manifest/probe/repair) |
|---|---|---|---|---|
| healthy (converged) | **2.0s** | 0 | 39/39 | 0.65 / 1.4 / 0 s |
| first after kill | **159.2s** | 96 | 26/39 | 22.3 / 23.0 / 113.9 s |
| second (object healed by first) | **45.3s** | 40 | 39/39 | 22.4 / 23.0 / 0 s |
| third (inside fresh stamps) | 1.9s | 0 | 39/39 | 0.7 / 1.3 / 0 s |

The healthy baseline matches the field's ~2.1s; the first-sweep 159s at 5s
timeouts scales by the fleet's 8s to the field's ~3–4 min band. The measured
attribution **confirms mechanisms 1–3 and adds precision the code-read missed**:

- The probe phase is NOT the per-shard dominator the issue guessed — probe
  concurrency (32) collapses all dead-walk ladders into one ~22s wall. The
  dominators are the **serial phases**: manifest-heal (one wall) and above all
  the **repair phase (113.9s)** — per-stripe fetch-resolution and per-shard
  placement walks re-paying ladder walls sequentially, re-laddering mid-sweep
  each time the 30s cooldown lapses (the 159s sweep spans 5 cooldown periods).
- The **45s re-discovery sweep recurs every ~cooldown period, indefinitely**,
  even after the object fully heals: corpses re-enter lookups via other peers'
  `FindNode` replies (`table.Observe`), the cooldown lapses, and the walk
  re-pays 2×~22s walls. The #277 comment predicted this shape ("deadUntil only
  RATE-LIMITS the re-dial — one RequestTimeout per HolderCooldown, forever")
  but at walk-LADDER cost, 40 timeouts a pop, not one RequestTimeout.

## The fix (decided on the measurement)

Two cache-scope changes, no transport deadline / retry / eviction semantics
touched (network-durability §1/§2 intact; bond-audit and static-peer
exemptions untouched):

1. **O3 — sweep-scoped exhaustion memory.** A peer that exhausts a full
   retry ladder is skipped by every gated leg (walk, probe, fetch, announce)
   for the remainder of the current repair tick, independent of cooldown
   expiry. Bounds one sweep to ≤1 ladder wall per corpse. The sole-candidate
   guards (#69 anyLive) keep their semantics.
2. **Decaying cooldown.** Each successive full-ladder exhaustion doubles the
   corpse's cooldown: 30s → 60 → 120 → 240 → capped 480s. The recurring
   re-discovery tax decays geometrically instead of firing every 30s forever.
   The cap sits under the reprovide period (ProviderRecordTTL 30min → 15min
   re-announce), and recovery is announced-in anyway: any inbound message
   clears the entry (proof-of-life, #69) — so a recovered holder is never
   probed-out slower than today. Literature analogue: negative caching with
   backoff (RFC 2308 negative TTL; libp2p dial backoff).

Not consensus-touching (no I1–I5 surface); not a security parameter (the
cooldown gates speculative dials only — bond audits never enter the cache).
The measurement test becomes the regression: first-sweep and healed-sweep
bounds asserted in sim-ms, born RED against the pre-fix behavior.

A third change rode along: discovery-kind RPC retries (`FindNode` /
`GetProviders` / `AddProvider` only) consult the cache before re-sending and
end early if the peer was proven dead by a parallel ladder. The lock-step sim
measured NO effect (concurrent identical ladders pass their last retry boundary
before the first stamp lands — the timeline autopsy showed 19 ladders to one
corpse all exhausting within 1s); it is kept as the correct behavior for the
desynchronized-WAN regime the sim cannot exercise, with that honesty stated
here. Bond challenges, static peers, and all non-discovery kinds keep their
full patience (#3 one-signal-one-job, #288).

## Post-fix results

Same rig, same seed: first post-kill sweep **159.2s → 72.4s** (= one ~22s
discovery ladder per corpse, sequenced because the phases meet the corpses
disjointly, + 27s of real per-stripe repair work); the decay drive
(six sweeps at 35s gaps) pays once (streak→120s), goes quiet, pays once more
(streak→240s), then stays quiet — asserted, vs the pre-fix flat ~45s on every
gap sweep. Whole repo green.

**On the wire (local e2e):** `TestRepairBountyPaysOnTheWire` pays in **27.8s**
(sweeps under kill complete in ~20ms, `total-ms=18-28` in the daemon journal)
— vs the ~181.8s cycle that flaked CI at the old 180s deadline. The 600s
window (#511) is re-tightened to 180s, restoring the deadline as the #501
regression signal.

## Discovered en route (filed separately)

Re-running the e2e surfaced a DISTINCT premise flake (~2/10 locally): the
kill-selector's `swarm holders` record-view sometimes diverges from where
bytes+records actually live by kill time, so the kill under-kills; the
caretaker then accurately reports `missing=2 ≤ slack=2`, watches (correctly),
and no bounty can pay at ANY window length. The old storm-choked probes
plausibly masked these extra copies (missing looked bigger, repair fired), so
the accurate post-fix probes may RAISE this mode's frequency. Attribution
needs a failing run's `place attempt`/`chunk stored` debug lines (#497
narration) — filed with capture instructions rather than guess-hardened
(build-immutable #7).
