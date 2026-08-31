# R-boundary: mechanical whole-set-read enumeration for the v5 floor box

Date: 2026-08-31
Author: Builder seat
Status: PACE (design before build). Test-infra + enumeration only. No production
consensus/validity rule change. The committed digest-root leaves are NOT added here
(gated format change, post Research-cert + owner ratification).

## The problem (root cause of three hand-enumeration misses)

The v5 HEAVY-posture floor box reproduces EVERY validity predicate: the full-node
acceptance contract is `apply(b) ∪ ValidateCommit(b)`. `ValidateCommit` runs the
QUORUM STACK (`requireQuorumStack` → `requireEpochWeightQuorum`,
`requireDeMatureSuperQuorum`, and, via `RequiredQuorum`, `qualifiedCount` /
`validatorSetSize`). Those predicates read committed maps AS A WHOLE — they SUM or
COUNT the entire `epochSet` / `bonded` / `slashed` map, not one key.

A whole-map SUM cannot be witnessed by per-key membership proofs. A floor box that
holds only individual `bonded[id]` witnesses can be handed a block whose
`requireDeMatureSuperQuorum` total was computed over a map with an EXTRA forged
member (raising `total`) or a MISSING member (lowering it), and it cannot detect the
forgery. To witness a whole-map sum trustlessly the box needs a COMMITTED DIGEST ROOT
(an MTH/accumulator leaf) over that keyspace, verified against StateRoot, from which
the sum is checkable. So the question this increment answers: WHICH committed
keyspaces does the contract read as a whole set (→ need a digest root) vs per-key
(→ witnessed individually)?

### Why hand-enumeration keeps missing them

The merged Part A guard (`readset_v5_drift_test.go`) is EXECUTION-DERIVED, which is
why it is trustworthy for the reads it covers. But its ground-truth driver runs only:

- `apply(b)` (the write-diff + apply-perturbation sources), and
- a VALIDITY source that runs `validateTakedowns` + per-entry `ValidateEntry`
  (`validityVerdict`, readset_v5_drift_test.go:167).

It NEVER runs the quorum stack. So the guard is structurally BLIND to every whole-map
read that lives ONLY in `requireQuorumStack`. Three hand-enumeration misses this
session all sit in that blind spot. A hand-list cannot be the fix — the SAME blindness
that produced the misses would score the hand-list green. The fix is to make the
EXECUTION-DERIVED driver run the FULL contract, then let a mechanical method enumerate.

## The mechanical method: the untouched-member perturbation oracle

DECISION. Classify each committed keyspace by a perturbation oracle that runs the FULL
contract `apply ∪ ValidateCommit`:

> For committed keyspace K, take a member m that the block b does NOT touch (b names no
> transition on m). Add m to K (or, if present, remove m) on a fresh clone. Re-run the
> full contract `apply(b) ∪ ValidateCommit(b)`. If the ACCEPT/REJECT outcome OR the
> recomputed post-state root CHANGES, then the contract read K AS A WHOLE SET → K needs
> a committed digest root.

The "member the block does not touch" clause is the whole-set discriminator. A per-key
read only consults keys the block names (a `bonded[id]` write-target for an id in
`b.BondRegs`, a `spent[serial]` gate for a serial in `b.Entries`). Perturbing an
UNTOUCHED member of a purely per-key keyspace changes nothing the contract reads — the
recompute never consults it. Perturbing an untouched member of a whole-SET keyspace
changes the SUM/COUNT the contract folds over the entire map, so the verdict or root
moves. The clause isolates exactly the whole-set channel.

### Why it is exhaustive (by construction, not a checklist)

Three properties make the oracle complete over the closed keyspace set, rather than a
list someone might under-populate:

1. **The keyspace set is closed and enumerated from the schema, not by hand.**
   `v5CommittedKeyspaceTags()` is the 23 committed v5 keyspaces, asserted `== 23`
   against `statehash.go`. The oracle runs over ALL of them; adding a keyspace to the
   schema without extending the tag list reddens the count assertion.

2. **The oracle drives the REAL contract, not a mirror of it.** The verdict is the
   composition of the production predicates `apply ∪ ValidateProposal ∪ quorum-stack`.
   Any whole-map fold ANY predicate performs — named in the cert or not — moves the
   verdict/root when an untouched member is perturbed. The method cannot miss a
   whole-set read that a hand-list forgets, because it does not consult a hand-list; it
   executes the predicate that performs the read.

3. **The perturbation covers every member class.** For each keyspace the oracle
   perturbs BOTH an untouched present member (remove it → sum/count drops) AND injects
   an untouched absent member (add it → sum/count rises). A whole-set fold is sensitive
   to at least one direction (a SUM to both; a COUNT to both; a max/threshold to at
   least one). The perturbation-coverage guard asserts every reachable keyspace is
   perturbable, so no keyspace silently escapes.

