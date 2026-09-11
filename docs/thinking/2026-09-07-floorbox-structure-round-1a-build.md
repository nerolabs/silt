# Floor-box STRUCTURE Round 1A — part 1 (the composition spine) — build record

Date: 2026-09-07 · Seat: BUILDER · Branch: `builder/floorbox-structure-1a` off main `5239625`.
Status: PACE-BEFORE-CODE record, written before the first `.go` edit; the per-step decisions below
are what the code implements. Deviations from the brief are listed in §4 with the reason each.

Governing documents (binding, read in full before this record):

- PE build brief §7 (steps 1–12; this pass is steps 1–5 + the stage-cover gate):
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md`
- P-table delta certification (the 24-stage table, M-1…M-5, G-D1…G-D12):
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md`
- Build-plan certification (the composition's definition, `StateView`, BG-1…BG-4):
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md`
- Owner call 16: MAIN-ONLY (Round 1A). Nothing from the box-entry round.

Donor: `builder/floorbox-structure @ 2ca01d9`, read with `git show`; never merged or rebased
(19 conflicting files, `carrier.go` add/add).

## 1. Mechanism (attribution before the patch)

The failure class is a box that reproduces a node predicate's TAIL without the precondition a
different stage established (N1: the author screen; RT2-CARRIER-13: the parent binding). It
recurred because the box and the node had two BODIES for one rule. This round writes the v5 accept
path ONCE, over a `StateView`, and dispatches the node to it for `Version >= 5`. Evidence: the PE
ruling §4(a) measured the drift live (main's `ValidateProposal` runs `validateIssuerKeys` and a
six-clause P5; the donor composition has neither), and the delta certification re-derived the
table at 24 stages.

## 2. Options weighed, per step

### Step 1 — the v5-driven fixture first

- (a) Reuse the donor `buildStructFixture` (4 anchors, objective, `Append` of a real v5 block).
- (b) Drive v2 blocks and call the composition directly. REJECTED: this is the vacuous-green trap
  the PE named (§4b). A fixture that never commits a v5 block makes every equivalence gate pass
  because the dispatch is never entered.

Decision: (a). Arm D asserts the committed block's `Version == BlockVersionWitnessable` AND that
`ValidateCommitV5(liveView{c}, b)` returns `Accept` on it; forcing `b.Version = 2` in `mkBlock`
must fail both.

### Step 2 — `StateView`, `HeadRef`, `Availability`, `Budget`

- `HeadRef` per M-3: `{Hash, NextHeight, ProposerID, StateRoot *Hash, LogRoot *Hash, Empty}`.
  The donor's `{Hash, Height, StateRoot, ProposerID}` is replaced, not extended: `Height` is
  renamed so the +1 semantics is in the name; the roots become absence-bearing.
- `Budget` per M-4: the ZERO value STALLS. Two constructors only: `UnlimitedBudget()` (what
  `liveView` returns) and `FrameBudget(maxBytes)` which refuses `maxBytes <= 0`. Byte-denominated
  (the PE's defect (c)1): step 0b measures the block's canonical encoded size (`Encode(b)`), the
  FRAME. The witness-side bytes are measured where the witness is looked at (the box entry, part 2).
  Option REJECTED: the donor's three entry COUNTS — `carrier.go:66-68` says distinct ids are free,
  so a count bounds nothing.
- `Params()` embeds `Config`, which already carries `MinProposerRep` / `MinAttesterRep` (M-1 is
  satisfied by the embedding; no new field). `StateView` gains `Rep(id) (int64, Availability)`.
- The ONE substituted step becomes `CommittedRoots(b) (FloorBoxOutcome, error)` (renamed from the
  donor's `StateRootPredicate(b) error`), because the proven side must be able to return
  `IndeterminateTrustlessly` for the k ≥ 1 LogRoot leg (M-4 / §3.3).

### Step 3 — `liveView` + `provenView`

- `liveView.Rep` = `(c.rep(id), Present)`; `provenView.Rep` = `(0, NoWitness)`.
- `liveView.Head()` reproduces `Chain.Head()` exactly, plus the parent's roots by pointer and
  `Empty` on a block-less chain.
- `liveView.CommittedRoots` = `validateEra3Roots(b)` (both roots, the node's own predicate).
- `provenView.CommittedRoots`: nil-root Reject (mirrors `era3validity.go` nil-reject); then P13b
  BEFORE P13a — the k = 0 equality `*b.LogRoot == *head.LogRoot` is O(1) and needs no witness,
  k ≥ 1 stalls with a named sentinel; then P13a through the recompute predicate. Order inside a
  conjunction is free; putting the O(1) leg first is what makes G-D7/G-D8 buildable in part 1
  without the box recompute wiring (part 2).

### Step 4 — the composition

- M-2: ONE composition, TWO entry points. `ValidateProposalV5` = 0 (L1 version partition),
  0b (budget), P1…P13; `ValidateCommitV5` = `ValidateProposalV5` then C1…C5.
- Consequence for the certification §2.2 order: P13 runs at the END OF THE PROPOSAL, before
  C1–C5 — the node's own order. §2.2 placed P13 after C1–C5 for the single-entry shape; M-2
  supersedes it. Same conjunction, node-identical order, no justification owed for a reorder that
  no longer exists.
- The S2 mode fence (§2.2 row 0a, "box-owned, node: no-op") is NOT in the composition. Under M-1
  the node's `liveView` takes the legacy branch through `v.Rep`; a fence keyed on `Objective()`
  inside the composition would stall a legacy node, which is the refuted direction (§2.3). The
  fence is box-entry code (part 2), where `provenView.Rep = NoWitness` already stalls a legacy box.
- The stage functions are MIRRORS over `StateView` (the donor's certified shape). They cannot be
  the node's `*Chain` methods: the composition takes no receiver and chain.go may not be touched
  outside the two dispatches. Consequence recorded in §4.
- Mirrors are re-derived against main line by line, not copied. Drifts found in the donor and
  corrected: P5 lacks `IssuerKeys`; P8b absent; P8 used `VerifyEquivocation` where main runs
  `SlashesBytesCap` + `CheckEquivocation` (R0.6); P4 returned the F2 message in legacy mode where
  the node's #572 branches are objective-only; `validateBondRegs` lacks the `!objective()` early
  return; `attesterQualifiedAt`/`proposerQualifiedAt` legacy legs absent.
- `nodeStages`: 24 rows, ordered, beside the composition, each row naming the node function (or
  `Inline`), the mirror, and for exactly one row `Substituted`.

### Step 5 — the dispatches

- Two hunks in `chain.go`, one at the top of `ValidateProposal`, one at the top of
  `ValidateCommit`. Both non-Accept outcomes refuse; a nil error with a non-Accept outcome is
  wrapped so the node never returns nil on a refusal.

### The STAGE-COVER gate (certification §4)

- Arm D first (fixture non-vacuity), Arm A (derived, ordered, transitive call cover with the
  floors ≥ 9 / ≥ 5), Arm B (sha256 of the comment-free bodies of exactly `ValidateProposal`,
  `ValidateCommit`, `requireQuorumStack`), Arm C (reverse cover).
- Arm A's derivation rule, exactly as certified: a kept call is a `c.<name>(...)` or bare
  `<name>(...)` whose name resolves to a package-level `FuncDecl` in {`chain.go`, `era3validity.go`,
  `issuerkey.go`, `carrier.go`} whose LAST result type is `error`. Transitive over the same rule.
- Arm C: every error-returning call in the composition (transitive over `validate_v5*.go`) is
  either a stage mirror in `nodeStages`, a listed HELPER mirror of a node helper that exists on
  main (`v5AttesterQualifiedAt` ↔ `attesterQualifiedAt`, …), or `stall`. An invented stage reddens.

## 3. Gates (RED-first, each recorded in the final report)

G-D1 (Arm D), G-D2 (Arm A), G-D3 (Arm B), G-D4 (Arm C), G-D5 (IssuerKeys-only parity, two
ablations), G-D6 (legacy-mode parity, accept + `MinProposerRep` refusal, `Rep` removal reddens),
G-D12 (both roots dispatch), G-5 (`liveView` never `NoWitness`; `NoWitness` is the zero),
G-D10 (`Budget{}` stalls; `FrameBudget(0)` refuses), and G-D7/G-D8 where they fall out of step 3.

## 4. Deviations from the brief and the certification, with reasons

1. **Mirrors, not calls.** Certification §4 Arm B says the composition "calls those stages rather
   than mirroring them". Over a `StateView` with no receiver it cannot: `validateTakedowns`,
   `validateBondRegs`, `validateIssuerKeys`, `ValidateEntry`, `validateSlashes`,
   `requireProposerPrepare`, `collectQuorumSigs`, `requireQuorumStack` are `*Chain` methods reading
   `c.<map>`. Only `validateCarrier` (receiverless) is called directly. Arm A therefore checks that
   each node stage's MIRROR is called, via a `Mirror` column in `nodeStages`. The honest bound this
   leaves: a predicate change INSIDE a mirrored stage body is seen by neither arm (the certified
   `R-PTABLE-DRIFT` bound, wider than the certification states). Surfaced to the planner; not
   widened here — Arm B's three-function scope is the certified calibration.
2. **P13 before C1–C5** (§2.2 row 8 vs M-2). M-2's definition of `ValidateCommitV5` fixes it.
3. **No S2 fence in the composition** (§2.2 row 0a). Required by M-1; the fence is box-entry code.
4. **P13b precedes P13a on `provenView`.** O(1) before O(payload); conjunction order is free; it is
   what lets G-D7/G-D8 be driven in part 1.
5. **`Budget` is frame-byte-denominated at step 0b.** The certified BG-3 is a "witness/frame BYTE
   budget"; at 0b no witness exists yet, so the frame is what is measured. Witness bytes are part 2.
6. **`ErrAboveCurrentEraVersion`** is defined here (the donor defined it in a box-entry file). The
   node never reaches it from the wire (`Decode` refuses `> 5`); the composition keeps the L1
   partition exact so an in-process caller cannot route a v6 block through the v5 rules.

## 5. Not built in this pass (part 2 of the brief)

Steps 6–12: P1-ahead-of-carrier in the box's recompute entry and the `CarrierParentProposerWitness`
deletion; N1; `NewBox` / the config-derived budget; the unexport sweep; the honest twins; the
glob widening; the `chain.go:736` comment correction and its two-sided gate (G-D11).

## 6. What landed (part 1), and the RED/GREEN record

Commits on `builder/floorbox-structure-1a`: `21aeaad` (step 1 + this record), `c7df16b` (steps 2–5 +
gates). `git diff main -- core/chain/chain.go` is exactly the two dispatch hunks, 28 insertions,
zero deletions; every `errors.Is` sentinel and every #572 attribution branch is byte-untouched.

| Gate | Ablation (one line) | RED line |
|---|---|---|
| G-D1 | `mkBlock` mints `BlockVersionRounds` | `FIXTURE VACUOUS: the committed block is v2, want v5`; arms A–C: `arm D — the fixture did not commit a v5 block` |
| G-D2 | delete the `v5ValidateTakedowns` arm | `arm A COVER — node stage P6 (validateTakedowns) is not carried` |
| G-D3 | `&& true` on the P5 chain in `chain.go` | `arm B — … hash to 4d26a58a…, pinned 42b27dc5…` |
| G-D4 | call an invented `v5ValidateInvented` | `arm C — the composition calls v5ValidateInvented, which is neither a stage mirror … nor a listed helper` |
| G-D5a | drop `len(b.IssuerKeys) == 0` from P5 | `G-D5 VIOLATED (P5): ValidateProposal refused an IssuerKeys-only v5 block: chain: empty block` |
| G-D5b | drop the `v5ValidateIssuerKeys` call | `G-D5 VIOLATED (P8b): … must refuse an UNBONDED issuer's registration by name; got <nil>` |
| G-D6 | `liveView.Rep` → `NoWitness` | `G-D6 VIOLATED (accept): … got … no witness for a required committed read — stall: rep[…] (legacy mode)` |
| G-D12 | revert the `ValidateProposal` dispatch | `G-D12 — ValidateProposal's FIRST statement must be the v5 dispatch` |
| G-5 | `liveView.Slashed` → `NoWitness` | the FIXTURE reddened: `fixture h1 must COMMIT … stall: slashed[proposer]` — a liveView stall costs the node a refusal, never an acceptance, exactly as the dispatch promises |
| G-D10 | zero `Budget` treated as unlimited | `G-D10 VIOLATED: the zero Budget must STALL at step 0b with ErrWitnessBudgetUnset; got … no witness` (the view proceeded past 0b) |
| G-D7 | delete the k = 0 LogRoot equality | `G-D7 VIOLATED (provenView): … got INDETERMINATE_TRUSTLESSLY / … recompute is research-gated` (the forged root passed P13b) |
| G-D8 | delete the k ≥ 1 stall | `G-D8 VIOLATED: … got REJECT / … LogRoot does not equal` (a stall rendered as a disproof) |

Every ablation was restored and re-run GREEN. Suites: `go test -short ./core/chain/` green after the
one classification the round owed — the verifier-inventory pin (`TestO3T_VerifierInventoryPin`)
enumerates every `ed25519.Verify` site, and the two mirrored sites (P3 in `ValidateProposalV5`,
`v5ValidateBondReg`) are classified `proposer-sig` / `bondreg-sig`, the classes of the node
functions they mirror. No attestation verifier was added: the composition's attestation verifies
route through `verifyAtt`.

Arm B digest pinned: `42b27dc5d6485dca09653d6df8969aa2e5df2e9c241a3c7bdc01bdea4172319a` over the
comment-free bodies of `ValidateProposal`, `ValidateCommit`, `requireQuorumStack` at `c7df16b`.

Owed to the planner (not this seat's file): a CHANGELOG line for the `core/chain` change; register
rows for `R-PTABLE-DRIFT`, `R-COMPOSITION-LEGACY-LEG` (closes with M-1 shipped here),
`R-BOX-STALLS-ON-TAKEDOWN`; the `R-STATEROOT-EQUIVALENCE-SEAM` rename the certification asks for.

---

# Part 2 (steps 6–12, G-D9, G-D11) — build record

Date: 2026-09-08 · Seat: BUILDER · Branch: `builder/floorbox-structure-1a` on `5e830c3`.
PACE-BEFORE-CODE record, written before the first `.go` edit of this pass. Same governing
documents as Part 1; binding "Do NOT build / carry / touch" list unchanged (no `AdoptPin`,
`PinAdoptionInput`, `BoxQuorumSupport`, `BoundaryQuorumVerdict`, no box-entry round-A file).

## 7. Options weighed, per step

### Step 6 — P1 ahead of the carrier leg; the witness parent-proposer deleted

Mechanism (RT2-CARRIER-13 + R-CARRIER-PARENTPROPOSER, ADD direction): the box's recompute entry
took `(b, parentStateRoot, witness)` and had no position of its own, so (a) the carrier was
verified over whatever `b.Prev` the block's author chose, and (b) the ONE class-A input that is
not a committed leaf — the parent's proposer id — was read from the witness, anchored only by "some
key signed b.Prev", which a fresh keypair satisfies (`carrier.go` doc, both directions).

- (a) Keep the witness fields and add a HeadRef cross-check. REJECTED: two sources for one fact is
  the seam class this round exists to close; the cross-check would be a third body.
- (b) The parent proposer comes from the view's OWN head record (`HeadRef.ProposerID`, class 3,
  M-3) and is THREADED into the recompute as a parameter the box door supplies. The witness fields
  and both `carrier.go` functions are deleted; the type has no slot for a driver to fill. CHOSEN.
  This is the donor's shape (`recomputeStateRootEntriesRevocations(…, parentProposer)`).
- Where (0a) goes. The composition's P12 calls `validateCarrier` AFTER P1–P4; the recompute's own
  (0a) call stays, because 21 files drive the recompute directly and the 2026-09-03 PE ruling's
  merge-condition 1 ("the box reproduces it by CALLING validateCarrier") is a condition on the
  recompute. Consequence: a block validated through the box door pays `validateCarrier` twice
  (P12 and (0a)); both are O(|LastCommit|) verifies and the door's budget now bounds the frame.
  Recorded as a cost, not hidden. The brief's "done when" — (0a) no longer PRECEDES P1–P4 — holds:
  the door reaches the recompute only through P13, after P1–P12.
- The box door. The brief's step 8 requires `NewBox`; no `Box` exists on main. Shape chosen:
  `NewBox(ch *Chain, parent Block, src WitnessSource, d RecoveryDirective) (*Box, error)` — the
  head record is DERIVED from a parent BLOCK the box holds (BG-2's premise, the HeadRef half the
  delta certification names deliverable in 1A), never declared as a bare hash. The N2/N3/N6/N7
  residual closures (pin adoption, held height, lineage) stay 1B. The door is
  `(*Box).Validate(b, w)`: budget (frame + witness bytes) → cold-auditor recovery decision → pruned
  refusal → `ValidateCommitV5(provenView, b)` with the P13a predicate wired to the recompute →
  the R1.8 downgrade (an Accept is returned as `IndeterminateTrustlessly` / `ErrRecomputeGated`,
  one line, so the flip is one line to remove and reviewed on its own).
- The existing `(c *Chain) WitnessValidateV5(b, parentStateRoot, d)` stays as-is (the delta
  certification §6 lists it three-parameter under 1A). It is one of the two exported doors.

### Step 7 — N1, the author screen in the standalone weight recompute

`recomputeEpochWeightQuorum` credits `proposer` with no screen because the NODE's
`requireEpochWeightQuorum` has none — `proposerQualifiedAt` (P4) refused a slashed author before the
tally. A direct caller of the standalone recompute has no P4, so it reproduces the tail alone: N1.
Fix: `EpochSetWitness.AuthorSlashedProof` — the box Resolves `slashed[proposer]` against the
committed root; proven ABSENT ⇒ credit; proven PRESENT ⇒ `(false, ErrRecomputeAuthorSlashed)`;
anything else ⇒ stall. The comment is rewritten to name `proposerQualifiedAt` as the screening
stage. The composition's `v5RequireEpochWeightQuorum` mirror is NOT changed: there P4 precedes it,
which is the node's own order and the structural closure of N1.

### Step 8 — BG-3 finished: box-owned, config-derived, witness bytes counted

- `Config.FloorBoxBudgetBytes int` (operator config, class 3). `NewBox` derives `ByteBudget` from it
  and REFUSES construction when it is unset (≤ 0). No constructor takes a Budget parameter.
- `FrameBudget`/`MaxFrameBytes` are renamed `ByteBudget`/`MaxBytes`: the ceiling now covers frame +
  witness, and the name must not claim less. Part 1 is unmerged, so no compatibility is owed.
- The witness is measured structurally (a reflection walk summing byte slices, hashes and ids over
  the exported carrier fields; `statehash.Witness` reports its own proof bytes) — not by re-encoding
  a possibly-huge witness. The measurement is O(witness) and allocation-free.
- The door checks `frame + witness > MaxBytes` BEFORE the composition, which then re-checks the
  frame alone at 0b (redundant on the box, load-bearing on any other caller of the composition).

### Step 9 — the exported-door sweep

Unexported: `recomputeStateRootEntriesRevocations`, `recomputeEpochWeightQuorum`,
`recomputeMatureNow`, `recomputeMatureNowStreaming`, `recomputeQualifiedCount`,
`recomputeDeMatureSuperQuorum`, `recoveryBoundaryDecision`, plus the exported witness helper
deleted in step 6. Kept: `WitnessValidateV5` (the pre-structure door, never-Accept) and
`WitnessReadSetV5` (fenced: legacy mode ⇒ nil; documented as a producer that expresses no verdict).
Gate G-6 derives the inventory by AST over `floorbox_*.go` + `readset_v5.go`.

### Step 10 — NG-2, honest twins

Carrier gates: `assertHonestTwinAgrees` on every gate (1, 1b, 1c×3, 1d, 12). Structure gates: an
`assertHonestTwinAccepts` helper (the composition over liveView Accepts the fixture's honest block)
called from every gate in `redteam_floorbox_structure_gate_test.go` and the three stage-cover arms.
A meta-gate counts twin call sites against `Test*` declarations in those files by AST.

### Step 11 — NG-4 tail

`foldFileGlob = "floorbox_*.go"`, floor raised to the measured file count (12 on main + the new
box file = 13); the gate asserts the glob matches every non-test `floorbox_*.go`. The new box file
is deliberately INSIDE the pin: its `*Chain` reads go through `liveView`/self-dispatch, never a
bare `c.<map>`.

### Step 12 — `chain.go` comment correction + G-D11

Comment-only hunk inside `Hash()`: ADDING carrier entries is exactly as free as DROPPING them
(harvest the parent's published `Atts`; `Hash()` returns `Pruned` unchanged); the descendant's
hash-covered `StateRoot` is the ONLY defence; the consequence under the longest-valid-prefix
contract is a SILENT head truncation at the first non-pruned descendant. G-D11 drives exactly that
through `Reload` and asserts both sides; `TestPrunedBlockHashDoesNotCoverCarrierOrStateRoot`'s
clause (4) is corrected in place (it asserted the refuted claim).

### G-D9 — the `m = 1` right-spine control

A red-team-style CONTROL, labelled as such: with a witness-supplied `m = 1`, a forged
`b.LogRoot = nodeHash(L_p, leafHash(leaf))` PASSES `VerifyConsistency` + the inclusion leg; with
`m` authenticated as the true size, the same input is REFUSED. Built over `core/translog` directly
with the chain's `RevocationLeaf`; the interior-node hash is re-derived in the test and pinned
against `translog.MTH` of a two-element list.

## 8. Gates in this pass (each RED-first; the record is in §9)

G-3 (RT2-CARRIER-13 through the door), G-A (the class-A exclusion is box-owned), N1, G-7 (the
witness leg), G-6 (door inventory + the read-set fence), G-4 (twins; the meta-count), NG-4, G-D11,
G-D9.

## 9. What landed (part 2), and the RED/GREEN record

Two commits on `builder/floorbox-structure-1a` after `5e830c3`: steps 6–10 (the box door, the
witness parent-proposer deletion, N1, BG-3, the door sweep, the twins), then steps 11–13 (the glob,
the `chain.go` comment + G-D11, G-D9). `git diff main -- core/chain/chain.go` is the two dispatch
hunks plus ONE comment-only hunk inside `Hash()`; zero code lines changed there.

Files (new): `core/chain/floorbox_box_v5.go` (`BoxConfig`, `Box`, `NewBox`, `(*Box).Validate`,
`witnessBytes`), `core/chain/floorbox_box_v5_test.go` (the head-derived recompute drivers,
`proverSource`, the door gates), `core/chain/floorbox_door_inventory_v5_test.go` (G-6, the read-set
fence cover, G-4), `core/chain/redteam_revlog_size_control_v5_test.go` (G-D9).
Files (changed): `carrier.go` (two functions deleted), `floorbox_recompute_stateroot_v5.go`
(fields deleted; `recomputeStateRootEntriesRevocations` + `assembleStateRootRecomputeOps` take
`parentProposer`), `floorbox_recompute_stateroot_atts_v5.go` (id threaded, pub/sig gone),
`floorbox_recompute_v5.go` (N1 screen, `AuthorSlashedProof`), `stateview_v5.go` (`ByteBudget`,
`MaxBytes`, `Budget.Check`), `validate_v5.go` (0b through `Check`), `readset_v5.go` (fence + doc),
`floorbox_v5.go` / `floorbox_recompute_{maturity,qualifiedCount,dematureQuorum,stateroot_maturitylatch}_v5.go`
(unexported), `core/statehash/witness.go` (`Witness.Bytes`), 30 test files (re-homed drivers, the
twins, the inventory row, the `FrameBudget` rename), `pruned_block_test.go` (clause 4 corrected,
G-D11), `chain.go` (the comment).

| Gate | Ablation (one line) | RED line |
|---|---|---|
| G-3 (RT2-CARRIER-13 through the door) | derive `v.head.Hash/NextHeight` from `b.Prev/b.Height` in `Validate` | `RT2-CARRIER-13: the box must refuse a stale-but-valid replay on the PARENT BINDING, by name; got INDETERMINATE_TRUSTLESSLY / … stall: ancestors(8)` (the verdict no longer names P1) |
| G-A (exclusion box-owned) | `parentProposer = ports.NodeID{}` at the top of the class-A write-set | `--- FAIL: TestClassA_ParentProposerExclusionIsBoxOwned` (the box-owned arm no longer agrees with the node's root) |
| N1 | delete the (1b) author screen | `N1 VIOLATED: a slashed-but-frozen author must NOT be credited … got met=true / <nil>` |
| G-7 (witness leg) | charge the frame alone at the door | `G-7 VIOLATED: frame + witness above the box's ceiling must STALL on the budget before any crypto; got REJECT / chain: bad signature: proposer` |
| G-6 | declare `func (c *Chain) TenthDoor()` in `floorbox_v5.go` | `SOURCE GATE: G-6 — the box files declare exported *Chain methods [TenthDoor WitnessReadSetV5 WitnessValidateV5]; exactly [WitnessReadSetV5 WitnessValidateV5] are permitted` |
| G-4a (twins, carrier + door) | `assembleStateRootRecomputeOps` returns `ErrRecomputeBoxWiring` unconditionally | every carrier gate (1, 1b, 1c×3, 1d, 12) and every door gate RED: `NON-VACUITY BROKEN (warm tier): the box stalls on the HONEST twin …` |
| G-4b (twins, structure) | `v5CheckBudget` returns `ErrWitnessBudgetUnset` unconditionally | all 11 structure/stage-cover gates RED: `NON-VACUITY BROKEN: the node refuses the honest twin (… witness budget is unset …)` |
| G-4c (the meta-count) | delete the twin call in `TestGD12` | `SOURCE GATE: G-4 — TestGD12_BothNodeEntryPointsDispatchToTheComposition … carries NO honest twin` |
| NG-4 | `foldFileGlob = "floorbox_*_v5.go"` | `PIN VACUOUS: only 12 fold files matched "floorbox_*_v5.go" (floor 13)` and `SOURCE GATE: NG-4 — foldFileGlob … covers 12 of 13 non-test floorbox_*.go files` |
| G-D11 | delete `validateEra3Roots` from `appendStructural` | `G-D11 (ii): the rewritten pruned ancestor must be ACCEPTED and the first non-pruned descendant REFUSED … got n=5 err=<nil>` (the forgery survives whole) |
| G-D9 | (built in) authenticate `m = 3` on the same forged input | arm (2) asserts REFUSED; arm (1) asserts the witness-supplied `m = 1` PASSES both legs |
| door twin | `Validate` stalls unconditionally | `NON-VACUITY BROKEN: the honest block must run the composition through the door to the R1.8 downgrade … got … pruned-block leg … stall` |

Every ablation restored and re-run GREEN. Suites: `go test -short ./core/chain/` green after ONE
classification the round owed — the verifier-inventory pin (`TestO3T_VerifierInventoryPin`) saw the
new `ed25519.Verify` in `NewBox` (the parent block's own proposer signature at construction) and it
is classified `proposer-sig`, the class of the node site it mirrors; `core/statehash`, `core/translog`
green; `go vet ./...`, gofmt clean; `check_cited_tests.py`, `check_source_gates.py`,
`check_residual_register.py` exit 0.

## 10. Deviations from the brief (part 2), with reasons

1. **`BoxConfig`, not a `Config` field, carries the budget.** Step 8 says "derived from `Config`".
   `Config` lives in `chain.go`, and this pass is allowed exactly ONE further `chain.go` hunk, which
   is the step-12 comment. `BoxConfig{BudgetBytes, Recovery}` is box-owned operator config in the
   same sense as `Config` (set at construction, never per block), `NewBox` refuses an unset value,
   and no type can express ∞. Whether the field should migrate into `Config` in a later pass is the
   planner's call; it costs one `chain.go` line.
2. **`validateCarrier` runs twice on the door path** (§7 step 6). Recorded, bounded by the budget.
3. **The recompute's 5th parameter is threaded through test helpers** (`recomputeViaHead`,
   `assembleOpsViaHead`, `coldRecompute(warm, …)`) rather than editing 134 call lines; every driver
   still exercises the real unexported entry, and the parent proposer comes from the stand-in head.
4. **The class-A "foreign id diverges" arm hands the witness the spurious seat's own proofs**, so the
   arm ends in a proven `ErrRecomputeStateRootMismatch` rather than a missing-witness stall — a
   stronger demonstration than the brief's minimum.
5. **`requireV5Fixture` counts as the stage-cover arms' twin** in G-4's meta-count: arm D IS the
   twin (it calls `assertHonestTwinAccepts`), and renaming it would churn the certified gate.

Owed to the planner: a CHANGELOG line for `core/chain` + `core/statehash`; register rows already
owed from part 1 stand; NO new `R-` name was minted in this pass (`R-CARRIER-PRUNED-HASH`,
`R-CARRIER-PARENTPROPOSER`, `R-CARRIER-BYTES` are cited, all pre-existing). The
`R-CARRIER-PARENTPROPOSER` row can be re-dispositioned: the witness slot is gone. (CORRECTED by the
PE ruling §3, see §12 below: the ADD direction is NOT "unrepresentable in 1A" — it is representable
at `NewBox`'s `parent Block` parameter and inert only by the R1.8 downgrade plus the absence of a
non-test caller; it folds into `R-DRIVER-ASSERTED-BOXSTATE` and closes with the 1B pin.)


---

# Review fixes (the two blind rulings on `869399e`) — build record

Date: 2026-09-08 · Seat: BUILDER · Branch: `builder/floorbox-structure-1a`, seven commits on `869399e`.
Rulings folded in, both blind on the same commit:

- PE: `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-floorbox-structure-round-1a-869399e-2026-09-08.md`
  (F-1, F-2 merge-blocking; F-3, F-5, F-6, F-7, F-8 record/hygiene; §3 the `R-CARRIER-PARENTPROPOSER` coupling).
- Researcher: `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md`
  (M-1A-1…M-1A-5; §3.1 G-D13; §7.2 the eight undriven mirrors; §7.3 the parity oracle).

## 11. The items, each its own commit, each with its ablation

| # | Item | Files | Ablation | RED line | GREEN |
|---|---|---|---|---|---|
| 1 | **M-1A-1** `v5MatureNow` calls the receiverless `nakamotoCoefficient`; the inline closure and its `threshold` deleted | `validate_v5_quorum.go`; new `validate_v5_maturity_gate_test.go` (`TestM1A1_V5MatureNowEqualsNodeMatureNow`, twelve committed worlds: degenerate total, equal bonds, whale, shared domain, operator margin, the anchor / slashed / below-MinBond exclusions) | a divergent inline coefficient (`cum >= total/3`) | `M-1A-1 VIOLATED (three equal bonds, MatureValidators 2): the node's matureNow answers true, the composition's v5MatureNow answers false` | `ok` |
| 2 | **M-1A-2** the `WitnessValidateV5` comment rewritten | `floorbox_v5.go` | comment-only; no gate | — | the door named is `(*Box).Validate`; the parameter-taking shape is stated as RT2-CARRIER-13, with "do NOT build the recompute here" |
| 3 | **M-1A-4** G-D6 relabelled `TestGD6_LegacyRepLegOracle`; the `nodeErr.Error() != compErr.Error()` clause struck | `redteam_floorbox_structure_gate_test.go`; the two citations in `composition_stage_cover_v5_test.go` | `liveView.Rep` → `NoWitness` | `NON-VACUITY BROKEN: the node refuses the honest twin (… stall: rep[proposer] (legacy mode))` | `ok` |
| 4 | **PE F-1** the fold-pin matcher widened to TYPE reachability: a Chain method's receiver, a `*Chain` parameter, a receiver-field alias (`s.c`) resolved from the fold files' own struct declarations, a local `x := s.c`; the classifier (`foldPinIndex.liveReads`) shared with the teeth test, which re-injects all four shapes; `(*Box).view`'s `s.c.verifyBond` given a site-scoped entry with its reason; the site map now carries a reason per site | `floorbox_recompute_foldlivestate_pin_v5_test.go` | (a) the ruling's `func (s *Box) ablationLiveRead() (bool, int) { return s.c.matureEpoch, len(s.c.epochSet) }`; (b) the `view` site entry removed | (a) `FOLD FILES READ LIVE BOX STATE (2 site(s)): floorbox_box_v5.go:229 s.c.epochSet (in ablationLiveRead) … s.c.matureEpoch (in ablationLiveRead)`; (b) `floorbox_box_v5.go:133 s.c.verifyBond (in view)` — the pre-existing read the old matcher never saw | `ok` |
| 5 | **PE F-2** `StateView` SEALED (`sealedStateView()`, implemented by `liveView` and `provenView` only); **G-6b** `TestG6b_ExportedPackageSurfaceInventory` derives every exported func / method-on-exported-receiver / type / const over `floorbox_*.go` + `validate_v5*.go` + `stateview*_v5.go` + `readset_v5.go` and holds it to `exportedPackageSurface` (a reason per entry; `Err*` vars excluded by rule) and asserts the seal; the never-Accept sentence in `floorbox_box_v5.go` names `(*Box).Validate` as the property's owner; **F-7** the dead `twins != gates` check deleted | `stateview_v5.go`, `stateview_live_v5.go`, `stateview_proven_v5.go`, `floorbox_box_v5.go`, `floorbox_door_inventory_v5_test.go` | (a) `func ExportedAblationDoor() {}` in `validate_v5.go`; (b) the seal deleted from the interface and both views | (a) `G-6b — … NEW (not listed): [func ExportedAblationDoor (validate_v5.go)]`; (b) `G-6b — StateView must carry the unexported seal method sealedStateView …` — and an out-of-package `fake` implementing all 24 methods **compiled and drove `ValidateCommitV5`** with the seal gone; with the seal present it fails to compile: `fake does not implement chain.StateView (missing method sealedStateView)` | `ok` |
| 6 | **M-1A-3** the v4/v5 PARITY ORACLE `TestM1A3_V4V5ParityOracle` (§13) | new `parity_oracle_v4v5_test.go`; `floorbox_door_inventory_v5_test.go` (the oracle and the maturity gate join `twinGateFiles`) | seven single-clause deletions, one per mirror (§13.3) | each RED names the regime and the case (§13.3) | `ok`, all nine regimes, eight mirrors recorded |
| 7 | **M-1A-5 / G-D13** `TestGD13_PerRowNodeBodyDigests`: sha256 of the comment-free body of all 28 node functions that `nodeStages` (transitively) or `compositionHelpers` name, pinned in `nodeBodyDigests`; a red names the function and every row/helper that mirrors it; floor 24; a pin with no mirroring row reddens; **F-5** the two accelerator substitutions named in `nodeStages`' doc with the drift guard | `composition_stage_cover_v5_test.go`, `validate_v5.go` (doc only) | `&& true` inside `validateBondRegs` | `G-D13 — a node body the composition MIRRORS changed (or is unpinned): validateBondRegs: body CHANGED — hashes to 40586a06544b095d, pinned ec0fdfd68d488ca5; mirrored by [P7 (v5ValidateBondRegs)]` | `ok` |

Every ablation restored; `git status` clean of everything but the commits.

## 12. Record corrections the PE ordered (F-3, F-6, F-8, §3)

- **F-3, the v6 delta, stated plainly.** For an in-process v6 block the pre-round node ACCEPTED (no
  upper version bound; the era-2 body plus `validateEra3Roots` ran); the composition REJECTS it
  (`ErrAboveCurrentEraVersion`, step 0). Refuse-more, wire-unreachable (`Decode` refuses `> 5`), and
  the delta certification's §2.2 row 0 "node: no-op" is therefore not exact. The code stays.
- **F-6, the second Q3 implementation.** `v5RequireEpochWeightQuorum` (no author screen; P4 runs
  first) and the standalone `recomputeEpochWeightQuorum` (the N1 screen; no production caller,
  reached from three fold files) are two live reproductions of Q3 with different screen sets.
  `R-SHARED-RULE-BLINDSPOT` grows in that second direction; the closer is deleting the standalone
  when its direct drivers migrate to the door (the same event the certification §3.6 names for the
  (0a) `validateCarrier` call).
- **§3, the `R-CARRIER-PARENTPROPOSER` ADD direction.** Struck from §10 in place: it is representable
  at `NewBox`'s `parent Block`; inert by the R1.8 downgrade and the absence of a non-test caller, not
  by the type. It folds into `R-DRIVER-ASSERTED-BOXSTATE` and closes with the 1B pin. This round is
  not to be credited with closing it.
- **F-8, the evidence line.** The merge evidence for `core/chain` is `go test -timeout 40m ./core/chain/`
  (the default 600 s fails inside `TestMeasureRecomputeMatureNowFoldCost` at N = 10⁶). **Not run in
  this pass**: the owner's load rule for this run is `-short` only. What is held: `go test -short
  ./core/chain/` `ok` (27.3 s), `go vet ./core/... ./cmd/...` clean, `go build ./...` clean, the
  three lints exit 0. The `-timeout 40m` run is owed at the merge commit.

## 13. The parity oracle — construction, what each regime proves, and the ablation record

### 13.1 Shape

`parityWorld.pair` mints the SAME content at `BlockVersionStateRoot` and `BlockVersionWitnessable`
on the same committed head; `assertParity` runs both through `ValidateCommit` (the v4 twin takes the
node's era-3 bodies verbatim; the v5 twin dispatches to the composition) and asserts the same
verdict, the same sentinel (`errors.Is`) and the same rendering. The ONE permitted text departure is
`rootsRendered` (P13 renders version-specific roots). Committed history is v5 with the `LastCommit`
carrier harvested from the parent's precommits, so seating goes through the carrier, never Atts.

**A trap the fixture principle hid:** a v4 twin's StateRoot depends on WHO attests — apply seats
`validatorsSeen` from `b.Atts` for a sub-v5 block, the era-3 non-hash-covered transition input —
while the attestations sign the hash that covers the root. `mint` therefore runs two passes (attach
the attester set provisionally → roots → sign → re-issue). The era-3 fixtures never hit this because
they mint v4 blocks with no attesters. For the v5 twin the second pass changes no root, which is the
property era-4 was built for, observed.

### 13.2 Regimes and evidence (each also carries the honest twin, NG-2)

| Regime | World | Accept | Refusals by name | Mirrors recorded |
|---|---|---|---|---|
| `objective-noepochs` | 4 anchors, epochs off, handed off at genesis | honest | P6 `ErrRevokeUnknownRoot`, C1 `ErrProposerPrepare`, Q1 `ErrNoQuorum`, P1 `ErrWrongParent`, P13a `ErrEra3StateRootMismatch` (roots rendered); **P7 pruned leg, both arms**: at/above the reader's floor `ErrPrunedAboveHorizon`; below it (`trustFloorOverride`, the receiver's anchor a Reconcile replay threads) Answer-less regs ACCEPTED and a smuggled Answer `ErrMalformedPruned` | `v5ValidateBondRegs` (pruned arm) |
| `launch-window` | MatureValidators 2 unmet, two bonded non-anchors | anchor proposes with anchor majority | P4 anchor-only arm `ErrLowReputation`; Q2 `ErrAnchorRequired` (Q1 met by 3 qualified) | — (Q2 / `v5HandedOff`) |
| `mature-epoch` | EpochBlocks 2, frozen at genesis (4 anchors, 8 MiB); a newcomer bonded mid-epoch | full frozen weight | Q3 `ErrNoQuorumWeight` at 4 of 8 MiB; P4 frozen arm `… not in the frozen epoch set governing height 2`; the attester frozen arm drops the newcomer → `ErrNoQuorum` | `v5RequireEpochWeightQuorum`, `v5RequireProposerQualified` (+ `v5AttesterQualifiedAt`'s frozen arm) |
| `reggate-active` | as above + TTL 32 (R = 10), gate from h > 1 | a fresh identity's first reg; a LAPSED frozen member re-proving its own root inside R (the shipped #535 fix-(4) surgery, `delete(c.bonded, member)`) | `ErrRegGate` twice-in-one-block; `ErrRegGate … re-registered 2 blocks after its last reg (R=10)` for a member with LIVE standing | `v5RegGateActive` (active arm), `v5RestoresHeldStanding` (both values) |
| `recovery-boundary` | EpochBlocks 2, LivenessRecoveryHeight 2, a 16 MiB joiner bonded at h1 (not frozen); a twin world with the directive OFF | ON: anchors + joiner; the joiner ALONE carries the weight quorum; the joiner PROPOSES | OFF: the same blocks → `ErrNoQuorum` (joiner dropped), `ErrLowReputation` (frozen arm) | `v5EffectiveEpochSet` (recovery arm, load-bearing), `v5RequireEpochWeightQuorum` (live set) |
| `demature` | MatureValidators 2; three 2 MiB validators seated at h2 via the carrier (latch), a 32 MiB whale seated at h3 | mature-and-decentralized (Q4 skipped); de-matured with the whale in the coalition | de-matured, whale absent: `ErrDeMatureQuorum … 14 MiB of 46 MiB bonded (need ≥31 MiB)` | `v5MatureNow` (true then false, on the accept path), `v5RequireDeMatureSuperQuorum` |
| `legacy` | `buildLegacyFixture` (MinBond 0) | honest | `ErrLowReputation` (proposer rep 10); `ErrNoQuorum` (two attesters demoted) | — (the G-D6 differential) |
| `era3-active` | Era3ActivationHeight 1 | both twins satisfy P10 | — | — |
| `era4-active` | Era3 1, Era4 2 | v5 accepted | **expected by-rule divergence**: the v4 twin `ErrEra4VersionRequired` | — |

The closing check asserts all eight names in `uncoveredMirrors` were recorded AND are real
composition functions (a renamed mirror reddens).

### 13.3 Ablations (one mirrored clause deleted each; all restored)

| Mirror ablated | RED line (regime / case) |
|---|---|
| `v5ValidateBondRegs`: the `seenReg[id]` twice-in-one-block check | `PARITY VIOLATED (reggate-active / the same identity registered twice in one block (P7 gate arm)): both twins must be REFUSED with … ErrRegGate` |
| `v5RestoresHeldStanding` returns false | `PARITY VIOLATED (reggate-active / a LAPSED frozen member re-proves its own root inside R …): both twins must be ACCEPTED` |
| `v5EffectiveEpochSet`: the recovery arm | `PARITY VIOLATED (recovery-boundary/on / the non-frozen joiner's attestation carries the weight quorum (recovery arm)): both twins must be ACCEPTED` |
| `v5RequireEpochWeightQuorum`: `2*total` → `total` | `PARITY VIOLATED (mature-epoch / coalition holds half the frozen weight (Q3)): both twins must be REFUSED` |
| `v5RequireDeMatureSuperQuorum`: `need` → 0 | `PARITY VIOLATED (demature / de-matured, the whale absent (Q4: 14 of 46 MiB)): both twins must be REFUSED` |
| `v5MatureNow`: `k+1 >=` | same de-mature case (Q4 skipped on the v5 side) |
| `v5RequireProposerQualified`: the frozen-set membership test | `PARITY VIOLATED (mature-epoch / mid-epoch newcomer proposes (P4 frozen arm))` and `(recovery-boundary/off / the joiner proposes with the directive OFF …)` |

## 14. Deviations from the fix brief, with reasons

1. **The G-D13 digests live in the TEST file** (`nodeBodyDigests`, keyed by node function), not as
   a `NodeDigest` field on the production `nodeStages` rows. The production table carries no test
   pins (arm B's digest already lives in the test file), and the attribution the certification asked
   for is preserved: a red names the function and every row/helper that mirrors it.
2. **`StateView` is sealed** — one unexported method — beyond the PE's "inventory + doc" fix. The
   inventory documents the surface; the seal turns the never-Accept claim into a compile-time fact
   (an out-of-package fake fails to compile). One line, structural, reviewed under G-6b.
3. **G-D6 keeps only the node's door** (`ValidateCommit`), not a second call to `ValidateCommitV5`:
   under M-2 they are one function.
4. **P7's pruned leg is driven on both arms**, the below-floor arm through `trustFloorOverride`. The
   box's `provenView.TrustFloor` answers 0 always (its door stalls on a pruned block first), so on
   the box the leg is unreachable; on the node it is the Reconcile-replay path's rule.

No new `R-` name minted. Cited from the certification: `R-SECOND-DOOR-COMMENT` (closed by M-1A-2),
`R-QUALIFIED-ACCELERATOR-SAFETY` (named in `nodeStages`' doc), `R-MIRROR-COVERAGE-REGIME` (closed by
M-1A-3), `R-PTABLE-DRIFT` (re-scoped; bounded by G-D13). Owed to the planner: the CHANGELOG line;
the register rows above; the `-timeout 40m` run at the merge commit.
