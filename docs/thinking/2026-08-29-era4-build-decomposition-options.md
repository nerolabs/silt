# era-4 — the ordered build decomposition (PACE deliberation, DESIGN ONLY)

**Date:** 2026-08-29
**Seat:** Builder
**Status:** design only — NO mechanism code. This doc locks the ORDERED build increments before
any `apply()`/predicate/consensus change. Each increment below is a separate PR through the blind
review loop, gated on its own ablation going red.
**Grounded against:** `origin/main` @ `0984db4` (HEAD verified; every `file:line` below re-checked
against this commit).

**Ratified inputs this decomposition executes** (do not re-litigate):
- Format veto-gate RATIFIED 2026-08-29 (`docs/decisions.md`, era-4 entry). RECERT2 cert:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`.
- `BlockVersion = 5`, `versionSupported <= 5`, PREDICATE-FIRST.
- Three new committed tags: `tagDueBucket` (TTL), `tagQualified` + `tagEpochStart` (rotation);
  `tagEpochSet` retained.
- TWO-keyspace layout: frozen materialized `epochSet` + live `qualified` accelerator.
- `RegCap = 256` fresh-reg validity rule (pinned; #299 re-mint gate recorded).
- Recovery boundary (`effectiveEpochSet` at `LivenessRecoveryHeight`) SCOPED OUT of era-4-minimum.

The design deliberation this build-plan implements is
`docs/thinking/2026-08-29-era4-witnessable-transitions-options.md` (RATIFIED). This doc answers a
different question: **in what order do the pieces land, and where does each safety gate sit.**

---

## 1. The answer up front

Land the **maintenance spine WITH the v5 predicate/activation in the same era bump, but as an
ordered sequence of PRs where the new keyspaces are committed and drift-guarded BEFORE they are
witness-load-bearing.** The ordering, stated as the sequence:

| # | Increment | What it does | State-root effect | First-class gate (must go red) |
|---|---|---|---|---|
| **4a** | widen-version + tags-defined-not-committed | Mint `BlockVersion=5`; add the three tags to the tag table and `stateRootTags`; extend `versionSupported <= 5` **guarded by the predicate landing in 4c** (see §3). Keyspaces declared, classification wired. | NONE yet (tags carry no leaves until the maps exist + are populated). | `TestStateFieldsAreClassified` reddens if a new committed field lacks a tag. |
| **4b** | maintenance spine — `qualified` + due-bucket maps, hooks, drift-guards | Add the `qualified` and due-bucket in-memory maps; wire the five `qualified` hooks (2989/2995/3008/3019/3020) and the due-bucket insert/move/delete; commit them under the state root as v5-only leaves. NO predicate change, NO activation. | Changes the root **only for v5-tagged blocks** — but no block is v5 yet (predicate/activation not landed), so on the live chain the root is unchanged. The new leaves are exercised by the model-check corpus + the byte-identical replay, not by any produced block. | The `qualified` drift-guard (per site, **2989 reddens specifically**); the T-3 due-bucket dual-source drift-guard; the T-3 byte-identical StateRoot replay vs era-3; `TestDryRunCloneCopiesEveryAppliedField`. |
| **4c** | v5 validity predicate + `RegCap` rule | The v5 committed-root validity predicate on EVERY disk-write path (incl. Reload); the `RegCap = 256` fresh-reg rule; `versionSupported <= 5` becomes live in the SAME release. | The predicate now REQUIRES the v5 root shape on v5 blocks. | The RegCap rejection test (>256 fresh regs → reject); the predicate-on-Reload test (re-signed wrong v5 root → reject on own-disk Reload). |
| **4d** | height-gated activation + mint-flip to v5 | Height-gated `H_era4`, one-way weight-tallied lock-in on an epoch-final boundary; all disk-write paths mint v5. | At/above `H_era4`, produced blocks are v5 and carry the new leaves; the root moves for real. | The activation-boundary test (v3/v4 block rejected at `H_era4` with the era-4 version error; laggard stalls not accepts); the Q5 recovery-branch agreement assertion. |

**Why WITH, sequenced — not BEFORE and not AFTER:**

- **Not fully BEFORE (spine on a pre-v5 chain, committed for all versions).** Committing the new
  keyspaces under the state root for era-3 (v4) blocks would **edit the frozen era-3 format** —
  the byte-identical-root property era-3 froze on `3af40bc` would break, and every deployed v4
  node would diverge. The era-3 freeze is IMMUTABLE (`docs/decisions.md`, era-3 FROZEN entry):
  changing it requires a new era, not an edit. So the new leaves MUST be v5-gated in the leaf
  marshaller — they contribute to the root only when the block is v5.
- **Not AFTER (predicate/activation first, spine later).** The v5 predicate validates the v5 root.
  If the maintenance spine is not yet producing the `qualified`/due-bucket leaves, the predicate
  either validates against an incomplete root (a silent completeness hole — the exact
  `TestStateFieldsAreClassified` failure the guard exists to catch) or the predicate has nothing
  to check. The predicate is meaningless without the leaves it commits, and the leaves are
  meaningless (un-checked) without the predicate. They are one atom of correctness.
- **WITH, sequenced (the chosen order).** Land the spine first (4b) so the drift-guards and the
  byte-identical replay prove the new maps are maintained correctly and the v5 root is
  byte-reproducible **before** any block is v5. Then land the predicate (4c) that requires that
  root, then flip activation (4d). The key property that makes 4b safe to land ahead of the
  predicate: **committing new keyspaces changes the state root only for v5 blocks**, and no block
  is v5 until 4d. So 4b is inert on the live chain (root unchanged for the v4 blocks still being
  produced) while being fully exercised by the model-check corpus and the byte-identical replay.
  This is the era-3 lesson applied: era-3's step 1 (`#627`) computed the roots behind the oracles
  with no `Block`/`BlockVersion` change before the field became a signed predicate-checked field.

