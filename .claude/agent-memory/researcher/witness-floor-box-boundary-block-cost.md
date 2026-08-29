---
name: witness-floor-box-boundary-block-cost
description: GATED 2026-08-29 — boundary-block witness read-set is O(registry) (TTL sweep + rotateEpoch scan whole maps); NO sound bounded witness mechanism with silt SMT; POSTURE decision (accept quorum-final root, self-validate O(payload) rest).
metadata:
  type: project
---

**Witness floor-box boundary-block cost — GATED 2026-08-29 (POSTURE decision).** Filed:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/witness-floor-box-boundary-block-cost-RESEARCH-2026-08-29.md`

**Q1 — O(registry), both classes, VERIFIED in source:**
- TTL-expiry sweep `chain.go:3005-3013` — `for id, regH := range c.bondRegHeight` ranges the WHOLE map every block; deleted set bounded but READ set = whole `bondRegHeight`+touched `bonded`/`regVersion` = Θ(registry). **Fires EVERY v4 block (BondTTLBlocks>0), not just epoch multiples** — so O(registry) root is a property of every block (R2, the sharpest finding).
- rotateEpoch `chain.go:3046→3124→liveQualifiedSet:1198` — `for id, sz := range c.bonded` + reads `slashed[id]` per id = Θ(registry); re-tallies regVersion over whole frozen set `:3144-3179`.
- Composed with `C_block = len(read-set)·16 KiB`: at 100k ids TTL sweep alone ≈ 300k leaves ≈ 4.8 GiB > 2 GB floor box.

**Q2 — NO sound bounded witness mechanism (structural, not a missing feature):** silt SMT (`pokt-network/smt@v1.0.0/types.go:39-60`, `proofs.go:322,357`) is single-key Prove/VerifyProof only — no batch/diff/range/apply-against-witness primitive. Deeper: a whole-map read is universally-quantified ("delete iff regH too old, for ALL ids"); a diff proof proves what CHANGED, cannot prove what was NOT deleted (the omitted-delete = inflate-own-weight safety attack, era3validity.go:19-23). A range proof covering "nothing anywhere should expire" IS the whole keyspace = O(registry). Verkle/stateless-client principle: witness bounded IFF state ACCESSED bounded; Verkle shrinks per-access proof, NOT the number of accesses. **Witness-validation of boundary transitions is fundamentally O(registry).**

**Verdict = POSTURE decision, not a mechanism to build.** Recommended Option A: floor box ACCEPTS the quorum-super-quorum-final `StateRoot` at boundary heights (trust it already has — `chain.go:3037-3044` §3 finality), does NOT recompute root by witness; still self-validates the O(payload) non-root predicates (sigs, bond proofs, six-transition validity, LogRoot RFC-6962 shape) by witness. Posture loss BOUNDED: only the aggregate root is trusted; O(payload) safety predicates still self-checked every block.

**Q3 — classification:** Option A is VALIDATION-LAYER, NOT consensus (I1-I5 untouched; full nodes recompute as today), NOT frozen-format (no field/encoding/BlockVersion change). No new numeric security param (inherits quorum finality). STILL routes to Andrew — it changes the floor box TRUST posture.

**Rejected levers:** per-block transition cap (doesn't help — sweep reads existing map regardless of this block's tx count; AND would be consensus, hits I4); transition redesign (expiry index / incremental qualified-set commitment) = Option B, real but touches FROZEN format → hard fork / new era, parked as trustless long-term path.

**Residuals:** R1 posture (owner's call, Andrew ratifies); R2 every-block-not-just-boundary (TTL sweep); R3 Option B format cost (HELD); R4 slow-loris (inherited, unchanged — Option A shrinks witness to O(payload)).

See [[witness-floor-box-readset-completeness]], [[witness-floor-box-dos-bound]], [[600-floor-box-direction-post-coexistence]], [[c7-witness-floor-box-validation]], [[era3-committed-state-root-format]].
