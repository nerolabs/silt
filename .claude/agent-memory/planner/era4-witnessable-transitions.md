---
name: era4-witnessable-transitions
description: era-4 new-era design (trustless witnessable transitions, Option B). Session-12: design CERTIFIED-WITH-CONDITIONS + ratified; 4a built & reviewer-cleared; the RegCap COUNTING RULE was REFUTED (fresh-only) → reopened.
metadata:
  type: project
---

# era-4 witnessable transitions — the new-era track (Option B, opened 2026-08-29)

Opened after Andrew ratified Option B ([[witness-floor-box-inc3-refuted]]): make the two whole-map `apply()` transitions
witnessable in a NEW era so the floor box stays fully trustless. Ground: `origin/main @ 0984db4`.

## THE MECHANISM (CERTIFIED design)
- **T-3 TTL:** commit a due-height BUCKET per height (`Key(tagDueBucket, uint64BE(h))`); "nothing else due at h" = ONE non-membership
  proof. Bucket value = MTH over a CANONICAL (sorted/dedup/unpadded) carried id list (variant b). One bucket = one block's regs.
- **E-2 rotation (corrected):** `epochSet` stays its OWN FROZEN materialized committed keyspace (era-3 `tagEpochSet` shape); a live
  `tagQualified` keyspace is maintained at ALL FIVE `bonded`/`slashed` sites (2989/2995/3008/3019/3020) as a boundary accelerator; at the
  boundary `epochSet := qualified` (a COPY). The boundary block is a DISTINCT, HEAVIER witness class = O(boundary-delta), bounded
  ≤ RegCap×EpochBlocks×SProofMax. (The PE's shared-keyspace pointer-advance was REFUTED by Research — it broke mid-epoch I1/I3 immutability.)
- **O-1:** commit `epochStart` (scalar `tagEpochStart`) — changes no quorum decision; doubles as the epoch pointer. Recovery boundary SCOPED OUT (R2).
- New `BlockVersion=5`, `versionSupported<=5` predicate-first. pokt SMT has NO batch/range proof (leaves at H(key)) — the load-bearing floor.

## REVIEW LINEAGE (all 2026-08-29, under silt-reviews/)
PACE `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md` → PE `RULING-era4-witnessable-transitions` (SHIP-WITH-FIXES) →
Research `era4-witnessable-transitions-EQUIVALENCE-RESEARCH` (GATED, caught missed 2989) → Builder rev → RE-CERT
`era4-witnessable-transitions-RECERT` (STILL GATED: pointer-advance REFUTED + cap value) → Builder rev-2 (frozen epochSet, direction b) →
**RE-CERT2 `era4-witnessable-transitions-RECERT2-2026-08-29.md` — CERTIFIED-WITH-CONDITIONS** (Q1 design sound; Q2 RegCap measurement-required).

## MEASUREMENTS (session-12, Tester, local/no-billable)
- SMT proof max 1,474 B @ 1M leaves << 16 KiB SProofMax; COUNT of proofs is the constraint. Boundary FITS 2 GB. RegCap UPPER bound = 16,384.
- #299 (succinct proofs) NOT shipped → deployed fresh bond reg carries ~1.5 MB Answer → honest FRESH ceiling = 1/block (2 MiB budget `node.go:270`).

## ★★★ THE RegCap COUNTING RULE IS REFUTED — value+rule REOPENED (Andrew authorized the correction)
Blind Research verdict `.../research-outcome/era4-regcap-value-VERDICT-2026-08-29.md`: a FRESH-ONLY cap is UNSOUND. Buckets fill with
EVERY BondReg (fresh AND renewal, `chain.go:2995`); #506 bounds renewals PER IDENTITY not per block → O(registry) ids can each renew once
in ONE block → one bucket = O(registry) → the wall era-4 removes. **Correct rule: cap per-block TOTAL BondReg count (= bucket population).**
Honest ceiling must be re-measured as `2 MiB / min(fresh, RENEWAL) reg size` (renewals pack smaller → ceiling >1, UN-measured; 256 may be too LOW).
Correction path: fix rule (design doc + decomposition hazard-3 + #635 canon) → re-measure → re-cert → re-ratify. #299 re-mint gate still applies.

## BUILD STATE
- Decomposition PACE `docs/thinking/2026-08-29-era4-build-decomposition-options.md`: **4a** schema/version-const+tags → **4b** maintenance
  spine → **4c** v5 predicate + RegCap + version-widen (predicate-first) → **4d** activation+mint-flip.
- **4a BUILT + CLEARED BOTH BLIND REVIEWERS** (commit `7241f82`, branch `era4-4a-schema-classification`): reserves `BlockVersionWitnessable=5`
  + tag STRINGS only, inert-by-guard. Tester PROMOTED; PE SHIP-WITH-FIXES (one DOC-only fix: the decomposition doc is cited in chain.go:346/
  statehash.go:67/CHANGELOG but not in 4a's tree — resolves when #635 lands on main). NOT merged.
- **PR #635** (canon record + decomposition doc, docs-only, green, NOT merged) — ⚠ still enshrines the REFUTED fresh-only RegCap; correct before merge.

## THREE HAZARDS (HOLD as gates)
1. New keyspaces MUST be v5-GATED in the leaf marshaller or committing them breaks the era-3 freeze. 4b ablation owed.
2. Rotate-LAST stale-capture (sharpest): any 4b hook after `rotateEpoch` reads `qualified` (`chain.go:3130`) freezes a STALE set = I3 divergence.
   PE adds: keep the coverage guard's `extra` branch STRICT in 4b/4c or the reservation stops being inert-by-guard.
3. The cap rule (REFUTED as fresh-only) must bound per-block TOTAL count. 4c ablation: honest renewal-heavy block ACCEPTS to ceiling; TOTAL>RegCap REJECTS.

## BUILD-TIME GATES (RECERT2 — each MUST ablate RED before its increment is trusted)
`qualified` maintenance drift-guard (per-site; 2989 reddens specifically) · T-3 dual-source guard (ablate missed renew old-bucket delete) ·
T-3 byte-identical era-3 replay corpus · Q5 recovery-branch agreement · existing completeness guards force the new tags.

Related: [[session-resume]], [[witness-floor-box-inc3-refuted]], [[witness-floor-box-track]].
