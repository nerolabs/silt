---
name: era4-regcap-299-gate
description: RegCap=256 is a SECURITY parameter with a hard #299 (succinct proofs) re-mint gate — measured honest ceiling rises above 256 under succinct proofs.
metadata:
  type: project
---

era-4's `RegCap` fresh-registration validity rule caps distinct FIRST-TIME bond
registrations per block (renewals are EXEMPT — #506 R-rule, `chain.go:1587` `ok==false`
marks fresh). It bounds the epoch-boundary witness read-set so it fits the 2 GB floor box.

**Ratified value = 256** (Andrew, 2026-08-29). Safe under the deployed ~1.5 MB genesis
proof scheme (honest ceiling ~1 fresh reg/block = `2 MiB / ~1.5 MB`); 256 sits far above
the honest ceiling and far below the desk-certified upper bound `16,384`
(`2 GiB / (EpochBlocks=8 × SProofMax=16 KiB)`).

**RegCap is a SECURITY parameter, same class as `SProofMax`
(`core/statehash/witness_bound.go:78`).** RECERT2 Q2 ruled the VALUE
MEASUREMENT-REQUIRED (not desk-pinnable): the honest ceiling reduces to the minimum valid
fresh-reg byte size under the deployed `verifyBond`, which is not a chain constant. The
proposer byte budget `MaxBondRegBytesPerBlock` (2 MiB) is PROPOSER-ONLY, not validity — it
does NOT bound an adversary (grep: only at `core/node/chainrole.go:798`, never in
`chain.go` validity).

**THE HARD GATE — recorded ON #299:** if #299 (succinct proofs) ships, smaller proofs
raise the measured honest ceiling to ~2,000 fresh regs/block — **above 256**. `RegCap`
MUST be re-measured and re-minted **before or with #299**, or honest fresh registrations
get rejected. Recorded in `docs/decisions.md` (era-4 entry) and `docs/design/owned-residuals.md`
(RegCap owed-input, section E6).

**How to apply:** never ship #299 without re-deriving RegCap from a measured minimum
fresh-reg byte size under the then-deployed proof scheme. When building the RegCap rule
(increment 4c — see [[era4-ratification-and-build-order]]), the ablation is: 300 renewals +
200 fresh must ACCEPT (renewals don't count); 257 fresh must REJECT.
