# #563 — Reconcile memory at depth: bench before mitigation

**Date:** 2026-08-25. **Scope call:** D-TIERING (ratified this day) re-scopes #563 to a
minimal bounded mitigation — snapshot sync is the structural fix. This doc records the
options and the decision for the first step.

## Context

Run `a434494-deep`: val-d kernel-OOM ×2 on the 2 GB box during post-drill cold-sync
(RSS 1.43 GiB sampled 2 min pre-kill; terminal spike between 30 s samples). Issue #563
hypothesizes the multiple: "full-fetch Reconcile holds the decoded fork + the throwaway
validation replica + the node's own chain ≈ 2× chain in RAM + decode inflation."

## The attribution gap

The 2–3× hypothesis is unmeasured, and there is a concrete reason to doubt it: Go's
`[]Block` appends copy the Block STRUCT shallowly — the multi-MB `Answer` backing arrays
are shared pointers. The three concurrent slices on the sync path (`served[]` accumulator
→ `reconstructFork` copy → `tmp.blocks` replay, chainrole.go:1374/607, chain.go:2647)
may therefore hold ONE copy of the payload bytes, not three. If so, the OOM driver is
elsewhere: CBOR decode inflation, map overhead, GC headroom on a saturated box, or simply
baseline residency (cohort RSS was already 1.38–1.44 GiB before the spike).

Build-immutable #7: no mitigation until the bench names the dominant term.

## Options

- **A — bench first (chosen).** A deterministic peak-live-heap bench in `core/chain`
  (`reconcile_mem_563_test.go`), wire-faithful like the #555 oracle: mint an n-block
  reg-carrying chain with realistic ~1.5 MiB Answers, encode→decode (memo-less, like a
  real ChainReply), Reconcile into a cold replica, sample `runtime.MemStats.HeapAlloc`
  concurrently, report peak-extra as a multiple of the fork's payload bytes. The budget
  assertion is set FROM the measured numbers (order-of-magnitude bound, #555 style), so
  the test is the promised RED home: it goes RED if a future change reintroduces an
  unbounded multiple.
- **B — mitigate first (rejected).** Picking a lever (bound `served[]`, widen the suffix
  fast path, early-release backing) before knowing the dominant term is a guess (#7), and
  the likeliest levers differ by 10× in payoff depending on where the bytes actually are.
- **C — streaming/bounded-window Reconcile redesign (rejected).** Consensus-adjacent
  (fork-choice compares whole chains), M1-shaped, and obsoleted by D-TIERING snapshot
  sync. Explicitly out of scope per the ratified direction.

## What each outcome implies (decided in advance, so the next step is mechanical)

- **Multiple ≈ 1× fork bytes:** the spike is inherent fork residency + decode inflation.
  Mitigation = none beyond what already shipped (#558 removed the per-restart full-fetch
  driver; prune bounds the payload window); the bench becomes the guard; the 2 GB fit is
  re-checked on the deep re-run's RSS telemetry.
- **Multiple ≥ 2× fork bytes:** real duplication exists; find the copy site (encode
  buffers held across decode? a deep copy in Append/apply?) and release/bound THAT — the
  smallest change the evidence justifies.
- Either way: also record the baseline resident-bytes-per-chain (post-Reload, pre/post
  `PruneBelowHorizon`) — the field baseline (1.38–1.44 GiB) is the other half of the OOM
  arithmetic and feeds the deep-run watch.

## Non-goals

Node-level fetch-loop accounting (`served[]` growth across windows) is observable in the
same bench shape at `core/node` if the chain-level numbers don't explain the field spike;
not built until needed (#7).

## OUTCOME (same day) — the hypothesis was wrong; the bench named the real mechanism

The bench (`core/chain/reconcile_mem_563_test.go`, born RED) measured, at a 48-block /
72 MiB wire fork with 1.5 MiB Answers:

- **No second resident copy.** Retained-after-GC is NEGATIVE (−67 MiB: adoption shares
  the fork's payload backing and frees the old state). The issue's "fork + tmp replica ≈
  2–3× chain resident" hypothesis is falsified — the shallow-copy sharing holds.
- **The spike is transient marshal garbage.** Peak extra 69 MiB ≈ 0.92× fork payload,
  collapsing to 19 MiB at GOGC=20. Dominant generator: each decoded block's first
  `Hash()` materialized the full ~1.6 MiB body via `encMode.Marshal`.
- **Decode inflation is 2.35× wire** (72 MiB fork → 168 MiB resident). With the field
  baseline already 1.38–1.44 GiB, that inflation plus the garbage burst plus GOGC=100's
  heap-doubling headroom is the 2 GB OOM arithmetic.
- **The proven runtime guard was dropped.** `run-justification.log:138` records the
  a9cfc06 finding — backpressure + GOMEMLIMIT are complementary, MEM_LIMIT=1500M on the
  2 GB box — but `console-a434494-deep.log` contains zero `-mem-limit` occurrences: the
  OOM'd deep run launched without it.

**Mechanism paragraph (#6):** the failure is kernel-OOM at depth BECAUSE the cold-sync
Reconcile path generates ~1× fork-bytes of transient CBOR marshal garbage (per-block
`Hash()` body marshal) over an already-inflated resident set, AND the fleet ran without
the proven GOMEMLIMIT cap, so GOGC=100 let the heap grow past the box. The fix addresses
both legs: (1) `Hash()` marshals into a pooled buffer via
`CanonicalEncOptions().UserBufferEncMode().MarshalToBuffer` — byte-identity with the
reference `Marshal` asserted by `TestHashPooledBufferIdentity_563` (a divergence would be
a silent all-hashes change, #558-class), peak extra drops 69→6 MiB (0.09× payload,
budget forkPayload/4+16 MiB, RED-proven by the pre-fix run); (2) `topology.py` now
DEFAULTS `MEM_LIMIT=1500M` (override per box size, `0` to disable for attribution runs) —
the third-time rule applied: the mitigation was proven, then forgotten, so it is now a
default, not a runbook line.

**Consensus surface:** none. Same struct, same canonical encoder options, same SHA-256;
identity is test-asserted. I1–I5 untouched.

**Residual (named):** decode inflation (2.35×) and the resident full chain are the
structural memory floor; they are D-TIERING territory (payload pruning already bounds the
window; snapshot sync is the endgame). Watch the deep re-run's RSS telemetry with the
1500M limit in force.
