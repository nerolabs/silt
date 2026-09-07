# Floor-box STRUCTURE Round 1A — part 1 (the composition spine) — build record

Date: 2026-09-07 · Seat: BUILDER · Branch: `builder/floorbox-structure-1a` off main `5239625`.
Status: PACE-BEFORE-CODE record, written before the first `.go` edit; the per-step decisions below
are what the code implements. Deviations from the brief are listed in §4 with the reason each.

Governing documents (binding, read in full before this record):

- PE build brief §7 (steps 1–12; this pass is steps 1–5 + the stage-cover gate):
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md`
- P-table delta certification (the 24-stage table, M-1…M-5, G-D1…G-D12):
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md`
- Build-plan certification (the composition's definition, `StateView`, BG-1…BG-4):
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md`
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
