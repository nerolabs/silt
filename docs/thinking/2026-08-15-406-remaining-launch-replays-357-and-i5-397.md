# 2026-08-15 — #406: the two remaining launch-tier replays (#357 and I5/#397) to close the P1 gate

**Context / trigger:** Owner ruling — complete the model-check fully; no P1 confirm-run with unfinished
work. The P1 gate criterion (`consensus-model-check.md`): *"launch exhaustive tier green, with the
#357/#397/#402 failing-first replays proven."* Mapping current coverage → gaps, so I build the right
two oracles and can then honestly signal P1-ready.

**Coverage so far (merged):**
- **#402** → I1 launch oracle (`modelcheck_test.go`), failing-first vs the pre-#402 rule. ✅
- **B2** → I3 mature weight-quorum oracle (`modelcheck_i3_test.go`), failing-first vs head-count. ✅
  (This is the *handoff/MATURING* gate, ahead of schedule — not required for P1, but done.)
- **#397 restart face (I2)** → I2-across-restart oracle (`modelcheck_tier2_test.go`), failing-first by
  construction (non-persisted-mark control). ✅
- **#397 non-intersecting-launch-finality face (I1)** → the I1 launch oracle already asserts no two
  disjoint coalitions finalize, which *is* the #397 2-of-4 non-intersecting-finality property. ✅

**Gaps for the P1 launch gate — the two oracles to build:**

### 1. #357 launch replay — fork-choice reorg-of-final + determinism (chain-level, I5+I1)
- **The bug (357 cert §1):** zero-weight tiebreak degeneracy — during the drain window anchor
  attesters have `bonded=0`, so a committed 2-block chain has `Weight()≈0`; `heavier()` falls to a
  height-blind hash tiebreak, and a bare-genesis fork wins on hash luck → committed blocks reorged to
  height 0.
- **The oracle:** build a committed anchor-attested launch chain (nonzero bootstrap weight, finalized);
  present competing forks via `Reconcile` (bare-genesis, shorter, conflicting) — assert the committed
  head is **never reorged** (`ErrPreFinalityReorg` / heavier keeps it), and fork-choice is a **pure
  function** (same fork set, any Reconcile order → same head). Chain-level, like I1/I3.
- **Failing-first:** temporarily revert `blockWeight` to NOT credit anchor bootstrap weight (the pre-#357
  zero-weight state) → the committed chain weighs 0 → a genesis fork wins the tiebreak → reorg-to-0 →
  RED. (Controlled revert, same style as I1/I3.)
- **Reuse:** `forkchoice_ramp357_test.go`'s `commitRampChain` pattern (4 anchors, zero bonds, committed
  blocks, `Weight()>0`).

### 2. I5/#397 launch replay — honest-never-slashed under the cross-attest race (node-loop, tier 2)
- **The bug (#397 wedge):** two honest proposers race a height; each attests the other's block
  (cross-attest) because the proposer's own signature wasn't in the never-sign-twice ledger; both forks
  commit; sync detects both as double-signers → **two honest anchors slashed.**
- **The shielding problem (established last increment):** in the *current* objective codebase, #402's
  I1 (derived ⌊A/2⌋+1) structurally prevents two forks committing at a height, so the honest-slash
  can't be reproduced — the #397 fix is shielded by #402. So a genuine, failing-first #397 catch needs
  a config where **two forks CAN both commit**, making the #397 propose-watermark the *sole* protection.
- **The baseline:** the derived ⌊A/2⌋+1 only applies in OBJECTIVE mode; **legacy (non-objective) mode
  uses the configured `AnchorQuorum` and has no finality gate** (`finalityQuorumActive` requires
  objective). So a legacy config with `AnchorQuorum=1` + a low quorum lets two disjoint 2-anchor forks
  both commit — the pre-#402 environment. The #397 propose-watermark (`recordSign` on propose, in
  `proposeBlock`) is **mode-independent**, so it is exactly the thing under test. Assert: no honest node
  slashed after the cross-attest race + sync.
- **Failing-first:** revert the propose-time `recordSign` (the #397 fix) → a proposer attests the rival
  → both forks commit → sync slashes both honest proposers → RED. With the fix → each refuses the rival
  → one commits → no slash → GREEN. (Same controlled revert I attempted before, now in a config where
  the fork is actually reachable — the fix for last increment's "shielded by #402" finding.)
- Built on the tier-2 held-delivery substrate (two proposers, cross-attest-first delivery order, then
  sync). Verify-first: confirm both forks DO commit under the revert before trusting the GREEN.

**Sequencing:** #357 first (chain-level, simpler, reuses ramp pattern), then I5/#397 (node-loop, needs
the legacy baseline). Each: verify-setup-first, then failing-first via controlled revert. When both are
green + failing-first-proven, the launch tier has all of #357/#397/#402 → **P1 gate criterion met**;
I signal ready then.

**What would change the plan:** if the legacy-baseline I5/#397 turns out not to reproduce the
both-commit fork even under the revert (verify-first will tell me), that's a real finding → I'd consult
research on the faithful #397 launch replay rather than force it. Flag to owner if so.

**Status:** planned; building #357 first.
