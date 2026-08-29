---
name: standing-deep-run-loop
description: The evergreen Tester's standing deep-run loop — CURRENTLY STOPPED after a git-reset hazard. How to discover/re-attach, and the fix required before restarting.
metadata:
  type: reference
---

The evergreen Tester runs a **standing local deep-run loop** — continuous depth-hammering to flush
silt's O(depth) / consensus bug tail (the single highest-leverage orchestra use, see
[[orchestra-leverage-and-loop-health]]). It is OS-detached and survives across Planner sessions.

## ⚠ STATUS 2026-08-27: STOPPED after a hazard — DO NOT restart the current design
The re-pointed loop implemented "track main" as **`git reset --hard origin/main` on the MAIN working
tree each cycle**. That silently reverts any uncommitted **tracked**-file modifications in the main
tree — a landmine for any main-tree work. **No damage occurred this time:** `.claude/` (all
agent-memory) and the `integration/cloudtest/*` evidence dirs are UNTRACKED, so `reset --hard` was a
no-op against them (verified: `git status` showed only `??` untracked, HEAD clean at 0a322c8). The
loop was stopped, all PIDs swept.
**FIX before ANY restart:** run the loop in an ISOLATED git clone (its own working dir), never touch
the main tree. Track main by fetching in the clone. Emit evidence to the durable sink via `git add -f`
from the clone. This is the [[planner-isolate-mutating-seats]] lesson applied to a standing loop.

## Discover it (when running)
Glob `integration/cloudtest/local-deep-*/` (newest = current): `loop.pid`, `results.jsonl`,
`console.log`, `FAILURE-<run-id>.txt` (written on failure; the loop continues, evidence stays).

## Re-attach / supervise (per the playbook)
Read the latest run-dir's `results.jsonl`; on a `FAILURE-*` manifest apply the
[[billable-run-orchestration-playbook]] (evidence already frozen → root-cause off it; in-run-fix vs
deep-dive; feed the scar to the MAIN-tree tester ledger; never re-run to mask). Stop via
`kill $(cat <run-dir>/loop.pid)` + a sweep by name.

## Baseline (normal)
`sim/TestConsensusMemoryGrowth`: `HeapInuse` grows **linearly ~26 KB/height** (prune off in-process).
Linear IS normal; **super-linear**, or a FAIL at heights that pass shorter, is a depth-war bug. The
O(depth) CI gate formalizes this bound so a regression fails CI, not just the loop.

## Durability note (separate concern)
`.claude/agent-memory/` is UNTRACKED (not committed / not on origin/main). It survives `reset --hard`
and session exit, but a `git clean -fdx` would delete it and it is NOT shared via version control /
a fresh clone. Worth deciding whether to commit agent-memory; not urgent.
