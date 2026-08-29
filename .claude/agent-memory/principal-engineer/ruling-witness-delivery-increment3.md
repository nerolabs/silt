---
name: ruling-witness-delivery-increment3
description: Witness floor-box increment 3 — delivery mechanism (Part B) + ReadEntry two-branch type gap; ship-with-gate + real structural gap
metadata:
  type: project
---

# Ruling: witness floor-box increment 3 — delivery + ReadEntry type gap (2026-08-29)

Judged at `0984db4` (#634, ahead of main `2003439`; NOT yet merged to main despite
consult framing "MERGED on main 0984db4"). Files: `core/statehash/witness.go` (R4
accessor), `witness_bound.go` (R3 DoS gate). Blind judge (did not read author design doc).

**Why:** floor box validates a block by verifying SMT proofs against the committed root;
increment 3 adds on-demand witness DELIVERY over a side channel + surfaces a ReadEntry gap.

**How to apply:** the two verdicts + the couplings below govern any follow-on witness work.

## Verdicts
- **Delivery (Part B): SHIP-WITH-ONE-RESEARCH-GATE.** any-of-N first-correct-wins is
  correct and reuses `fetchFrom` (`core/node/file.go:414`) verbatim — verify-every-byte
  (`file.go:480`), transient-only re-sweep (`file.go:448,490`), per-provider deadline
  (`node.go:1272` AfterFunc(requestTimeoutFor)). All 3 knobs exist (`FetchAttempts`
  node.go:143, `requestTimeoutFor` :1198, `RequestSizeFloorBytesPerSec` :61). Failure
  →NoWitness wiring SOUND — I traced every path, NONE reaches ProvenAbsent (only site:
  Resolve len==0 && verify-true). `:557` no-privileged-provider line HOLDS.
- **"No new knob" is OVERSTATED — THE GATE.** Witness re-sweep/fan-out bound is a NEW
  parameter (reusing FetchAttempts's value is a hidden coupling: chunk-miss degrades one
  file, witness-miss STALLS block validation). Researcher decides if it's a security param
  (sets stall-before-fail window = liveness-vs-DoS). Do not assert "no new sec param."
- **Type gap (Part 2): REAL, STRUCTURAL, BROADER than the bondRegHeight example.**
  `chain.go:1587` `if regH,ok:=c.bondRegHeight[id]; ok &&...` — absent→exempt(admit),
  present-with-value→check interval. BOTH acceptance-relevant. Merged ReadEntry can't
  express: QueryAbsent misses present; QueryPresent needs value UP FRONT but box doesn't
  know regH. Library-confirmed (`pokt smt proofs.go:428` verifyProofWithUpdates hashes
  caller-supplied value into leaf; VerifyProof returns only bool) — membership proof
  CANNOT reveal an unknown value. Also hits bondRootOwner/bonded via restoresHeldStanding
  (`chain.go:3262`).

## The fix (Researcher-gated)
Third QueryKind `QueryPresentOrAbsent`: block carries CLAIMED committed value (proof-
bound — a lying regH=0 needs a membership proof of that value, fails unless true), box
accepts membership XOR non-membership proof, routes predicate by VERIFIED kind. Does NOT
weaken NoWitness-never-ProvenAbsent (ProvenAbsent still only from verified non-membership;
both-fail → NoWitness). Value-carrying leaves exist (`statehash.go:97-112`). ZERO external
consumers of ReadEntry/Resolve at 0984db4 — fix touches only core/statehash + tests, clean.

## Coupling the consult under-weighted
Delivery + R3 are ONE byte budget. R3 `CBlock = len(readSet)·SProofMax` was certified for
a bundle IN HAND. On-demand fetch must cap AGGREGATE PULLED bytes at CBlock before verify,
or any-of-N fan-out is a bytes amplifier reopening the DoS R3 closed. Design named
"over-budget→NoWitness" but not the enforcement point. REQUIRED before fetch path ships
(my engineering call, settled).

## Sequencing
- Buildable now: delivery plumbing (mechanism over certified accessor) + aggregate
  side-channel ceiling (re-applies ratified CBlock).
- Blocked on read-set soundness cert: QueryPresentOrAbsent; the re-sweep-bound-is-sec-param
  question. DO NOT wire bond predicates (chain.go:1587, restoresHeldStanding) until certs.
- Era-3 freeze (#632) NOT touched — its own message excludes C-7/#600 floor-box; witness
  rides OUTSIDE the block. No I1–I5 touched (box READS the root).

Ruling: /Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-witness-delivery-increment3-2026-08-29.md
