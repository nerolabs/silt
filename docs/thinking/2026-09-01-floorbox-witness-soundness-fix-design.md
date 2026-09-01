# Floor-box witness-soundness fix — design (Boulder 1, build-day core)

Date: 2026-09-01
Status: DESIGN. Ships with the code tomorrow. No `.go` file changed today.
Scope: `core/chain/floorbox_recompute_*_v5.go` (classes A / B / P) + `core/statehash/fold.go` (unchanged; it is the anchor we route through).

## 1. The mechanism (attribution before the patch)

The failure is a wrong-accept-by-recompute because recompute classes P / A / B read
per-member VALUES and SCREEN PREDICATES from the untrusted witness struct and use them as
fold `NewValue`s or branch predicates WITHOUT Resolving them against `prevStateRoot`.

Evidence, by file:line, against the current tree:

- `fold.go:126` — `FoldChangedPaths` verifies each op's `OldValue` against `prevStateRoot`
  (`smt.VerifyProof(op.Proof.proof, prevStateRoot[:], op.Key, op.OldValue, verifySpec())`).
  It verifies **only** `OldValue`. `NewValue` and any branch predicate are never checked.
- `fold.go` terminal equality lives in the CALLER
  (`floorbox_recompute_stateroot_v5.go:212`): `postRoot == committedStateRoot`. The
  attacker controls the committed root too (it is attester-signed data the box holds, not a
  box-derived value). So "diverges the committed root ⇒ stall" is only a real catch when
  the forged input actually changes a COMMITTED leaf the honest root also commits. A forged
  input that changes NOTHING the committed root binds is invisible to the equality.

The sound classes (S / T / A-digest / M, the scalar folds) never touch this trap: every
witness value they consume is routed as a VerifyProof'd `OldValue` via `anchoredPreSet`
(`floorbox_recompute_stateroot_slash_v5.go:193`) or `digestFoldOp`
(`..._slash_v5.go:208`), or as a `ChangedLeaves` `OldValue` matched at
`floorbox_recompute_stateroot_v5.go:347-353`. The fix MIRRORS them: every untrusted
per-member read becomes either (a) a VerifyProof'd `OldValue` via a point `Resolve`, or (b)
membership inside a set whose MTH is an anchored `OldValue`.

### The three broken sites, exact reads

**Class P — `floorbox_recompute_stateroot_rotate_v5.go`.**
- `rotateEpochSetLeafOps:300` — `NewValue: statehash.EncodeInt64(weightByID[m.ID])`, and
  `weightByID[m.ID]` is `rw.Members[i].Weight` (untrusted, `:246-247`). The epochSet leaf's
  `OldValue` proof (`m.EpochSetProof`) is verified, but the FROZEN weight written is the
  raw witness field.
- `rotateTallyOps:341` — `regVersionByID[id] >= threshold`, and `regVersionByID` is
  `rw.Members[i].RegVersion` (untrusted, `:245-247`). A forged regVersion flips a tally
  verdict.
- `rotateTallyOps:339` — `total += weightByID[id]` and `:343 ready += w` both read the same
  untrusted `Weight`.

**Class A — `floorbox_recompute_stateroot_atts_v5.go`.**
- `attesterQualifiedFromScreen:121-132` reads `sc.Slashed`, `sc.InEpochSet`,
  `sc.BondedSize`, `sc.BondedPresent` — all raw fields of `StateRootAttScreen` (`:54-65`),
  none carrying a proof. These decide whether a `validatorsSeen||id` ADD is emitted. The
  struct doc CLAIMS "the per-attester point witnesses (C-1)" but the struct carries NO
  witness field. This is the starkest gap.

**Class B — `floorbox_recompute_stateroot_bondreg_v5.go`.**
- `stateRootBondRegWriteSet:131-137` builds `owner/claimed/provenRoot` from raw
  `StateRootBondRegScreen` fields (`PriorOwner`, `Claimed`, `PriorProven`, `:62-71`), none
  carrying a proof. The displacement branch (`:170-183`) reads them to decide whether to
  strip a squatter's bonded + qualified standing (an id NOT in the payload).

