# 2026-08-15 — #406 consensus model-check: build approach (pace-before-code prep)

**Context / trigger:** #402 is landing; #406 (the deterministic adversarial consensus model-check) is
the certified next build AND the top item from the testing-tiers assessment — the missing rung that
turns the tier order into a gate. Spec: `docs/design/consensus-model-check.md` (ADOPTED). This is the
pace-before-code plan, written before touching code and while #402 CI finishes, so the build starts from
a decided approach (not momentum). Not a build yet — sequencing: build on clean `main` after #402 merges.

**Evidence — the substrate already exists (grounding the "reuse, don't rebuild" mandate):**
- `adapters/simclock`: seeded scheduler with **`Step() bool`** (fire the next single event) + **`Pending() int`** + `Run()`. Single-event stepping is already there.
- `adapters/simnet`: already has **`Partition`/`ClearPartition`**, **`Kill`/`Restart`/`Alive`**, NAT/Relay. Partition and crash-restart controls exist.
- **The gap:** `Endpoint.Send` (`simnet.go:107`) schedules delivery via `sched.AfterFunc(drawLatency(), deliver)` — a *random* latency into the time-ordered heap. The adversary needs to choose **delivery ORDER**, which today is a function of drawn latency, not driver choice.
- `core/node/adversary.go` already has Byzantine primitives (`-equivocate`/`-forge-block`, `equivPlan`), so the Byzantine node's *actions* are built; the model-check drives their *scheduling*.

## Options — the delivery-order control mechanism (the one real design choice)

- **(A) Held-delivery mode in simnet.** Add a mode where `Send` enqueues the delivery closure into a
  controllable pending queue (stable ids) instead of `AfterFunc`-ing it; the driver calls
  `DeliverNext(choice)` to fire a chosen pending message. Cost: ~40 lines in simnet + a flag. Benefit:
  full, explicit delivery-order control; exhaustive interleaving is just "enumerate the choice at each
  step"; partition/kill re-checks already happen at delivery time. Composes with the existing
  `Partition`/`Kill`.
- **(B) Latency manipulation.** Force orderings by choosing latencies. Fragile (ties, indirect), can't
  cleanly express "deliver X before Y"; rejected.
- **(C) Priority hook in simclock.** Let the driver reorder the heap. Leakier (couples the model-check
  to clock internals; affects timers, not just messages); rejected.

**Decision → (A) held-delivery mode.** It is the literal "adversarial delivery scheduler" the spec
names, it keeps the change inside the network adapter (core untouched — the whole point), and it makes
the exhaustive tier a clean tree enumeration over the pending set at each step. Guard it behind a mode
flag so the existing sim tests (which want random latency + `Run()`) are unaffected.

## The oracle (assert after every delivered step) — `assertInvariants(replicas)`

