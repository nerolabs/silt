---
name: ruling-era4-witnessable-transitions
description: era-4 T-3 (TTL due-bucket) + E-2 (committed qualified-set) design — SHIP-WITH-FIXES; bucket-cap is NEW rule not #506, boundary rewrite not O(1), floor-box vs full-node acceptance split
metadata:
  type: project
---

# Ruling: era-4 witnessable O(payload) transitions (T-3 / E-2)

Judged 2026-08-29 at `origin/main` @ `0984db4` (local main `2003439` stale — warned, confirmed).
Doc: `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md`.
Filed: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-witnessable-transitions-2026-08-29.md`

**Verdict: SHIP-WITH-FIXES.** Spine sound, T-1/E-1 rejections correctly foreclosed by the
primitive floor. Three load-bearing claims fail verification:

1. **HIGH — bucket-size cap is a NEW consensus validity rule, NOT a consequence of #506.**
   #506 R-rule (`chain.go:1587`) bounds per-IDENTITY re-reg frequency, not distinct-id count/block.
   No per-block `len(BondRegs)` validity cap exists. Only bound is `MaxBondRegBytesPerBlock`,
   PROPOSER-SIDE ONLY (`core/node/chainrole.go:798`, daemon flag, `0=unbounded`). So bucket size
   is unbounded at validity → TTL-firing block class is O(registry) until a new cap rule ships.
   Doc's "confirm don't assume" resolves AGAINST the optimistic reading.

2. **MEDIUM — E-2 boundary O(1) in apply but NOT in changed-leaf set.** `rotateEpoch`
   (`chain.go:3130-3131`) does wholesale `c.epochSet = liveQualifiedSet()`. If `qualified` and
   `epochSet` stay distinct keyspaces, boundary changed-leaf set = epoch-ACCUMULATED delta
   (not one block's payload). Fix: boundary must advance an epoch POINTER over ONE shared
   `qualified`/`epochSet` keyspace, changing ZERO leaves. "Re-label" in doc does too much work.

3. **LOW — Judge #4 acceptance answered for FLOOR BOX only.** Full-node `validateEra3Roots` →
   `postApplyRoots` (`era3validity.go:119`) deep-clones whole chain + recomputes FULL root
   (`stateRootLeaves` scans `c.bonded` `statehash.go:98`). O(registry) every block; era-4
   ENLARGES it (3 new tags). Fine (floor box is the target) but doc conflates witness-read-set
   with full-node recompute. Name it.

**Verified TRUE:** primitive floor exactly as described — leaves at `H(key)`
(`smt@v1.0.0/hasher.go:69`), only single-key `Prove` + hash-space `ProveClosest` + sum trie,
NO batch/range (`proofs.go`). T-1 genuinely foreclosed. `liveQualifiedSet` = filter of bonded
by MinBond+!slashed (`chain.go:1198-1206`). `MinBond` genesis-fixed, never mutated mid-chain →
E-2 config-drift risk DORMANT. Renew-bucket-move is the real sharpest TTL hazard (register
resets regH `chain.go:2996` then sweep runs after). 2a→2b→2c reuse correct.

**Couplings doc missed:** (a) bucket-cap + carried-list shape gate are ONE gate not two
(gate can only pin the list if cap bounds its length); (b) `qualified` and `epochSet` want to
be the SAME keyspace — highest-value structural move; (c) `liveQualifiedSet` also feeds #535
`effectiveEpochSet` recovery re-base (`chain.go:1194`), inside the rotation-equiv gate.

**Sequencing note (LOW):** era-3 widened `versionSupported` to 4 in step 2a BEFORE the 2b
predicate — opened a decode-accept-without-predicate window. Era-4 should widen to `<=5` in the
2b release OR accept the same window on purpose. Couples to my prior era-3 finding: `versionSupported`
already admits v3 = silent mis-validation ([[ruling-era3-committed-state-root-format]]).

**Human's call:** opening era-4 now vs deferring behind era-3 residuals is Andrew's scope +
veto-gate (new BlockVersion). Recommend: fix findings 1+2 in doc → route to Research as one
equivalence+completeness cert → then format decision to Andrew. Do not build before cert.

Related: [[ruling-era3-committed-state-root-format]], [[ruling-boundary-block-witness-cost]]
(that ruling: BOTH TTL-sweep and rotateEpoch are O(registry) self-recompute — era-4 is the
fix it pointed at), [[ruling-r3-witness-bound-review]] (SProofMax cap precedent for the
bucket-size security param).
