# 2026-08-15 — Testing across the five tiers: coverage is broad, but efficiency is inverted

**Context / trigger:** Andrew, thinking out loud, asked how test coverage stands across unit /
integration / e2e / field / cloud; how bugs force introspection on test *quality and efficiency*;
and what to improve in how we call for and operationalize testing. This is the assessment, recorded
so PE and research can react and so the trend is visible over time. Not a build deliberation — a
process one — but the same evidence discipline applies.

**Evidence (measured, 2026-08-15):**
- Unit/component: **355** core + **95** adapter test funcs across 146 files.
- Sim: `integration/consensus` + **70** `simnet`/`simclock`-based tests.
- e2e: **16** funcs, 7 files (real daemons over sockets, local).
- Integration Docker: **21** harnesses (sybil, redteam, nat, flakynet, chaos, churn, …) — the
  acceptance surface, but **outside `go test ./...`**.
- Field/cloud: `integration/cloudtest` (multi-region GCP, expensive, non-deterministic).
- Model-check (#406): design doc exists (`docs/design/consensus-model-check.md`); **not built**.
- Honesty debt: **#303 open — 27 confirmed test-honesty issues** (positive-control gaps, confounds,
  invented assertion strings).

**The one diagnostic fact.** Four RC-blocking consensus bugs (#357, B2, #397, #402) were **one
invariant** (non-intersecting finality quorum), and **each was found by the most expensive tier** — a
multi-region cloud run — when a deterministic laptop property test would have caught all four. The
cost-to-catch is **upside down**. The tier order exists on paper (`unit → model-check → sim → netem →
field`) but the **model-check rung is missing**, so the class fell through to the bottom of the
ladder, one region at a time.

**The reframe (the load-bearing idea).** The metric is not "do we have coverage" but "is the coverage
at the **cheapest deterministic tier** that can own it?" A bug caught in the cloud that a laptop could
catch is a **process failure even though it was caught.** That turns the tier order from a guideline
into a gate.

**Honest tier-by-tier read.**
- Unit/component — healthy; where failing-first repros and the invariants map live.
- Sim (`integration/consensus`, 70 tests) — **missed all four consensus bugs**: it exercised the
  settled single-set regime, not adversarial scheduling. *Green sim ≠ correct consensus.* Thoroughness
  was measured by count, not by coverage-of-adversary.
- e2e (16 funcs) — thin for the daemon surface; flaky under load (the #402 satellite test contended).
- Integration Docker (21 harnesses) — broad and valuable, but **drifts** because it is not run
  per-change (the #402 sybil `-anchor-quorum` staleness could not be validated locally today — the
  drift symptom).
- Field/cloud — became the **discovery** tier for consensus invariants: the exact inversion PE named.

**How bugs force introspection today — honestly.**
- Working: failing-first-at-the-right-tier (V5) is real; per-bug test-rethinking caught two downstream
  interactions on #402 (seam-7 A=3; `SupportMeetsQuorum` predicate drift).
- Gap: introspection is **per-instance, not per-class** — we fix the bug's test, rarely ask "which
  tier let the *class* through, and does that tier need a new *capability*?" The model-check is the
  first per-class fix and is unbuilt.
- Debt: #303's 27 honesty issues are "green must be honest" in arrears; GAP-reclassification (an
  undrivable security drill graded GAP not RED) is scoreboard management.

**Improvements, in leverage order.**
1. **Build #406 (the model-check).** Highest-leverage move; converts consensus correctness from
   cloud-fuzzed to laptop-asserted. Already the certified next step — this confirms the priority.
2. **Add the cheapest-tier question to every bug post-mortem** (now in the PR template, 2026-08-15):
   name the cheapest deterministic tier that could catch the *class*; confirm it now does, or the
   capability gap *is* the fix.
3. **Measure thoroughness as coverage-of-adversary, not count** — the sim's 70 tests missed the class
   for lack of a scheduled adversary.
4. **Pay down #303; retire GAP-reclassification** — every security property gets a deterministic
   RED/GREEN home where the attack can be *scheduled* (D-CONSENSUS ruled it; needs enforcement).
5. **Close Docker-suite drift** — a fast subset in CI, or a config-lint, so the acceptance surface
   stops rotting between the rare full runs.

**Decision / what changes now:** (a) this assessment recorded; (b) the PR template gains the
cheapest-tier bug question + the I1–I5 consensus-invariants section (operational mechanisms, not
prose); (c) #406 is reaffirmed as the immediate next build after #402. No tier is rebuilt on the
strength of this note alone.

**What would change my mind / open questions for PE + research:** is a *fast Docker subset* in CI
worth the CI-time cost, or is a config-lint enough to catch drift? Is "coverage-of-adversary" better
enforced as a checklist (per quorum site, per security property) or as a model-check obligation? Route
before investing.

**Status:** recorded; PR-template mechanism landed; #406 remains the next build.
