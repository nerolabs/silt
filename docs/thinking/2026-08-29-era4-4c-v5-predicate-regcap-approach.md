# era-4 4c — v5 validity predicate + RegCap + version-widen (PACE, build increment)

**Date:** 2026-08-29
**Seat:** Builder
**Status:** build increment 4c of the ratified era-4 decomposition. IMPLEMENTS a certified design;
does NOT re-open it.
**Branch:** `era4-4c-v5-predicate-regcap` off `origin/main` @ `2ab87a0` (has 4a + 4b).
**Grounded against:** `origin/main` @ `2ab87a0` (every `file:line` below re-checked on this commit).

**Certified inputs this increment implements (do NOT re-litigate):**
- Decomposition `docs/thinking/2026-08-29-era4-build-decomposition-options.md` §1 (4c row), §3
  (PREDICATE-FIRST), §7.3 (RegCap counts per-block TOTAL, fresh + renewal).
- RegCap counting rule CERTIFIED (per-block TOTAL after `canonicalBondRegs`) and VALUE N=256
  CERTIFIED for the total rule:
  - `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-recert-VERDICT-2026-08-29.md`
    (rule + Q3 must-be-validity-not-byte-budget CERTIFIED).
  - `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`
    (N=256 CERTIFIED for the total rule; floor N≥18 at k=1; ceiling ≤16,384; I1–I5 clean).
