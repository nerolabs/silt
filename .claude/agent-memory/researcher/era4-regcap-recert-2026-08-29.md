---
name: era4-regcap-recert-2026-08-29
description: era-4 RegCap RE-CERT (value=9,320) GATED 2026-08-29; counting rule CERTIFIED, value REFUTED — 225 B min renewal is a PHANTOM (Answer-less reg; deployed verifier rejects); true min valid renewal ~1.4 MB → honest ceiling ~1 reg/block not 9,320.
metadata:
  type: project
---

# era-4 RegCap re-cert (value 9,320) — GATED

Re-cert of corrected RegCap claim (per-block TOTAL count cap, value 9,320 = floor(2MiB/225)).
Grounded `origin/main` HEAD `0076337`. Verdict **GATED**, filed
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-recert-VERDICT-2026-08-29.md`.
Supersedes [[era4-regcap-value-verdict-2026-08-29]] on the counting axis (Option B was named
there as the sound shape; this cert adopts it).

**The three-part split:**
1. COUNTING RULE (per-block total BondReg count, fresh+renewal, as v5 VALIDITY rule) —
   **CERTIFIED**. This is my prior Option B. Bounds due-bucket inflow, closes O(registry) wall.
2. STRUCTURAL Q3 (must be v5 VALIDITY rule, NOT the byte budget) — **CERTIFIED**.
   `MaxBondRegBytesPerBlock` is proposer-ONLY (`node.go:79-80` verbatim), absent from chain.go
   validity → an adversary is not bound by it. Only a replica-checked validity cap removes wall.
3. VALUE 9,320 and its 225 B floor — **REFUTED**.

**The decisive artifact (why 225 B is phantom):** EVERY reg fresh+renewal must pass
`verifyBond` at `chain.go:1617` → deployed `bond.VerifySpaceTime` (`objectivechain.go:34,50`,
BondVDFDelay=1000 k=64). A verifying Answer carries Samples=20 possession blocks + k=64 label
bundles (each Node+Pred+3 Parents = 5 blocks) + seed + VDF, ALL full BlockSize=4096 B, sample
counts FIXED (not size-scaled). Verifier cost `O((Samples+5k)·log n)` (`bond.go:51`). Raw block
bytes ≈ 341×4096 ≈ 1.33 MiB MINIMUM. Corroboration: `objectivechain.go:89` — renewal
re-broadcasts "full ~1.5 MB space-time proof." The 225 B reg has NO Answer → rejected. node.go
"renewals pack many" = SMALLER MinBond plots, not intrinsically-small renewals at fixed MinBond.

**Consequence:** true honest ceiling ≈ floor(2MiB / ~1.4MB) = ~1 reg/block, NOT 9,320. A
9,320-reg block is ~13 GB → the count cap NEVER FIRES → decorative; block-SIZE rule already
bounds bucket inflow. Certified boundary witness at 9,320 = 9,320×8×16KiB ≈ 1.11 GiB (>half the
2GiB box) — over-provisions against an attack the number does not bound.

**To lift:** measure TRUE min valid renewal reg (plot-seal→AnswerSpaceTime→EncodeAnswer→
bondRegEncode, expect ~1.4-1.5MB) under deployed knobs; decide per-block BYTE cap (bounds
bucket inflow directly if min entry is a full reg) vs count cap sized floor(block-byte-rule /
min-reg-bytes) with margin <16,384; re-check liveness edge (`node.go:73-74` — count cap too low
lapses renewals). Re-derivation gate must bind to ANY of (k, Samples, BlockSize, MinBond,
block-size rule), NOT #299 alone (#299 is ONE determinant; Q4 binding-to-#299-alone is
INCOMPLETE).

**I1-I5:** count/byte validity cap touches none (deterministic block predicate, I5 determinism
preserved); one caution = I4 liveness edge (don't reject honest renewal-heavy block).

**Scratch note:** left an empty `core/bond/regcap_measure_test.go` (blanked to `package bond`);
Builder should delete. It sketches the measurement the Tester should run.
