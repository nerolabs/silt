# Floor-box class-P: the young→mature handoff-boundary completeness gap

Date: 2026-08-31
Author: Builder seat
Scope: `core/chain/floorbox_recompute_stateroot_rotate_v5.go` (class P), tests, two cleanups.
Base: `origin/main` = `398977c`.

## The gap (blind PE finding)

`apply()` latches `everMature` (the maturity latch, class M) BEFORE `rotateEpoch`
(chain.go:3303-3305 latch, then 3315-3316 rotate). The class-P recompute
(`rotateOps`, floorbox_recompute_stateroot_rotate_v5.go:210) reads `everMature` as a
READ-ONLY PRE-scalar:

```
everMature := decodeBoolLeaf(rw.EverMature.OldValue)   // line 210 — PRE value
...
if !everMature { return ops, nil }                     // line 219 — early return
```

On the ONE boundary that flips `everMature` false→true — the young→mature handoff —
real `apply()` latches `everMature=true` then freezes the epochSet, but the box takes
the `!everMature` early-return and freezes NOTHING. The box's recomputed root omits the
`matureEpoch`/`epochSet`/`epochSetRoot`/tally writes AND the `everMature` write itself,
so `postRoot != committedStateRoot` → STALL.

It is SAFE (fold-caught, never wrong-accept) but a LIVENESS gap: every real objective
chain crosses the handoff exactly once, and the box can never validate that one block.
Every class-P positive test uses `MatureValidators:0` (mature-from-genesis), so
`everMature` is latched at genesis (h=0 apply) and this path is unexercised.

## Root-cause determinations (read the source, not memory)

### 1. Order: the latch runs BEFORE the freeze

`apply()` (chain.go):

- 3303-3305: `if !c.everMature && c.Mature() { c.everMature = true }` — the latch.
- 3315-3316: `if c.epochsEnabled() && b.Height%c.cfg.EpochBlocks == 0 { c.rotateEpoch(b.Height) }`.

`rotateEpoch` (chain.go:3393-3397): `if !c.everMature { return }` — reads the
POST-latch `everMature`. So on the handoff block the latch has ALREADY set
`everMature=true` by the time `rotateEpoch` reads it, and the freeze fires.

The latch condition is `!c.everMature && c.Mature()`. `Mature()` (chain.go:2139-2144)
returns `true` when `MatureValidators<=0`, else `matureNow()`. In the objective phase
(the only phase the box reproduces; v5 is always objective) with `MatureValidators>0`,
`Mature() == matureNow()`. So on the handoff:

```
post_everMature = pre_everMature || matureNow(thisBlock)
```

and `matureNow(thisBlock)` is exactly what `RecomputeMatureNow` reproduces over the
POST-apply committed state (it reads `committedStateRoot`, the post-apply root).

### 2. `everMature` IS a committed v5 leaf → M is a WRITE obligation on the handoff

statehash.go:196: `add(tagEverMature, nil, statehash.EncodeBool(c.everMature))` — a
Class-C scalar leaf in `stateRootLeaves` (inherited by `stateRootLeavesV5`). So the
`everMature` false→true transition on the handoff CHANGES a committed leaf. The
recompute must reconstruct that write (fold the `tagEverMature` scalar), not merely
consume the post value. There is NO class-M recompute wired into the entry
(`RecomputeStateRootEntriesRevocations` dispatches E/R/S/B/T/A/P — no M), so the P
class is the only place the handoff `everMature` write can be reconstructed. The
handoff is always a boundary for the freeze to matter, and P is the boundary class, so
this is the correct home.

## The fix (reuse the existing maturity recompute; no consensus change)

In `rotateOps`, replace the pre-scalar read with a POST-latch computation:

1. `preEverMature = decodeBoolLeaf(rw.EverMature.OldValue)`.
2. If `!preEverMature`: run `RecomputeMatureNow(committedStateRoot, rw.SeenSet)` —
   the EXISTING maturity recompute (#668), NOT a rebuild — to get
   `matureNowVerdict`. `postEverMature = preEverMature || matureNowVerdict`.
   (When `preEverMature` is already true, skip the recompute — no handoff, and the
   maturity witness is not required.)
3. If `postEverMature != preEverMature`, emit the `tagEverMature` scalar FoldOp
   (false→true) via `scalarFoldOp` — reconstructing the M-write.
4. Gate the freeze + tallies on `postEverMature` (was `everMature`).

This is the same class as R-P-sameblock-order (consume the same-block POST value, not
the pre value), but for the `everMature` scalar. The maturity recompute already:

- verifies set-completeness against the committed `validatorsSeenRoot`,
- verifies every member's bonded/domain/slashed leaf against the committed root,
- reads MinBond/Anchors/OperatorMargin/MatureValidators from OWN cfg (C-6),

so a forged maturity witness cannot make the box wrongly latch (or wrongly skip). The
box passes `committedStateRoot` — the POST-apply root — because the latch reads the
post-block bonded/seen set.

Cost stays O(registry): `RecomputeMatureNow` is a whole-set fold over `validatorsSeen`,
the same order as the freeze already pays. No `apply()` / consensus change; the box
STILL never-Accepts.

## `EverMature` witness: from READ-ONLY to a conditional M-witness

`StateRootRotateWitness.EverMature` was documented READ-ONLY. It now carries the
pre-scalar proof AND (on the handoff) is folded false→true. A new field
`SeenSet SeenSetWitness` carries the maturity witness, required ONLY when
`preEverMature` is false (the box needs it to decide the handoff). A pre-latched
boundary (the common steady state) supplies no SeenSet and is unaffected.

## Tests (red-before-green, drive the REAL entry)

- **Handoff-boundary POSITIVE** (`...RotateHandoffAgreesWithApply`): a fixture with
  `MatureValidators` small and validators young at genesis, maturing AT the boundary
  (latch flips false→true at the boundary height). The box AGREES with real `apply()` +
  `StateRootForVersion(5)`. MUST fail against pre-fix code (verified by temporarily
  reverting the gate).
- **Handoff ablation** (`...RotateHandoffAblationPreEverMature`): force P to use the PRE
  `everMature` (the bug) ⇒ freezes nothing ⇒ mismatch ⇒ STALL (RED); restore ⇒ GREEN.

## The two cleanups (blind Tester on #677)

1. Rename `TestRecomputeStateRootRotateAblationFlippedTally` → it forges a freeze WEIGHT
   (diverges the epochSet leaf), it does NOT exercise the tally path (fixture locks all
   tallies at genesis; the tally is a no-op at h=2). New name reflects the
   forged-freeze-weight → wrong epochSet leaf mechanism. Note that
   `TestRecomputeStateRootRotateAblationLiveTallyForgedRegVersion` (3b) is the
   load-bearing tally test.
2. Add a standalone `epochSetRoot` byte-exact check (analogous to class A's
   `TestRecomputeStateRootAttDigestByteExact`): the rotate op's `NewValue` for
   `tagEpochSetRoot` equals `nodeSetMTHFromInt64(clone.epochSet)`.

## Gates

Box never-Accepts. Reuse `RecomputeMatureNow`. No consensus change. Full build/vet/fmt +
`go test` on core/chain + `-race` on core/chain. CHANGELOG + website regen + check_links.
