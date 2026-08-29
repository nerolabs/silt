---
name: era3-reload-root-check-gap
description: The own-disk Reload path skips validateEra3Roots — a re-signed wrong-root v4 block is accepted; the depth-war cost trap and the post-apply key finding.
metadata:
  type: project
---

The era-3 (v4) committed-root predicate `validateEra3Roots` hooks ONLY into
`ValidateProposal` (`chain.go:2282`). The own-disk Reload path
(`Reload`→`appendStructural`→`validateStructural`) BYPASSES it. A v4 block with a WRONG
`StateRoot`, re-signed with the proposer key, is ACCEPTED by `Reload` (Tester-confirmed
on `a0f8839`). Unreachable until 2c mints v4, but a FREEZE-BLOCKER.

**Why the 2b reasoning was wrong:** the 2b deliberation argued the block signature covers
the root (2a Q4) so `validateStructural`'s sig-verify catches a bad root. FALSE — the
signature covers the root but does not VALIDATE it against post-apply state. Re-signing
with the proposer key passes every sig/quorum check. Integrity ≠ root-correctness.

**THE KEY FINDING (option A incremental):** `appendStructural` calls `c.apply(b)` on the
LIVE chain, so the post-apply state is already in hand on the Reload path — NO dry-run
clone needed (unlike the proposal path, which runs before apply). The check reduces to:
after apply, assert `*b.StateRoot == c.StateRoot()`. BUT `StateRoot()` rebuilds a fresh
SMT over ALL leaves every call (`statehash.Root`, O(state)) — so per-block recompute is
still O(depth) per block ⇒ O(depth²) over a full Reload = the depth-war shape (#528/#572,
gate `TestPerHeightCostLinear`). The key finding removes the O(state) clone (a large
constant + the HeapObjects offender) but NOT the asymptotic problem.

**Recommendation filed:** A′ = post-apply recompute (no clone) BOUNDED by option C's
rolling finalized-anchor window (the Q2-gate / `WSCheckpoint` trust boundary, `chain.go:
388-395`) ⇒ recompute only above the anchor ⇒ O(depth) linear. Below the anchor, skip the
RECOMPUTE (not the sig-verify) under finality-trust, same class as Reconcile's pruned-
block Q2 gate. Option B (trust-local-disk) REJECTED: contradicts `validateStructural`'s
own B7 contract (disk is untrusted for integrity). Fallback: A-bare (unbounded, boot-time-
only O(depth²)) if the rolling anchor is not available at Reload.

**Load-bearing OPEN item:** `WSCheckpoint` is STATIC genesis-config, not a rolling anchor —
it does NOT give A′ its constant window. Whether the rolling finalized-anchor is available
at Reload time is UNVERIFIED and gates whether A′ is buildable as specified.

**Structural enforcement of the PE invariant** ("every path that writes a v4 block to disk
runs the era-3 check"): (1) regression test in `reload_era3_boundary_test.go` — the re-
signed-wrong-root-via-Reload scenario, RED on a0f8839, ablation must show the ROOT check
catching it (not a nil panic / sig error); (2) a write-set enumeration guard over
`Append`/`appendStructural`/Reconcile's `tmp.Append`; (3) correct the 2b deliberation's
Decision-1 record.

**2c SYMMETRY COMPLETION (commit `8629f09` on `era3-step2c-activation-mint-flip`):** the
SAME "every disk-write path enforces the era-3 rules" invariant had a SECOND asymmetry the
blind PE flagged (RULING-era3-step2c-...-2026-08-29). 2b put the ROOT check on
`appendStructural`; the 2c VERSION-boundary rule (`ErrEra3VersionRequired` — a v2 block at/
above H_era3 is invalid) was on the commit path ONLY. The TRAP in the write-set guard: it
keyed only on `validateEra3Roots`, and a v2 block carries NO roots (root check era-gated
OFF for sub-v4), so a future fast-sync/import path running only the root check would satisfy
the guard while persisting a v2 block at/above H_era3. Fix: extract `validateEra3Version`
(pure header check, era3Active from PRIOR committed state), run it on `appendStructural`
BEFORE apply (longest-valid-prefix), and extend the guard to require BOTH validators on
every disk-write path. RED proof: a path running only the root check REDs the guard, flagged
for the missing version rule. The RED for the Reload check itself must show ACCEPTED (got
<nil>) AFTER `validateStructural` asserts the forged v2 block is signature-valid — proving
the cause is the version rule, not a sig error. Doc fix: no `transferState` fn exists;
snapshot boot is a model-check property (`snapshotBoot`, reflection over committedSet).

**Research-gated** (consensus-rule, I5). Deliberation:
`docs/thinking/2026-08-29-era3-reload-root-check-options.md`. See [[keystone-leave-one-out-probes]].