This mirrors the era-3 analog (2a widen-version → 2b predicate → 2c activation+mint-flip) with one
addition: era-4 has a **maintenance spine** era-3 did not (era-3 committed EXISTING maps; era-4
adds NEW live-maintained maps). That spine is 4b, and it lands between widen-version (4a) and
predicate (4c) — not folded into either — so its drift-guards get their own blind-review PR and
their own red ablation before the predicate makes them load-bearing.

---

## 2. The increment sequence — cost/benefit per increment

### 4a — widen-version + tags-defined (the schema-only PR)

**Does:** Mint `BlockVersion = 5` (a new const beside `BlockVersionStateRoot = 4`,
`chain.go:339`). Add `tagDueBucket`, `tagQualified`, `tagEpochStart` to the tag block
(`statehash.go:39-40` region) and to `stateRootTags` (`statehash.go:57`). Wire classification so
the new fields are recognized. **Does NOT** yet populate the maps or change `versionSupported` to
`<= 5` live (see §3 — the version widen is bundled into 4c's release, predicate-first).

**Cost:** small. Schema + classification only. No `apply()` change, no predicate.
**Benefit:** gets the tag additions and classification wiring through the blind loop in isolation,
where `TestStateFieldsAreClassified` is the whole story. A tag added without a classification entry
(or vice versa) reddens here, cheaply, before any behavior depends on it.
**Ablation that reddens it:** delete one of the three new tag→classification links →
`TestStateFieldsAreClassified` goes red (the guard "catches the field nobody classified,"
`modelcheck_state_completeness_test.go:128,148`).

**Ordering note:** whether the `versionSupported <= 5` widen lands in 4a or is held to 4c is the
PREDICATE-FIRST decision — see §3. Recommendation: **hold the live `<= 5` widen to 4c** so the
version ceiling and the predicate ship together.

### 4b — the maintenance spine (the heaviest, most gate-dense PR)

**Does:** Add the `qualified map[ports.NodeID]int64` and the due-bucket index (an
`map[uint64]map[ports.NodeID]struct{}` keyed by due-height, or equivalent) to `Chain`. Wire:
- **Five `qualified` hooks** at the `bonded`/`slashed` mutation sites — displaced-squatter delete
  (`chain.go:2989`, deletes `owner`), fresh/renew write (`chain.go:2995`, writes `id`), TTL delete
  (`chain.go:3008`), slash mark (`chain.go:3019`), slash evict (`chain.go:3020`). Each site that
  changes `filter(bonded, slashed, MinBond)` must correspondingly maintain `qualified`.
- **Due-bucket insert/move/delete** at the TTL machinery — insert on fresh/renew reg (due-height =
  `bondRegHeight[id] + ttl + 1`, per the due math at `chain.go:1784,1800`), move on renew
  (delete the OLD due-height entry, insert the NEW), delete on TTL expiry (`chain.go:3005-3013`
  sweep) and on slash.
- Commit both maps under the state root as **v5-only** leaves (the leaf marshaller emits them only
  when the block/height is v5 — the gating that keeps era-3 blocks byte-identical).
- Deep-copy both in `cloneForDryRun` (`era3validity.go:173-175`, joins `slashed/bonded/epochSet` as
  distinct maps) and in `adopt` (`chain.go:3546` region).

**Does NOT:** change `versionSupported`, add the predicate, or activate. On the live chain (all v4
blocks) the root is unchanged because the new leaves are v5-gated and no block is v5 yet.

**Cost:** the largest PR. Five hooks + due-bucket lifecycle + clone/adopt + commitment. This is the
"new whole-map maintenance" that era-3 never had.
**Benefit:** all the CERT's build-time drift obligations land and ablate here, in one PR whose ONLY
job is maintenance correctness — before any predicate makes the maps load-bearing. If a hook is
wrong, the drift-guard reddens against the model-check corpus, not against a produced v5 block.
**Why this can precede the predicate safely:** the state-root-changes-only-for-v5-blocks property
(the leaf gating). See §1.

### 4c — v5 predicate + `RegCap` + version widen (the predicate-first PR)

**Does:** Add the v5 committed-root validity predicate on **every disk-write path including
Reload** (the era-3 lesson: `5951a76`/`3af40bc` put the era-3 predicate on all paths incl. Reload;
the own-disk Reload gap was a real finding — see `docs/thinking/2026-08-29-era3-reload-root-check-options.md`).
Add the `RegCap = 256` fresh-reg validity rule (count distinct first-time registrations per block;
`ok == false` in `chain.go:1587` marks a fresh reg; reject if > 256). Widen `versionSupported` to
`<= 5` (`chain.go:740`) in this SAME release — PREDICATE-FIRST (§3).

**Cost:** medium. Predicate wiring on all paths + the RegCap counter.
**Benefit:** closes the interim window era-3 left open (version accepted before predicate). The
predicate now checks the root the 4b spine produces.
**Ablations:** RegCap — a block with 257 fresh regs must reject; the predicate — a re-signed wrong
v5 root must reject on own-disk Reload (the era-3 A′ anchor-bound lesson).

### 4d — height-gated activation + mint-flip (the go-live PR)

**Does:** Height-gate `H_era4`; one-way weight-tallied `>⅔` lock-in on an epoch-final,
reorg-stable boundary gated on a `regVersion >= 5` supermajority (era-3 shape,
`docs/decisions.md` era-3 FROZEN entry, `chain.go:255` region). All disk-write paths mint v5 at/above
`H_era4`.

**Cost:** medium. Activation tally + mint-flip on all write paths.
**Benefit:** the root moves for real; era-4 is live. Laggards stall (safe), never accept.
**Ablations:** a v4 block at `H_era4` must reject with the era-4 version error; the Q5 recovery
agreement assertion (materialized `qualified` vs recomputed `liveQualifiedSet()` at the recovery
boundary) must hold.

---

## 3. PREDICATE-FIRST `versionSupported <= 5` sequencing (ratified)

Andrew ratified PREDICATE-FIRST: **widen the version ceiling in the SAME release as the validity
predicate.** Today `versionSupported(v) = v >= 1 && v <= BlockVersionStateRoot` (= 4,
`chain.go:740`). era-3 widened `<= 4` a release AHEAD of the predicate, leaving an interim window
where a binary decode-accepted a v4 block but had no state-root predicate to check it — a
"silently accept a forged root" exposure the era-3 certification called out.

era-4 closes that window: the `versionSupported <= 5` widen lands in **4c**, the same PR as the v5
predicate — NOT in 4a. Consequence for the decomposition: 4a mints the `BlockVersion = 5` constant
and defines the tags but does NOT lift the decode ceiling. A v5 block does not decode-validate until
4c ships the predicate that checks it. This is why 4a is "tags-defined-not-committed" and the live
`<= 5` widen is explicitly held to 4c.

**Hazard this forecloses:** if the widen shipped in 4a (ahead of the predicate), a v5 block from a
misbehaving/ahead proposer would decode-accept with no predicate — the exact era-3 interim exposure.
Predicate-first means the ceiling and the check are atomic in one release.

---

## 4. Intra-block ordering preservation (a covered ablation)

The intra-block apply order is load-bearing and must be preserved byte-for-byte: **bonds → TTL →
slashes → rotate-LAST.** Verified against `0984db4`:

- Fresh/renew bond writes: `chain.go:2995` (and the displaced-squatter delete at `2989`).
- TTL-expiry sweep: `chain.go:3003-3013` (deletes `bonded`/`bondRegHeight`/`regVersion`).
- Slash: `chain.go:3019` (mark), `3020` (evict from `bonded`).
- Rotate LAST: `chain.go:3046` (`if c.epochsEnabled() && b.Height%c.cfg.EpochBlocks == 0 {
  c.rotateEpoch(b.Height) }`), which copies `c.epochSet = c.liveQualifiedSet()`
  (`chain.go:3130-3131`).

The 4b maintenance hooks MUST fire in this same order, because `qualified` and the due-bucket are
derived state: if a hook fires before the `bonded`/`slashed` mutation it mirrors, or if rotation
reads `qualified` before the block's own bonds/TTL/slashes have updated it, the boundary copy
`epochSet := qualified` captures a stale set — an I3 mid-epoch-churn-equivalent divergence.

**Ablation that reddens it (owed in 4b):** reorder any hook relative to its mutation site (e.g.
maintain `qualified` for the fresh bond BEFORE the TTL sweep runs its deletes) → the byte-identical
post-apply StateRoot replay vs an era-3 replay diverges over the corpus (renew-reset, ttl==0,
slash-before-due). The rotate-LAST ordering specifically: move the `qualified`-read that feeds the
boundary copy ahead of `rotateEpoch` and the Q5 agreement assertion (materialized `qualified` vs
recomputed `liveQualifiedSet()`) goes red.

---

## 5. Where each safety gate lands, and what ablation reddens it

Per the CERT's build-time obligations (RECERT2 Residuals, "Owed at build"). Each MUST go red before
its increment is trusted — the inject-the-defect rule. The BIG LESSON of session 7: a green check
with no demonstrated red is a comment that compiles.

| Gate | Increment | Invariant it holds | Ablation that reddens it |
|---|---|---|---|
| `qualified` maintenance drift-guard | **4b** | `qualified == filter(bonded, slashed, MinBond)` after every block | Skip the `qualified` maintenance at each of 2989/2995/3008/3019/3020 in turn. **The 2989 hook MUST redden specifically** — it deletes the displaced squatter's `owner` (distinct from the `id` written at 2995, RECERT2 verified); a `qualified` update that mirrors only 2995 and misses 2989 leaves a stale qualified entry for the displaced owner. |
| T-3 due-bucket dual-source drift-guard | **4b** | `bucket-membership(id) ⟺ (bondRegHeight[id] + ttl + 1 == D AND bonded[id] present)`, AND byte-identical StateRoot vs an era-3 replay | Drop the OLD-due-height delete on renew (insert the new bucket entry, forget to remove the old). Membership then names a due-height that no longer matches `bondRegHeight[id]+ttl+1`; the dual-source predicate goes red and the StateRoot diverges from the era-3 replay. |
| T-3 byte-identical post-apply StateRoot replay | **4b** | The v5 post-apply StateRoot is byte-identical to an era-3 replay of the same block stream, over a corpus covering **renew-reset, ttl==0, slash-before-due** | Any maintenance bug (wrong hook, wrong order, missed delete) that changes a committed leaf diverges the root byte-for-byte from the era-3 replay on at least one corpus case. |
| Q5 recovery-branch agreement assertion | **4d** | materialized `qualified` == recomputed `liveQualifiedSet()` at the recovery boundary | Introduce any `qualified` drift, then hit the recovery boundary (`chain.go:1243-1248`, gated on `LivenessRecoveryHeight`): the two producers of the recovery set disagree → assertion red. (Both are `filter(bonded, slashed, MinBond)`; they agree IFF the 4b maintenance invariant holds.) |
| `TestDryRunCloneCopiesEveryAppliedField` | **4b** | Every applied field is deep-copied in `cloneForDryRun` (the postApplyRoots drift guard) | Add the `qualified`/due-bucket map to `Chain` and to `apply()` but forget it in `cloneForDryRun` (`era3validity.go:173-175`) → the dry-run root diverges from the applied root → guard red (`modelcheck_era3_validity_test.go:256,266`). |
| `TestStateFieldsAreClassified` | **4a** | Every committed field has a tag + classification | Add a new committed field without its `stateRootTags` entry → guard red (`modelcheck_state_completeness_test.go:128,148`). |

**The one gate that is easy to leave decorative:** the `qualified` drift-guard's 2989 site. Because
2989 deletes `owner` while 2995 writes `id`, a naive maintenance mirror ("whenever `bonded[id]`
changes, set `qualified[id]`") covers 2995 and 2989's *write side* but not 2989's *delete of a
DIFFERENT key* (the displaced owner). The guard is not shipped until the 2989-specific ablation is
watched going red — mirror-only-2995 must NOT pass the guard.

---

## 6. The proposed FIRST code increment (to be gated)

**4a — widen-version + tags-defined-not-committed.** Scope, precisely:
1. Add `const BlockVersion = 5`-class constant beside `BlockVersionStateRoot = 4` (`chain.go:339`);
   do NOT change `versionSupported` (held to 4c, predicate-first — §3).
2. Add `tagDueBucket`, `tagQualified`, `tagEpochStart` to the tag block (`statehash.go:39-40`) and
   to `stateRootTags` (`statehash.go:57`).
3. Wire classification for the three new fields.
4. **No** `Chain` map additions, **no** `apply()` change, **no** predicate, **no** activation.

**Gate on 4a:** the only red-able check is `TestStateFieldsAreClassified`. Ablate by removing one
tag→classification link and confirm red. This is a deliberately thin first PR — it isolates the
schema wiring so the heavier 4b spine lands against a clean, classified tag table.

**Recommended review framing for the blind loop:** pass 4a as "schema + classification only, no
behavior" and let the reviewer confirm no `apply()`/predicate/version-ceiling change slipped in.

---

## 7. Ordering hazards found while decomposing

1. **The era-3 freeze forbids committing the new keyspaces for v4 blocks.** The new leaves MUST be
   v5-gated in the leaf marshaller. If 4b commits them unconditionally (for all versions), every
   deployed v4 node's root diverges and the era-3 byte-identical freeze breaks. This is why 4b is
   "inert on the live chain" only because of the v5 leaf gating — that gating is load-bearing, not
   an optimization. **Owed check in 4b:** an ablation that removes the v5 gate on a new leaf and
   confirms the era-3 replay root diverges (proves the gate is doing its job).
2. **The rotate-LAST read of `qualified` is a stale-capture trap.** `rotateEpoch` (`chain.go:3130`)
   copies from `liveQualifiedSet()` today. In era-4 the boundary copy source becomes the
   materialized `qualified`. If any 4b hook that should have updated `qualified` for THIS block's
   bonds/TTL/slashes fires AFTER the rotation read (or not at all), the boundary captures a stale
   set — an I3-equivalent mid-epoch divergence. The rotate-LAST ordering (§4) is the mitigation;
   the Q5 assertion (§5) is the catch. **This is the sharpest ordering hazard:** it couples the
   intra-block ordering (§4) to the recovery-branch agreement (Q5) through the shared `qualified`
   map.
3. **`RegCap` counts FRESH regs only, and fresh = `bondRegHeight` unset.** The rule must count
   distinct first-time registrations (`chain.go:1587`, `ok == false`), NOT renewals — renewals are
   exempt (#506 R-rule) and unbounded by RegCap by design. A RegCap that counts all `BondRegs`
   would reject honest renewal-heavy blocks. **Owed check in 4c:** an ablation with 300 renewals +
   200 fresh regs must ACCEPT (renewals don't count) while 257 fresh regs must REJECT.
4. **Predicate-first coupling.** The `versionSupported <= 5` widen and the v5 predicate must be
   atomic in one release (§3). If they split across releases (widen in 4a, predicate in 4c), the
   interim binary decode-accepts v5 with no predicate — the era-3 interim exposure. The
   decomposition holds the widen to 4c specifically to foreclose this.

---

## 8. What this doc does NOT decide (routed elsewhere)

- The `RegCap` VALUE is ratified at 256 with the #299 re-mint gate (`docs/decisions.md`, era-4
  entry; `docs/design/owned-residuals.md`, RegCap owed-input). Not re-opened here.
- The recovery-boundary direction (`effectiveEpochSet` at `LivenessRecoveryHeight`) is scoped OUT
  of era-4-minimum (Andrew's call). era-4 keeps the recovery branch reading `liveQualifiedSet()`
  as today; the Q5 assertion only proves the materialized `qualified` AGREES with that recompute —
  it does not witness the recovery re-base. Witnessing recovery is a separate gated item (R2).
- The witness floor-box mechanism itself (C-7 / #600) is a separate track. era-4 makes the two
  whole-map transitions witnessable; the floor box that consumes the witnesses is not built here.