- `docs/decisions.md` era-4 entry + `docs/design/owned-residuals.md` RegCap entry (total-count,
  #299 re-mint gate broadened to all seven determinants).

**No deviation from the certified rule/value/design is required.** The task is a faithful
implementation. This PACE proceeds to build.

---

## 1. The answer up front — what 4c changes, and why it is small

4c is a SMALL code delta because 4b already built all the v5 root machinery. Two changes:

1. **RegCap validity predicate (new code).** Reject a v5 block whose TOTAL BondReg count, after
   `canonicalBondRegs`, exceeds `RegCap = 256`. Placed in `validateBondRegs` (`chain.go:1640`), the
   one per-block bond-reg validity function. v5-gated (fires only for `b.Version >=
   BlockVersionWitnessable`); v4/earlier are byte- and behaviour-identical.

2. **Version-widen, PREDICATE-FIRST.** Widen `versionSupported` (`chain.go:758`) from
   `<= BlockVersionStateRoot` (4) to `<= BlockVersionWitnessable` (5). Update the two `Decode`/
   `DecodeBlocks` error strings and one schema-test assertion that used `BlockVersionStateRoot+1`
   (=5) as the "beyond the ceiling" version.

**Why the v5 committed-root predicate needs NO new code.** The era-3 root predicate
`validateEra3Roots` (`era3validity.go:88`) gates on `b.Version < BlockVersionStateRoot` (< 4). A v5
block is `>= 4`, so it already flows through the predicate, and `postApplyRoots`
(`era3validity.go:119`) recomputes via `StateRootForVersion(b.Version)` (`statehash.go:225`), which
selects the v5 leaf marshaller for `version >= BlockVersionWitnessable`. The predicate is ALREADY on
every disk-write path: the commit path (`ValidateProposal:2452` → `ValidateCommit`) and the own-disk
Reload path (`appendStructural:2933`). So "the v5 committed-root validity predicate on every
disk-write path incl. Reload" (decomposition §1, 4c) is satisfied the instant `versionSupported`
accepts v5 — the machinery 4b wired makes it automatic. 4c's job is to open that gate atomically
with RegCap, not to write a new predicate.

**Why RegCap goes in `validateBondRegs`, not `appendStructural`.** RegCap is a bond-reg CONTENT
validity rule that replicas enforce ON RECEIPT (recert Q3). `validateBondRegs` is called from
`ValidateProposal:2415`, which `ValidateCommit` invokes first — so every replica accepting a block
via consensus runs it. The own-disk Reload path (`appendStructural`) replays THIS node's
already-committed blocks, which passed RegCap when they committed; re-checking a content rule there
is redundant (the root predicate on Reload already catches any post-commit disk tampering). This
mirrors the existing split: `validateBondRegs`/`validateSlashes`/entry checks live on the commit
path only; only the era-3 version + root checks are duplicated onto Reload (they are the ones a
re-signed tampered disk block could otherwise slip). RegCap is not that class — a tampered
over-cap disk block fails the root predicate on Reload, because its committed BondRegs change the
post-apply `dueBucket`/`qualified`/`bonded` leaves and the recomputed StateRoot no longer matches.

---

## 2. The RegCap predicate — exact rule

```
const RegCap = 256   // era-4 (v5) per-block TOTAL BondReg count validity ceiling.

// inside validateBondRegs, v5-gated:
if b.Version >= BlockVersionWitnessable && len(canonicalBondRegs(b.BondRegs)) > RegCap {
    return ErrRegCapExceeded
}
```

- **TOTAL count, any mix.** Counts `len(canonicalBondRegs(b.BondRegs))` — fresh AND renewal, no
  distinction. This is the flipped rule: the REFUTED fresh-only form (which counted only
  `bondRegHeight[id]` unset regs) is NOT implemented.
- **After `canonicalBondRegs`.** Same-id folding (`chain.go:3212`) applies before the count, so a
  block that lists one id twice counts as one reg — matches the due-bucket geometry (one canonical
  reg = one bucket entry, VALUE cert Q1).
- **v5-gated.** `b.Version >= BlockVersionWitnessable`. A v4 block is never subject to RegCap (era-3
  is frozen; RegCap is a v5 rule). This keeps v4 byte- and behaviour-identical.
- **Placement in `validateBondRegs`.** At the top of the function, after the `!c.objective()` and
  `IsPruned()` early returns, before the per-reg loop. Rationale: it is a cheap O(1)-after-fold
  count gate; failing fast avoids per-reg space-time verification on an already-over-cap block. It
  does NOT depend on the per-reg loop's state (`seenReg`/`seenRoot`), so ordering is free.

**Placement caveat (pruned blocks).** A pruned block returns early (`chain.go:1653-1667`) before the
canonical-count site if I place RegCap after that return. That is correct: a pruned block's BondRegs
are Answer-less and trusted only strictly below the anchor (already finalized); its reg count was
capped when it originally committed as a full block. RegCap is enforced on the full-block commit
path, where it bounds the due-bucket inflow. I place RegCap AFTER the `IsPruned` early return, so
the count gate applies to full (verifiable) blocks — the ones that populate a live due-bucket.

---

## 3. Version-widen — PREDICATE-FIRST, atomic

`versionSupported(v) = v >= 1 && v <= BlockVersionStateRoot` becomes
`v >= 1 && v <= BlockVersionWitnessable`. Shipped IN THE SAME increment as the RegCap predicate and
riding on the already-wired v5 root predicate — so no v5 block is ever decode-accepted before its
validity (root + RegCap) is enforceable. This closes the era-3 interim window (decomposition §3).

Touch points for the widen (all in `chain.go` unless noted):
- `versionSupported` (`:758`) — the ceiling.
- `Decode` error string (`:766`) — `want 1..%d` now names `BlockVersionWitnessable`.
- `DecodeBlocks` error string (`:816`) — same.
- Schema test `TestV4DecodesAndIsAccepted` (`modelcheck_era3_schema_test.go:132-139`) — the
  "beyond the ceiling" future version moves from `BlockVersionStateRoot+1` (=5, now SUPPORTED) to
  `BlockVersionWitnessable+1` (=6). Without this the test breaks: it asserts v5 is REJECTED, which
  is now false. This is a test truing-up, not a rule change.

**Not touched (out of scope, 4d):** `BlockVersion` (minted version) stays `BlockVersionRounds`;
`MintVersion` (`chain.go:3360`) stays `BlockVersionStateRoot` at/above the era-3 boundary. No node
mints v5 from this increment. No activation height. `MaxBondRegBytesPerBlock` (proposer policy,
`node.go:270`) untouched. The 4b maintenance spine untouched.

---

## 4. The ablation plan — every gate RED before trust (inject → RED → restore green)

Each row: the defect I inject, the RED I must observe, then restore. A green check with no
demonstrated red is a comment that compiles (session-7 BIG LESSON).

| # | Gate (invariant) | Defect injected | Expected RED |
|---|---|---|---|
| A | **RegCap total-count, ALL-FRESH** | remove the RegCap predicate | a v5 block with 257 all-fresh regs ACCEPTS (should reject) |
| B | **RegCap total-count, ALL-RENEWAL** | remove the RegCap predicate | a v5 block with 257 all-renewal regs ACCEPTS (should reject) — the flipped ablation: the OLD fresh-only rule wrongly accepted this |
| C | **RegCap total-count, MIXED** | remove the RegCap predicate | a v5 block with 130 fresh + 127 renewal (257 total) ACCEPTS (should reject) |
| D | **Counted after `canonicalBondRegs`** | count `len(b.BondRegs)` instead of `len(canonicalBondRegs(...))` | a block of 257 regs that fold to ≤256 distinct ids wrongly REJECTS (a same-id renew/resize pair counted as two) |
| E | **I4 liveness — at-ceiling accept** | (no defect; positive gate) | a v5 block AT the ceiling (exactly 256, any mix) ACCEPTS. Ablation of the gate: set `RegCap = 255` and watch the 256-block wrongly reject |
| F | **v4 unaffected** | (no defect; positive gate) | the era-3 byte-identical replay corpus + a v4 block with 257 regs BOTH stay green (RegCap is v5-only; v4 has no count cap) |
| G | **Predicate-first ordering** | widen `versionSupported` to `<=5` but SKIP the RegCap gate (simulate the wrong order) | a v5 block with >256 regs decode-accepts AND commits with no cap = RED (an unbounded v5 block accepted) |

**On gate G — the honest form.** "Predicate-first" for 4c means the RegCap gate and the version
widen ship together, and the root predicate is already wired. The strongest available ablation:
temporarily comment out the RegCap gate while leaving `versionSupported <= 5`, and show a >256-reg
v5 block commits (unbounded). Restore, and it rejects. This proves the widen without the cap is the
hole, i.e. the cap is what the widen is gated behind. The root-predicate half of predicate-first is
covered by the existing era-3 Reload/commit root tests, which already run for v5 via
`StateRootForVersion` (I add a v5 re-signed-wrong-root Reload ablation to prove it fires for v5, not
only v4).

**On gate F — the era-3 byte-identical replay.** The 4b increment already carries the
byte-identical v5-vs-era-3 replay and the completeness/clone guards. 4c adds NO leaf, so those stay
green unchanged. I run the full `core/chain` model-check tier to confirm.

### Test I add (the regression test, at the tier the rule lives)

A `core/chain` model-check test `modelcheck_era4_regcap_test.go` (unit/model-check tier — RegCap is
a consensus validity rule, so it lives beside the other `modelcheck_*` block-validity tests). It
builds v5 blocks at/over the cap in all three mixes (fresh/renewal/mixed), asserts reject > 256 /
accept ≤ 256, asserts the count is post-canonicalization, and asserts a v4 block over 256 is
unaffected. Each assertion has a matching ablation demonstrated in the report.

The RegCap block-building needs real space-time BondRegs (each must pass `verifyBond`). I reuse the
existing era-4/era-3 test fixtures' bond-reg construction (the same helpers 4b's maintenance tests
use to build fresh/renewal regs), so the regs are valid and the count gate is the only thing under
test. If the per-reg verify cost makes a 257-reg block too slow for the model-check tier, I inject a
test-local permissive `verifyBond` (as the existing chain tests do) so the test isolates the COUNT
rule, not the space-time verifier — the count gate is a pure function of `len(canonicalBondRegs)`,
independent of whether each reg verifies.

