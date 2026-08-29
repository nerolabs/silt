---
name: ruling-617-order-independence-spent-slashed
description: PR #617 blind review — SHIP; spent/slashed coverage genuine, vacuous-∅ guard is a real two-way ratchet, all 11 orderVacuous declarations are honest deferred debt
metadata:
  type: project
---

# Ruling: PR #617 order-independence + leave-one-out for spent/slashed — SHIP

Verdict **SHIP** (blind review, branch `keystone-order-independence-spent-slashed`, head `ee1bcf3`, base `7d2a292`). Answers `RULING-keystone-spent-slashed-classification-2026-08-28.md` (the vacuous-∅ finding). Test/fixture-only; no consensus rule moves (chain.go untouched in diff).

**Why:** This PR discharges the covering-probe condition the prior ruling made a prerequisite for freezing the `spent`/`slashed` era-3 leaves.

**How to apply:** `spent` and `slashed` are now freeze-ready on BOTH oracles. The other committedSet leaves are not — see the coupling below before any freeze decision.

## What I verified by my own execution (not the PR's ablation claims)
- Guard defeated in BOTH directions by injection: (a) removed `bonded` from `orderVacuous` → RED "EMPTY in both orderings and NOT declared"; (b) declared populated `spent` in `orderVacuous` → RED "populated ... yet still listed ... stale entry". Direction (b) makes orderVacuous a STRUCTURALLY SHRINKING debt — you cannot park a reachable field there and leave it.
- Leave-one-out flips real: `spent` full=reject/ablated=accept (double-spend); `slashed` full=disqualified/ablated=qualified (re-admit via launchAnchor). Flip depends on `slashed` alone: `attesterQualifiedAt` refuses slashed at chain.go:1027 BEFORE launchAnchor fallthrough at :1046.
- All 11 `orderVacuous` entries legit against write sites: bond-reg family (6) all in one loop over b.BondRegs (chain.go:2791-2797), fixture commits zero BondRegs; mature-epoch family (3) gated on Mature()>=99 coefficient, 4 bond-less anchors never reach it; #506 gate (2) doubly unreachable (never-mature + no ready-signal). validatorsSeen correctly NOT vacuous (populated at :2827).
- Hardest scrutiny: `bonded` IS populatable (residual test bonds 2 via genesis), so parking it is a choice. NOT dodging: bonded's only order-sensitive path is paired delete(c.bonded,culprit) at :2820, and TestBondedOrderFreeUnderSlashInteraction exercises exactly that (2 opposite-order deletes commute). The deferred bit — bondRootOwner G3 proof-beats-declaration displacement — is NAMED as order-sensitive and owed.

## Coupling the consult missed
TWO debt lists now overlap and DISAGREE — correctly. `probeUncovered` (leave-one-out) dropped bonded/epochSet; `orderVacuous` (order-independence) still lists them. "Covered" is PER-ORACLE. The freeze gate for a leaf is the UNION of both lists empty for that field, not either alone. Only `spent`/`slashed` clear both. Do not mistake a half-covered field (leave-one-out done, order-independence still ∅) for freeze-ready.

## Residuals (non-blocking)
Bond-registration ordering probe (esp. bondRootOwner G3) owed before those 6 leaves freeze; still no adversarial SCHEDULE in the oracle (grow-only union makes it low-risk).

See [[ruling-keystone-spent-slashed-classification]] and [[ruling-keystone-probes-bonded-epochset]].
