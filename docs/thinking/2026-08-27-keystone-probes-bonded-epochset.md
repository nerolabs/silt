# Keystone leave-one-out — proving `bonded` and `epochSet` load-bearing

Date: 2026-08-27
Author: builder seat
Ships with: the two new probes in `core/chain/modelcheck_snapshot_equivalence_test.go`.

## The goal

The keystone state-root oracle has a sharp half (`TestLeaveOneOutProvesEachFieldLoadBearing`):
for every committed field with a probe, omitting it from the snapshot MUST change a finality
verdict. Today `bonded` and `epochSet` sit in `probeUncovered` — declared debt, unproven
necessity. This work moves each into `probes()` with a probe whose omission flips an
accept/reject verdict via the **membership/qualification** rule (which identities are
admitted as qualified attesters), using the predicates **as written**. The PE hard-gated the
era-3 format freeze on the consensus-weight probes (`bonded`/`epochSet`), so this is on the
critical path — though the WEIGHT half of that gate is owed separately (issue #603, see
owed-work below).

## The mechanism each field is load-bearing for (attribution before code)

Both fields gate mature/objective qualification. The flip each probe proves is carried by
**membership** (which identities are admitted as qualified attesters), NOT by the
⅔-weight predicate `requireEpochWeightQuorum`. This attribution was corrected after the
blind PE review verified the RED
(`../silt-reviews/principle-engineer/RULING-keystone-probes-bonded-epochset-2026-08-27.md`):
in both worlds the verified rejection is `ErrNoQuorum` (the count floor), not
`ErrNoQuorumWeight`. The weight predicate never fires — with the field empty its
`total <= 0` branch short-circuits to nil (`chain.go:2452`).

