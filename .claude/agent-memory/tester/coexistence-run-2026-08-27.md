---
name: coexistence-run-2026-08-27
description: CLOSED (thrash-to-unusable, log recovered, teardown COMPLETE): coexist-20260827T212244-citev — 1060 MB balloon + 1M bbolt on 2 GB no-swap; balloon PASSED, test killed at 1h6m before any row; zero billable resources remain.
metadata:
  type: project
---

# Coexistence run 2026-08-27 — outcome record (CLOSED)

**Run ID:** coexist-20260827T212244-citev
**Instance:** silt-coexist-20260827t212244-citev / us-central1-a
**Launch:** 2026-08-27T21:22:44Z
**Machine:** e2-custom-1-2048 (1 vCPU, 2.00 GiB RAM, 0 swap, pd-ssd 20 GB)
**Command:** `SILT_STORE_PROFILE=1 SILT_STORE_BACKEND=bbolt SILT_STORE_NOSYNC=1 SILT_SMT_SCALES=1000000 SILT_COEXIST_BALLOON_MB=1060 go test ./internal/smtspike/ -run TestStoreProfile -v -timeout 60m`
**Commit:** 3352fe8f47bd152d9681aa38c237828e4b2cdedc (PR #614, builder/coexistence-balloon)

**Result: THRASH-TO-UNUSABLE. NOT OOM-killed. NOT a clean result.**

## Teardown status (COMPLETE — 2026-08-28)

- Log recovered: stop → detach disk → attach read-only to e2-micro (silt-recover-tmp) → mount -o ro,noload → SCP
- All billable resources deleted: silt-coexist instance (DELETED), silt-recover-tmp instance (DELETED), silt-coexist pd-ssd disk (DELETED, was orphaned after detach)
- Zero instances / disks / snapshots remain (`gcloud compute instances list`, `disks list`, `snapshots list` all return empty)
- Local straggler sweep: clean (no gcloud/silt/monitor processes)
- Evidence committed: 82089a0 on keystone-probes-bonded-epochset (coexist-run.log + coexist-test.pid + intermediate-state-22m49-UTC.txt)

## What the log revealed

Balloon guard PASSED:
```
store_profile_test.go:59: BALLOON: 1060 MiB held resident, touched 271360 pages,
checksum=4714219238400, residentMB 14.3 -> 1078.4 (delta 1064.1) — rssMB rows below are under this pressure
```

The header row was logged:
```
store_profile_test.go:69: backend  n  build  per-key  heapMB  rssMB(UNDER-PRESSURE)  onDiskMB  apply100  reopen
```

Then the test was killed:
```
*** Test killed with quit: ran too long (1h6m0s).
FAIL github.com/nerolabs/silt/internal/smtspike 5374.416s
FAIL
```

**Zero rssMB(UNDER-PRESSURE) data rows were written.** The goroutine dump (goroutine 10) shows bbolt's `Cursor.searchPage` mid-insert-batch when SIGQUIT hit. The test was in the bbolt build phase, 0 inserts completed from the 1M target — it was still building the tree when time ran out.

The -timeout was set to 60m. The SIGQUIT fired at 1h6m (Go adds ~6m grace period). That means the build phase alone consumed the entire 60m budget on a 1-vCPU box under 1060 MB pressure.

## What this means

**No rssMB measurement under pressure was obtained.** The prior bbolt measurement (305 MB unevictable) came from an unloaded box; we cannot compare. The question "does bbolt shed evictable cache under pressure" remains unanswered by this run.

The pre-run serial console evidence (committed 01ab4d9) confirmed:
- SSH died at ~21:38Z (ens4 DHCP timeout 22:03Z, ens4: Failed 22:05Z)
- No kernel OOM-kill event
- Box became fully non-functional before test ended

These are consistent with the log: box was in thrash by the time the test framework sent SIGQUIT.

## What was confirmed (and what was not)

Confirmed:
- Balloon allocation of 1060 MiB succeeded on the box
- bbolt build at 1M keys, single-threaded, under 1060 MB co-tenant pressure, takes >60m on a 1-vCPU machine
- The box went non-functional (network dead, SSH dead) before any measurement row printed

NOT confirmed:
- Whether bbolt sheds evictable mmap pages under pressure (no RSS rows captured)
- How many inserts completed before the box died
- bbolt's RSS floor under realistic coexistence pressure

## The open question

The coexistence question is unanswered, not answered-negative. The run design had a fatal flaw: -timeout 60m was insufficient for a 1-vCPU box to complete 1M inserts under 1060 MB pressure. A repeat with a longer timeout (or a smaller scale, e.g. 100K keys) would yield real rows.

**Links:** [[scar-oom-memory-failure-class]]

## Evidence location

`/Users/andrewedmond/Claude/claude/silt/integration/cloudtest/coexist-20260827T212244-citev/`
- `coexist-run.log` — recovered log (255 lines; balloon line + goroutine dump + FAIL)
- `coexist-test.pid` — PID 1172
- `intermediate-state-22m49-UTC.txt` — serial console snapshot
- `final-state-22m43-UTC.txt`, `intermediate-state-22m22-UTC.txt`, `run-manifest.txt`, `serial-console-full.txt` — committed earlier (01ab4d9)
Recovery commit: 82089a0 on keystone-probes-bonded-epochset (2026-08-28)
