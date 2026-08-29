---
name: era4-witnessable-transitions-track
description: The era-4 design track — making the two whole-map apply() scans (TTL sweep, epoch rotation) witnessable in O(payload); the pokt-SMT primitive-floor finding that kills range/frontier proofs; the recommended T-3 bucket + E-2 incremental-qualified options and where rule-equivalence is at risk.
metadata:
  type: project
---

Era-3 is FROZEN. A witness floor box CANNOT witness-validate era-3 blocks: two `apply()`
ops scan WHOLE committed maps → sound read-set is O(registry). Both a Researcher and a PE
certified this independently; Andrew ratified the TRUSTLESS path (Option B) — redesign the
transitions O(payload)-witnessable in a NEW era (era-4 = BlockVersion 5). Design options
filed (UNCOMMITTED): `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md`.

**The two whole-map ops (read at source):**
- TTL-expiry sweep `chain.go:3005-3013` — `for id,regH := range c.bondRegHeight`, fires
  every block when BondTTLBlocks>0. Completeness problem: proving "nothing else expired".
- Epoch rotation `rotateEpoch` (`chain.go:3124`) via `liveQualifiedSet` (`chain.go:1198-1206`)
  — `for id,sz := range c.bonded`, fires at h%EpochBlocks==0. Rebuilds epochSet.

**★ THE PRIMITIVE-FLOOR FINDING (load-bearing, verified against pokt-network/smt@v1.0.0):**
The SMT stores every leaf at `H(key)`, so KEY ORDERING IS DESTROYED. Exported proofs are
`VerifyProof` (single-key membership/non-membership), `VerifySumProof` (sum trie),
`VerifyClosestProof` (SparseMerkleClosestProof — closest in HASH space, NOT key/domain
space). NO batch proof, NO native range proof. Consequence: you CANNOT read a range/frontier
proof in DOMAIN order (e.g. by expiry height) off the raw committed-set SMT. A sorted-index
"prove the next due height" option (T-1) is REJECTED on this floor — it would need a second
order-preserving accumulator (auth skip-list / Merkle-Patricia range tree) the codebase does
not have. The fix that works with shipped primitives: encode the domain order INTO THE KEY so
domain-adjacency becomes exact-key membership/non-membership.

**Recommended options:**
- **TTL → T-3 due-height BUCKET commitment.** One committed leaf per due-height h,
  `Key(tagDueBucket, uint64BE(h))`, value = commitment to the id-set due at h (recommend
  variant (b): RFC-6962 MTH over the carried sorted id list, reusing revLog MTH machinery).
  Due height `D(id) = bondRegHeight[id] + BondTTLBlocks + 1`. At block h: resolve the bucket
  key — PROVEN_ABSENT (one non-membership proof) discharges the ENTIRE "nothing else due"
  completeness claim; PROVEN_PRESENT → delete the bucket's ids (O(payload)). Register moves
  id from D_old bucket to D_new (O(1)).
- **Rotation → E-2 incrementally-maintained committed `qualified` root.** `Key(tagQualified,
  id) → EncodeInt64(w)`, the materialized `liveQualifiedSet()`, updated at every bond/slash/
  expiry (O(1) each). Boundary does `epochSet := qualified` — a re-label, NO scan, witnessed
  by the SAME post-apply-root equality era-3 already enforces (`era3validity.go:88`). E-3
  (sum-trie weight commitment) is an OPTIONAL follow-on only if per-boundary weight-sum
  witness measures too heavy — its own cert.

**★ SHARPEST rule-equivalence hazards (representation change must NOT smuggle a rule change —
each is research-gated + must ABLATE):** (1) renew resets the TTL clock → T-3 must move id
old-bucket→new-bucket in the same apply, a missed old-delete expires EARLY; (2) `qualified`
maintenance completeness → a missed maintenance site freezes a DIFFERENT epoch set = an I1
weight-sum SAFETY attack (this is the sharpest); (3) intra-block ordering (era-3: bonds→TTL→
slashes→rotate LAST, `chain.go:3037-3048`) must be preserved; (4) MinBond config-vs-committed
divergence if MinBond ever mutable; (5) ttl==0 / slash-before-due no-ops must stay post-root-
identical.

**#535 observable gap (era-4 owns it):** `epochStart`/`effectiveEpochSet` are non-committed
observables the quorum stack reads (`statehash.go:25` excluded epochStart deliberately).
Recommend O-1: commit `epochStart` (one scalar leaf, cheap) AND route the operator-directed
`effectiveEpochSet` recovery-boundary re-base to Research as a SEPARATE gated item — it is
not a pure function of committed state and may need a committed-recovery-height rule change.
Do NOT smuggle a recovery-boundary rule change into era-4's representation work.

**Reuse (all UNCHANGED if read-set stays O(payload)):** R4 accessor `core/statehash/witness.go`,
R3 bound `witness_bound.go` (`C_block = len(read-set)·SProofMax` re-derives per block),
ReadEntry/QueryKind (carry the bond-family two-QueryKind fix forward), post-apply recompute
`era3validity.go:88` (extend stateRootLeaves + cloneForDryRun; completeness guards force it).
New security param: a per-height due-bucket SIZE CAP (uncapped = O(N) expiry DoS); #506
per-block reg R-rule bounds it — confirm composition, don't assume.

**Scope:** era-4 = BlockVersion 5, versionSupported<=5, era-3 (v4) + era-2 (v2) blocks stay
BYTE-IDENTICAL. Activation follows era-3 2a→2b→2c + regVersion>=5 lock-in.

**★ STALE-REF TRAP (bit me too, warned in task):** local `main` HEAD was `2003439` (#632
freeze) and LACKS the merged R3/R4 witness machinery. `origin/main` `0984db4` (#634) HAS it
(`witness.go`, `witness_bound.go`). ALWAYS `git fetch` + read from `origin/main` (or the
named commit), never trust local `main`. Files were on `origin/main` tree but not on the
checked-out disk (local HEAD behind).

See [[witness-floor-box-open-items]], [[keystone-leave-one-out-probes]], [[era3-reload-root-check-gap]].
