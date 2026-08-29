---
name: ruling-keystone-spent-slashed-classification
description: era-3 classification of spent/slashed — both UNDER the SMT state root; freeze-gated on covering probes because order-independence currently passes over an EMPTY map (vacuous green)
metadata:
  type: project
---

# Ruling — keystone `spent` and `slashed` era-3 classification (2026-08-28)

Repo HEAD `7d2a292` (main advanced past the `9bfe8e2` in the question, through #615).
Ruling: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-keystone-spent-slashed-classification-2026-08-28.md`

**Verdict.** Both `spent` and `slashed` belong UNDER the history-independent SMT state root
(committedSet). Classification already correct in `stateClass`; verified against code.

**Verified premises (file:line):**
- `spent map[string]bool` (chain.go:828); read by validity predicate `ValidateEntry` →
  `ErrTokenSpent` (chain.go:2229); grow-only, written `=true` (chain.go:2745), no delete.
- `slashed map[ports.NodeID]bool` (chain.go:882); read by qualification/quorum predicates
  (chain.go:1623, 1458, 1527, 1098, 1121, …); grow-only, written `=true` (chain.go:2819),
  no delete. Both order-independent by construction (union is commutative).
- NOT revLog-shaped (no order), NOT bondDomain/epochStart-shaped (they ARE predicates,
  not metrics/observables).

**Sequencing verdict: PROVE before freeze, and it is NOT proven yet.**
- Order-independence oracle iterates every committedSet field
  (modelcheck_order_independence_test.go:84-91) BUT its fixture `twoOrderings` only
  publishes/revokes — never populates spent or slashed. Confirmed empirically:
  `len(a.spent)==len(b.spent)==0`, same for slashed. So the 16/16 "identical" green
  includes two `DeepEqual(∅,∅)` comparisons that assert NOTHING.
- Both fields are in `probeUncovered` (snapshot_equivalence_test.go:496-512); leave-one-out
  only asserts flips for fields in `covered`, so their necessity is declared, not proven.
- #597 cert mandates the order-varying oracle run BEFORE the era-3 freeze (cert line 176-177).
  Precedent: prior PE ruling hard-gated the freeze on the epochSet weight-bytes probe
  (RULING-keystone-probes-bonded-epochset). Same posture here.

**Coupling the consult missed (the high-value finding):** the placement was never the risk —
it was already right. The risk is a VACUOUS GREEN masking an unproven leaf: the oracle
reports 16/16 identical while exercising 14. Durable fix = a fixture-side guard that every
committedSet field compared by the order-independence test is NON-EMPTY in at least one
ordering. This is the "ablate every green check" lesson recurring at the oracle level.

**The one call that is Andrew's:** freeze on current coverage (defer spent/slashed probes)
vs wait for full grow-only-set coverage. Recommend WAIT — cheap local probes, forever-format
decision. Sequencing preference, not an immutable trade.

Related: [[ruling-keystone-probes-bonded-epochset]], [[ruling-keystone-node-store-backend]]
