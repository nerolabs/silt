# LastCommit attestation carrier — round A design record

**Date:** 2026-09-03 · **Seat:** BUILDER · **Branch:** `builder/lastcommit-carrier`
Built on `origin/main` = `d7e4df0`; rebased onto `1adca0f` (#707) before the final verification run, clean, suite re-run green.

**Binding inputs (read before any code was written):**

- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
  — §2.3 the hazard, §7 the carry-list, §10 **O1** (the ratified field / validity / transition /
  order rules — binding), §11 gates G1–G9, §12 residuals.
- `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R-BOX-ATTESTS-scoping-crosscheck-2026-09-02.md`
  — the oppositions and their code sites.
- `docs/design/consensus-invariants.md` (I1–I5), `docs/build-process.md` (consensus-correctness
  discipline), and the R-ROTATE-EPOCH-LAST pin (`core/chain/rotate_epoch_last_drift_test.go`,
  commit `76cdb75`) for how an apply-order pin is done in this repo.

**Owner ratification:** O1 ratified 2026-09-03. This round builds O1 only.
**Explicitly NOT in this round:** O2 (the stamp is NOT raised), O3 (fork-choice weight — no
`blockWeight` / `heavier` code), O4 (the canon amendment — an owner ratification, not a build).

---

## 1. The mechanism, stated before the fix

The failure is that a precommit certificate which would seat a **new** attester makes the
recomputed post-apply state root differ from the root the proposer signed **before** gathering,
because `apply` writes `validatorsSeen` from `b.Atts` (`chain.go:3293-3298`) while `Hash()`
excludes `Atts` (`chain.go:656`) and the era-3/era-4 root predicate re-runs the real `apply` over
the attached certificate (`era3validity.go:117-138`, `:148-160`). Every replica therefore rejects
that block. Two consequences: (a) `validatorsSeen` freezes at the pre-activation set, permanently
ceilinging `MatureCoefficient`; (b) the height stalls for any round whose first-to-quorum prefix
contains a never-seen qualified attester.

This change addresses the cause — a transition input that is not hash-covered — by moving the
seating write off the block's own uncovered `Atts` and onto a **hash-covered** `LastCommit`
carrier that republishes the **parent's** precommits. The proposer holds those bytes before it
signs, so the root it signs is the root the block commits.

## 2. Options considered

| Option | Shape | Verdict |
|---|---|---|
| **A · `LastCommit` carrier** (chosen) | `[]Attestation` over `b.Prev` at `PhasePrecommit`, folded into `Hash()`; seats one block late | **Ratified (O1).** The proposer holds the bytes before signing; CometBFT's settled analogue. |
| B · same-block `SeatAdds []NodeID` | proposer predicts the certificate | Rejected in O1: re-creates the stamp-after-gather circularity and invents schema. |
| C · drop `validatorsSeen` from the root | | Refuted (C): the `everMature` latch input becomes replica-local ⇒ regime fork. |
| D · derive seats from the replica-local certificate | | Refuted (D): a replica-local certificate is not a determinism source (S5). |
| E · re-sign after gather | | Refuted (E): orphans the #432 lock. |
| M · re-base the maturity metric on `bonded` | | Declined by default in O1 — a published-claim change, separate certification. |

## 3. Decisions this round had to make inside O1's envelope

### 3.1 cbor key 18, not 17

O1 says "additive cbor key". Key 17 is taken on `builder/r0.4b-c3-close`
(`IssuerKeys []IssuerKeyReg \`cbor:"17,keyasint,omitempty"\``, verified by
`git show builder/r0.4b-c3-close:core/chain/chain.go`), and the verdict's carry-list §7 item 6
expects R0.4b to land before the stamp raise. Taking 17 here would be a silent wire collision
between two unmerged branches. `LastCommit` takes **18** and reserves 17 for R0.4b.

### 3.2 "at ANY single round" — read as per-entry, not set-wide

O1: *"every entry verifies over `b.Prev`'s hash at `PhasePrecommit` at any single round
(`CommitRound` is uncovered, so the rule must not bind to it)"*. The PE ruling states the same
rule as *"verifies over `Prev` at `PhasePrecommit`, any round"* (§4, I2 paragraph).

Read: **each entry verifies at its own declared `Round`**; the rule constrains neither the round
nor agreement between entries. Reason: the alternative (all entries at one common round) would
make honest-maximal carry ("carry everything you hold") produce an *invalid* block whenever a
proposer holds parent precommits from two rounds, which contradicts O1's producer rule in the
same section. Both readings agree on every certificate `collectQuorumSigs` accepts, since that
function is fatal on a mixed-round set (`chain.go:2758-2761`); they differ only on the
supra-certificate set an honest-maximal proposer may hold. Recorded here as an interpretation, not
a rule change; if the certifier reads it the other way the change is one `if` in `validateCarrier`.

### 3.3 The parent proposer's identity in the floor box — anchored by the parent's own proposer signature

O1's transition rule excludes `id == parent.ProposerID()`. The chain has the parent block
(`c.blocks`). The **floor box does not** — `WitnessValidateV5(b, parentStateRoot, d)` and
`RecomputeStateRootEntriesRevocations(prevStateRoot, committedStateRoot, b, w)` receive only the
block and a root, and the parent's proposer identity is **not** a committed leaf (no tag for it in
`statehash.go`; adding one is a schema change, out of scope).

