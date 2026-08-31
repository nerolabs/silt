# 2026-08-30 — #406 consensus model-check: the unified step-oracle increment

**Seat:** Builder. **Issue:** #406 (consensus model-check I1–I5). **Branch:**
`feat/406-consensus-modelcheck-harness` off `origin/main` `61c75eb`.

## What already exists (verified by reading, not memory)

The consensus model-check tier is NOT a greenfield. Reading the tree on `61c75eb`, every
one of I1–I5 already has an exhaustive, controlled-revert-ablated oracle:

| Inv | Oracle file | Shape |
|---|---|---|
| I1 launch | `core/chain/modelcheck_test.go` | exhaustive anchor coalitions, no two disjoint finalize (A∈{3,4,5}) |
| I1 mature / I3 | `core/chain/modelcheck_i3_test.go` | exhaustive weighted coalitions, finality ⟺ >⅔ weight (B2) |
| I2 | `core/node/modelcheck_i2_exhaustive_test.go` | signed-slot × competitor-slot across a real restart |
| I4 liveness | `core/node/modelcheck_i4_liveness_test.go`, `modelcheck_441_*`, `modelcheck_451_*` | wedged-height recovery, publish-starvation, view-sync |
| I5 | `core/chain/modelcheck_i5_accountable_test.go` | equivocation predicate exhaustive; fork-choice all 24 permutations |

Each is failing-first proven (recorded controlled reverts). Rebuilding any of these would
be redundant green — the exact decoration the hard bar forbids.

## The genuine gap — the spec's architectural centerpiece is unbuilt

`docs/design/consensus-model-check.md` line 40–49 and DoD/first-steps name the piece the
per-invariant oracles do NOT provide:

> Write the I1–I5 oracle as a single `assertInvariants(replicas)` called each step.

The existing oracles each drive their OWN bespoke scenario and assert their OWN invariant
inside it. There is no cross-cutting monitor that runs over the LIVE multi-replica state of
an arbitrary driven scenario and asserts I1/I5 continuously — so an invariant break that
surfaces in a scenario written to probe a DIFFERENT invariant goes unwatched. The spec's
whole "assert after every step" model — the thing that turns a scenario driver into a
property harness — is missing.

`grep -rn assertInvariants core/ sim/` returns nothing. Confirmed absent.

## Decision — scope for THIS increment

Build the unified continuous oracle `assertInvariants(replicas)` and the step-driver that
calls it after every delivery, over the real multi-node held-delivery world (`s1s2World`,
`drainHeldExcept`) that the round oracles already use. Cover the two invariants that are
expressible as pure functions of observable cross-replica state, to the hard bar:

- **I1 (agreement):** no two replicas expose DIFFERENT finalized block hashes at the same
  height. Read via `Chain().FinalizedHeight()` + `Chain().Blocks(h)` per replica. This is
  I1's observable consequence: an intersecting quorum ⇒ at most one final block per height
  ⇒ all replicas that finalized height h agree on its hash. A non-intersecting quorum shows
  here as two replicas final-disagreeing.
- **I5 (accountable safety):** no honest replica is ever slashed, observed live via
  `OnSlash` on every honest node through the whole schedule.

These two are exactly the pair the spec's oracle table marks as "across all replicas, any
partition" (I1) and "no honest node is ever slashed" (I5) — the continuous ones. I2/I3/I4
are predicate/liveness properties already exhaustively covered at their own tiers; folding
them into the step-monitor adds no red the dedicated oracles do not already own, so I do
NOT reimplement them here (simplicity — cover what the monitor genuinely adds).

**Why stop at two, to the hard bar, rather than five shallow:** the task's own rule —
quality over coverage, a green check with no demonstrated red is decoration. The monitor's
value is being a SINGLE oracle that catches I1/I5 in ANY driven scenario. I prove that
value by ablating both directions with a controlled Byzantine driver that a per-invariant
scenario oracle would not have been pointed at.

## The harness design

- **Driver:** `stepDriver` wraps the `s1s2World` net. It exposes `deliverNext(hold)` which
  delivers exactly ONE parked message (or fires one node sweep), then calls
  `assertInvariants` — the "after every step" contract. Deterministic: message pick is
  FIFO over `net.Pending()`, node order is fixed, no wall-clock, no map iteration in the
  ordering. Same world + same schedule ⇒ same result, every run (seed-replay, spec v1).
- **Oracle:** `assertInvariants(t, replicas)` reads each replica's finalized suffix and the
  honest-slash flag, asserts (a) finalized-hash agreement across replicas at every common
  height, (b) no honest slash. Called after every step AND at quiescence.
- **Adversary for the ablation:** a Byzantine anchor that proposes-and-commits two
  DIFFERENT blocks at one height to two DISJOINT honest subsets (the `proposeAndCommitTo`
  primitive already in `core/node/adversary.go`). Under an intersecting finality quorum
  this cannot make two replicas BOTH finalize the height → oracle GREEN. The ablation
  forces the split to land as two finalized heads.

## Ablation plan (the hard bar — each covered invariant goes RED then GREEN)

- **I1 agreement:** inject by having the driver deliver the equivocating anchor's two
  conflicting single-anchor commits to disjoint replicas AND temporarily accept them as
  final (drive each target with a lone-anchor commit while asserting the monitor). The
  RED is "replicas R0 and R1 finalized DIFFERENT hashes at height h." Restore: require the
  real strict-anchor majority so neither lone-anchor commit finalizes → GREEN. The RED is
  produced by a test-local relaxation of the DRIVER's finality bar (a test knob), never by
  editing consensus code.
- **I5 honest-never-slashed:** inject by feeding the monitor a synthetic honest-slash
  event (flip the observed `OnSlash` honest flag in a `_ablation` sub-run) → monitor goes
  RED "honest replica slashed." Real schedule: no honest slash → GREEN.

Both ablations are recorded in the test file's failing-first comment with the exact
red→green, per the discipline of the sibling oracles.

## Scope / gates

Test-infra only. No consensus rule, validity predicate, or `apply()` change. The I1
ablation relaxes a TEST driver's acceptance bar, not `ValidateCommit`. If a genuine red
demanded a production consensus change, that is gated (Researcher) and out of this
increment — it did not.
