# floor-box v5 recompute — class M: the off-boundary everMature latch + the emission-keyed guard

Date: 2026-08-31
Seat: Builder
Base: `origin/main` = `07762e6`
PE ruling that scoped this: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-v5-write-obligation-ledger-2026-08-31.md`

## The mechanism (attribute before the fix)

The failure is a **liveness stall on the generic maturity crossing** because the only
reproducer of the committed `tagEverMature` leaf is boundary-gated.

`apply()` latches `everMature` false→true at the TOP of every block where
`!everMature && Mature()` (chain.go:3303-3305), BEFORE the boundary gate (chain.go:3315).
The write can land on ANY height. But the recompute reproduces the write ONLY inside class P's
`rotateOps` (rotate_v5.go:258, added by #678), which fires only when
`h % EpochBlocks == 0`. On an off-boundary maturity crossing (`h % EpochBlocks != 0`) the
recompute folds no `tagEverMature` op, so the recomputed post-root commits `everMature=false`
while the committed root commits `everMature=true` ⇒ `postRoot != committedStateRoot` ⇒ stall.

Safe (a stall is never-Accept — the box still never Accepts), but a liveness gap fatal to the
#657 accept-flip: every objective chain crosses maturity exactly once, and off-boundary is the
generic case (probability ≈ `(EpochBlocks-1)/EpochBlocks`).

This change addresses the gap by adding a **boundary-independent class-M reproducer** in the
recompute entry, making it the **single owner** of the `tagEverMature` write, and removing the
emission from class P (P keeps only the post-latch READ it needs to gate the freeze).

## The fix

1. **Class M in the entry** (`RecomputeStateRootEntriesRevocations`). Before the boundary
   dispatch, when `!preEverMature`, reconstruct `matureNow(thisBlock)` via `RecomputeMatureNow`
   (#668 — reused, not rebuilt) over the committed (POST-apply) root. If it returns mature, emit
   the `tagEverMature` false→true FoldOp. This fires on ANY height. The post-latch value is
   threaded into class P so P can gate its freeze without re-deriving or re-emitting.

2. **Single owner.** Remove the `tagEverMature` emission from `rotateOps`. P still READS the
   post-latch `everMature` (threaded from the entry) to decide the pre-latch early-return vs the
   freeze. No double-emit: at a boundary-coincident crossing M emits once; P reads only.

3. **Witness.** Add a top-level `Maturity *StateRootMaturityWitness` to `StateRootWitness`
   carrying the pre-everMature scalar proof + the `SeenSet`. Class M reads this. At a boundary,
   the entry threads the SAME maturity decision into `rotateOps`, so `Rotate.SeenSet` is no
   longer the boundary's maturity source — the top-level `Maturity` witness is the single source.
   (Kept `Rotate.SeenSet` field removed; the boundary reads the top-level witness.)

## The guard (the real deliverable — end one-at-a-time discovery)

An **emission-keyed differential leaf-diff** test. For a real `apply()` over `cloneForDryRun`:
diff `stateRootLeavesV5()` PRE vs POST; the symmetric key-diff is the exact committed write-set
apply() produced. Assert it equals the recompute's folded key-set for the block. Keys on the
LIVE marshaller output, so a future format tag is caught with zero guard edits. Driven by a
reachability generator whose schedule INCLUDES an off-boundary maturity crossing.

Ablation: with class-M removed, the guard must FAIL naming `everMature` on the off-boundary
block (RED). With the fix in, it passes (GREEN). A guard with no demonstrated red is decoration.

## Coverage tests (the #678 Tester flag), via the REAL entry, off- AND on-boundary

- (b) omit the `tagEverMature` write ⇒ stall (the committed root latches, the recompute omits).
- (c) forged maturity screen (per-member bonded/slashed) ⇒ stall (RecomputeMatureNow rejects).

## Discipline

Box never-Accepts. Reuse `RecomputeMatureNow`. No `apply()` / consensus change. This reproduces
an EXISTING committed write more completely; not research-gated (PE ruling §"Gaps to close",
`RecomputeMatureNow` already certified #668). Terminal never-Accept STOP preserved.

## #678 thinking-doc true-up

The `everMature` LATCH is any-height (apply() latches every block); the epoch FREEZE handoff is
boundary-only. #678's doc conflated them ("the handoff is always a boundary"). The FREEZE that
coincides with the latch is boundary-only; the LATCH itself is not. Corrected in the #678 doc.
