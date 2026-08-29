---
name: era3-committed-state-root-format
description: era-3 two-root block-format — original CERT-with-conditions (2026-08-28, mint v4 REFUTATION), then BUILT RE-CERTIFICATION (2026-08-29) = CERTIFIED FOR THE FREEZE, human pulls the trigger.
metadata:
  type: project
---

# era-3 committed state-root format

## BUILT RE-CERTIFICATION — CERTIFIED FOR THE FREEZE (2026-08-29)

Re-cert (pre-FREEZE gate): `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-BUILT-RECERTIFICATION-2026-08-29.md`

BUILT format on origin/main HEAD `3af40bc` (PRs #627 root, #629 2a schema+v4-accept, #630 2b predicate,
#631 2c activation+mint-flip). Read the code; verified every load-bearing claim against file:line +
injected-defect oracles. **All 7 carry-forward items CERTIFIED. Nothing must change for soundness.** The
FREEZE trigger + immutability are the human's to pull.

- **Fidelity**: two `*ports.Hash` roots in Hash body (chain.go:438-439,590); StateRoot=SMT over 18
  committedSet fields (core/statehash + core/chain/statehash.go); LogRoot=RFC-6962 reuse; NUL-tag
  keyspace; canonical VALUE encoders (8-byte BE bonded/epochSet, statehash.go:47-51); BlockVersionStateRoot=4
  (chain.go:338); versionSupported<=4 (:739); activation on regVersion>=4 weight >⅔ (:3166-3178). CERTIFIED.
- **omitempty amendment (was freeze-cond-4 "required")**: `*ports.Hash`+omitempty PRESERVES era-2
  byte-identity (a literal `required` [32]byte emits 32 zero bytes → breaks era-2 hash) + non-zero era-3
  root constants (empty-log=sha256(""), empty-state=SMT over scalar leaves) + 2b nil-reject
  (ErrEra3RootMissing, era3validity.go:92-94). Satisfies the empty-vs-absent INTENT. CERTIFIED. Cond-4
  AMENDED.
- **Value-encoding (safety-critical)**: R2 oracle non-vacuous — TestStateRootChangesOnPerturbedValue
  perturbs bonded/epochSet WEIGHT bytes → root moves (modelcheck_stateroot_determinism_test.go:168,190);
  TestStateRootIsNodeIndependent = cross-node byte-identity (:143). Wrong-value witness can't forge a
  super-quorum weight. CERTIFIED.
- **Activation (Q5 was GATED → LIFTED)**: >= boundary (H_era3=first v4 height, distinct from #506's >),
  epoch-final h+EpochBlocks, weight-tally (PE injected ready++→RED), one-way guarded, reorg-stable/
  replay-derived oracle. #506 Q2 form transfers at hard-fork readiness level. CERTIFIED.
- **Two new fields era3LockedIn/era3Height**: classified committedSet, in stateRootTags, carried in
  clone/adopt, probeUncovered EMPTY, load-bearing (era3Probes). Q1 partition stays total. CERTIFIED.
- **Uniform enforcement**: BOTH root check AND version rule on commit path AND own-disk Reload
  (appendStructural, A-bare CHECK-BEFORE-APPLY — post-apply-reject would poison longest-valid-prefix,
  PE corrected own prior ruling), before apply; structural write-set guard keyed on `c.apply(` requires
  BOTH rules, ablated by TestGuardMatchesCallsNotCommentText. A-bare O(depth²) boot = hold-tree bridge
  (#600), NOT a freeze-blocker. CERTIFIED.
- **Residuals**: R3 witness-size DoS + R4 missing-witness-vs-exclusion accessor = C-7 WITNESS follow-on
  (#600), do NOT gate the FORMAT freeze. #603 DISCHARGED both axes (membership: probeUncovered empty;
  value/weight: perturbation oracle).

**Non-blocking corrections recorded**: (1) stale "16 committedSet fields" / "four scalar leaves" comments
(chain.go:321,417; determinism test:23,231) — build commits 18 fields / 6 scalars; coverage bound by
reflection not the literal, so soundness unaffected — Builder should fix comments. (2) appendStructural
version-symmetry already CLOSED in build.

## Original CERTIFICATION — CERTIFIED-WITH-CONDITIONS (2026-08-28)

Cert: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`

**The one REFUTATION (corrected the builder's choice-2A):** minting BlockVersion=3 for era-3 is UNSOUND.
versionSupported(v)=v>=1 && v<=BlockVersionRegGate(=3), so a CURRENT binary DECODE-ACCEPTS a v3 block and
validates it under era-2 with NO root predicate → silently accepts a FORGED root. era-3 ADDS a predicate
(not subset-shrinking) → HARD fork, #506's soft-fork activation does NOT transfer. **MUST mint
BlockVersion=4; extend versionSupported <=4 same release** (current binary then rejects v4 loudly at
decode via ErrBlockVersion). Distinct readiness regVersion>=4 (era-3 needs SMT+witness verifier, a
different software state than #506 R-rule). BUILT AS SPECIFIED — refutation honored.

**Certified sound as proposed:** Q1 two-root composition (SMT over committedSet + RFC-6962 revLog;
epochStart the sole observable); Q2 value-encoding (pokt SMT folds value into leaf digest); Q3 NUL-tag
injective keyspace; Q4 verifier invariant ("no witness → reject/stall" complete via membership OR
non-membership, both witnessed); Q6 sha256 pinned + value-widths are consensus params + witness-size DoS
deferred.