Three ways to give the box the parent proposer:

1. **Unanchored witness field.** A forged value flips exactly one seat in *either* direction
   (naming a non-signer seats the real parent proposer; naming a real signer drops it). Rejected:
   it is a value-bearing witness input with no anchor, the exact class Boulder 1 closed.
2. **Whole parent block in the witness**, box re-hashes to `b.Prev`. Perfectly sound, but the
   parent body carries `BondReg.Answer` proofs (~1.5 MB), so it blows the box's cost story.
3. **The parent's own proposer signature** (chosen). The witness carries
   `ParentProposer` (the parent's `Proposer` pubkey) and `ParentProposerSig` (the parent's
   `ProposerSig`), and the box requires `ed25519.Verify(ParentProposer, b.Prev[:], ParentProposerSig)`
   — the same bare-hash proposer-signature arithmetic the chain uses at `chain.go:2514`, `:3083`,
   `:3141`. ~96 bytes.

What (3) proves and what it does not: it proves *the named key signed `b.Prev`*, not *this key is
THE parent's proposer*. Anyone can sign `b.Prev` with their own key.

**CORRECTION (2026-09-03), forced by the build certification
`LASTCOMMIT-CARRIER-round-A-5d3fda0-RESEARCH-CERTIFICATION-2026-09-03.md` §6.2.** This section
originally stated the residual as bounded to "an attacker who holds key K can make the box skip K's
own seat", and claimed the stall meant the box never falls through to "no exclusion". **The bound is
one-sided and the never-falls-through claim is false. Both are withdrawn.** The witness-supplied key
has **two** effects relative to truth:

1. **DROP.** The named key K *is* a carrier signer ⇒ K's seat is skipped. Requires the attacker to
   hold K. Downward-only, genuinely bounded to the forger's own seat. This is the direction the two
   driven FIX gates (`TestAdversarialRoot_ClassA_ForgedParentProposer` /
   `…_MissingParentProposerSig`) cover.
2. **ADD — not bounded by key ownership.** The attacker mints a **fresh keypair**, signs `b.Prev`
   with it, and supplies that. The pair verifies. Its derived id matches **no** carrier entry, so
   **nothing is skipped** and the parent's **true** proposer P self-seats. **This requires no key of
   P's at all** — one `ed25519.Sign` call. The stall fires only for a *missing or malformed* pair,
   which an attacker has no reason to supply, so the anchor is close to vacuous against this
   direction.

**What (2) buys, priced exactly:** at most **one extra id per block** — but it is precisely the id
the exclusion exists to remove, the anti-self-declaration property `validatorsSeen` is built on. And
it is a **wrong-accept** direction, not a denial: the attacker authors a block whose `StateRoot`
contains P seated, supplies the fresh-key witness, and the box's fold reproduces that root and
returns nil, while every full node computes without P and rejects the block.

**Status: inert, and a FLIP PRECONDITION (FP-1).** `WitnessValidateV5` returns
`IndeterminateTrustlessly` with `ErrRecomputeGated` (never-Accept), so there is no live exposure at
this commit. It is carried as **R-CARRIER-PARENTPROPOSER** in §7 and is now tracked in `ROADMAP.md`
under Boulder 1, because ROADMAP is the task SSOT and a flip precondition living only in
`docs/thinking/` is untracked.

