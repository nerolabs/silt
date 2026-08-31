# PACE — P1-b: extend the O(payload) state-root recompute to classes S (slashes) + A (atts)

Date: 2026-08-31
Author: Builder seat
Status: STOP-and-consult. Design before build. NO production consensus/validity rule change.
The box STILL never-Accepts. This increment is BLOCKED on a soundness/scope conflict I surface
below with evidence rather than resolve by building.

Certs/rulings this builds on (full paths):
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-recompute-P1a-Opayload-multileaf-RESEARCH-CERTIFICATION-2026-08-31.md`
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-recompute-P1a-Opayload-multileaf-VERIFICATION-ADDENDUM-2026-08-31.md`
- `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-v5-Rboundary-Rscope-RECONCILIATION-2026-08-31.md`
- `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-recompute-P1a-Opayload-multileaf-2026-08-31.md`

## The task, restated

Extend `RecomputeStateRootEntriesRevocations` (the E/R O(payload) hybrid) to reproduce
`validateEra3Roots`' StateRoot equality for blocks that also carry classes S (slashes) and A
(attestation tracking), reusing `statehash.FoldChangedPaths`. Widen the scope gate so S/A blocks
are in scope; keep B/T/P/M stalling. Box still never-Accepts.

## The apply() writes for S + A (verified in source, chain.go:3282-3298)

- **Slash** (`for _, s := range b.Slashes`): for each culprit
  - `slashed[culprit] = true`   (per-member add, key = payload-named)
  - `delete(bonded, culprit)`   (per-member delete, key = payload-named)
  - `qualifiedMaintain(culprit)` → `delete(qualified, culprit)` if it was qualified
    (per-member delete, key NOT payload-named — DERIVED from the slash screen)
- **Att** (`for _, a := range b.Atts`): for each att whose attester is qualified and not the proposer
  - `validatorsSeen[attester] = true` (per-member add, key = payload-named, SCREENED)

## THE BLOCKER — measured, not reasoned: S/A change whole-set DIGEST-ROOT leaves

I probed the real `stateRootLeavesV5()` before and after a slash / att block (temporary test,
now removed; reproducible from the fixture in `floorbox_recompute_stateroot_v5_test.go`). The
committed leaf diff a full node produces is:

**Slash block** changes SIX leaves:
1. `slashed\x00||culprit`   ADDED   (Present)             — per-member, payload-named
2. `bonded\x00||culprit`    DELETED                       — per-member, payload-named
3. `qualified\x00||culprit` DELETED                       — per-member, NOT payload-named
4. `slashedRoot\x00`        CHANGED (MTH over whole slashed set)
5. `bondedRoot\x00`         CHANGED (MTH over whole bonded set)
6. `qualifiedRoot\x00`      CHANGED (MTH over whole qualified set)

**Att block** changes TWO leaves:
1. `validatorsSeen\x00||att` ADDED  (Present)             — per-member, payload-named, SCREENED
2. `validatorsSeenRoot\x00`  CHANGED (MTH over whole validatorsSeen set)

The digest-root scalars (`slashedRoot`, `bondedRoot`, `qualifiedRoot`, `validatorsSeenRoot`) are
committed in `b.StateRoot` on EVERY v5 block (statehash.go:262-266). Their value is
`nodeSetMTH(canonical sorted WHOLE id-set)` — an RFC-6962 batch Merkle root over ALL ids of the
keyspace, NOT an incremental accumulator. Adding or removing ONE id changes the tree, and
recomputing the new MTH requires every OTHER id in the set.

**Consequence for the fold.** `FoldChangedPaths` requires `postRoot == b.StateRoot`. The
digest-root scalar leaves are CHANGED leaves in every real S/A block. To make the fold's computed
root equal `b.StateRoot`, the fold must include those digest-scalar changes with their CORRECT new
values. Those new values are whole-set MTHs. So the S/A recompute canNOT be pure O(payload): it is
O(payload) for the per-member SMT fold PLUS O(whole affected keyspace) for the digest MTH
recompute. There is NO non-empty S/A block whose state-root recompute avoids the whole-set digest
reconstruction — the digest scalar changes on every slash and every att.

**Is the digest new value self-derivable from the payload-named changed leaves alone? No.**
The fold witnesses only the CHANGED (payload-named) members. The MTH is over the whole set,
including the untouched members the fold never sees. `translog.MTH` is a batch tree, not an
incremental accumulator, so the delta cannot update it. A whole-set id-list witness
(completeness-anchored on the PRE-state digest root, the shape `RecomputeMatureNow` already ships)
is MANDATORY to recompute each affected digest. That whole-set id-list + digest reconstruction is
exactly the R-boundary machinery.

