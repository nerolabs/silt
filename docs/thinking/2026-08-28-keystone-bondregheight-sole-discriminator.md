# Keystone: the shared-block shadowing defect and the neuter meta-guard

Date: 2026-08-28
Seat: Builder
Scope: model-check / unit tier. Touches NO consensus rule — the probes drive the
existing validity + gate predicates; they move no threshold. `RegGateActivationHeight`
is per-world genesis config on a throwaway chain (it selects the regime, exactly as
the certified pre-latch trusted-fleet deployment mode does, chain.go:201); the #506
R-rule itself is untouched.

## The defect a blind review found

`bondRegHeightProbe` in `TestLeaveOneOutProvesEachFieldLoadBearing`
(core/chain/modelcheck_snapshot_equivalence_test.go) was DECORATION in the running
oracle. It shared its `past` block — a within-R re-registration past `gateHeight` — with
`lockedInProbe`, both bound to the `gate-lock` world built by `gateWorld`.

In that world the #506 gate is armed by `gateLockedIn` (the chain-derived maturity
lock-in path, chain.go:3030). The `past` block therefore flips for BOTH fields:

- omit `gateLockedIn` → `regGateActive` false → R-rule never fires → accept
- omit `bondRegHeight` → `regH, ok := c.bondRegHeight[id]` returns `ok=false` → R-rule
  never fires → accept

The leave-one-out loop breaks on the FIRST probe that flips. `lockedInProbe` runs first
and catches the `bondRegHeight` ablation (same block, same code path), so
`bondRegHeightProbe` never runs in the loop.

