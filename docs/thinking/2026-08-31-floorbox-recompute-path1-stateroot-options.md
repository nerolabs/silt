# Path-1 state-root recompute for the trustless floor box — PACE

Date: 2026-08-31
Seat: Builder
Branch: `feat/floorbox-recompute-path1-stateroot` off `origin/main` (`eff6c51`)
Status: **PACE + first sub-increment. STOP-after-first: the full recompute is far larger
than one blind-reviewable unit. This doc reports the decomposition for planner sequencing.**

## The task and the crux

Reproduce `validateEra3Roots` in the root-only floor box: recompute the POST-STATE
`StateRoot` from the committed pre-state + the block payload + the era-4 accelerators
(the `dueBucket[h]` non-membership digest for TTL, the F1 digest roots for whole-set
commitments), and compare it to the block's committed `StateRoot`. This is Path-1, the
largest remaining recompute piece; it consumes `qualifiedRoot` for the boundary freeze.

A full node recomputes the root by `cloneForDryRun()` → `apply(b)` →
`StateRootForVersion(5)`. A ROOT-ONLY box cannot replay `apply()`: `apply()` scans whole
O(registry) maps (the `bondRegHeight` TTL sweep at `chain.go:3272`; the three
`rotateEpoch` frozen-set tallies at `chain.go:3442/3465/3489`; the objective-maturity
`bonded` fold). The box must instead reproduce the WITNESSABLE recompute: apply the
block's PAYLOAD transitions to the witnessed pre-state reads, use the accelerators to
reconstruct the CHANGED post-state leaves, and compare to the committed `StateRoot`.

## How the committed root is formed (verified against source)

`StateRootForVersion(5)` → `stateRootLeavesV5()` (`statehash.go:223`) emits, for a v5
block, the 18 era-3 leaves (`stateRootLeaves`, `statehash.go:149`) PLUS the maintenance
spine: `qualified` (one value leaf per member), `dueBucket` (one MTH leaf per occupied
due-height), `epochStart`, `era4LockedIn`, `era4Height`, and the five F1 whole-set digest
roots (`bondedRoot`/`epochSetRoot`/`qualifiedRoot`/`slashedRoot`/`validatorsSeenRoot`).
`statehash.Root(leaves)` folds the whole leaf SET into ONE SMT root, order-independent.

The recompute's job: from the committed pre-state (witnessed) + the payload, produce the
EXACT post-state leaf set, then `statehash.Root()` it and compare to `b.StateRoot`. Because
the SMT root is a pure function of the leaf SET, the box does not need `apply()`'s
control flow — it needs the post-state VALUE of every leaf that changed, plus the
guarantee that no OTHER leaf changed (completeness).

## Enumeration of the post-state committed leaves and how each is derived

Grouped by `apply()` transition class (`chain.go:3185-3318`). For each: the leaves it
writes, and how a root-only box derives the post-state value trustlessly (per-member proof
C-1 where a value matters, own config C-6 for screens, digest-root reconstruction for
whole-set completeness).

### Class E — entries (`chain.go:3187-3192`)
- Writes: `byRoot[e.Root] = e` (one leaf/entry); `spent[serial] = true` (one leaf/spent
  serial) when `e.Token != nil`.
- Derivation: PAYLOAD-DRIVEN. The entries are IN THE BLOCK; the box reconstructs each
  post-state leaf directly from `b.Entries` (key + value are payload). No whole-map scan.
  Pre-state witness needed only to confirm a `byRoot`/`spent` key is NOT already present
  when the value encoding is a set-marker (idempotent overwrite; value is `Present`
  either way, so a prior-present leaf is byte-identical). Completeness: the set of CHANGED
  `byRoot`/`spent` keys is exactly the block's entries — no hidden writer.