- **`epochSet`** is the FROZEN MEMBERSHIP set for a mature epoch. In
  `attesterQualifiedAt` a mature-epoch attester qualifies only if `effectiveEpochSet(h)[id]`
  is present. Omit `epochSet` → the snapshot restores an EMPTY map (`snapshotBoot`'s
  faithful omission model) → every attester fails membership → `seen` collapses to 0 →
  the count floor rejects (`ErrNoQuorum`). The full snapshot, with the frozen set present,
  qualifies the members and ACCEPTS. Verdict flip: full=accept, ablated=reject. This proves
  `epochSet` MEMBERSHIP is load-bearing. It does NOT prove the per-member WEIGHT bytes are
  load-bearing — `requireEpochWeightQuorum` is short-circuited on the ablated side and clears
  the threshold comfortably on the full side, so a wrong weight is never the discriminator.
  The weight probe is owed (see owed-work below, issue #603).

- **`bonded`** gates proposer/attester qualification in the **non-epoch objective**
  regime. `proposerQualifiedAt` / `attesterQualifiedAt` fall through to
  `c.bonded[id] >= MinBond || launchAnchor(id)` (`chain.go:1046,1083`). After handoff
  `launchAnchor` returns false (F-1), so a matured non-anchor validator qualifies ONLY by
  `bonded`. But a simpler, launch-regime flip works without a handoff and without touching
  any count/weight sum: a **non-anchor** validator whose committed bond is the thing that
  admits it as a qualified attester. Omit `bonded` → that validator fails
  `attesterQualifiedAt` → it is dropped from `seen` (unqualified sigs are ignored, not
  fatal — `collectQuorumSigs:2363`) → the count quorum `RequiredQuorum` is no longer met →
  `ErrNoQuorum` (reject). Verdict flip: full=accept, ablated=reject.

  This is NOT the #402 seam. The seam is changing how `total`/`support` are SUMMED
  (`chain.go:2450-2456`). This probe changes neither; it changes which identities are
  *admitted* as qualified attesters, which is precisely what `bonded` is for (the
  qualification filter, not the weight arithmetic).

## The block the `epochSet` probe uses — a full-set-accepted commit

The `epochSet` probe needs a block the FULL frozen set ACCEPTS, so that omitting `epochSet`
(→ empty membership → attesters disqualified → `ErrNoQuorum`) is a genuine accept→reject
flip.

Over a frozen set of N=4 equal `twoMiB` bonds, the block is proposer keys[0] + two attesters
(¾ of the frozen weight, well clear of both the count floor and the ⅔-weight bar). The full
snapshot qualifies all three by frozen membership and accepts. The `epochSet`-omitted
snapshot restores an empty frozen set, disqualifies the attesters, and rejects with
`ErrNoQuorum` — the count floor, NOT `ErrNoQuorumWeight`. The flip is membership, not weight.

**Correction (post blind-PE review):** an earlier draft of this deliberation described the
`epochSet` probe as a ½-below-⅔ WEIGHT flip (build the block below the ⅔ bar so the full set
rejects with `ErrNoQuorumWeight` and the ablated set accepts via `total==0`). That is a
different, stronger probe than what shipped and than what the RED verified. As implemented and
verified, the probe flips on membership (`ErrNoQuorum`), not the weight predicate. The
weight-flip probe is real and worth building, but it is owed separately (issue #603), not what
this branch proves.

## The world these probes run against

The leave-one-out harness ablates fields on the SHARED `richHistory` world and hands the
restored replica to each probe. For an `epochSet` ablation to matter, `richHistory` must
produce a non-empty frozen `epochSet` — i.e. it must be a MATURE-EPOCH world. For a `bonded`
ablation to flip a qualification, `richHistory` must contain a non-anchor validator admitted
by its committed bond.

### Options considered

- **(A) Per-probe dedicated mini-worlds** — each new probe builds its own chain internally,
  ignores the passed-in `c`. REJECTED: the harness ablates the field on the shared
  `richHistory` snapshot; a probe that builds its own world never sees the ablation, so
  leave-one-out could not detect the loss. The probe MUST read the field off the handed-in
  `c`.

- **(B) Enrich `richHistory` into a mature-epoch world with a non-anchor bonded validator.**
  CHOSEN. Keeps the harness generic (one world, ablated field-by-field). The mature-epoch
  recipe is the well-trodden `epoch_test.go` path: `Config{EpochBlocks:N}`, no anchors, no
  `MatureValidators` (so `Mature()`==true and `everMature` latches), commit past the first
  boundary to freeze `epochSet`. Cost: the 5 existing launch-world probes must be re-checked
  against the new regime; `richHistory` changes from a 4-anchor launch world to a mature
  epoch world.

- **(C) Set the mature-epoch fields directly via `setField` inside `richHistory`.** REJECTED:
  hand-poking `matureEpoch`/`epochSet`/`everMature` bypasses `apply()`, so the replica would
  not be a faithful replay-booted chain — it would prove the probe against a fiction. Grow
  the state organically through committed blocks, as the rest of the oracle does.

### Risk to the existing probes (B)

The existing probes read `byRoot`, `revoked`, `bondRootOwner`. Those are populated by the
same entry/revoke/bondReg history, regime-independent. Moving to a mature-epoch world keeps
that history; the anchors go away but the probes never depended on anchor-ness (they assert
publish/revoke/bond-ownership rules). Verified by running the full oracle after the change.

## Failing-first discipline (the session-7 nil-map scar)

Each new probe is watched via the harness's per-probe `t.Logf` (`full=X ablated=Y`). A
"divergence" that is really an unrelated panic renders as `full=... ablated=panic`, which is
visible and is NOT the flip claimed. The required evidence for each field:

- `epochSet`: full=accept, ablated=reject (`ErrNoQuorum` — empty frozen membership
  disqualifies the attesters). NOT `panic`, NOT `ErrNoQuorumWeight`.
- `bonded`: full=accept, ablated=reject (`ErrNoQuorum`). NOT `panic`.

Both use `quorumVerdict` (`collectQuorumSigs` + `requireQuorumStack`, a pure verdict path,
no mutation), so both run in the replay-vs-snapshot equivalence half too
(`TestSnapshotBootMatchesReplayBoot`) and are not `mutates`.

## STOP conditions checked

- Not changing how weight is summed (`total`/`support`, `chain.go:2450-2456`) — the #402
  seam. Both probes use the sums as written.
- Not changing where/when `epochSet` freezes (I3). `richHistory` grows the epoch through
  committed blocks; the freeze is `rotateEpoch`, untouched.
- Not deciding `⌈A/2⌉` vs `⌊A/2⌋+1` inclusivity — that is the launch anchor gate, not
  touched here; both probes flip via membership, no ⅔-boundary decision.

## Owed work — the weight-discriminator probe (issue #603, era-3 freeze gate)

These two probes prove `bonded` and `epochSet` MEMBERSHIP is load-bearing in the committed
root. They do NOT prove the committed per-member WEIGHT bytes are load-bearing: the flip is
`ErrNoQuorum` (count floor via lost membership), and `requireEpochWeightQuorum` is never the
discriminator. A leave-one-out that flips via the weight predicate specifically — a coalition
that clears the count floor but sits below ⅔ of frozen weight, so the RED is `ErrNoQuorumWeight`
— is still owed. That probe is the gate on the era-3 format freeze: committing a per-member
weight is justified only if a wrong weight flips finality. Do not freeze era-3 on the weight
claim until issue #603 lands. Until then, the `epochSet` weight-summation role is proven only
by the `requireEpochWeightQuorum` unit tests, not by the snapshot leave-one-out oracle.
