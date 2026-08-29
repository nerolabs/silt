# era-4 step 4b — the maintenance spine (build approach + ablation plan)

**Date:** 2026-08-29
**Seat:** Builder
**Status:** build approach for an ALREADY-CERTIFIED design. NO design change. If any step below
required deviating from the certified design or the RECERT2 gates, this doc would STOP and report
the deviation instead of building. It does not: every step traces to the design or a build-time
obligation the cert names.
**Grounded against:** this worktree's branch `era4-4b-maintenance-spine` off `origin/main` @
`effe115` (4a landed: `BlockVersionWitnessable = 5` at `core/chain/chain.go:357`; the three inert
tag strings `tagDueBucket`/`tagQualified`/`tagEpochStart` at `core/chain/statehash.go:68-70`).

**Certified inputs this build executes (do not re-litigate):**
- Design: `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md` (RATIFIED).
- Decomposition: `docs/thinking/2026-08-29-era4-build-decomposition-options.md` (4b row).
- Cert: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`
  (CERTIFIED-WITH-CONDITIONS; the one hard pre-build condition is the RegCap VALUE, which is a
  **4c** rule — NOT built here).

---

## 1. The answer up front — what 4b builds, and what it does NOT

4b adds the two new live-maintained derived committed maps (`qualified`, the due-bucket index) and
the frozen `epochStart` scalar to `Chain`, wires their maintenance at the five bonded/slashed sites
and the TTL machinery, commits them under the state root **as v5-only leaves**, and ships the two
drift-guards + the byte-identical replay + the ordering ablation. It does NOT add the v5 predicate,
does NOT change `versionSupported`, does NOT add the RegCap rule, and does NOT flip minting. Those
are 4c/4d.

The load-bearing property that makes 4b safe to land before the predicate (decomposition §1, §7):
**committing the new keyspaces changes the state root ONLY for v5 root computations.** No block is
v5 until 4d, so on the live chain the committed root is byte-identical to era-3. The new leaves are
exercised by the model-check corpus (tests that build v5 roots), not by any produced block.

### The one non-obvious wiring the design mandates (NOT a deviation)

`stateRootLeaves()` / `StateRoot()` today are a pure function of committed state with no version.
The design requires the new leaves to be emitted "only when the block/height is v5" (decomposition
§1, hazard-1). So the marshaller MUST become era-aware. The mechanism:

- Add `stateRootLeavesV5()` that appends the three new keyspaces' leaves to the era-3 leaf set, and
  an era-parameterized `stateRootFor(era)` / `StateRootV5()`.
- `postApplyRoots(b)` selects the marshaller by `b.Version`: a v5 block recomputes the v5 root (era-3
  leaves + the three new keyspaces); a v4 block recomputes the era-3 root **byte-identically** (the
  new keyspaces contribute nothing). `StateRoot()` (the era-3 entry) is UNCHANGED and still emits
  exactly the 18 era-3 leaves — so every era-3 caller and the frozen era-3 replay corpus stay
  byte-identical.
- This is the era-3 lesson applied (decomposition §1): the marshaller gates the new leaves on the
  era, exactly as era-3 step-1 computed roots behind the oracles before the field was predicate-checked.

This is wiring the certified decomposition explicitly calls for ("the leaf marshaller emits them
only when the block/height is v5"). It is not a design change. Confirming it is not a deviation:
RECERT2 Q1 certifies the two-keyspace layout and the v5-gated commit; the decomposition names the
v5 leaf gate as "load-bearing, not an optimization" (§7 item 1).

### Classification: the new maps are `committedSet`, but v5-gated in the marshaller

`qualified` and the due-bucket index are derived set-valued state that goes under the state root, so
they classify `committedSet`. `epochStart` today is classified `observable` (its only reader is
`Regime()`); O-1 promotes it to committed (it now emits a v5 leaf). Because the completeness guards
(`TestStateRootCoversExactlyTheCommittedSetFields`, `TestStateRootEmitsALeafForEveryCommittedField`,
snapshot-boot equivalence, order-independence) assert over `committedSet` fields and today assume
"always emitted," they must be extended to test the **v5 marshaller path** for the new fields while
keeping the era-3 path (18 fields) unchanged. This is the "completeness guards force the new tags"
obligation (task item 5 + decomposition §5) — the guards are what make a forgotten new keyspace a
RED unit test, not a field divergence.

Two representation choices, both fixed by the cert (not re-opened here):
- **Due-bucket value = MTH over a CANONICAL (sorted-ascending / dedup / unpadded) carried id list**
  (design §4 variant b; RECERT2 "canonical id-list encoding … forecloses MTH malleability"). 4b
  commits the bucket leaf as that MTH. The shape-gate REJECT of non-canonical lists is a floor-box
  concern (witness verification) that lands with the witness path, not 4b; but the COMMITTED value
  4b writes is the MTH over the canonical list, so the committed representation is canonical from
  the start. (4b builds the committed side; it does not build the floor-box shape gate.)
- **Two keyspaces, `qualified` (live) + `epochSet` (frozen, era-3 shape retained).** Settled by
  RECERT2 Q1. `epochSet` shape is UNCHANGED; the boundary copies `qualified -> epochSet`.

---

## 2. The maintenance sites (verified against THIS worktree, line numbers re-checked)

The design cites 2989/2995/3008/3019/3020 against `0984db4`. On `effe115` the same five sites are at
DIFFERENT line numbers (the file shifted). Verified by reading `apply()` this worktree:

| # | Design line | THIS worktree | Mutation | `qualified` action |
|---|---|---|---|---|
| 1 | 2989 | **3007** | `delete(c.bonded, owner)` (displaced squatter) | `delete(qualified, owner)` — the MISSED site; `owner` ≠ `id` |
| 2 | 2995 | **3013** | `c.bonded[id] = r.Size` (fresh/renew/resize) | set `qualified[id]=r.Size` if `r.Size>=MinBond && !slashed[id]`, else delete |
| 3 | 3008 | **3026** | `delete(c.bonded, id)` (TTL expiry) | `delete(qualified, id)` (co-located with due-bucket delete) |
| 4 | 3019 | **3037** | `c.slashed[culprit]=true` (slash mark) | `delete(qualified, culprit)` |
| 5 | 3020 | **3038** | `delete(c.bonded, culprit)` (slash evict) | co-located with #4 |

Site 2 (fresh/renew) is also where `bondRegHeight[id] = b.Height` is set (worktree line 3014), so the
due-bucket insert/move hangs off the same site: due-height `D = b.Height + BondTTLBlocks + 1`; on a
renew the OLD bucket entry (`D_old` from the previous `bondRegHeight[id]`) is deleted first, then the
new `D` is inserted. Site 3 (TTL) deletes the bucket entry at the current height. TTL disabled
(`BondTTLBlocks==0`) inserts NO bucket (due height undefined) — matches era-3 skipping the sweep.

Ordering: the five hooks fire AT each site in the existing intra-block order (bonds → TTL → slashes),
and the boundary copy `epochSet := qualified` stays LAST (worktree `rotateEpoch`, the
`c.epochSet = set` line, sourced from `qualified` instead of `liveQualifiedSet()`). rotate-LAST is
gated at the worktree's `if c.epochsEnabled() && b.Height%c.cfg.EpochBlocks == 0` site.

The `rotateEpoch` subtlety verified: today `rotateEpoch` sets `epochStart = h` UNCONDITIONALLY, then
`if !c.everMature { return }` before `set := liveQualifiedSet(); c.epochSet = set`. So the boundary
copy source-swap (`liveQualifiedSet()` → `qualified`) goes at the existing `c.epochSet = set` line,
AFTER the everMature gate — unchanged control flow, only the source of the copy changes. `epochStart`
is already written at the top of `rotateEpoch`; O-1 just commits it.

---

## 3. Build order within 4b (each piece lands green before the next)

1. Add `qualified map[ports.NodeID]int64` and `dueBucket map[uint64]map[ports.NodeID]struct{}` and
   promote `epochStart` to committed — classify all three (`committedSet` for the two maps;
   `epochStart` moves `observable → committedSet`). Init in `New`. This REDDENS
   `TestStateFieldsAreClassified` / `TestDryRunCloneCopiesEveryAppliedField` / adopt guard until
   wired — the guards doing their job.
2. Wire `cloneForDryRun` (deep-copy all three) and `adopt` (swap all three). Greens the clone/adopt
   guards.
3. Wire the five `qualified` hooks + the due-bucket insert/move/delete at the TTL machinery + the
   boundary copy source-swap.
4. Make the marshaller era-aware: `stateRootLeavesV5()` appends the three keyspaces; `postApplyRoots`
   selects by `b.Version`. Extend the completeness/coverage guards to test the v5 path (new fields
   emit v5 leaves; era-3 path unchanged at 18 leaves).
5. Ship the drift-guards + byte-identical replay + ordering ablation (section 4).

The due-bucket is an in-memory `map[height]set`; its COMMITTED form is one leaf per occupied
due-height, value = MTH over the canonical sorted id list (design §4b). The marshaller derives the
canonical list from the in-memory set at leaf time, so the committed value is canonical regardless of
map order (mirrors the existing order-invariant leaf property).

---

## 4. Ablation plan — every guard driven RED before it is trusted

The standing lesson (session-7): a green check with no demonstrated red is a comment that compiles.
For EACH, I write the test, inject the defect, observe RED, restore green, and record the red in the
report. Scratch repros go to /tmp, never `core/`.

| # | Guard | Defect injected | Expected RED |
|---|---|---|---|
| A | **Era-3 byte-identical replay corpus stays GREEN with new keyspaces present** | Populate `qualified`/`dueBucket`/`epochStart` on a chain, compute the **era-3** (v4) root; assert byte-identical to the same chain without the new maps. Then inject a NON-gated commit (emit the new leaves into the era-3 path) | the era-3 replay root DIVERGES → RED (proves hazard-1 v5-gating) |
| B | **`qualified` maintenance drift-guard, per-site** | `qualified == filter(bonded, slashed, MinBond)` after every block over a corpus exercising displacement(3007)/renew/slash/expiry in ONE block. Drop the maintenance at site **3007** specifically | drift-guard RED on the 3007 hook (RECERT2 R1 — must redden on that exact site) |
| C | **T-3 dual-source guard** | `bucket-membership(id) ⟺ (bondRegHeight[id]+ttl+1==D AND bonded[id] present)` + byte-identical v5 StateRoot vs an era-3-shape replay. Drop the OLD-bucket delete on renew | dual-source predicate RED + root diverges |
| D | **Rotate-LAST stale-capture ordering ablation (sharpest)** | A hook that reads `qualified` to feed the boundary copy AFTER `rotateEpoch` freezes a STALE set. Inject: move the boundary `qualified`-read ahead of this block's bond/TTL/slash maintenance (or feed `epochSet` from a pre-maintenance snapshot) | the byte-identical replay / Q5-shape agreement RED (stale set ≠ era-3 `liveQualifiedSet()` at the boundary = I3 divergence) |
| E | **Q5 recovery-branch agreement (shape)** | Assert materialized `qualified == liveQualifiedSet()` at a recovery-boundary height. Inject any `qualified` drift, hit the recovery boundary | the two producers disagree → RED |
| F | **Completeness guards force the new tags** | `TestStateRootCoversExactlyTheCommittedSetFields` (extended v5 path) / `TestStateRootEmitsALeafForEveryCommittedField` (v5). Add the field to `Chain` but DROP its v5 leaf loop | coverage/emit guard RED (the field escapes the v5 root) |
| G | **`extra` branch stays STRICT** | Keep the coverage guard's `extra` (a tag with no classified field) branch. Add a stray v5 tag with no committedSet field | `extra` branch RED |
| H | **`TestDryRunCloneCopiesEveryAppliedField`** | Omit one of the three new maps from `cloneForDryRun` | clone guard RED (dry-run root ≠ applied root) |

Q5 (E) and D are the sharpest: they couple the intra-block ordering to the boundary copy through the
shared `qualified` map. Both are v5-shape assertions (4b does not activate v5, so they run against
v5 root computations the test constructs, not produced blocks).

The RegCap rule (4c) is explicitly OUT. The T-3 byte-identical replay here uses whatever corpus the
model-check tier supplies; it does NOT assert any per-block cap. `MaxBondRegBytesPerBlock` is NOT
touched (proposer policy).

---

## 5. Where this could deviate — checked, and it does not

- **v5-gating the marshaller** is design-mandated (decomposition §1/§7), not a new mechanism.
- **Promoting `epochStart` to committed** is O-1, CERTIFIED narrowly (RECERT2 Residuals; changes no
  quorum decision — its only reader is `Regime()`). Reclassifying `observable → committedSet` and
  emitting its v5 leaf is exactly O-1.
- **The two-keyspace layout** matches RECERT2 Q1 verbatim; `epochSet` shape unchanged.
- **The five sites** are the cert's grep-complete enumeration, re-verified at their shifted lines.
- **No RegCap, no predicate, no version widen, no mint-flip** — all 4c/4d. 4b commits new leaves that
  are inert on the live chain because no block is v5.

If, during build, the completeness guards cannot be extended to the v5 path without ALSO forcing the
era-3 path to emit the new leaves (which would break the frozen era-3 replay), that WOULD be a
deviation — I would STOP. The design forecloses it: the marshaller keys the new leaves on the era,
and the era-3 entry point stays the 18-field marshaller. Ablation A is the proof this holds.

**Decision:** build 4b as specified above. No deviation. Proceed to implementation behind the
completeness guards and both drift-guards, each ablated RED, plus the ordering and Q5-shape ablations.
