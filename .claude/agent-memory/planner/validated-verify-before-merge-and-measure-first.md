---
name: validated-verify-before-merge-and-measure-first
description: Andrew-validated orchestration discipline — hold verify-before-merge under throughput pressure; measure-first for posture calls
metadata:
  type: feedback
---

Two orchestration stances Andrew explicitly endorsed at the end of session-8 (2026-08-27,
"awesome plan"):

**1. Hold verify-before-merge on consensus/economic-adjacent code, even under throughput pressure.**
The blind PE + Tester gates BEFORE merge are load-bearing, not ceremony.
- **Why:** this session the gates caught defects CI alone would have shipped — #607's #514 fix was
  still flaky at ~5% (Tester's 20-iteration stress caught iteration 18; CI passed it 19/20) AND
  carried a ~200s liveness dial-storm the PE caught (CI's single-stripe test never hits it); plus two
  builder overclaims on the keystone probes (a ⅔-weight claim that was really membership; a
  deliberation describing a construction that never shipped). Andrew floated "merge all before
  testing"; I pushed back and held test-then-merge for the CODE PRs; he accepted and endorsed the plan.
- **How to apply:** docs/config PRs can merge on a faithfulness/PE-sanity review; but any PR touching
  `core/`, consensus (I1–I5), or economic mechanism goes through blind PE + Tester first. Run the gates
  CONCURRENTLY (background, isolated worktrees) so it's gate-safety, not wall-clock delay. Never merge
  red; verify each merge landed directly (not a pipeline echo).

**2. Measure-first for posture/values calls.** For a decision that trades against an immutable (e.g.
#600 floor-box hold-tree-vs-witness × immutable #8): take the cheap measurement FIRST, THEN get PE +
Research recommendations folding it in, THEN the human decides. Don't route the decision consults, or
narrow scope, on prediction before the measurement exists. The PE's own framing: "narrow on a measured
FAIL, not on prediction."

**Also validated this session:** isolate any working-tree MUTATOR in its own worktree
([[planner-isolate-mutating-seats]]); pass reviewers the artifact + question BLIND, never the builder's
rationale; surface Builder↔Tester tension to Andrew rather than let it resolve by attrition (the #514
third-time-rule call).