**Certified fix direction — named, NOT built in this round.** Bind the parent proposer to
**committed content**: a v5 committed scalar leaf, `tagLastProposer`, written by `apply()` to
`b.ProposerID()` and Resolved by the box against `prevStateRoot` like every other class-A input.
This is the shape the class-M / class-P scalars already use (`statehash.go`), it costs one leaf, and
it makes the input Resolve-anchored rather than self-signed. It is an **additive committed-format
change on the open era**, so it needs **its own certification** and it must land **before the era-4
freeze** — after the freeze it is a new era. Option (2) above (the whole parent block in the
witness) remains sound but is rejected on cost; explicitly accepting the residual at flip time is an
owner's call, and may only be put to the owner with this corrected bound in front of him.

### 3.4 Era gating and the frozen prior-era rule

The carrier transition fires only for `b.Version >= BlockVersionWitnessable` (v5). The
`for _, a := range b.Atts` seating loop is left **byte-for-byte** and now runs only for
`b.Version < BlockVersionWitnessable`, so every era-3 (v4) and era-2 (v2) block transitions
exactly as it does on `d7e4df0`.

## 4. What was built

| Site | Change |
|---|---|
| `core/chain/chain.go` `Block` | `LastCommit []Attestation \`cbor:"18,keyasint,omitempty"\`` |
| `core/chain/chain.go` `Hash()` | `LastCommit` folded into the unsigned struct; `omitempty` keeps every carrier-free block byte-identical |
| `core/chain/carrier.go` (new) | `validateCarrier` (the O1 validity rule), `applyCarrier` (the O1 transition rule), `HeadCarrier` (the producer source), `headProposerID` |
| `core/chain/chain.go` `apply()` | `applyCarrier` call pinned **before** the bond-reg loop; the legacy `b.Atts` seating loop era-gated to sub-v5 |
| `core/chain/chain.go` `ValidateProposal` / `appendStructural` | `validateCarrier` on both disk-write paths |
| `core/chain/chain.go` `AppendGenesis` | genesis `LastCommit` refused by rule (`ErrGenesisLastCommit`). *(Rebase correction 2026-09-04: the round-A build also refused `Atts`; that half is now research-gated — R-CARRIER-GENESIS-DISPOSAL — and `AppendGenesis` keeps main's pre-carrier `Atts` behaviour.)* |
| `core/node/chainrole.go` | the proposer populates `LastCommit` from `HeadCarrier()` before `PopulateEra4Roots` and before `Sign` |
| `core/chain/floorbox_recompute_stateroot_atts_v5.go` | class-A derivation re-pointed to `b.LastCommit` verified over `b.Prev`, with the parent-proposer exclusion anchored per §3.3 |
| `core/chain/floorbox_recompute_stateroot_v5.go` | `ParentProposer` / `ParentProposerSig` on `StateRootWitness` |
| `core/chain/readset_v5.go` | `readSetAtts` re-pointed to the carrier |
| `docs/design/block-format-by-era.md` | the v5 `LastCommit` row |

## 5. Gates — tier, RED evidence, GREEN

Every gate was driven RED before the fix (or by ablating the fix, where the gate needs the field
to exist in order to compile). The RED output is transcribed in §6.

| Gate | Tier | RED | GREEN |
|---|---|---|---|
| **G1** unseen-attester commit | cold chain | `era4AnchorChain(t,1,1)` → height-1 v5 `Append` fails `ErrEra3StateRootMismatch` | `TestG1_CarrierSeatsUnseenAttestersOneBlockLate` — height 1 commits with 0 seen; height 2 carries 4 entries and seats 3 |
| **G2** node-tier reply order, 3 arms + 3 masks | node path, held delivery, no box driver | all three arms: `propose: commit rejected by own replica: … StateRoot does not equal the recomputed post-apply committed state root` | `TestG2_CarrierNodeTierReplyOrder` (`core/node`) — arm (i) commits; the fifth node FIRST and LAST **both commit**, and each arm pins the EXACT carrier membership and seated count: FIRST carries 4 (`{proposer, fifth, id1, id2}`) and seats **3**, the fifth included; LAST carries 3 (`{proposer, id1, id2}`) and seats **2**, the fifth **structurally NOT seated** — its reply landed after the first-to-quorum prefix closed and `finishPC` discarded it (`chainrole.go:1059-1061`), so it is seated only once it makes a prefix at some later height (R-CARRIER-PREFIX-ONLY, §7). 3-versus-2 is the discriminator; `seen != 0` was not, and was satisfied by the pre-existing anchors (MG-1) |
| **G3** served-variant determinism, 3 variants | cold chain | pre-carrier: a replica holding a superset certificate rejects a canonical block | `TestG3_ServedVariantDeterminism` — same-round superset AND different-round `(PrepareQC_r′, Atts_r′)` pair with rewritten `CommitRound` both accept with identical `StateRoot`; one child appended to all three replicas seats identically |
| **G4** seating liveness + the metric **CEILING** | cold chain, objective arm | pre-carrier: an operator bonded after activation is never seated; `C2Metric().Participants` never rises | `TestG4_NewOperatorsRaiseTheCoefficient` — the joiner is seated within 2 heights. Asserts the ceiling lifting, NOT monotonicity (a seated member may lapse and re-bond, so a monotone assertion would be wrong and pass vacuously) |
| **G5** compile-time rollout gate | unit | ablation: force the stamp to 4 → the gate fatals on the never-stamp-4 rule | `TestG5_StampFiveImpliesTheCarrierIsHashCovered` — **vacuously green by design** (the stamp is 3; this round does not raise it). It fires the moment a binary stamps 5 without carrier coverage, and fatals immediately on a stamp of 4 |
| **G6a** no-v4-window, tally path | cold chain | in-test ablation, recorded as **FORBIDDEN not accepted**: an all-stamp-4 fleet locks era-3 alone and mints v4 at H_era3 | `TestG6a_NoV4WindowOnTheTallyPath` — with every reg stamped 5 both tallies lock at the same rotation, `H_era3 == H_era4`, and no committed block is ever v4 |
| **G6b** no-v4-window, **override** path | cold chain / config | — (exposure gate) | `TestG6b_OverrideActivationIgnoresTheStamp` — pins that a pre-latch `Era3ActivationHeight` with `Era4ActivationHeight` unset activates era-3 **regardless of `regVersion`** and opens a v4 window, and that `New`'s layering assertion does not prevent it. This is why O2's rule must read "by tally **or** by genesis override" |
| **G7** invariant II driver pin | box-entry Part B | — | **OUT OF SCOPE for this round**, as the verdict files it (owed to box-entry Part B, not here) |
| **G8** box class-A re-point + S3 agreement | cold box | the R-CARRIER-REFLECTION pin fired on the two new witness fields (§6); the whole class-A/M/P box suite went red on the source re-point | `TestG8_BoxIsBlindToTheBlocksOwnAtts` + the re-pointed class-A suite. A same-hash served variant with a different certificate no longer moves the box verdict |
| **G9** stub-`Atts` proposal refusal | cold chain | an UNSIGNED stub attestation from a qualified identity MOVED the recomputed state root | `TestG9_StubAttsDoNotMoveTheRecomputedRoot` — pins O1's first alternative: `Atts` in proposal bytes do not move the recomputed root |

Plus three structural pins the design rests on, each ablated RED and restored:

| Pin | What it pins | Ablation |
|---|---|---|
| `TestCarrierHashDriftGuard` | every carrier-free block hashes BYTE-IDENTICALLY to pre-carrier `origin/main` (recomputed from a frozen mirror of the pre-carrier unsigned body, not derived from `Block`), and a carrier-bearing block hashes differently | dropping `LastCommit` from `Hash()` → RED on the complement |
| `TestCarrierFoldPrecedesBondRegsInApply` | the carrier fold is a top-level statement of `apply()` running BEFORE the bond-reg loop, the TTL sweep and the slash loop (the O1 order rule, pinned the R-ROTATE-EPOCH-LAST way) | moving the fold below the bond-reg loop → RED |
| `TestEveryDiskWritePathRunsTheEra3RootCheck` (extended) | `validateCarrier` is now one of the era rules EVERY disk-write path must run — structural, discovered by scanning for `c.apply(b)`, not a hand list | a future unguarded write path fails it |
| `TestAdversarialRoot_ClassA_ForgedParentProposer` / `…_MissingParentProposerSig` | the two driven FIX gates the R-CARRIER-REFLECTION coverage table's new rows name | both are the ablation |

## 6. RED evidence (verbatim)

**G1** (throwaway probe at `d7e4df0`, before the field existed):

```
--- FAIL: TestRedProbeG1
    G1 RED: height-1 v5 block with a four-key certificate REJECTED:
    chain: era-3 (v4) block StateRoot does not equal the recomputed post-apply committed state root
```

**G9** (same probe, with a stub att from a QUALIFIED identity — an unqualified one is a no-op and
would have made the probe green for the wrong reason):

```
--- FAIL: TestRedProbeG9
    G9 RED: stub Atts MOVED the recomputed state root (6435bb… != 3231…)
```

**G2** (the shipped gate, run with the fix ablated — producer line removed and the `b.Atts` seating
loop un-era-gated):

```
--- FAIL: TestG2_CarrierNodeTierReplyOrder/i/first-v5-round-commits
    G2(i) RED: the proposer's own replica rejected its own v5 block:
    propose: commit rejected by own replica: chain: era-3 (v4) block StateRoot does not equal …
--- FAIL: TestG2_CarrierNodeTierReplyOrder/ii/fifth-node-FIRST
--- FAIL: TestG2_CarrierNodeTierReplyOrder/ii/fifth-node-LAST
```

All three arms RED — the intermittent-stall symptom of converged verdict §2.3(b), reproduced at
the node tier with all three masks cleared.

**G8 / R-CARRIER-REFLECTION** (the pin firing on the two new witness fields, exactly as the routing
predicted):

```
--- FAIL: TestFoldInputCarrierCoverageIsComplete
    R-CARRIER-REFLECTION: 2 coverage gap(s) …
      - UNCLASSIFIED FIELD: StateRootWitness.ParentProposer …
      - UNCLASSIFIED FIELD: StateRootWitness.ParentProposerSig …
```

The pin was NOT loosened. Both fields were classified **FIX** (they decide a branch), each with its
own driven adversarial-root gate.

**The two structural pins, ablated:**

```
ABLATION A (drop LastCommit from Hash()):
--- FAIL: TestCarrierHashDriftGuard
    Hash() does NOT cover LastCommit — the carrier's seating transition would ride on unsigned bytes

ABLATION B (move the carrier fold below the bond-reg loop):
--- FAIL: TestCarrierFoldPrecedesBondRegsInApply
    APPLY-ORDER PIN BROKEN: the carrier fold (stmt 7) runs AFTER the bond-reg loop (stmt 6)
```

## 7. Residuals

**Named and carried (from O1, unchanged):**

- The seat lands **one block late**. Monotone, benign, disclosed.
- Carrier bytes ≈ 100 B/signer. **CORRECTION (cert §10.1): the "bounded by R-membership" clause is
  WITHDRAWN — it never held, and the certification withdrew it from its own §12 too.** R-membership
  bounds the *qualified* / `validatorsSeen` sets; `validateCarrier` applies **no** qualification
  screen — it requires only `PhasePrecommit`, a verifying signature over `b.Prev`, and a distinct
  id, all of which **any freshly minted keypair** satisfies. Unqualified entries write nothing in
  `applyCarrier`, so they never enter the bounded set, but they are hash-covered, permanently
  committed, and `ed25519.Verify`-ed by every replica on every validation and every reload. Derived
  ceiling: `maxFrame` = 132 MiB (`adapters/tcpnet`) ÷ ~105 B per canonical-CBOR `Attestation`
  ≈ **1.3 million entries in one block**; the verification wall-clock is **UNMEASURED** and is owed
  on the real target before the freeze. Re-priced and made non-strippable rather than newly created:
  `b.Atts` already carried the same per-entry verify cost, but the carrier's bytes are hash-covered
  (not strippable by a serving peer) and are verified twice per validation. Tracked as
  **R-CARRIER-BYTES** in `ROADMAP.md` (Boulder 1 carry-list, the stamp-raising release). Still OPEN.
- **Proposer under-carry discretion, and `R-CARRIER-PREFIX-ONLY` (cert §5, held-in-tension).**
  "Carry everything you hold" is unenforceable — no replica can know what a proposer held — so a
  proposer can DELAY a seating. Downward-only: signatures are genuine and unforgeable, so it can
  never FORGE one. Named in a code comment on `HeadCarrier` and at the propose site, as O1 requires.
  **What `HeadCarrier` actually carries, stated exactly:** the parent's **stored certificate**
  (`head.Atts`), which is the **first-to-quorum prefix** — `finishPC` snapshots `pcs` at the moment
  the predicate holds (`chainrole.go:1028`) and discards every later reply (`:1059-1061`). So it is
  honest-maximal for any proposer that did **not** itself gather the parent, and **not** maximal for
  a node proposing consecutive heights: precommits received after the prefix closed were discarded
  and cannot be carried. Consequence, and it corrects an earlier framing here: an **honest** proposer
  can also delay a seating, not only a malicious one, and a persistently-slow attester is seated only
  once it makes a first-to-quorum prefix at some height. That is a **latency condition, not a
  permanent ceiling** — the §2.3(a) freeze is still closed. Downward-only and benign.
  **Not fixed in this round, and the reason is measured, not stylistic:** carrying "everything you
  hold" is not a producer-side one-liner, because the node **holds nothing more**. The late reply is
  dropped at `chainrole.go:1059-1061` (`if finishedPC { return }`, before `pcs = append` at `:1069`)
  and is never stored. Retaining it would need a new post-commit attestation store with its own
  lifecycle and memory bound inside the round machinery — beyond the producer, so it is out of scope
  for a text-only fold-in.
- An attester that attests the parent and lapses within it is never seated. Bounded, benign.

**New, opened by this round:**

- **R-CARRIER-PARENTPROPOSER** — **open, FP-1. Tracked in `ROADMAP.md` (Boulder 1).** The box's
  parent-proposer exclusion is anchored by the parent's own proposer signature over the hash-covered
  `b.Prev`, which proves *the named key signed `b.Prev`*, not *this key is THE parent's proposer*.
  **Two directions, per §3.3 (the original one-sided bound is WITHDRAWN):** **DROP** — an attacker
  holding key K can make the box skip K's OWN seat (bounded, downward-only, the discretion O1
  discloses); **ADD** — a freshly minted keypair verifies, matches no carrier entry, so nothing is
  skipped and the parent's TRUE proposer self-seats, requiring **no key of that proposer's**. ADD is
  a **wrong-accept** direction, at most one id per block, and it is exactly the id the rule excludes.
  Inert today (the box never-Accepts). **Certified fix direction (not built): a `tagLastProposer`
  committed scalar, Resolved against `prevStateRoot` — an additive open-era format change that needs
  its own certification and must land BEFORE the era-4 freeze.**
