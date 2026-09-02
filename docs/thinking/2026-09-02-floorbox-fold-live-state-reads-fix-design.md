# Floor-box fold live-state reads — Direction-A fix design (R-FOLD-LIVE-STATE-READS)

Date: 2026-09-02 · Seat: Builder · Boulder 1 (floor-box witness-soundness spine)
Binding spec: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-R-FOLD-LIVE-STATE-READS-RESEARCH-CERTIFICATION-2026-09-02.md`
Routed from: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R-CARRIER-REFLECTION-pin-2026-09-02.md`

Terminology: the current open era is **v4**; the code's `*V5*` identifiers name this same era.

## The decision

Build the certified Direction A, recompute-side, with no committed/wire format change. The
class-A branch selector and the launch-anchor predicate stop reading box-own live state and
read the Resolved committed pre-state instead. Add a cold-box test tier so the defect class
cannot recur invisibly.

## The mechanism (why the fix is what it is)

**The failure is** a cold floor box screening a mature-epoch block's attestations under the
PRE-maturity rule, **because** the class-A screen selects its branch from `c.matureEpoch`
(`core/chain/floorbox_recompute_stateroot_atts_v5.go:176`) and its anchor value from
`c.launchAnchor` → `c.handedOff()` (`chain.go:1252 → :1223`) — box-own fields written only by
`apply→rotateEpoch` (`chain.go:3398`) and `adopt` (`chain.go:3911`). The deployment target
"holds NO registry and replays NO apply()"
(`floorbox_recompute_stateroot_v5.go:181-182`), so those fields are never written and the
mature branch at `:176-193` is unreachable. **This change addresses that by** resolving
`tagMatureEpoch` (already a committed leaf, `statehash.go:197` / `readset_v5.go:576`) present
against `prevStateRoot`, unconditionally, before the branch, and evaluating the anchor
predicate over the anchored pre-values.

