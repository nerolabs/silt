# Trustless floor-box recompute — increment 1 (one weighted predicate, C-1 pattern)

Date: 2026-08-31
Author: Builder
Base: `origin/main` `266ae62` (F1 five whole-set digest-root leaves landed, #666)
Feeds: the trustless floor-box recompute (lane-1 Part B core), the accept path #657 defers.

## The goal (this increment)

Reproduce ONE weighted validity predicate trustlessly, from witnesses alone, proving the
C-1 weight-composition pattern the certification names as the load-bearing gap. The box
STILL never-Accepts — this is the first recompute brick, not the accept flip.

## What the certification requires (the C-1 gap, restated from source)

`v5-wholeset-digest-root-addition-RESEARCH-CERTIFICATION-2026-08-31.md`, Q2 + C-1:

> The digest binds MEMBERSHIP only. Four of the five folds are WEIGHT sums, not
> membership counts. The recompute is sound ONLY if it composes `digest (the id-set) ∪ a
> per-member value/inclusion proof for every id in the reconstructed set`. Membership-
> completeness without the per-member value proofs certifies the wrong thing.

So the recompute of a weighted fold is a THREE-part proof:
1. reconstruct the keyspace's digest root from the witnessed id-list; compare to the
   committed `K-Root` leaf (set-completeness — one missing id ⇒ different MTH ⇒ stall);
2. for EVERY id in the reconstructed set, verify its per-member weight via a per-member
   value leaf + SMT inclusion proof against the committed StateRoot (C-1 — the tally is
   forgeable without this);
3. read genesis config (`MinBond`/`Anchors`/margin) from the box's OWN config, never the
   witness (C-6).

## PACE decision — which predicate first

**Reproduce `requireEpochWeightQuorum` (the `Σ epochSet` weight super-quorum).** Recommended
by the routing and confirmed by reading the source (chain.go:2845-2866):

```
set := c.effectiveEpochSet(h)          // off the #535 boundary this is c.epochSet
total := Σ set[id]                      // whole-set WEIGHT fold — the C-1 target
support := set[proposer] + Σ set[seen]
require 3*support > 2*total            // >⅔ frozen-weight super-quorum
```

Why this predicate first, over `requireDeMatureSuperQuorum` / whole-`bonded`:

| Candidate | Fold domain | Cost | C-1 exercise |
|---|---|---|---|
| **`requireEpochWeightQuorum`** (chosen) | `Σ epochSet` weight | O(RegCap)-bounded (frozen set ≤ RegCap) | clean: one keyspace, one weight sum, one threshold |
| `requireDeMatureSuperQuorum` | `Σ bonded` weight | O(registry) — whole `bonded` | heavier; also folds `qualifiedCount` (`bonded ∧ ¬slashed`) — TWO keyspaces + a negative predicate |
| `qualifiedCount` (whole `bonded`) | count `bonded[id]≥MinBond ∧ ¬slashed[id]` | O(registry) | drags in the `slashed` non-inclusion (R-4) — defer |

`requireEpochWeightQuorum` is the cleanest single C-1 exercise: one keyspace (`epochSet`),
one weight sum, one genesis-independent threshold (⅔), O(RegCap)-bounded. It reads `epochSet`
weights (per-member value proofs) and the `epochSetRoot` digest (set-completeness). It reads
NO genesis config in the threshold itself (⅔ is a fixed constant) — but I still exercise C-6
by reading `MinBond` in the eligibility screen the box applies, so the C-6 ablation is real.
`bonded`/`slashed` O(registry) folds are a later increment (they need the negative-predicate
non-inclusion proof, R-4).

Scope guard: off the #535 recovery boundary, `effectiveEpochSet(h) = epochSet`. The boundary
case folds `liveQualifiedSet` (bonded+slashed) and is the ratified trust-the-directive carve-out
(C-2). This increment reproduces the NON-boundary `epochSet` fold only; the boundary stays the
#535 policy `floorbox_v5.go` already ships.

## The recompute structure (this increment)

New file `floorbox_recompute_v5.go`, one exported entry:

```
RecomputeEpochWeightQuorum(committedStateRoot, h, proposer, seen, witnessedIDs,
                           memberWeightWitness, digestRootWitness) (bool, error)
```

Steps, in order:
1. **Set-completeness (digest root).** Reconstruct `nodeSetMTH(witnessedIDs)` and verify the
   committed `epochSetRoot` leaf via an SMT inclusion proof against the committed StateRoot,
   AND require the reconstructed MTH == the witnessed digest value. A withheld/omitted member
   ⇒ a different MTH ⇒ mismatch ⇒ stall. This is the F1 digest root FINALLY READ.
2. **Per-member weight (C-1).** For EVERY id in `witnessedIDs`, `Resolve` the `epochSet[id]`
   value leaf against the committed StateRoot (a `ProvenPresent` with the committed weight).
   A forged weight fails `smt.VerifyProof` ⇒ `NoWitness` ⇒ stall. This is the C-1 closure:
   the digest gave membership; the inclusion proofs give the values.
3. **Genesis config (C-6).** Read `MinBond` from the box's OWN `c.cfg`, never from any
   witness, to screen eligible members. An attacker who could shift the box's `MinBond` via
   the witness could shift the tally; reading own config forecloses it.
4. **The fold + threshold.** Sum verified weights → `total`; sum proposer + `seen` verified
   weights → `support`; return `3*support > 2*total` (matching chain.go:2861 exactly).

The box NEVER-Accepts: `RecomputeEpochWeightQuorum` returns the predicate's boolean, it does
NOT flip `WitnessValidateV5` to Accept. #657 (the accept flip) stays gated until ALL predicates
are reproduced.

## Guard changes (F1 → this increment)

- **Remove `epochSetRoot`'s `isDigestRootLeaf` exclusion** in the drift guard: the digest root
  is now READ by the recompute (no longer inert for `epochSet`). Add a red-on-drop ablation for
  the `epochSetRoot` READ, matching the discipline the 23 member keyspaces get.
- **Keep the OTHER four** digest roots excluded (`bondedRoot`/`qualifiedRoot`/`slashedRoot`/
  `validatorsSeenRoot`) — they stay inert this increment. Add a SKIP-GUARDED PLACEHOLDER
  ablation per still-inert root so the "remove-the-exclusion-on-recompute" obligation lives in
  CODE, not just prose (the PE's request).

## The three hard ablations (C-5, red-before-green)

1. **Forged weight** — a witness with the RIGHT members but a forged per-member weight ⇒
   REJECT (the inclusion proof fails against the committed root). Proves C-1 closed the
   forgeable-tally hole.
2. **Omitted member** — a witness missing a frozen member ⇒ reconstructed MTH ≠ committed
   `epochSetRoot` ⇒ REJECT. Proves set-completeness.
3. **Genesis-config-from-witness** — if the recompute read `MinBond` from the witness instead
   of own config, a shifted threshold would flip the verdict; the test shifts a witness-carried
   MinBond and confirms the verdict does NOT move (own config governs). Proves C-6.

## Gates respected

- Reproduces ONE predicate; changes NO full-node consensus/validity RULE. The full node still
  computes `requireEpochWeightQuorum` from its own map (chain.go untouched). The recompute is a
  SEPARATE root-only path — additive, the same posture `floorbox_v5.go` already holds.
- Reads only COMMITTED state (via witnesses) + the box's own genesis config. No new committed
  leaf, no new format.
- If reproducing it required changing a consensus RULE, STOP + report (gated). It does not.
