---
name: keystone-leave-one-out-probes
description: How the keystone snapshot-boot oracle's leave-one-out proves a committed field load-bearing; the two-regime trap for bonded/epochSet; the membership-not-weight overclaim scar; the owed weight probe (#603).
metadata:
  type: project
---

The keystone state-root oracle lives in `core/chain/modelcheck_snapshot_equivalence_test.go`.
Its sharp half (`TestLeaveOneOutProvesEachFieldLoadBearing`) ablates each `committedSet`
field from a snapshot and requires a verdict FLIP; a field with no flip is either bloat or an
under-adversarial probe (a finding to route, not a test to relax). Uncovered fields sit in
`probeUncovered` as declared, shrinking debt.

**Why: the era-3 committed-state format freeze is PE-hard-gated on the consensus-weight
probes (`bonded`, `epochSet`) — a field committed into the SMT root that buys no verdict is
permanent commit-bloat on a forever-growing term.**

**How to apply — the two-regime trap (bit me on this task, 2026-08-27):** `bonded` and
`epochSet` are load-bearing in MUTUALLY EXCLUSIVE regimes, so they cannot share one world.
- `epochSet` gates a verdict only in a MATURE epoch (frozen-set membership via
  `effectiveEpochSet`). Build: no anchors, `EpochBlocks>0`,
  `MatureValidators` unset → `Mature()` holds → `everMature` latches → `rotateEpoch` freezes
  `epochSet` at genesis. Flip: a full-set-accepted commit rejects when epochSet is emptied
  (attesters fail frozen-membership → `seen=0` < COUNT floor → `ErrNoQuorum`).
- `bonded` gates a verdict only where qualification reads the LIVE bonded map — a non-epoch
  objective regime. In a mature epoch, qualification reads `epochSet`, NOT `bonded`, so
  `bonded` is shadowed there. Build: anchorless, epochs DISABLED, `MatureValidators` unset
  (everMature latches → no `launchAnchor` crutch → qualification is purely `bonded[id] >=
  MinBond`). In a 4-anchor LAUNCH world (the existing `richHistory`), anchors carry the
  count quorum and shadow any bonded non-anchor — `bonded` will NOT flip there.

The harness is now world-aware: `worldGroup{build, probes}`; the leave-one-out ablates each
field on the world where it flips, and a field is proven once ANY world's probe flips.

**Verdict-path gotcha:** these probes must NOT call `ValidateCommit` — a snapshot-booted
node has no block history, so the `Prev==head` check fails for any real block (same reason
the set-valued probes call `ValidateEntry`/`validateTakedowns` directly). Use
`collectQuorumSigs` → `requireQuorumStack` (the exact qualification+quorum path the field
feeds), which skips the head check.

Ablation-of-the-check (the session-7 rule, applied): the leave-one-out goes RED with
"changed NO verdict" when `quorumVerdict` is made field-blind; each ablated rejection is the
frozen-set/bonded qualification error ("0 qualified, need N"), NOT a panic.

