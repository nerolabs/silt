# Floor-box recompute increment 4 — the two remaining whole-set reads (qualified, slashed)

Date: 2026-08-31
Author: Builder seat
Status: PACE (design before build). Reproduces ONE quorum-stack predicate
(`qualifiedCount`, the count-quorum leg). NO production consensus/validity rule
change. The box STILL never-Accepts. Two STOP-and-report findings govern the scope
(below): `qualified` needs its own increment (Path-1 recompute), and `slashedRoot`
the digest has no whole-set reader.

## The two reads, precisely classified (grounded in code, not the task's framing)

The enumeration (#664,
`docs/thinking/2026-08-31-Rboundary-mechanical-wholeset-enumeration-options.md`)
split the whole-set reads into a QUORUM-STACK channel (ValidateCommit predicates the
box reproduces like increments 1-3) and an APPLY channel (state-transition writes
whose only validity gate is the Path-1 state-root recompute). The two remaining reads
fall on OPPOSITE sides of that split:

### `qualified` — APPLY channel, NOT quorum-stack. Needs its own increment. STOP.

Every read of the `qualified` map (grep `c.qualified`, chain.go / readset_v5.go /
statehash.go / era3validity.go):

- writes: `c.qualified[id] = w` / `delete` (chain.go:1380-1382) — maintenance;
- the freeze: `set = cloneInt64MapID(c.qualified)` (rotateEpoch, chain.go:3425);
- clone/restore/emit: era3validity.go:195, chain.go:3915, statehash.go:231/264,
  readset_v5.go:660/689.

There is **no quorum-stack reader**. The one whole-set read is the epoch-boundary
freeze `epochSet := clone(qualified)` in `rotateEpoch`. That is an APPLY-channel state
transition. Its validity gate is NOT a ValidateCommit quorum predicate — it is the
committed-root equality `validateEra3Roots` → `postApplyRoots` → `apply()` on a
scratch clone → `StateRootForVersion(5)` (era3validity.go:114-157). The block's
committed `StateRoot` must equal the recomputed post-apply root; the freeze's
correctness rides entirely on that equality.

Reproducing that equality TRUSTLESSLY is the **full Path-1 state-root recompute**:
witness the entire read-set of `apply(b)`, run the real state transition, and
re-derive the whole committed root (all 23 keyspaces + 5 digest roots). That is a
large, separate piece — categorically different from the increments 1-3 pattern,
which reconstructs ONE keyspace's MTH from a witnessed id-list and folds ONE quorum
predicate. The increment-1-3 shape cannot reproduce an apply-channel transition; there
is no single quorum fold to mirror.