One function over all replicas' `Chain` state, checked each step:
- **I1** — for each height, collect every block any replica treats as *final*; assert ≤1 distinct hash. (The direct #357/#397/#402 catch.)
- **I2** — each node's persisted last-signed watermark never shows two hashes at one height, **across a Restart** (kill mid-schedule, restart, feed a competitor → must refuse). (#397.)
- **I3** — the quorum-basis set is constant within an epoch; a mid-epoch bank adds zero weight until the next finalized boundary. (B2.)
- **I4** — a committed-but-non-final block is reorgable; a healed network leaves no permanent non-final stall; no publish link on a non-final block.
- **I5** — fork-choice is a pure function (replay the same multiset in two delivery orders → identical head); **no honest node is ever slashed** (the accountable-safety oracle — the direct #397 catch).

Needs a read-only accessor for each replica's finalized head + last-signed watermark; check whether `core/chain` exposes these to a test in-package (likely add a small test-only getter — no production change).

## Tiering & the failing-first proof

1. **N=4 launch exhaustive** FIRST (all four scars lived here): bounded enumeration of delivery
   interleavings up to a step/height bound, anchors + up to 8 sybils, Byzantine anchor allowed to
   equivocate. This tier is the gate for the next P1 run.
2. **Handoff transition** — matured net crossing the boundary with the sybil cohort declining to attest (B2). Gates MATURING=1.
3. **Mature/randomized** — seed-logged property runs; **log the coverage bound** (S5 — a randomized pass is never printed as a proof).

**Failing-first replay strategy (DoD #2) — the one tricky bit.** The harness lives at HEAD; to prove it
would have caught each scar, cherry-pick *just the model-check files* onto the pre-fix commit and run —
observe RED — then record the commit+seed+verdict in the PR. This is a **one-time recorded validation**,
not a permanent CI job (the harness can't live at the old commit continuously). Structure the harness so
the scar schedules are self-contained (no dependency on post-fix helpers) to make the cherry-pick clean.
Per the owner v1 scope call: ship **seed + logged schedule** as the repro artifact; **auto-shrink is a
follow-up, not a gate.**

## Scope / non-goals (v1)

- No auto-shrink (owner call). No real sockets (that's e2e/field). Not a replacement for `integration/consensus` (that checks convergence of specific scenarios; this checks *no schedule violates an invariant* — the complement).
- Consensus code is **untouched** except possibly test-only read accessors for the oracle.

## M1 note (Andrew's efficiency-corner lens)

This IS the M1 answer for consensus correctness: a laptop property test in seconds replaces billable
multi-region field runs as the *discovery* tool. It does not add a hot-path cost to the shipping core
(the held-delivery mode is test-only). No efficiency corner — it removes one (the field-run-as-fuzzer).

## What would change the approach

If exposing the finalized-head/watermark to the oracle cleanly requires more than a test-only getter (i.e., a production API change), that is a consensus-touching change → research-gated (build-immutable #6) before proceeding. Expectation: a small in-package test accessor suffices.

## Refinement (found while surveying accessors) — build the IN-PACKAGE chain-level tier FIRST

Surveying the oracle accessors changed the first-step ordering. `Chain.Head()`/`Blocks()`/`Len()` are
exported, but `finalityQuorumActive()` and the node `signMark` are **unexported** — so a `sim/modelcheck`
package would need new production getters (a consensus-touching API change, research-gated). **But the
spec also names `core/chain/modelcheck_test.go` for the small-N exhaustive core**, and an *in-package*
test reads unexported state directly — **no production change, not research-gated.**

So the tractable v1 ordering is:
1. **In-package chain-level exhaustive oracle FIRST** (`core/chain/modelcheck_test.go`): enumerate
   adversary-chosen (proposer, attester-set, competing-block) combinations at each height — exactly the
   shape `fork_anchor_gate_402_test.go` already uses — and assert **I1** (no two finality-passing blocks
   at one height) + I3 (set constancy) + I5 (deterministic fork-choice / honest-never-slashed) directly
   against the real `Chain`. **This covers the core of all four scars with ZERO infra change.** It is the
   cheapest deterministic tier and the gate for the next P1 run.
2. **simnet held-delivery layer SECOND** (`sim/modelcheck/`, option A above): adds what the chain-level
   tier can't reach — I2 across a real **restart** (sign→crash→restart→competitor through the node loop)
   and I5 slashing **through the real gather** — needs the held-delivery mode + possibly a test-only
   accessor. Build only after tier 1 proves the pattern.

This inverts the spec's "first steps" list (which led with the simnet scheduler) for a good reason: the
in-package tier is cheaper, needs no infra, isn't research-gated, and already covers I1 — the invariant
all four scars violated. Lead with the cheapest tier that catches the class (the same efficiency
principle as the testing-tiers assessment). Recorded as a deliberate, reasoned deviation from the spec's
suggested order — not a silent one.

**Status:** approach decided; awaiting #402 merge to build on clean main. First code step (revised):
the in-package `core/chain/modelcheck_test.go` I1/I3/I5 exhaustive oracle for the N=4 launch regime,
with #357/#402 as failing-first replays; then the simnet held-delivery layer for I2-restart / I5-gather.
