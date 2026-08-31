# Floor-box state-root recompute — P1-b class S (slashes): the changed-digest write-set primitive

Date: 2026-08-31
Seat: Builder
Base: `origin/main` `8a9a505`
Certs built to (full paths):
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md`
- `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-recompute-P1b-SA-digest-scope-2026-08-31.md`

## The problem

The O(payload) HYBRID state-root recompute (`RecomputeStateRootEntriesRevocations`,
`floorbox_recompute_stateroot_v5.go`) reproduces `validateEra3Roots`' `StateRoot` equality
root-only, for classes E + R only. Those two classes touch NO whole-set digest scalar leaf
(byRoot / spent / revoked have no committed digest root). Every other class does. The scope
gate stalls S/A/B/T/P/M out-of-scope.

Class S (slashes) is the first delta-derivable digest class. A slash of `culprit`:
- `slashed[culprit] = true`     → adds a `slashed||culprit` per-member leaf + changes `slashedRoot`
- `delete(bonded, culprit)`     → deletes a `bonded||culprit` per-member leaf + changes `bondedRoot`
- `qualifiedMaintain(culprit)`  → deletes `qualified||culprit` if it was qualified + changes `qualifiedRoot`

(`chain.go:3285-3290`). The per-member leaves are ordinary changed leaves the existing E/R
fold shape already handles. The THREE digest scalars are the new object: each is an MTH over
the WHOLE post-state id-set of its keyspace, and reproducing `postRoot == b.StateRoot` requires
reconstructing each.

## The certified primitive (build to it exactly)

For each touched digest scalar, a `FoldOp` whose:
1. `OldValue = preDigest`, proven against **prevStateRoot** (NOT StateRoot — that is circular;
   residual R-anchor-prevroot). The existing `FoldChangedPaths` already verifies each op's
   `OldValue` against `prevStateRoot` via `smt.VerifyProof`, so routing the digest scalar as a
   `FoldOp` with the pre-digest as `OldValue` gets the anchor for free.
2. The box reconstructs the pre-set id-list from the witness and requires
   `nodeSetMTH(pre-list) == preDigest`. An omitted/injected pre-member yields a different MTH →
   stall. This is the completeness anchor (cert sub-Q2).
3. Applies the payload-DERIVED membership delta (from `b.Slashes` + own cfg + per-member
   witnesses) to the pre-set → post-set.
4. `NewValue = nodeSetMTH(post-list)`.

Then `FoldChangedPaths` + the terminal `postRoot == committedStateRoot` equality closes it. No
new fold primitive, no apply() change, box still never-Accepts.

## The S write-set derivation

Per culprit in `b.Slashes` (culprit = `Equivocation.CulpritID()`):
- `slashed` set: ADD culprit (membership). Always.
- `bonded` set: DELETE culprit IF culprit was bonded pre-state (a slash of an unbonded id does
  not change bonded). Derived from the culprit's witnessed pre-state bonded membership.
- `qualified` set: DELETE culprit IF culprit was qualified pre-state. Qualified membership is
  `filter(bonded, slashed, MinBond)` (`idQualifies`, `chain.go:1364`). Post-slash the culprit is
  slashed, so it is unqualified regardless. So the delta = DELETE culprit from qualified IF it was
  qualified pre-state. Derived from the culprit's pre-state qualified MEMBERSHIP as reconstructed
  in the anchored pre-set — NOT trusted from a witness scalar (C-1). The box works from the
  completeness-anchored pre-set id-lists (each anchored to its pre-digest), so "was it a member"
  is answered by the anchored pre-set, not a witness claim. The forged-per-member-value ablation
  (C-5) proves the delta is derived, not trusted.

The pre-set id-list for each digest is reconstructed from a witnessed id-list and anchored to the
pre-digest. The delta is applied to that verified-complete pre-set.

## The honest cost statement (R-cost-wholeset, R-membership)

NOT O(payload). `nodeSetMTH` is a whole-list fold with no incremental update, so reconstructing
any changed digest needs the WHOLE post-set id-list. Cost is **O(payload leaves) + O(|keyspace|)
per touched digest** — O(|slashed|) + O(|bonded|) + O(|qualified|), i.e. O(registry) per touched
digest. This rides directly on R-membership (no code-enforced bound on total bonded/qualified/
slashed membership), OPEN and load-bearing for the #657 accept-flip. Box-fits at RegCap-era
populations (kilobytes per digest); degrades to megabytes-per-block at 100k-member populations.
The primitive states this honestly; it does NOT claim O(payload).

## Scope decisions

- **In scope:** S blocks (slash-only, plus the E/R writes a slash block may also carry).
- **Out of scope, still stalling:** A (frozen-epochSet screen — R-A-frozenset), B (displacement
  residual), T (dueBucket), P (whole-set overwrite), M (scalar latch). The scope gate keeps
  A/B/T/P/M stalling. A slash block that ALSO trips a boundary / carries a bond reg / fires a TTL
  expiry / has a non-proposer att stalls (out-of-scope compound).

## `isDigestRootLeaf` / `inertDigestRootTags` — NOT touched, and why

The task says "remove their `isDigestRootLeaf` exclusions IF now genuinely read." That guard
(`readset_v5_drift_test.go`) governs the READ-SET PRODUCER (`WitnessReadSetV5`, `readset_v5.go`),
a DIFFERENT path from this WRITE-side state-root recompute. `RecomputeStateRootEntriesRevocations`
carries its own `StateRootWitness` bundle and does not flow through the read-set producer. My S
change adds no read of `qualifiedRoot`/`slashedRoot` to the PRODUCER, so removing the exclusion
would falsely redden `TestInertDigestRootsAwaitRecompute` (it asserts those two roots stay inert
until a PRODUCER recompute reads them). `bondedRoot` is already out of inert (read-set increment 3).
So: KEEP `qualifiedRoot`/`slashedRoot` inert; do not touch the read-set drift guard. The write-side
digest consumption is guarded by this file's own red-on-drop ablations (below), not the producer's.

## Ablations (C-5, red-before-green)

1. S block agrees with real apply() + StateRootForVersion(5) (the R3 oracle) — GREEN.
2. Forged per-member value in the qualified screen (claim culprit unqualified when it was
   qualified) ⇒ wrong post-qualified digest ⇒ post-root ≠ StateRoot ⇒ stall.
3. Mis-derived delta — slash does NOT delete bonded (leave culprit in the reconstructed post-bonded
   set) ⇒ wrong bondedRoot ⇒ stall.
4. Wrong culprit in the delta ⇒ wrong digests ⇒ stall.
5. OMITTED touched-digest reconstruction (drop the qualifiedRoot op) ⇒ post-root missing the
   qualifiedRoot change ⇒ stall.
6. Pre-set anchored against StateRoot instead of prevStateRoot (the circular bug) ⇒ the fold's
   own per-op VerifyProof is against prevStateRoot, so a pre-digest proof issued against StateRoot
   fails there ⇒ stall. Proves the anchor is prevStateRoot.
7. Byte-exact: the reconstructed digests match `statehash`-emitted `nodeSetMTH(post)` for the real
   post-state.

## Decision

Build the changed-digest `FoldOp` primitive (`stateRootDigestOp`) + wire the S write-set into
`RecomputeStateRootEntriesRevocations` behind a widened scope gate. Reuse `FoldChangedPaths` /
`nodeSetMTH`. State the O(registry)-per-digest cost honestly. Keep the box never-Accept. Ship with
the seven red-before-green ablations. Do NOT touch the read-set producer or its drift guard.
