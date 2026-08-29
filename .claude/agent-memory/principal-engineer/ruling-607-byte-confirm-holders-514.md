---
name: ruling-607-byte-confirm-holders-514
description: PR #607 byte-confirm swarm holders — SHIP; root-cause fix for #514 repair-bounty flake, ungated diagnostic read, ablation red proven by me
metadata:
  type: project
---

Ruling: PR #607 (`fix(#514): byte-confirm swarm holders`, head `65e81db`) — **SHIP**.

Full ruling: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-607-byte-confirm-holders-514-2026-08-28.md`

**The bug (verified in code):** the kill-selector `columnHoldersEntry` returned raw DHT
provider records unfiltered (`core/node/file.go:873-874`); the repair judgment
`probeShard` byte-confirms each record with `MsgHasChunk` before counting it
(`core/node/repair.go:464,505-508`, comment "a bare provider record isn't trusted"
`repair.go:446`). Split → selector kills record-holders while a real byte copy survives →
caretaker's byte sweep sees missing ≤ slack → bounty never arms → ~20% e2e premise flake.

**The fix:** `confirmColumnHolders` filters providers by `MsgHasChunk` (answered from
`store.Has`, `node.go:1551`), reusing the repair sweep's mechanism. Selector now agrees
with the caretaker's view.

**What I verified myself:** ran the unit reproducer (PASS with fix); ablated
`confirmColumnHolders` to `done(provs)` → RED on the phantom-holder assertion (both REAL
`0x02` and phantom `0x03` listed) — honest reproducer, not decoration. Full `core/node`
green. Only production caller of `ColumnHolders` is `swarm holders`
(`cmd/silt/swarm.go:174`) — a read-only ephemeral CLI command, NOT hot/consensus/repair
path. Ungated: no I1–I5, no bounty/escrow/skim math (the "reconstruction PAYS" invariant
is untouched — only the test premise arming is fixed), no M0/C1/C2.

**Couplings/residuals I named (builder missed):**
- CHANGELOG/PR overstate blast radius ("cloud economy selector uses this selector") —
  false, only `swarm holders` calls it. Fix the narrative.
- Corpse-gating asymmetry: `probeShard` corpse-gates, `confirmColumnHolders` does not.
  SAFE — fresh ephemeral node has no dial-storm to gate, and any divergence kills MORE
  (real byte-holders), never fewer. Not a defeater.
- 3 TOCTOU timing residuals (confirm-then-kill gap, repair-during-window, confirm-side
  timeout) survive — absorbed by the 30-sweep premise window; the retained premise
  fast-fail is now the regression signal for all three.
- **Future-coupling flag:** `confirmColumnHolders` and `probeShard` both assume
  `MsgHasChunk == store.Has`. The `n.liar`/`chunkDenied` truth-benders exist in the
  handler already. If the proof-of-retrieval seam ever makes `MsgHasChunk` answer from a
  proof-cache, the view-split re-opens. Keep the two coupled or re-audit both.

**Did NOT run the e2e** (needs full harness) — 30/30 green is the builder's claim,
un-reproduced by me. SHIP rests on mechanism + unit ablation, not the e2e count. Tester
owns the stress pass before merge.