### Class R — revocations / un-revocations (`chain.go:3193-3203`)
- Writes: `revoked[r] = true` (revoke) or `delete(revoked, r)` (unrevoke); each ALSO
  appends to `revLog` (LogRoot, NOT the StateRoot — out of scope for the state root).
- Derivation: PAYLOAD-DRIVEN. `b.Revocations` / `b.Unrevocations` are the exact key set.
  A revoke adds a `revoked` leaf (value `Present`); an unrevoke removes one (needs a
  pre-state proof that the leaf WAS present, to know a leaf is removed → the digest/leaf
  set shrinks). The changed-key set is the payload; completeness holds.

### Class B — bond registrations (`chain.go:3204-3265`) — the HARD class
- Writes, per canonical winner (`canonicalBondRegs`, ordering-canonicalized so the leaf
  set is order-free): `bondRootOwner[root]`, `bondRootProven[root]`, `bonded[id]`,
  `bondRegHeight[id]`, `regVersion[id]`, `bondDomain[id]`, plus `dueBucket` moves
  (`dueBucketMoveOnReg`) and `qualified[id]` maintenance (`qualifiedMaintain`).
  A DISPLACEMENT (proof-beats-declaration, `chain.go:3239-3250`) ALSO writes
  `delete(bonded, oldOwner)` + `qualifiedMaintain(oldOwner)`.
