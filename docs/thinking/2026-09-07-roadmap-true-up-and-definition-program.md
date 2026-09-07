# 2026-09-07 — Roadmap true-up and definition program: the critical path to the Release Candidate

**Purpose.** The owner's directive for this session: *"a full boulder and task true up before we
resume any new work … as little undefined work in the roadmap as possible … if there are
certifications or design work needed we do it this session … organize, sequence, prioritize and
render whatever design or certification work is needed to the point of a Release Candidate."* This
is the PACE record (options → decision → rationale) for that program and the index of the rulings
and certifications it produced. It ships in the same PR as the `ROADMAP.md` rewrite it justifies.

**Discipline.** No feature code was written. Seats judged blind (artifact + question, never a
rationale); read-only on the tree except the Tester, which writes a test-only gate on its own
branch. Research-gated items went to the Researcher for CERTIFIED / GATED / REFUTED; the PE ruled
on build readiness and the register; the Economist specified telemetry and advised on scope. The
owner ratifies; nothing here decides an immutable.

## The problem, measured

| Fact | Number |
|---|---|
| `ROADMAP.md` on 2026-09-01 (after the seven-seat audit) | ~1,150 lines |
| `ROADMAP.md` on 2026-09-07 morning (main `e963034`) | 2,153 lines |
| Boulder/Rock section alone | ~1,450 lines |
| Named residuals 2026-09-01 → 2026-09-06 | 7 → 127 |
| PRs merged 2026-09-01 → 2026-09-07 | 71 (#697–#767) |
| Rocks whose ROADMAP status said BUILT / PR pending while the PR had merged | 14 |

The roadmap was growing because every seat run appended its findings in place, and finished work
stayed in the present tense. The backlog was not growing; the record of it was.

## Options considered

1. **Status-only pass** — true each Rock line in place, leave the prose. Cheapest; leaves a
   2,000-line plan nobody can read, and the "it keeps growing" feeling intact.
2. **Extract the detail to an archive (the 2026-09-01 precedent) and rewrite the spine as a
   critical path.** Every seat verdict and owner wording survives verbatim in
   `/archive/roadmap-boulders-detail-2026-09-07.md`; `ROADMAP.md` carries only what is LEFT, as
   ordered lanes with one buildable sentence per row. Costs a day; the register's line anchors
   become archive anchors.
3. **Split into per-Boulder files.** Better navigation, but a second task list per file is the
   failure mode the SSOT rule exists to stop.

**Decision: option 2.** `ROADMAP.md` now opens with *The Release Candidate — what it is, and the
critical path to it* (Lanes A–E, ~30 rows), then a compact *Boulder status ledger* (DONE with PRs;
OPEN by lane row; HELD), then the unchanged narrative sections, then the Residual register with a
lane tag on every ACTIONABLE row. 2,153 → ~840 lines; nothing deleted.

## The work packages (definition program)

| WP | Item | Seat(s) | Output | Status |
|---|---|---|---|---|
| A | `R-H43-ROUND-LADDER-DESYNC` model-check reproduction (RED-first) | Tester | branch `tester/h43-round-ladder-desync-modelcheck` | landed — G-H43-1 RED on main |
| A′ | Consensus-liveness family: h43 attribution + fix direction; #441; #380 | Researcher | `CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md` | landed — ATTRIBUTED, fix CERTIFIED |
| B | `R-STRUCTURE-REDERIVATION` build readiness on `e963034` + the build brief | PE | `RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md` | landed — NOT buildable as written; 1A/1B split |
| C | The era-4/v5 freeze manifest (15 items disposed) | Researcher | `ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md` | landed — 22 items |
| D | Residual register true-up: the four UNCLEAR rows; every CLOSED-BY-BOUND closer re-verified | PE | `RULING-residual-register-true-up-e963034-2026-09-07.md` | landed |
| E | Boulder 2: R2.7 telemetry spec, R2.2 build list, R2.4 checklist, S1 scope advice | Economist | `ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` | landed |
| F | Boulder 2 residual closures: `R-BOUNTY-TRUNCATION`, `R-LAMBDA-DUST′`, `R-RELAY-ANON-SET′`, `R-REFUSE-AND-SELF-SPEND`, `R-PARITY-AMPLIFICATION`, G-BB-21, G-R212-8 | Researcher | `BOULDER2-residual-closures-…-RESEARCH-CERTIFICATION-2026-09-07.md` | landed |

## Outputs (filled in as each seat reports)

### WP-A — the h43 model-check (Tester, landed): first GREEN, then RED
Branch `tester/h43-round-ladder-desync-modelcheck` (`8b467e1` → `8b1e1ef`, test-only, unmerged). The first
property (skew × down-window grid, 10 cells) was GREEN — the harness could not drive the field shape. After
WP-A′ attributed the mechanism, **G-H43-1 (heterogeneous arming) went RED on main**: 3 of 12 seats armed,
8 quiescent, one seat killed and returned mid-ladder; no commit within the certified 190 s bound. G-H43-6
pins the fixture-blindness (`matureWorld12`'s constructor calls `refill()` and arms every seat — a third
layer under the two the certification names). The Tester's third-time rule FIRED on the
fixture-green-on-wrong-arm family (scar count 3): `.claude/agent-memory/tester/scar-fixture-green-on-wrong-arm.md`.
Evidence: scratchpad `h43-modelcheck/run5-gates-final.log`.

### WP-A′ — consensus liveness family (Researcher, landed) — ATTRIBUTED, fix CERTIFIED
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md`
- Timer-skew smear (the #451 shape) REFUTED. **Root M1:** `core/node/rounds.go:306-310` arms the round clock on
  LOCAL mempool content, so the round number is a function of unreplicated private state (3 of 13 seats ran
  the pacemaker; 10 sat at r0 for ten minutes). M1b: the quiescent branch ZEROES `rs.Sweeps`. M2 (GATED):
  `proposeBlock` re-derives the round from `rs.Round`. M3: `Changes[r]` is a point record, so the catch-up
  target is the LOWEST quorum-bearing round.
- **Fix CERTIFIED:** (A) arm on a replicated condition; (B) suffix-semantics round-changes + a relayable round
  certificate (DiemBFT TC); (C) the designee proposes at the certificate's round. I1 holds via `slotCompare`.
- Bound ≤ f+1 rounds = 190 s at f=1 (observed 996 s). The harness's 1445 s cap REFUTED twice (never measured —
  `ft_publish` returned nothing; and it priced the defect).
- #441 SHARES the root (one closer); #380 SEPARATE but composes (a divergent floor is a dead designee).
- Corrections to the Researcher's own #451 / #549 / #560 rulings (the #560 GREEN was fixture-blind).
- Nine owner sentences → `ROADMAP.md` owner calls 18–20.

### WP-B — structure build readiness (PE, landed): NOT buildable as written
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md`
- The round is largely BUILT on the unmerged `builder/floorbox-structure` (`2ca01d9`, 81 files); 19 files
  conflict with main (`chain.go`, `carrier.go` add/add) — re-apply file-by-file, never rebase.
- The composition's stage table DRIFTED: main's `ValidateProposal` runs `validateIssuerKeys` (v5-only) and P5
  gained an `IssuerKeys` clause — neither in the plan or the branch. `R-LOGROOT-FORMAT-SCOPE` is never addressed
  (`HeadRef` has no `LogRoot` field). Item 8 (MG-C strip) superseded by #723. `BoxQuorumSupport` gone.
- Recommends Round 1A (main-only spine, 12-step brief) then Round 1B after the HELD box-entry round-A merges;
  the updated stage table is research-gated → WP-B′ (the delta certification, below).

### WP-C — the era-4/v5 freeze manifest (Researcher, landed): 22 items, 4 classes, 3 deadlines
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md`
- T-FREEZE-SURFACE: validity rules land in-era (the ROADMAP's "cannot land in-era" REFUTED by
  `D-F2-EVIDENCE-RECOMPUTE`'s own history), so FORMAT freezes at the freeze, VALIDITY at the stamp raise, one
  test at the flip.
- `tagRevLogSize` CERTIFIED-BUY (a wrong `m` is a wrong-accept, not liveness-only); five → three GATED (G-1 not
  satisfied on main: `MinBond` uncovered); (d-3) `AnswerDigest` specified, recommend BUY; PoP slot reserve;
  `R-AAXIS-TAG-RESERVE` REFUTED (the Researcher corrects its own R4.2 §4b); the genesis hash is network
  identity, outside the era surface; `IssuerKeys` cap = 4,096 and inadmissible alone; `SerialSize` 32; FDH
  change DECLINED for a prefix-freeness gate; `R-CREDITSPENT-UNBOUNDED` not a consensus format.
- Activation: the readiness tally only; the release is ONE constant; the tally sits behind `everMature`
  (`R-ERA4-NEEDS-MATURITY`). Nine owner sentences → owner call 9. The B8 green list is §6.

### WP-D — the residual register (PE, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md`
- Four UNCLEAR rows disposed (two HELD, two ACTIONABLE with a wrong home corrected); `R-REAPER-FORFEIT`
  re-bucketed ACTIONABLE; six residuals that lived only in certifications/rulings filed
  (`R-POR-SAMPLE-REGIME`, `R-OBJECT-UNCHECKED`, `R-SHARED-RULE-BLINDSPOT`, `R-R3-GOB-ALLOC-AMPLIFICATION`,
  `R-R31-PREFIX-PANIC`, `R-R31-SI2-WIDTH-NOTE`).
- The finding that re-sized an owner call: the delivery idle reaper runs on the WALL clock
  (`core/node/deliverysession.go:29-31`), so the 17-minute h43 stall would have reaped every live session at a
  2-epoch window — owner call 4 now waits on Lane A1's liveness bound.
- Lint gap named: `check_residual_register.py` scans `ROADMAP.md` only (a follow-on: scan `docs/design/`).

### WP-E — Boulder 2 telemetry, R2.2, R2.4, S1 (Economist, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md`
- R2.7: seven counters + one split, each with its increment site; **`bountyPaidToEscrowFunder` is DEGENERATE**
  (every escrow funder on a ledger is `n.id`) and is replaced; a third detector (`spendRefusersDistinct`) is the
  build-immutable-#4 floor, abort-only. No fetcher × object join anywhere (Don't #3).
- R2.2: 17-row build list; the design doc's network repair-Gini gate is VACUOUS (0.9902 by construction) —
  replaced by three assertable gates.
- R2.4: the phased flip as a checklist with numeric canary aborts (window = 1 epoch at the measured `T_b`).
- S1: recommends (b) — the RC ships economy default-OFF with every lane built; B8 attacks economy-ON in the
  harness; the flip is a `0.9.x` release before `1.0.0`.

### WP-F — Boulder 2 residual closures (Researcher, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md`
- `R-BOUNTY-TRUNCATION` GATED (G-BT-1 warning arm + G-BT-2 divide-after-multiplier; the worst case is
  `-chunk-size 52412` at 50 %, not 64 KiB); `R-LAMBDA-DUST′` CLOSED (24.00 GiB escrow leg exact; D-S7 holds);
  `R-RELAY-ANON-SET` stays open and held (moved both ways); `R-REFUSE-AND-SELF-SPEND` inert on the relay lane,
  armed at R2.4 with a showing-binding direction; `R-PARITY-AMPLIFICATION` DISCHARGED (the per-server floor
  corrects DOWN to 27.94 GiB — 64 GiB gains margin); G-BB-21 DISCHARGED with the commissionable flixz handoff
  paragraph (§7); G-R212-8 DISCHARGED as restated (`docs/decisions.md` struck-and-appended).
- Two corrections to the Researcher's own certifications filed as dated siblings, with pointer lines appended
  to each parent.

### WP-B′ — the stage-table delta (Researcher, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md`
- 24 stages (was 22): P8b `validateIssuerKeys`, P5's sixth clause, and **P13b the `LogRoot` conjunct** the
  2026-09-03 plan missed — a wrong-accept on the cheapest mutation. `HeadRef` gains `LogRoot`; zero format change.
- A witness-supplied `m` is a wrong-accept (`translog.go:215,222-234`) ⇒ `tagRevLogSize` is a SAFETY leaf; the
  zero-leaf freeze scope of `R-STATEVIEW-ENUMERATION` does not survive (agrees with WP-C).
- Four-arm stage-cover gate spec; MG-C do not build; `chain.go:736` REFUTED with a two-sided gate owed.
- 1A/1B CERTIFIED as sequencing; §6.4's identity claim needs M-1 (legacy `rep` leg) and M-2 (`ValidateProposal`
  must dispatch too) — two conditions nobody had named.


## The owner's calls after this program

The consolidated list lives in `ROADMAP.md` under *Owner calls open (2026-09-07)* (fifteen
one-sentence ratifications with the seats' recommendations) and *Scope calls* (S1 economy-ON inside
the RC; S2 the operational floor; S3 `#558`; S4 `#437`). Each names its source.

## Close

All eight seat runs landed (Tester ×1 in two passes, Researcher ×4, PE ×2, Economist ×1), every one blind and
read-only except the Tester's test-only branch. Two things this program changed that the roadmap did not
predict: the h43 stall went from "unattributed, model-check gated" to attributed, certified and RED on a
laptop gate inside one day; and the structure round, which the roadmap called "build open, own session",
turned out to be 81 files on an unmerged branch with a drifted stage table and a missed `LogRoot` conjunct —
the kind of thing a builder session would have discovered on day two. Nothing here built product code.
Twenty owner calls and four scope calls are consolidated in `ROADMAP.md`; the next session builds Lane A.
