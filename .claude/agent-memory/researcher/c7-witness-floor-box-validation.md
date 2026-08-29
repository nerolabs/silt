---
name: c7-witness-floor-box-validation
description: C-7 CERTIFIED — witness-based (stateless-style) floor-box validation is sound/complete for silt's set-valued validity state; era-3 format freeze is the gate.
metadata:
  type: project
---

# C-7 witness-based floor-box validation — CERTIFIED (2026-08-27)

**Verdict: CERTIFIED**, with one gate on the era-3 format freeze (not on the scheme).
Cert: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`

**Why:** the floor-box validity predicates reduce to set inclusion + non-inclusion over
the committed SMT maps — the decisive artifact is `core/chain/chain.go:2188-2246`:
- `byRoot` NON-membership (dup-publish reject, `:2189`) + MEMBERSHIP (revoke-known-root, `:2236`)
- `spent` NON-membership (double-spend reject, `:2215`)
- `revoked` MEMBERSHIP (un-revoke-revoked-only, `:2241`)
Witness must carry BOTH inclusion AND non-inclusion proofs — both classes are load-bearing.

**Soundness proven by EXECUTION, not assumed** — `internal/smtspike/exclusion_test.go`
rejects all three forgeries (membership-reread-as-absence; another key's absence proof
replayed — and it FORCES the pokt `related-leaf` guard at proofs.go:422 to fire, 256
candidates; stale/transplanted proof under old root). Delete-then-absence across a flush:
`boltstore_correctness_test.go:132-183`.

**Availability decomposition (the key insight):** safety needs NO trust in the tier above
(witnesses self-verify against the trusted committed root); liveness needs ≥1 reachable
honest provider. Withheld/partial witness → floor box STALLS (safe, prefer-stall-to-reorg
#535), never accepts. **The one banned implementation move: "no witness → accept."** Must
be an explicit verifier invariant: missing witness for a read key = reject/stall.

**THE GATE:** era-3 `Block` commits NEITHER root today (verified `chain.go:311-405`; the
`Root` at :419 is the BOND commitment, not a state root). Witness scheme is vacuous without
a committed, quorum-attested root to verify against. So era-3 format freeze is hard-gated
on committing BOTH roots (state SMT + append-only log, the #597 two-root shape) over the
COMPLETENESS-proven field set (the consensus-weight probes bonded/epochSet/spent/slashed
reaching the equivalence + order-varying oracles — the PE's existing hard gate).

**Residuals:** witness-completeness is DOWNSTREAM of field-enumeration completeness (inherited,
held-in-tension). OPEN construction items (do NOT block soundness): witness SIZE BOUND / DoS
(per-key is O(log n) + grind-resistant, but per-block aggregate needs a committed byte budget
like the #441 entry/reg split); who generates + carry/gossip mechanism; sharded-registry
"not in any shard" omission proof (§11.2, separate consult).

**Scope:** #600 (floor box holds tree vs validates-by-witness) is Andrew's posture call, NOT
mine — I certified the witness path is SOUND so his call is informed. Soundness is no longer
a reason to hesitate on the witness direction.

**Literature schema adopted:** Ethereum stateless-client / EIP-1186 (inclusion+exclusion vs
committed root); SMT non-membership = audit path to default leaf (Dahlberg-Pulls-Peeters
ePrint 2016/683 §3.1, Haider 2018/955). Sorted-Merkle neighbor-bracketing is the UNSOUND
alternative silt correctly rejected.
