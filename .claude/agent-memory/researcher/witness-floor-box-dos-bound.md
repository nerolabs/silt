---
name: witness-floor-box-dos-bound
description: GATED — witness floor-box DoS bound (shape gate + pre-verify BYTE ceiling); ceiling MUST be bytes not count (NonMembershipLeafData unbounded in pokt smt); byte ceiling = validation-layer NOT consensus; slow-loris still open.
metadata:
  type: project
---

**Witness floor-box DoS bound — GATED 2026-08-29.** Closes the OPEN "witness size bound / DoS" residual from [[c7-witness-floor-box-validation]] (:94), promoted load-bearing by [[600-floor-box-direction-post-coexistence]] (witness path is PRIMARY floor-box validation).

**The composed bound:** shape gate (witness = proof for EXACTLY the block read-set, no padding/dup/unread) + a pre-verify aggregate BYTE ceiling checked at ingest before any VerifyProof.

**THE DECISIVE FINDING (verified in vendored source):** `pokt-network/smt@v1.0.0/proofs.go:55-95` `validateBasic` caps side-node COUNT (256 = PathSize×8, PathSize=32 for SHA-256 → 8 KiB side-nodes) but leaves `NonMembershipLeafData` **UNBOUNDED UPWARD** — only a MINIMUM check (:62-75). `parseLeafNode` (trie_spec.go:243-244) reads the value tail with no max. So a per-proof or per-count cap does NOT bound a proof's bytes; the ceiling MUST be a BYTE ceiling. This is the reason a count cap ships-and-lies.

**R-algo CLOSED (verified):** verifyProofWithUpdates (proofs.go:395-453) is a single side-node loop + one leaf parse = Θ(witness_bytes). No super-linear cost. The byte ceiling IS the CPU bound.

**Q2 ceiling:** `C_block = N_tx · k_read · S_proof_max`. k_read=2 (fixed by predicates). N_tx UNBOUNDED today (no per-block transition cap in ValidateProposal, chain.go:2245-2327, only empty-block reject :2283). Derived `S_proof_max ≈ 16 KiB` (256 side-nodes×32 + silt's small values). Tightest sound C_block = `expected_witness_bytes(this_block)+slack` computed per-block from the block's read-set (attester holds the block). Flat constant only defensible pinned to maxFrame=132 MiB (MaxChunkSize 128 MiB + 4 MiB overhead) — loose on the 2 GB floor box.

**Q3 CLASSIFICATION (load-bearing for build sequencing):** the witness-byte ceiling is a VALIDATION-LAYER/ingest limit, NOT a consensus-rule change — touches none of I1-I5. Precedent: `MaxBondRegBytesPerBlock`/`MaxEntryBytesPerBlock` are already "proposer-side policy only — validity unchanged" (node.go:79-80,89-91). A per-block TRANSITION cap inside ValidateProposal WOULD be a consensus change (new block-validity predicate, interacts with I4 operation-liveness = #441 starvation shape) and is the WRONG lever — NOT needed, because the block's own size bounds its honest witness. Value of S_proof_max/C_block IS a security parameter (research-gated, Andrew ratifies).

**OPEN (the gate):** R-loris — A-serve slow-loris (trickle a within-budget witness slowly) is a TIME attack the byte ceiling doesn't touch. Needs per-provider read deadline + any-of-N fallback, REUSING existing fetch machinery (FetchAttempts/FetchBackoff/HolderCooldown/RequestSizeFloorBytesPerSec, node.go:137-149,61). R-amplification: reuse FetchAttempts cap.

**Lift conditions:** (a) S_proof_max=16 KiB per-proof decode cap enforced pre-parse; (b) C_block per-block from read-set; (c) fetch-path read deadline reusing existing machinery; (d) Andrew ratifies the two security params.

**Filed:** `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/witness-floor-box-dos-bound-RESEARCH-CERTIFICATION-2026-08-29.md`
