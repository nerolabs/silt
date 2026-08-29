---
name: era4-regcap-value-derivation-2026-08-29
description: GATED 2026-08-29 HEAD 0076337 — RegCap VALUE N=256 CERTIFIED-CONDITIONAL on rule being corrected to per-block TOTAL count first; design of record STILL fresh-only (REFUTED 3rd time); floor N>=18 (k=1 honest ceiling); Q1 geometry CERTIFIED; re-derive on ALL 7 determinants not #299 alone.
metadata:
  type: project
---

# era-4 RegCap VALUE derivation — GATED (2026-08-29, HEAD 0076337)

Verdict: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`

## The verdict in one line
**GATED. N=256 is the RIGHT number but on the WRONG rule.** The design of record
(instrument doc §B.1, ratified param `era4-ratification-and-build-order.md:24`, #299-gate memo,
PR #635) STILL encodes fresh-only counting — REFUTED for the third time. The value is
CERTIFIED-CONDITIONAL on correcting the rule to per-block TOTAL count (fresh+renewal) FIRST.

## Q1 geometry — CERTIFIED (one reg = one bucket entry)
- `canonicalBondRegs` (chain.go:2969) folds same-id to one reg/id; each writes ONE
  `bondRegHeight[id]=h` (chain.go:2996) → one due-height D=h+ttl+1 → one bucket entry.
- Ratified schema: `Key(tagDueBucket, uint64BE(h))`, value=MTH over canonical id list;
  "one bucket = one block's regs". Bucket population = count of BondRegs in originating block.
- A renewal = delete old-bucket + insert new-bucket = ONE net entry. Count→witness map is exact,
  linear, unchanged. N-count cap bounds every bucket to N, TTL witness to N×SProofMax.

## Fresh-only is UNSOUND (why the value can't ship as-is)
- Two witness surfaces: (1) BOUNDARY = epochSet/qualified symmetric-diff, ONLY fresh ids change
  it; (2) TTL DUE-BUCKET = renewals populate it by delete+reinsert (chain.go:2995-2996).
  era-4 commits BOTH (tagQualified + tagDueBucket). RegCap must bound the tighter = due-bucket.
- Builder's fresh-only argument (instrument doc §89-90) is true for surface (1), FALSE for (2).
- #506 rate-limits renewals PER IDENTITY not per block → O(registry) distinct ids each renew once
  in one block → bucket O(registry) → the wall era-4 removes. Fresh-only cap sits idle.
- CORRECT rule: `if len(canonicalBondRegs(b.BondRegs)) > N: reject` — v5 validity, on receipt.

## Q2 k-range → the value FLOOR
- k=`BondLabelSamples` is NODE-LOCAL config wired into verifier (objectivechain.go:50); must be
  NETWORK-UNIFORM for consensus (else fork on Answer acceptance) → genesis-fixed, flag-day to
  change. Security param: soundness `(1-ε)^k`.
- Min EFFECTIVE k = 1 (`resolveK` bond.go:123 maps k<=0 → 64). No upper cap.
- Honest ceiling floor(2MiB/M(k)): k=64→1 (Tester exact M=1,485,573B), k=8→7, k=1→18 (PE sweep).
- **FLOOR: N >= 18** (k=1 honest ceiling). k=1 is soundness-broken but resolveK PERMITS it, so
  the format constant must not reject an honest block at k=1.

## Q3 value → N=256 (CERTIFIED-CONDITIONAL)
- 256 ∈ [18, 16,384]. Re-reached from corrected rule + k-range, not deference.
- TIGHT N (18-32): smallest max block but forces #299-style re-mint on any growth. HEADROOM
  N=256: tolerates growth; worst-case valid block 256×1.485MB ≈ 363 MiB.
- 363 MiB block NOT a DoS surface: each of 256 regs needs a DISTINCT sealed MinBond plot
  (per-root dedup chain.go:2949-2953/2980-2990 → one identity per Root) = full M0 Sybil cost.
  Witness 256×8×16KiB = 32 MiB ≪ 2 GiB. No companion mitigation needed. Human ratifies number.
- I1-I5: pure fn (block,cfg), I5-deterministic, touches no invariant; I4-edge is the floor-18
  reason, 256 clears 14×.

## Re-derivation gate — bind ALL 7 determinants (NOT #299 alone)
B(2MiB node.go:270) · k(node.go:292) · Samples(20 bond.go:108) · BlockSize(4096 bond.go:93) ·
BondVDFDelay(1000 node.go:291) · MinBond(1MiB daemon.go:93) · proof scheme(#299). Upper bound
16,384 is SEPARATE gate (EpochBlocks=8, SProofMax=16KiB, 2GiB box).

## Ablation must change too
Old (fresh-only) ablation "300 renew + 200 fresh ACCEPT; 257 fresh REJECT" is itself a green
check with defect uninjected — it ACCEPTS a 300-renewal block the correct rule must reject once
total>N. New ablation: >256 TOTAL (any mix) REJECT; ≤256 total ACCEPT.

Related: [[era4-regcap-recert-2026-08-29]], [[era4-regcap-value-verdict-2026-08-29]],
[[era4-witnessable-transitions-recert]].
