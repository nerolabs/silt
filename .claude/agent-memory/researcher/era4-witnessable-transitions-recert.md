---
name: era4-witnessable-transitions-recert
description: era-4 RE-CERT ROUND-2 2026-08-29 → CERTIFIED-WITH-CONDITIONS (LIFTED from GATED); Q1 two-keyspace boundary SOUND (epochSet frozen kept, qualified live accelerator, boundary=distinct O(boundary-delta) class); Q2 RegCap = MEASUREMENT-REQUIRED (proposer byte budget bounds honest not adversary, cannot pin at desk); build-guards owed
metadata:
  type: project
---

# era-4 witnessable transitions — RE-CERT ROUND 2 (rev-2 design)

Round-2 re-cert of the rev-2 era-4 design (BlockVersion=5). Verdict: **CERTIFIED-WITH-CONDITIONS**
(lifted from STILL GATED). Cert at
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`.
Grounded `origin/main` @ `0984db4`. Supersedes [[era4-witnessable-transitions-recert]] prior round.

**Why: rev-2 folded my prior Q2 REFUTATION (kept epochSet frozen, added qualified as live
accelerator, boundary as distinct witness class) + asked the λ_H desk-bound question.**
**How to apply: the DESIGN is sound and buildable. The ONE remaining hard blocker is the RegCap
VALUE — it is MEASUREMENT-REQUIRED, cannot be pinned at desk. Do not build with an invented cap.**

## Q1 — the two-keyspace boundary is SOUND (rev-2 lifted the REFUTATION)
Rev-2 adopts my prior direction (b): `epochSet` STAYS its own frozen committed keyspace
(`statehash.go:40,101-103` tagEpochSet; `c.epochSet` written ONLY at rotateEpoch `chain.go:3131`
+ adopt `3546`, verified by grep — all other c.epochSet sites are reads). `qualified` is a
SEPARATE live keyspace (boundary-computation accelerator). Boundary block = COPY epochSet:=qualified,
correctly stated as a DISTINCT heavier witness class, changed-leaf set O(boundary-delta) bounded
by RegCap×EpochBlocks×SProofMax — NOT O(payload)/zero-leaves. Mid-epoch immutability preserved:
frozen set read by requireEpochWeightQuorum (`chain.go:2597` via effectiveEpochSet) + RoundCatchupMet
(`chain.go:2631,2638`); qualified never contaminates it. Q5 recovery coupling correctly named
(recovery branch `liveQualifiedSet()` must agree with frozen epochSet — a build assertion). CERTIFIED.

## Q2 — RegCap VALUE: MEASUREMENT-REQUIRED (verdict (b), cannot close at desk)
Upper bound RegCap ≤ 16,384 DERIVED + EXACT (2 GiB / (8 × 16 KiB) = 2^14; EpochBlocks=8
`daemon.go:1729`, SProofMax=16KiB `witness_bound.go:78`). The desk-bound argument via proposer
byte budget FAILS to pin the value:
- `MaxBondRegBytesPerBlock=2MiB` is PROPOSER-ONLY (`chainrole.go:798`); `node.go:79-80` states
  verbatim "Proposer-side policy only — validity is unchanged (a block with N regs is valid)".
  Grep confirms it appears NOWHERE in chain.go validity. So it bounds the HONEST proposer's λ_H,
  NOT an adversary. It cannot be the validity bound.
- Even for honest λ_H: min valid fresh-reg size is NOT pinned in chain.go. A valid reg needs
  ed25519 pubkey(32)+sig(64)+Size≥MinBond≥MinBondBytes+verifyBond(Answer) accepts (`chain.go:1604-1620`).
  Answer (space-time proof, `omitempty`) size is a property of the INJECTED verifier + plot, not
  a chain constant. Renewals pack small (`node.go:72`); genesis ~1.5MB. So 2MiB/min-reg-size is
  not a desk-computable integer.
→ RegCap needs a MEASURED min-valid-fresh-reg byte size (under the deployed verifyBond) composed
against the R3 C_block budget to pick a value in [λ_H, 16,384] with margin. Same class as SProofMax.
The proposer-budget composition does NOT change the prior "cannot close at desk" verdict.

## What LIFTED vs prior round
- Q1 boundary: REFUTED → CERTIFIED (rev-2 is the correction).
- Five-site enumeration, canonical pin, O-1, MinBond dormancy, ordering: CLOSED (unchanged).
- Q2 cap value: still the one hard gate, now sharpened to MEASUREMENT-REQUIRED (not desk-boundable).

## Owed at BUILD (standing "inject the defect" rule — build-time, NOT cert blockers)
qualified maintenance guard (per-site ablate, 2989 reddens); T-3 dual-source guard (renew
old-bucket-delete reddens); T-3 byte-identical era-3 replay (renew-reset/ttl==0/slash-before-due).
Recovery boundary (R2) = human's separable call. Format veto-gate (BlockVersion=5, new tags,
RegCap rule) = human ratifies.
