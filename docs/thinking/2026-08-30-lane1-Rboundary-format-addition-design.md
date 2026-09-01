# Lane-1 R-boundary: the additive v5 digest-root format for the trustless floor box

Date: 2026-08-30
Seat: Builder (design only — NO production code in this increment)
Status: DESIGN for research certification + owner ratification. This is a GATED v5
committed-format change (consensus-rule). The owner has RATIFIED the recompute direction
(the `dueBucket` MTH-reconstruction close); this doc specifies the NEXT step — closing
R-boundary, the frozen-set and maturity-latch whole-map completeness faces.

## BLUF

The v5 witnessable recompute reads three committed keyspaces as WHOLE SETS — the whole
map, not a producer-chosen subset — and each commits one leaf PER MEMBER with NO aggregate
digest. A root-only floor box can prove any member present but cannot prove it holds ALL
members. An omitted frozen member flips the `3*ready > 2*total` activation tally (or the
maturity-latch coefficient) → WRONG-ACCEPT of a forged boundary. `pokt-network/smt@v1.0.0`
exposes only a whole-trie `Root()`, no per-keyspace sub-root, so reconstruction has nothing
to bind against — unlike `dueBucket`, whose committed leaf value already IS an MTH digest.

**The fix (additive, mirrors `dueBucket`):** commit new v5 MTH DIGEST-ROOT leaves — one per
whole-set keyspace — over each keyspace's canonical sorted id-list. The box recomputes each
MTH over the witnessed member set and compares to the SMT-proven committed digest.
Completeness then closes by collision-resistance, exactly as it does for `dueBucket`.

**The exhaustive enumeration — three keyspaces need a digest root:**

| Keyspace | New digest leaf | Whole-set read site | Why it is a completeness hazard |
|---|---|---|---|
| `qualified` | `qualifiedRoot` | `readSetBoundaryDelta` `for id := range c.qualified` (`readset_v5.go:526`) → the freeze source frozen into `set` and tallied `3*ready>2*total` (`chain.go:3442/3465/3489`) | An omitted `qualified` member changes `total`/`ready` → flips era-gate / era-3 / era-4 lock-in |
| `epochSet` | `epochSetRoot` | `readSetBoundaryDelta` `for id := range c.epochSet` (`readset_v5.go:535`) → prior frozen set, freeze write-target removals; and the mature-objective atts qualification-set (`attesterQualifiedAt:1297`) | An omitted prior-`epochSet` member hides a freeze removal → wrong post-freeze `epochSet` leaves; omitted membership mis-qualifies an attester |
| `validatorsSeen` | `validatorsSeenRoot` | `readSetMaturityLatch` `for id := range c.validatorsSeen` (`readset_v5.go:408` legacy, `:419` objective) → `C2Metric` ranges `validatorsSeen` (`chain.go:2305`) on the maturity latch | An omitted `validatorsSeen` member changes the maturity coefficient → forges a permanent one-way `everMature` latch |

No other committed v5 keyspace is read whole-set. `dueBucket` is already digest-committed and
closes by reconstruction (the ratified TTL crux). Everything else is witnessed per-key. The
derivation and the per-keyspace justification are in §1.

This is an ADDITIVE change: it only APPENDS digest-root leaves to the v5 leaf set, gated on
`BlockVersionWitnessable` (v5). It alters no existing committed leaf, no full-node accept
path, no validity predicate, and no I1–I5 invariant. Era-3 / v4 roots stay byte-identical.

---

## 1. The exhaustive enumeration — every whole-set read, classified

### 1.1 Method — derive from the recompute's reads, not from a list

The completeness hazard is specific: it exists at a read where the recompute must know it
holds EVERY member of a committed keyspace, and the committed form gives the box no way to
prove it holds every member. That happens exactly when BOTH of these hold:

1. **The recompute ranges the WHOLE committed map** (a `for id := range c.<map>` that is
   NOT bounded by the block payload). A payload-bounded read (one key per named id) is
   witnessed per-key and carries no hidden-member hazard — the box sees the exact key set.
2. **The committed form is one leaf PER MEMBER with no aggregate digest** (Class-A
   set-membership or Class-B value-carrying leaves). If the committed form is already a
   single DIGEST leaf (like `dueBucket`), reconstruction closes completeness with no new
   leaf.

A keyspace that fails (1) — read per-key — is fine. A keyspace that passes (1) but is
already digest-committed — `dueBucket` — is fine (the ratified TTL close). A keyspace that
passes (1) AND fails to be digest-committed needs a new digest root. This is the set below.

I derived the whole-set reads by reading the v5 recompute read-set producer
(`core/chain/readset_v5.go`, the CERTIFIED read-set identity, AMENDED cert), which is
payload-driven by construction and emits exactly the keys the recompute reads. Every
unbounded `for id := range c.<map>` in that producer is a whole-set read. There are exactly
three, plus the already-digest-committed `dueBucket` bucket-member loop.

