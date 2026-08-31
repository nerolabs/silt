# Floor-box O(payload)+O(registry) state-root recompute — classes B (bond regs) and T (TTL sweep)

Date: 2026-08-31
Author: Builder
Status: PACE — options + decision, before code. Ships in the same PR as the build.

## The task

Extend the era-4 (v5) trustless floor-box state-root recompute
(`RecomputeStateRootEntriesRevocations`) from class S (slashes, PR #675, `d42fee8`) to
classes **B (bond registrations)** and **T (TTL sweep)**, reusing the merged
changed-digest write-set primitive. The box STILL never-Accepts (R-scope); it reproduces
`validateEra3Roots`' `StateRoot == recompute` verdict root-only and stalls loud on
everything else.

Cert:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md`
— B and T are **CERTIFIED-IN-DIRECTION**. B carries the R-B-displacement residual
(displacement is a screen the delta must reproduce exactly; fold-caught, liveness-only). T
inherits the CRUX dueBucket reconstruction.

## Measurement — the real leaf-diff (evidence, not guessed)

Ran a leaf-diff of the real `apply()` + `stateRootLeavesV5()` before/after each block class
(`TestMeasureBondRegLeafDiff`, `TestMeasureTTLExpiryLeafDiff`). The exact write-sets:

### B — FRESH bond reg (new id, new root)
```
ADD    bondDomain||id      = EncodeUint64(domain)
ADD    bondRegHeight||id   = EncodeUint64(height)
ADD    bondRootOwner||Root = EncodeID(id)
ADD    bondRootProven||Root= EncodeBool(true)      (only if proven = height>0)
ADD    bonded||id          = EncodeInt64(size)
ADD    qualified||id       = EncodeInt64(size)     (iff size >= MinBond && !slashed)
ADD    regVersion||id      = EncodeUint8(version)
ADD/CHG dueBucket||D       = dueBucketMTH(members) (D = height + ttl + 1)
CHANGE bondedRoot                                   (whole-set digest)
CHANGE qualifiedRoot                                (whole-set digest, iff qualified changed)
```

### B — RENEW (same id, resize/version bump)
```
CHANGE bonded||id, bondRegHeight||id, regVersion||id, qualified||id
ADD    bondRootProven||Root (genesis-declared → now proven)
CHANGE dueBucket||oldD  (id removed)  +  ADD/CHANGE dueBucket||newD (id inserted)
```
NOTE: on a same-id renew the MEMBERSHIP set of bonded/qualified is UNCHANGED, so bondedRoot
and qualifiedRoot do NOT change. The digest reconstruction must reflect that (no touched
digest for a pure renew).

### B — DISPLACEMENT (proven reg of a genesis-squatted root)
```
(all the FRESH per-member leaves for the new owner) +
DELETE bonded||oldOwner
DELETE qualified||oldOwner        (iff oldOwner was qualified)
CHANGE bondRootOwner||Root        (old owner → new owner)
CHANGE bondedRoot, qualifiedRoot
```
The displaced `oldOwner` is NOT in the payload — it is read from `bondRootOwner[Root]`
(a committed per-key leaf). This is the R-B-displacement residual: the delta must derive the
displaced owner from the committed pre-state, exactly reproducing apply()'s displacement
branch (`chain.go:3239-3265`).

### T — TTL sweep (per expired member)
```
DELETE bonded||id
DELETE bondRegHeight||id
DELETE regVersion||id
DELETE qualified||id              (iff was qualified)
DELETE dueBucket||h               (the expiring bucket at h, ALL its members)
CHANGE bondedRoot, qualifiedRoot
```
NOTE: `bondDomain||id` is NOT deleted on expiry (apply chain.go:3275-3277 deletes bonded /
bondRegHeight / regVersion, not bondDomain). Confirmed by the diff — no bondDomain DELETE.
The expired set = the members of `dueBucket[h]` (the O(1)/O(bucket) accelerator witness), NOT
a whole `bondRegHeight` scan — that is the whole point of era-4.

## The one new primitive class this needs: the dueBucket MTH leaf

Classes B and T change `dueBucket` leaves. Each `dueBucket[D]` leaf value is
`dueBucketMTH(members)` — an RFC-6962 MTH over a bucket's id-set, the SAME closure as the
whole-set digest roots (`nodeSetMTH`), scoped to one due-height. So the changed-digest
primitive from #675 applies directly: witness the bucket's pre-set id-list against
prevStateRoot, apply the payload-derived membership delta (insert/delete), fold the new
`dueBucketMTH(post-members)` as the changed leaf. It is `stateRootDigestOps` generalized from
a fixed scalar key `Key(tag, nil)` to a keyed leaf `Key(tagDueBucket, uint64BE(D))`.

## Separability

B and T are cleanly separable and each reuses the primitive. They share the digest-root
reconstruction (bondedRoot/qualifiedRoot) and the dueBucket MTH reconstruction, but their
membership deltas are independent:

- **T** derives its delta from ONE accelerator witness: the members of `dueBucket[h]`. It is
  the tractable one — no displacement, no canonicalization, a pure delete-set.
- **B** derives its delta from the payload `b.BondRegs` + `canonicalBondRegs` fold +
  per-root displacement screens + `dueBucketMoveOnReg` (old bucket delete + new bucket
  insert). It is materially harder (the R-B-displacement residual).

Decision: **ship both in one PR**, because they share the dueBucket MTH primitive and the
scope-gate widening, and T is small enough that splitting would be churn. B and T are wired
as two separate write-set/digest-op derivations behind the same `RecomputeStateRootEntriesRevocations`
entry, each guarded by its own scope clause. A/P/M stay out-of-scope-stalling.

## Cost — HONEST (R-cost-wholeset, R-membership)

NOT O(payload). Reconstructing bondedRoot/qualifiedRoot needs the WHOLE post-set id-list
(MTH is a whole-list fold, no incremental update). So B and T are
`O(payload) + O(|bonded|) + O(|qualified|)` ≈ **O(registry) per touched whole-set digest**,
plus `O(|bucket|)` per touched dueBucket leaf. It rides directly on R-membership (no
code-enforced bound on total bonded/qualified membership — OPEN, load-bearing for the #657
accept-flip). Box-fits at RegCap-era populations (kilobytes); degrades to megabytes-per-block
at 100k-member populations. This is stated in the file doc-comment, same as class S.

## Gates honored

- Box never-Accepts (R-scope): this reproduces the root-equality verdict and stalls; it does
  NOT flip #657 WitnessValidateV5 to Accept.
- Reuse the primitive: `stateRootDigestOps`/`digestFoldOp`/`anchoredPreSet` extended, not
  re-invented; dueBucket leaves fold through the same `FoldChangedPaths`.
- No `apply()`/consensus change.
- Height-contiguity invariant (T): the dueBucket gate leans on it — the box reads
  `dueBucket[h]` as the expired set at exactly `b.Height`, honoring the accelerator's
  contract (chain.go:3271-3281 sweeps at h iff `h - regH > ttl`, i.e. the bucket keyed at
  `regH + ttl + 1 == h`).
- R3: every derivation checked against real `apply()` + `StateRootForVersion(5)`, ablated RED.
