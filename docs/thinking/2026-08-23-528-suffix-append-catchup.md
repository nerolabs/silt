# #528 — the h56 liveness knee: suffix-append catch-up (deliberation)

**Date:** 2026-08-23. **Issue:** #528. **Evidence:** RC run `0de4b96-64567`
artifacts (committed, PR #529) — 198 eventloop watchdog HANGs, all
`kind=ChainReply`, stack `SyncChain → Reconcile → Append → ValidateCommit`;
`Append` sampled at h14–18 fifteen seconds into a reconcile (~1s per 1.5 MB
reg block); h57 never committed. 2/2 deterministic (`94ef1e8-36901` froze at
the same h56).

## The mechanism paragraph (build-immutable #6)

The failure is a liveness wedge at h≈56 **because** every catch-up reconcile
re-validates the entire chain from genesis: `SyncChain` already fetches only
the suffix above the requester's finalized head (slice 5, #382), but
`reconstructFork` (`core/node/chainrole.go:590`) prepends the local prefix and
`Chain.Reconcile` (`core/chain/chain.go:2707`) replays the **whole** fork in a
throwaway replica — O(height) bond re-verifications (~1s per 1.5 MB reg block)
plus an O(height × attestations) `heavier`/`Weight()` ed25519 pass — pinning
the single event loop 40–60s per reconcile, which starves sweeps and
round-change processing, so no co-round quorum forms, head mismatches trigger
more syncs, and the wedge feeds itself. This change addresses the redundant
replay **by** routing a suffix that provably extends the local committed head
through the normal `Append` commit path, per served window — validating only
the new blocks — so catch-up cost becomes O(delta) with loop occupancy bounded
by the ~3.75 MiB serve window. The #382 comment explicitly deferred this:
*"a genuine genesis-to-head block DIFF within Reconcile is a further
follow-up."* This is that follow-up, forced by the first field measurement of
its absence.

## Options

**A. Extension fast path in node sync, per served window (CHOSEN).**
When BFT finality is active and the served window anchors byte-identically on
our committed head, append the blocks above our head directly to the live
replica via `chain.Append` — the same full validation a live commit runs
(`ValidateCommit`: quorum, ancestry, bond re-verification). No
`reconstructFork`, no throwaway replay, no full-chain `Weight()`.
- *Cost:* small, node-layer only; `core/chain` consensus rules untouched.
- *Benefit:* removes the O(height) knee (the fix direction #1 in the issue)
  AND bounds loop occupancy per window (~2–3 heavy blocks per callback,
  control returns to the loop between windows — the fix direction #2's
  effect) in one move. Steady-state catch-up (1–2 blocks) costs what a live
  commit costs.

**B. Clone the live chain, append suffix into the clone, keep `heavier` +
`adopt`.** Semantics-identical by construction, but requires a correct deep
`Chain.Clone` — a new consensus-state surface where one missed field is a
silent divergence. `adopt()` documents that all derived state is a pure
function of the blocks, so A gets the same guarantee through the existing
commit path without new machinery. Rejected: more surface, no added safety.

**C. Move/chunk the full genesis replay off the loop (continuation
discipline, #467/#473 style).** Does not remove the O(height) cost, only
relocates it; the replay still burns a CPU per sync and the knee's growth
term remains. Worth doing *if* evidence shows the remaining slow path
(genuine fork divergence, legacy no-finality configs) pinning the loop —
today's evidence attributes all 198 HANGs to the catch-up path A fixes.
Deferred as a residual, not built on a hypothesis (build-immutable #7).

**D. Shrink the weight driver (reg size/cadence).** Real (ROADMAP Phase 3
"cheap heights" #299 tiers) but orthogonal and slower; the knee returns at a
higher height for any per-block cost. TTL re-denomination stays
CONTRAINDICATED per the #503 certification. Not this session.

## Why adoption-without-`heavier` is sound on the fast path

- With finality active, `Reconcile`'s gate already refuses any fork that does
  not contain our committed head (`ErrPreFinalityReorg`) — an **extension is
  the only adoptable shape**. The fast path's anchor check (served block at
  our head height hash-equals our head; each appended block's `Prev` chains
  from it) proves the peer's prefix is byte-identical to the history we
  already validated, so replaying it proves nothing new.
- Every appended block re-proves a super-quorum commit inside
  `Append → ValidateCommit` (launch: strict anchor majority; mature: >⅔
  frozen epoch weight). A committed block therefore carries strictly positive
  weight, so the extension is strictly `heavier` by construction — the
  fork-choice decision is unchanged, only no longer recomputed from genesis.
- Trust posture unchanged (C1/long-range): we still anchor on our OWN prefix,
  never a peer-served head; a peer serving junk fails `Append` and wastes
  only the suffix's validation time; the weak-subjectivity/pruned-gap refusal
  is preserved (`Append`'s pruned-block error keeps signaling
  `ErrNeedCheckpoint`).
- Equivocation detection: an extension shares no height with a
  differing local block, so the cross-fork double-sign scan is vacuous on
  this path; any divergence (same height, different block) fails the anchor
  check and falls to the unchanged slow path, which keeps the scan.
- Partial catch-up (mid-suffix peer failure) leaves a prefix of fully
  validated committed blocks — the same state as having synced one sweep
  earlier. The slow path's discard-partial-fork rule still governs forks.

## Consensus-discipline statement

No consensus rule, quorum arithmetic, or security parameter changes;
`core/chain` decision surfaces (`Reconcile`, finality gate, `heavier`,
`RequiredQuorum`) are not edited. The change is node-layer routing of an
already-validated-equivalent case through the existing commit path. Per the
consensus-correctness discipline the enforcement is deterministic tests, not
a field run: a born-RED catch-up test (no slow-path reconcile on a pure
extension), divergence/pruned-gap/mid-failure regressions, and the existing
model-check tier stays green. I1/I3 untouched (no quorum sizing or set
change). I2 untouched (no signing path). I4: liveness improved — the sync
path can no longer starve the round ladder for O(height); commit/final
semantics unchanged. I5: fork-choice outcomes identical (extension adoption
is the `heavier`-forced outcome; all other shapes take the unchanged path).

## Scope (one goal)

Ship A with failing-first tests and a local before/after knee measurement.
Not this session: C (off-loop replay residual), D (#299 proof-size tiers),
any RC re-run (billable; needs the fix banked and Andrew's go).
