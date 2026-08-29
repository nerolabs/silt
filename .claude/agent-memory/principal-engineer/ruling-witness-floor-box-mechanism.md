---
name: ruling-witness-floor-box-mechanism
description: Ruling on the 3 open witness floor-box construction items (R3 DoS bound / Delivery / R4 accessor) — 2026-08-29, era-3 format frozen
metadata:
  type: project
---

# Ruling — witness floor-box mechanism (R3 / Delivery / R4), 2026-08-29

Filed: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-witness-floor-box-mechanism-2026-08-29.md`
Head `2003439` (era-3 committed state-root format FROZEN, #632). Judged blind of the
`docs/thinking/2026-08-29-witness-floor-box-*` design doc, from source + C-7 cert.

**Why:** three open construction items for the ratified semi-stateless witness-validating
floor box (holds roots, not the tree; validates by membership/non-membership proofs).
**How to apply:** these are the verdicts to hold when the floor-box components get built.

## Per-item verdicts
- **R3 (DoS bound):** aggregate per-block byte ceiling at the INGEST boundary, BEFORE
  proof verification. Per-proof cap does NOT suffice (library caps ONE proof at 256
  side-nodes, `proofs.go:57`; nothing caps proof COUNT or unread-key padding). Menu
  MISSED the stronger option: a SHAPE GATE — witness must carry a proof for EXACTLY the
  read-set, no unread keys. Ship shape-gate + byte-ceiling; shape gate is stronger. The
  ceiling VALUE is a security parameter → research-gated (precedent: SHA-256 flagged as
  sec param, `statehash.go:117`). Mechanism is mine; value is Research's.
- **Delivery:** ON-DEMAND any-of-N, first-correct-wins, no permission bit, is the `:557`
  floor default. In-block carry + gossip are PERMITTED accelerations, layered on top —
  they compose, not either/or. NO option needs a format change (witness lives OUTSIDE the
  frozen block, un-committed side data; keep it out of `Hash()`). Bright-line trap =
  privileged/permissioned provider → `:557` violation, reject.
- **R4 (accessor):** THREE-VALUED — PROVEN_PRESENT(value)/PROVEN_ABSENT/NO_WITNESS. Make
  PROVEN_ABSENT constructible ONLY from a verified non-membership proof; every failure
  (R3-rejected, Delivery-failed, no proof) → NO_WITNESS → stall. Two-valued+flag = same
  bug surface, worse; panic = violates stall posture (remote DoS). silt's SINGLE root over
  the whole 18-field keyspace (`statehash.go:10,34-62`) CLOSES the sharded-omission
  soundness hazard C-7 §96 flagged — it becomes a delivery-availability concern, not a
  soundness one.

## The coupling the menu missed (highest value)
The three are ONE pipeline: fetch→bound→verify→accessor. R3-over-budget AND Delivery-fetch-
fail MUST both map to R4 `NO_WITNESS`, never `PROVEN_ABSENT`. That single mis-wire IS the
one banned move in C-7 §104 ("no witness → accept"). Build R4's three-valued type FIRST
(safety spine); then R3/Delivery cannot introduce the conflation bug by construction.

## Load-bearing facts I verified
- `pokt smt@v1.0.0 proofs.go:57` per-proof cap = PathSize*8 = 256 (SHA-256, PathSize 32).
- `ValidateProposal` (`chain.go:2245-2340`) enforces NO per-block byte cap; only bound is
  transport `maxFrame = MaxChunkSize+overhead` = 128 MiB (`tcpnet.go:72`). **Builder's R3
  framing ("bounded by existing payload caps") OVERSTATES it** — #441 "separated budget"
  is mempool FAIRNESS (`consensus-invariants.md:127`), not a DoS ceiling.
- Committed root = SINGLE SMT over whole 18-field field-tagged keyspace (`statehash.go`).
- Grind defense real: position = SHA-256(key), depth ≈ log2(n) (`exclusion_test.go:221-283`).
- C-7 cert (`.../C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`)
  certifies safe-degradation + 3-forgery rejection; I built on it, did not re-derive it.

## Classification
NONE of the three is a consensus-rule change (I1-I5) — all reproduce already-committed
predicates against the frozen root (validation/delivery layer). ONLY the R3 ceiling VALUE
is research-gated. The one call that's Andrew's: ratify the R3 sec-param value after
Research certifies.
