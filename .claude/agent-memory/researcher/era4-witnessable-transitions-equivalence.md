---
name: era4-witnessable-transitions-equivalence
description: Era-4 (BlockVersion=5) T-3/E-2/O-1 equivalence cert — GATED; E-2 site enumeration REFUTED (misses chain.go:2989 squatter-delete); O-1 does NOT close recovery boundary; bucket cap is a security param.
metadata:
  type: project
---

Era-4 witnessable-transitions equivalence certification, 2026-08-29, vs `origin/main` @ `0984db4`.
Cert: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-EQUIVALENCE-RESEARCH-2026-08-29.md`.
Proposal: `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md`.

**Verdict: GATED.** Three load-bearing findings, each correctable:

1. **E-2 maintenance enumeration REFUTED (the decisive artifact).** Proposal lists 3 sites
   that mutate bonded/slashed (register-write 2995, TTL-delete 3008, slash 3019/3020). It
   MISSES `chain.go:2989` `delete(c.bonded, owner)` — the squatter-displacement delete
   (proof-beats-declaration G3). `owner` is a DIFFERENT id from the `id` written at 2995, so
   maintenance as listed never does `delete(qualified, owner)` → `qualified` keeps a member
   `liveQualifiedSet()` drops → DIFFERENT frozen epoch set → I1 weight-sum attack. Grep the
   FIVE sites (2989/2995/3008/3019/3020) — the list of 3 is wrong.

2. **O-1 (commit epochStart) does NOT close the recovery boundary.** epochStart's ONLY reader
   is Regime() (chain.go:1932; modelcheck_state_completeness_test.go:109) → committing it
   changes no quorum decision (sound alone). BUT the observable that blocks a floor-box quorum
   witness is `effectiveEpochSet` (chain.go:1243), read by requireEpochWeightQuorum (2597).
   At recovery boundary it returns liveQualifiedSet() (WHOLE-MAP scan) gated on
   `c.cfg.LivenessRecoveryHeight` (OPERATOR FLAG, config, not committed state). Two separate
   problems O-1 leaves open: the scan (E-2 could serve it only if effectiveEpochSet routes
   through `qualified` — proposal doesn't), and the operator flag (floor box can't know h==
   recovery-height from roots). SEPARATE gate, human's-call direction (committed recovery-height
   vs O-2 posture bound).

3. **Per-height bucket-size cap IS a security param, no existing constant bound.** #506 R-rule
   bounds RENEWALS per identity (regMinInterval ≈ ttl/4, chain.go:3288); FIRST-time regs are
   EXEMPT (chain.go:1587). N fresh identities at height r → all in due-bucket D=r+ttl+1 →
   expiry O(N). Only soft proposer byte-budget (chainrole.go:810) + mempool cap bound it, NO
   consensus cap. Proposal's "R-rule already bounds regs/block" is REFUTED as stated. Cap value
   must be certified numerically before build (like SProofMax).

**CERTIFIED sub-parts:** Q1 arithmetic D(id)=bondRegHeight+ttl+1 = era-3 h-regH>ttl (exact);
Q3 bucket completeness via single non-membership proof (witness.go single-root whole-set
exclusion — sound; needs canonical id-list pin for MTH variant b); MinBond fixed-per-chain
(config 130, no mutation — reopens only if governance-tunable); intra-block order
bonds→TTL→slashes→rotate-LAST must be preserved.

**Residuals:** R1 (add 2989 to E-2 + ablate) held; R2 (recovery boundary direction) held;
R3 (bucket cap value) open; R4 (canonical id-list encoding) open.

Related: [[witness-floor-box-boundary-block-cost]] (this is the trustless fix Andrew ratified
for the O(registry) boundary read that cert flagged), [[era3-committed-state-root-format]]
(era-3 FROZEN, era-4 is new BlockVersion=5), [[c7-witness-floor-box-validation]] (the floor-box
soundness base).
