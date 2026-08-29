# era-4: witnessable O(payload) transitions — design options

Status: **RATIFIED 2026-08-29 (parameters LOCKED).** Andrew ratified the format veto-gate on the
strength of the Research RE-CERTIFICATION ROUND 2 (RECERT2, CERTIFIED-WITH-CONDITIONS,
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`).
**Locked parameters:** `BlockVersion = 5`, `versionSupported <= 5` (PREDICATE-FIRST — widen the
version ceiling in the SAME release as the v5 predicate); three new committed tags `tagDueBucket` +
`tagQualified` + `tagEpochStart` (`tagEpochSet` retained); the TWO-keyspace layout (frozen
materialized `epochSet` + live `qualified` accelerator); the new `RegCap` fresh-registration
validity rule **value = 256**, with a HARD coupling gate — **if #299 ships, `RegCap` MUST be
re-measured/re-minted** (measured honest ceiling rises above 256 under succinct proofs).
SEPARABLE / deferred by Andrew: the recovery-boundary direction (`effectiveEpochSet` at
`LivenessRecoveryHeight`) is NOT in era-4-minimum. Canon record: `docs/decisions.md` (era-4 entry,
2026-08-29). The ORDERED build decomposition is a separate PACE deliberation:
`docs/thinking/2026-08-29-era4-build-decomposition-options.md`. This doc below is the design
deliberation as certified; the parameters above supersede any un-pinned brackets in the body.

Prior status (superseded): DESIGN OPTIONS (PACE deliberation), REVISED (rev 2) to fold in the Research
RE-CERTIFICATION (2026-08-29), on top of the earlier PE ruling and first Research cert. No code.
Date: 2026-08-29. Author seat: Builder.
Grounded against `origin/main` @ `0984db4` (local `main` @ `2003439` was STALE and lacks
the merged R3/R4 witness machinery — verified before writing).

Reviews folded in (read all three; they name the exact fixes):
- PE: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-witnessable-transitions-2026-08-29.md` (SHIP-WITH-FIXES).
- Research (first pass): `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-EQUIVALENCE-RESEARCH-2026-08-29.md` (GATED).
- Research (RE-CERT, the current authority): `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT-2026-08-29.md` (STILL GATED — Q2 REFUTED, R3 cap value OPEN).

What rev 2 changed (summary; detail in each section):
1. **§5 E-2 boundary is CORRECTED.** The RE-CERT REFUTED the single shared `qualified`/`epochSet`
   keyspace + epoch-pointer advance (Q2). Reason: a floor box holds only the CURRENT block's
   root, so a single live-mutating keyspace either is unwitnessable as-of the boundary or leaks
   mid-epoch changes into the governing quorum set — an I1/I3 divergence. The sound direction
   (RE-CERT direction b): `epochSet` STAYS its own FROZEN materialized keyspace (era-3
   `tagEpochSet` shape); a live `qualified` keyspace is committed too, but as a BOUNDARY-COMPUTATION
   ACCELERATOR, not a pointer target.
2. **§5/§7 the epoch-boundary block is now a DISTINCT, HEAVIER witness class, NOT O(payload).**
   Its changed-leaf set is the symmetric difference between last epoch's `epochSet` and this
   epoch's `qualified` = O(boundary-delta), bounded by `RegCap × EpochBlocks × SProofMax`, which
   fits the 2 GB box for `RegCap ≤ 16,384`. The prior "zero leaves changed" claim is withdrawn.
3. Direction (a) alone (keep only era-3's `epochSet`, no live `qualified`) reintroduces the
   O(registry) boundary scan and so does NOT meet era-4's goal — the live `qualified` accelerator
   is why we still commit it.
4. **§9 folds in the MEASURED RegCap bracket:** upper bound `RegCap ≤ 16,384 ids/block` (derived,
   tight, from the boundary-witness fit); lower bound `λ_H` (honest fresh-reg arrival rate) is the
   one input NOT pinned in canon. Whether λ_H can be upper-bounded at desk is a QUESTION for
   Research, not asserted here.
5. The five-site E-2 enumeration, the canonical carried id-list pin, and the honest new-cap-rule
   statement are CERTIFIED by the RE-CERT and stand unchanged.
6. SCOPED OUT still: the `effectiveEpochSet` recovery-boundary observable (`chain.go:1243`) is a
   SEPARATE gated item (R2). O-1 (commit `epochStart`) stays; it is sound and cheap. The Q5
   coupling (recovery branch's `liveQualifiedSet()` recompute must agree with the frozen set) is
   now stated in §5.

Related, do not re-litigate:
- `docs/thinking/2026-08-29-witness-floor-box-validation-mechanism-options.md` — the floor box.
- `docs/thinking/2026-08-29-witness-floor-box-delivery-increment3-options.md` — read-set derivation + delivery.
- C-7 cert (`silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`).
- The era-3 format FREEZE (`3af40bc`, ratified 2026-08-29) — era-2/era-3 blocks stay byte-identical under their versions; era-4 does not edit them.

---

## 1. The answer up front

A witness floor box holds only the two committed roots and validates a block by verifying
O(payload) SMT proofs against those roots. It **cannot** witness-validate era-3 blocks,
because two `apply()` operations scan whole committed maps:

| Operation | Site | Scan | Fires |
|---|---|---|---|
| TTL-expiry sweep | `chain.go:3005-3013` | `for id, regH := range c.bondRegHeight` (whole map) | every block, when `BondTTLBlocks>0` |
| Epoch rotation | `rotateEpoch` → `liveQualifiedSet`, `chain.go:1198-1206` | `for id, sz := range c.bonded` (whole map) | every boundary, `h % EpochBlocks == 0` |

Both prove a whole-keyspace claim ("nothing else was due", "no other bond qualifies"). No
bounded witness certifies a whole-map scan. era-3 is FROZEN. Era-4 (a new `BlockVersion`,
= 5) re-represents these two transitions so a block's FLOOR-BOX WITNESS read-set is O(payload)
and a floor box can prove BOTH the touched set AND the completeness of the untouched remainder.
(The FULL-NODE recompute stays O(registry) and era-4 makes it slightly heavier; O(payload) is
the witness read-set only — PE finding 3, section 7.)

Recommended per operation:

| Operation | Recommended | Why (one line) |
|---|---|---|
| TTL expiry | **T-3: a due-height committed queue (bucket-per-height) + a next-bucket non-membership proof** | Turns "nothing else expired" into a single next-bucket non-membership proof; O(expiring-at-h) apply. |
| Epoch rotation | **E-2: a live-maintained committed `qualified` set as a BOUNDARY-COMPUTATION ACCELERATOR; `epochSet` STAYS its own FROZEN materialized keyspace (era-3 `tagEpochSet` shape); the boundary COPIES `qualified` → `epochSet`** | Rotation stops the O(registry) `liveQualifiedSet()` scan over `bonded` — the boundary copy reads the already-materialized `qualified`. The boundary block is a DISTINCT, HEAVIER witness class (O(boundary-delta) changed leaves), NOT O(payload). |

Both recommendations re-represent an era-3 transition so the SAME ids expire and the SAME
set qualifies. But era-4 is NOT pure rule-equivalent representation. It is a representation
change **PLUS one new consensus validity rule**: a per-block fresh-registration / due-bucket
size cap. Without it, a TTL-firing block is O(registry) at the validity layer (both reviews;
section 4 and section 9), and the design fails its own O(payload) acceptance test. Both seats
verified that #506 bounds per-IDENTITY renewal, NOT distinct-identity registrations per block,
and that `MaxBondRegBytesPerBlock` is proposer-only (applied at `chainrole.go:798`, value
`2 << 20` set at `node.go:270`, `0 = unbounded`), never at validity. State this honestly up
front: **era-4 = the two whole-map transitions re-represented + one new validity cap rule +
O-1 (commit `epochStart`).**

The epoch-boundary block is NOT O(payload). This is the load-bearing correction the RE-CERT
forced (Q2 REFUTED). The ordinary block and the TTL-firing block are O(payload); the
epoch-boundary block is a distinct, heavier witness class whose changed-leaf set is the
symmetric difference between last epoch's frozen `epochSet` and this epoch's live `qualified`.
Its cost is bounded: `boundary_witness ≤ boundary_delta × SProofMax ≤ RegCap × EpochBlocks ×
SProofMax`, which fits the 2 GB floor box for `RegCap ≤ 16,384` (section 5, section 9). Do not
claim the boundary is O(payload) or zero-leaves.

Both transition changes are consensus-mechanism + format changes and are **research-gated**
on the equivalence and completeness arguments below (RE-CERT verdict: STILL GATED — Q2 design
correction routed back, R3 cap value OPEN; lift conditions in section 11). The `#535`
observable is handled in two parts: era-4 commits `epochStart` (O-1, sound and cheap), but the
deeper `effectiveEpochSet` recovery-boundary re-base is SCOPED OUT as a separate gated item
(section 6) — not smuggled, not fixed here.

---

## 2. The primitive floor: what pokt-network/smt@v1.0.0 actually gives us

Read the API before designing proof shapes over it. The tree stores each leaf at
`H(key)`, and exposes exactly:

| Primitive | Proves | Shape |
|---|---|---|
| `VerifyProof(proof, root, key, value)` | membership (`len(value)>0`) OR non-membership (`len(value)==0`) of ONE key | single-key |
| `VerifyClosestProof(proof, root)` | the leaf whose **hashed path** `H(key)` is closest to a supplied path | single-leaf, hash-space |
| `VerifySumProof` / sum trie | membership with a summed weight leaf | single-key, weighted |
| Compact variants | the same, smaller encoding | single-key |

There is **NO batch proof and NO native range proof.** This is the load-bearing constraint
for the TTL options. Two consequences drive the design:

1. **The tree destroys domain ordering.** Leaves sit at `H(key)`, so "the next key after X
   in key order" is NOT "the closest leaf under `ProveClosest`" — `ProveClosest` is
   closeness in HASH space. A range/frontier proof in the *domain* order (expiry height)
   cannot be read off the raw committed-set SMT. It requires the domain order to be encoded
   INTO the key so that domain-adjacency becomes single-key membership/non-membership. That
   is what the T-3 bucket key does (section 4).
2. **A membership proof of a scalar leaf is O(depth) = O(256 sibling nodes) bytes**, already
   bounded by the R3 per-proof cap (`SProofMax = 16 KiB`, `witness_bound.go`). Every era-4
   witness is a bundle of these, so the merged R3/R4 machinery applies unchanged as long as
   the read-set is O(payload).

---

## 3. What "witnessable" costs, precisely

A floor box that holds only the roots must, for each committed key a transition reads,
prove one of:
- **present-with-value** (membership): reused via `Resolve` with `len(value)>0`.
- **absent** (non-membership): reused via `Resolve` with `len(value)==0`.

The banned move (C-7 §104) is reading "no witness" as "absent". The R4 accessor already
makes that unrepresentable. Era-4's job is upstream of R4: make the *set of keys a block
reads* bounded by the block's payload, and — the hard part — make the **completeness
claim** ("no OTHER key was due / qualifies") expressible as a bounded set of these same
single-key proofs. A whole-map scan fails only the completeness half: enumerating the
touched set is already O(payload); proving nothing else was touched is the O(registry)
part. Every option below is judged on how it discharges the completeness half in O(payload).

---

## 4. TTL expiry → a committed due-height structure

### The era-3 rule to preserve (rule-equivalence anchor)

`chain.go:3005-3013`: at block height `h`, for every id with `h - bondRegHeight[id] >
BondTTLBlocks`, delete `bonded[id]`, `bondRegHeight[id]`, `regVersion[id]`. Equivalently:
an id expires at the FIRST height `h` where `h > bondRegHeight[id] + BondTTLBlocks`, i.e.
its **due height** is `D(id) = bondRegHeight[id] + BondTTLBlocks + 1`. The sweep fires
every block; the set expiring exactly at `h` is `{ id : D(id) == h AND still bonded }`
(re-registration resets `bondRegHeight`, hence resets `D`).

Era-4 must expire the SAME ids at the SAME heights. It may change only HOW the due set at
`h` is represented and proven.

### Options

**T-1 — sorted-by-due-height index, frontier via closest-proof. REJECTED.**
Maintain a committed index keyed by `(dueHeight, id)`. At height `h`, prove the block
touched exactly the entries with `dueHeight == h` and that the next entry is `> h`.
*Fails on the primitive floor (section 2.1):* the SMT stores leaves at `H(key)`, so a
`ProveClosest` on a sorted key gives hash-space closeness, not "the next due height". There
is no range proof in domain order over a hashed-key SMT. Reconstructing domain order would
require a second, order-preserving structure (an authenticated skip-list / Merkle-Patricia
range tree) — a NEW committed accumulator the codebase does not have, its own soundness
cert, and its own DoS surface. Rejected as gold-plating: T-3 gets the same guarantee with
the primitives already shipped.

**T-2 — per-id due-height leaf, absence-swept. REJECTED for completeness.**
Commit `dueHeight[id]` as a value leaf. At `h`, the block carries the expiring ids and a
membership proof each. *Fails the completeness half:* proving "no OTHER id has
`dueHeight == h`" is again a whole-keyspace claim — you would need a non-membership proof
for every id NOT expiring, which is O(registry). Same wall as era-3, just relocated.

**T-3 — due-height BUCKET commitment (RECOMMENDED).**
Commit a per-height bucket: one committed leaf per due-height `h` whose value is a
commitment to the SET of ids due at `h` (a nested SMT root, or an RFC-6962 MTH over the
sorted id list — a small ordered list, since a single height's cohort is payload-sized in
the honest case and bounded by the per-block reg-admission rate, the #506 R-rule).

Key: `Key(tagDueBucket, uint64BE(h))`. One leaf per occupied height.

At block height `h`, the transition is:
- Read the bucket at `h`: `Resolve(root, Key(tagDueBucket, h), bucketRoot, w)`.
  - **PROVEN_ABSENT** → no bucket at `h` → nothing expires this height. ONE non-membership
    proof discharges the ENTIRE completeness claim. This is the whole win.
  - **PROVEN_PRESENT(bucketRoot)** → the ids due at `h` are exactly the members of
    `bucketRoot`. The block payload carries that id list; the floor box verifies the list
    against `bucketRoot` (a bounded set of membership proofs into the nested commitment, or
    re-derives the MTH from the carried list and checks equality). Delete each; the count is
    the bucket size = O(payload).
- On (re)registration of `id` at height `r`: compute `D = r + BondTTLBlocks + 1`, insert
  `id` into bucket `D`, and (on a renew) remove `id` from its previous bucket `D_old`. Both
  are O(1) bucket touches with per-key witnesses — the register transition's read-set grows
  by exactly {old bucket, new bucket}, still O(payload).

Why T-3 works where T-1/T-2 fail: it converts the completeness claim "nothing else was due
at `h`" into a SINGLE non-membership proof of the key `h` in the bucket keyspace. The
domain order (by height) is encoded into the KEY (`uint64BE(h)`), and only exact-key
membership is ever asked — never range, never closest. The primitives already shipped cover
it.

Costs:
- **Apply:** O(expiring-at-h) + O(1) per register. No whole-map scan.
- **Commit:** one new committed keyspace (`tagDueBucket`) → a new field-tag in
  `statehash.go`, a new-era format decision.
- **Witness (floor box):** the empty-height case is ONE proof; the non-empty case is
  1 + bucket-size proofs. Bounded by payload.
- **Storage:** one leaf per occupied due-height. Buckets self-clean: after height `h` is
  processed the bucket at `h` is deleted (it can never be due again — heights are
  monotone).

**T-3 variant (bucket-as-nested-root vs bucket-as-carried-list).** Two sub-choices for the
bucket value:
- **(a) nested SMT root**: value = root of an SMT over the ids. Membership of one id is a
  proof-into-a-proof. Cleaner completeness (the id set is itself committed), heavier witness.
- **(b) MTH over the carried sorted id list**: value = RFC-6962 root; the block payload
  carries the full sorted id list and the floor box recomputes the MTH. Lighter (one hash
  recompute, no nested proofs), and it reuses the `revLog` MTH machinery already in-tree.
  Recommended sub-choice **(b)** — the honest cohort at one height is payload-sized, so
  carrying the list is cheap, and it avoids a second accumulator type.

  **Canonical encoding (Research Q3 / R4 — a hard-fork format parameter, must be pinned).**
  The carried id list MUST be pinned CANONICAL: sorted ascending by id, deduplicated, no
  padding. Reason: if two encodings of the same id set could hash to different roots, the MTH
  is malleable and "recompute to `bucketRoot`" no longer uniquely identifies the set — two
  distinct carried lists could both verify against different committed roots, a malleability
  seam. The shape gate (`witness_bound.go` checkShape) MUST reject any list that is not
  strictly ascending, contains a duplicate, or is padded — the floor box recomputes the MTH
  over the canonical list and rejects on any non-canonical input BEFORE hashing to
  `bucketRoot`. With the canonical pin, an adversary cannot pad (root changes), omit (root
  changes), or re-encode (rejected as non-canonical). This mirrors the era-3 `statehash.go`
  per-field canonical value encoding, which the era-3 cert already treated as a hard-fork
  parameter. Research CERTIFIED Q3 (bucket completeness) PROVIDED this pin is in place.

  Coupling the reviews flagged: the canonical-list shape gate and the per-height bucket-size
  cap (section 9) are ONE gate, not two. The shape gate can only bound the length it must hash
  if the cap bounds the bucket. Certify them together.

### Rule-equivalence risk points (TTL)

| Risk | Assessment |
|---|---|
| Does `D(id) = bondRegHeight[id] + BondTTLBlocks + 1` reproduce the era-3 `h - regH > ttl` decision? | YES arithmetically; MUST be pinned by an equivalence test that replays a corpus and asserts identical expiry sets height-by-height. **RESEARCH-GATED** (it is a consensus-rule equivalence claim). |
| Renew/resize resets the clock — does the bucket move correctly? | The era-3 rule resets `bondRegHeight` on every re-reg (`chain.go:2996`). T-3 must move the id from `D_old` to `D_new` in the SAME transition. If a renew misses the old-bucket delete, a stale entry expires the id early — a SEMANTIC divergence. This is the sharpest equivalence hazard; it must be a covered ablation. |
| `BondTTLBlocks == 0` (TTL disabled) | era-3 skips the sweep entirely. Era-4 must commit NO buckets when ttl==0 (the due height is undefined). Config-gated, no divergence if the bucket-insert is also ttl-guarded. |
| A slash between register and due height | era-3: a slashed id is removed from `bonded` but its `bondRegHeight` entry still gets swept later (harmless double-delete). Era-4: the bucket may still name a slashed id at `D`; processing it deletes an already-absent `bonded[id]` — harmless, but the equivalence test must confirm the post-state root is identical (a no-op delete does not change the committed set). Research confirmed this holds ONLY if T-3 does not resurrect or mis-handle the slashed id; the equivalence test must assert byte-identical StateRoot across the slash-before-due path. |
| **Dual-source drift (`bondRegHeight` AND the bucket) — Research Q1, R1. NEW surface era-3 lacks.** | Era-3 has ONE source of truth: it sweeps `bondRegHeight` and deletes `bondRegHeight[id]` on expiry (`chain.go:3009`). Era-4 keeps BOTH `bondRegHeight` (it still feeds the #506 R-rule distance, `statehash.go` field 9) AND the due-bucket (for expiry). The two are now a DRIFT surface. On expiry, era-4 must delete `bondRegHeight[id]` AND the bucket entry; on renew, era-4 must reset `bondRegHeight[id]` AND move the bucket. A drift between them is a divergence era-3 cannot have. **Drift-guard (owed to lift Q1):** a recording harness that replays a branch-covering corpus and asserts, after every block, `bucket-membership(id) ⟺ (bondRegHeight[id] + ttl + 1 == D AND bonded[id] present)`, and that the recomputed StateRoot equals an era-3 replay's expiry-by-expiry — ABLATED (inject a missed old-bucket delete on renew, watch it go red). This is the sharpest TTL hazard AFTER the renew-reset. |

---

## 5. Epoch rotation → an incremental committed qualified-set

### The era-3 rule to preserve (rule-equivalence anchor)

`rotateEpoch` (`chain.go:3124-3131`) at a boundary calls `liveQualifiedSet`
(`chain.go:1198-1206`): `epochSet := { id → sz : bonded[id] == sz, sz >= MinBond,
!slashed[id] }`. The rotation FREEZES this set as the governing epoch set, and the #506 /
era-3 lock-in tallies weight over it (`chain.go:3144-3179`). The set is a pure function of
committed `bonded`, `slashed`, and the `MinBond` config. Era-4 must freeze the SAME set.

### Options

**E-1 — witness the whole scan with a completeness accumulator. REJECTED.**
Prove `epochSet` at the boundary by proving, for every bonded id, whether it clears MinBond
and is unslashed. This is O(registry) proofs at the boundary — the exact wall era-4 exists
to remove. Rejected.

**E-2 — a live-maintained committed `qualified` keyspace as a BOUNDARY-COMPUTATION ACCELERATOR;
`epochSet` STAYS its own FROZEN materialized keyspace (RECOMMENDED, RE-CERT direction b).**
Maintain the qualified set INCREMENTALLY as an explicit committed structure, updated by the
same transitions that already move `bonded`/`slashed`. At the boundary, `epochSet := qualified`
is a COPY into a SEPARATE frozen keyspace — so rotation no longer runs the O(registry)
`liveQualifiedSet()` scan over `bonded`. The live `qualified` IS that scan's answer, kept up to
date every block.

Structure — TWO committed keyspaces, distinct by construction:
- **`qualified`** (`Key(tagQualified, id) → EncodeInt64(weight)`): the LIVE materialization of
  `liveQualifiedSet()`, holding exactly the ids that currently clear MinBond and are unslashed.
  Maintained at EVERY `bonded`/`slashed` mutation (the five sites below). It MUTATES mid-epoch.
- **`epochSet`** (`tagEpochSet`, the era-3 shape at `statehash.go:40, 101-103`): the FROZEN
  governing set, byte-for-byte the set `liveQualifiedSet()` produced at the last boundary, one
  `EncodeInt64(w)` leaf per member. It is UNCHANGED until the next boundary. This is exactly
  what era-3 already commits; era-4 does not alter its shape.

**Why NOT one shared keyspace (the RE-CERT Q2 refutation, verified against source).** The prior
rev collapsed `epochSet` and `qualified` into one keyspace and reconstructed "the frozen set
governing height `h`" from "the committed `qualified` root as-of the block at `epochStart`."
That is REFUTED. A floor box validating block `h` holds only the root committed IN block `h`
(`witness.go:12-13`); it has NO access to a historical root as-of `epochStart`, and the full-node
recompute (`era3validity.go:88-127`) produces the root over the POST-APPLY set at `h`, not a
retained boundary-height root. Two readings, both broken:
- **As-of the boundary block:** needs the `qualified` root as-of `epochStart`, not in the root
  at `h`. Unwitnessable.
- **The current `qualified` read at `h`:** available, but it is the LIVE set, mutated on every
  bond/slash/expiry since the boundary. Reading it as the governing quorum set is the mid-epoch
  set change `chain.go:1239-1241` declares "impossible" — an I3 churning-set unsoundness and an
  I1 sizing-set≠membership-set divergence. Verified: `requireEpochWeightQuorum` reads
  `effectiveEpochSet(h)` (`chain.go:2597`) and `RoundCatchupMet` reads `c.epochSet` directly
  (`chain.go:2631, 2638`); both consume the frozen snapshot, and `c.epochSet` is assigned ONLY at
  `rotateEpoch` (`chain.go:3131`) and `adopt` (`chain.go:3546`), never mid-epoch.

The two keyspaces are SEMANTICALLY different objects — a live filter versus a frozen snapshot —
that MUST diverge mid-epoch by construction. Collapsing them is unsound, not an optimization.

**Direction (a) alone does NOT meet era-4's goal.** Keeping only era-3's frozen `epochSet` with
no live `qualified` leaves `rotateEpoch` running the O(registry) `liveQualifiedSet()` scan over
`bonded` at every boundary. That is the exact wall era-4 exists to remove. The live `qualified`
accelerator is REQUIRED: it makes the boundary copy read a materialized set instead of rescanning
the registry. That is why era-4 commits a second, redundant keyspace.

**Maintenance sites — ALL FIVE, verified against source (the first Research cert REFUTED the
three-site list; the RE-CERT CERTIFIES this five-site enumeration as grep-complete).** Grep of
`core/chain/chain.go` for `bonded`/`slashed` writes and deletes inside `apply()` gives exactly
these five production sites (the doc's prior list named three and MISSED `2989` — an I1
weight-sum attack latent in its own list):

| # | Site | Line | Mutation | `qualified` maintenance action |
|---|---|---|---|---|
| 1 | **Squatter displacement** | `2989` | `delete(c.bonded, owner)` | `delete(qualified, owner)` — **THE MISSED SITE.** `owner` is a DIFFERENT id from the `id` written at 2995 (proof-beats-declaration, retest G3). If the displaced squatter was in `qualified` (an unproven-but-`>=MinBond` bonded entry), NOT deleting `qualified[owner]` here keeps a member `liveQualifiedSet()` drops → a different frozen epoch set → an I1 weight-sum attack. |
| 2 | **Register / renew / resize** | `2995` | `c.bonded[id] = r.Size` | after the write, if `r.Size >= MinBond && !slashed[id]` set `qualified[id] = r.Size`, else `delete(qualified, id)`. One key touch. |
| 3 | **TTL expiry** | `3008` | `delete(c.bonded, id)` | `delete(qualified, id)` (co-located with the T-3 bucket delete). Bounded by bucket size = O(payload) under the new cap. |
| 4 | **Slash: mark** | `3019` | `c.slashed[culprit] = true` | `delete(qualified, culprit)` (slashed ⟹ never qualified). |
| 5 | **Slash: evict** | `3020` | `delete(c.bonded, culprit)` | co-located with #4; the `qualified` delete covers both. One key touch per slash. |

There is no sixth per-key maintenance site in `apply()`. Two adjacent copy sites (NOT per-key
mutations) still need the completeness guards to force them: `cloneForDryRun`
(`era3validity.go:173-175` already copies `bonded`/`slashed`/`epochSet`) must ALSO copy the new
`qualified` map, and `adopt` (`chain.go:3525-3549`, already swaps `epochSet` at `3546`) must add
`c.qualified = t.qualified` (the reorg swaps the whole derived-state set; `t.qualified` was
already maintained by the per-block hooks during `t`'s replay). The `epochSet` copy/swap at those
sites is UNCHANGED from era-3. The era-3 completeness guards
(`TestDryRunCloneCopiesEveryAppliedField`, `TestStateFieldsAreClassified`) fail a test on a
forgotten field, so these are caught at unit, not in the field.

**The boundary is a COPY `epochSet := qualified` into the SEPARATE frozen keyspace, and it is a
DISTINCT, HEAVIER witness class — NOT O(payload)/zero-leaves (RE-CERT Q2 correction).** The
shipped `rotateEpoch` path (`chain.go:3130-3131`) does `set := c.liveQualifiedSet(); c.epochSet
= set` — a WHOLESALE replacement of the committed `epochSet` map every boundary. Era-4 keeps that
shape but sources the copy from the already-materialized `qualified` instead of the O(registry)
scan. The consequence the RE-CERT makes load-bearing: the boundary block's changed-leaf set is
the symmetric difference between last epoch's `epochSet` and this epoch's `qualified` — the
epoch-ACCUMULATED delta, O(boundary-delta), NOT one block's payload. A floor box proving the
boundary post-state root owes O(boundary-delta) proofs. This is a GENUINE, UNREMOVED cost of E-2,
not an artifact of representation, and it does NOT collapse to zero by any keyspace choice — the
frozen snapshot and the live filter are different objects that must diverge mid-epoch.

State the boundary block honestly as its own witness class, and bound it:
- `boundary_witness ≤ boundary_delta × SProofMax ≤ RegCap × EpochBlocks × SProofMax`.
- With the measured single-key proof sizes (max 1,474 B at 1M leaves; theoretical worst case
  ~8,830 B — both well under `SProofMax = 16 KiB`, `witness_bound.go:78`) and `EpochBlocks = 8`
  (`daemon.go:1729`), this fits the 2 GB floor box for `RegCap ≤ 16,384` (the derivation is in
  section 9). The boundary is ONE block per epoch, so the amortized cost is small, but the block
  itself is heavier and MUST be witnessed as such.

The mechanism, per keyspace:
- **`qualified`** (`Key(tagQualified, id) → EncodeInt64(weight)`) is maintained live (the five
  sites above), so at any height it IS the current qualified set. Its mutations are already in
  each transition's read-set.
- **`epochSet`** (`tagEpochSet`, the era-3 frozen shape) holds the governing set. It changes ONLY
  at the boundary (the copy) and at `adopt` — never mid-epoch. Mid-epoch quorum readers keep
  reading it, byte-identical to era-3.
- At a boundary `h % EpochBlocks == 0`, `rotateEpoch` sets `epochStart := h` (the O-1 scalar) AND
  copies `epochSet := qualified`. The changed-leaf set is `{tagEpochStart}` PLUS the `epochSet`
  symmetric-difference delta = O(boundary-delta). The scalar is O(1); the set copy is the heavier
  part.
- The #506 / era-3 lock-in tallies (`chain.go:3144-3179`) that sum weight over the frozen set
  read the SAME frozen `epochSet` as era-3, byte-identical IFF the boundary copy equals era-3's
  `liveQualifiedSet()` — which it does IFF the `qualified` maintenance invariant holds (the Q1
  gate). No new read shape for the mid-epoch quorum.

**Q5 coupling — the recovery branch must agree with the frozen set (RE-CERT Q5).** `effectiveEpochSet`
(`chain.go:1243`) reads the frozen `epochSet` at every height EXCEPT the operator recovery boundary,
where it returns a fresh `liveQualifiedSet()` recompute (`chain.go:1194-1196` notes
`liveQualifiedSet` feeds BOTH rotation and the recovery re-base). Era-4 keeps the NON-recovery
branch reading the frozen `epochSet` (it must, per Q2) and the recovery branch running the
`liveQualifiedSet()` recompute. The two PRODUCERS of the recovery set — the materialized `qualified`
(now the boundary source) versus the recomputed `liveQualifiedSet()` — MUST agree at the recovery
boundary, or the divergence is named. Confirm they agree, or name it. This rides with the Q2
correction; it is not a separate open item.

**Intra-block ordering is preserved and load-bearing (Research confirmed).** Era-3 applies,
within `apply()`, in this order: entries → revocations → unrevocations → bond registrations
(incl. displacement at 2989) → TTL expiry (3005-3013) → slashes (3017-3021) → maturity latch →
rotate-LAST (gated at `chain.go:3046`). The five maintenance hooks run at each site in THIS
order, and the boundary copy `epochSet := qualified` stays LAST. A register at 2995 followed by
a TTL delete at 3008 of the same id in one block yields a different `bonded` than the reverse;
the same is true of `qualified`. Any reordering is a rule change and must be a covered ordering
ablation.

Costs:
- **Apply:** O(1) per bond/slash/expiry to maintain `qualified`. Boundary apply is O(1) — the
  copy `epochSet := qualified` reads the already-materialized set, no O(registry) scan. Contrast
  era-3: O(registry) `liveQualifiedSet()` scan at every boundary. THIS is the era-4 apply win.
- **Commit:** TWO committed keyspaces — the live `tagQualified` (new) and the frozen `tagEpochSet`
  (era-3 shape, retained) — plus the epoch scalar (`tagEpochStart`, O-1) → new field-tag for
  `qualified`, new-era format. `qualified` is REDUNDANT with `bonded`+`slashed`+MinBond (it is
  their materialized filter), so the state root now commits a derived view. PE ruled the
  redundancy an ACCEPTABLE trade — the price of dropping the boundary scan — ON ONE CONDITION:
  the drift-guard below ships as a HARD GATE, not a comment.
- **Witness (per block class):** ordinary and TTL-firing blocks are O(payload); the per-mutation
  `qualified` deltas are already in each transition's read-set. The EPOCH-BOUNDARY block is a
  DISTINCT, HEAVIER witness class: its changed-leaf set is the `epochSet` symmetric-difference
  delta = O(boundary-delta), bounded by `RegCap × EpochBlocks × SProofMax` (section 9). Do NOT
  assume the boundary is O(payload).

**Drift-guard (the condition PE and Research both make load-bearing — ships as a hard gate,
ablated per site).** A recording harness that wraps the committed maps, replays a
branch-covering corpus, and asserts after EVERY block:

    qualified == filter(bonded, slashed, MinBond)

The corpus MUST exercise displacement (2989), renew, slash, and expiry IN THE SAME block. The
guard ABLATES PER SITE: remove each of the five maintenance hooks in turn and confirm the guard
goes RED — specifically, removing the `2989` `delete(qualified, owner)` hook MUST go red, or the
correction (adding that site) is itself unverified (Research R1 holds the correction until the
guard reddens on that exact site). A green guard with no demonstrated per-site red is a comment
that compiles. This is the same shape as the increment-3 read-set drift-guard.

**E-3 — sum-trie weight commitment (OPTIONAL enhancement, not required).**
pokt SMT ships a SUM trie (`VerifySumProof`) that commits a running total. Committing
`qualified` as a sum trie would let a floor box verify the TOTAL frozen weight (the quantity
the I1 super-quorum arithmetic sums) with one proof, instead of summing per-id leaves. This
is attractive because the weights feed the finality predicates (I1), and the era-3 cert
already flagged the bonded/epochSet weights as consensus-SAFETY-load-bearing
(`era3validity.go:19-24`). BUT it is a distinct accumulator, a distinct value encoding, and
its own cert. Recommend: E-2 first (correctness), E-3 as a follow-on ONLY if a floor box's
per-boundary weight-sum witness proves too heavy in measurement. Flag E-3 as a NEW
committed structure needing its own soundness cert; do not bundle it into the era-4
minimum.

### Rule-equivalence risk points (rotation)

| Risk | Assessment |
|---|---|
| Is `qualified` (materialized) always equal to `liveQualifiedSet()` (computed)? | This is the WHOLE equivalence claim, and Research REFUTED the doc's prior three-site list. It holds ONLY if ALL FIVE `bonded`/`slashed` mutation sites (2989, 2995, 3008, 3019, 3020 — table above) update `qualified` in the same apply. The missed `2989` squatter-displacement delete kept a member `liveQualifiedSet()` drops = a DIFFERENT epoch set = an I1 weight-sum attack. **RESEARCH-GATED**; the recording drift-guard (above) is the instrument, and it MUST ablate the `2989` hook to RED specifically, per Research R1. |
| `MinBond` is a config, not committed | era-3's filter reads `c.cfg.MinBond`. Research and PE CONFIRMED `MinBond` is a genesis `Config` field (`chain.go:130`), set once at `New`, with NO `c.cfg.MinBond =` assignment anywhere in core/cmd — fixed-per-chain, so this risk is DORMANT, not live. Residual: if `MinBond` is ever made governance-tunable, that is a SEPARATE consensus-rule item that reopens this (materialized `qualified` would need re-derivation at the config change). Named, not open today. |
| Ordering: era-3 applies bonds → TTL → slashes → rotate-LAST (gated at `chain.go:3046`) | Era-4 maintains `qualified` under the SAME intra-block ordering so the boundary copy sees the same set era-3 would have computed. The five maintenance hooks run at each site in the existing order; the boundary copy `epochSet := qualified` stays LAST. Research confirmed the order is load-bearing (a register-then-TTL-delete of the same id in one block yields a different set than the reverse). Any reordering is a rule change and MUST be a covered ordering ablation. |
| The #506 / era-3 lock-in tallies (`chain.go:3144-3179`) sum weight over the frozen `set` | They read the frozen `epochSet`, byte-identical to era-3. The boundary copy `epochSet := qualified` equals era-3's `liveQualifiedSet()` IFF the maintenance invariant holds. Same gate as row 1. No new read shape. |
| Boundary changed-leaf set (RE-CERT Q2, REFUTED as prior-written) | The single-shared-keyspace + pointer advance is REFUTED: a floor box holds only the CURRENT root, so a live-mutating shared keyspace is either unwitnessable as-of the boundary or leaks mid-epoch changes into the quorum set (an I1/I3 divergence). The sound direction: `epochSet` STAYS its own FROZEN materialized keyspace (era-3 shape); the boundary COPIES `qualified` → `epochSet`. The boundary block's changed-leaf set is then O(boundary-delta), a DISTINCT heavier witness class bounded by `RegCap × EpochBlocks × SProofMax`. NOT O(payload)/zero-leaves. The one-vs-two keyspace question is settled: it MUST be two. |
| `effectiveEpochSet` recovery re-base (RE-CERT Q5 coupling) | `liveQualifiedSet()` also feeds the #535 recovery re-base (`chain.go:1194-1196` note; `effectiveEpochSet` at `1243`). The NON-recovery branch reads the frozen `epochSet` (it MUST, per Q2). The recovery branch runs a fresh `liveQualifiedSet()` recompute. The two PRODUCERS of the recovery set — the materialized `qualified` (the boundary copy source) vs the recomputed `liveQualifiedSet()` — MUST agree at the recovery boundary, or the divergence is NAMED. This rides INSIDE the Q2 correction. The recovery-BOUNDARY witnessability is still SCOPED OUT (section 6). |

---

## 6. Scoped-out: the `effectiveEpochSet` recovery boundary (a separate gated item)

The `#535` machinery reads TWO non-committed observables. Era-4 takes ONE and explicitly
DEFERS the other. Both seats confirmed this split is the honest scope line.

### O-1 — commit `epochStart`. IN era-4 (sound and cheap; Research CERTIFIED narrowly).

`epochStart` is a scalar (`h` of the last rotation). Committing it is one scalar leaf
(`tagEpochStart`), mirroring the era-3 scalar leaves (`everMature`, `gateHeight`, …). Research
grep-confirmed `epochStart` is read ONLY at `chain.go:1932` (the `Regime()` status snapshot),
written at `3125` (rotateEpoch) and `3547` (adopt); NO validity or quorum predicate reads it,
and `modelcheck_state_completeness_test.go:109-113` independently classifies it observable
("its ONLY reader is Regime()... losing it misreports restore health, never validity").
Therefore committing it changes NO quorum decision. It is sound in isolation, cheap, removes
one observable, and it doubles as the E-2 epoch pointer (section 5). KEEP it in era-4.

### The `effectiveEpochSet` recovery boundary — SCOPED OUT of era-4-minimum.

Committing `epochStart` does NOT close the observable that actually blocks a floor-box quorum
witness. That observable is `effectiveEpochSet` (`chain.go:1243-1249`), read by
`requireEpochWeightQuorum` (`chain.go:2597`) — a REAL quorum decision. At the recovery boundary
`effectiveEpochSet(h)` returns `liveQualifiedSet()` (a whole-map scan), gated on
`c.cfg.LivenessRecoveryHeight` — an OPERATOR flag, coordination config (`chain.go:227-246`),
NOT committed state. Two distinct problems remain after O-1, per Research Q4:

1. **The scan.** At the recovery boundary the quorum re-bases against `liveQualifiedSet()`,
   O(registry). E-2's materialized `qualified` COULD serve this in O(payload) — but only if
   `effectiveEpochSet` is rewritten to read `qualified`. That is a further consensus-rule change.
2. **The operator flag.** Whether `h == LivenessRecoveryHeight` is NOT a function of committed
   state. A floor box holding only the two roots cannot know an operator armed a recovery at
   `h`, so it cannot independently determine WHICH set governs `h` even if the scan is solved.

**Do NOT design a fix here and do NOT smuggle a `LivenessRecoveryHeight` rule change into
era-4.** The direction is an OPEN DECISION for Andrew, with a real trade:
- **(a) Commit the recovery directive into state** (a committed, reorg-stable recovery-height
  leaf) AND route `effectiveEpochSet` through `qualified` — both consensus-rule changes. Result:
  fully TRUSTLESS recovery-boundary witnessing. Cost: more scope, a fresh Research gate.
- **(b) Posture-bound the floor box there** (O-2): declare the floor box witnesses
  transition-validity (publish/takedown/unrevoke/bond-reg/slash/TTL/rotation-set) but does NOT
  independently re-derive the recovery-boundary quorum re-base, and NAME that trust surface.
  Result: a bounded, named trust surface. Cost: not fully trustless at recovery.

Either is defensible; the choice is the human's. **era-4-minimum = the two whole-map
transitions (T-3, E-2) + the new reg-cap validity rule (section 4/9) + O-1 (commit
`epochStart`).** The recovery-boundary observable is a SEPARATE, gated follow-on — Research
R2. It is NOT solved by O-1 and MUST NOT be represented as solved.

---

## 7. Reuse: the merged witness machinery applies unchanged

Confirmed against `origin/main` `0984db4`:

| Component | Site | Era-4 impact |
|---|---|---|
| R4 three-valued accessor (`Resolve`, `Outcome`, `Result`) | `core/statehash/witness.go` | UNCHANGED. Every era-4 read is a single-key present/absent query — exactly what `Resolve` serves. The T-3 empty-bucket case is a `ProvenAbsent`; the T-3 occupied-bucket case is a `ProvenPresent(bucketRoot)`. |
| R3 DoS bound (`SProofMax`, `C_block`, shape gate) | `core/statehash/witness_bound.go` (`SProofMax` at `:78`, `CBlock` at `:202`) | UNCHANGED PROVIDED the read-set stays bounded. `C_block = len(read-set)·SProofMax` re-derives per block; for the ordinary and TTL-firing blocks the read-set is (transition keys) + (T-3 bucket keys) + (E-2 qualified-delta keys), all payload-bounded. For the EPOCH-BOUNDARY block the read-set ALSO includes the `epochSet` symmetric-difference delta = O(boundary-delta), bounded by `RegCap × EpochBlocks × SProofMax` — a heavier but bounded `C_block`. The shape gate pins the bundle to exactly that read-set. The per-proof cap is NOT the binding constraint (measured max 1,474 B ≪ 16 KiB); the COUNT of proofs is. |
| `ReadEntry` / `QueryKind` (present/absent) | `witness_bound.go` | REUSED. The bond-family `map[k],ok` gap the increment-3 doc flagged (both present-with-value AND absent are acceptance-relevant) recurs for the T-3 bucket and E-2 qualified reads; era-4 must model them with the SAME two-`QueryKind` entries, not one. Carry that fix forward. |
| Post-apply root recompute | `era3validity.go:88` (`postApplyRoots`/`cloneForDryRun`) | EXTENDED, not replaced. Era-4 adds the new committed keyspaces (`tagDueBucket`, `tagQualified`, `tagEpochStart`) to `stateRootLeaves` and to `cloneForDryRun`; the `TestDryRunCloneCopiesEveryAppliedField` and `TestStateFieldsAreClassified` completeness guards force both, so a forgotten field fails a test, not a field run. |
| Delivery any-of-N + `MsgGetWitness` side-channel | increment-3 Part B (not yet merged) | UNCHANGED. Era-4 witnesses ride the same side channel; the frozen-then-superseded block format still carries no witness field. |

The design constraint that makes this reuse hold: **the era-4 FLOOR-BOX WITNESS read-set per
block must be BOUNDED — O(payload) for ordinary and TTL-firing blocks, O(boundary-delta) for the
once-per-epoch boundary block.** Every option above is chosen to keep it so. If any option forced
an O(registry) read-set, the R3 `C_block` ceiling would balloon to O(registry)·SProofMax and the
whole point would be lost — that is the acceptance test for every era-4 mechanism. The boundary
block is the ONE class that is not O(payload); its O(boundary-delta) read-set is bounded by the
new RegCap (`RegCap × EpochBlocks × SProofMax ≤ 2 GB` for `RegCap ≤ 16,384`, section 9), so it
still fits the floor box — but it MUST be witnessed as its own class, not folded into O(payload).

**Clarity the PE ruling requires (finding 3 / LOW but dangerous if built on): O(payload) is the
FLOOR-BOX WITNESS read-set, NOT the full-node recompute.** The full node validates by
recomputing the WHOLE root every block: `validateEra3Roots` → `postApplyRoots`
(`era3validity.go:119-127`) deep-clones the whole chain (`cloneForDryRun`, every map copied)
and `StateRoot` → `stateRootLeaves` scans `c.bonded` and every other committed map
(`statehash.go:98`). era-4 ENLARGES that scan (three new tags). So the full-node validation
cost stays O(registry) per block and era-4 makes it slightly HEAVIER, not cheaper. The
O(payload) claim is TRUE for the witness read-set (the floor box is the design's reason to
exist) and FALSE for the full-node recompute. Named here so no reader later claims era-4 made
full-node validation cheaper, or builds a full-node optimization on the wrong invariant.

Per-block-class witness verdict for the FLOOR BOX (PE Judge #4; corrected by RE-CERT Q2):
- **Ordinary block:** transition keys + T-3 {old,new} bucket touches on any reg + E-2
  per-mutation `qualified` deltas. O(payload). Holds.
- **TTL-firing block:** reads the bucket at `h`, size O(bucket). O(payload) ONLY IF the new
  bucket-size validity cap exists (section 9). Without it, O(registry).
- **Epoch-boundary block: a DISTINCT, HEAVIER witness class — NOT O(payload).** Its apply is
  O(1) (the copy reads the materialized `qualified`), but its changed-leaf set is the `epochSet`
  symmetric-difference delta = O(boundary-delta). Bounded by `boundary_delta × SProofMax ≤ RegCap
  × EpochBlocks × SProofMax`, which fits the 2 GB box for `RegCap ≤ 16,384` (section 9). The prior
  `{tagEpochStart}`/zero-leaves claim is WITHDRAWN — the RE-CERT REFUTED the shared-keyspace
  pointer that made it. The boundary is ONE block per epoch; witness it as its own class, do not
  fold it into the O(payload) claim.

---

## 8. Scope confirmation: era-4 is a NEW era

- Era-4 mints a NEW `BlockVersion` = 5 (era-3 is `BlockVersionStateRoot` = 4; the next free
  value is 5). `versionSupported` extends to `<= 5` in the same release, exactly as era-3
  did for 4 (`chain.go:327`, cert Q7 hard-fork reasoning).
- Era-2 (v2) and era-3 (v4) blocks stay **byte-identical** under their versions. Era-4 adds
  new committed field-tags and new transition representations that fire ONLY for v5 blocks
  at/above the era-4 activation boundary. The era-3 predicate (`validateEra3Roots`) and the
  frozen 18-field set are untouched for v4 blocks.
- Activation follows the era-3 pattern: a `regVersion >= 5` super-quorum lock-in
  (`Era4ActivationHeight` / `era4LockedIn` / `era4Height`), one epoch of notice, minted
  only after the predicate exists (the 2a→2b→2c sequencing era-3 used).

**Sequencing note (PE Judge #5, LOW — pick one on purpose).** The era-3 build widened
`versionSupported` to 4 in step 2a, BEFORE the 2b predicate existed. That opened a window
where a v4 block DECODED and was accepted with no root predicate (safe only because 2c had not
flipped minting and the block still had to pass era-2 rules and gather quorum). Era-4 must
choose deliberately: **(a)** widen `versionSupported` to `<= 5` in the 2b release
(predicate-FIRST), closing the window; or **(b)** explicitly accept the same interim window
era-3 accepted. PE recommends being deliberate and NOT compounding the still-open era-3 finding
(`versionSupported` already admits v3 = silent mis-validation, per
`ruling-era3-committed-state-root-format.md`). This doc recommends (a) predicate-first, since it
costs nothing extra and closes a window rather than adding a second one.

---

## 9. Gate classification — what needs whom

### Research-gated (soundness / equivalence — do NOT build or assert on these)

The RE-CERT verdict is **STILL GATED**: two substantive items remain (Q2 design correction routed
back, R3 cap value OPEN). Three original items are CERTIFIED (the five-site enumeration, the
canonical id-list pin, the honest new-cap-rule statement). Status per item:

1. **TTL equivalence (Q4 in the RE-CERT — CERTIFIED in design, guard owed):** that the T-3
   due-bucket `D(id) = regH+ttl+1` representation expires EXACTLY the era-3 id set at EXACTLY the
   era-3 heights, including renew-resets, ttl==0, slash-before-due, AND the NEW dual-source
   (`bondRegHeight` ⟺ bucket) consistency era-4 introduces. Arithmetic CERTIFIED against
   `chain.go:3005-3013`; the dual-source drift-guard is a build artifact still OWED (must go red
   under ablation). (Consensus-rule equivalence; I5 determinism.)
2. **Rotation — the load-bearing correction (Q1 enumeration CERTIFIED; Q2 boundary REFUTED as
   prior-written, corrected in rev 2):** the five-site enumeration (2989, 2995, 3008, 3019, 3020)
   is grep-complete and CERTIFIED. BUT the single-shared-keyspace + pointer boundary is REFUTED —
   it does not preserve mid-epoch immutability of the frozen set (an I1/I3 divergence). Rev 2
   adopts the sound direction: `epochSet` STAYS its own FROZEN materialized keyspace; the boundary
   COPIES `qualified` → `epochSet` and is a DISTINCT O(boundary-delta) witness class (section 5).
   Lifts on the corrected boundary + the per-site-ablated drift-guard (must redden on `2989`).
3. **Bucket-commitment completeness (Q4-encoding in the RE-CERT — CERTIFIED):** a single
   next-bucket non-membership proof soundly discharges "nothing else was due at h." CERTIFIED
   against the whole-set exclusion primitive (`witness.go:65-72`) PROVIDED the carried id-list is
   pinned canonical (see the format list below). The bounded LENGTH still waits on the cap value.
4. **Recovery boundary (Q5 — SCOPED OUT of era-4, CLEAN):** O-1 (commit `epochStart`) is CERTIFIED
   narrowly (changes no quorum decision) and stays IN era-4. The `effectiveEpochSet` recovery
   re-base is a SEPARATE gated item (section 6) — do NOT design it here. The Q5 coupling (recovery
   branch's `liveQualifiedSet()` recompute must agree with the frozen `epochSet` at the recovery
   boundary) rides with the Q2 correction (section 5).
5. Any sum-trie weight commitment (E-3), if pursued.

### New security parameter (a proof depends on it — Research-gated NUMERICALLY, before build)

- **The per-height due-bucket / per-block fresh-registration size cap (RegCap). This is a
  genuinely NEW consensus VALIDITY rule + a security parameter, NOT a consequence of #506 — both
  seats REFUTED the doc's prior "confirm it composes" as resolving AGAINST the optimistic
  reading.** #506's R-rule (`chain.go:1579-1590`, `regMinInterval` `chain.go:3288-3300`) bounds
  per-IDENTITY re-registration frequency and EXEMPTS first-time registrations. It does NOT bound
  distinct-identity registrations per block. The only per-block reg-volume bound is
  `MaxBondRegBytesPerBlock`, applied SOLELY at the proposer block-BUILD path (`chainrole.go:798`,
  value `2 << 20` set at `node.go:270`, `0 = unbounded`) — a daemon flag, NOT a validity rule; an
  adversarial proposer is not bound by it. So N distinct FRESH identities can register at one
  height `r`, all landing in bucket `D = r+ttl+1`, making that expiry O(N) = O(registry) and
  ballooning `C_block`. **era-4 MUST mint a validity-enforced per-block/per-due-height cap.**

  **Measured bracket (Tester, `origin/main @ 0984db4`, pokt-network/smt@v1.0.0):** `λ_H ≤ RegCap
  ≤ 16,384 ids/block`.
  - **Upper bound `RegCap ≤ 16,384` — DERIVED and TIGHT.** From the boundary-witness fit (section
    5): the epoch-boundary block's witness must fit the 2 GB floor box, so `RegCap × EpochBlocks ×
    SProofMax ≤ 2 GB`. With `EpochBlocks = 8` (`daemon.go:1729`) and `SProofMax = 16 KiB`
    (`witness_bound.go:78`): `RegCap ≤ 2 GiB / (8 × 16 KiB) = 16,384` (128 KiB of witness per
    boundary id). The measured single-key proof sizes (max 1,474 B at 1M leaves; theoretical worst
    case ~8,830 B) are well under `SProofMax`, so the per-proof cap is NOT the binding constraint —
    the COUNT of proofs is, which is exactly what RegCap bounds.
  - **Lower bound `λ_H` — the one input NOT pinned in canon.** `λ_H` is the honest fresh-registration
    arrival rate per block. It is declared "owed input, measured not assumed" (`docs/design/m0.md:487`,
    `docs/design/owned-residuals.md:392`) and it CANCELS out of the maturity theorem
    (`docs/decisions.md:662` — "the honest arrival rate `λ_H` cancels ... leaving a pure
    budget-vs-threshold inequality"). So it is not pinned as a constant anywhere I can compose at
    desk. RegCap must be `≥ λ_H` or a legitimate onboarding wave is rejected.

  **Its VALUE is a security parameter (mirrors R3 `SProofMax`), certified numerically before
  build.** The upper bound is settled at desk. The lower bound `λ_H` is the single input that
  cannot be closed at desk from existing canon.

  **A question FOR Research, NOT asserted here as settled:** can `λ_H` be UPPER-bounded at desk
  instead of measured? The honest per-block fresh-registration volume is already ceilinged by the
  proposer reg-byte budget (`MaxBondRegBytesPerBlock = 2 << 20`, `node.go:270`) and the target
  scale — a fresh registration carries a bond proof of bounded size, so 2 MiB/block admits a
  bounded id count. IF that ceiling upper-bounds `λ_H` below 16,384, then a precise measured
  `λ_H` is NOT required to pick a safe RegCap in the bracket. This is a composition for Research to
  certify or refute; do not build on it as settled. (Caveat: `MaxBondRegBytesPerBlock` is a
  proposer-BUILD flag, not a validity rule, so it bounds the HONEST proposer's output, not an
  adversary — the argument bounds `λ_H` for cap-sizing, it does NOT replace RegCap as the validity
  bound.)

  This cap and the canonical carried-list shape gate are ONE gate (the gate can only bound the
  list length if the cap bounds the bucket) — certify them together.

### New-era / format decisions (a new BlockVersion, new committed tags — human ratifies)

- `BlockVersion` = 5, `versionSupported <= 5`, `Era4ActivationHeight`/lock-in scalars. Plus the
  sequencing choice (section 8): predicate-first widening (recommended) vs the era-3 interim window.
- New committed field-tags: `tagDueBucket`, `tagQualified` (new live keyspace), `tagEpochStart`
  (O-1). `tagEpochSet` is RETAINED from era-3 unchanged (the frozen keyspace). Each new tag enters
  `stateRootLeaves`, `stateRootTags`, `cloneForDryRun`, `adopt`, and the completeness guards.
- **`qualified` and `epochSet` are TWO keyspaces — SETTLED by the RE-CERT (Q2), no longer an open
  question.** The RE-CERT REFUTED collapsing them into one: a floor box holds only the current
  root, so a shared live-mutating keyspace either is unwitnessable as-of the boundary or leaks
  mid-epoch changes into the quorum set (an I1/I3 divergence). Era-4 commits BOTH: a live
  `tagQualified` (boundary-computation accelerator) and the frozen `tagEpochSet` (era-3 shape).
  The boundary block is a distinct O(boundary-delta) witness class (section 5). This remains a
  format decision the human ratifies, but the direction is fixed by soundness.
- The bucket value encoding (nested SMT root vs carried-MTH — section 4 variant b recommended).
- **The canonical carried id-list encoding (Research Q3 / R4 — a hard-fork parameter).** For
  variant (b): the id list MUST be sorted ascending + deduplicated + unpadded, and the shape
  gate MUST reject any non-canonical or padded list. Without the pin the MTH is malleable.

### Buildable once gated (representation, no new rule)

- The `stateRootLeaves` / `cloneForDryRun` / `adopt` extensions for the new tags (completeness
  guards already force correctness; `adopt` must add `c.qualified = t.qualified`).
- The `ReadEntry`/`QueryKind` modeling of the new keys (reuse increment-2 machinery).
- The recording drift-guards, TWO of them, both ablated:
  - **`qualified` maintenance guard** — asserts `qualified == filter(bonded, slashed, MinBond)`
    after every block; ablates per site, MUST redden on the `2989` hook (Research R1).
  - **T-3 dual-source guard** — asserts `bucket-membership(id) ⟺ (bondRegHeight[id]+ttl+1 == D
    AND bonded[id] present)` after every block, plus byte-identical StateRoot vs an era-3
    replay; ablates on a missed old-bucket delete on renew (Research R1/Q1).
  The ablation harness shape is already established by increment-3.

---

## 10. Every place rule-equivalence is at risk (consolidated)

A representation change becomes a SMUGGLED rule change at any of these. Each MUST be a
covered ablation and is flagged to Research:

1. **Renew resets the TTL clock** — T-3 must move the id from its old due-bucket to the new
   one in the same apply; a missed old-bucket delete expires the id EARLY (a different
   height). Sharpest TTL hazard.
2. **`qualified` maintenance completeness — ALL FIVE sites (2989, 2995, 3008, 3019, 3020)** —
   every site that mutates `bonded`/`slashed` must update `qualified`; a missed site freezes a
   DIFFERENT epoch set (an I1 weight-sum attack). The missed `2989` squatter-displacement delete
   was Research's REFUTATION of the prior three-site list. Sharpest rotation hazard; the guard
   must redden on the `2989` hook specifically.
3. **T-3 dual-source drift (`bondRegHeight` AND the bucket)** — era-4 keeps both (era-3 had one
   source); on expiry delete both, on renew reset+move both. A drift is a divergence era-3
   cannot have. Its own drift-guard (section 4/10-guards).
4. **Intra-block ordering** — era-3 applies bonds → TTL → slashes → rotate (rotation LAST, gated
   at `chain.go:3046`). Era-4 maintenance must run at each site in that ORDER and keep the
   boundary copy `epochSet := qualified` last. Ordering ablation owed.
5. **`MinBond` config-vs-committed** — DORMANT: `MinBond` is fixed-per-chain (both seats
   confirmed no mid-chain mutation). Reopens only if made governance-tunable.
6. **ttl==0 and slash-before-due edge cases** — both are no-ops in era-3 that must remain
   post-state-root-identical in era-4.
7. **The boundary block is a DISTINCT witness class (RE-CERT Q2)** — `epochSet` STAYS its own
   FROZEN keyspace; the boundary COPIES `qualified` → `epochSet`, so the boundary changed-leaf set
   is O(boundary-delta), NOT O(payload). Collapsing `epochSet` and `qualified` into one keyspace
   is REFUTED (an I1/I3 divergence). Not rule-equivalence: a witness-cost class that MUST be
   certified bounded (by RegCap), not assumed O(payload).
8. **The new bucket-size / fresh-reg cap (RegCap)** — this is NOT rule-equivalence; it is a NEW
   validity rule era-4 mints. Its ABSENCE breaks the read-set bound (O(registry) TTL block). Its
   VALUE is a security parameter: upper bound `≤ 16,384` derived, lower bound `λ_H` un-pinned;
   certified numerically before build (section 9).
9. **`epochStart` commit (O-1)** — sound alone (no predicate reads it). The `effectiveEpochSet`
   recovery-boundary re-base is SCOPED OUT (section 6), a separate gated item, NOT solved by O-1.

Items 1–6 change only the committed REPRESENTATION and proof shape — WHO expires and WHO
qualifies is unchanged, IFF each ablation holds. Item 7 is a witness-cost class (the boundary
block), corrected by the RE-CERT to be non-O(payload) but bounded. Item 8 is a genuine NEW rule
(stated honestly, not smuggled). Item 9 is scoped out. Every point is gated, not assumed.

---

## 11. Recommended path

The RE-CERT verdict is **STILL GATED**. Two items are the substantive gate: the Q2 design
correction (routed back — this rev is that correction) and the R3 cap VALUE (a numeric
certification owed before build). It lifts to CERTIFIED on FIVE conditions (from the RE-CERT's
"What would lift the gate"):

1. **Q2 correction (R1) — the load-bearing one, addressed in this rev.** Keep `epochSet` a FROZEN
   materialized keyspace (era-3 shape); maintain `qualified` live as a boundary-computation
   accelerator; the boundary COPIES `qualified` → `epochSet` and is a DISTINCT witness class whose
   changed-leaf set is O(boundary-delta), bounded by `RegCap × EpochBlocks × SProofMax` — NOT
   claimed O(payload)/zero-leaves. Confirm the recovery branch's `liveQualifiedSet()` recompute
   agrees with the frozen `epochSet` at the recovery boundary (the Q5 coupling). Section 5.
2. **Q3 cap VALUE (R3) — the item that cannot be closed at desk.** A certified NUMERIC value for
   the per-height bucket-size / fresh-reg cap, derived from the honest-arrival measurement composed
   against the R3 `C_block` budget, with confirmation the cap composes with the #506 admission path
   to bound the read-set. Upper bound `≤ 16,384` is derived (section 9); the lower bound `λ_H` is
   the un-pinned input. The desk question — whether `λ_H` can be upper-bounded via the proposer
   reg-byte budget — is FOR Research to certify (section 9), not asserted here.
3. **The two drift-guards built and ablated RED (R4):** the `qualified` maintenance guard (per-site,
   `2989` reddens specifically); the T-3 dual-source guard (renew old-bucket-delete reddens). Both
   specified in design, both build artifacts still owed.
4. **The T-3 equivalence assertion:** byte-identical post-apply StateRoot vs an era-3 replay over a
   corpus covering renew-reset, ttl==0, slash-before-due.
5. **R2 direction (recovery boundary):** ratified by the human — committed recovery-height (a fresh
   gate) or the O-2 posture bound. SCOPED OUT of era-4-minimum (section 6); not a blocker for the
   other items.

The five-site enumeration, the canonical id-list pin, the honest new-cap-rule statement, O-1, the
`MinBond` dormancy, and the intra-block ordering are CERTIFIED closed by the RE-CERT.

The ordered path:

1. **This rev-2 doc** (folds in the RE-CERT; condition 1 is now the sound direction, conditions 3
   and 4 are specified in design, condition 5 is scoped out; condition 2's VALUE is still owed
   numerically, and the λ_H desk question is flagged for Research) →
2. **RE-CERT round 2 with Research** — re-route the corrected doc as ONE era-4
   equivalence-and-completeness question, to lift STILL-GATED → CERTIFIED. The doc now carries the
   corrected FROZEN-`epochSet` boundary (distinct O(boundary-delta) witness class), the measured
   RegCap bracket with the λ_H question, both drift-guards, the canonical-list pin, and the honest
   new-cap-rule statement. The one item Research still needs numerically is the RegCap VALUE (plus
   a ruling on the λ_H desk-bound question). The Builder shapes the question; the Researcher
   certifies; the human ratifies. →
3. **Format veto-gate to Andrew** — on a CERTIFIED verdict, the new-era/format decisions go to
   the human: `BlockVersion = 5` + `versionSupported <= 5` + the new committed tags (`tagDueBucket`,
   `tagQualified`, `tagEpochStart`; `tagEpochSet` retained) + **the new per-block fresh-registration
   cap validity rule (RegCap)**. The one-vs-two keyspace question is SETTLED by soundness (two).
   This is a veto-gate (opens a new BlockVersion, mints a new consensus rule, re-represents I1/I5
   transitions). Whether to open era-4 now vs defer behind the era-3 residuals is Andrew's scope
   call (PE). →
4. **Build 2a→2b→2c** behind the completeness guards and BOTH recording drift-guards (each
   ABLATES red). Predicate-first `versionSupported` widening recommended (section 8).

Do NOT build any era-4 mechanism before the equivalence certification is LIFTED to CERTIFIED —
a field run confirms a fix, it never discovers a consensus invariant (`build-process.md` #6).
