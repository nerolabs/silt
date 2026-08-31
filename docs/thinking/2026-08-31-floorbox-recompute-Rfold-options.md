# PACE — the O(payload) multi-leaf state-root recompute for the floor box (P1-a, E + R)

Date: 2026-08-31
Author: Builder
Scope: the R-fold primitive + the E/R hybrid recompute + the scope-gate re-anchor.
Certs this builds on (full paths):
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-recompute-P1a-Opayload-multileaf-RESEARCH-CERTIFICATION-2026-08-31.md`
- `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-recompute-P1a-Opayload-multileaf-2026-08-31.md`

## The decision, up front

Build the **certified HYBRID**: the box derives the E/R write-set from the block payload
itself, collects a pre-state proof for each changed leaf against `prevStateRoot`, folds only
those changed paths to compute the post-state root, and requires the computed root ==
`b.StateRoot`. It keeps P1-a's **never-Accept** posture (it is a recompute mechanism, not an
Accept path). The whole-pre-state transfer (the superseded O(whole-state) P1-a) is dropped;
completeness moves to (payload generator + per-changed-path proofs + a dueBucket digest for
the TTL scope gate).

**The tension with the PE ruling, resolved.** The PE ruling (same date) says "build Option 2
(verify pre+post per-leaf proofs), not the fold." The research certification (same date, the
soundness gate) REFUTES standalone Option 2 as a self-sufficient closure — it has a concrete
wrong-accept (an adversary changes an un-named leaf `X`; every named proof still verifies) —
and certifies the HYBRID (payload-derived write-set + the fold over changed paths) as the sound
O(payload) spine. On a soundness question the research certification is the gate. The two are
reconcilable: the PE's Option 2 is sound ONLY when the box derives the write-set itself (not the
prover) and closes "nothing else changed" — and once you add that derivation, the cheapest sound
"nothing else changed" IS a fold over the derived paths, which is the hybrid. I build the hybrid.
I honor the PE's decisive engineering guidance inside it: **do not hand-roll the tree surgery.**
See "How the fold avoids the 36% trap" below.

## How the fold avoids the 36% trap — delegate surgery to the library

The naive attempt hit 36% wrong-root because it hand-rolled the composition of multiple changed
leaves: add-displacement at a data-dependent `prefixLen` (`smt.go:154-195`), delete
sibling-promotion (three cases, `smt.go:305-317`), and extension split/absorb. Reproducing that
byte-exact across interacting paths is the trap.

The fold does NOT reproduce it. It reconstructs a **partial trie** rooted at `prevStateRoot`,
seeds a `simplemap` node store with exactly the node preimages along the changed paths (derived
from each changed leaf's pre-state proof), then calls the library's OWN audited `Update` /
`Delete` for each payload write, then reads `Root()`. All tree surgery — displacement,
promotion, extension split/absorb, shared-prefix reconciliation — is done by
`pokt-network/smt@v1.0.0` itself, the same code that produced `b.StateRoot` on the honest node.
The box writes no digest arithmetic beyond reconstructing the partial trie's node preimages,
which it PINS byte-exact against the library.

### Reconstructing the partial trie from proofs (the one novel step)

`statehash.Prover.Prove(key)` yields a `SparseMerkleProof` with `SideNodes` (full-depth sibling
DIGESTS, extension spans expanded to placeholders — cert Fact 1) plus `NonMembershipLeafData`
(the displaced leaf for a non-membership proof) and `SiblingData` (the immediate sibling's
preimage). To seed a partial trie I need, for each changed key, every node preimage along its
descent path so the library can `resolveLazy` down it:

1. Verify each changed key's proof against `prevStateRoot` via the library `VerifyProof`
   (rejects a forged/omitted proof → stall). This gives the trusted pre-state leaf value (or
   proven-absence) for that key.
2. Reconstruct the on-path node preimages bottom-up, exactly as `verifyProofWithUpdates`
   folds (`proofs.go:437-450`): start from the leaf digest, and at each level combine the
   current on-path digest with the level's sidenode digest via `encodeInnerNode` (ordered by the
   path bit) → the inner node preimage; store `digest → preimage` in the seed map. Placeholder
   sidenodes (extension spans) produce inner nodes with a placeholder child — the library
   resolves a placeholder to nil (`smt.go:560`) and re-forms the extension on write, so no
   explicit extension reconstruction is needed on the SEED side.
3. Also store the leaf preimages (the changed leaf itself, and any `NonMembershipLeafData`
   displaced leaf) so an add that displaces a resident leaf finds it.
4. `ImportSparseMerkleTrie(seedMap, sha256, prevStateRoot)`; assert `trie.Root() ==
   prevStateRoot` (the seed is faithful). Then `Update`/`Delete` each payload write; read
   `Root()`; require `== b.StateRoot`.

The seed reconstruction (step 2) is the only hand-written digest arithmetic. It is the SAME
fold `verifyProofWithUpdates` already runs and is PINNED byte-exact (below). Everything after
the seed is the library's own surgery.

### Why the seed is complete for interacting paths

When two changed keys share a prefix, their proofs' sidenodes AGREE on the shared frontier
(they are the same committed nodes) and DIVERGE below it — each proof carries the OTHER key's
subtree digest as a sidenode at the divergence level. Reconstructing both paths into one seed
map yields, at every shared level, the same inner-node preimage from both (idempotent Set), and
at the divergence level two inner nodes whose children include each other's on-path digest. The
library then descends both and performs the displacement/promotion itself. The seed is complete
because the box DERIVED the changed-key set from the payload (cert sub-Q1): there is no
un-witnessed key whose subtree the seed omits, because an un-witnessed change makes the honest
`Root()` differ from the forged `b.StateRoot` and the final equality fails.

## The structural cases the fold MUST handle byte-exact (the pin cross-product)

Ground truth for every case: `statehash.Root(postLeaves)` where `postLeaves` is the honest
pre-state leaf SET with the payload writes applied. The fold's computed root MUST equal it.

1. **Overwrite** — a changed key already present, new value. (In E/R the only overwrite is a
   byRoot/spent/revoked key re-set to Present, a no-op value-wise; included for completeness and
   because later classes overwrite value-carrying leaves.)
2. **Add, disjoint** — a new key whose nearest resident leaf diverges high; creates an inner
   node near the root.
3. **Add, displacing** — a new key sharing a long prefix with a resident leaf; creates an inner
   node deep, at a data-dependent `prefixLen`, wrapping the two leaves. This is Fact 3, the
   structural change the naive fold got wrong.
4. **Delete, leaf-sibling promote** — deleting a key whose sibling is a leaf: the sibling
   promotes up (`smt.go:308-309`).
5. **Delete, extension-sibling absorb** — deleting a key whose sibling is an extension node:
   the extension absorbs the freed level via `pathBounds[0]--` (`smt.go:310-315`). The case the
   PE flagged as missing from a two-rule discriminator.
6. **Delete, inner-sibling placeholder-fold** — deleting a key whose sibling is an inner node:
   no promotion, the inner stays (`smt.go:305-317`, the no-case).
7. **Shared-prefix / interacting changed paths** — two+ changed keys under one subtree
   (add+add, add+delete, delete+delete), reconciled in one fold.
8. **Extension present / absent** — every case run both with and without a compressed extension
   span on the changed path (placeholder sidenodes vs real).

Because the fold delegates surgery to the library, cases 2–6 are handled by `Update`/`Delete`
themselves; the pin proves the SEED reconstruction feeds them a faithful partial trie so their
output matches full-state `Root()`. The pin is run over a RANDOMIZED cross-product of these
cases at scale (target: 200+ trials, and a large 5000+ trial for the headline 100%-correct
claim), each trial building a random pre-state, a random E/R payload spanning the case mix, and
asserting fold-root == `Root(postLeaves)`.

### Per-case ablation (red-before-green)

For each structural rule, break it and watch the pin go red:
- seed inner-node child order swapped (path-bit ordering) → red.
- placeholder sidenode mis-mapped (extension span) → red on the extension cases.
- displaced `NonMembershipLeafData` leaf omitted from the seed → red on add-displacing.
- delete routed as no-op (skip `Delete`) → red on all delete cases.
Each ablation is a test that asserts the pin FAILS with the rule broken and PASSES restored.

## Decomposition — is this one reviewable unit?

Two units, both in this increment:

- **Unit A — the fold primitive** (`foldChangedPaths`): given `prevStateRoot`, a set of changed
  keys each with a pre-state proof + old value, and the write ops (set value / delete), compute
  the post-root by seed-reconstruct + library replay. Pinned byte-exact standalone against
  `Root()` over the structural cross-product. This is the soundness-critical core.
- **Unit B — the E/R hybrid recompute** (`RecomputeStateRootEntriesRevocationsOpayload`):
  derive the E/R write-set from the block payload, drive Unit A, re-anchor the scope gate on the
  dueBucket digest, keep never-Accept. Ablated on tampered StateRoot / omitted / forged payload
  write / forged / omitted changed-leaf proof / out-of-scope class.

Unit A is the pinned primitive; Unit B composes it. Both land here because Unit A has no meaning
without a consumer to prove O(payload) and the scope-gate re-anchor, which the task requires
measured in this increment.

## The scope-gate re-anchor (or O(payload) is false)

The superseded whole-state P1-a scans the whole `bondRegHeight` map to detect a firing TTL
expiry. That is O(whole-state) and would force a whole-state witness even under the fold. The
O(payload) box re-anchors it on the `dueBucket[h]` accelerator: a TTL expiry fires at height `h`
iff `dueBucket[uint64BE(h)]` is OCCUPIED (`chain.go:3274`, `readset_v5.go:605`). The box tests
this with ONE non-membership witness of `dueBucket[uint64BE(b.Height)]` against `prevStateRoot`:
`ProvenAbsent` ⇒ no expiry ⇒ E/R-only is safe; anything else (present, or no witness) ⇒ stall.
O(1), no whole-map scan. The boundary/BondReg/Slash/seen clauses are payload-visible or
config-derived and stay O(1) as they already are.

## Cost proof (the whole point)

Measure the same small E/R payload block against a 100-entry and a 10,000-entry pre-state.
Report the witness leaf count and the fold node-op count for both. The hybrid touches only:
payload-derived changed keys (× ~256-deep proof) + one dueBucket non-membership + O(1) gate
reads. It MUST be flat across the two state sizes (payload + O(log N)), NOT scale with total
state. The superseded whole-state P1-a witnessed all 100 vs all 10,000 leaves; the flat result
is the O(payload) win.

## Soundness posture

- Never-Accept preserved (R-scope): the box returns nil ("recompute agrees") only as a
  MECHANISM result the never-Accept scaffold consumes; it does NOT flip WitnessValidateV5 to
  Accept. Out-of-scope classes stall.
- R3 execution-derived drift guard: compare the box's derived write-set + fold root against the
  real `apply()` + `StateRootForVersion(5)` on a corpus, ablated red on an omitted / extra /
  mis-valued write.
- No `apply()` / consensus change.