### 1.2 The 23-keyspace read-set, each keyspace marked

The v5 committed leaf set is 23 keyspaces: the 18 era-3 leaves (`stateRootTags`,
`statehash.go:95-101`) + 5 v5 leaves (`stateRootTagsV5`, `statehash.go:89`: `qualified`,
`dueBucket`, `epochStart`, `era4LockedIn`, `era4Height`). Classified:

| # | Keyspace | Leaf shape | Read pattern in the recompute | Verdict |
|---|---|---|---|---|
| 1 | `byRoot` | Class-A per-member | per-entry absent/present (`readSetEntries:197`, `readSetTakedowns:218`) — payload-bounded | witnessed per-key — FINE |
| 2 | `spent` | Class-A per-member | per-entry absent when a token rides (`readSetEntries:199`) — payload-bounded | witnessed per-key — FINE |
| 3 | `revoked` | Class-A per-member | per-unrevocation present (`readSetTakedowns:228`) — payload-bounded | witnessed per-key — FINE |
| 4 | `slashed` | Class-A per-member | per-named-id gate (`readSetBondRegs:260`, `readSetSlashes:329`, `readSetAtts:360`, `readSetMaturityLatch:421`) — always bounded by a payload id or a `validatorsSeen` member | witnessed per-key — FINE (its objective-maturity reads are subordinate to `validatorsSeen`; see §1.4) |
| 5 | **`validatorsSeen`** | **Class-A per-member** | **`readSetMaturityLatch` ranges the WHOLE map (`:408` legacy, `:419` objective); `C2Metric` ranges `validatorsSeen` (`chain.go:2305`)** | **WHOLE-SET, no digest → NEEDS `validatorsSeenRoot`** |
| 6 | **`bonded`** | Class-B per-member | per-named-id write-target (`addBondedRead`); per-`validatorsSeen`-member in objective maturity (`:426`); per-bucket-member on TTL (`:481`) | witnessed per-key — FINE, but its maturity read is subordinate to `validatorsSeen` completeness (§1.4) |
| 7 | **`epochSet`** | **Class-B per-member** | **`readSetBoundaryDelta` ranges the WHOLE prior map (`:535`); mature-objective atts qualification (`addEpochSetRead`, per attester)** | **WHOLE-SET, no digest → NEEDS `epochSetRoot`** |
| 8 | `bondRootOwner` | Class-B per-member | per-reg displacement branch (`readSetBondRegs:270`) — payload-bounded | witnessed per-key — FINE |
| 9 | `bondRootProven` | Class-B per-member | per-reg displacement branch (`readSetBondRegs:272`) — payload-bounded | witnessed per-key — FINE |
| 10 | `bondRegHeight` | Class-B per-member | per-named-id TTL-clock (`readSetBondRegs:292`); per-bucket-member on TTL (`:483`) — payload/bucket-bounded | witnessed per-key — FINE |
| 11 | `regVersion` | Class-B per-member | per-named-id (`addRegVersionRead`); **per-`qualified`-member at the boundary (`:528`)** | witnessed per-key, but its boundary read is subordinate to `qualified` completeness (§1.4) |
| 12 | `bondDomain` | Class-B per-member | per-named-id (`addBondDomainRead`); per-`validatorsSeen`-member in objective maturity (`:427`) | witnessed per-key, subordinate to `validatorsSeen` (§1.4) |
| 13 | **`qualified`** | **Class-B per-member (v5)** | **`readSetBoundaryDelta` ranges the WHOLE map (`:526`) — the freeze source frozen into `set` and tallied `3*ready>2*total`** | **WHOLE-SET, no digest → NEEDS `qualifiedRoot`** |
| 14 | `dueBucket` | **DIGEST leaf (v5)** | occupied-bucket members ranged (`readSetTTLCompleteness:480`), but the leaf VALUE is `dueBucketMTH(ids)` | already digest-committed — closes by RECONSTRUCTION (ratified TTL crux) — FINE |
| 15 | `everMature` | scalar | one reserved-key leaf (`readSetScalars:441`) — always present | single-key scalar — FINE |
| 16 | `matureEpoch` | scalar | `readSetScalars:442` | single-key scalar — FINE |
| 17 | `gateLockedIn` | scalar | `readSetScalars:443` | single-key scalar — FINE |
| 18 | `gateHeight` | scalar | `readSetScalars:444` | single-key scalar — FINE |
| 19 | `era3LockedIn` | scalar | `readSetScalars:445` | single-key scalar — FINE |
| 20 | `era3Height` | scalar | `readSetScalars:446` | single-key scalar — FINE |
| 21 | `epochStart` | scalar (v5) | `readSetScalars:447` | single-key scalar — FINE |
| 22 | `era4LockedIn` | scalar (v5) | `readSetScalars:448` | single-key scalar — FINE |
| 23 | `era4Height` | scalar (v5) | `readSetScalars:449` | single-key scalar — FINE |

