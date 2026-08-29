# era-3 — closing the own-disk Reload root-check gap (PACE deliberation, DESIGN ONLY)

**Date:** 2026-08-29
**Seat:** Builder
**Status:** design only — NO production code. Options + recommendation to the Planner.
**Branch:** `era3-step2b-predicate-wt` @ `a0f8839` (NOT merged).

---

## The gap, stated as a mechanism

The failure is: **a v4 block with a WRONG `StateRoot`, re-signed with the proposer key,
is ACCEPTED by `Reload`** — because the own-disk replay path
(`Reload`→`appendStructural`→`validateStructural`, `chain.go:2688/2725/2733`) does NOT
call `validateEra3Roots`. Step 2b hooked the root predicate only into `ValidateProposal`
(`chain.go:2282`), which `ValidateCommit`→`Append` and `Reconcile`→`tmp.Append` run, but
which `validateStructural` deliberately bypasses.

**Why the step-2b reasoning was wrong.** The 2b deliberation
(`2026-08-29-era3-step2b-validity-predicate.md`, Decision 1, lines 52-60) argued the
Reload path needed no root check because "the proposer/attester signatures cover the
whole block hash, which now includes the roots (2a Q4), so `validateStructural`'s
existing signature verify rejects a bit-flipped root at load." That argument is sound
only against tampering **without the proposer key**. The blind review + the Tester
refuted it empirically: an attacker who **re-signs** the wrong-root block with the
proposer key produces a block whose signature verifies cleanly. `validateStructural`
checks `ed25519.Verify(...)` (`chain.go:2743`) and a quorum of attester sigs, all of
which pass on a validly re-signed block. Nothing recomputes the root, so the wrong root
is accepted. The signature covers the root; it does not *validate* the root against the
post-apply state.

**Reachability.** Unreachable today: minting stays `BlockVersionRounds` (v2) until step
2c (`BlockVersion = BlockVersionRounds`, `chain.go:281`), so no v4 block is on any disk.
But this is a **freeze-blocker**, not a defect to defer. The PE named the required
invariant: *every path that writes a v4 block to disk runs the era-3 root check.* Once
2c mints v4, any disk-write path that skips the predicate re-opens the exact hole 2b
closes. The invariant must be **structural** before the era-3 format freezes, because
after the freeze a missed write path is a consensus fork on our own history.

---

## The hard constraint — the depth-war

