---
name: feedback-delegate-reversible-step-merges
description: Andrew delegates the intermediate step-merges of a multi-step build to the Planner once each clears the full blind loop; he ratifies only at the irreversible gate.
metadata:
  type: feedback
---

For a multi-step build marching toward a veto-gate (e.g. the era-3 format freeze), Andrew delegates the
INTERMEDIATE step-merges to the Planner — I merge each step increment once it clears the full three-gate blind
loop (Builder + blind PE + Tester), without asking per-merge.

**Why:** the intermediate steps are REVERSIBLE and INERT before the gate (e.g. era-3 steps 2a/2b commit schema +
predicate but nothing mints/validates a v4 block until 2c activation + the Researcher re-cert). Nothing
meaningful is ceded by delegating them, and it keeps momentum. Decided session-10 (2026-08-29): "delegate step
merges, and merge please" (#629).

**How to apply:** merge each cleared step increment myself (route a merge Builder with the verify-target-state
discipline — a merge command can lie). Reserve the human for the IRREVERSIBLE ratification: the Researcher
re-cert of the composed built format, then the FREEZE veto gate. Still surface each step's outcome in status,
and STOP + escalate immediately if a step turns up something that touches an immutable / the published claims /
the freeze scope — delegation is about mechanical merges, not about deciding a gate.