### Why not the alternatives

- **Hand-enumerate the quorum reads (rejected).** This is the exact failure mode: three
  misses this session came from hand-enumeration, and the blind guard scored them green.
  A hand-list is not falsifiable by execution.
- **Static import/AST analysis of `requireQuorumStack` (rejected).** It would flag every
  `c.bonded` / `c.epochSet` reference but cannot distinguish a `range c.bonded` (whole-set
  sum) from a `c.bonded[id]` (per-key). Distinguishing them is the whole point; only
  execution with a targeted perturbation separates them.
- **Per-key perturbation only (the existing Source 2/3, insufficient).** Deleting a
  member the block TOUCHES flips the verdict for both per-key and whole-set keyspaces,
  so it cannot classify. The UNTOUCHED-member clause is the added discriminator.

## The build (test-infra only)

1. **Extend the ground-truth driver to the full contract.** Add the quorum-stack
   predicates to `validityVerdict` so the execution-derived guard runs
   `apply ∪ ValidateProposal ∪ requireQuorumStack`. The corpus blocks must carry a
   quorum the stack can evaluate (a signed attester coalition), so the perturbation of a
   coalition-sizing map (`epochSet` / `bonded` / `slashed`) genuinely moves the verdict.

2. **Add the whole-set oracle.** For each of the 23 keyspaces, perturb an UNTOUCHED
   member and re-run the full contract; record the keyspaces whose verdict/root moves as
   the whole-set-read set.

3. **Prove the blind spot is closed (red→green ablation).** For each newly-covered
   whole-set keyspace, show a producer that omits its whole-map completeness read leaves
   the PRE-extension guard GREEN and the EXTENDED guard RED.

## DERIVED whole-set-read set (the mechanical result governs)

The oracle was RUN. The result differs from the pre-build prediction in two ways — which
is exactly why the method is execution-derived, not a hand-list.

**Quorum-stack whole-set reads (the blind spot this increment closes):
`{bonded, epochSet, slashed, validatorsSeen}`.**

- **`bonded`** — `requireDeMatureSuperQuorum` sums the whole map (`total`, chain.go:2949);
  `qualifiedCount` counts the whole map (chain.go:1481).
- **`epochSet`** — `requireEpochWeightQuorum` sums `effectiveEpochSet` = whole `epochSet`
  (`total`, chain.go:2851); `validatorSetSize` reads `len(epochSet)` (chain.go:1561).
- **`slashed`** — `qualifiedCount` folds `!slashed[id]` over the whole `bonded` domain
  (chain.go:1482); an untouched slash drops N. This is the keyspace the ≥4 list OMITS.
- **`validatorsSeen`** — the de-mature gate `!c.matureNow()` (chain.go:2827) folds
  `MatureCoefficient` → `C2Metric` over the WHOLE `validatorsSeen` map (objective mode,
  chain.go:2305). An untouched `validatorsSeen` member can flip `matureNow()` and turn the
  de-mature bar on/off. This read lives behind a GATE (`matureNow`), not a direct sum —
  the shape hand-enumeration keeps missing.

**Apply-channel whole-set reads (already covered by the merged guard's Source 1 write-diff):
`{qualified, validatorsSeen, bonded, slashed}`.** `qualified` is here, NOT in the quorum
stack — the boundary freeze ranges the whole `qualified` map. The ≥4 list mis-attributed it
to a quorum fold.

**EXHAUSTIVE full-contract union: `{bonded, epochSet, qualified, slashed, validatorsSeen}`.**

**Reconciliation vs the ≥4 starting list `{qualified, epochSet, validatorsSeen, bonded}`:**
it OMITS `slashed` (a real quorum-stack whole-set read) and MIS-ATTRIBUTES `qualified`
(apply-channel, not quorum-stack). `validatorsSeen` is read whole-set by BOTH channels.

**Exhaustiveness closed by execution, not assumption:** the oracle skips the 18 non-
NodeID-weight keyspaces; `TestQuorumStackNonWholeSetKeyspacesCleared` PROBES every skipped
NodeID-keyed map (`bondDomain`, `regVersion`, `bondRegHeight`, `bondRootOwner`,
`bondRootProven`) under the full contract and confirms NONE flips a verdict — so they are
cleared as non-whole-set by execution. (`bondDomain` is read by `C2Metric` but only for
`validatorsSeen` members, so an untouched `bondDomain` member outside `validatorsSeen`
changes nothing — a per-key, `validatorsSeen`-scoped read, not a whole `bondDomain` fold.)

## Scope / gates

Test-infra + enumeration. NO committed digest-root leaves (gated format change). If the
build must touch production consensus/validity code, STOP and report. The increment
ESTABLISHES the exhaustive whole-set-read set and CLOSES the guard's quorum-stack blind
spot; adding the digest roots is the next, gated increment.
