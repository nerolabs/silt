# #549 Q3 — round-duration base tune: assessment and decision

**Date:** 2026-08-24. **Issue:** #549 (companion Q3). **Gate:** research-certified
direction (`silt-reviews/research/research-outcome/549-h68-view-synchronization-stall-RESEARCH-CERTIFICATION-2026-08-24.md`,
Q3). **Decision: NO numeric change — the base is already the certified-optimal
smallest value; make the derivation explicit and guard it.**

## The certification's Q3 direction (verbatim intent)

> "A parameter tune is a COMPLEMENT, not the fix. Even with the Q2 fix, the
> base/increment must let `duration(r)` outrun the 3-region + mass-restart skew
> within a few rounds… pick the SMALLEST base that reliably outruns the measured
> cross-region skew, not larger… do not tune the protocol to satisfy a harness
> deadline."

Plus the M1 note: on the 1 vCPU / 2 GB box, longer durations trade slower
recovery for fewer round-change broadcasts — so the base must be small, but not
so small it fails to outrun the skew.

## The measurement: what the cross-region skew actually is

The round-duration ladder must outrun the spread in *when honest members enter a
common round*. That spread has two sources:

1. **Sweep-timer phase skew (dominant).** Each node's `chainSyncTick` fires every
   `ChainSyncInterval` (= **30 s**, `core/node/node.go`), at an arbitrary phase
   set by when it (re)started. Two nodes' phases differ by **< ChainSyncInterval
   = 30 s** — this is a STRUCTURAL bound, not an empirical guess: a node sweeps at
   least once per interval, so no two nodes' timeout triggers can be ≥ 30 s apart.
   A mass restart (the #549 trigger) re-randomizes the phases but cannot exceed
   this bound.
2. **WAN delivery delay (negligible).** The netem cert profile is `delay 80ms
   20ms` (≈ 80 ms one-way, ~160 ms RTT across 3 regions). With the Q2 fix
   (catch-up-to-highest at message speed), a laggard jumps to the leader's round
   on *receiving* its round-change, so the catch-up entry spread is ~WAN latency
   (sub-second), not the timer phase. WAN latency is < 0.3 s ≪ 30 s.

**So the cross-region skew a round must outrun is ≈ ChainSyncInterval = 30 s**
(timer-phase bounded), with sub-second WAN on top.

## The base, derived

`base round duration = roundAdvanceSweeps × ChainSyncInterval`.

- `roundAdvanceSweeps = 1` → 30 s = **exactly the skew**. No overlap margin: a
  worst-case-30 s-late member enters the round the instant the earliest member
  leaves it — the round can hold zero simultaneous quorum. **Unreliable.**
- `roundAdvanceSweeps = 2` → 60 s = **2× the skew**. Even a worst-case-30 s-late
  member has 30 s of overlap inside the round — enough to assemble a quorum.
  **The smallest integer base that reliably outruns the skew.**
- `roundAdvanceSweeps = 3` → 90 s = 3× the skew. Reliable but **larger than
  necessary** — the certification's explicit "not larger", and slower recovery +
  more churn on the 2 GB box.

**Even the FIRST round (r0, duration = base) outruns the skew**, so the ladder
outruns skew "within a few rounds" — in fact from round 0 — satisfying the
"quickly" concern without any increment change either. The increasing increment
`r·(r+1)/2` only matters if the network climbs to high rounds; with the Q2 fix
collapsing the smear at message speed, it rarely does.

## Why the field smeared at r1 despite an adequate base

The h68 field stall was NOT the base being too small (60 s already > 30 s skew).
It was the Q2 catch-up-TARGET defect (jump to the smallest round of the union),
which pinned the effective round low so the ladder never *climbed* — the duration
formula never got to produce a value, adequate or not. Fixing the target (PR #551)
lets the base do its job. So Q3 confirms: no base change is needed once Q2 lands.

## Options considered

| Option | Base | Duration | Verdict |
|---|---|---|---|
| Lower | 1 | 30 s | REJECTED — equals the skew, zero overlap margin (unreliable) |
| **Keep** | **2** | **60 s** | **CHOSEN — smallest base that reliably outruns the 30 s skew** |
| Raise | 3 | 90 s | REJECTED — larger than necessary; slower recovery + more churn (against the M1 "not larger" guidance) |

## Decision

**No numeric change to `roundAdvanceSweeps` or the increment.** The current base
is the certified-optimal smallest value. The Q3 deliverable is therefore to
*harden* the parameter, not change it:

1. Make the derivation explicit in the `roundAdvanceSweeps` comment — from "≈ 60 s"
   to "= the smallest multiple of ChainSyncInterval that outruns the
   ChainSyncInterval-bounded cross-region timer skew" (build-immutable #5: derived,
   not a magic constant).
2. Add a derivation guard test: `roundAdvanceSweeps × ChainSyncInterval` must
   strictly exceed the max sweep-timer skew (ChainSyncInterval), i.e. base ≥ 2.
   A future lowering to 1 fails it with the skew-margin rationale, so the
   parameter can never silently regress below the skew.

This closes Q3 honestly: the certification asked to *consider* raising the base;
the measurement says the base is already minimal-and-sufficient, and raising it
would violate the "not larger" guidance. The value is the recorded derivation +
the guard, so the parameter is no longer an unexplained constant.

## Residual (owned)

The skew bound rests on `ChainSyncInterval` being the sweep cadence on every
seat. If a future change decouples the round-change timer from the chain-sync
tick, or makes `ChainSyncInterval` per-node-variable, the "< ChainSyncInterval
skew" bound must be re-derived. The guard test pins the current relationship;
that re-derivation is where it would fire.