---

## 5. Invariants + scope check

- **I1–I5:** RegCap is a new block-validity predicate, a pure function of `(block, cfg)`, computed
  identically on every replica → I5 determinism preserved. Touches none of I1 (quorum
  intersection), I2 (never-sign-twice), I3 (validator-set boundary), I4 (commit-vs-final
  structurally), I5 (fork-choice/slashing). The one I4 liveness edge — do not reject an honest block
  — is why N=256 sits 14× above the k=1 honest ceiling of 18 (VALUE cert Q3). CERTIFIED clean.
- **era-3 freeze immutable (#632):** RegCap and the widen are v5-gated / additive. No v4 block's
  bytes or verdict change. The frozen era-3 format is untouched.
- **Scope-out confirmed:** no mint-flip, no activation, no `MaxBondRegBytesPerBlock` change, no 4b
  spine change. All 4d or out-of-band.

## 6. Re-derivation gate (recorded, not enforced-in-code)

N=256 = f(B, k, Samples, BlockSize, BondVDFDelay, MinBond, proof scheme) — all SEVEN determinants
(VALUE cert, "re-derivation gate"). Any change to any one re-derives N at the next `BlockVersion`
mint. #299 (succinct proofs) is the sharpest single determinant (raises honest ceiling above 256 →
forces a re-mint) but NOT the only one. This is recorded in `docs/decisions.md` / `owned-residuals.md`
and in the `RegCap` const doc-comment; it is a documentation obligation, not a runtime check (the
value is a frozen v5 format constant until the next mint, exactly like `SProofMax`).
