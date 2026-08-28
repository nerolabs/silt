# Order-independence: covering the bond-registration family (#617 debt)

Date: 2026-08-28
Author: Builder seat
Status: increment — closes part of the `orderVacuous` debt opened by PR #617

## The debt

PR #617 added a non-emptiness guard to the order-independence model-check oracle
(`TestCommittedSetFieldsAreOrderIndependent`) plus an `orderVacuous` map: committed
fields the `twoOrderings` fixture compares over ∅ in every ordering. The era-3 state-root
freeze gate requires every committed field be proven order-independent by a COVERING
probe (non-vacuous), and — the two-list union rule — also proven load-bearing by a
covering leave-one-out probe in the snapshot-equivalence oracle. A field is freeze-ready
only when it clears BOTH lists.

This increment covers the **bond-registration family** of `orderVacuous`:
`bonded` (already covered elsewhere for slash-delete; here for registration),
`bondRootOwner`, `bondRootProven`, `bondRegHeight`, `regVersion`, `bondDomain`.

Explicitly OUT of scope (later increments): the mature-epoch family
(`epochSet`, `matureEpoch`, `everMature`) and the #506-gate family
(`gateLockedIn`, `gateHeight`).

## The mechanism (attribution)

The bond-registration state is written by `apply()`, chain.go:2769-2799, iterating
`b.BondRegs` in slice order. Per-reg it may set `bondRootOwner[root]`, `bondRootProven[root]`,
`bonded[id]`, `bondRegHeight[id]`, `regVersion[id]`, `bondDomain[id]`.

`bondRegHeight[id] = b.Height` is the load-bearing constraint on how we vary order. It binds
a validator to the HEIGHT its registration was committed. If the same validator's reg lands
at a different height in the two orderings, `bondRegHeight` genuinely differs — but that is a
FIXTURE artifact (we moved the event), not a consensus finding, and it is not the "same
logical end state" the oracle requires. So the two orderings must preserve each event's
height and vary only the **processing order WITHIN a height** (the slice order of
`b.BondRegs` inside one block, and the order of independent same-height registrations).

That intra-block order is exactly where the G3 displacement rule could be order-sensitive.

## The G3 displacement rule (the one that could genuinely matter)

chain.go:2780-2794. A genesis (height 0) BondReg is `proven=false`: it sets
`bondRootOwner[root]=squatter`, `bonded[squatter]=size`, leaves `bondRootProven[root]`
false. A later height>0 proven BondReg by a DIFFERENT id on the SAME root fires the
displacement branch: `delete(c.bonded, squatter)`, `bondRootOwner[root]=honest`,
`bondRootProven[root]=true`.

The genesis declaration is definitionally height 0 (applied first, at boot). The proven
displacement is a height>0 block. These two events CANNOT swap heights — genesis is always
first. So "reverse genesis and the claim" is not a legal ordering. The order variable that
IS legal and could matter: within the displacing block, the slice order of the honest
proven claim relative to OTHER BondRegs, and the order of independent registrations.

## The construction

`bondOrderings(t)` builds a world with committed BondRegs and commits the SAME
registrations in two OPPOSITE intra-block orders, every event at the identical height in
both:

- Genesis (height 0, shared, identical in both orderings): declares a squatter `sq` as the
  unproven owner of `rootShared` (the G3 pre-condition), plus a normal genesis bond for an
  anchor so quorum holds.
- Height 1 (one block, TWO BondRegs whose SLICE ORDER is the variable):
  - the honest holder `h` registers a PROVEN claim on `rootShared` → G3 displacement of `sq`.
  - an independent validator `x` registers a PROVEN bond on its own `rootX`.
  Ordering A commits `[h-claim, x-reg]`; ordering B commits `[x-reg, h-claim]`.