### 1.3 The three that need a digest root — per-keyspace justification and wrong-accept mechanism

**`qualified` → `qualifiedRoot`.** `readSetBoundaryDelta` (`readset_v5.go:526`) ranges the
whole `qualified` map at a boundary. The recompute freezes `qualified` into `set`
(`chain.go:3425` `cloneInt64MapID(c.qualified)`) and runs three activation tallies over the
FULL frozen weight: `for id, w := range set { total += w; if regVersion[id]>=V { ready += w }}`
then `3*ready > 2*total` (`chain.go:3442/3448`, `:3465/3471`, `:3489/3495`). This is a
super-quorum over the complete frozen weight; it is NOT delta-computable. **Wrong-accept:** a
producer that OMITS a `qualified` member from the boundary read-set lowers `total` (and, if
the omitted member had NOT signalled, RAISES `ready/total`), which can flip a lock-in from
not-yet to locked-in — forging an era-gate / era-3 / era-4 activation boundary a full node
would not reach. The current shape gate anchors completeness to the producer's own read-set
(`witness_bound.go:301-329`, `checkShape`), which the adversarial producer controls. No
committed digest exists to bind the frozen set complete → the hazard is live.

**`epochSet` → `epochSetRoot`.** `readSetBoundaryDelta` (`readset_v5.go:535`) ranges the
whole PRIOR `epochSet` map. The freeze OVERWRITES `epochSet = clone(qualified)`, so each
prior `epochSet` member is a write-target the recompute reads to compute the removal delta
(members present before the freeze but absent after). Separately, in a mature objective
epoch the attestation loop reads `epochSet` membership as the qualification set
(`attesterQualifiedAt:1297`, via `addEpochSetRead`). **Wrong-accept:** a producer that omits
a prior-`epochSet` member hides a freeze REMOVAL — the box computes a post-freeze `epochSet`
leaf set that still contains the stale member, so its recomputed v5 root diverges from what a
full node commits, and a matching forged root would let a lapsed member keep qualifying. The
membership read also mis-qualifies an attester if the box cannot bind the complete `epochSet`.
No committed digest → the hazard is live. (Note: `epochSet` was not explicitly enumerated as
a distinct face in the CRUX cert or the PE cross-check, which named `qualified`/`bonded`/
`validatorsSeen`. It surfaces here because `readSetBoundaryDelta` ranges the WHOLE prior
`epochSet` at `:535`. I flag it as a Builder-added face for the Researcher to confirm or
refute — see §6 and the honesty note in §7.)

**`validatorsSeen` → `validatorsSeenRoot`.** `readSetMaturityLatch` ranges the whole
`validatorsSeen` map (`readset_v5.go:408` legacy, `:419` objective). On the maturity-latch
transition, `matureNow`/`MatureCoefficient`/`C2Metric` iterate `validatorsSeen`
(`chain.go:2305`) and per SEEN member read `slashed`/`bonded`/`bondDomain`. The predicate is
a coefficient over the complete seen set. **Wrong-accept:** a producer that omits a
`validatorsSeen` member changes the maturity coefficient and can force the one-way
`everMature` latch to fire (`chain.go:3303-3305`; the latch is permanent, `chain.go:2146-2150`).
A forged maturity latch is a PERMANENT wrong-accept — it never re-arms. The PE cross-check
(Correction 1) confirmed the source ranges `validatorsSeen`, NOT `bonded` — the earlier cert
prose lagged the source-correct producer. So the digest MUST be over the `validatorsSeen`
iteration set. No committed digest → the hazard is live.

### 1.4 Why the subordinate reads do NOT need their own digest

`slashed`, `bonded`, `bondDomain` are read per-`validatorsSeen`-member in objective maturity;
`regVersion` is read per-`qualified`-member at the boundary. These are NOT independent
whole-set reads. Once the box binds the COMPLETE `validatorsSeen` set (via
`validatorsSeenRoot`) and the COMPLETE `qualified` set (via `qualifiedRoot`), the per-member
`slashed[id]`/`bonded[id]`/`bondDomain[id]`/`regVersion[id]` reads are single-key witnessed
reads keyed by a member the box now provably holds ALL of. Completeness of the OUTER set
makes the inner per-member reads complete by construction. So no `slashedRoot`/`bondedRoot`/
`bondDomainRoot`/`regVersionRoot` is required. **This is the load-bearing simplicity call:**
three digest roots, not seven. It rests on the claim that the maturity/boundary per-member
reads are keyed ONLY by members of `validatorsSeen`/`qualified` and never by an id outside
those sets. §6 lists this as a model-check obligation (the subordination invariant) so it is
proven, not asserted.

