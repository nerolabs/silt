# Build refinements to the proof-map OOM plan (grounded in code at build time)

**Date:** 2026-08-17 · **Companion to:**
`2026-08-17-proof-map-oom-fix-plan.md` (the settled design). This records the two
under-specifications the plan left open, resolved against the actual code before
the 14-site refactor. Pace-before-code: decide once, in writing, then build.

## Refinement 1 — where full proofs live in MEMORY-ONLY mode

The plan splits `StorageProof` into resident metadata (`Root, Index, Total,
Column`) + a bounded `proofCache` over a `ProofStore`, paging the big fields
(`Path`, `PorTags`). But **the node runs memory-only in every sim/test/ephemeral
client** — `proofStore == nil` today, and `n.proofs` was the *only* home for the
full proof. If full content moves behind `proofCache`-over-`ProofStore`, a
nil-backed cache has **nothing to page from** → `answerChallenge` (por.go:133) and
`fanOut` (demand.go:93) break for every memory-only node.

**Decision:** `proofCache` always wraps a `ProofStore`; the backing defaults to
`memproofs` (the existing in-RAM `ProofStore`) and `SetProofStore` swaps it to
`diskproofs`. So:
- **prod daemon** (has diskproofs): full proofs page from disk → resident RAM is
  O(hot). The OOM fix.
- **sim/test** (memory-only): full proofs sit in `memproofs` (in-RAM, small) —
  identical residency to today's `n.proofs`, which is fine for sims (the plan's own
  framing: "nil ProofStore means memory-only, fine for sims and ephemeral
  clients"). The bound doesn't *help* here, but it doesn't *hurt* — sims are small.

`proofMeta` is ALWAYS resident and is the existence + small-field authority, in
both modes. This keeps every non-content site (existence, `HeldRoots`,
`EnforceDenylist`, `chunkDenied`, re-announce `placementKey`) paging-free.

## Refinement 2 — the LIAR branch now write-throughs its proof

`node.go:1065` (the `n.liar` "keep the receipt, ditch the goods" path) currently
stashes the proof in `n.proofs` **memory-only** — it deliberately skips both
`n.store.Put` and `n.proofStore.Put`. Under the split, the full proof (esp.
`PorTags`, which the liar needs to form its fraudulent `answerChallenge`) must be
retrievable via `proofCache.Get`, which pages from the backing. A bounded,
no-warm cache can evict it, so the proof MUST be in the backing store.

**Decision:** the liar's store path calls `proofCache.Put(id, full)` too — a
write-through to the backing (`memproofs` in sims, `diskproofs` in prod). Behavior
nuance: a liar node now persists its proof to the backing where before it was
RAM-only. This is a *where-it-resides* change (consistent with the plan's frame),
not a security change: the liar still withholds the chunk DATA (`n.store.Put` is
still skipped), still lies on `MsgHasChunk`, and its μ still fails verification.
The proof persisting is harmless — a restart would just re-announce a shard whose
bytes it doesn't have, exactly as the honest-restart path handles a missing chunk.
Flagged in the PR. No test asserts an empty liar proof-dir (checked).

## Everything else is as the plan specified

- `ports.ProofStore` gains `Get(id)(StorageProof,bool,error)` + `Keys()([]ChunkID,error)`; `Put/Load/Delete` stay (Load retained for compat; node no longer calls it).
- `LoadProofs` populates `proofMeta` via `Keys()`+`Get()` per id (one full proof resident at a time → O(N) startup I/O, NOT O(N) RAM). Compact-sidecar to cut the I/O is the noted fast-follow.
- `proofCache` mirrors `cachestore` exactly, incl. `TestPutDoesNotWarmCache` scan resistance; sized by proof bytes (`len(Path)*32 + Σlen(PorTags) + fixed`).
- Budget: new `Config.ProofCacheBytes` (0 → a sane default const in `New`); lets the memory-wall test force a tiny budget to prove O(hot).