Both orderings therefore reach the identical logical end state: `sq` displaced (removed
from `bonded`, no longer owner), `h` the proven owner of `rootShared`, `x` bonded on `rootX`.
Every field lands at height 1, so `bondRegHeight` is order-free by construction of the
heights (the property under test is that the intra-block slice order does not perturb it).

This populates all six family fields non-empty in both orderings:
`bonded` (h, x, plus the genesis anchor), `bondRootOwner` (rootShared→h, rootX→x, plus
anchor root), `bondRootProven` (rootShared→true, rootX→true), `bondRegHeight`,
`regVersion`, `bondDomain` (via a non-zero Version/Domain on the proven regs).

## Why this is a legitimate order test, not forced green

The oracle asserts the full committedSet is byte-identical across the two orderings. The
ablation (below) proves the assertion has teeth: an injected intra-block order-dependence
(processing BondRegs by a rule that is NOT commutative) makes the fixture go RED and NAMES
the divergent field. Only after showing that red do we remove the fields from `orderVacuous`.

## Consensus-correctness trip-wire

If the G3 probe had shown the final bond-root / bonded state DIFFERING between the two valid
orderings, that would be a REAL consensus finding (an order-sensitive displacement validity
rule under a history-independent root), STOP-and-escalate, no rule change by this seat.
The result is reported in the PR.

## Two-list union — snapshot-equivalence coverage

`bondRootProven`, `bondRegHeight`, `regVersion`, `bondDomain` are currently in
`probeUncovered` (the snapshot oracle's debt). `bondRootOwner` already has a leave-one-out
probe (the F1 dedup probe). `bonded` clears via the objective-bonded world. To clear the
union rule, this increment adds covering leave-one-out probes for the fields that can flip a
real verdict on omission and removes them from `probeUncovered` where genuinely covered.
Fields whose omission cannot flip a VALIDITY verdict (only a metric) stay declared with the
honest reason. The PR reports exactly which fields clear BOTH lists.

## What actually happened (results)

- **G3 came back order-INDEPENDENT (safe).** The displacement fired in both orderings
  (`TestBondRegG3DisplacementIsOrderIndependent`: squatter removed from `bonded`, honestH
  the proven owner, validatorX bonded on its own root) and all six bond fields are
  byte-identical across the two orderings. The consensus-correctness trip-wire did not
  trip. Ablation evidence: forcing `apply()` to process only the first BondReg per block
  made the oracle go RED naming all six fields
  `[bonded bondRootOwner bondRootProven bondRegHeight regVersion bondDomain]`, then revert → green.

- **A latent test-infrastructure bug surfaced, and was fixed.** Adding a SECOND mutating
  leave-one-out probe (for `bondRootProven`) exposed that `snapshotBoot` carried committed
  maps by REFERENCE. A mutating probe's `apply()` wrote through the shared header into
  `src`, so the `bondRootOwner` ablation (which comes first in struct order) corrupted
  `src` before `bondRootProven` was ablated — masking the flip. Isolated diagnosis proved
  `bondRootProven` IS load-bearing (full=proven-owner-held, ablated=displaced-the-proven-owner);
  the loop just never saw it. Fix: `snapshotBoot` deep-copies carried maps/slices. This is a
  test-helper aliasing bug, not a consensus rule — no research gate. It hardens every future
  mutating probe in the leave-one-out loop.

## Fields clearing which lists

- **Both lists (freeze-ready by the union rule):** `bonded`, `bondRootOwner`, `bondRootProven`.
- **Order-independence list only** (snapshot-equivalence coverage still owed, tracked in
  `probeUncovered`, out of scope here):
  - `bondRegHeight` — read only by the #506 reg-gate min-interval rule, gated behind
    `regGateActive`; needs a gate-active world (#506 family).
  - `regVersion` — read by the #506 rotateEpoch lock-in tally; needs a boundary (#506).
  - `bondDomain` — read only by C2Metric (a metric, not a validity predicate), so omission
    cannot flip a verdict; a leave-one-out probe would need a metric assertion, not a
    verdict flip.