`bonded`'s objective-maturity read (`C2Metric` line `chain.go:2309` `c.bonded[id]` for
`id ∈ validatorsSeen`) is the specific case the PE Correction 1 clarified: the maturity face
iterates `validatorsSeen`, and `bonded` is read per-seen-member — so `validatorsSeenRoot`
covers it. There is no whole-`bonded`-map read anywhere in the v5 recompute. Confirmed
against `readset_v5.go` (no `for id := range c.bonded`).

---

## 2. The digest structure — one MTH per new root leaf

Each new root leaf mirrors `dueBucket` exactly. The `dueBucket` leaf VALUE is
`dueBucketMTH(ids)` — the RFC-6962 Merkle Tree Head over the CANONICAL (sorted-ascending by
raw NodeID, dedup, unpadded) id list (`statehash.go:217-234`, verified: sort by
`bytes.Compare` at `:229-231`, `translog.MTH` at `:232`). The new roots reuse the SAME
construction.

### 2.1 Serialization — the canonical id-list MTH

For each of the three keyspaces, define a keyspace root as the RFC-6962 MTH over the
canonical member-id list:

```
qualifiedRoot        = MTH( sort_asc_by_rawbytes( { id : id ∈ keys(qualified) } ) )
epochSetRoot         = MTH( sort_asc_by_rawbytes( { id : id ∈ keys(epochSet) } ) )
validatorsSeenRoot   = MTH( sort_asc_by_rawbytes( { id : id ∈ keys(validatorsSeen) } ) )
```

- **Leaf entries:** the raw 32-byte `ports.NodeID` bytes, exactly as `dueBucketMTH` uses
  `ports.Hash(id)` (`statehash.go:227`). Keys only — NOT the weight value. The digest
  commits SET MEMBERSHIP (which ids are present). The per-member WEIGHT is already committed
  in the existing per-member Class-B leaf (`qualified[id] → EncodeInt64(w)`,
  `statehash.go:191`), which the box witnesses per-key once it knows the complete key set.
  This keeps the new digest a pure membership commitment and avoids re-committing weight in
  two places (a divergence seam).
- **Canonical order:** sort ascending by raw id bytes (`bytes.Compare`), dedup (a map keyset
  is already unique), unpadded — identical to `dueBucketMTH`. Canonical order forecloses MTH
  malleability (the RECERT2 canonical-list pin) and makes the committed value independent of
  Go map iteration order.
- **Empty keyspace:** if the keyspace is empty (`len(map) == 0`), the MTH is the RFC-6962
  empty-tree hash (`translog.MTH(nil)`). The design must pin whether an empty keyspace emits
  the empty-MTH leaf or emits NO leaf. **Recommendation: emit NO leaf when empty**, matching
  `dueBucket`'s per-height no-empty-bucket discipline, and let the box discharge "keyspace
  empty" with ONE non-membership proof of the root key. This requires a no-empty-keyspace
  invariant analogous to the `dueBucket` one (§5). ALTERNATIVE: always emit the empty-MTH
  leaf (simpler invariant, one extra leaf). The Researcher should pin this — it is a
  soundness-adjacent choice, not a free one. See §5.

The reuse point: `dueBucketMTH` (`statehash.go:224-234`) already implements exactly this
over a `map[ports.NodeID]struct{}`. A shared helper `keysetMTH(ids)` generalizes it to any
id-keyed map. NO new crypto, NO new primitive — `translog.MTH` is the one audited RFC-6962
implementation, already in-tree.

### 2.2 Why membership-only, not a value-committing digest

The tallies read WEIGHT (`total += w`), so one might commit `MTH(id || weight)` pairs. Do
not. The weight is already per-member-committed. A membership digest + per-key weight witness
gives the box the complete member set AND each member's authentic weight, with no second
commitment of the weight to keep in sync. Committing weight into the digest too would create
a divergence seam (two committed encodings of the same weight). The membership digest is the
minimal completeness commitment; weight rides the existing per-member leaf. This is the same
shape `dueBucket` uses (the bucket digest commits membership; each member's
`bonded`/`bondRegHeight`/`regVersion` ride their own per-member leaves,
`readset_v5.go:480-492`).

---

## 3. How each root leaf is committed in the v5 state root

Three new v5-ONLY tags and three new leaves, appended in `stateRootLeavesV5`
(`statehash.go:182-214`), gated on the v5 leaf set only.

### 3.1 New tags (statehash.go const block)

```
tagQualifiedRoot       = "qualifiedRoot\x00"
tagEpochSetRoot        = "epochSetRoot\x00"
tagValidatorsSeenRoot  = "validatorsSeenRoot\x00"
```

The `\x00` suffix keeps `tag || rawKey` injective across all keyspaces and scalar reserved
keys (research cert Q3). Each root leaf is a SCALAR-shaped leaf: one leaf at the reserved key
`Key(tag, nil)` (empty raw key), value = the 32-byte MTH digest. It is NOT a per-member
leaf; it is a single digest leaf, exactly like a scalar but carrying a hash value.