`apply()` reads `matureEpoch` in the attestation loop (`chain.go:3293-3298`) which runs BEFORE
`rotateEpoch` (`:3306`, rotate-LAST, pinned by the PR #703 drift guard). So the value `apply()`
sees is exactly the PRE-state value committed in `prevStateRoot`. The Resolved pre-value
reproduces the oracle byte-for-byte, including on the handoff block itself.

## What changes

| Step | Change | File |
|---|---|---|
| 1 | `MatureEpoch StateRootRotateScalar` added to the class-M carrier `StateRootMaturityWitness` (required on every block), not to the boundary-only Rotate witness | `floorbox_recompute_stateroot_maturitylatch_v5.go` |
| 2 | New `handoffPreState` step: anchors `EverMature` **and** `MatureEpoch` against `prevStateRoot` via the existing `anchorRotateScalar` primitive, hoisted ahead of the class-A dispatch | `floorbox_recompute_stateroot_maturitylatch_v5.go`, `floorbox_recompute_stateroot_v5.go` |
| 3 | `:176` branch selector → the anchored `pre.matureEpoch`; `:213` → `c.launchAnchorGiven(id, pre.handedOff)` | `floorbox_recompute_stateroot_atts_v5.go` |
| 4 | `launchAnchor(id)` split into `launchAnchorGiven(id, handedOff bool)`; the live path calls it with `c.handedOff()` — ONE function, two callers (#402 non-fork rule) | `chain.go` |
| 5 | Loud entry assertion that the injected bond verifier is wired (`R-VERIFYBOND-WIRING`, the #572 replay shape) | `floorbox_recompute_stateroot_v5.go` |
| 6 | Cold-box test tier + four driven D1 gates | `floorbox_recompute_coldbox_v5_test.go` |
| 7 | Corrected AST allowlist pin on `c.` selectors in the fold files | `floorbox_recompute_foldlivestate_pin_v5_test.go` |
| 8 | Carrier-reflection rows for `StateRootMaturityWitness` | `floorbox_recompute_adversarialroot_v5_test.go` |

`handoffPreState` also carries the certified defensive cross-check (Q3 step 4): a committed
pre-state of `matureEpoch=true ∧ everMature=false` is not one any honest `apply()` can commit
(`rotateEpoch:3395-3398`), so the box stalls. Stall-adding only.

`maturityLatchOps` consumes the already-anchored `preEverMature` rather than re-Resolving it —
one Resolve per scalar per block, not two.

## Options considered

| Option | Home for the anchored pre-`matureEpoch` | Verdict |
|---|---|---|
| A (chosen) | Class-M carrier `StateRootMaturityWitness`, required every block | Certified. The entry already requires the class-M witness on every block so the latch is never silently skipped, and the carrier already holds the sibling `EverMature` scalar. |
| B | Class-P `StateRootRotateWitness` | Rejected: the Rotate witness is nil off-boundary (`stateroot_v5.go:161`), and the class-A screen runs on every block. |
| C | A new top-level witness field | Rejected: a third home for the same handoff pair, when a required-every-block carrier for exactly that pair already exists. |
| D | Committed-format change (a new leaf) | Rejected and unnecessary: `tagMatureEpoch` is already committed under both roots. A format change would also be a STOP condition under this task's constraints. |

Dispatch-order options: hoist a handoff-pre-state step ahead of class A (chosen) vs. reorder
class M before class A. FoldOps are keyed by distinct leaves, so op assembly order is
irrelevant; only the data dependency constrains order. The hoist keeps the class dispatch
order (A → M → P) mirroring `apply()`'s order and keeps the anchor unconditional — it runs
even for a block with no attestations, so a forged `MatureEpoch.OldValue` can never be
suppressed by the absence of a class-A dispatch.

## Cost

One extra `statehash.Resolve` of one scalar leaf per block, plus one `O(log N)` scalar proof
in the witness bundle. The class-A cost class (`O(|atts|)` screen + `O(|validatorsSeen|)`
digest) is unchanged. This is a soundness fix, not a cost residual.

## Constraints held

- **STALL-ADDING ONLY.** Every new path returns an error; `Resolve` yields `NoWitness` ⇒ stall.
  No Accept path is added. `WitnessValidateV5` still returns
  `IndeterminateTrustlessly, ErrRecomputeGated`.
- **The `everMature`/`matureEpoch` one-way latch WRITE is untouched** (product-immutable
  corner 3). The fix reads the committed pre-value; `rotateEpoch:3398` and class P's emit are
  unchanged.
- **The #402 tally arithmetic is untouched**, and `launchAnchor` is parameterized rather than
  forked.
- **No committed/wire format change.** `tagMatureEpoch` is an existing committed leaf; only the
  witness carrier (an untrusted, non-committed transport struct) gains a field.
- **#620 rotate-LAST preserved.** No rotate op moves. The fix substitutes the committed
  PRE-state — precisely the value rotate-LAST guarantees `apply()` read.

## The cold-box tier (R-COLD-BOX-HARNESS)

Third occurrence of "the test shares the producer's blind spot" in this spine (R1.3 fold-caught
premise, class-P suppression, now live-state). The Tester's third-time rule applies: the
harness is a permanent tier, not a one-off test.

`coldBox(t, cfg)` builds a FRESH `New(cfg)` + `SetBondVerifier`, asserts no `apply()` has run
(`matureEpoch`/`everMature` false, the committed maps empty), and drives the real recompute
entry with only `(prevStateRoot, committedStateRoot, b, w)`. The existing class-A / class-M /
class-P gates are re-run under it, and the four D1 gates are driven on it.

## Residuals

- **R-HANDOFF-EPOCHS-OFF** — covered for free: `pre.handedOff` threads the anchored
  `preEverMature` when `EpochBlocks == 0`, rather than arguing unreachability.
- **R-VERIFYBOND-WIRING** — closed as a loud entry assertion (was: stall-only, fold-caught).
- **R-membership, external pass (R1.7), recovery boundary, legacy-mode** — unchanged R1.8
  preconditions.
