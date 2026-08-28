# era-3 committed state-root block format — design options

**Date:** 2026-08-28
**Seat:** Builder (ADVISE/SHAPE mode — design only, no code)
**Status:** deliberation for the era-3 hard freeze. Research-gated: the composed
format must be certified before it freezes (`.claude/CLAUDE.md` research gate —
consensus-rule change). This doc shapes the question; it does not decide it.

## Why this exists

`docs/decisions.md` carries a ratified obligation: **HARD FREEZE PREREQUISITE
2026-08-27 (era-3 format)** (decisions.md:546-559, from C-7 Q3). The era-3
`Block` MUST commit BOTH roots before the format can freeze:

- (a) a **state SMT root** over the completeness- and order-independence-proven
  set-valued validity state, and
- (b) a **separate append-only (RFC-6962) transparency-log root** for the
  ordered log (the #597 two-root shape),

both as **Hash-covered, attester-signed** block fields. AND the floor-box
verifier MUST carry the invariant **"no witness supplied for a key a predicate
reads → never accept (reject / stall)"** (decisions.md:552-555). The SMT backend
is `pokt-network/smt` v1.0.0, adopted at #596 (`internal/smtspike/doc.go`).

**State of the block today** (verified against `core/chain/chain.go`): the
`Block` struct (chain.go:310-390) commits NEITHER root. There is no state-root or
log-root field. The two-root capability exists only in read-only accessors
(`RevocationLogRoot()` at chain.go:2092-2094 returns the RFC-6962 MTH; the SMT is
exercised only in `internal/smtspike/`, imported by nothing). `BlockVersionRegGate
= 3` (chain.go:299) is a **readiness tag, not minted** — `BlockVersion` stays
`BlockVersionRounds = 2` (chain.go:281-284).

This deliberation lays out the real design choices to get from "commits neither
root" to a frozen era-3, per the PACE-BEFORE-CODE rule
([[silt-pace-before-code]]). It re-litigates nothing that is ratified; it designs
*to* the obligation.

## The proven field set (the input to choice 3)

The keystone oracles already enumerate and classify every `Chain` field
(`core/chain/modelcheck_state_completeness_test.go:76-124`). The classification
is the authoritative source, not this doc:

- **16 `committedSet` fields** (the state SMT): `byRoot`, `spent`, `revoked`,
  `slashed`, `bonded`, `epochSet`, `bondRootOwner`, `bondRootProven`,
  `bondRegHeight`, `regVersion`, `bondDomain`, `validatorsSeen`, `gateLockedIn`,
  `gateHeight`, `everMature`, `matureEpoch` (completeness_test.go:81-96). All 16
  are proven load-bearing and order-independent (the leave-one-out +
  order-varying oracles).
- **1 `committedLog` field** (the RFC-6962 root): `revLog`
  (completeness_test.go:99-105) — a CT-style ordered log, history-dependent by
  design, gets its OWN append-only root, NEVER an SMT leaf (the #597 category
  error).
- **1 `observable`** (under NO committed root): `epochStart`
  (completeness_test.go:107-111) — read only by `Regime()`, misreports health,
  never validity.

Nuance the encoding must respect (verified against chain.go:809-874): the 16
`committedSet` fields are **not all set-membership**. Some are set-valued
(`revoked map[Hash]bool`, `spent map[string]bool`). Others are **key→value maps
carrying a meaningful value** (`bonded map[NodeID]int64` — weights;
`bondRegHeight map[NodeID]uint64`; `bondRootOwner map[Hash]NodeID`;
`regVersion map[NodeID]uint8`). Others are **scalars** (`everMature bool`,
`matureEpoch`, `gateLockedIn`, `gateHeight`). The spike's `present = []byte{1}`
marker (`internal/smtspike/exclusion_test.go:30`) is correct only for
set-membership fields; a value-carrying field must commit its VALUE in the leaf,
or a witness proves presence without pinning the weight a predicate reads. This
is the load-bearing encoding decision below.

---

## Choice 1 — how the two roots enter `Block`

The roots must be inside `Hash()` so attesters sign them. This is unlike `Atts`,
`PrepareQC`, `CommitRound`, `Pruned`, which are deliberately EXCLUDED from `Hash`
(chain.go:523 lists exactly what the unsigned hash body contains:
`Version, Height, Prev, Entries, Proposer, Revocations, Unrevocations, BondRegs,
Slashes`). A root that is not in that list is not signed by the quorum, so it
carries no consensus weight — fatal for a state commitment.

**Option 1A — two new `keyasint` fields, added to the `Hash()` body.**
Add `StateRoot ports.Hash` and `LogRoot ports.Hash` at the next free CBOR field
numbers (15 and 16 — `Pruned` holds 14, chain.go:375). Extend the `unsigned`
struct literal in `Hash()` (chain.go:523) to include both.

- Benefit: minimal, additive, mirrors every prior field addition (the Token,
  Domain, Version idioms all rode `keyasint` additive). CBOR field numbers are
  explicit, so wire compatibility is a decode concern, not a positional one.
- Cost: the moment these are in the `Hash()` body, a block that sets them hashes
  DIFFERENTLY from an era-2 block. That is the whole point (they must be signed),
  but it forecloses `omitempty` as a silent-compat trick — see below.

**Option 1B — one combined `Roots` sub-struct field.**
A single `Roots struct{ State, Log ports.Hash }` at field 15.

- Benefit: groups the two roots; one field number spent.
- Cost: adds a nested type for no gain. The two roots have different lifecycles
  (state = SMT, log = RFC-6962) and different witness stories. Flat fields read
  straight in `Hash()` and in validity predicates. No benefit over 1A.

**`omitempty` vs required.** Both root fields MUST be **required** (no
`omitempty`) once era-3 is minted. Reasoning: `omitempty` on `BondRegs`/`Slashes`
(chain.go:343-350) was safe precisely because an absent value hashed identically
to the pre-field era — that is how they stayed additive without a version bump. A
state root does the opposite: a present-but-empty state (genesis) must commit a
DEFINITE root (the empty-SMT root), not an absent field, or two nodes disagree on
whether "no state" means "empty root" or "field omitted." An `omitempty` state
root re-opens the exact order/ambiguity hole the SMT was chosen to close. The
zero-value `ports.Hash{}` is a legal, meaningful value (empty-tree root and
empty-log root are fixed constants), so the field is always present in era-3.

**RECOMMENDATION (choice 1): Option 1A** — two flat, required `keyasint` fields
(`StateRoot` = 15, `LogRoot` = 16), both added to the `Hash()` unsigned body.
Simplest additive shape; required-not-omitempty closes the empty-vs-absent
ambiguity; flat beats nested for no cost.

---

## Choice 2 — version / minting sequencing (the hard-fork boundary)

This is the delicate one, and it is Andrew's call at the boundary (below). v3 is
today a **readiness tag, not minted** (chain.go:286-299). Committing roots is a
REAL schema + validity change: a block now carries fields the quorum signs and a
new validity predicate (root-matches-recomputed-state) rejects on. That is a hard
fork by any honest definition — a pre-era-3 binary cannot check the new predicate
and must not pretend to.

The #506 precedent is instructive but does NOT transfer. #506 deliberately did
NOT bump the minted version (chain.go:286-298): its R-rule "only REJECTS
payloads," needs no schema change, so it height-gated activation
(`regGateActive`, "apply the rule to every block of height > H_act") and kept
`BlockVersion = BlockVersionRounds`. era-3 is the opposite: it ADDS
attester-signed schema (two roots) AND a validity predicate. The #506 "no schema
change" escape hatch is unavailable here.

**Option 2A — mint a new version `BlockVersionStateRoot = 3` (reuse the const
value 3), height-gated activation.**
Flip `BlockVersion` to 3 at an activation height `H_era3` derived from committed
history (the #506 `regGateActive` mechanism, chain.go:293, reused for the schema
boundary). Below `H_era3`: era-2 blocks, no root fields, validated under era-2
rules — committed history stays valid, no re-interpretation (the era-gating
already in `ValidateCommit`/`VerifyEquivocation`, chain.go:274). At and above
`H_era3`: blocks MUST carry both roots and are validated under era-3 rules.

- Benefit: no silent flag-day. A validator that has not upgraded sees v3-tagged
  blocks it does not mint and — per chain.go:296-298 — this binary ALREADY
  ACCEPTS v3-tagged blocks (validated under the ≥-rounds rules). That acceptance
  clause was written for exactly this: "a future era that genuinely diverges the
  schema can flip minting without stranding it." The #506 readiness signal
  (`regVersion`, the bond-reg `Version` field, chain.go:439-448) already lets the
  chain COUNT rule-aware weight before flipping — so `H_era3` can be gated on a
  supermajority of frozen-epoch weight signalling `Version >= 3`, exactly as #506
  gated its R-rule. This is the certified soft-activation shape, reused.
- Cost: the collision on the const value 3. `BlockVersionRegGate = 3` is a
  READINESS threshold (a reg signalling `Version >= 3` is rule-aware), NOT a
  minted block version — today nothing mints 3. If era-3 mints `Version = 3`, the
  readiness meaning and the minted meaning must be reconciled: a reg signalling
  `>= 3` now means "I both know the #506 R-rule AND can validate committed
  roots." If those two readinesses are not actually the same software state, one
  const cannot carry both. This must be resolved before the flip (see
  decisions-for-Andrew).

**Option 2B — decode-gated (reject v3 outright on pre-era-3 binaries).**
Make `versionSupported` an exact set excluding 3 on pre-era-3 binaries, so a
v3-tagged block is rejected at decode.

- Benefit: no ambiguity — an un-upgraded node cannot even parse an era-3 block,
  so it cannot mis-validate one.
- Cost: this is the HARD fork chain.go:288-291 explicitly warns against
  ("versionSupported on every pre-gate binary is an EXACT set — a v3-tagged block
  is rejected outright at decode, which is a hard fork, not the certified soft
  fork"). It strands every un-upgraded node at the boundary — a flag-day. The
  canon's whole #506 design note exists to AVOID this. Contraindicated unless a
  clean-break flag-day is a deliberate governance choice.

**Option 2C — height-gated activation WITHOUT a minted version bump (roots as
`omitempty`, present only above `H_era3`).**
Keep `BlockVersion = 2`; add the root fields as `omitempty`; require them by
height, not by version.

- Benefit: no version collision; smallest diff.
- Cost: REJECTED. This re-introduces the empty-vs-absent ambiguity choice 1 just
  closed, and it divorces the schema-present signal from the version tag, so a
  block's validity rules are no longer a function of its self-declared era — the
  exact property `BlockVersion` exists to guarantee (chain.go:260-268: "committed
  by Hash and checked at decode, so a block from one era can never be silently
  mis-validated under another era's rules"). Do not weaken that guarantee to save
  a const.

**RECOMMENDATION (choice 2): Option 2A** — mint `BlockVersion = 3`,
height-gated activation via the #506 `regGateActive` mechanism, gated on a
frozen-epoch-weight supermajority of `regVersion >= 3` readiness. It is the
certified soft-activation shape, keeps era-2 history valid with no
re-interpretation, and reuses machinery that already exists and is model-checked.
The one blocker is the const-3 collision reconciliation, which is a
decision-for-Andrew, not a builder call.

---

## Choice 3 — state-root contents & encoding

**Contents.** Exactly the 16 `committedSet` fields under the state SMT
(completeness_test.go:81-96). NOT `revLog` (its own RFC-6962 root). NOT
`epochStart` (observable, no root). This is not a fresh judgment — it is the
oracle-enforced classification, and the freeze is HARD-GATED on the
consensus-weight fields (`bonded`/`epochSet`, then `spent`/`slashed`) reaching
the keystone oracles green (issue #603, decisions.md:551-552). Do not freeze the
contents until #603 is green; that gate is ratified.

**Encoding — the load-bearing decision.** One SMT over a **field-tagged single
keyspace**, exactly as the spike proved
(`internal/smtspike/exclusion_test.go:16-25`): a leaf key is `tag ‖ rawKey`, where
`tag` is the field name plus a separator (`"byRoot\x00"`, `"spent\x00"`, ...).
`TestFieldTagSeparatesKeySpaces` (exclusion_test.go:188-207) already proves the
same raw key under two tags is two independent entries, so committing a `byRoot`
entry never implies the serial was spent. This is the multi-map commitment shape.

- **Set-membership fields** (`revoked`, `spent`, `byRoot`-as-existence, `slashed`
  as a set, `validatorsSeen`): value is the fixed `present` marker
  (exclusion_test.go:27-30). A witness proves presence/absence; the value carries
  no information.
- **Value-carrying maps** (`bonded` weights, `bondRegHeight`, `bondRootOwner`,
  `bondRootProven`, `regVersion`, `bondDomain`, `byRoot` if the Entry value is
  read, not just its existence): the leaf value MUST be the canonical encoding of
  the field's value (e.g. the 8-byte big-endian bond weight for `bonded`), NOT
  `present`. Otherwise a witness proves "this validator is bonded" without pinning
  HOW HEAVY — and a predicate that reads the weight (`blockWeight` fork-choice,
  quorum) could be fed a true-presence / wrong-weight witness. **This is a
  soundness question for the Researcher** (see open questions): does every
  value-carrying committedSet field commit its value in the leaf, and is the
  canonical value encoding fixed so two honest nodes produce byte-identical
  leaves?
- **Scalar fields** (`everMature`, `matureEpoch`, `gateLockedIn`, `gateHeight`):
  these are single values, not maps. Each is one leaf at a fixed reserved key
  (`tag ‖ ""` or a fixed constant key) with the scalar's canonical encoding as
  the value. They must be under the state root (a predicate reads them — F-1
  maturity, #506 activation) and they are already proven order-independent.

**Canonicalisation leverage.** The order-independence work
(`core/chain/modelcheck_order_independence_test.go`) already proves the 16 fields'
values are identical across history orderings. The SMT is inherently
order-independent (the root is a function of the key→value SET, not insertion
order — the whole reason it was chosen, decisions.md:498-504). So the encoding
does not need to impose an order; it needs a **deterministic per-field
value encoding** so the same logical state produces the same leaf bytes on every
node. That determinism is the thing to pin and certify.

**Transparency-log root, kept separate.** `revLog` gets the RFC-6962 MTH already
computed by `RevocationLogRoot()` (chain.go:2092-2094). `LogRoot` (choice 1) = that
value. The era-3 snapshot carries the FULL revLog entry list (decisions.md:504-505),
so a snapshot-booted node can extend the log and still serve H9
inclusion/consistency proofs. This root NEVER folds into the SMT (the #597
category error).

**RECOMMENDATION (choice 3):** field-tagged single-keyspace SMT over the 16
committedSet fields, with a **per-field-class value encoding** (present-marker for
set fields, canonical value for value-carrying fields and scalars). `LogRoot` =
`RevocationLogRoot()`. Freeze contents only after #603 is green. The
value-encoding soundness is a Researcher question, not a builder call.

---

## Choice 4 — witness / verifier seam (do NOT build the mechanism; do not preclude it)

The ratified end state (decisions.md:639-671, #600 DECIDED 2026-08-28): the floor
box is a **semi-stateless witness-validating full validator**. It verifies every
transition against the committed root from tier-above-supplied witnesses; it does
NOT retain the full tree. The invariant that must live in the verifier:
**"no witness supplied for a key a predicate reads → never accept (reject /
stall)"** (decisions.md:552-555). Accepting on a missing witness inverts the
safe-degradation proof (C-7 Q2 — a witness-less floor box STALLS, never accepts).

The FORMAT's only job here is to **not preclude** witness delivery. Two things
the format must guarantee:

1. **The root the witnesses verify against is a committed, attested block field.**
   Choice 1 delivers this (`StateRoot` in the `Hash()` body, signed by the
   quorum). decisions.md:557-559: "A sound witness scheme cannot exist until the
   root it verifies against is a committed, attested block field, so this is a
   hard prerequisite for the witness path." This is THE reason era-3 is a hard
   freeze prerequisite, not an optimization.

2. **Witness delivery must be sourceable from any tier-above provider** — the
   format and gossip must not bake in a single or permissioned witness source
   (decisions.md:657-662, HARD REQUIREMENT, `TENETS.md:557`): witness-serving MUST
   stay open, un-permissioned, multi-provider. A permissioned witness set is the
   banned load-bearing centralized dependency. The format choice this constrains:
   whatever carries witnesses (a gossip message, a block-adjacent structure) must
   be reconstructable/servable by ANY archival-or-pruning node from the committed
   root, not signed by or bound to a privileged set.

**Where the invariant lives.** In the block-validity path, at the point a
predicate reads a committedSet key. The design seam: the validity predicate does
not read `c.bonded[id]` from a local map (a semi-stateless floor box has no such
map); it reads it THROUGH a witness-checked accessor that returns
`(value, present)` verified against `StateRoot`, and returns a rejection/stall if
no witness was supplied for that key. This is a follow-on build (witness soundness,
a witness-size DoS bound, who generates witnesses — decisions.md:530-532,663-671).
**The format must not assume stateful floor-box validation** (decisions.md:528-530).

**What the format must NOT do (the near-term bridge, stated honestly).**
Until witness-based validation is built, a validating node holds the tree
(decisions.md:524-527). Shipping the disk-backed store must NEVER silently
redefine build-immutable #8 upward to "validation requires 2.2 GB of state." The
FORMAT freeze must be compatible with BOTH the near-term hold-the-tree bridge and
the ratified semi-stateless end state — i.e. the root and its contents are the
same whether the validator holds the tree or verifies by witness. Choice 1 + 3
satisfy this: the root is a pure function of the committedSet, independent of how
a validator sources the leaves.

**RECOMMENDATION (choice 4):** the format commits the state root as an
attester-signed field (choice 1) and pins the committedSet contents/encoding
(choice 3), which is the necessary-and-sufficient FORMAT precondition for
witnesses. Do NOT design witness gossip, witness-size bounds, or the
`(value, present)` accessor here — that is the certified follow-on. Record the two
format constraints above (root-is-attested-field; witness-serving-stays-open) as
freeze conditions so the follow-on is not precluded.

---

## Choice 5 — build & migration order (safe increment sequence)

The safe path from today (commits NEITHER root) to a frozen era-3. Each step is
model-checked green before the next (the consensus-correctness discipline:
model-check before field; `docs/build-process.md`). Nothing here is a field run —
a field run confirms, never discovers.

1. **Close #603 first** (widen oracle probes to `bonded`/`epochSet`, then
   `spent`/`slashed`). RATIFIED gate: the freeze is hard-gated on these
   consensus-weight fields reaching the oracles green (decisions.md:551-552). The
   format cannot freeze over an unproven field set. **Local model-check tier.**

2. **Land the per-field-class value encoding + SMT root computation as product
   code behind the existing oracles** (promote from `internal/smtspike/` into
   core, wired to the 16 committedSet fields). No block-field change yet. Prove:
   the computed root is order-independent (extend
   `modelcheck_order_independence_test.go` to assert root equality, not just field
   equality) and snapshot-boot-equivalent (the root a snapshot-booted node
   computes equals the replayed node's — extend
   `modelcheck_snapshot_equivalence_test.go`). **Local model-check tier. This is
   where a value-encoding bug is caught, before it is a signed block field.**

3. **Add `StateRoot` + `LogRoot` to `Block` as required era-3 fields, in the
   `Hash()` body** (choice 1), gated so era-2 blocks are unaffected (choice 2).
   Prove: era-2 history hashes and validates identically (no re-interpretation);
   an era-3 block's roots are attester-signed (a tampered root fails the
   proposer/attester signature check, like any Hash-covered field). **Local
   model-check tier + the existing era-boundary Reload oracle.**

4. **Wire the era-3 validity predicate** (a block's committed `StateRoot` must
   equal the root recomputed from the post-apply committedSet; `LogRoot` must
   equal `RevocationLogRoot()`). Prove: a block with a wrong root is REJECTED; a
   correct block is accepted; the predicate is era-gated so era-2 blocks skip it.
   **Local model-check tier — the I1–I5 statement in the PR body
   (consensus-invariants.md:6): this touches validity, so name which invariants it
   preserves.**

5. **Height-gated activation** (choice 2A): set `H_era3` derivation from committed
   history, gate on `regVersion >= 3` supermajority. Prove: below `H_era3` no root
   required; at/above, required; the boundary cannot be reorged out from under
   enforcement (the #506/#357 Condition-A argument, chain.go:881-884). **Local
   model-check tier.**

6. **Researcher certification of the composed format** (below), THEN freeze. THEN
   a field run confirms (never discovers). The witness path is a separate
   follow-on after the freeze (choice 4).

**RECOMMENDATION (choice 5):** steps 1→6 in order; do not merge a step red; the
root-computation correctness (step 2) is proven at the model-check tier BEFORE it
becomes a signed block field (step 3), so a value-encoding defect is caught as a
root mismatch in a test, not as a consensus split in the field. Freeze only after
step 6's certification.

---

## OPEN QUESTIONS FOR THE RESEARCHER (the era-3 format is research-gated)

The composed format must be certified before it freezes. Precise questions:

1. **Two-root composition soundness.** Is committing `StateRoot` (history-
   independent SMT over the 16 committedSet fields) AND `LogRoot` (RFC-6962 MTH
   over `revLog`) as two independent attester-signed fields sound and complete for
   block validity? Confirm the #597 two-root shape is fully discharged by this
   composition and no committed-state field escapes both roots.

2. **Value-encoding soundness (the load-bearing one).** For each value-carrying
   committedSet field (`bonded`, `bondRegHeight`, `bondRootOwner`,
   `bondRootProven`, `regVersion`, `bondDomain`, and `byRoot` if its Entry value
   is read by a predicate), does the SMT leaf commit the field's VALUE (not just
   presence)? Is there a fixed canonical value encoding such that two honest nodes
   produce byte-identical leaves? Certify that no predicate can be fed a
   true-presence / wrong-value witness. (Scalars `everMature`, `matureEpoch`,
   `gateLockedIn`, `gateHeight` at fixed reserved keys — confirm the reserved-key
   scheme does not collide with any map keyspace.)

3. **Field-tag keyspace separation.** Confirm the `tag ‖ rawKey` scheme
   (exclusion_test.go:16-25) is collision-free across all 16 field tags AND the
   scalar reserved keys — no raw key under one tag can ever equal a key under
   another. (The spike proves it for `byRoot`/`spent`; certify it for the full 16
   plus scalars.)

4. **The verifier invariant.** Certify that "no witness supplied for a key a
   predicate reads → never accept (reject / stall)" is the complete safe-
   degradation rule for the composed format — i.e. there is no committedSet key a
   predicate can read for which a missing witness could be silently treated as
   absence/zero and wrongly accepted. (This binds C-7 Q2 to the concrete field
   set.)

5. **Activation-boundary soundness.** Certify the height-gated `H_era3`
   activation (choice 2A) is a sound hard-fork boundary: era-2 history stays valid
   with no re-interpretation, the boundary cannot be reorged out from under
   enforcement, and the `regVersion >= 3` readiness gate is the correct
   supermajority condition (does the #506 certification's Q2 form transfer to a
   SCHEMA change, or does a schema+validity change need a stronger condition?).

6. **Security-parameter coupling.** Does anything in the composed format couple to
   a security parameter a proof depends on (recall build-process.md — a durability
   knob was twice also a security parameter)? Specifically: the SMT hash
   (sha256), the value-encoding widths, and any witness-size bound the format
   reserves space for. Flag any knob that is also a security parameter before it
   freezes.

7. **Const-3 reconciliation (also a decision-for-Andrew).** Is it sound for a
   single `regVersion >= 3` readiness signal to gate BOTH the #506 R-rule
   awareness AND era-3 root-validation capability, or must these be distinct
   readiness levels (implying a `BlockVersion 4` for era-3 and `3` reserved to the
   reg-gate readiness)?

---

## RECOMMENDATION (composed)

- **Choice 1:** two flat, required `keyasint` fields — `StateRoot` (15),
  `LogRoot` (16) — added to the `Hash()` unsigned body so attesters sign them.
- **Choice 2:** mint `BlockVersion = 3`, height-gated soft-activation via the
  #506 `regGateActive` mechanism, gated on a `regVersion >= 3` frozen-weight
  supermajority. Keeps era-2 history valid, no flag-day.
- **Choice 3:** one field-tagged single-keyspace SMT over the 16 committedSet
  fields with per-field-class value encoding (present-marker for set fields,
  canonical value for value-carrying fields and scalars); `LogRoot` =
  `RevocationLogRoot()`; freeze contents only after #603 green.
- **Choice 4:** the format commits the attested root and pins the contents — the
  necessary-and-sufficient FORMAT precondition for witnesses. Do NOT build the
  witness mechanism here; record the two freeze constraints (root-is-attested-
  field; witness-serving-stays-open-and-multi-provider) so the follow-on is not
  precluded.
- **Choice 5:** steps 1→6, model-checked green at each step, root correctness
  proven before it becomes a signed field, certify then freeze.

This is the simplest shape that discharges the ratified obligation and does not
preclude the ratified semi-stateless end state. It reuses machinery that already
exists and is model-checked (the #506 activation mechanism, the RFC-6962
`RevocationLogRoot`, the field-tagged SMT keyspace from the spike, the three
keystone oracles).

## DECISIONS FOR ANDREW (his call — the version/mint boundary and freeze scope)

1. **The const-3 collision / version-mint boundary.** Does era-3 mint
   `BlockVersion = 3` (reusing the value `BlockVersionRegGate = 3` holds as a
   readiness threshold), reconciling the two meanings of "Version 3"? Or does
   era-3 mint `BlockVersion = 4`, leaving 3 as the reg-gate readiness threshold
   only? This is the version/mint-boundary call the research gate reserves to
   ratification. Depends on Researcher question 7.

2. **Soft-activation vs deliberate flag-day.** Ratify Option 2A (height-gated
   soft-activation, no stranding) as recommended, or choose a deliberate clean-
   break flag-day (Option 2B) if the governance intent is a hard cutover. The
   canon leans hard toward 2A (chain.go:288-298); confirm.

3. **Freeze scope.** Confirm the freeze covers exactly the 16 committedSet fields
   + `revLog` log root, and that the freeze is HELD until #603 (the
   `bonded`/`epochSet`/`spent`/`slashed` oracle probes) is green — this is
   ratified (decisions.md:551-552) but the freeze-trigger is Andrew's to pull.

4. **Ratify the Researcher verdict.** Per the research gate, the composed era-3
   format is a consensus-rule change: the Builder shapes the question (this doc),
   the Researcher certifies (questions above), Andrew ratifies before the format
   freezes. No code lands on the format until that verdict is filed and ratified.