### 3.2 Emission in stateRootLeavesV5 (v5-only)

Append to `stateRootLeavesV5` AFTER the existing v5 leaves, BEFORE `return leaves`:

```
// R-boundary digest roots: one MTH-over-canonical-keyset leaf per whole-set-read
// keyspace, so a root-only floor box can bind each set COMPLETE by reconstruction
// (mirrors the dueBucket digest). v5-ONLY; era-3/v4 roots are untouched.
add(tagQualifiedRoot,      nil, keysetMTH(keysOf(c.qualified)))
add(tagEpochSetRoot,       nil, keysetMTH(keysOf(c.epochSet)))
add(tagValidatorsSeenRoot, nil, keysetMTH(keysOf(c.validatorsSeen)))
```

(If the no-empty-keyspace choice in §2.1 is "emit no leaf when empty", each `add` is guarded
by `if len(map) > 0`. If "always emit", it is unconditional. §5 pins this.)

### 3.3 The era gate — additive, v4/era-3 untouched

These leaves are emitted ONLY by `stateRootLeavesV5`, never by `stateRootLeaves` (the era-3
marshaller, `statehash.go:108-163`). `StateRootForVersion` (`statehash.go:236+`) selects the
v5 leaf set only for a v5+ block; a v4 block's root stays byte-identical to era-3 (hazard-1,
immutable #632 — the frozen era-3 leaf set is not edited). This is the SAME gating the
existing five v5 leaves use. The three coverage/classification oracles
(`stateRootTagsV5` in `statehash.go:89` and `TestStateRootV5CoversExactlyTheV5Fields`) must
be extended to include the three new tags so the coverage guard stays exact (§5).

**Format-surface impact:** this ADDS three leaves to the v5 committed root. Any node
computing a v5 root — producer and validator — must add these three leaves or its v5 root
diverges. That is the whole point (the root now commits set-completeness), and it is why this
is a v5 committed-format change requiring certification + ratification (§6). It changes the
v5 root value for EVERY v5 block. Because era-4 is still OPEN (not frozen, not activated),
this is additive-before-freeze, not a format break of a shipped era.

---

## 4. How the recompute reconstructs and binds each root

The same reconstruct-and-compare pattern the ratified `dueBucket` close uses. For each of the
three roots, on the block class that reads the corresponding whole set:

1. **Witness the root leaf.** The box fetches the single-key SMT witness for the reserved
   root key (e.g. `Key(tagQualifiedRoot, nil)`) and verifies it against the committed parent
   StateRoot (`core/statehash/witness.go:195` `Resolve` → `smt.VerifyProof`). A verified
   proof yields the AUTHENTIC committed digest `R = qualifiedRoot_committed`; a wrong value
   fails to verify → `NoWitness` → stall (`witness.go:201-207`). Never Accept on a failed
   proof.
2. **Collect the witnessed member set.** The box collects the id set the witness bundle
   carries for that keyspace — the per-member leaf keys the producer emits for
   `qualified`/`epochSet`/`validatorsSeen` (already in the read-set:
   `readSetBoundaryDelta:526/535`, `readSetMaturityLatch:408/419`).
3. **Reconstruct.** Recompute `R' = keysetMTH(collected_ids)` locally (the same RFC-6962
   `translog.MTH` the producer commits with).
4. **Compare.** Require `R' == R`. If they differ, STALL (`IndeterminateTrustlessly`); never
   Accept. Only after `R' == R` may the recompute run the boundary tally / maturity
   coefficient over the collected set — it is now provably COMPLETE.

**Soundness (mirrors the ratified `dueBucket` argument).** `keysetMTH` is a deterministic
collision-resistant SHA-256 RFC-6962 MTH over a canonical input. If `R' == R`, the collected
set equals the committed keyset except with SHA-256-collision probability. An adversary who
OMITS a member produces a strict subset → a different MTH → mismatch → stall. An adversary
who ADDS a non-member produces a different MTH → mismatch → stall. Completeness is bound by
the digest itself. No accumulator, no vector commitment, no per-keyspace SMT sub-root (which
`smt@v1.0.0` does not expose).

**Shape-gate extension.** As with `dueBucket`, the shape gate
(`witness_bound.go:301-329`, `checkShape`) currently binds the bundle key set to the
PRODUCER's read-set, not to the digest. It must be EXTENDED so the collected member set for
each whole-set keyspace is bound to the proven root via the reconstruction check above — the
completeness anchor moves from "the producer's read-set" (adversary-controlled) to "the
committed digest" (root-protected). This is a validation-step addition in the box, gated on
v5; it changes no full-node path.

**Per-class firing (bounds the obligation):**

- `qualifiedRoot` + `epochSetRoot`: reconstructed on EPOCH-BOUNDARY blocks (the boundary
  tallies + freeze). `readSetBoundaryDelta` already gates on
  `epochsEnabled && h%EpochBlocks==0`.
