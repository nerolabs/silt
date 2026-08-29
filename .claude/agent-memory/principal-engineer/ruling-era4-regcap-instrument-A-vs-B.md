---
name: ruling-era4-regcap-instrument-A-vs-B
description: era-4 RegCap instrument — SHIP count-cap (B) not byte-cap (A); I measured M collapse 13x with k (64→1), the axis that separates them; k-sensitivity was the missed finding
metadata:
  type: project
---

# Ruling: era-4 RegCap instrument — byte cap (A) vs count cap (B)

Filed 2026-08-29 against `origin/main` @ `0076337`. Full ruling:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-regcap-instrument-A-vs-B-2026-08-29.md`

**Verdict: SHIP (B) per-block COUNT cap as v5 validity; do NOT ship (A) byte cap alone.
Severity of wrong instrument = HIGH (floor-box trust regression, not perf).**

**Why:** the epoch-boundary witness is `count-in-bucket × EpochBlocks × SProofMax`
(`witness_bound.go:202` CBlock = len(readSet)×SProofMax, SProofMax=16KiB FLAT). Load-bearing
quantity is COUNT, not bytes. (B) bounds count directly. (A) bounds count only as
floor(L/M), where M = min valid reg bytes.

**THE MISSED FINDING (mine, neither Tester nor Researcher surfaced it):** M is NOT a
constant — it is dominated by k label bundles × 5 blocks × 4096 B. k (`BondLabelSamples`,
`bond.go:114-117`, `node.go:292`) is an EVOLVING per-network knob. I measured M collapse
13× as k drops:
- k=64 → M=1.433 MiB → 1 reg/2MiB block
- k=8  → M=0.257 MiB → 7 regs/block
- k=1  → M=0.110 MiB → 18 regs/block
Under a FIXED byte cap L, dropping k silently inflates bucket count (and witness) with NO
edit to L. At the deployed k=64 A and B COINCIDE (ceiling 1) — the instrument choice is
INVISIBLE at k=64. Both other seats measured only at k=64. Vary k and B wins decisively.

**Premises I verified myself (measured, not taken on faith):**
- Witness = count × flat 16KiB envelope: `witness_bound.go:202,:78`. Bytes of a reg don't
  enter witness cost; count does. THE PIVOT.
- Every reg fresh+renewal runs verifyBond, same apply site, same due-bucket:
  `chain.go:1604-1620`, `:2995-2996`.
- M floors ~1.43 MiB at smallest plot (1 MiB), INCREASES with plot size; ceiling=1 at k=64.
  My measurement cross-checks Tester's 1,485,573 B independently.
- "225 B renewal" was phantom (Answer-less, rejected); "small renewals pack many"
  (node.go:71-73) is FALSE at fixed k.

**Composition recommended:** (B) count cap at validity + KEEP MaxBondRegBytesPerBlock as
proposer policy. NOT redundant — different axes (validity COUNT vs proposer BYTES / WAN
gatherability). Shipping (A) as validity ALONGSIDE the byte-policy = two byte knobs on one
axis that drift; (B) avoids it.

**I5/I4:** (B) is one integer compare (deterministic); (A) needs cross-node byte-accounting
agreement (I5 split risk). Liveness value must clear honest ceiling at min sanctioned k;
bracket [ceiling, 16384] is wide (18 << 16384 even at k=1).

**Not my call:** RegCap VALUE is research-gated + Andrew-ratified (security sizing knob);
BlockVersion=5 format freeze is Andrew's veto-gate. I set SHAPE + constraints only.

**Contingent premise (did NOT verify):** era-4 due-bucket geometry (all-regs-at-h → one
bucket; one reg = one bucket entry). Took the Researcher's cert as the question's frame. If
one reg contributes >1 bucket entry, revisit the count-to-witness mapping.

Related: [[ruling-era4-witnessable-transitions]] (E-2 boundary, RegCap = NEW validity rule
not #506), [[ruling-r3-witness-bound-review]] (SProofMax byte-cap-not-count logic),
[[ruling-boundary-block-witness-cost]] (O(registry) boundary cost this rule bounds).
