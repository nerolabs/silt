# silt — Consensus Model-Check (harness spec)

**Status: ADOPTED build spec (owner ratified 2026-08-14).** Companion to `consensus-invariants.md` (I1–I5, ADOPTED canon).

**Gating (canon, 2026-08-14) — the tier order is `unit → consensus model-check → integration/sim → e2e/netem → field`, and each graded field run is gated on the model-check tier covering its regime:**
- **launch exhaustive tier green** (with the #357/#397/#402 failing-first replays proven) → gates the next **P1 all-corners run**;
- **handoff tier green** (B2 replay) **+ the #399 WS-recovery drill** → gates the **MATURING=1 run**;
- **full schedule budget green** → red-team (#183) entry criterion.

The evidence for the gate is the base rate it corrects: **four consecutive graded field runs each discovered a new consensus bug** (#357, B2, #397, #402). A field run's job is to *confirm* on real WAN what the model proved — never to discover an invariant (build-immutable #6/#7).

**v1 scope call (owner, 2026-08-14):** v1 ships **seeded replay + the logged schedule** as the deterministic repro artifact; **automatic schedule-shrinking is a follow-up, not a gate** — the seed already satisfies the #6 requirement, and holding the gate hostage to a shrinker would delay the class closure it exists for. This is the tool that ends the "discover a consensus invariant by multi-region field run" loop: it asserts I1–I5 under adversarial scheduling **on a laptop, deterministically, in seconds** — so the field run returns to *confirming* what the model proved, not *discovering* what nobody specified (build-immutable #6, applied to the consensus layer as a whole).

**Provenance / the case for it.** #357, B2, #397, #402 were each found by an expensive, slow, non-deterministic multi-region field run — one bug per run, over days. All four are corollaries of I1/I3/I4. A property test over the *deterministic core* would have found all four in one afternoon. silt already has the substrate to build it: the lock-free single-loop core (B2 tenet), `simclock` (seeded scheduler), and `simnet` (latency/loss/partition) mean consensus runs **identically in-process** — the model-check is not new infrastructure, it is a new *driver + oracle* over infrastructure that already exists.

---

## What it is, and is not

- **IS:** a deterministic, adversarial **property** test over the in-process consensus core. It drives N validators (anchors + honest bonded + Byzantine + sybils) through the real `Chain`/node loop under a scheduler that controls delivery order, delay, partition, restart, and the Byzantine node's message choices, and — after **every step** — asserts the I1–I5 oracle holds. On violation it emits the **seed + the shrunk minimal schedule** that broke it: a laptop repro, the exact #6 artifact.
- **IS NOT** a field test (no real sockets), and **IS NOT** the existing `integration/consensus` sim (which checks that *specific scenarios converge*, mostly in the mature regime). The existing sim asks "does this scenario work?"; the model-check asks "**does *any* adversarial schedule violate an invariant?**" — the complement, and the one that finds the seams.

---

## Architecture

Reuse, don't rebuild:
- **Core under test:** the real `core/chain` + `core/node` consensus path, unmodified (that is the point — it must be the shipping code).
- **Clock:** `adapters/simclock` (seeded, single-thread) — every run reproduces from its seed.
- **Network:** `adapters/simnet` extended with an **adversarial delivery scheduler** (below).
- **Driver:** a new `sim/modelcheck/` (or `core/chain/modelcheck_test.go` for the small-N exhaustive core) that constructs the validator set, runs the scheduler, and calls the oracle.

### The adversary (what the scheduler may do)
Within the stated fault bound (≤ f Byzantine of the relevant set):
1. **Network:** arbitrary delay, reorder, and drop of messages; induce and heal **partitions** at any step (bounded so liveness is testable — an eventually-synchronous model).
2. **Byzantine node(s):** equivocate (sign conflicting blocks/votes), withhold, propose competing blocks at a contested height, send a valid-looking vote to disjoint subsets — everything a real malicious anchor/validator can do. This is what turns #402/#397 from "honest race" into the **adversarial** variant the red team will try.
3. **Crash-restart:** kill and restart any node at any step — the direct test of I2 persistence (sign → crash → restart → present competitor).
4. **Validator-set churn:** bank bonds / expire TTLs at adversarial times and heights — the direct test of I3 (mid-epoch admission must gain no quorum weight).

### The oracle (asserted after every scheduler step)
| Invariant | Oracle check |
|---|---|
| **I1** | No two distinct blocks at one height both satisfy the *finality* predicate — across all replicas, any partition. |
| **I2** | No node has signed two different blocks at one height in its whole history, **including across a restart** (check against the persisted watermark, not just live memory). |
| **I3** | The quorum-basis set is constant within an epoch; a mid-epoch bank adds no weight until the next finalized boundary; quorum is weight, not count. |
| **I4** | Any committed-but-non-final block is reorgable; in a connected (healed) network every fork is resolved (no *permanent* non-final stall); no publish link is issued on a non-final block. |
| **I5** | Fork-choice is a pure function (replay the same message multiset in two orders → identical head); **no honest node is ever slashed** (accountable safety). |

A violation halts with the seed and the schedule; the harness then **shrinks** the schedule to the minimal delivery sequence that still violates, and prints it as a deterministic repro. *(v1: the seed + logged schedule is the required repro artifact; auto-shrink is a follow-up — see the scope call above.)*

---

## Schedule-space coverage

- **Launch phase, N=4 (+ up to 8 sybils):** this is where *all four* scars lived. Bounded **exhaustive** exploration of small schedules (all interleavings up to a step/height bound) — small enough to be tractable, and it is the exact regime the field kept tripping.
- **Handoff transition:** targeted schedules that mature the net and cross the boundary with the sybil cohort present and **declining to attest** (the B2 / weight-quorum drill) — the transition is where I1 and I3 interact and where in-process coverage has been thinnest.
- **Mature phase, larger N:** randomized schedules with shrinking (property-based), since exhaustive is intractable; seed-logged so any failure replays.
- **Restart & churn overlays:** applied across all of the above (I2/I3).

Where coverage is bounded (mature-phase randomization is not exhaustive), **log the bound** — never let a randomized pass read as a proof (S5 honesty; the same "don't fake green" rule as the field harness).

---

## Definition of Done

1. **The harness exists** and runs in `go test` on a laptop (seconds for the exhaustive N=4 tier; minutes for the randomized mature tier).
2. **It reproduces all four scars as failing-first:** checked out at the commit *before* each fix, the model-check goes **RED** on #357, B2, #397, and #402 (each caught by the invariant the table above assigns it); at/after each fix it goes **GREEN**. This is the proof that it would have caught them without a field run — and the regression wall that stops their return.
3. **It is wired as the first consensus gate:** the tier order becomes **unit → consensus model-check → integration/sim → e2e/netem → field**. A consensus-touching PR must be model-check-green before a field run is spent on it. This operationalizes "the field run confirms, it does not discover."
4. **It is a red-team entry criterion:** the model-check must be green on the **full** schedule budget (all five invariants) before the external red team (#183) starts — so the red team attacks what the model *couldn't* break, not what it never checked. A break they find that the model-check didn't is either an invariant the set was missing (add it to `consensus-invariants.md`) or a schedule class the harness didn't explore (widen it) — both are precise, actionable signal.

---

## Why this is also the M1 and morale answer

- **M1 / efficiency:** a laptop property test in seconds replaces multi-region billable field runs as the *discovery* tool for consensus correctness. Cheaper, faster, deterministic — and it frees the expensive field run for what only it can prove (real WAN liveness/timing at scale).
- **The tail stops:** I1–I5 is a *closed* set; once the model-check asserts all five across the schedule space, the stream of "new" consensus surprises stops (a genuine new one means the set was incomplete — a rare, high-signal event, not a weekly occurrence). The perimeter becomes finite and visible, which is the difference between walking a fence and chasing a tail.

---

## First steps (suggested order)

1. Extend `simnet` with the adversarial delivery scheduler + partition/restart controls.
2. Write the I1–I5 oracle as a single `assertInvariants(replicas)` called each step.
3. Land the **N=4 launch exhaustive** tier first and prove the #357/#397/#402 failing-first replays (B2 at the handoff tier).
4. Add the handoff and mature/randomized tiers.
5. Wire it as the pre-field consensus gate and the red-team entry criterion.

*Build the oracle once, drive the real core under an adversary, and let a laptop enumerate the perimeter the field has been discovering one region at a time.*
