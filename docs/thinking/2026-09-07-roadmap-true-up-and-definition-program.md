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
| A | `R-H43-ROUND-LADDER-DESYNC` model-check reproduction (RED-first) | Tester | branch `tester/h43-round-ladder-desync-modelcheck` | ⟨WP-A⟩ |
| A′ | Consensus-liveness family: h43 attribution + fix direction; #441; #380 | Researcher | `CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md` | ⟨WP-A′⟩ |
| B | `R-STRUCTURE-REDERIVATION` build readiness on `e963034` + the build brief | PE | `RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md` | ⟨WP-B⟩ |
| C | The era-4/v5 freeze manifest (15 items disposed) | Researcher | `ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md` | ⟨WP-C⟩ |
| D | Residual register true-up: the four UNCLEAR rows; every CLOSED-BY-BOUND closer re-verified | PE | `RULING-residual-register-true-up-e963034-2026-09-07.md` | ⟨WP-D⟩ |
| E | Boulder 2: R2.7 telemetry spec, R2.2 build list, R2.4 checklist, S1 scope advice | Economist | `ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` | ⟨WP-E⟩ |
| F | Boulder 2 residual closures: `R-BOUNTY-TRUNCATION`, `R-LAMBDA-DUST′`, `R-RELAY-ANON-SET′`, `R-REFUSE-AND-SELF-SPEND`, `R-PARITY-AMPLIFICATION`, G-BB-21, G-R212-8 | Researcher | `BOULDER2-residual-closures-…-RESEARCH-CERTIFICATION-2026-09-07.md` | ⟨WP-F⟩ |

## Outputs (filled in as each seat reports)

⟨OUTPUTS⟩

## The owner's calls after this program

The consolidated list lives in `ROADMAP.md` under *Owner calls open (2026-09-07)* (fifteen
one-sentence ratifications with the seats' recommendations) and *Scope calls* (S1 economy-ON inside
the RC; S2 the operational floor; S3 `#558`; S4 `#437`). Each names its source.

## Close

⟨CLOSE⟩
