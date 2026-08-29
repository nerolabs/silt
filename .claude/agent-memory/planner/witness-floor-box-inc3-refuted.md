---
name: witness-floor-box-inc3-refuted
description: Inc-3 read-set REFUTED (sound v4 read-set = O(registry)). Boundary crux RESOLVED (no bounded witness). ★ Andrew RATIFIED Option B — trustless via a NEW witnessable era (era-4).
metadata:
  type: project
---

# Inc-3 read-set REFUTED → the O(registry) boundary crux → ★ OPTION B RATIFIED (2026-08-29)

Certs/rulings (full paths under `/Users/andrewedmond/Claude/claude/silt-reviews/`):
- Research completeness (REFUTED): `research/research-outcome/witness-floor-box-readset-completeness-RESEARCH-CERTIFICATION-2026-08-29.md`
- Research boundary-cost: `research/research-outcome/witness-floor-box-boundary-block-cost-RESEARCH-2026-08-29.md`
- PE delivery+type-gap: `principle-engineer/RULING-witness-delivery-increment3-2026-08-29.md`
- PE boundary-cost: `principle-engineer/RULING-boundary-block-witness-cost-2026-08-29.md`

## What's settled (both seats, independent + blind)
- **Sound v4 read-set = the FULL `apply()` committed touch-set** (a v4 block's acceptance recomputes the root via
  `postApplyRoots`→real `apply()` on a clone, `era3validity.go:88-142`). Transition-validity subset is NOT a sound closure.
- **The boundary read-set is O(registry), and NO bounded witness mechanism exists.** TTL sweep (`chain.go:3005-3013`, whole
  `bondRegHeight` map, EVERY block when `BondTTLBlocks>0`) + `rotateEpoch`/`liveQualifiedSet` (`:3124`/`:1198`, whole `bonded`
  map each `h%EpochBlocks==0`). Structural: a diff proves what CHANGED not what was NOT deleted; a range proof over "nothing
  expired" = whole keyspace. ~100k ids ⇒ ~4.8 GiB witness > 2 GB box. A POSTURE decision, not a mechanism.

## ★★★ DECISION — Andrew RATIFIED **OPTION B** (2026-08-29): trustless via a NEW witnessable era
Option A (accept the >⅔-final root under O(payload) self-checks — both seats' recommendation, my recommendation) was
OVERRIDDEN. Andrew defends the FULLY-TRUSTLESS floor box over the faster ship: the floor box must INDEPENDENTLY verify weight/
epoch state, not trust the quorum root. **Consequence (accepted):** the whole-map transitions (TTL sweep, `rotateEpoch`) must be
REDESIGNED to be witnessable — an expiry INDEX / incremental qualified-set COMMITMENT that makes them O(payload). That changes
what `apply()` does + what is committed → a NEW ERA (era-4); era-3 is FROZEN (amendable only by a new `BlockVersion`). The floor
box does NOT ship as a trustless full validator until era-4 lands — a real timeline cost Andrew accepted to keep it trustless.

## Roadmap under Option B
- **NEW TRACK (large, new-era-sized): era-4 witnessable transitions.** Design the expiry index + incremental qualified-set
  commitment so TTL-expiry and epoch rotation become O(payload)/witnessable. This is a CONSENSUS-MECHANISM + FORMAT redesign →
  PACE → PE → Research-certify → human-ratify → FREEZE, the same gated cycle era-3 ran. Multi-session.
- **FOUNDATIONS STAND (format-agnostic witness machinery):** inc1 R4 accessor (#633) + inc2 R3 bound (#634) remain valid — R3's
  `C_block=len(read-set)·16 KiB` HOLDS once the read-set is O(payload) under era-4. The any-of-N delivery (SOUND, = existing
  `fetchFrom`) + the aggregate-CBlock cap + the type-gap 3rd `QueryKind` are all still needed and reusable.
- **Inc-3-as-scoped (era-3 delivery/accept-under-finality) is SHELVED** — Option A path not taken.

## Open items to fold into the era-4 design
- Type gap (PE, CONFIRMED real): merged `ReadEntry` can't model bond-family `map[k],ok` reads; add a 3rd `QueryKind`
  present-or-absent (block carries proof-bound claimed value). core/statehash only; cert-gated.
- Delivery: any-of-N + MUST cap AGGREGATE PULLED bytes at CBlock before verify (PE gate); the `FetchAttempts`-reuse-is-a-new-
  liveness-bound question (research).
- `#535` `epochStart`/`effectiveEpochSet` observable gap (no committed root) — must be handled in era-4 too (a floor box that
  finalizes a `LivenessRecoveryHeight` boundary needs these witnessable or explicitly posture-bounded).

## OPS — stale local main struck 2× (PACE Builder + PE both misread local `main`=2003439). Ground truth: origin/main=`0984db4`,
#633+#634 MERGED (Tester `gh pr view`). BAKE `git fetch origin` + read-at-named-commit into EVERY seat prompt.

## State
Inc1 #633 + inc2 #634 MERGED (origin/main `0984db4`, VERIFIED). Option B RATIFIED. NEXT: open the era-4 witnessable-transition
design (PACE) — proposed as a fresh scoped goal; check start-now vs bank-session with Andrew.
Related: [[witness-floor-box-track]], [[session-resume]], [[vision-drift-honesty-pass]], [[keystone-era3-freeze-sequencing]].
</content>