**DECISION (per the task's explicit gate): STOP on `qualified`. It warrants its own
increment (the Path-1 recompute), not a fold here.** Its `qualifiedRoot` digest stays
inert. `TestInertDigestRootsAwaitRecompute` continues to skip exactly `qualifiedRoot`.

### `slashed` — quorum-stack, reproduced HERE via `qualifiedCount`.

`qualifiedCount` (chain.go:1479-1487) folds `bonded[id] >= MinBond && !slashed[id]`
over the **whole `bonded` domain**. It is the N the count-quorum is sized against:
`validatorSetSize` (chain.go:1563, the fall-through when NOT anchor-window and NOT
mature-epoch) → `RequiredQuorum` (chain.go:1526-1537, the count floor) → the
`requireQuorumStack` count leg (chain.go:2779). This is a ValidateCommit predicate and
fits the increments 1-3 quorum-stack pattern exactly.

The oracle proves the whole-set channel (readset_v5_quorum_wholeset_test.go:108-138,
`countFloorPoisedWorld`): five equal bonds ⇒ N=5, bftThreshold(5)=3; a coalition of
proposer + 2 attesters is one short (REJECT). Slashing ONE untouched bonded member
drops N to 4 ⇒ bftThreshold(4)=2 ⇒ the same coalition clears (ACCEPT). The flip is
carried solely by `qualifiedCount` reading `slashed[untouched-bonded-member]`.

## The `slashedRoot` finding — the digest has NO whole-set reader. STOP on it.

The task says "remove `slashedRoot`'s `isDigestRootLeaf` exclusion." Mechanically that
is WRONG, and I will not fabricate a read to satisfy it. Here is the evidence:

`qualifiedCount` iterates the **bonded** domain and reads `slashed[id]` PER-MEMBER. Its
set-completeness is anchored on `bondedRoot` (already read by increment 3), not on
`slashedRoot`. For each bonded member the box proves `slashed[id]` present (inclusion)
or absent (non-inclusion) against StateRoot — exactly the per-member slashed proof the
maturity increment already ships (floorbox_recompute_maturity_v5.go:197-210). A member
that is slashed but NOT bonded is invisible to `qualifiedCount` (the loop never reaches
it), so the WHOLE slashed set is never folded.

Grep confirms it: the only whole-`slashed`-set iterations are clone (era3validity:210),
restore (chain.go:3905), and statehash emission (statehash.go:165/265). NONE is a
validity or quorum predicate. Every PREDICATE read of `slashed` is per-member:
`qualifiedCount` (bonded domain), `attesterQualifiedAt` (chain.go:1280, block-named
attesters — per-key), `C2Metric` (validatorsSeen members — increment 2). So the
`slashedRoot` DIGEST has no whole-set reader.

Removing `slashedRoot`'s exclusion and "emitting a `slashedRoot` read" in the producer
would make `TestSlashedRootReadReddensOnDrop`-style ablation IMPOSSIBLE to honor: the
ground-truth driver (execution-derived) never reads `slashedRoot` whole-set, so it
would never redden on drop — a green check with no demonstrable red, the exact
decoration the ablation discipline forbids. The honest, sound reproduction reads
`bondedRoot` (completeness) + per-member `bonded[id]`/`slashed[id]`; it does not read
`slashedRoot`.

**DECISION: reproduce `qualifiedCount` (consuming per-member `slashed[id]` over the
bonded domain), but DO NOT remove `slashedRoot` from the inert set — no predicate reads
the whole slashed set, so `slashedRoot` is a legitimately-inert derived commitment.**
This is a correction to the task's premise, surfaced as tension per the orchestra rule.

## The inert-set end-state (revised)

After this increment: `qualifiedRoot` and `slashedRoot` BOTH remain inert.
`qualifiedRoot` because `qualified` needs the Path-1 recompute (its own increment);
`slashedRoot` because no predicate folds the whole slashed set. So
`TestInertDigestRootsAwaitRecompute` still skips two roots. That contradicts the task's
"NO digest root should remain" end-state — but the task's end-state assumed both reads
were foldable quorum-stack whole-set reads, which the code refutes for both.

## The build (increment 4, `slashed` via `qualifiedCount`)

Same structure as increments 1-3, closest to increment 3 (the whole-bonded fold):

1. **SET-COMPLETENESS over `bonded`.** Reconstruct `nodeSetMTH(witnessedIDs)`; require
   it equals the committed `bondedRoot` leaf (proven present vs StateRoot). Same
   `bondedRoot` read increment 3 uses — reuse `BondedSetWitness` shape. One
   omitted/injected bonded member ⇒ different MTH ⇒ stall.
2. **PER-MEMBER `bonded[id]` (C-1)** — Resolve each bonded weight leaf (the `>= MinBond`
   screen operand). Forged weight fails ⇒ stall.
3. **PER-MEMBER `slashed[id]` (C-1)** — Resolve each slashed leaf present/absent, like
   the maturity increment. A prover cannot drop a slashed member's proof (that would
   inflate N) — every bonded member gets a slashed proof either way.
4. **OWN CONFIG (C-6): MinBond** read from `c.cfg.MinBond`, never the witness. This is
   the eligibility screen; an attacker-controlled MinBond shifts N.
5. **THE FOLD, byte-for-byte `qualifiedCount`:** `N = count(bonded[id] >= MinBond &&
   !slashed[id])`. Then reproduce the count-quorum verdict the box is asked for at this
   state — expose N (and the derived `bftThreshold(N)` count floor) as the recompute's
   result. It does NOT flip WitnessValidateV5 to Accept (STOP boundary).

New file: `core/chain/floorbox_recompute_qualifiedCount_v5.go` +
`_test.go`. It reads `bondedRoot` (already non-inert) — so it adds NO new digest-root
read and requires NO producer change and NO inert-set change.

## Hard ablations (C-5, red-before-green)

- forged per-member `bonded[id]` weight ⇒ REJECT (C-1);
- forged/dropped per-member `slashed[id]` bit ⇒ REJECT (C-1) — both a claimed-absent
  slash that is committed present, and a claimed-present slash that is absent;
- omitted/injected bonded member ⇒ REJECT (set-incompleteness vs bondedRoot);
- config-from-witness: a witness-carried MinBond cannot move N ⇒ REJECT (C-6,
  failing-first).

## Scope / gates

Reproduce `qualifiedCount` (the `slashed`-over-bonded count-quorum leg). No
consensus-rule change; box never-Accepts. `qualified` STOPPED and reported (needs the
Path-1 recompute as its own increment). `slashedRoot` STOPPED and reported (no
whole-set reader). Both inert roots remain inert; the drift test is unchanged. Run the
full gate set; `core/chain` must be exit 0.