- `validatorsSeenRoot`: reconstructed on the SINGLE pre-latch maturity transition
  (`readSetMaturityLatch` early-returns once `everMature` is set, `:399`). The latch is
  one-way (`chain.go:2146-2150`), so the box needs `validatorsSeen` completeness on exactly
  the one transition. Bounded, but not optional — a forged latch is permanent.

**Witness size.** Each reconstruction transfers the keyspace's full id-list (O(members)).
All three keyspaces are RegCap-bounded (frozen set / seen set ⊆ RegCap; R1). At RegCap=256
this is kilobytes per root — the boundary witness is already O(RegCap) (AMENDED cert), so the
three id-lists do not change the asymptotic box-fit. NO witness-size regression beyond the
already-priced O(RegCap) boundary class.

---

## 5. Model-check items owed

### 5.1 The empty-bucket / empty-keyspace non-membership shortcut invariant (PE-flagged)

The PE cross-check (Q3) flagged the `dueBucket` empty-bucket shortcut as a COMPLETENESS FACE
contingent on the `dueBucketRemove` no-empty-bucket invariant, which it did not read. **I
read `dueBucketRemove` this round (`chain.go:1416-1428`) and state the invariant:**

> `dueBucketRemove(id, d)` deletes `id` from bucket `d`, and `if len(b) == 0 { delete(c.dueBucket, d) }` — it removes the bucket entirely once its last id leaves (`chain.go:1424-1426`). `dueBucketInsert` only ever creates a bucket to add an id to it (`chain.go:1407-1413`). Therefore **`dueBucket` never holds an empty bucket**: a height key is present iff at least one id is due at that height. So `dueBucket[h]` ABSENT ⟺ nothing is due at h, and ONE non-membership proof of `dueBucket[h]` soundly discharges the entire "nothing expired at h" claim (`readSetTTLCompleteness:472-475`).

This invariant MUST be in the model-check tier (`docs/build-process.md` consensus-correctness
discipline), stated as: *for every reachable committed state, `dueBucket[h]` is present iff
`∃ id : regHeight(id)+ttl+1 == h`; no empty bucket persists.* It is a consensus-rule property
of `apply()`/`dueBucketRemove`/`dueBucketInsert`; discover it in the model-check, never in the
field. Ablate: inject a persisted empty bucket → the empty-bucket shortcut must NOT yield
Accept.

**Extension to the new roots.** IF §2.1 chooses "emit no leaf when the keyspace is empty",
each of `qualified`/`epochSet`/`validatorsSeen` inherits the SAME obligation: prove the
keyspace root key is present iff the keyspace is non-empty, so the box may discharge "keyspace
empty" with one non-membership proof. This is weaker than the `dueBucket` per-height
invariant (a single whole-keyspace emptiness, not per-height), but it is still a model-check
obligation. IF §2.1 chooses "always emit the empty-MTH leaf", this obligation disappears (the
root leaf is always present) at the cost of one always-present leaf. **Recommend "always
emit"** for the three new roots specifically — they are single scalar-shaped leaves, so the
one-leaf cost is trivial and it removes an invariant. (This differs from `dueBucket`, which is
per-height and would pay one leaf per empty height — there "no empty bucket" earns its keep.)
The Researcher pins this.

### 5.2 A completeness ablation per new root (execution-derived, R3)

