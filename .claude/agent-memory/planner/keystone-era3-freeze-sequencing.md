---
name: keystone-era3-freeze-sequencing
description: Era-3 freeze critical path — #600 witness-floor; TWO consensus forks found+fixed (#618, #622); order-independence axis COMPLETE; leave-one-out axis + format build remain.
metadata:
  type: project
---

The era-3 committed state-root format freeze is the pacing gate in the keystone track (Phase-4 track 3),
on the CRITICAL PATH after #600. main HEAD `9a9cc4e`.

**Probe trip-wire (PE):** test/fixture work is NOT research-gated PROVIDED the rule isn't touched.
*Probe the threshold; don't move it.* STOP → Researcher on the weight sum (`chain.go:2450-2456`), the
`epochSet` freeze (I3, `rotateEpoch`), or `⌈A/2⌉` vs `⌊A/2⌋+1`.

## ★★★ TWO SAME-BLOCK bond-reg forks — BOTH FIXED (session-9)
The stressing-ordering push through `orderVacuous` uncovered TWO latent intra-block bond-reg order-
dependence forks in committed SMT fields (identical block content in two slice orders → different root),
both caught + fixed before the era-3 freeze. **Different collisions → different fixes** (key lesson: do NOT
assume fork N's fix mirrors N-1's — Research REFUTED the mirror-#618 guess for fork 2).

**FORK 1 — distinct-ID same-root — CLOSED (#618).** `seenRoot` per-root dedup in `validateBondRegs`
(UNCONDITIONAL, admits same-ID renew/resize; `ErrSharedRootInBlock`). REJECT works — distinct-id-same-root
has NO legit form. Cert `.../same-root-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`
+ ADDENDUM; Rulings `.../RULING-618-*`.

**FORK 2 — same-ID two-version — CLOSED (#622, MERGED `9a9cc4e`).** A same-id two-version block (any roots)
pre-#506-gate was ADMITTED (`seenReg` gate-gated; `seenRoot` exempts same-id), and `apply()` was
last-writer-wins over `regVersion`/`bondDomain`/`bonded` → order-dependent; `regVersion` feeds the #506
lock-in tally so `gateLockedIn`/`gateHeight` inherited the split. **Fix = CANONICALIZE (not reject —
resize is legit):** `canonicalBondRegs`/`bondRegLess` fold each id to one winner by TOTAL ORDER
Size→Version→Domain→Sig, winner-takes-all-fields. Also closes the same-id two-root variant (fold-safe under
F1 one-plot-one-standing). Certified + Andrew-ratified + triple-cleared (PE SHIP, Researcher CERTIFIED,
Tester PROMOTED RED-then-GREEN). Cert `.../sameid-twoversion-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`
+ CERT-ADDENDUM; Rulings `.../RULING-622-sameid-canonicalize-apply-2026-08-28.md`.
**Meta-flag for the era-3 format design:** 2 apply() order forks in this surface this session → consider a
GENERAL per-id canonicalization pass rather than patching one collision class at a time.

## ★ ORDER-INDEPENDENCE AXIS COMPLETE — `orderVacuous` EMPTY
Every committedSet field is covered on the order-independence list (spent/slashed #617; bond-reg family
#618/#619; epochSet/everMature/matureEpoch #620; regVersion/bondDomain/gateLockedIn/gateHeight #622).

## ★★ THE FREEZE GATE — union of BOTH lists; the LEAVE-ONE-OUT axis is what remains
FREEZE-READY per field = clears BOTH `orderVacuous` (order-indep, now EMPTY) AND `probeUncovered`
(leave-one-out / completeness), with could-diverge orderings. **Freeze-ready NOW (both lists):** `spent`,
`slashed`, `bonded`, `bondRootOwner`, `bondRootProven`, `epochSet`. **Owed on the LEAVE-ONE-OUT axis
(Researcher R4-LOO):** `everMature`, `matureEpoch`, `regVersion`, `bondDomain`, `gateLockedIn`, `gateHeight`
— these cleared order-independence but NOT completeness. That is the next coverage tranche.

## ★★ GENESIS residual — NAMED (#619, option a); tripwires live
`AppendGenesis` skips the dedup; genesis apply() order-dependent for distinct-id unproven same-root regs,
safe only by byte-identical + ZERO-BondRegs genesis. #619 tripwire tests fire if that changes. Option (b)
DEFERRED. (These tripwires CAUGHT the #622 Builder's over-broad first fold — they earn their keep.)

## ★ Standing couplings to pin before freeze
- **rotateEpoch must be LAST in `apply`** — `epochSet` order-independence depends on it (#621, open).
- **R6-MIXED:** same-id two-root fold-drop shown-safe by analysis; a dedicated fixture is recommended (not
  required) to make it probe-proven.

## ★ #600 RESOLVED — witness-floor (canon #615)
Floor box = semi-stateless witness-validating full validator; hold-tree = bigger-box opt-in behind
`ports.NodeStore`; witness-serving stays OPEN + MULTI-PROVIDER. Rulings:
`.../RULING-600-floor-box-direction-2026-08-28.md`, `.../600-floor-box-direction-post-coexistence-RESEARCH-NOTE-2026-08-28.md`.

## ★ Era-3 format — Block commits NEITHER root today (`chain.go:311-390`)
C-7 obligation (ratified): era-3 `Block` must commit BOTH roots (state SMT + transparency-log) over the
proven field set + verifier invariant "no witness → never accept (stall)". Format BUILD not started.

## ★ Critical path (updated)
1. **Leave-one-out coverage** for the 6 fields above → then the field-set is coverage-complete (both lists).
2. **Build era-3 format** — commit BOTH roots; pin rotateEpoch-last (#621); consider general per-id
   canonicalization.
3. **Witness floor-box validation** — open/multi-provider; "no witness → stall".
4. **FREEZE** — VETO GATE: Researcher certifies composed format, Andrew ratifies.

## Landed this session
#613 (O(depth) gate; #616), #617 (spent/slashed + guard), #618 (fork-1 fix), #619 (genesis premise),
#620 (mature-epoch; #621), #622 (fork-2 canonicalize). #607 CLOSED. Details/scars: [[session-resume]].

## Standing rule — MatureCoefficient is a consensus predicate (PE #611)
`chain.MatureCoefficient()` (chain.go:1848) feeds BOTH the maturity shed AND the λ_H floor. Any edit is a
consensus-rule change → research-gated. Drift-guard `h4_consensus_test.go:186-206` pins it.

Related: [[session-resume]], [[vision-research-parallel-lane]], [[planner-isolate-mutating-seats]], [[seat-scratch-file-hygiene]].