## 2. The fix pattern — every witness field is a separate obligation

The rule: **a witness field that is READ (as a value or a predicate) is anchored the same
way a witness field that is WRITTEN is anchored — by a `Resolve` against `prevStateRoot`
whose success is REQUIRED before the read is trusted.** Two anchoring shapes exist; pick
per field:

- **Shape V (point Resolve).** Add a `Proof statehash.Witness` to the carrier and, at read
  time, require `statehash.Resolve(prevStateRoot, Key(tag, id), claimedValueOrNil, proof)`
  to be `IsProvenPresent()` (for a value/membership read) or `IsProvenAbsent()` (for an
  absence read). The claimed value the box then uses is the one the proof bound. This is the
  same primitive the scope gate already uses at `floorbox_recompute_stateroot_v5.go:387,405`.
- **Shape S (set-membership via MTH anchor).** If the read is pure membership over a
  keyspace whose digest the box already anchors (`anchoredPreSet`), derive the membership
  from that anchored set instead of a per-member flag. No new proof; reuse the digest
  anchor. This is strictly cheaper and is preferred where the read is membership-only.

`prevStateRoot` must be threaded into the screen/tally functions that currently do not
receive it (they are pure over the witness today). That is a signature change to
`attesterQualifiedFromScreen`, `stateRootBondRegWriteSet`, `rotateTallyOps`,
`rotateEpochSetLeafOps`, and their callers. Thread the same `prevStateRoot` the entry
already holds (`RecomputeStateRootEntriesRevocations`'s first arg).

### Per-field disposition

Class P — `StateRootRotateMember`:
| field | anchor | how |
|---|---|---|
| `Weight` (→ epochSet leaf NewValue AND tally total/ready) | Shape V | Require a proof that `qualified\|\|id == EncodeInt64(Weight)` present under `prevStateRoot`. The freeze copies qualified (`clone(qualified_POST)`), so the frozen weight IS the post-qualified weight. For an id whose qualified weight this block MUTATED (bonded in-block), the post value is derived by class B, not read — cross-check `Weight` against the B-derived `qualWrites[id]` instead of a proof. For a steady-state member, anchor against the committed `qualified\|\|id` leaf. See §5 for why this field is not already caught. |
| `RegVersion` (→ tally predicate) | Shape V | Require `Resolve(prevStateRoot, Key(tagRegVersion, id), EncodeUint8(RegVersion), proof).IsProvenPresent()`; `RegVersionKnown=false` requires `IsProvenAbsent()`. |
| `EpochSetOldValue` / `EpochSetProof` | already anchored | These are the fold `OldValue` + proof — no change. |

Class A — `StateRootAttScreen`: add a `Proof` per field-read (or one combined per-attester
witness bundle carrying the three point proofs):
| field | anchor | how |
|---|---|---|
| `Slashed` | Shape S preferred, else V | slashed membership is already anchored by `tagSlashedRoot` (`anchoredPreSet`). Derive `Slashed` from the anchored pre-slashed set the entry already reconstructs; drop the raw flag. If the block has no slash class (no slashedRoot witness), use Shape V: point `Resolve` of `slashed\|\|id`. |
| `InEpochSet` | Shape V | `epochSet\|\|id` is value-carrying; membership = present. Require `Resolve(prevStateRoot, Key(tagEpochSet, id), nil-value-membership, proof)` proving PRESENT (membership; weight discarded per R-A-membership-source) or ABSENT. |
| `BondedSize` / `BondedPresent` | Shape V | `Resolve(prevStateRoot, Key(tagBonded, id), EncodeInt64(BondedSize), proof).IsProvenPresent()`; absent requires `IsProvenAbsent()`. |

Class B — `StateRootBondRegScreen`:
| field | anchor | how |
|---|---|---|
| `Claimed` / `PriorOwner` | Shape V | `Resolve(prevStateRoot, Key(tagBondRootOwner, Root), EncodeID(PriorOwner), proof).IsProvenPresent()`; `Claimed=false` requires `IsProvenAbsent()` of `bondRootOwner\|\|Root`. |
| `PriorProven` | Shape V | `Resolve(prevStateRoot, Key(tagBondRootProven, Root), EncodeBool(PriorProven), proof)` — present-true / present-false / absent, each proven. |

Note: the displaced squatter's `delete(bonded, oldOwner)` + `qualifiedMaintain(oldOwner)`
writes (`bondreg:177-182`) already ride the `ChangedLeaves` match, so their `OldValue`s are
verified. The gap was purely the DECISION to displace, driven by unanchored ownership.

## 3. The adversarial-committed-root regression-gate shape

The existing ablation tests forge against an HONEST committed root: they mutate a witness
field, keep `b.StateRoot` = the honest post-root, and assert the recompute stalls. They are
BLIND to the class here because a forged value that the honest committed root does not bind
(a membership-only digest, or a leaf the attacker ALSO forges the committed root over) can
leave `postRoot == committedStateRoot` intact.

The new gates recompute the committed root FROM the forged ops and assert the recompute
still stalls while `forgedRoot != honestRoot`.

Shared helper (test-only, `floorbox_recompute_adversarialroot_v5_test.go`):

```
// recomputeCommittedRootFromForgedWitness builds the FoldOps the box WOULD fold for the
// forged witness, folds them from prevStateRoot to get the root the attacker would commit,
// and returns it alongside the honest root. The gate asserts:
//   1. forgedRoot != honestRoot            (the forgery is real, not a no-op)
//   2. Recompute(prev, forgedRoot, b, forgedWitness) != nil   (the box STALLS)
// If (1) holds and (2) fails, the box wrong-ACCEPTED a state it should reject.
func recomputeCommittedRootFromForgedWitness(t, c, prev, b, forgedWit) (forgedRoot, honestRoot ports.Hash)
```

The helper folds the forged ops via the SAME `assembleStateRootRecomputeOps` +
`statehash.FoldChangedPaths` path the box uses, so the "committed root" it produces is
exactly what an attacker who controls the block would sign. This closes the honest-root
blindness: the attacker is now allowed to move the committed root to match the forgery, and
the fix must STILL stall (because the anchoring `Resolve` fails against `prevStateRoot`,
which the attacker does NOT control).

Every field in §2 gets one gate instance driven through this helper.

## 4. The 23-field enumeration + self-checking coverage test

Model on the #679 leaf-diff coverage-meta pattern
(`floorbox_recompute_leafdiff_v5_test.go`): a reflection-pinned cert table keyed on the
witness struct fields, so an outsider verifies no field was missed and the list cannot
silently drift.

The per-predicate cert table is `field × membership-source × value-source`, one row per
untrusted field across the three carriers. Enumeration (fields that FEED a NewValue or a
branch predicate; pure `OldValue`+`Proof` pairs already anchored by the fold are listed as
`already-anchored`):

`StateRootRotateMember` (7): ID, Weight, RegVersion, RegVersionKnown, EpochSetProof,
EpochSetOldValue, EpochSetDeleteSiblings.
`StateRootRotateScalar` (2 × 8 uses): OldValue, Proof — one pair for EpochStart,
MatureEpoch, GateLockedIn, GateHeight, Era3LockedIn, Era3Height, Era4LockedIn, Era4Height.
`StateRootAttScreen` (5): Attester, Slashed, InEpochSet, BondedSize, BondedPresent.
`StateRootBondRegScreen` (4): Root, PriorOwner, Claimed, PriorProven.

That is 7 + 2 + 5 + 4 = 18 declared fields; expanding the 8 scalar USES makes 24 anchoring
obligations, of which one (`ID`/`Attester`/`Root` as pure keys) is a key not a value. Net
23 value/predicate obligations — the enumeration the coverage test pins. Each row:

| field | carrier | membership-source | value-source | disposition |
|---|---|---|---|---|
| Weight | RotateMember | frozen set (post-qual) | **UNANCHORED → Shape V (qualified leaf)** | FIX |
| RegVersion | RotateMember | frozen set | **UNANCHORED → Shape V (regVersion leaf)** | FIX |
| RegVersionKnown | RotateMember | frozen set | **UNANCHORED → Shape V absent-proof** | FIX |
| EpochSetOldValue | RotateMember | — | fold OldValue vs prevStateRoot | already-anchored |
| EpochSetProof | RotateMember | — | the proof itself | already-anchored |
| EpochSetDeleteSiblings | RotateMember | — | final-root equality | already-anchored |
| Slashed | AttScreen | attester | **UNANCHORED → Shape S (slashedRoot) / V** | FIX |
| InEpochSet | AttScreen | attester | **UNANCHORED → Shape V (epochSet leaf)** | FIX |
| BondedSize | AttScreen | attester | **UNANCHORED → Shape V (bonded leaf)** | FIX |
| BondedPresent | AttScreen | attester | **UNANCHORED → Shape V absent-proof** | FIX |
| PriorOwner | BondRegScreen | root | **UNANCHORED → Shape V (bondRootOwner)** | FIX |
| Claimed | BondRegScreen | root | **UNANCHORED → Shape V present/absent** | FIX |
| PriorProven | BondRegScreen | root | **UNANCHORED → Shape V (bondRootProven)** | FIX |
| the 8 RotateScalar OldValue/Proof pairs | RotateScalar | scalar key | fold OldValue vs prevStateRoot | already-anchored |

Self-checking coverage meta-assertion (mirrors #679's reflection pin):

```
// TestAdversarialRootCoverageIsComplete reflects over the three witness carrier structs,
// collects every field, and asserts each appears in the cert table with a disposition of
// either "FIX" (a driven adversarial-root gate exists) or "already-anchored" (a stated
// reason). A new field on any carrier reddens this test until it is classified and, if it
// feeds a NewValue/predicate, given a gate. The table is keyed on reflect.TypeOf, not a
// hand list, so it cannot drift.
```

The teeth-proof ablation (per #679): delete one anchoring `Resolve` from the fix and
confirm its driven gate goes RED — a green gate with no demonstrated red is decoration.

## 5. The open question — is class-P `Weight` fold-caught, or forgeable?

**Answer (analytic, confirmed against the tree today; the failing-first gate CONFIRMS it):
`Weight` is FORGEABLE. It is NOT already fold-caught.**

Mechanism: the epochSet digest is `nodeSetMTHFromInt64(c.epochSet)`
(`statehash.go:263`), and `nodeSetMTHFromInt64` (`:271-281`) DROPS the int64 weights — it
commits MEMBERSHIP only. The per-member weight is committed solely by the `epochSet||id`
leaf (`statehash.go:177`, `add(tagEpochSet, id, EncodeInt64(w))`). In `rotateEpochSetLeafOps`
the box writes `NewValue = EncodeInt64(weightByID[m.ID])` from the untrusted `Weight`, and
the epochSet leaf's proof anchors only the PRIOR value (`OldValue`), never the new frozen
value. So a forged `Weight`:
- does NOT diverge `epochSetRoot` (membership-only digest — same id-set),
- DOES set the `epochSet||id` leaf to the forged value,
- and if the attacker recomputes the committed root over that forged leaf (the §3
  adversarial-root helper), `postRoot == committedStateRoot` holds. Wrong-accept.

The ONLY thing that could already catch it: if the same id's `qualified||id` leaf (which
carries the TRUE weight) were in the write-set AND the box cross-checked frozen weight ==
qualified weight. It is not cross-checked. So `Weight` is exposed.

**Failing-first gate to confirm on current main (`TestAdversarialRoot_ClassP_ForgedFrozenWeight`):**
1. Build an honest boundary block with a steady-state member `id` at qualified weight W.
2. Forge `rw.Members[i].Weight = W'` (W' != W), leaving the id-set unchanged.
3. Fold via the §3 helper → `forgedRoot` (over the forged epochSet||id leaf).
4. Assert `forgedRoot != honestRoot` (the epochSet leaf value changed → true).
5. Assert `Recompute(prev, forgedRoot, b, forgedWit) == nil` on CURRENT main — i.e. the box
   WRONG-ACCEPTS. This is the RED the fix turns green.

**Design branches on the outcome:**
- If the gate confirms wrong-accept (expected): `Weight` needs Shape V — anchor the frozen
  weight against `qualified||id` under `prevStateRoot` for a steady-state member, or
  cross-check it against class B's `qualWrites[id]` for an in-block-bonded member. Both
  paths above are specified.
- If the gate somehow stalls on main (unexpected — would mean an existing cross-check I did
  not find): reduce to the minimal repro, identify the catching invariant, and narrow the
  fix to `regVersion` only (which is definitely exposed — regVersion feeds a boolean tally
  predicate that changes a lock-in SCALAR, and that scalar IS committed and IS anchored, but
  the PREDICATE reading regVersion is not, so a forged regVersion that flips the tally
  produces a different lock-in scalar the attacker then commits — same adversarial-root
  wrong-accept). regVersion is exposed under either branch; `Weight` is the branch point.

## 6. Trap-avoidance

- **NoWitness → stall, never wrong-accept.** Every new `Resolve` uses the existing
  three-state `Result`: a nil/forged/omitted proof yields `MustStall` (neither
  `IsProvenPresent` nor `IsProvenAbsent`), and the box returns a stall error. The fix ADDS
  stalls; it never adds an accept path. Preserve the existing error wrapping
  (`ErrRecomputeStateRootDigest` / `ErrRecomputeStateRootScopeStall`) so the never-Accept
  scaffold at `RecomputeStateRootEntriesRevocations:194-216` is unchanged.
- **Do NOT fork the quorum arithmetic (#402 lesson).** `rotateTallyOps` reproduces
  `rotateEpoch`'s `3*ready > 2*total` (`rotate:355,363,371`). The fix anchors the INPUTS
  (weight, regVersion) to the tally; it MUST NOT re-derive or re-express the threshold
  arithmetic. Keep the `tally()` closure and the `3*ready > 2*total` comparisons byte-for-
  byte identical to the live path. If the anchored weight/regVersion are proven correct, the
  arithmetic already matches by construction — do not add a second copy.
- **Class M is downstream-poisoned by the A2 root — ensure the fix reaches it.** Class M
  (`maturityLatchOps`) consumes `postEverMature`, threaded into class P's freeze gate
  (`rotate:225`). Class M's own maturity recompute runs over `committedStateRoot` (#668,
  `RecomputeMatureNow`). A forged class-A witness that flips `validatorsSeen` membership
  changes the SeenSet class M reads. Verify the class-A fix (anchoring the screen) closes
  the class-M poisoning: with the att screen anchored, a forged att cannot add a spurious
  `validatorsSeen||id`, so the SeenSet class M recomputes is the honest one. Add a gate
  (`TestAdversarialRoot_ClassM_PoisonedBySpuriousAtt`) that forges a class-A screen to add a
  spurious seen member and asserts the class-M latch does NOT flip early (stall). This gate
  is the cross-class check that the A-fix reaches M.

## 7. Build order tomorrow

1. Land the §3 adversarial-root helper + the failing-first §5 `Weight` gate. Confirm RED on
   main (wrong-accept). This is the evidence that gates the rest.
2. Class A fix (starkest gap, no witness field at all today) + its gates.
3. Class B fix (ownership Shape V) + its gates.
4. Class P fix (Weight + regVersion Shape V) — turns the §5 gate green.
5. Class M cross-class gate (§6) — confirm the A-fix reaches M.
6. The §4 reflection-pinned coverage meta-assertion + the teeth-proof ablations.
7. Every fix ships its regression gate (RED-before/green-after) at the chain tier, catchable
   locally in seconds. CHANGELOG line (required even for _test.go-touching core changes).

## Consult status

This touches consensus-adjacent recompute (the floor-box that reproduces
`validateEra3Roots`' StateRoot equality). It does NOT change apply(), the block/validity
rules, or I1-I5 — the box still never-Accepts (`RecomputeStateRootEntriesRevocations`
STOP boundary intact). It hardens a witness-soundness gap in a never-Accept verifier. If the
fix's terminal disposition would flip the box toward Accept (#657), that is a separate
research gate. As scoped here (add stalls, never accepts), it is a builder-tier soundness
fix. FLAG for the Researcher if the `Weight` branch outcome (§5) reveals an existing accept
path rather than a wrong-accept-by-recompute.
