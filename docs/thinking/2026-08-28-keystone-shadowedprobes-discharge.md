# Discharging the three `shadowedProbes` entries — sole-discriminator worlds for the bond-root fields

Date: 2026-08-28
Seat: Builder
Scope: model-check/unit tier only. Zero `chain.go` lines. STOP boundaries honored.
File: `core/chain/modelcheck_snapshot_equivalence_test.go`.

## What this discharges

The neuter meta-guard (`TestNeuteringAnyProbeBreaksCompleteness`, #625) surfaced three
pre-existing shadowed probes, quarantined in `shadowedProbes`:

1. `revoking an unknown root must be rejected` — mis-tagged `byRoot`.
2. `a second identity cannot take an already-owned bond root` — `bondRootOwner`, claimed
   coupled with `bondRootProven`, routed research-gated.
3. `a proven bond-root owner cannot be displaced by a later proven claim` — `bondRootProven`,
   claimed coupled with `bondRootOwner`, routed research-gated.

The blind PE ruling `RULING-bondregheight-probe-neuter-guard-2026-08-28.md` (Q4) OVERRODE the
research-gated routing on (2)/(3): all three are IN-SCOPE FIXTURE work. The "coupling" was a
property of how the two debt probes were built (both ask via `c.bonded[claimant]` in the
`richHistory` world where dropping EITHER field produces the same observable), not a property
of the fields. This doc builds the two sole-discriminator worlds the PE named, and retags the
mis-tagged `byRoot` probe. The list ends EMPTY.

## The field-coupling asymmetry that makes this a fixture, not a research question

`bondRootProven` is read at exactly ONE verdict-relevant site: the displacement predicate
(`chain.go:2845`, `!(proven && !c.bondRootProven[r.Root])`).

`bondRootOwner` is read at TWO: the displacement predicate (`chain.go:2839`) AND
`restoresHeldStanding` (`chain.go:3054`, `c.bondRootOwner[root] != id`), the #506 R-rule
exemption. `bondRootProven` never touches `restoresHeldStanding`.

That asymmetry gives each field its own uncoupled world:

- `bondRootProven` → a proven-vs-proven displacement world where dropping it ALONE flips the
  displacement verdict (`held`→`displaced`). `bondRootOwner` is held constant.
- `bondRootOwner` → a returning-lapsed-member world exercising `restoresHeldStanding`, where
  dropping it ALONE flips a within-R re-registration verdict (accept→`ErrRegGate`).
  `bondRootProven` is not read on this path at all, so it cannot shadow.

Neither probe MOVES a rule. Each OBSERVES an existing predicate on a state where the field is
the only variable. Proving a committed field load-bearing is exhibiting one world where its
omission changes a verdict — that is fixture work, not a design change.

## Probe A — `bondRootProven` sole discriminator (`provenDisplaceWorld`)

Mechanism (`chain.go:2839-2849`, the displacement predicate; `proven := b.Height > 0` at 2813):

- Owner `keys[1]` proves `root` at height 1 → `bondRootOwner[root]=owner`, `bondRootProven[root]=true`.
- The probe applies a PROVEN challenger `keys[2]` registering the SAME `root` at height ≥ 1
  (`proven=true`). Full snapshot: `owner != challenger` (claimed), so the displacement guard
  `!(proven && !bondRootProven[root])` = `!(true && !true)` = `!false` = `true` → `continue` →
  challenger earns nothing. Verdict `proven-owner-held`.
- Ablate `bondRootProven` ALONE, holding `bondRootOwner`: the guard becomes
  `!(true && !false)` = `!true` = `false` → does NOT continue → `delete(c.bonded, owner)` runs
  and `c.bonded[challenger]` is set. Verdict `displaced-the-proven-owner`.

The flip is carried by `bondRootProven` alone. `bondRootOwner` is present on both sides, so it
cannot shadow. This is `mutates` (it `apply()`s a block), so it runs against throwaway replicas
and is skipped by the replay-vs-snapshot equivalence loop, same as the existing displacement
probes.

Why the existing "a proven bond-root owner cannot be displaced" probe was shadowed and this one
is not: the existing probe runs in `richHistory`, where `bondRootProven` was set by the SAME
height-1 reg that set `bondRootOwner`, and it asks via `c.bonded[challenger]`. There, dropping
`bondRootOwner` ALSO admits the challenger (the `owner != id` claimed-check no longer sees an
owner), so the two ablations are observationally identical and the loop's first-flipping probe
(bondRootOwner's) shadows it. The new world seats a proven owner whose OWNERSHIP survives the
`bondRootProven` ablation, so only `bondRootProven` moves the verdict.

## Probe B — `bondRootOwner` sole discriminator (`restoreOwnerWorld`)

Mechanism (`chain.go:1497` R-rule + `chain.go:3050-3065` `restoresHeldStanding`):

The R-rule fires iff `regGateActive(h) && (b.Height-regH < R) && !restoresHeldStanding(id, root)`.
`restoresHeldStanding` returns true iff ALL of:
1. `epochsEnabled() && matureEpoch` — a mature epoch.
2. `bondRootOwner[root] == id` — THE FIELD UNDER TEST.
3. `bonded[id] < MinBond` — a LAPSED member (standing dropped).
4. `id in epochSet` — still seated in the frozen epoch.

World: a mature-epoch chain with the #506 gate armed by `RegGateActivationHeight` (per-world
genesis config, NOT the latch — so `gateLockedIn` is unset and `gateHeight`=0, and dropping
EITHER does nothing to `regGateActive`, `chain.go:3028`). Member `x` freezes into `epochSet`,
re-registers its OWN root so `bondRegHeight[x]` is recent, then its live `bonded[x]` is set
below `MinBond` (a lapse, via `setField`). The probe validates a within-R re-reg for `x` on its
OWN root past the activation boundary.

- Full snapshot: `restoresHeldStanding(x, rootX)` = true (owner match + lapsed + in epoch) →
  R-rule EXEMPTED → the reg falls to `validateBondRegWindow`, which accepts → verdict `accept`.
- Ablate `bondRootOwner` ALONE, holding `bondRootProven`: `bondRootOwner[rootX] != x` →
  `restoresHeldStanding` returns false at chain.go:3054 → the R-rule fires → `ErrRegGate`
  (`re-registered N blocks after its last reg`) → verdict `reject`.

`bondRootProven` is NEVER read on this path, so its ablation cannot flip this world. Dropping
`bondRegHeight` would ALSO flip this world (the R-rule reads it too), but this probe DETECTS
only `bondRootOwner`, so the leave-one-out loop ablates only `bondRootOwner` for it, and
`bondRegHeight` already has its own sole-discriminator world (`bondRegHeightWorld`). The neuter
guard judges sole-catchness by the actual verdict flips of the FIELDS a probe detects, so
declaring `["bondRootOwner"]` keeps this probe's unique credit to `bondRootOwner`.

## Probe C — `revoking an unknown root` retag/removal (the mis-tagged decoration)

Per the PE and the in-file debt note: `validateTakedowns` on a never-published root is rejected
regardless of whether `byRoot` was carried (isolated ablation shows no flip on `byRoot`). Its
`byRoot` detect tag is mis-declared. `byRoot` is independently and soundly covered by the real
`dup-publish must be rejected` probe (same `probes()` list), which asks `ValidateEntry(entry(9))`
— a republish of the height-1 entry, rejected ONLY because `byRoot` carries that root.

FIX: REMOVE the `revoking an unknown root` probe from `probes()`. It is a validity-rule probe
that proves no committed field is load-bearing; keeping it with a corrected (empty) detect tag
would leave a probe the neuter guard flags as covering no field. Deleting it loses NO coverage:
`byRoot` stays covered by `dup-publish`. Verified by the leave-one-out loop still flipping
`byRoot` (via dup-publish) after removal, and by the equivalence loop still exercising
`validateTakedowns` through the `un-revoking` probe (which detects `revoked`).

## STOP boundaries — honored

No `chain.go` line changes. `RegGateActivationHeight` and `EpochBlocks`/`MatureValidators` are
pre-existing per-world genesis config, not new rules. The displacement predicate
(`chain.go:2839-2849`), `restoresHeldStanding` (`chain.go:3050`), the R-rule (`chain.go:1497`),
the weight-sum seam, `rotateEpoch`/epochSet (I3), and the ⌈A/2⌉ threshold are all only OBSERVED,
never modified.

## Discipline — each new probe survives its own neuter

For each new sole-discriminator probe: neuter it (constant ask, keep detect tag) and confirm the
oracle goes RED naming its field with the REAL rejection (displacement verdict / `ErrRegGate`),
not a panic. RED evidence recorded in the report. Then `shadowedProbes` is EMPTY and
`TestNeuteringAnyProbeBreaksCompleteness` is GREEN.
