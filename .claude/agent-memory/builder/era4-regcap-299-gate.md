---
name: era4-regcap-299-gate
description: RegCap=256 is a SECURITY parameter (per-block TOTAL count cap, fresh+renewal); its re-derivation gate is all SEVEN determinants at the next BlockVersion mint, of which #299 (succinct proofs) is the sharpest.
metadata:
  type: project
---

era-4's `RegCap` validity rule caps the per-block **TOTAL** BondReg count (fresh + renewal),
counted AFTER `canonicalBondRegs`, enforced as a v5 block-validity rule every replica checks
on receipt. It bounds the TTL due-bucket read-set so it fits the 2 GB floor box.

**Renewals are NOT exempt — this is the load-bearing correction (Research, third refutation
of fresh-only).** Both fresh and renewals write `bondRegHeight[id] = h` at the identical apply
site (`core/chain/chain.go:2995-2996`), so both land in the same TTL due-bucket. #506
rate-limits renewals per-IDENTITY (`chain.go:1587`, `regMinInterval`), NOT per block: the
per-block `seenReg[id]` guard only stops one id appearing twice in one block. So O(registry)
distinct ids can each renew once in one block → an O(registry) TTL read-set = the exact wall
era-4 exists to remove. A fresh-only cap sits idle while the renewal term is unbounded.
A renewal is NOT smaller than a fresh reg (both carry the full ~1.485 MB space-time Answer),
so the total ceiling equals the fresh ceiling; the total cap lapses no honest renewal a
fresh-only cap would have admitted.

**Instrument = a COUNT cap, not a byte cap (PE ruling).** The boundary/TTL witness is
`count × SProofMax` (`core/statehash/witness_bound.go:202`, a FLAT per-proof envelope) — reg
BYTES do not enter the witness cost, count does. A byte cap `L` bounds count only as
`floor(L/M)`, and `M` (min valid reg size) collapses ~13× as `k`=`BondLabelSamples` drops
64→1 (PE measured), silently inflating the count a fixed `L` admits. A count cap is invariant
to `M`, and is a single integer compare (I5-clean). `MaxBondRegBytesPerBlock` stays PROPOSER
POLICY only (`core/node/node.go:79-80` "validity is unchanged", `chainrole.go:798`); it does
NOT bound an adversary and is NOT touched.

**Ratified value = 256** (Andrew, 2026-08-29). Derived for the correct total rule: it clears
the honest ceiling at the lowest permitted `k` (18 at k=1, via `resolveK`) with margin, sits
far below the desk upper bound `16,384` (`2 GiB / (EpochBlocks=8 × SProofMax=16 KiB)`), and
its worst-case valid block (~363 MiB = 256 × ~1.485 MB) is bounded by real M0 Sybil cost (256
distinct sealed plots, per-root dedup) — not a free DoS surface, and its witness (256 × 8 ×
16 KiB = 32 MiB) fits the box 64× over.

**RegCap is a SECURITY parameter, same class as `SProofMax`
(`core/statehash/witness_bound.go:78`).** The value is MEASUREMENT-REQUIRED, not
desk-pinnable: the honest ceiling reduces to `floor(B / M)` where `M` is the minimum valid
reg byte size under the deployed `verifyBond`, which is not a chain constant.

**THE RE-DERIVATION GATE — all SEVEN determinants, not #299 alone.** `N` is a function of
`B` (block reg-body budget), `k` (`BondLabelSamples`), `Samples` (possession blocks),
`BlockSize`, `BondVDFDelay`, `MinBond`, and the proof scheme. Any one changing re-derives `N`,
gated at the NEXT BlockVersion mint (a validity-affecting parameter change requires a mint
anyway). #299 (succinct proofs) is the SHARPEST single determinant: `M` drops ~1000×, the
honest ceiling rises to ~2,000 regs/block — **above 256** — so `RegCap` MUST be re-measured
and re-minted before or with #299, or honest registrations get rejected. Recorded in
`docs/decisions.md` (era-4 entry) and `docs/design/owned-residuals.md` (RegCap owed-input, F).

**How to apply:** never mint a new BlockVersion without re-deriving RegCap from a measured
minimum valid reg byte size under the then-deployed determinants. When building the RegCap
rule (increment 4c — see [[era4-ratification-and-build-order]]), the ablation is: a block with
> 256 TOTAL BondRegs of ANY mix (all-renewal, all-fresh, or mixed) must REJECT; ≤ 256 TOTAL
must ACCEPT. The old fresh-only ablation (300 renewals + 200 fresh ACCEPT) is itself a green
check with the defect uninjected — the correct rule must be able to reject a 300-renewal block
once total > N.

Certs (full paths):
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-recert-VERDICT-2026-08-29.md` (counting rule → per-block total, CERTIFIED)
- `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-regcap-instrument-A-vs-B-2026-08-29.md` (count cap, not byte cap)
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md` (N=256, floor 18, 7-determinant gate)
