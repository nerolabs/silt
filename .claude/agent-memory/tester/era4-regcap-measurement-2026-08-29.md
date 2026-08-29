---
name: era4-regcap-measurement-2026-08-29
description: Era-4 RegCap sizing: bond Answer measured ~1.5MB (#299 unshipped), honest-ceiling=1, RegCap bracket [1,16384] non-empty, proposed RegCap=256 (2026-08-29)
metadata:
  type: project
---

Measurement run 2026-08-29, against origin/main @ 0984db4. Updated same day with full measurement.

## #299 shipped status: NO — OPEN

Issue #299 is OPEN. Grep of `#299` across `0984db4` confirms it is referenced only as a FUTURE
structural close (`node.go:60,79`, `daemon.go:105`, `chainrole.go:1278`), not implemented. The
deployed `verifyBond` scheme uses the full ~1.5 MB `Answer` field per fresh bond registration.

## Measured minimum fresh BondReg CBOR byte size

Source: `TestMeasure_AnswerSizeBreakdown299` (`core/bond/answer_size_measure_test.go`), run on
0984db4. PASS in 0.68 s.

| Bond size | n blocks | Answer CBOR |
|---|---|---|
| 8 MiB | 2048 | **1,497 KiB** (measured) |
| 64 MiB | 16384 | **1,531 KiB** (measured) |
| 1 GiB (floor) | ~263,672 | **~1,582 KiB** (extrapolated) |

Answer size breakdown (64 label opens × 5 blocks × 4,096 B = 1,280 KiB block data;
possession 20 × 4 KiB = 80 KiB; Merkle proofs grow ~34 B/depth level — proven by CBOR
probe of manifest.Proof; model fits measured data within 6 KiB).

Production `DerivedBondFloor` = 2 × (2s × 270 MB/s) = 1,080,000,000 B ≈ 1,029 MiB
(`daemon.go:1646-1653`). A 4 KiB minimum bond is never valid on an objective chain.

Non-Answer BondReg fields (CBOR-measured): Validator(32) + Root(32) + Size(5) + Sig(64) +
CBOR overhead(6) + Answer field header(5) = 144 bytes.

**Minimum fresh BondReg CBOR size (at production floor) ≈ 1,531–1,582 KiB ≈ ~1.5 MB.**

The Answer field dominates entirely. The 144 B overhead is negligible.

## Honest-ceiling computation

`MaxBondRegBytesPerBlock = 2 MiB = 2,097,152 B` (`node.go:270`, confirmed at `node.go:79-80`
"Proposer-side policy only"). This is the honest proposer's per-block byte budget for fresh regs.

```
honest_ceiling = floor(2,097,152 / 1,567,888) = 1   [using 64MiB measured proxy]
honest_ceiling = floor(2,097,152 / 1,620,112) = 1   [using 1GiB extrapolation]
```

Both approaches give **honest_ceiling = 1**. Under the deployed scheme (#299 unshipped), an
honest proposer can embed exactly ONE fresh bond registration per block within its 2 MiB budget.
(node.go:76-77 confirms the proposer always embeds at least one even if it exceeds the budget,
so the honest ceiling is ~1 fresh reg/block regardless of whether it slightly exceeds 2 MiB.)

## RegCap bracket

- Upper bound: `RegCap ≤ 16,384` (boundary-epoch witness fit: `RegCap × EpochBlocks × SProofMax ≤ 2 GiB`; `EpochBlocks=8` at `daemon.go:1729`, `SProofMax=16 KiB` at `witness_bound.go:78`). Derived. Exact.
- Lower bound (honest-ceiling): **1**.
- Bracket `[1, 16,384]` is NON-EMPTY.

## Proposed RegCap = 256

| Property | Value |
|---|---|
| Margin above honest ceiling | 256× (ceiling=1, RegCap=256) |
| Margin below upper bound | 64× (16,384 / 256) |
| Boundary epoch witness | 256 × 8 × 16 KiB = 32 MiB vs 2 GiB (64× headroom) |

R3 C_block composition: `C_block = len(readSet) × SProofMax` (`witness_bound.go:202-203`). For a
block with RegCap fresh regs, C_block adapts to the readSet (not a separate hard cap). The
boundary-epoch constraint is the binding one; RegCap=256 gives 64× headroom there.

RegCap=256 is a validity rule, not a proposer policy. The honest proposer operates at ~1 fresh
reg/block (far below 256). RegCap=256 gives headroom for future proof compression (#299 — if
shipped, ~192 B proofs → ~10k regs/2 MiB block → honest ceiling rises to ~10k, still within
16,384) without needing a rule change.

## Sources cited (all on 0984db4)

- `MaxBondRegBytesPerBlock`: `node.go:270` (default 2<<20), "Proposer-side policy only" `node.go:79-80`
- `DerivedBondFloor`: `daemon.go:1646-1653` (~1 GiB)
- `EpochBlocks=8`: `daemon.go:1729`
- `SProofMax=16 KiB`: `witness_bound.go:78`
- `C_block = len(readSet)×SProofMax`: `witness_bound.go:202-203`
- Answer size measurement: `core/bond/answer_size_measure_test.go:21`, run PASS 0.68s
- #299 status: OPEN (`github.com/nerolabs/silt/issues/299`)
- BondReg struct fields + omitempty: `chain.go:482-513`
- `validateBondReg` requirements: `chain.go:1604-1620`
- First-reg exemption from #506 rate-limit: `chain.go:1587` (`ok==false`)
