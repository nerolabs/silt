# orderVacuous debt: the mature-epoch family — enumerate, deliberate, cover one

Date: 2026-08-28
Author: Builder seat
Scope: ONE field family this increment. Test/fixture-only. No rule change.

## The debt this increment attacks

The order-independence oracle (`core/chain/modelcheck_order_independence_test.go`)
compares every `committedSet` field across two OPPOSITE event orderings and asserts
byte-identity — a field under the history-independent SMT must reach the same value
however the state was reached (#597). Fields the `twoOrderings` fixture cannot
populate are compared as `DeepEqual(∅, ∅)` — vacuous — and MUST be declared in
`orderVacuous`, a shrinking debt guarded so it cannot silently grow.

Prior increments moved `spent`/`slashed` (#617) and the whole bond-registration
family (#618, which surfaced a REAL same-root order-dependence consensus fork, now
fixed at the validity layer) out of `orderVacuous`. What remains:

```
orderVacuous (5 fields):
  epochSet     — mature-epoch family
  matureEpoch  — mature-epoch family
  everMature   — mature-epoch family
  gateLockedIn — #506-gate family
  gateHeight   — #506-gate family
```

## Family enumeration, with each field's declared reason and the regime that populates it

### Mature-epoch family

| field | kind | populated when | current reason in orderVacuous |
|---|---|---|---|
| `everMature` | scalar bool (committedSet) | `MatureCoefficient() >= MatureValidators` first holds after a block's bonds/slashes are applied (`chain.go:2879`) — the one-way maturity latch (F-1) | "needs the maturity latch to trip; MatureValidators=99 keeps it launch-phase" |
| `matureEpoch` | scalar bool (committedSet) | first `rotateEpoch` at-or-after the latch trips (`chain.go:2907`) — the #357 Condition-B handoff, one-way | "needs the #357 Condition-B handoff; this world never matures" |
| `epochSet` | `map[NodeID]int64` (committedSet) | every rotation post-handoff freezes `liveQualifiedSet()` (`chain.go:2908-2909`) — #357 Condition A | "needs a mature epoch (rotateEpoch freeze); this world never matures" |

Regime needed: epochs ENABLED (`EpochBlocks > 0`), NO launch anchors (or a bonded set
that clears maturity so `everMature` latches), a bonded set of ≥ `MatureValidators`
distinct operators, and at least one epoch boundary crossed AFTER the latch trips so
`rotateEpoch` runs its post-latch path.

The `twoOrderings` world blocks this deliberately: it sets `MatureValidators=99` with 4
anchors, so `Mature()` never holds and the launch-anchor phase never ends.

### #506-gate family (NOT covered this increment)

| field | kind | populated when |
|---|---|---|
| `gateLockedIn` | scalar bool | in `rotateEpoch`'s post-latch path, when the frozen set's `regVersion >= BlockVersionRegGate` WEIGHT clears the >⅔ super-quorum (`chain.go:2922-2934`) |
| `gateHeight` | scalar uint64 | set to `h + EpochBlocks` in the same branch |

Regime needed: everything the mature-epoch family needs, PLUS a >⅔-weight coalition
carrying `regVersion[id] >= BlockVersionRegGate`, tallied across a boundary. The gate
lock-in code runs ONLY inside the post-latch branch of `rotateEpoch` — it strictly
layers on top of the mature-epoch machinery.

## Which family is more self-contained — recommendation

**Cover the mature-epoch family this increment. Leave the #506-gate family for next.**

Reasoning:

- The mature-epoch family is the SUBSTRATE. The gate family cannot be exercised without
  first standing up a matured, epoch-rotating world — the exact fixture the mature-epoch
  family needs. Building the substrate first is the smaller, self-contained step, and the
  gate fixture reuses it.
- An existing helper (`weightWorld`, in the snapshot oracle) already proves this world
  shape works: anchorless, epochs on, `MatureValidators` unset so the latch trips at
  genesis. The mature-epoch fixture is a well-trodden shape; the gate fixture adds a
  `regVersion` weight tally on top, which is genuinely more moving parts.
- Keeping them separate honors ONE-family-per-increment and lets the review land on the
  substrate before the layer that depends on it.

## The order-stress design (the #618 lesson: a commutative fixture is a decoration)

A single-actor or genesis-only fixture would be a decoration — it would not exercise the
adversarial ordering these fields could actually diverge on. The stress must contend the
exact code paths that write these fields:

1. **`everMature` (the latch).** `Mature()` reads `MatureCoefficient()` over the committed
   `bonded` ledger. The latch trips the FIRST block after which the coefficient crosses
   `MatureValidators`. The stress this increment ACTUALLY builds: bring the network to
   maturity and vary a SLASH height (1 vs 3), then run to the same final height.
   `everMature` is a one-way latch, so both end `true`. NOTE the residual named below —
   the latch HEIGHT is not varied in the built fixture; all validators bond at genesis, so
   the latch trips at the same height in both orderings.

2. **`matureEpoch` + `epochSet` (the rotation freeze).** `rotateEpoch` freezes
   `liveQualifiedSet()` — a pure read of `bonded`/`slashed` at the boundary. Because
   `rotateEpoch` runs LAST in `apply` on the final post-block state, the freeze reads only
   the converged final maps; the intermediate `bonded=5` state the slash-timing varies
   never reaches it. So `epochSet` is order-INVARIANT BY CONSTRUCTION here: a deterministic
   read of `bonded`/`slashed`, whose own order-independence #617/#618 cover. The stress
   built varies the slash height so the two `(bonded, slashed)` histories differ before the
   boundary and must converge to the same frozen `epochSet`.

3. **Assertion.** Commit the identical governing set in two opposite slash orderings, cross
   the same boundary at the same final height, and assert `everMature`, `matureEpoch`, and
   `epochSet` are byte-identical (`reflect.DeepEqual`). The existing `orderVacuous` guard
   in `TestCommittedSetFieldsAreOrderIndependent` then enforces the fields stay non-empty
   and covered.

**What this CONFIRMS vs what #618 DISCOVERED.** This fixture confirms `epochSet` is an
order-invariant read of already-covered inputs. It does NOT discover-or-refute a fork the
way #618 did — no admissible slash ordering in this world can put the divergent
intermediate state into the freeze, because the freeze is last in `apply`. A green here
means "the freeze is a deterministic read of order-independent maps," not "a
divergence-capable ordering was tried and converged."

**Un-stressed residual (on the record for the era-3 freeze).** Two dimensions are NOT
exercised, and the claim above does not cover them:

- **Latch HEIGHT under spread bonds.** All bonds are at genesis, so the latch trips at the
  same height in both orderings. An ordering where maturity-making bonds arrive across
  DIFFERENT blocks could trip the latch at different heights. `everMature` is a one-way
  final-state bool, so this cannot flip the committed value — but it is not stressed.
- **`matureEpoch` handoff HEIGHT.** Same root cause: the handoff fires at the first
  post-latch boundary, which is height 4 in both orderings because the latch is at height
  1 in both. Handoff-height variation is not exercised.

These are acceptable to defer — they cannot flip a one-way final-state bool — but they are
the honest residual, not a proven property of this fixture.

The two orderings reach the SAME logical state (same validators bonded, same final
height, same boundary). If the committed mature-epoch state DIFFERS, that is a REAL
consensus finding — an order-sensitive maturity/rotation rule under a history-independent
root — and it STOPS here for the Researcher + human, exactly as #618 did. It is NOT a test
to force green, and NO rule is touched (`chain.go:2450-2456`, `rotateEpoch`,
`MatureCoefficient()`, `apply()` tie-breaks, all off-limits).

## Two-list union rule

A field is freeze-ready only when it clears BOTH `orderVacuous` (order-independence
oracle) AND `probeUncovered` (snapshot leave-one-out oracle). `epochSet` already clears
`probeUncovered` — it has a leave-one-out probe via `weightWorld`/`weightProbes` (#604) and
a weight-bytes discriminator (#603). `everMature` and `matureEpoch` are STILL in
`probeUncovered`. This increment covers the order-independence side for all three; it adds
leave-one-out probes for `everMature`/`matureEpoch` only if a self-contained probe is
cheap. Otherwise those two clear only ONE list this increment and remain
`probeUncovered`-owed. The report states exactly which fields clear BOTH.

## Discipline

- ABLATION-FIRST for each newly-covered field: inject an order-dependence defect, paste
  the FAILING output naming the field, revert, green.
- Test/fixture-only. CHANGELOG line. This doc ships in the same PR.
- `go test ./core/... ./sim/...` and the model-check tier under `-race` → green.
- PR opened, NOT merged. STOP for review before the #506-gate family.
