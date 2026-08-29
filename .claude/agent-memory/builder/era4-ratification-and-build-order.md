---
name: era4-ratification-and-build-order
description: era-4 witnessable-transitions RATIFIED (params locked) + the locked 4a→4b→4c→4d build order and why the maintenance spine lands BEFORE the predicate.
metadata:
  type: project
---

era-4 witnessable state transitions (Option B) RATIFIED 2026-08-29 (PR #635, docs-only).

**Why:** two `apply()` scans (TTL sweep `chain.go:3005-3013`, epoch-rotation
`liveQualifiedSet` over all `bonded` `chain.go:1198`) prove whole-map claims a floor box
can't witness. era-4 makes both witnessable. Cert: RECERT2 CERTIFIED-WITH-CONDITIONS
(`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`).

**Locked parameters (Andrew ratified the format veto-gate):**
- `BlockVersion = 5`, `versionSupported <= 5`, PREDICATE-FIRST (widen the ceiling in the
  SAME release as the v5 predicate — era-3 widened a release ahead and left an
  accept-a-wrong-root window; do NOT repeat).
- Three new committed tags: `tagDueBucket` (TTL), `tagQualified` + `tagEpochStart`
  (rotation); `tagEpochSet` RETAINED (frozen era-3 shape).
- TWO-keyspace: frozen materialized `epochSet` (sizes the quorum, mid-epoch immutable) +
  a live `qualified` accelerator (boundary-copy source, NOT a pointer target). RECERT2 Q1
  REFUTED the single-shared-keyspace direction.
- `RegCap = 256` fresh-reg validity rule. See [[era4-regcap-299-gate]].

**The locked build order (PACE:
`docs/thinking/2026-08-29-era4-build-decomposition-options.md`):**
4a widen-version+tags-defined-not-committed → 4b maintenance spine (qualified + due-bucket
maps, the five hooks, drift-guards) → 4c v5 predicate + RegCap + the `<=5` widen
(predicate-first) → 4d height-gated activation + mint-flip.

**The key ordering call — spine lands WITH the predicate but SEQUENCED BEFORE it.**
Committing new keyspaces changes the state root ONLY for v5 blocks (leaf marshaller
v5-gates the new leaves). No block is v5 until 4d, so 4b is INERT on the live v4 chain
while fully exercised by the model-check corpus + byte-identical replay. Landing the spine
fully BEFORE (committed for v4) breaks the era-3 freeze; landing it AFTER leaves the
predicate checking an incomplete root.

**How to apply:** the FIRST code increment is 4a (schema + classification only, no
behavior, no version-ceiling widen). Gate it on `TestStateFieldsAreClassified` reddening.
Then 4b is the heaviest PR (all drift-guards ablate here). Do NOT fold the version widen
into 4a — hold it to 4c (predicate-first).

**The five `qualified` hooks (verified @ 0984db4):** 2989 (delete displaced `owner`),
2995 (write `id`), 3008 (TTL delete), 3019 (slash mark), 3020 (slash evict). The **2989
hook MUST redden specifically** in the drift-guard: it deletes a DIFFERENT key (`owner`)
than 2995 writes (`id`), so a mirror-only-2995 maintenance passes a naive guard. Rotate
LAST at `chain.go:3046`. Intra-block order: bonds → TTL → slashes → rotate-LAST.

**Owed build-time ablations (inject-the-defect, each MUST go red):** qualified drift-guard
per site (2989 specifically); T-3 due-bucket dual-source (ablate on missed renew
old-bucket delete); T-3 byte-identical StateRoot replay vs era-3 (corpus: renew-reset,
ttl==0, slash-before-due); Q5 recovery-branch agreement (materialized qualified vs
recomputed `liveQualifiedSet()` at `chain.go:1243-1248`); the two completeness guards
(`TestDryRunCloneCopiesEveryAppliedField`, `TestStateFieldsAreClassified`).

**Scoped OUT of era-4-minimum (Andrew's call):** the recovery-boundary direction
(`effectiveEpochSet` at `LivenessRecoveryHeight`). era-4 keeps recovery reading
`liveQualifiedSet()`; the Q5 assertion only proves agreement, does not witness the re-base.
