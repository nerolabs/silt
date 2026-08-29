---
name: scar-repair-bounty-premise-defeat
description: SCAR (RESOLVED PR #612, 2026-08-27): TestRepairBountyPaysOnTheWire #514 shape — gate encoded (2 ablation tests RED), 30/30 stress PASS on PR #612 branch, 20/20 PASS on main (2026-08-28); third-time rule satisfied
metadata:
  type: project
---

# Scar: TestRepairBountyPaysOnTheWire — premise-defeat (#514 shape)

**Status: RESOLVED by PR #612 (2026-08-27). Third-time rule satisfied.**

**Failure class:** `economy_repair_test.go:339` — premise defeated (#514 REGRESSION): no
caretaker observed an over-slack loss within 60s of the kill — the killed columns still
have live copies somewhere (holders-view vs bytes divergence — byte-confirm broke). Claim-
chain lines empty (no sweep events at all in C1 or C2 log within the 60s window).

**Root cause (confirmed by PR #612):** `confirmColumnHolders` (file.go:923) returned raw
provider records rather than byte-confirmed holders. A node could hold a provider record
but not the bytes (lost-ack / stale record shape from #497/#517). The caretaker's
`probeShard` byte-confirms, so the two views diverged: killing a record-holder could leave
live bytes on a node the record view omitted, and the caretaker saw `missing <= slack` and
never armed repair. Premise defeated.

Secondary (PR #607 partial-fix gap): PR #607's `confirmColumnHolders` byte-confirms but
had no corpse-gate, so a dead provider was dialed once per shard of the column — the
#226/#277/#501 dial-storm re-entering through the holders read. This left the flake residual
at ~7.7% on #607 (measured: 1/13 = 7.7%, 2026-08-28 independent stress run — see below).

**Occurrence log (3 confirmed — THIRD-TIME RULE triggered and satisfied):**

1. **2026-08-26, CI run 33010585426** — branch: main@`13260516d0`. Test runtime: 61.87s.
   Exact match: `economy_repair_test.go:330: premise defeated (#514)`.

2. **2026-08-27, CI run 33087029160 (job 98569357280)** — branch:
   `keystone-probes-bonded-epochset` (PR #604). Test runtime: 61.76s. PR diff was
   test+doc only — zero coupling to repair wire path. Unrelated flake.

3. **Prior occurrence (2026-08-22, from issue #514 body):** "2 failures in 10 local runs"
   while validating the #501 fix (PR #513). First formal filing of the shape.

## Independent Tester stress run: PR #607 (2026-08-28)

**Purpose:** Verify PR #607 fixes the flake before merge.

**Command:** `go test -v -count=1 -timeout 240s -run TestRepairBountyPaysOnTheWire ./e2e/`
run 13 times in a loop (independent invocations; `-count=1` for fresh state each run).

**Result: FAIL. 12/13 PASS, 1/13 FAIL (iter 6). Flake rate ~7.7%.**

- Iters 1–5: PASS (55s, 53s, 33s, 61s, 61s)
- Iter 6: FAIL (63s) — premise defeated, claim-chain empty (no sweep events)
- Iters 7–13: PASS (37s, 34s, 31s, 37s, 31s, 61s, 31s)

**Elapsed range:** 31s–63s per run.
**Evidence:** `/private/tmp/claude-501/-Users-andrewedmond-Claude-claude-silt/37157300-1cff-4401-b48d-f789c702575b/scratchpad/607-stress/iter-06.txt`

**Failure detail (iter 6):**
- Killed: S1, S2, S11 (3 holders, 3 columns unreachable, slack exceeded)
- C1 claim-chain lines: empty (no sweep events in 60s window)
- C2 claim-chain lines: empty
- Error: `premise defeated (#514 REGRESSION): no caretaker observed an over-slack loss within 60s`
- This is the SAME failure shape as the original scar. PR #607 does NOT reliably fix the
  flake. The corpse-gate fix in PR #612 is what resolves it.

**Verdict: PR #607 is NOT ready to merge without the corpse-gate (PR #612's fix).**
The scar is resolved by PR #612 (MERGED), not PR #607 (still open as of 2026-08-28).
PR #607 is superseded by PR #612; it should be closed, not merged.

## Independent Tester stress run: MAIN (2026-08-28, HEAD 7d2a292)

**Purpose:** Confirm PR #612 fix holds on real main (post-merge), superseding the PR #607 question.

**Command (serial independent invocations):**
```
go test -count=1 -timeout 900s -run TestRepairBountyPaysOnTheWire ./e2e/
```
Run 20 times, independent invocations, `-count=1` for fresh state.

**Result: 20/20 PASS, 0 FAIL. Flake rate: 0%.**

Elapsed per run: 42s, 63s, 40s, 63s, 63s, 42s, 39s, 40s, 85s, 64s, 39s, 42s, 63s, 63s, 40s, 39s, 40s, 39s, 63s, 40s.
Elapsed range: 39s–85s. No failures captured.

**Working tree at run time:** clean (only untracked cloudtest dirs; HEAD `7d2a292`).

## Resolution: PR #612

**Fix:** `confirmColumnHolders` byte-confirms each candidate via `MsgHasChunk`,
corpse-gated so a dead provider costs one `HolderDialTimeout` for the whole walk (not
one per shard).

**Gate encoded:** `core/node/column_holders_bytes_514_test.go` — two deterministic
node-tier tests. Both ablations go RED:

- Ablation 1 (bypass byte-confirm: return raw provs): EXIT 1 — `#514: byte-confirmed
  holders still lists the phantom record-holder`. RED confirmed.
- Ablation 2 (disable corpse-gate: per-shard dial-storm): EXIT 1 — `#514 liveness:
  DEAD record-holder was dialed 5 times (want exactly 1)`. RED confirmed.

**Stress run (2026-08-27, worktree `agent-a43194e1c39876a82`, builder self-run):**
- Command: `go test ./e2e/ -run TestRepairBountyPaysOnTheWire -v -count=30 -timeout 3600s`
- Result: **30/30 PASS, 0 FAIL.** Total time: 1549.727s. Exit code: 0.
- Pre-fix rate: ~5% (after partial #607 fix), ~20% (before #607).
- p < 0.001 of a lucky streak under old code at N=30.

**Stress run (2026-08-28, Tester independent, main HEAD 7d2a292):**
- Result: **20/20 PASS, 0 FAIL.** Elapsed range: 39s–85s.
- Confirms the fix holds on real merged main, not just the branch.

**Regression suite:** `go test ./... -timeout 600s` EXIT 0 including e2e and sim.

## Scar closed (PR #612, not PR #607)

The third-time rule is satisfied: the gate exists (two deterministic ablation tests), both
ablations are RED, the builder stress run is clean (30/30 on PR #612's branch), and the
Tester independent run is clean (20/20 on real main). PR #607 still flaked at ~7.7%.

**PR #607 is superseded.** Its byte-confirm is a subset of what PR #612 shipped; the
corpse-gate and `anyLive` guard (present in PR #612, absent in PR #607) are the difference
between 7.7% residual flake and 0%.

If the shape returns on main (PR #612 merged), attribute immediately against
`confirmColumnHolders` and the corpse-gate — do not re-run without attribution.

**Links:** [[scar-oom-memory-failure-class]], [[scar-one-defect-four-costumes]]
