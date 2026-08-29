---
name: standing-job-local-deep-runs
description: STANDING JOB: continuous local deep sim loop — UPDATED 2026-08-27 to track origin/main; PID 80638, evidence in local-deep-20260827T204536, first two cycles PASS at 0a322c8 (#612 HEAD).
metadata:
  type: project
---

# Standing Job: Continuous local deep runs

**Purpose:** Flush silt's O(depth) / consensus bug tail (scar-depth-war-lineage) continuously
in the background. The depth-war bugs only emerge at real chain depth; inspection never finds
them; only hammered deep runs do.

## CURRENT launch state (2026-08-27 — re-pointed to track origin/main)

- **Loop PID:** 80638 (bash, nohup OS-detached)
- **Loop script:** `/private/tmp/claude-501/-Users-andrewedmond-Claude-claude-silt/0eff9e41-07b3-45ed-9a00-c0bec7678e82/scratchpad/deep-loop-main-tracking.sh`
- **Evidence dir:** `/Users/andrewedmond/Claude/claude/silt/integration/cloudtest/local-deep-20260827T204536/`
- **PID file:** `integration/cloudtest/local-deep-20260827T204536/loop.pid`
- **Console log:** `integration/cloudtest/local-deep-20260827T204536/deep-loop-console.log`
- **Results:** `integration/cloudtest/local-deep-20260827T204536/results.jsonl`
- **Tracking:** `origin/main` — each cycle runs `git fetch origin && git reset --hard origin/main` before testing
- **First cycle HEAD:** `0a322c8c83e80ad5ac12ce68947a36e74f7faaff` (PR #612, fix/repair-bounty-premise)

## Prior (stale) loop — STOPPED

- Old PID 70976 — stopped 2026-08-27 (was pinned to `9bfe8e2`, stale)
- Old evidence: `integration/cloudtest/local-deep-20260827T223411/` — 16 PASS runs archived

## Re-attach procedure

```bash
# Check if loop is still alive
cat /Users/andrewedmond/Claude/claude/silt/integration/cloudtest/local-deep-20260827T204536/loop.pid
ps aux | grep deep-loop | grep -v grep

# Read current progress
tail -30 /Users/andrewedmond/Claude/claude/silt/integration/cloudtest/local-deep-20260827T204536/deep-loop-console.log

# Check for failures (presence of FAILURE-* files means a bug was caught)
ls /Users/andrewedmond/Claude/claude/silt/integration/cloudtest/local-deep-20260827T204536/FAILURE-* 2>/dev/null || echo "no failures yet"

# Read results
cat /Users/andrewedmond/Claude/claude/silt/integration/cloudtest/local-deep-20260827T204536/results.jsonl
```

## First cycle results (PASS, 2026-08-27 against 0a322c8 = #612)

| Run | Type | Height | HeapInuse | Status |
|-----|------|--------|-----------|--------|
| r0001 | deep-height | 200 | 6.3 MiB | PASS |
| r0002 | deep-height | 500 | 13.5 MiB | PASS |

(r0003 suite-race in flight when this record was written)

## Baseline (established 2026-08-27, main@9bfe8e2 — prior loop)

| Height | HeapInuse (MiB) | HeapObjects | Chain len |
|--------|-----------------|-------------|-----------|
| 0      | 1.1             | 1,750       | 1         |
| 200    | 6.3             | 48,822      | 201       |
| 500    | 13.6            | 119,021     | 501       |
| 1000   | 26.4            | 236,057     | 1,001     |
| 2000   | 50.5            | 470,104     | 2,001     |

HeapInuse grows linearly with height (~26 KB/height). Prune is NOT engaged in the sim.

## Harness

**Primary:** `sim/TestConsensusMemoryGrowth` (opt-in: `SILT_OOM_DIAG=1`)
- Drives `heights` real commits through the sim consensus loop
- Seeds VARIED per cycle (11, 17, 31, 42, 99, 7, 53, 23 cycling)
- Heights cycling: 200 → 500 → 1000 → 2000

**Secondary:** `go test ./sim ./core/chain -race -count=1` every 3rd run
- Catches data races and correctness bugs the OOM diagnostic misses

## Failure protocol (DEFAULT-PRESERVE)

On any PASS→FAIL transition:
1. Loop writes `FAILURE-<run-id>.txt` manifest to the evidence dir — DO NOT delete it.
2. Do NOT SIGTERM the loop — it continues running on the next seed.
3. `git add -f integration/cloudtest/local-deep-20260827T204536/` to bank the failure evidence.
4. Notify the Builder + Planner with the FAILURE manifest path and reproduction command.

For memory/OOM failures: use `kill -9` if you must stop something; NEVER SIGTERM.

## Loop schedule

- Run 1, 2 of every 3: `deep-height` at heights [200, 500, 1000, 2000] cycling, varied seeds
- Run 3 of every 3: `suite-race` (`go test ./sim ./core/chain -race`)
- Delay between runs: 2s
- Each cycle: `git fetch origin && git reset --hard origin/main` before test

**Links:** [[scar-depth-war-lineage]], [[class-gate-o-depth-review]]