Per the R3 mandate (execution-derived drift guard, the session-7 scar — a hand-written mirror
shares the producer's blind spot), each new root needs a completeness ablation that goes RED
before the reconstruction check and GREEN after:

- **`qualifiedRoot` ablation:** drop one frozen `qualified` member from the witnessed set at a
  boundary → the reconstruction `R' == R` must FAIL → box stalls, never Accepts. Assert the
  dropped member would have flipped a tally (or moved `total`) so the ablation exercises the
  wrong-accept mechanism, not just a hash mismatch.
- **`epochSetRoot` ablation:** drop one prior-`epochSet` member → reconstruction FAILS → stall.
- **`validatorsSeenRoot` ablation:** drop one `validatorsSeen` member on the pre-latch
  transition → reconstruction FAILS → stall; assert the drop would have changed the maturity
  coefficient.

Each ablation must be derived from the REAL v5 recompute (the execution-derived guard,
`readset_v5_drift_test.go` shape), not a hand-written mirror.

### 5.3 The subordination invariant (§1.4)

Model-check that the per-member reads in the maturity/boundary recompute are keyed ONLY by
members of the outer whole set:

- Every `bonded[id]`/`slashed[id]`/`bondDomain[id]` read on the maturity latch has
  `id ∈ validatorsSeen`.
- Every `regVersion[id]` read at the boundary has `id ∈ qualified`.

If this holds, `validatorsSeenRoot`/`qualifiedRoot` completeness makes the inner reads
complete by construction, and no `bondedRoot`/`slashedRoot`/`regVersionRoot` is needed
(the three-roots-not-seven call). If it fails — an inner read keyed by an id outside the
outer set — that read is an UN-enumerated whole-set hazard and this design is INCOMPLETE
there. Ablate: a member read at an id outside the bound outer set must be catchable.

### 5.4 Recompute equivalence across all three block classes

Model-check that the v5 witnessable recompute, WITH the three reconstruction checks, Accepts
iff a full node accepts, across ordinary / TTL-firing / epoch-boundary blocks. This is the
standing R-crux + R-boundary equivalence obligation; the three new roots are the boundary/
maturity half of it.

---

## 6. Scope, gated surfaces, and what needs certification + ratification

### 6.1 What this change is (additive)

- ADDS three v5-only tags (`tagQualifiedRoot`/`tagEpochSetRoot`/`tagValidatorsSeenRoot`).
- ADDS three digest leaves to `stateRootLeavesV5` (v5 root only).
- ADDS three reconstruction-and-compare checks + a shape-gate extension in the FLOOR-BOX
  validation path (`WitnessValidateV5`), gated on `BlockVersionWitnessable`.
- EXTENDS the v5 coverage oracles (`stateRootTagsV5`,
  `TestStateRootV5CoversExactlyTheV5Fields`) to include the three tags.

### 6.2 What this change must NOT do (the bright lines)

- It must NOT alter any existing committed leaf (era-3 or the existing five v5 leaves). The
  v4/era-3 root stays byte-identical (immutable #632, hazard-1).
- It must NOT change `apply()`, `validateEra3Roots`, `postApplyRoots`, any validity predicate,
  fork-choice, epochs, slashing, or any I1–I5 invariant. The full-node accept path is
  untouched; this is a floor-box (root-only client) completeness addition + the digest leaves
  the producer commits.
- It must NOT change RegCap or any security parameter (R1 unchanged; the boundary witness is
  already O(RegCap)).
- It commits MEMBERSHIP only (not weight) into the digests, to avoid a second committed
  encoding of the weight (§2.2).

### 6.3 What needs Research certification

- **The digest structure** (§2): that a membership-only canonical-keyset MTH is the correct
  completeness commitment for each of the three keyspaces, and that membership-digest +
  per-member-weight-leaf is sound (no gap from committing membership and weight separately).
- **The exhaustive enumeration** (§1): that these THREE keyspaces (and no others) are the
  whole-set reads needing a digest — in particular, CONFIRM or REFUTE the Builder-added
  `epochSet` face (§1.3), which the CRUX cert and PE cross-check did not separately enumerate.
- **The subordination claim** (§1.4 / §5.3): that the maturity/boundary per-member reads are
  keyed only by members of the bound outer sets, so three roots suffice (not seven).
- **The reconstruction-and-compare soundness** (§4) for each root, and the shape-gate
  extension binding the collected set to the proven digest.
- **The empty-keyspace choice** (§2.1 / §5.1): emit-no-leaf-when-empty vs always-emit, and
  the invariant each entails.
- **Recompute equivalence across the three block classes** with the three checks (§5.4).

### 6.4 What needs owner ratification

- **The v5 committed-format change itself.** Committing `qualifiedRoot`/`epochSetRoot`/
  `validatorsSeenRoot` digest leaves changes the committed v5 state-root format — a
  consensus-rule change under `silt/.claude/CLAUDE.md`. The owner ratifies the format
  addition. This is the R-boundary gate.
- **Keeping the v5 format freeze deferred until this lands.** The certification and the PE
  cross-check both concluded the v5 freeze stays deferred until R-boundary is certified +
  ratified. Freezing before these leaves land would lock a format the sound recompute cannot
  bind at the boundary/maturity. The owner's call, on the certified evidence.

### 6.5 Coupling to the #535 recovery boundary (R2)

The recovery-branch frozen set is `liveQualifiedSet()` (`chain.go:3423`), not the
materialized `qualified`. Its completeness is the #603-gated keystone obligation (R2). The
`qualifiedRoot` digest binds the NORMAL boundary's frozen set (`cloneInt64MapID(c.qualified)`).
The recovery boundary's completeness binding rides the SAME digest ONLY IF `liveQualifiedSet()`
equals `qualified` at that height — which the recovery re-base is defined NOT to assume
(`chain.go:3415-3419`). So the recovery-boundary completeness may need its own binding under
the operator directive; sequence it with #603. This design binds the normal boundary; it flags
the recovery boundary as the R2-coupled residual, unclosed here.

---

## 7. Honest uncertainties

- **The `epochSet` face is Builder-added.** The CRUX cert (sub-Q2) and the PE cross-check
  named `qualified`, `validatorsSeen`, and (cert prose) `bonded`. Neither separately
  enumerated `epochSet`. I add it because `readSetBoundaryDelta` ranges the WHOLE prior
  `epochSet` at `readset_v5.go:535`, which is a whole-set read of a per-member-leaf keyspace
  by the §1.1 criterion. I may be wrong that the prior-`epochSet` read needs COMPLETENESS
  (as opposed to being fully determined by the freeze write, which overwrites `epochSet` from
  `qualified`). If the post-freeze `epochSet` is entirely a function of `qualified` (already
  bound by `qualifiedRoot`) and the prior `epochSet` is read ONLY to compute removed leaves
  whose completeness follows from `qualifiedRoot` + the prior root's own commitment, then
  `epochSetRoot` may be redundant. I flag this as the sharpest open question for the
  Researcher — I did NOT resolve it, and I would rather over-enumerate (name it, let Research
  refute) than miss a wrong-accept. See §6.3.
- **The subordination invariant (§1.4) is asserted, not proven.** The three-roots-not-seven
  simplicity call depends on it. I read the maturity/boundary reads and believe every inner
  per-member read is keyed by an outer-set member, but I did not exhaustively trace every
  branch of `attesterQualifiedAt` / `C2Metric` / the boundary tally for an id sourced outside
  the outer set. It is a §5.3 model-check obligation, not a settled fact.
- **The empty-keyspace choice (§2.1) is a recommendation, not a decision.** I recommend
  always-emit for the three roots (removes an invariant, one trivial leaf). The Researcher
  pins it; it is soundness-adjacent.
- **This is DESIGN ONLY.** No production code is written. The tags, leaves, checks, and
  invariants above are the specification for Research to certify and the owner to ratify. No
  step here Accepts a block; the #657 scaffold's never-Accept default holds until the
  recompute is built, certified, and ratified.

---

## Sources (verified this round at `origin/main` 61c75eb)

- `core/chain/statehash.go:38-82` the tag const block; `:89` `stateRootTagsV5` (5 v5 leaves);
  `:95-101` `stateRootTags` (18 era-3); `:108-163` `stateRootLeaves` (era-3, per-member
  Class-A/B leaves — `validatorsSeen` `:127-128`, `bonded` `:132-133`, `epochSet` `:135-136`);
  `:182-214` `stateRootLeavesV5` (`qualified` per-member `:190-192`, `dueBucket` DIGEST leaf
  `:199-203`); `:217-234` `dueBucketMTH` (the MTH pattern to mirror — sort `:229-231`,
  `translog.MTH` `:232`).
- `core/chain/readset_v5.go` — the v5 recompute read-set producer. Whole-set reads:
  `readSetMaturityLatch` ranges `validatorsSeen` (`:408` legacy, `:419` objective);
  `readSetBoundaryDelta` ranges `qualified` (`:526`) and prior `epochSet` (`:535`);
  `readSetTTLCompleteness` ranges the occupied `dueBucket[h]` members (`:480`, bounded by the
  digest). All other reads are payload/member-bounded (per-key). Subordinate per-member reads:
  objective maturity `slashed`/`bonded`/`bondDomain` per `validatorsSeen` member (`:421-427`);
  boundary `regVersion` per `qualified` member (`:528`).
- `core/chain/chain.go:2300-2318` `C2Metric` ranges `validatorsSeen` (`:2305`), per-seen-member
  reads `slashed`/`bonded`/`bondDomain` (`:2306-2312`) — confirms the maturity face iterates
  `validatorsSeen`, not `bonded` (PE Correction 1). `:3421-3427` frozen-set source
  (`cloneInt64MapID(c.qualified)` normal, `liveQualifiedSet()` recovery). `:3440-3499` the three
  activation tallies range the WHOLE frozen `set` (`:3442/3465/3489`), predicate
  `3*ready>2*total`. `:2146-2150` + `:3303-3305` the one-way maturity latch. `:1407-1428`
  `dueBucketInsert`/`dueBucketRemove` — the no-empty-bucket invariant (delete at `len(b)==0`,
  `:1424-1426`).
- `core/statehash/witness.go:195-222` single-key `Resolve`/`VerifyProof` (the reconstruction's
  proof step). `core/statehash/witness_bound.go:301-329` `checkShape` (binds to the producer
  read-set today — the extension point).
- `core/translog/translog.go` `MTH` — the in-tree RFC-6962 tree-head reused by `dueBucketMTH`
  and by the three new keyset roots. No new primitive.
- Priors (full paths):
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-v5-floorbox-bounded-recompute-CRUX-RESEARCH-CERTIFICATION-2026-08-30.md`
  (R-crux ratified direction, R-boundary named);
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-v5-floorbox-recompute-crux-CROSS-CHECK-2026-08-30.md`
  (the structural reason a format addition is required; Correction 1: the maturity face
  iterates `validatorsSeen`; the empty-bucket completeness face);
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30.md`
  (the 23-keyspace read-set identity).
