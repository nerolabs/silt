# Lane-1 Part A — the validity-read guard gap (spent/revoked)

Date: 2026-08-30
Branch: `feat/lane1-partA-readset-producer` (PR #656)
Scope: TEST-ONLY (guard ground-truth + corpus). No consensus/validity/statehash
production change. The producer's certified read-set is UNCHANGED — `spent`/`revoked`
stay emitted.

## The gap (found by blind review, confirmed by probe)

The execution-derived completeness guard
(`TestWitnessReadSetV5ExecutionDerivedGuard`) derives its ground truth from two
sources, BOTH `apply()`-shaped:

- Source 1 — write-diff: leaves whose value changes across `apply(b)`.
- Source 2 — leaf-sensitivity perturbation: perturb a leaf, re-run `apply(b)`, see
  if another leaf's post-value changes.

But the read-set a floor box needs is `validity ∪ apply-recompute`-shaped. Two
committed leaves are read ONLY in the validity predicate, never in `apply()`:

- `spent[serial]` — the double-spend gate, `ValidateEntry` (`chain.go:2617`).
- `revoked[root]` — the un-revocation gate, `validateTakedowns` (`chain.go:2643`).

Perturbing `spent`/`revoked` changes nothing in the apply-recompute output, so
neither source lists them. **Probe evidence (throwaway `TestKeyspaceProbeBaseline`):
dropping each of the 23 committed keyspaces from the producer one at a time reddens
21; `spent` and `revoked` stay GREEN.** That is the exact "a read no guard catches"
failure this guard exists to kill: the producer emits `spent`/`revoked` correctly
today, but nothing execution-derived proves it stays complete.

## The fix — a third, validity-read ground-truth source

Add Source 3 (validity-read perturbation), execution-derived like the others:

For each committed leaf L present pre-apply, perturb L on a fresh clone and run the
REAL validity read-predicates against the perturbed clone vs the unperturbed clone.
If the accept/reject VERDICT flips, L is a validity read.

The validity driver runs the production predicates that perform the reads —
`validateTakedowns(b)` (reads `revoked`, `byRoot`) and, per entry, `ValidateEntry(e)`
(reads `spent`, `byRoot`). It EXCLUDES `validateEra3Roots` (the root recompute).

### Why exclude the root predicate

`validateEra3Roots` recomputes the post-apply root and compares it to the block's
committed `StateRoot`. Perturbing ANY committed leaf changes the recomputed root, so
including the root predicate would flip the verdict for all 23 keyspaces — every leaf
a false positive, the source useless. The root-recompute channel is ALREADY Source 1
(the write-diff IS the set of leaves the root commits). This exclusion mirrors Source
2's exclusion of the perturbed leaf's own key: we isolate the channel the other
sources miss. Source 3 isolates the PURE VALIDITY-GATE reads (`spent`/`revoked`) that
never touch the apply-recompute.

Over-witnessing stays sound; the binding direction is unchanged (producer ⊇
ground-truth). Source 3 only ADDS reads to the ground truth, never removes.

## The corpus — exercising the validity reads

The reads only fire when the corpus contains the transitions:

- `revoked` read (`validateTakedowns:2643`): a block must UN-REVOKE a currently
  revoked root. So the corpus publishes a root, revokes it, then unrevokes it.
- `spent` read (`ValidateEntry:2617`): gated on `tokenQuorum > 0`. The read fires
  when a token-carrying entry is validated (spent or fresh — the gate reads the leaf
  either way). This needs the token-issuer machinery (`newOrderIssuers`/`RequireTokens`).

The maintenance corpus (`buildV5ReadSetCorpus`) runs `tokenQuorum=0` (no tokens on
its entries), so forcing tokens through it would require minting a token for EVERY
entry and reworking its qualification world. Instead, add a DEDICATED validity corpus
(`buildV5ValidityReadCorpus`) that carries a token entry (drives `spent`) and a
revoke→unrevoke pair (drives `revoked`). The guard and the 23-keyspace check run over
BOTH corpora; a keyspace reddens if it reddens in ANY corpus block. This keeps the
maintenance corpus's boundedness/vacuity properties untouched and adds the two
validity reads where they naturally live.

The boundedness test (`TestWitnessReadSetV5BoundedNotRegistrySized`) has its OWN
`build()` and is unaffected.

## The definitive check

`TestWitnessReadSetV5AllKeyspacesRedOnDrop`: drop EACH of the 23 committed keyspaces
from the producer, one at a time, over the union of both corpora; assert the guard
goes RED for every one — all 23, including `spent` and `revoked` now red. This is the
completeness proof (inject the defect, watch it go red — no green check ships
undemonstrated).

## Kept

The existing regression ablations (attestation-loop / slash-path / boundedness) pass
unchanged. No assertion weakened. The corpus is not over-narrowed — the maintenance
corpus is unchanged; the validity corpus is additive.
