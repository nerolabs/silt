# Leave-one-out: closing the last two committed fields (bondRegHeight, validatorsSeen)

Date: 2026-08-28
Seat: Builder
Scope: model-check / unit tier. Touches NO consensus rule — the probes drive the
existing validity + qualification predicates; they move no threshold.

## Context

`TestLeaveOneOutProvesEachFieldLoadBearing` (core/chain/modelcheck_snapshot_equivalence_test.go)
proves each committed field is load-bearing: omit it from a snapshot-booted replica
and a real verdict must flip. `probeUncovered` is the declared, shrinking debt of
fields with no probe yet. After #623 it held exactly two: `bondRegHeight` and
`validatorsSeen`. This closes both.

The build-or-report discipline: a probe ships ONLY after its defect was injected and
watched go RED with a real qualification/quorum/gate error — never a nil-map panic,
never a silent skip, never a decoration "all-identical" pass. If a field genuinely
buys no verdict in any non-legacy regime, it is a FINDING to route to the Planner
(a format-design question — what era-3 commits), not a weak probe to close the debt.

## Field 1 — bondRegHeight: cheaply probable now, stale reason

**The failure a snapshot must not have.** A snapshot-booted node that lost
`bondRegHeight` cannot enforce the #506 min-interval R-rule (chain.go:1497): the rule
reads `regH, ok := c.bondRegHeight[id]` and fires only when `ok`. Lose the field and
`ok` is false, so a within-R re-registration (a reg-flood identity, each carrying a
~1.5 MB Answer — the #503 OOM driver) is admitted where the network rejects it.

**Why it is cheap now.** The old `probeUncovered` reason — "this world has no
RegGateActivationHeight, so the rule never fires — needs a gate-active world" — is
STALE. `gateWorld` (merged #623) IS a gate-active world: it latches maturity and locks
the #506 gate at a boundary (`gateLockedIn` true, `gateHeight = 4`), and member x
carries `bondRegHeight[x] = 1` from its height-1 re-registration. `gateProbes` already
builds the within-R re-reg blocks straddling `gateHeight`.

**The probe.** Add a third probe to `gateProbes` on the SAME `past` block (a within-R
re-reg for x at `gateHeight + 1` = height 5) that the `gateLockedIn` probe uses.
- Full snapshot: `bondRegHeight[x] = 1`, gate active at height 5, `5 - 1 = 4 < R = 10`,
  x still holds live standing so `restoresHeldStanding` is false → the R-rule fires →
  **reject (ErrRegGate)**.
- `bondRegHeight`-dropped snapshot: `c.bondRegHeight[x]` returns `ok = false` → the
  R-rule never fires → the reg falls through to `validateBondRegWindow`, which accepts
  (the reg is signed against `BondRegNonce(Hash{})`, the first window nonce a
  history-less replica computes) → **accept**.

The flip is reject→accept, driven by the exact `ErrRegGate` min-interval value. Two
probes now share the `past` block, each detecting its own field via a different
mechanism (gate-armed vs. last-reg-height) — the leave-one-out structure exactly.

`restoresHeldStanding` note: it exempts a LAPSED frozen-set member re-proving its own
root. x is bonded (`bonded[x] >= MinBond`) in gateWorld, so the exemption is false and
the R-rule reaches `bondRegHeight`. Verified against chain.go:3057.

## Field 2 — validatorsSeen: the reason is WRONG; a real objective-regime flip exists

**The standing reason is false.** `probeUncovered` said "read by Mature/C2Metric in
legacy mode only." `validatorsSeen` is read in the OBJECTIVE path too:

- Legacy branch: `matureNow()` at chain.go:1854 enumerates `validatorsSeen` directly.
- OBJECTIVE branch: `matureNow()` at chain.go:1867 calls `MatureCoefficient()` →
  `C2Metric()`, which at chain.go:1978 iterates `for id := range c.validatorsSeen` to
  build the participating bonded set the Nakamoto coefficient is computed over. An
  empty `validatorsSeen` yields `total == 0` → `NakamotoBonds/Operators/Domains == 0`
  → `MatureCoefficient() == 0`.

`matureNow()` gates the maturity latch (chain.go:2893) which gates the launch-anchor
shed (`launchAnchor` returns false once `handedOff()`, chain.go:1019). This is the
SAME verdict path `bondDomain` rides in `domainWorld` — a real, non-legacy verdict.
So this is NOT a legacy-only residual; the (b) finding does NOT apply. Build the probe.

**The world (validatorsSeenWorld).** Epochs DISABLED, four anchors (`AnchorQuorum: 1`),
`MatureValidators: 2`, six equal 2 MiB real bonds each in a DISTINCT declared domain.
Bake the regime a snapshot carries via `setField`: `bonded`, `bondDomain` (six distinct
domains), and `validatorsSeen` (all six seen). everMature stays UNLATCHED at construction
(genesis has anchors, no real bonds seen yet).

With six equal bonds in six distinct domains: total = 12 MiB, threshold = 4 MiB,
NakamotoDomains = NakamotoBonds = 3 (cumulative 6 MiB > 4 MiB at the third),
MatureCoefficient = min(3, 3) = 3 >= MatureValidators = 2 → matureNow TRUE **only when
validatorsSeen enumerates the participants.**

**The probe (mutating, like domainProbe).** It applies a trivial trigger block that
re-evaluates `Mature()`:
- Full snapshot: `validatorsSeen` present → matureNow true → applying the trigger
  latches everMature → sheds the anchors → the anchor-only commit fails qualification
  → **reject (ErrNoQuorum)**.
- `validatorsSeen`-dropped snapshot: empty set → `C2Metric` sees zero participants →
  coefficient 0 → matureNow false → applying the trigger does NOT latch → anchors keep
  eligibility → the anchor-only commit **accepts**.

The flip is reject→accept via the maturity latch → anchor shed → ErrNoQuorum. This
changes which identities are ADMITTED as qualified (the anchors), not how any
total/support is summed — the #402 weight-sum seam is untouched.

## Options considered for validatorsSeen

1. **Objective-regime latch flip (chosen).** A real non-legacy verdict via the exact
   mechanism `bondDomain` already uses. Distinct domains make validatorsSeen the sole
   variable that clears the coefficient. Proven by ablation RED.
2. **Report as a (b) legacy-only finding.** REJECTED — the premise (legacy-only) is
   false on the evidence at chain.go:1978. Reporting it would be inaccurate.
3. **A legacy-mode direct-read probe.** REJECTED — a legacy world is a weaker,
   less-adversarial regime, and the objective flip is available and stronger.

## STOP boundaries (research-gated — NOT crossed)

- No change to the weight-sum seam (chain.go:2450-2456): both flips are qualification
  (which identities are admitted) or validity (is a reg a valid payload), never how
  total/support is summed.
- No change to the epochSet freeze / rotateEpoch (I3): the validatorsSeen world runs
  epochs DISABLED; the bondRegHeight world reuses gateWorld unchanged.
- No change to the `⌈A/2⌉` threshold or #603 (weight-discriminator).
- bondRegHeight flips a VALIDITY verdict (valid reg payload or not). validatorsSeen
  flips a QUALIFICATION verdict (anchor admitted or not). Neither touches I1's arithmetic.

## Deliverable

`probeUncovered` is now EMPTY. Every committed field has a leave-one-out probe with a
demonstrated ablation RED.