`Reload` uses the cheap `validateStructural` BY DESIGN. This is the depth-war lineage
(#528, #535, #549, #555, #556, #558, #560, #561, #562, #563, #572): per-block work that
grows with chain depth turns a full replay into O(depth²) and saturates the event loop.
The standing gate is `TestPerHeightCostLinear`
(`2026-08-27-o-depth-ci-gate.md`): per-height cost must stay linear in depth; the
lineage shape is "O(n) per block ⇒ O(n²) total" (#555 `AllEntries` built an O(n) slice
per block).

**The naive option-A trap.** `validateEra3Roots` calls `postApplyRoots`
(`era3validity.go:92`), which `cloneForDryRun()` (a deep copy of ALL committed maps) +
`apply(b)` on the clone + `StateRoot()`. `StateRoot()` (`statehash.go:131` →
`statehash.Root`, `statehash.go:122`) builds a **fresh SMT from scratch over every
committed leaf** and commits it — O(state) per call. State grows with depth. So calling
`validateEra3Roots` verbatim per block in `Reload` is:

- **the clone:** O(state) deep-copy of all 16 maps per block ⇒ O(depth) per block ⇒
  **O(depth²) total** across a full Reload.
- **the SMT recompute:** O(state) fresh-tree build per block ⇒ **O(depth²) total**.

That is the exact depth-war shape. The naive patch re-opens #528/#572 and would trip
`TestPerHeightCostLinear`. This is the mechanism that forces the design work below.

---

## THE KEY FINDING — Reload already computes the post-apply state; the root check is
## a recompute, not a re-apply

`appendStructural` (`chain.go:2725`) calls `c.apply(b)` on the **live** chain right after
`validateStructural`. So immediately after each block is applied during Reload, the live
chain's committed state IS the post-apply state that `validateEra3Roots` needs to compare
against. **The dry-run clone is pure waste on the Reload path** — the whole reason 2b
clones (proposal/commit run BEFORE `apply`, `era3validity.go` Decision 2) does not hold
in Reload, which runs the root check AFTER apply on state it already owns.

This collapses option A's cost from two O(state) operations to **one**:

| operation | 2b proposal path | Reload (post-apply) |
|---|---|---|
| deep-clone all maps | YES (O(state)) | **NO — not needed** |
| `apply(b)` | on the clone | already done on live chain |
| `StateRoot()` recompute | on the clone | on the live chain, post-apply |
| `LogRoot()` | on the clone | live `RevocationLogRoot()` |

The check becomes: after `c.apply(b)`, if `b.Version >= BlockVersionStateRoot`, assert
`*b.StateRoot == c.StateRoot()` and `*b.LogRoot == c.LogRoot()`. **No clone. No second
apply.** Just the two root reads against the state Reload just built, plus the nil-reject.

**But the SMT recompute is still O(state) per block.** `StateRoot()` rebuilds the whole
tree from scratch every call (`statehash.Root` inserts every leaf into a fresh
`NewSparseMerkleTrie`). Even without the clone, calling it per block during Reload is
O(state) per block ⇒ **O(depth²) total**. Removing the clone halves the constant and
removes the allocation-heavy deep copy (the `TestPerHeightCostLinear` metric is
HeapObjects — the clone is the bigger object-count offender), but the asymptotic
depth-war problem is the recompute itself. **The key finding makes option A cheaper by a
large constant; it does not by itself make it linear.** That distinction drives the
recommendation: the incremental win is real but insufficient alone, so option A must be
paired with a scope-limiter (below), OR the recompute must become incremental against a
persisted tree (the ratified NodeStore follow-on, #600 — not yet built).

---

## The three options

### (A) Recompute-on-Reload (per-block root check, post-apply, no clone)

Add the root check to `appendStructural` after `c.apply(b)`, using the KEY FINDING (no
clone; compare against the live post-apply roots).

**Correctness:** exact. Every v4 block re-validates its committed root against the
authoritative post-apply state, byte-for-byte the same recompute the commit path ran.
Turns the Tester's re-signed-wrong-root block RED: the recompute ≠ the forged root.

**Depth-war cost — the decisive analysis:**
- Per block: one `StateRoot()` = O(state). No clone (the key finding).
- Full Reload of N blocks: Σ O(state at height h) = **O(N²)** in the worst case
  (state grows ~linearly with depth). This is the lineage shape.
- **But note the asymmetry:** Reload is BOOT-TIME-ONLY and runs BEFORE the node joins
  consensus. The depth-war scars (#528/#572) were about O(depth) work **on the live
  event loop / per steady-state commit**, saturating the loop a node needs for liveness.
  Boot-time O(depth²) is a different severity: it delays rejoin, it does not wedge a
  running node. Whether that is acceptable is a **quantitative** question, not a
  yes/no one — and `TestPerHeightCostLinear` does not distinguish "boot-time" from
  "steady-state"; it fails on super-linear per-height cost regardless.

**Sub-variant (A′) — recompute only above a bound.** Combine A with a height/anchor
bound (option C's mechanism): recompute the root only for blocks above the finalized
anchor, structural-only below it. This keeps A's exactness where it matters (recently
committed, not-yet-deeply-finalized v4 blocks) and bounds the O(state) recompute to a
constant window ⇒ **O(state × window) = O(depth), linear.** This is A and C composed,
and is the shape the recommendation lands on.

**Verdict on A alone:** correct, and the key finding makes it far cheaper than the naive
patch, but the bare per-block recompute is still O(depth²) over a full Reload and trips
the standing gate. Not shippable unbounded; shippable as A′ (bounded).

### (B) Trust-local-disk + structurally-enforced write-gate invariant

Keep `validateStructural` cheap (no recompute). Make "every disk-write path runs the
era-3 check" a **structural** guard: a test that fails if a new caller writes a v4 block
to a chain without the predicate having run, plus a documented trust boundary for Reload.

**Is the trust argument sound?** The claim would be: everything on our own disk was
quorum-validated at commit (ValidateCommit ran the root check), so re-checking on Reload
is redundant. **This argument is REFUTED by the same evidence that opened the gap.** The
disk is not a trusted channel: the Tester's attack is bit-rot / tampering / a malicious
local-disk edit re-signed with a compromised proposer key. `appendStructural`'s own
doc (`chain.go:2713-2724`) states the design intent explicitly — "any tampering, bit-rot,
or truncation is still caught (B7 — persisted state is re-verified on load, not
trusted)." The whole POINT of `validateStructural` is that disk is UNtrusted for
integrity (it re-verifies signatures) while trusted for POLICY (it skips the rep gate).
A wrong root is an INTEGRITY failure, not a policy one. So B's trust boundary contradicts
`appendStructural`'s stated B7 contract: it would trust the disk for exactly the property
(root correctness) that `validateStructural` exists to re-verify. **B is unsound.**

**Can the guard catch a future unguarded path?** A test that greps for callers, or a
type-level marker, can catch a NEW code path that writes without the check. But even a
perfect structural guard on the write-set does not fix the CURRENT gap: it would just
assert that Reload (which by B does NOT recompute) is "allowed" to skip the check —
codifying the unsound trust boundary. B answers the wrong question: it makes the
"who-writes-v4" set enumerable, but leaves the actual hole (Reload accepts a wrong root)
open by design.

**Verdict on B:** rejected on soundness. The trust boundary contradicts the B7 contract
`validateStructural` already documents and already enforces for signatures. The structural
guard is a good IDEA — but as a guard that every write path RUNS the check (option A′'s
enforcement), not as a license for Reload to skip it.

### (C) Checkpoint/anchor-based re-validation

Reload trusts blocks strictly below the finalized anchor (already quorum-final,
irreversible), re-validates only above it. This is the **Q2-gate pattern** already in the
codebase: `Reconcile` accepts a pruned block ONLY strictly below the finalized anchor
(`chain.go:388-395`, "the Q2 gate"), and `WSCheckpoint` (`chain.go:212-226`) is a recent
trusted (height, hash) a replica will not reorg before.

**Fit:** strong. The anchor/weak-subjectivity machinery exists and is already the trust
boundary for "history below finality is settled." Below the finalized anchor, the block's
root was validated at commit AND has ⌊n/3⌋+1-Byzantine-fault finality behind it — the
same argument that lets `Reconcile` accept a pruned (Answer-stripped) block below the
floor. Above the anchor (the recent, not-yet-deeply-final window), recompute the root.

**Cost:** the recompute window is bounded by the anchor→head distance, a CONSTANT
independent of chain depth. So the per-Reload recompute cost is O(window × state) —
still O(state) per checked block, but only a constant number of blocks are checked ⇒
**O(depth) total, linear.** Passes `TestPerHeightCostLinear`.

**The soundness seam C must respect (vs B):** C trusts below-anchor blocks for
FINALITY, not for disk integrity. Signature re-verification (`validateStructural`'s
existing check) still runs on EVERY block including below-anchor — so bit-rot/tampering
without the key is still caught everywhere, exactly as today. C only skips the ROOT
RECOMPUTE below the anchor, and only because a re-signed-wrong-root block below a
finalized anchor would have had to be finalized by a Byzantine quorum, which the anchor's
finality already precludes (the same trust class as `WSCheckpoint` refusing a pre-
checkpoint reorg, `ErrPreCheckpointReorg`, `chain.go:796`). This is NOT B's "trust the
disk" argument; it is "trust finality below the anchor," the argument the codebase
already makes for pruned blocks.

**Verdict on C:** sound and linear, reusing an existing mechanism (build-process #6:
the mechanism already exists). Its one cost is a dependence on a finalized-anchor notion
being available at Reload time — which needs verification (below).

---

## RECOMMENDATION — A′ = option A's post-apply recompute (the key finding), bounded by
## option C's anchor window

**Recommend: recompute the era-3 root on the Reload path, post-apply (no clone, the key
finding), for blocks at/above the finalized anchor; below the anchor, keep
`validateStructural` cheap (signatures still re-verified, root recompute skipped under the
finality-trust boundary the codebase already uses for pruned blocks).**

Reasoning:
1. **Correctness where it is reachable.** Every v4 block in the recent window
   re-validates its root against the post-apply state. The re-signed-wrong-root attack in
   the recent window is rejected. Below the anchor, the block is quorum-final; a
   wrong-root block could not have been finalized without a Byzantine quorum the anchor
   precludes — the same soundness the Q2 gate already relies on.
2. **Linear, passes the depth-war gate.** The recompute is bounded to the constant
   anchor→head window ⇒ O(depth) total Reload cost. `TestPerHeightCostLinear` stays
   green. The key finding removes the clone, so even the checked blocks cost one
   `StateRoot()` (no O(state) deep copy) — the smaller constant.
3. **Reuses an existing mechanism** (build-process #6): the finalized-anchor / Q2-gate /
   `WSCheckpoint` trust boundary is already the codebase's answer to "history below
   finality is settled." No new trust argument is invented; B's unsound one is avoided.
4. **Rejects B's unsoundness.** B would trust the disk for root correctness, contradicting
   `validateStructural`'s own B7 contract. A′ keeps signature re-verification on every
   block and only bounds the ROOT recompute by finality — a different, already-ratified
   boundary.

**Fallback if the anchor is not cleanly available at Reload time (must be verified before
building):** ship **bare option A with the key finding** (post-apply recompute, no clone,
EVERY v4 block). This is exact and simple. Its O(depth²) is BOOT-TIME-ONLY, off the live
event loop, and materializes only once v4 blocks actually populate the disk (post-2c) —
so it can ship as the correct-but-unbounded step now, with the anchor bound (A′) as a
fast-follow before the O(depth²) window grows large in the field. The severity trade:
`TestPerHeightCostLinear` would go RED on a v4-heavy Reload ladder, so this fallback is
acceptable ONLY if the gate is scoped to steady-state (not boot Reload) — a decision for
the PE/Tester, not the Builder. **Preferred is A′; A-bare is the fallback if the anchor
plumbing is not ready.**

**Open verification owed before ANY build (evidence-or-nothing):**
- Confirm a finalized-anchor / WSCheckpoint height is available to `Reload` at the point
  `appendStructural` runs (Reload is fed our own disk; does it know the anchor?).
  Confirmed available-in-principle: `c.cfg.WSCheckpoint` is reachable from
  `appendStructural` (`chain.go:3265` reads `c.cfg.WSCheckpoint` in Reconcile). BUT
  `WSCheckpoint` is a STATIC genesis-config (height, hash), not a rolling anchor that
  advances toward head as blocks finalize — so the recompute window it bounds is
  "everything after the genesis checkpoint," which does NOT shrink with depth and does
  NOT give A′ its constant window. The rolling finalized-anchor the Q2 gate uses
  (`chain.go:388-395`) is the notion A′ needs, and whether Reload can compute/access
  that rolling anchor at replay time is UNVERIFIED — this is the load-bearing open item.
  If the rolling anchor is not available at Reload, A′ is not buildable as specified and
  the fallback (A-bare) or a new anchor source is required. Measure before assuming.
- Confirm `TestPerHeightCostLinear`'s ladder does or does not exercise the Reload path
  (it drives live commits per the O-depth doc; Reload may be a separate cost home needing
  its own bound-test). This decides whether A-bare trips the gate.

---

## Making the invariant STRUCTURAL (not a comment)

The PE's invariant — *every path that writes a v4 block to disk runs the era-3 root
check* — becomes structural three ways, layered:

1. **The regression test (the Tester's scenario, permanent).** Extend
   `reload_era3_boundary_test.go` (the RED-home file shipped ahead of era-3, which already
   holds `mixedEraHistory` and the future-era Reload tests). Add
   `TestReloadRejectsResignedWrongRootV4`: build a real v4 block with correct roots, commit
   it, then on the persisted copy PERTURB `StateRoot` and RE-SIGN with the proposer key
   (and re-attest), and assert `Reload` returns `ErrEra3StateRootMismatch` (not a signature
   error — the point is the sig VERIFIES and the ROOT is caught) and reports the honest
   restored prefix. This is the exact re-signed-wrong-root-via-Reload scenario the Tester
   ran. **Failing-first proof (build-immutable #7 / the "ablate every green" lesson):**
   the test must be RED on `a0f8839` (Reload accepts it today) and GREEN after the fix —
   and the ablation must show the ROOT check catching it, not a nil-map panic or a
   signature failure (the session-7 leave-one-out lesson: model the omission as accepted,
   not crashed). A paired `TestReloadAcceptsCorrectRootV4` positive control ensures the
   rejection is about the root, not an unrelated malformation.

2. **The write-set enumeration guard (catches a FUTURE unguarded path).** A test that
   enumerates the functions which apply a v4 block to a `*Chain` (`Append`,
   `appendStructural`, `Reconcile`'s `tmp.Append`) and asserts each runs
   `validateEra3Roots` (or, for A′, the bounded variant) on v4 input. Mechanism: a v4
   block with a wrong root fed through EACH write path must be rejected. A new write path
   added without the check fails this test — the guard is the structural enforcement of
   "every disk-write path runs the check," which is B's good idea kept and B's unsound
   license dropped. This is the "third-time rule" encoding: the gap appeared because the
   check lived at ONE site (`ValidateProposal`) and a second write path bypassed it;
   encode the completeness of the write-set as a test, not a comment.

3. **Correct the 2b deliberation's record.** The 2b doc's Decision 1 (lines 52-60)
   asserts Reload needs no root check. That reasoning is now known wrong (the signature
   covers the root but does not validate it). The fix PR amends that section to point at
   this deliberation and the corrected invariant, so the auditable record does not carry
   the refuted argument forward. (Per build-process #6: the mechanism paragraph that was
   wrong gets corrected, not silently overwritten.)

---

## Boundary / research gate

This touches a **consensus-rule** path (validity of a block on the Reload/own-disk
path, era-3 root enforcement, I5 fork-choice determinism). Per the research gate, the
Builder advises and shapes the question; the **Researcher certifies** and the human
ratifies. This deliberation is the shaped question. Specifically research must certify:

- The finality-trust boundary for C/A′ (skipping the root RECOMPUTE — but not the
  signature re-verify — below the finalized anchor) is sound for era-3 roots, i.e. a
  wrong-root v4 block cannot be finalized below the anchor. This is the same class as the
  Q2-gate pruned-block soundness but must be certified for the root specifically, since
  the root encodes the `bonded`/`epochSet` weight sums (2b Decision, `era3validity.go`
  lines 19-24).
- Whether A-bare (unbounded per-block recompute) is acceptable as an interim, given its
  boot-time-only O(depth²).

**No production code is written by this deliberation.** Options + recommendation to the
Planner; the fix (A′ or A-bare + the two structural guards + the regression test) builds
only on the filed research verdict and the anchor-availability verification.