- Derivation: mostly payload-driven (the winner's fields come from the reg), BUT the box
  must reproduce the SCREENS trustlessly:
  - `r.Size < c.cfg.MinBondBytes` skip → own config (C-6).
  - `c.slashed[id]` skip → per-member `slashed[id]` proof (C-1).
  - the ownership/displacement branch reads pre-state `bondRootOwner[r.Root]` +
    `bondRootProven[r.Root]` → per-key proofs.
  - `dueBucketMoveOnReg` reads the PRIOR `bondRegHeight[id]` → per-key proof; recomputes
    the affected `dueBucket` leaves' MTH from the witnessed bucket id-lists.
  - `qualifiedMaintain(id)` recomputes `qualified[id]` from post-state `bonded[id]`,
    `slashed[id]`, own MinBond — reproducible per-member.
  This class is the single largest reviewable unit: the canonicalize fold, the
  proof-beats-declaration displacement, and the coupled dueBucket + qualified maintenance
  are each their own sub-proof.

### Class T — TTL sweep (`chain.go:3271-3281`) — the whole-map-scan class
- Writes: for every `id` with `b.Height - bondRegHeight[id] > ttl`:
  `delete(bonded, id)`, `delete(bondRegHeight, id)`, `delete(regVersion, id)`,
  `dueBucketRemove(id, regH+ttl+1)`, `qualifiedMaintain(id)`.
- Derivation: this is where a root-only box CANNOT scan `bondRegHeight`. The certified
  accelerator (CRUX cert R-crux; `readset_v5.go` occupied/empty bucket handling) is:
  the EXPIRING set at height `h` is exactly the members of `dueBucket[expiry]` where
  `expiry = h` (`regH+ttl+1 == h` ⟺ `h-regH == ttl+1 > ttl`). The box:
  - proves `dueBucket[h]` present with committed digest `D_h`, OR non-membership (empty
    bucket ⟹ nothing expires, the one-proof shortcut resting on the accelerator invariant
    `dueBucket[h] absent ⟹ no id expires at h` — model-checked, R6);
  - reconstructs `dueBucketMTH(witnessed id-list)` and requires `== D_h` (completeness);
  - for each expiring id, derives the delete-writes; recomputes the affected `qualified`
    and `dueBucket` post-state leaves.
  This is a self-contained sub-increment built on the ALREADY-CERTIFIED dueBucket digest
  reconstruction.

### Class S — slashes (`chain.go:3285-3290`)
- Writes: `slashed[culprit] = true`, `delete(bonded, culprit)`, `qualifiedMaintain`.
- Derivation: PAYLOAD-DRIVEN (`b.Slashes` carries the culprits). Adds a `slashed` leaf,
  removes the `bonded` leaf (pre-state proof it was present), recomputes `qualified`.

### Class A — attestation tracking (`chain.go:3293-3298`)
- Writes: `validatorsSeen[id] = true` for each att id ≠ proposer that `attesterQualified`.
- Derivation: PAYLOAD-DRIVEN (`b.Atts` carries the attester set — the CRUX cert names this
  as NOT a completeness hazard: the attester set is in the block). `attesterQualified`
  reads per-member `bonded`/`slashed`/epochSet membership → per-member proofs.

### Class M — maturity latch (`chain.go:3303-3305`)
- Writes: `everMature = true` iff `!everMature && Mature()`.
- Derivation: `Mature()` in objective mode folds the WHOLE `bonded` map (C2Metric). This is
  the THIRD whole-map read (CRUX cert R-boundary, third face) — anchored on `bondedRoot`.
  Increments 2/3 (`floorbox_recompute_maturity_v5.go`) already reproduce the maturity fold;
  this class REUSES that recompute to derive the post-state `everMature` leaf.

### Class P — epoch rotation (`rotateEpoch`, `chain.go:3393-3500`) — the boundary class
- Fires iff `epochsEnabled() && b.Height % EpochBlocks == 0`. Writes:
  - `epochStart = h` (always at a boundary).
  - if `everMature` (post-latch): `matureEpoch = true`; `epochSet = clone(qualified)`
    (normal) or `liveQualifiedSet()` (recovery re-base); then the THREE lock-in tallies
    (`gateLockedIn`/`gateHeight`, `era3LockedIn`/`era3Height`, `era4LockedIn`/`era4Height`),
    each a `3*ready > 2*total` super-quorum over the WHOLE frozen `set` with `ready` gated
    on `regVersion[id] >= threshold`.
- Derivation: THE FREEZE consumes `qualifiedRoot`. `epochSet` post-state = the frozen
  `qualified` set. The box:
  - reconstructs the complete `qualified` membership from the witnessed id-list against the
    committed `qualifiedRoot` digest (this is the `qualifiedRoot` READ this task lands);
  - per-member `qualified[id]` weight proofs (C-1) give the frozen weights;
  - writes the post-state `epochSet` leaves = the frozen (id, weight) pairs, and recomputes
    the post-state `epochSetRoot`.
  - the three lock-in tallies fold the frozen set's weight with a `regVersion` gate → each
    needs per-member `regVersion` proofs (C-1) + own EpochBlocks/threshold (C-6), producing
    the post-state `gate/era3/era4 LockedIn/Height` scalar leaves.
  This is the largest post-Class-B sub-increment and the natural HOME for consuming
  `qualifiedRoot`.

### Scalars carried through unchanged
`gateLockedIn/Height`, `era3LockedIn/Height`, `matureEpoch`, `epochStart`,
`era4LockedIn/Height` are single leaves; on a NON-boundary block they are unchanged from
pre-state (carried through by proof). Only Class P mutates them.

## The decomposition — sub-increments for planner sequencing

The full `validateEra3Roots` recompute is **not** one blind-reviewable unit. It spans 8
transition classes, three of which (B, T, P) each carry their own multi-part sub-proof
(canonicalize/displace; dueBucket-anchored TTL completeness; qualifiedRoot-anchored freeze
+ three tallies). A single increment covering all of them would be exactly the "giant
increment" the task forbids. Proposed sequence:

- **P1-a (THIS increment): the ROOT-EQUALITY MECHANISM on the pure set-write classes E + R**
  — entries (`byRoot`, `spent`) and revocations/un-revocations (`revoked`). These are the
  transitions whose changed-key set is EXACTLY the block payload AND which apply NO membership
  screen (no `slashed`/`bonded`/`epochSet`/MinBond read), so the post-state leaf value is a
  pure function of the payload. P1-a establishes the whole recompute SPINE end-to-end:
  1. witness the COMPLETE pre-state leaf set; require `Root(preLeaves) == prevStateRoot` (the
     PREVIOUS block's committed StateRoot is the pre-state completeness anchor — one omitted or
     injected pre-leaf changes the reconstructed root ⇒ stall);
  2. apply the E + R payload transitions to the pre-state leaf set, deriving the changed leaves;
  3. require `Root(postLeaves) == b.StateRoot`.
  It STALLS (never-Accepts) on any block carrying a bond reg, a slash, an att that would write
  `validatorsSeen`, a TTL expiry at this height, or a boundary rotation — those are the
  screen-bearing / whole-map classes deferred to P1-b..e. This is the minimal unit that proves
  reconstruct→hash→compare AND the pre-state-root completeness closure, the load-bearing
  novelty every later class reuses. The pre-state-root anchor is why P1-a needs NO per-keyspace
  digest read: the full pre-state root already commits the entire pre-state leaf set.
- **P1-b: Class S + A (slashes, attestation tracking)** — the payload-driven writes that DO
  carry a membership screen: `slashed`+`bonded`-delete+`qualified` recompute (S), and the
  `attesterQualified` screen behind `validatorsSeen` (A). Reuses the P1-a spine; adds per-member
  `bonded`/`slashed`/`epochSet` proofs (C-1) + own MinBond (C-6) for the screens.
- **P1-c: Class T (TTL sweep)** — dueBucket-anchored expiring-set completeness + the
  delete-writes + affected qualified/dueBucket leaf recompute. Builds on the certified
  dueBucket reconstruction.
- **P1-d: Class B (bond regs)** — canonicalize fold, proof-beats-declaration displacement,
  coupled dueBucket + qualified maintenance.
- **P1-e: Class P (epoch rotation)** — CONSUMES `qualifiedRoot` for the freeze; the three
  lock-in tallies. This is where `qualifiedRoot`'s `isDigestRootLeaf` exclusion + red-on-drop
  ablation are removed.
- **P1-f: Class M whole-`bonded` maturity latch** — reuses the increment-2/3 maturity
  recompute to derive the post-state `everMature` leaf (may fold into P1-d if small).

### Note on consuming `qualifiedRoot` this increment

The task offers consuming `qualifiedRoot` "if in scope". It is NOT in scope for P1-a: the
boundary freeze that reads `qualified` whole-set lives in Class P (P1-d). Forcing it into
the non-boundary increment would either (a) require building Class P (violating the
one-reviewable-unit bound) or (b) read `qualifiedRoot` with no transition that consumes it
(a decoration read — a green check with no demonstrated red, the session-7 scar). So P1-a
does NOT touch `qualifiedRoot`'s exclusion/ablation; P1-d does. This is the simplest thing
the evidence justifies: land the root-equality mechanism on the tractable classes first.

## Invariants / gates

Preserves I1–I5: the recompute is a pure function of (committed pre-state, block) — every
honest replica computes the same verdict (I5). It ADDS a root-only path; it changes NO
`apply()` rule and does NOT flip `WitnessValidateV5` to Accept (the box still never-Accepts).
No consensus-rule change (STOP-and-report if one is needed). Research gate: this builds
on the CRUX cert's certified direction (digest reconstruction + per-member proofs); it does
not decide a new consensus rule.

## Ablations (C-5, red-before-green) for P1-a

- forged per-member value (a `bonded`/`qualified` weight the committed root does not commit)
  ⇒ the member's inclusion proof fails ⇒ REJECT (stall).
- omitted / injected changed leaf (a payload write dropped, or an extra leaf added) ⇒ the
  reconstructed post-state leaf set differs ⇒ recomputed root ≠ committed ⇒ REJECT.
- a tampered committed (carried-through) leaf ⇒ its per-key proof fails, or if forced into
  the reconstruction the recomputed root diverges ⇒ REJECT.
