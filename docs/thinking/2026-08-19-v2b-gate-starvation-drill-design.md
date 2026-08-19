# Design — the v2b timed starvation drill (the PE-mandated RED oracle)

**Date:** 2026-08-19 · **Gate:** PE ruling
`silt-reviews/principle-engineer/RULING-v2b-consensus-reserve-approach-2026-08-19.md`
(full path: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-v2b-consensus-reserve-approach-2026-08-19.md`):
the v2b consensus reserve now rests on zero field evidence, so a **timed** RED drill is the
first — and possibly only — deliverable. If the drill cannot reach gate-level consensus
starvation under a defensible cost model, **v2b is shelved** (not deprioritized) and the
residual is filed for #183 to probe.

## What the drill asserts (the v2b oracle, stated as the DESIRED property)

Under a sybil-cohort bulk flood — every member an authenticated non-validator, every member
**within its v2a per-peer share**, the global inbound cap **engaged** — a consensus-kind frame
from a distinct validator peer still reaches the handler within the starvation bound.
RED = the property fails (starvation demonstrated) → the v2b mechanism (identity-gated
reserve per the ruling) earns its slot and must turn this exact drill GREEN.
GREEN = starvation unreachable at these parameters → shelve v2b, file the residual.

## The cost model (disclosed, per the ruling's "timed, not order-only")

| Parameter | Value | Why |
|---|---|---|
| cap | 8 MiB (per-peer share 2 MiB) | scale model of the shipped 256M default, same 1/4 share ratio |
| bulk frame | 128 KiB | mid-sized store traffic |
| cohort | 8 flooders | each ≤ its share; collectively ≥ 2× cap, so the global cap binds |
| drain cost | 64 ms per bulk message (≈ 2 MiB/s) | the slow-drain regime is field-attested: the `e03f80d-heapprof` run accumulated a **266 MB** inbound backlog (intake > drain, sustained), and consensus-heavy handling (1.5 MB bond-reg blocks, VDF/sig verification at 100+ ms each) puts drain in the single-digit-MB/s band during exactly the windows that matter |
| consensus frame | 8 KiB `MsgProposeBlock`, every 200 ms | small, round-machinery-shaped |
| starvation bound | 2 s | the production queue-wait saturation threshold (`daemon.go` `QueueWaitThreshold`): a frame delayed 2 s is "on its way to timing out" against the 8 s transport deadline |

Analytic prediction at these parameters: a full gate means ≈ cap/drain = **4 s** of queued
work ahead of any newly admitted byte, and a blocked consensus reader further contends with
8 blocked bulk readers on every `Broadcast` wake — so the model predicts RED (> 2 s), and the
drill's job is to confirm it *and* record the measured magnitude.

## "RED for the right reason" (the ruling's requirement)

The drill only counts a failure when, concurrently: (a) sampled gate usage ≥ 90% of cap
(cap engaged), (b) every flooder's sampled in-flight stays ≤ its per-peer share (the cohort
is playing inside v2a's rules), and (c) the control phase (no flood, same drain cost) lands
consensus frames at ≪ bound (the latency is flood-attributable, not harness overhead).

## The observation the timed numbers will adjudicate (recorded before running)

**Gate-full ⟺ ≈ cap bytes of admitted work already sit in the FIFO loop queue** — the gate
releases only when the loop *finishes* a message. So an admission-side reserve alone buys a
consensus frame at most `R/drain` of relief; the frame still processes behind up to
`(cap−R)/drain` of queued bulk. If the drill goes RED with latency dominated by the queued
backlog (not the acquire wait), the honest conclusion is that v2b-as-admission-reserve
cannot turn the drill GREEN by itself — it needs a loop-level processing-priority companion
(or a smaller cap) — and that finding goes back to the PE before any mechanism is built.
This is exactly what an order-only drill would have hidden.

## Verdict procedure

Run the drill on this branch. RED → report the measured latencies + attribution to the PE
and only then design the mechanism against it. GREEN → shelve v2b per the ruling: record
in the roadmap/owned residuals, park the drill branch as the evidence, nothing merges to
the hot path.

## Verdict (2026-08-19, post-run) — RED, and the pre-registered observation confirmed

The drill ran RED for the right reason: all 10 consensus frames at a uniform **~4.10–4.12 s**
vs the 2 s bound (delayed, not dropped), cap sampled ≥ 90% engaged, every flooder within its
v2a share, idle-gate control ≪ bound. Measured worst **4.121 s** vs the analytic FIFO
prediction cap/drain = **4.096 s** — within 1%, uniform: the starvation is the admitted
backlog draining FIFO through the single loop, not acquire-contention. The pre-registered
implication held: an admission-side reserve is necessary but insufficient.

**PE drain ruling** (`RULING-v2b-drill-RED-drain-not-gate-2026-08-19.md`, superseding the
admission-only structure of the approach ruling): the severe regime (`cap/drain ≈ 128 s` at
the shipped 256M) requires drain pinned at ~2 MiB/s — which **is** the bond-reg/VDF
CPU-flood regime, Phase 1.2's domain. Sequence: **Phase 1.2 first** (raises the drain
denominator) → measure the real saturation drain rate at 256M as a rider on 1.2 validation →
**re-run this drill re-parameterized to the measured drain** as the go/no-go → only if still
RED, build the **one two-class mechanism** (class → priority drain) with this drill as the
merge oracle plus a bounded-priority second oracle (bulk/repair must not starve — I4 storage
plane). Residual filed as **E5** in `../design/owned-residuals.md`; the drill stays parked on
`drill/v2b-gate-starvation` (84b2788) as the oracle. The `-inbound-cap` two-axis sizing note
(OOM headroom vs `cap/drain` latency, opposite pulls) shipped in the flag help.