## The soundness/scope conflict I will not resolve by building

The P1-a certification is internally split on where S/A land:

- **Gated reading (main cert line 46):** "the classes with a membership screen or a whole-map
  fold (B, P, M)" are NOT yet discharged; the verdict is GATED for them. S/A have BOTH a
  membership screen (att qualification; slash-targets-bonded) AND a whole-map fold (the digest
  roots my probe measured). By this criterion S/A belong in the GATED bucket.
- **Payload-only reading (main cert line 185-186, residuals line 322-327):** "E+R (and S/A once
  screens are proven) close from the payload generator + fold"; the recompute is "certified-in-
  direction for E/R/T, gated-open for B/P/M." By this reading S/A close now.

The two readings disagree because the payload-only reading treats S/A's changed KEYS (payload-
named) as the whole story and does not account for the digest-root scalar VALUES, which my probe
shows change on every S/A block and are whole-set MTHs. `bondedRoot` and `qualifiedRoot` — the two
digests a slash changes — are named in the SAME cert (lines 293-296) and the R-scope reconciliation
(era4-v5-Rboundary-Rscope-RECONCILIATION line 109, 148) as R-boundary reads that are OPEN/GATED.
So reproducing a slash's post-root requires reconstructing exactly the digests the cert says are
gated.

This is a completeness-soundness question about a consensus recompute (`validateEra3Roots` for the
slashing/attestation classes). Per the silt research gate it is NOT a builder call: consensus-rule
reproduction + whole-set completeness soundness route to the Researcher. Building an S/A fold on my
own resolution of a cert's internal split would be exactly the "assert on a soundness question the
research gate reserves" the rules forbid.

## The options I see (for the consult to choose among)

- **Option 1 — S/A ride the R-boundary digest reconstruction (sound, not pure O(payload)).**
  Fold the per-member S/A changes O(payload) via `FoldChangedPaths`, AND for each affected digest
  keyspace (slashed/bonded/qualified for a slash; validatorsSeen for an att) take a whole-set
  id-list witness, anchor it on the PRE-state digest root (the `RecomputeMatureNow` shape), apply
  the derived membership delta, recompute the new MTH, and fold the digest scalar as a changed leaf
  with that value. SOUND. Cost = O(payload) + O(affected keyspaces). Requires the R-boundary digest
  reconstruction, which is GATED-OPEN — so this needs R-boundary lifted (or a scoped cert that the
  S/A digest reconstruction is sound independent of the rest of R-boundary). Also requires proving
  the att-qualification screen and the slash-target-bonded screen from committed per-member proofs
  (C-1) + own config (C-6) — the "once screens are proven" clause.

- **Option 2 — keep S/A OUT of scope; this increment is a no-op beyond documenting the finding.**
  The P1-a addendum already places S/A behind the scope gate (line 216: "scope gate stalls all of
  B/S/A/T/P"). If the digest reconstruction is genuinely R-boundary-gated, then S/A cannot be
  certified O(payload)-complete now, exactly like P/M. Under this reading the honest disposition is:
  do NOT widen the scope gate for S/A; record that S/A inherit the R-boundary digest obligation
  (correcting the cert's line-322 grouping of S/A with the ungated E/R/T). No production change.

- **Option 3 — a HEAVY-posture S/A recompute that is sound-now but explicitly not O(payload).**
  Same as Option 1's mechanism but framed as the heavy floor box (whole affected keyspaces
  witnessed), landing the S/A recompute for correctness while conceding the cost class. Whether the
  whole-set digest reconstruction for slashed/bonded/qualified/validatorsSeen is certifiable
  independent of the still-open R-boundary is the soundness question for the Researcher.

## My recommendation to the consult

The task's premise ("reuse R-fold, O(payload)") is refuted by the measured digest-root writes: no
S/A block is pure O(payload). The honest split is Option 2 UNLESS the Researcher certifies that the
S/A digest reconstruction (slashed/bonded/qualified/validatorsSeen whole-set MTH from a pre-state-
anchored id-list) is sound independent of the open R-boundary, in which case Option 1 is the sound
in-scope build. Either way this is a Researcher gate, not a builder decision. I stop and consult
rather than widen the scope gate on my own reading of a cert's internal split.

## What I did NOT change

No source touched. `RecomputeStateRootEntriesRevocations` and its scope gate are unchanged; S/A
still stall out-of-scope, which is the certified never-Accept posture. This doc is the deliverable
of the STOP.
