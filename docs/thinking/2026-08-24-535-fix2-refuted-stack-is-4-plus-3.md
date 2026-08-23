# #535 fix (2) refuted by the proof-first model-check — the recovery stack is (4) + (3)

**Date:** 2026-08-24. **Issue:** #535. **Rests on:** the research certification
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/535-epoch-boundary-liveness-cliff-RESEARCH-CERTIFICATION-2026-08-23.md`.

## What the certification asked

The ruling adopted fix (2) — boundary-local quorum re-basing against `old∩next`
— **conditional on a model-checked #402 handoff-intersection proof** for a bled
set, and named the safety it must preserve: *"the boundary block must be final
under both the old and the new set so two competing boundaries cannot each
finalize."* It left "the precise `old∩next` sizing and the handoff proof" as an
open builder/model-check detail, and pre-ruled the fallback: *"until proven, fix
(3) is the guaranteed-safe recovery."*

## The proof, discharged — fix (2) is UNSAFE as an automatic re-basing

`core/chain/modelcheck_535_fix2_rebasing_test.go` discharges the obligation and
finds a counterexample at field-realistic parameters.

The mature super-quorum finalizes on `3·support > 2·total`. Two conflicting
blocks both finalize iff honest weight can split into disjoint H1, H2 with
`Byz+H1` and `Byz+H2` each over the bar — feasible iff **Byz > total/3** (the
frozen set's whole safety premise is `Byz_old < ⅓·T_old`).

Re-basing to `old∩next` shrinks the total to `T−L` (L = lapsed weight). The
adversary chooses which weight lapses: it keeps its own weight in `old∩next` and
exploits **honest** members going offline. So the worst case is *only honest
weight excluded, Byzantine unchanged over a smaller denominator* → the Byzantine
**fraction rises**. At the field numbers:

- Frozen `T_old = 516 MiB`; Byzantine `171 MiB < ⅓·516 = 172` → **safe** for the
  full frozen set (a coalition the quorum is designed to tolerate).
- Lapsed honest `L = 192 MiB` (the 3 maturers). Re-based `T−L = 324 MiB` (the
  field's `324 MiB across 9`). Byzantine `171 > ⅓·324 = 108` → **two conflicting
  boundary blocks can each gather the re-based super-quorum → I1 BREAK.**

This is the **same fault-tolerance wall the certification used to reject fix
(1)**: excluding possibly-honest weight cheapens capture. The safe exclusion
bound is `L < T − 3·Byz`, which at `Byz` near the ⅓ premise limit is `≤ 0` — so
*no* automatic exclusion is safe without knowing which weight is Byzantine
(which is unknowable). **Fix (2) is not safely realizable as an automatic
denominator re-basing.**

## Consequence — the recovery stack is (4) + (3)

- **(4) R-gate restore exemption — SHIPPED (PR #541).** A returning frozen member
  re-bonds to heal, unconditionally. Heals the common case where the departed
  members come back (the observed field case: val-d restarted).
- **(2) boundary-local re-basing — NOT ADOPTED.** The proof obligation the
  certification required is discharged NEGATIVE; the model-check stands as the
  permanent evidence and regression (the shipped no-re-basing behavior — the
  boundary STALLS rather than forks — is the safe one, and the test asserts it).
- **(3) weak-subjectivity liveness-floor escape — THE recovery for genuine
  >⅓-honest loss.** Exactly the certification's fallback: when live-qualified
  weight sits below the frozen bar for K consecutive boundaries, an
  operator-visible degraded mode re-snapshots against live weight **with an
  explicit weak-subjectivity / social-recovery caveat** (the same class of
  action `WSCheckpoint` already requires for long-range recovery). Safe where
  (2) is not, because a **human confirms the >⅓-honest-loss event is real and
  accepts the weak-subjectivity trust assumption** — the protocol never silently
  weakens its own quorum. This is the honest ceiling every weakly-subjective
  chain lives with: a genuine >⅓-of-live honest loss is *outside* the BFT
  liveness model; recovery is bounded and explicit, never automatic.

## Why this needs no re-consult

The certification pre-ruled this branch ("until proven, fix (3) is the
guaranteed-safe recovery"). The model-check is the "certify (2) only once the
proof is model-checked" gate returning NEGATIVE — a builder/model-check detail
resolved within the certified invariants, not a new consensus question. Fix (3)
is the next build; it is a `WSCheckpoint`-class operator-signaled re-snapshot,
not an automatic quorum change, so it does not carry fix (2)'s intersection
subtlety. Its deterministic home is the #535 model-check extended with the
recovery schedule (a re-snapshot triggered by an explicit operator directive
commits; a silent automatic one never does).

## Status

- Model-check regression shipped (`modelcheck_535_fix2_rebasing_test.go`), GREEN
  (asserts shipped-safe, documents fix (2) unsafe with the counterexample).
- Recovery stack updated to (4)+(3); fix (2) closed as not-safely-realizable.
- **Next build: fix (3)** — the operator-signaled WS liveness-floor escape.