**Proof of the decoration (pre-fix):** neuter `bondRegHeightProbe` (force its `ask` to a
constant, keep its `detect` tag) and `TestLeaveOneOutProvesEachFieldLoadBearing` STAYS
GREEN — `bondRegHeight` still flips via the shadowing sibling. A probe that proves
nothing. The FIELD `bondRegHeight` is genuinely load-bearing (a snapshot that loses it
cannot enforce the #506 min-interval R-rule and admits a reg-flood identity, each Answer
~1.5 MB — the #503 OOM driver); the PROBE just did not prove it in the running oracle.

This is the shared-block SHADOWING class. It is the 2nd/3rd instance: a prior
`bondRootProven` aliasing bug is documented near `..._test.go:90-102`, where mutating
probes sharing `bonded`/`owner` maps each displaced the other's ablation.

## Part 1 — the sole-discriminator world (the fix)

Give `bondRegHeight` its own world, `bondRegHeightWorld`, where the gate is armed by
`cfg.RegGateActivationHeight > 0` (chain.go:3027) instead of by `gateLockedIn`. In that
world:

- `gateLockedIn` is UNSET and `gateHeight` is 0 — ablating either does nothing, because
  `regGateActive` takes the `RegGateActivationHeight` branch (chain.go:3028).
- `bondRegHeight[x]=1` is recorded by x's height-1 re-reg. The probed block is a within-R
  re-reg past the activation boundary (height 3 > `RegGateActivationHeight`=2; 3−1=2 < R=10).
- Full snapshot → `bondRegHeight[x]=1`, epochs disabled so `restoresHeldStanding` is false
  → the R-rule fires → `ErrRegGate` (reject). Drop `bondRegHeight` → `ok=false` → the rule
  never fires → the reg falls to `validateBondRegWindow`, which accepts.

So `bondRegHeight` is the SOLE committed-field discriminator in this world. Dropping it
flips the verdict and NO other probe catches that flip first.

**RED evidence (post-fix):** neutering `bondRegHeightProbe` now turns
`TestLeaveOneOutProvesEachFieldLoadBearing` RED:
`omitting committed field "bondRegHeight" changed NO verdict in any world`. Its RED is the
real #506 rejection: `validator … re-registered 2 blocks after its last reg (R=10)`
(`ErrRegGate`), not a panic.

`gateProbes` now returns only `(lockedInProbe, heightProbe)`; `bondRegHeightProbe` and
`bondRegHeightWorld` are their own pair, wired into both the equivalence oracle and the
leave-one-out oracle under a new `bondreg-height` world.

## Part 2 — the neuter meta-guard (the durable fix)

`TestNeuteringAnyProbeBreaksCompleteness` makes the shadowing class impossible to
reintroduce silently. For EACH probe in the leave-one-out oracle it neuters that one probe
(forces its `ask` constant, keeps its `detect` tag) and re-runs the ablation. If the
completeness guard would STILL pass — every covered field still flips in some world — that
probe is the SOLE catcher of NO field: shadowed decoration, and the guard FAILS naming it.

**The precise rule the guard enforces: every probe uniquely flips at least one field.**
The guard judges sole-catchness by ACTUAL VERDICT FLIPS (`leaveOneOutFlipped`), NOT by
`detect` tags — a probe cannot claim via its tag a field it never flips. This is stricter
than "no decoration": a probe that redundantly covers a field is permitted ONLY if it also
uniquely flips some OTHER field; a probe that uniquely flips nothing is flagged even if its
tag names a covered field. That conservative bias is exactly what a completeness-axis
freeze gate wants — it forbids any probe deletable without losing field coverage.

Non-destructive by construction: `buildLeaveOneOutWorlds` is a factory, so each neuter
targets a fresh probe copy; mutation cannot leak into the next iteration or the real oracle.
The oracle's ablation logic is extracted into `leaveOneOutFlipped` so the guard and the
oracle run the SAME code path.

**RED-then-GREEN evidence.** Reintroducing a `bondRegHeight` probe into the `gate-lock`
world (sharing `lockedInProbe`'s block, the exact pre-fix wiring) is detected as SHADOWED —
the guard REDs on it (undeclared → `t.Errorf`). Post-fix, with `bondRegHeight` in its own
world, the guard is GREEN and `bondRegHeightProbe` is a sole discriminator.

### What running the guard honestly surfaced — a finding, not a suppression

Running the guard for the first time flagged three PRE-EXISTING shadowed probes this fix
did not create. Rather than weaken the guard to hide them, they are quarantined in a
declared, shrinking debt list `shadowedProbes` (the `probeUncovered` discipline applied to
probes). The list may only shrink; removing a probe from it without making it a sole
discriminator RED-flags the guard, and a probe that becomes a sole discriminator but stays
listed also RED-flags it.

1. **`revoking an unknown root must be rejected` (detect: `byRoot`).** `byRoot` is soundly
   caught by `dup-publish`. This probe's verdict does not depend on the carried `byRoot`
   set — revoking a never-published root rejects whether or not `byRoot` is present
   (isolated ablation shows no flip). Its detect tag is mis-declared. FIX (in scope, low
   risk, deferred to keep this change reviewable): retag / move it out of the field set —
   it is a validity-rule probe, not a snapshot-field probe.

2. **`a second identity cannot take an already-owned bond root` (detect: `bondRootOwner`)**
   and **`a proven bond-root owner cannot be displaced …` (detect: `bondRootProven`).**
   The blind PE review (`RULING-bondregheight-probe-neuter-guard-2026-08-28.md`)
   OVERTURNED this consult's initial research-gated routing. It is **IN-SCOPE FIXTURE
   work, not research-gated** — an owed fixture, not a Researcher consult.

   The coupling I saw is a property of THESE PROBES, not of the fields. Both current
   probes ask via `c.bonded[claimant]` in a world where dropping EITHER field produces
   the same observable, so they cannot tell the fields apart. The fields themselves are
   NOT symmetrically coupled: `bondRootProven` feeds exactly ONE predicate (displacement,
   chain.go:2845), while `bondRootOwner` feeds TWO (displacement AND the R-rule exemption
   `restoresHeldStanding`, chain.go:3054). A fixture that drops `bondRootProven` alone
   (holding `bondRootOwner`) flips the displacement verdict `held`→`displaced` via the
   EXISTING rule (chain.go:2845 as written): `proven && !bondRootProven` goes
   `true && !true = false` (no displacement) to `true && !false = true` (displaces). That
   moves no threshold and re-decides no ownership semantics — it only OBSERVES the rule
   that already exists.

   Research-gating would only apply to CHANGING the rule (merging the two fields, or
   discounting ownership so proof no longer beats declaration). Proving the current
   fields individually load-bearing is fixture work. OWED: build two probes — (a) a
   proven-vs-proven displacement world isolating `bondRootProven`; (b) a `restoresHeldStanding`
   exemption (or owner-observing) world isolating `bondRootOwner`. Reclassify from
   `research-gated` to `owed fixture`; not fixed blind in THIS commit only to keep it
   reviewable.

## Tension surfaced to the Planner

The task scoped "every probe must be the SOLE catcher for at least one field." Taken
literally that also condemns legitimate redundant coverage (two honest probes exercising
different rules that both read one field). The guard as shipped distinguishes the real
defect — a probe that is the sole catcher of NOTHING (shadowed) — from redundant coverage
(a field still covered by an independent probe). The pre-existing shadowing it found is a
genuine finding routed above, not scope this change silently absorbs.

## STOP boundaries honored

No probe or world moves a consensus rule. The weight-sum seam (chain.go:2450-2456), the
epochSet freeze / `rotateEpoch` (I3), and the ⌈A/2⌉ threshold are untouched.
`RegGateActivationHeight` is per-world genesis config selecting the regime; the R-rule is
unchanged. The bond-root ownership split is OWED IN-SCOPE FIXTURE (per the PE ruling), not
built in this commit — a later fixture, not a Researcher consult.
