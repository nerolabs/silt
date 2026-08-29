---
name: era4-regcap-measurement-2026-08-29
description: Era-4 RegCap re-measure 2026-08-29: valid BondReg=1,485,573B (passes VerifySpaceTime); honest ceiling=1; prior 225B renewal measurement was PHANTOM (Answer-less reg rejected by verifyBond)
metadata:
  type: project
---

## CORRECTION (2026-08-29, HEAD 0076337)

The prior renewal measurement of **225 bytes** was a PHANTOM. An Answer-less BondReg is
REJECTED by the deployed `verifyBond` at `chain.go:1617` → `bond.VerifySpaceTime`. The
225-byte reg carries `Answer: []byte("answer")` (1 byte) in the gate tests, which fails
verification. A reg that PASSES the validity verifier is NOT Answer-less.

## Valid BondReg measurement (re-measured 2026-08-29, HEAD 0076337)

Run: standalone Go program, `go run` in scratchpad (deleted after run), using:
- `bond.Seal(pub, 1<<20)` — MinBond floor, 1 MiB
- `c.AnswerSpaceTime(42, vdf.Default(), 1000, 64)` — deployed field config
- `bond.VerifySpaceTime(pub, root, 1<<20, 42, ans, vdf.Default(), 1000, 64)` → **PASS**
- `bond.EncodeAnswer(ans)` → answer bytes
- `chain.NewBondReg(priv, root, size, ansBytes, prev, 0)`
- `chain.Block{Version: chain.BlockVersion, BondRegs: []chain.BondReg{reg}}` + `chain.Encode`
  (identical to `bondRegEncode` at `core/node/chainrole.go:1757-1759`)

### Measured output (exact)

```
Encoded Answer: 1485340 bytes (1450.5 KiB, 1.417 MiB)
bondRegEncode size: 1485573 bytes (1450.8 KiB, 1.417 MiB)
bond.VerifySpaceTime: PASS
Honest per-block ceiling = floor(2097152 / 1485573) = 1
```

**Minimum valid BondReg (renewal or fresh, passes VerifySpaceTime): 1,485,573 bytes.**

The 233-byte envelope overhead (Block wrapper + BondReg header fields) is negligible
relative to the ~1.485 MB Answer. Both fresh and renewal regs carry the same full Answer —
the Answer is mandatory for the proof to pass.

### Honest per-block ceiling

```
MaxBondRegBytesPerBlock = 2 << 20 = 2,097,152 B   (core/node/node.go:270)
min valid reg = 1,485,573 B
ceiling = floor(2,097,152 / 1,485,573) = 1
```

**Honest per-block ceiling = 1.** This is the same as the "fresh" number from the prior
session. The prior session's fresh ceiling of 1 was correct; the renewal ceiling of 9,320
was based on the PHANTOM 225-byte shape.

## Determinants (with file:line for re-derivation gate)

| Parameter | Value | Source |
|---|---|---|
| Samples (possession blocks) | 20 | `core/bond/bond.go:108` |
| DefaultLabelSamples k | 64 | `core/bond/bond.go:117` |
| BlockSize | 4096 bytes | `core/bond/bond.go:93` |
| BondVDFDelay (default) | 1000 squarings | `core/node/node.go:291` |
| BondLabelSamples (default) | 64 | `core/node/node.go:292` |
| MinBond floor (default -min-bond) | 1 MiB = 1,048,576 B | `cmd/silt/daemon.go:93` |
| MaxBondRegBytesPerBlock (block body budget) | 2 MiB = 2,097,152 B | `core/node/node.go:270` |

## Combined picture (corrected)

| Reg shape | Min VALID size | Honest ceiling/block |
|---|---|---|
| Any (fresh or renewal, passes VerifySpaceTime) | 1,485,573 B | **1** |

Ceiling = 1 at both the fresh and renewal shapes. The prior 225-byte phantom and the
derived 9,320-ceiling are RETRACTED.

## RegCap bracket and proposed value (updated)

- Lower bound (honest valid ceiling): 1
- Upper bound (witness fit at 2 GiB): 16,384
- Bracket [1, 16,384] non-empty.
- RegCap=256 sits 256× above the measured honest ceiling and 64× below the witness bound.
  The earlier concern that RegCap=256 was "too low for renewal-packed blocks" is RETRACTED
  — a renewal-packed block cannot hold more than 1 valid reg under the current byte budget.

## Sources cited (all on HEAD 0076337)

- `MaxBondRegBytesPerBlock`: `core/node/node.go:270` (2<<20)
- `bondRegEncode`: `core/node/chainrole.go:1757-1759`
- `verifyBond` call: `core/chain/chain.go:1617`
- `VerifySpaceTime`: `core/bond/bond.go:428`
- Field config defaults: `core/node/node.go:291-292`
- MinBond floor default: `cmd/silt/daemon.go:93`
- Measurement run: scratchpad Go program, exit 0, output captured 2026-08-29, deleted after
