# `tagRevLogSize` — freeze-manifest item 1 (2026-09-11)

Deliberation for the era-4/v5 committed leaf that authenticates the revocation log's size.
Ratified 2026-09-07 (freeze manifest §8, owner call 9). Certification:
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md`
§4.1. Build-and-hold: a FORMAT item, so it does not open a PR.

## The certified clause, quoted

> **C-a ALWAYS-EMIT.** Every v5 block emits the leaf, including when the log is empty
> (`EncodeUint64(0)`). No absent-vs-empty shortcut — otherwise a box reading absence cannot
> distinguish "empty log" from "no witness", which is the R4 accessor defect the digest roots
> already pay C-4 to avoid.
>
> **C-b POST-APPLY POSITION.** The value is the log size **after this block's own appends**,
> committed in this block's own `StateRoot`. The box validating H Resolves it against
> `prevStateRoot` = H−1's `StateRoot`, which is exactly the `m` the consistency proof needs.
> Getting this off by one silently mis-sizes every proof.
>
> **C-c THE ABLATION, with the right positive control.** A box driven with an omitted or forged
> `tagRevLogSize` witness must STALL (`NoWitness ⇒ stall`), never fall through to a default `m`.
> RED-first, and **the positive control is the degenerate `m = 1` right-spine extension derived
> above** — not a generic "wrong number". A gate that only tests `m = m_true + 1` proves nothing
> about the branch that actually breaks.

## The mechanism, attributed

The failure is a **wrong-accept on a revocation-bearing block's `LogRoot`** because the floor box
verifies the new log root with `translog.VerifyConsistency(parentRoot, m, b.LogRoot, m+k, pi)`, and
`m` is supplied by the witness. `VerifyConsistency` degenerates at the attacker's chosen `m`: at
`m == 0` it returns true without reading either root, and at `m == 1` the `isPow2` seeding leaves
the accumulator `fr` equal to `oldRoot` for the whole loop, so the only surviving constraint is
that `newRoot` folds up from `oldRoot` with attacker-chosen siblings — and `newRoot` is `b.LogRoot`,
which the attacker also chooses. `TestGD9_WitnessSuppliedLogSizeIsUnsound_Control`
(`core/chain/redteam_revlog_size_control_v5_test.go`) drives that forgery today and asserts it
PASSES. This change addresses it by committing the log size as a v5-only SMT leaf, so `m` resolves
against the parent's committed `StateRoot` instead of arriving from the witness.

`m` cannot ride the box-owned `HeadRef` like every other non-leaf fact: it is a field of no block
and a value of no committed root, and it is not recoverable from committed state
(`apply` deletes from `revoked` on an un-revocation, `chain.go` `apply`, so `|revoked| != len(revLog)`;
`revLog` is deliberately outside the SMT). Certification §4.1 re-derives this; verified at source.

## Options weighed

| # | Option | Cost | Verdict |
|---|---|---|---|
| 1 | Decline the leaf; keep the k >= 1 stall | Zero format cost. Every floor box dies permanently at the first takedown after its pin — terminal, not transient. The box is the entire reason era-4 exists. | REJECTED by the cert and by owner call 9 |
| 2 | Seed `m` from the `-ws-checkpoint` like `parentStateRoot` | Zero format cost | REJECTED at source: a wrong `parentStateRoot` makes the box STALL; a wrong `m` makes it ACCEPT. The asymmetry is the whole argument |
| 3 | Commit `m` as a v5-only scalar leaf (CERTIFIED) | One `uint64`, v5-only, additive, beside five digest roots that already precede it | **BUILT** |

## Shape decisions taken here (not in the cert)

**A fourth tag list, `stateRootDerivedTagsV5`.** `revLogSize` is not a `committedSet` field name, so
it cannot go in `stateRootTagsV5` — that list is pinned by reflection to the live field
classification and an unclassified entry reddens `TestStateRootCoversExactlyTheCommittedSetFields`.
It is not a whole-set membership MTH either, so `stateRootDigestTagsV5` would be a semantic lie:
`readset_v5_drift_test.go` derives its inert/read digest partition from that list and
`provenView.members` resolves entries in it as id-list digests. A one-entry fourth list is the
honest home. `revLog` is classified `committedLog`, so the leaf is a DERIVED scalar over a
committed log — its own class.

**Where the value comes from.** `stateRootLeavesV5` reads `len(c.revLog)`. `postApplyRoots` clones,
applies the block, then calls `StateRootForVersion(b.Version)` — so C-b holds by construction, at
the same site that already gives `qualified`/`dueBucket`/`epochStart` their post-apply values. The
gate drives it rather than asserting it: an off-by-one fails the honest consistency proof.

**Two commits, not one.** Commit 1 is the FORMAT surface (the leaf) plus the one box-side change
its own correctness requires: the class-R fold op, without which the leaf-diff completeness guard
reds on any revocation-bearing block driven at the recompute tier. Commit 2 is the CONSUMPTION —
the `WitnessSource` log-extension channel and the P13b `k >= 1` lift. The split exists because the
owner reviews format items individually (`D-RC-POSTURE-2026-09-09` (6)); the freeze surface is
exactly commit 1's `statehash.go` hunk, and nothing in commit 2 is format.

## Why no live-history block hash moves

The change adds NO `Block` field and NO cbor key: `Block.Hash()` and the block encoding are
untouched. The leaf is emitted only by `stateRootLeavesV5`, which `StateRootForVersion` selects
only for `version >= BlockVersionWitnessable` (5). Every v2/v3/v4 root goes through
`stateRootLeaves()`, which this change does not touch. Production mints `BlockVersion =
BlockVersionRounds` (2), so no v5 block exists on any live chain and no v5 root moves either.
Driven, not asserted: `TestRevLogSizeLeafDoesNotMoveTheEra3Root` varies the log size across two
chains and requires the v4 root to be byte-identical, and `TestRevLogSizeLeafBindsTheV5Root`
requires the v5 root to DIFFER over the same pair (the non-vacuity twin — without it the first gate
is satisfied by a leaf that was never added).

## Residual, recorded not fixed

`translog.VerifyInclusion` / `VerifyConsistency` and the `Revocation*Proof` / `RevocationLogSize` /
`RevocationLogRootAt` accessors have zero non-test callers, and `NewBox` has no production caller
at all. Committing the size does not by itself put an inclusion-proof server on the wire. That is
the witness-server surface, out of scope here (`R-WITNESS-RESIDENT-HEAP`, Boulder 1).

## The ablations, run — and one thing they corrected

Recorded here because two of them changed what the code says.

| # | Ablation | Expected | Observed |
|---|---|---|---|
| 1 | Emit the leaf from `stateRootLeaves` (the era-3 set) too | the frozen-format gate reds | RED: *"the era-3 root moved with the revocation-log size"*, plus a duplicate-key error on the v5 path |
| 2 | `if false` over the `countDelta` derivation in the op builder | the fold commits size 0 and stalls | RED on `ErrRecomputeStateRootMismatch` — a stall, never a wrong-accept |
| 3 | Take the source's size WITHOUT Resolving it (m witness-supplied, as before the leaf) | the `m = 1` forgery becomes a wrong-accept | RED: the forged `LogRoot` passes P13b and reaches `ErrRecomputeGated` |

Each patch was `diff`-verified against the original before the run and restored byte-identical
after (`scar:ablation-noop-guard` — a patch that silently fails to apply reports green and is
indistinguishable from a passing ablation).

**Ablation 3 corrected a claim this branch first wrote.** The certification names two degeneracies,
`m == 0` and `m == 1`, and the obvious reading is that either one breaks the construction. Measured:
with the Resolve removed, the `m = 1` forgery passes and **the `m = 0` claim still fails**. At
`n = k` the per-leaf inclusion legs pin the tree to `MTH(leaves)`, and the block's real `LogRoot` is
not that. So the two legs of `verifyLogExtension` are not belt-and-braces — the consistency leg
binds the PREFIX and the inclusion legs bind the CONTENT, and `m = 0` discards only the first. The
gate's comment says that now instead of over-reading the certification.

`m = 0` is still refused at authentication, and the arm is kept, because `m = 0` is what a box would
compute if it read an ABSENT leaf as "empty log". That is exactly what C-a always-emit exists to
foreclose, and why `logExtends` treats `ProvenAbsent` as a stall rather than a zero.

## One finding the lift surfaced, diagnosed by evidence

Lifting the P13b stall made the cold auditor's `unrevocations` class reach the fold for the first
time, where it failed: *"delete revoked\x00||… : key already empty"*. The first hypothesis was that
the new counter write caused it. **Suppressing the counter write entirely reproduced the identical
failure**, which ruled that out. The real cause is `structWitnessFor` serving no `DeleteSiblings`
for a delete — a fixture gap the stall had masked since the class never reached the fold. It is the
coverage `R-LOGROOT-FORMAT-SCOPE` recorded as *"owed when the leaf lands"*, and the fix is the
witness a real server would serve (`ProveWithSiblings`, which the sibling helper `witnessForBlock`
has used all along).