- **The "any single round" reading.** §3.2. Recorded as an interpretation, not a rule change. If
  the certifier reads O1 the other way (all entries at one common round), the change is one `if`
  in `validateCarrier`.

**Deliberately NOT done in this round (and why):**

- **The readiness stamp is NOT raised** (owner call O2 is not ratified here). G5/G6a/G6b encode
  the rollout rule as asserts without raising it.
- **Fork-choice weight is untouched** (owner call O3 — reserved for the owner, a separate round).
  `blockWeight`, `:3699`'s bare-hash verify and `heavier` are byte-for-byte unchanged. The carrier
  is shaped so a later round COULD derive block *h*'s weight from block *h+1*'s hash-covered
  `LastCommit`, but no weight code is added.
- **The canon amendment is not made** (owner call O4 — the rule text and its numbering are the
  owner's ratification, not a build). `docs/design/consensus-invariants.md` is untouched.
- **`SupportMeetsQuorum` and its two callers (I1/#402), the #432 prepare-QC/round/lock machinery,
  and the frozen era-3 format** are untouched. The carrier counts NOTHING — it is a seating
  witness, not a commit proof — which is exactly what keeps the quorum stack from forking.
- **G7** (the invariant-II driver pin) belongs to floor-box entry Part B, as the verdict files it.

**Stale-list items closed here** (converged verdict §9): the class-A header + derivation doc, the
`chain.go:3295` proposer-skip comment, `readSetAtts`'s per-attester rows and its `O(len(b.Atts))`
bound, the two `Populate*Roots` "the roots cover the block as it will actually commit" comments
(now qualified — false for v4, true for v5 under the carrier), the class-A tests that expected a
seat in the same block, and `docs/design/block-format-by-era.md`'s missing v5 row.

**Still open on the stale list:** the R1.6 class-A per-field oracle probes and the R1.8 class-A
target must not be declared final until re-pointed — they now read the carrier, which is the
re-point, but the DECLARATION is a certification act, not a build act.

## 8. Verification run

- `gofmt -l .` clean; `go vet ./...` clean.
- `go test ./...` **without `-short`** — every package `ok`, including the e2e tier (`e2e` 384 s)
  and the `sim` model-check tier (23 s). The consensus model-check tier that covers
  `validatorsSeen`/epochs is `core/chain/modelcheck_*_test.go` plus `core/node/modelcheck_*` (the
  tier-2 node-loop oracles); both are in that run and both are green, and G2 is filed at the
  tier-2 node tier alongside them.
- `scripts/gen_changelog.py` regenerated `website/changelog.html`; `scripts/check_links.py`,
  `check_status_headers.py`, `check_tenet_qualifiers.py`, `check_claims.py` all OK.
