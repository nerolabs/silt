---
name: witness-floor-box-track
description: Keystone track after era-3 freeze — witness floor-box validation (C-7/#600). Inc1 (R4) MERGED #633, Inc2 (R3) MERGED #634. Inc3 (delivery) PACE done, in gated review.
metadata:
  type: project
---

# Witness floor-box validation — the next keystone track (post era-3 freeze)

Started session-11 (2026-08-29). main HEAD `0984db4` (inc1+inc2 merged). VERIFIED origin/main truth after a stale-local-main scare.

## DECIDED / RATIFIED — do NOT re-litigate the direction
- Floor box = semi-stateless witness-validating full validator (proof against committed roots; no tree). Ratified.
- Hold-the-tree = bigger-box opt-in behind `ports.NodeStore`; NEVER the floor default.
- Witness-serving MUST be open + multi-provider — `TENETS.md:557`. Any permissioned-provider option = HUMAN VETO GATE.
- "no witness → stall (never accept)" built into the frozen format. C-7 soundness CERTIFIED.
- **SECURITY PARAMS RATIFIED (Andrew, 2026-08-29):** `S_proof_max`=16 KiB per-proof (PRE-parse); `C_block` derived per-block.

## Build plan (staged reversible increments, each blind-PE + Tester-ablate)
1. **R4 accessor spine — DONE #633** (`core/statehash/witness.go`).
2. **R3 byte caps + shape gate — DONE #634** (`core/statehash/witness_bound.go`, main `0984db4`).
3. **Delivery — ★ PACE DONE, IN GATED REVIEW.** Doc `docs/thinking/2026-08-29-witness-floor-box-delivery-increment3-options.md`.
   Two coupled parts:
   - **Part A (Block→read-set derivation, SOUNDNESS):** recommend A2 = derivation in `core/chain` (only chain reads the
     unexported committed fields) + a RECORDING drift-guard test (run real ValidateCommit+apply over a branch-covering corpus,
     assert recorded⊆derived, ABLATE). Routed to Researcher.
   - **Part B (D-2 delivery, TENET):** recommend B2 = any-of-N first-correct-wins, no permission bit, reuse existing
     `FetchAttempts`/`RequestTimeout`/`RequestSizeFloorBytesPerSec` (no new knob), `MsgGetWitness{root,key}` side-channel RPC.
     Every fetch failure → `NoWitness`, never `ProvenAbsent`. Routed to blind PE.

## ★★★ THREE SUBSTANTIVE PACE FINDINGS (reshape inc3)
1. **`apply()` reads committed state, not just validity predicates** (`chain.go:2977 slashed`, `:2980 bondRootOwner`,
   `:2986 bondRootProven`) to decide its WRITES → the read-set MUST be the UNION of validity-path reads AND apply-branch reads.
   A predicate-only read-set computes a WRONG post-state root even when validity passed. Under-inclusion = accept-unverified
   (soundness bug); over-inclusion = one wasted fetch (safe). Central to the Researcher's completeness cert.
2. **The merged `ReadEntry` type is INSUFFICIENT for the bond family.** Six bond-reg reads are `map[k],ok` idioms where BOTH
   present-with-value AND absent are acceptance-relevant; a single present-XOR-absent `QueryKind` can't model that. Inc3 likely
   EXTENDS `ReadEntry` (touching merged inc2 code). Routed to PE.
3. **★ QUORUM-STACK SCOPE BOUNDARY (vision-relevant):** the quorum-weight/qualification predicates (`attesterQualifiedAt`,
   `requireEpochWeightQuorum`, `requireDeMatureSuperQuorum`) read NON-committed observables (`epochStart`, `effectiveEpochSet`
   #535 recovery boundary) that have NO committed root → their witness story CANNOT close with the frozen roots alone. Builder
   scoped them OUT of inc3. **THE PIVOTAL QUESTION (to Researcher):** is the transition-validity read-set a SOUND CLOSURE (a
   floor box can safely accept a block having witnessed only transition-validity reads, quorum-stack a separate concern), OR
   must the quorum-stack reads be witnessed for safe acceptance? If the latter → the floor box as designed is INCOMPLETE and
   this touches the VISION "validates everything against the committed root" claim → likely a VETO-GATE / VISION true-up to Andrew.
   ESCALATION RIDES ON THE RESEARCHER'S SCOPE RULING — flagged to Andrew as pending.

## Routed now (parallel)
- Researcher: Part A completeness (validity∪apply) + the scope-boundary question →
  `.../research-outcome/witness-floor-box-readset-completeness-RESEARCH-CERTIFICATION-2026-08-29.md`.
- Blind PE: Part B delivery (B2, NoWitness wiring, no-permission, existing-knob reuse) + the ReadEntry duality gap →
  `.../principle-engineer/RULING-witness-delivery-increment3-2026-08-29.md`.

## ★★ RECURRING CLASS (2×, third-time armed): len(value)==0 conflates query-kind
R4 empty→ProvenPresent (`e6712a0`); R3 QueryPresent+nil→ProvenAbsent (`cf22cdd`). ROOT: `Resolve` infers kind from
value-length. THIRD occurrence → make `Kind` structurally authoritative (touches merged R4 — surface to Andrew first).
NOTE: finding 2's ReadEntry extension may be the right place to make Kind authoritative and retire this class.

## ★ OPS LESSON — stale local main in fresh worktrees
An agent read local `main` (=`2003439`, pre-merge) and wrongly concluded inc1+inc2 weren't merged; origin/main was `0984db4`.
Ground design/analysis against `origin/main` (fetch first) or a NAMED commit, never local `main`. Verify state discrepancies
with a Tester ground-truth check (git fetch + rev-parse origin/main + gh pr view) before trusting OR correcting a report.

## ★ LESSON — blind PE catches the MIRROR the Tester's injections don't; a Tester's fast "all N pass" is a BASELINE not a verdict
Keep BOTH seats on a safety spine. Require demonstrated RED + real-fixture/honest-verify before accepting (Andrew's session-11 catch).

## Discipline
Isolated worktree per mutator; COMMIT HASH before review; blind PE + Tester ablate each green. Merge = reversible step, Andrew
authorized the Planner to merge floor-box increments (via a Bash seat, verify-target-state — `gh pr merge` can print a local
worktree error that is NOT a refusal; verify origin/main directly). Andrew ratifies irreversible gates. LOW fully-specified PE
fix skips a 2nd full PE pass. See [[feedback-delegate-reversible-step-merges]], [[planner-isolate-mutating-seats]],
[[validated-verify-before-merge-and-measure-first]].

## Key artifacts (full paths under /Users/andrewedmond/Claude/claude/silt-reviews/ unless noted)
- PACE inc3: `docs/thinking/2026-08-29-witness-floor-box-delivery-increment3-options.md`
- PACE mechanism: `docs/thinking/2026-08-29-witness-floor-box-validation-mechanism-options.md`
- PE mechanism / R4 review / R3 review: `.../principle-engineer/RULING-witness-floor-box-mechanism-2026-08-29.md`,
  `.../RULING-R4-accessor-review-2026-08-29.md`, `.../RULING-R3-bound-review-2026-08-29.md`
- Research DoS bound (GATED→ratified): `.../research-outcome/witness-floor-box-dos-bound-RESEARCH-CERTIFICATION-2026-08-29.md`
- Direction: `.../principle-engineer/RULING-600-floor-box-direction-2026-08-28.md`,
  `.../research-outcome/600-floor-box-direction-post-coexistence-RESEARCH-NOTE-2026-08-28.md`
- Soundness base: `.../research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`

Related: [[session-resume]], [[keystone-era3-freeze-sequencing]], [[planner-isolate-mutating-seats]], [[seat-scratch-file-hygiene]].
</content>