STOP boundaries respected: the flip changes which identities are ADMITTED (qualification),
never how `total`/`support` are summed (the #402 seam, `chain.go:2450-2456`), and never where
`epochSet` freezes (I3, `rotateEpoch`).

**OVERCLAIM SCAR (blind-PE caught it, 2026-08-27 — RULING-keystone-probes-bonded-epochset):**
The flip is carried by MEMBERSHIP (qualification), NOT the ⅔-weight predicate. For BOTH fields
the verified RED is `ErrNoQuorum` (the COUNT floor), because emptying the field disqualifies
the attesters. `requireEpochWeightQuorum` NEVER fires — with `epochSet` empty its `total <= 0`
branch short-circuits to nil (`chain.go:2452`), so if membership were not the discriminator
the block would not flip at all. My first draft named the ⅔-weight quorum as the flip
mechanism (probe name "at ¾ frozen weight", CHANGELOG "frozen ⅔-weight quorum") and even
described a ½-below-⅔ weight construction that never shipped. Lesson: when a probe short-
circuits a predicate on the ablated side, that predicate is NOT what you proved — name the
rule that actually fired. Attribute the RED to the exact error value, not the predicate you
intended to exercise.

**OWED — the WEIGHT-DISCRIMINATOR probe (issue #603, era-3 format-freeze gate):** these two
probes prove MEMBERSHIP is load-bearing, not the committed per-member WEIGHT bytes. A probe
that flips via `requireEpochWeightQuorum` specifically (coalition clears the count floor but
sits below ⅔ of frozen weight → `ErrNoQuorumWeight`) is still owed BEFORE era-3 freezes.
Do NOT freeze era-3 on the weight claim until #603 lands.

Reworded + shipped as PR #604 (merge is Andrew's call).
Deliberation: `docs/thinking/2026-08-27-keystone-probes-bonded-epochset.md`.

**THE LATCH/GATE/DOMAIN TRANCHE (2026-08-28) — six more fields moved out of
`probeUncovered`, each ablation-proven with a named RED. Awaiting blind review before
merge.** `probeUncovered` now holds only `bondRegHeight` + `validatorsSeen`. The
mechanism per field (all in chain.go; the flip reaches the field's SOLE verdict-relevant
read, ValidateCommit-free via `collectQuorumSigs→requireQuorumStack` or `validateBondRegs`):
- `everMature` → de-mature bar (`requireQuorumStack:2471`, `everMature && !matureNow()`
  gates `requireDeMatureSuperQuorum`). World: epochs DISABLED (so the epoch weight quorum
  can't fire), latch matures then live weight concentrates. RED `ErrDeMatureQuorum`.
- `matureEpoch` → frozen-weight quorum (`:2457`). World: epochs ON, frozen unequal weights,
  live `bonded` narrowed to the 2 coalition members (count floor bftThreshold(2)=1). RED
  `ErrNoQuorumWeight`. Dropping it skips the weight rule AND leaves the frozen-set
  qualification branch.
- `gateLockedIn`/`gateHeight` → #506 R-rule (`regGateActive:3030 = gateLockedIn && h>gateHeight`).
  ONE `gateWorld` locks the gate at a boundary; two probes straddle H_act. gateLockedIn RED
  reject→accept (past-gate reg), gateHeight RED accept→reject (below-gate reg, H_act→0).
- `regVersion` → read at EXACTLY ONE verdict site: the `rotateEpoch:3007` lock-in tally. NOT
  in any Validate path. So the probe is `mutates:true`: it APPLIES the boundary block (trips
  the latch → runs the tally). Full (⅔-ready) locks → within-R reg `ErrRegGate`; dropped
  tallies 0 ready → never locks → accept.
- `bondDomain` → **NOT metric-only** (overturns the prior excuse). Feeds `matureNow()` via
  `C2Metric`→`MatureCoefficient` (A-axis NakamotoDomains), and `matureNow()` gates the
  maturity LATCH (`:2893`) → the launch-anchor shed. `mutates:true` probe: `domainWorld`
  merges all bonds into ONE domain → stays immature → anchor-only commit accepted; dropped
  → independent groups → coefficient rises → latch trips → anchors shed → REJECT.

**KEY MECHANISMS worth remembering:**
- A history-less snapshot CAN drive `validateBondRegs`: `recentBondRegNonces(prev)` appends
  `BondRegNonce(prev)` as its FIRST window nonce BEFORE the blockByHash break
  (chain.go:1385-1386), so a reg signed against any chosen `prev` validates. This is the
  ValidateCommit-free entry point for the gate/regVersion probes.
- The de-mature bar can ONLY reject a WEIGHT-CONCENTRATED coalition. With EQUAL weights a
  count-clearing coalition (~bftThreshold ≈ ⅔ by count) is ALWAYS ≥ ⅔ by weight — proven
  empirically. That is why `bondDomain` (which needs equal weights for the domain-merge to
  bite) CANNOT flip via the de-mature bar; its verdict comes through the maturity LATCH →
  anchor-shed instead. (I first mistook bondDomain for a (b) finding via the de-mature bar;
  the latch path is the real one.)
- World builders BAKE the live/frozen divergence with `setField(c, "bonded"/"bondDomain"/
  "validatorsSeen", ...)` AFTER the Append ramp, so `snapshotBoot` deep-copies the exact
  regime a real snapshot would carry — cleaner than driving TTL expiry through Appends.

Ablation discipline done BOTH ways: (1) per-field RED with the named error injected and
watched; (2) end-to-end proof the oracle's "changed NO verdict in any world" guard FIRES
(temporarily broke gateLockedIn's probe → the leave-one-out failed on exactly that field).
Deliberation: `docs/thinking/2026-08-28-keystone-leaveoneout-latch-gate-domain.md`.
