---
name: era3-format-design-and-consults
description: era-3 committed state-root format — the composed design, the Researcher+PE consults (CONVERGED on mint-v4), the freeze conditions, and the DECISIONS-FOR-ANDREW (awaiting ratification).
metadata:
  type: project
---

# era-3 committed state-root format — design + consults (2026-08-28)

The era-3 FORMAT is the state-root keystone: `Block` gains a `StateRoot` (SMT over the 16 committedSet
fields) + `LogRoot` (RFC-6962 over revLog), attester-signed, enabling witness/stateless "floor-box"
validation. Research-gated + terminates in the FREEZE veto gate (Andrew ratifies). Design deliberation
(Builder, design-only): `docs/thinking/2026-08-28-era3-format-design-options.md` (first written in worktree
`agent-a546e47a231b4486b` — ensure it lands durably with the first era-3 build PR).

## ★★★ BOTH CONSULTS CONVERGED — CERTIFIED-WITH-CONDITIONS, mint v4 not v3. AWAITING ANDREW RATIFICATION.
PE ruling `RULING-era3-committed-state-root-format-2026-08-28.md`; Researcher cert
`.../research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`. Run INDEPENDENTLY,
they reached the SAME load-bearing conclusions:
- **MINT `BlockVersion = 4`, NOT 3 (both, via the SAME verified source `chain.go:668`).** `versionSupported(v)=
  v>=1 && v<=BlockVersionRegGate(=3)` → a current un-upgraded binary ALREADY decode-accepts a v3 block and
  validates it under era-2 rules (no state-root predicate) → a forged StateRoot rides through unvalidated =
  SILENT MIS-VALIDATION. era-3 ADDS a predicate → HARD fork, not the #506 soft fork (whose reject-only rule
  shrank the valid set, a subset property that let it skip a mint bump). Fix: mint 4, extend `versionSupported`
  to <=4 same release; a laggard STALLS loudly (`ErrBlockVersion`) — safety-first. Dissolves the const-3
  collision (3 stays #506 readiness; era-3 signals `regVersion>=4`).
- **Value-encoding = consensus SAFETY (PE) / CERTIFIED-with-condition (Researcher).** 3 super-quorum predicates
  SUM `bonded`/`epochSet` weights (`chain.go:2527,2593,3150`) → a wrong-weight witness poisons a tally.
  Researcher verified the pokt SMT FOLDS value into the leaf digest (`proofs.go:428`, `hasher.go:111`,
  `exclusion_test.go:174`) so a wrong-value witness can't reconstruct the root — SOUND, PROVIDED a FIXED
  CANONICAL value encoding + a byte-identical-leaf cross-node DETERMINISM ORACLE (owed, R2).
- **Verifier invariant (both):** needs the missing-witness ≠ verified-exclusion distinction; witness pins
  `(value, present)`, predicates branch on both, never Go's map-zero default (`bondDomain=0` is the trap).
- Q1/Q3/Q6 CERTIFIED sound (Q3 NUL-terminated tags injective). Q5 sound WITH v4 (boundary immutability transfers
  via #357 Cond A; the soft-fork acceptance clause does NOT — the v4 fix). Q6: sha256 pin + value widths are
  consensus params (name them); witness-size DoS bound deferred (not a freeze blocker).

## FREEZE CONDITIONS (both seats, all required)
1. Mint `BlockVersion = 4`. 2. Coverage gate green (= #603, see below). 3. Fixed canonical value encoding +
byte-identical-leaf determinism oracle (owed). 4. Empty roots as fixed constants; both fields REQUIRED (not
omitempty). 5. Record witness-open constraints so the C-7 witness follow-on isn't precluded.

## ★★ DECISIONS FOR ANDREW — VETO GATE, presented, AWAITING RATIFICATION
1. Ratify mint-v4 + the HARD-fork posture (laggard STALLS at H_era3 rather than accepting unvalidated roots).
2. Confirm the freeze is HELD until the coverage gate (#603) is green.
(Soft-activation 2A stands as the mechanism; flag-day 2B not chosen. Freeze scope = 16 committedSet + revLog root.)

## ★ #603 = the coverage gate — traced: discharges when #626 lands
#603's substance = consensus-weight + spent/slashed fields reaching the keystone oracles green. Weight-bytes
half DISCHARGED (`epochSet` #606, `bonded` #624). The rest = the completeness axis, CLOSED pending #626 merge.
So "#603" is NOT a separate blocker — it discharges when #626 lands. Verify + formally close the issue.

## Post-ratification build sequence (certified, model-check each step)
Promote SMT root computation from `internal/smtspike` into core behind the oracles → add value-encoding
determinism oracle (R2) → add `StateRoot`/`LogRoot` to `Block` (mint v4, tighten `versionSupported`) → wire the
era-3 validity predicate (committed root == recomputed) → height-gated activation → Researcher re-cert of the
composed built format → FREEZE (Andrew). Witness mechanism is a separate C-7 follow-on (R3/R4).

Related: [[session-resume]], [[keystone-era3-freeze-sequencing]].
