---
name: scar-oom-memory-failure-class
description: SCAR — OOM/memory-thrash is silt's recurring failure class: #503 island storm, SMT OOM@2M keys, coexistence-test CONFIRMED thrash-to-unusable at 1060 MB pressure (2026-08-27 run). Count=3 real incidents.
metadata:
  type: project
---

# Scar: OOM/memory-thrash — silt's recurring failure class

**Failure class:** Memory exhaustion or memory-pressure thrash under load. Three confirmed
incidents; the coexistence test is now CONFIRMED (thrash-to-unusable result, not clean OOM).

## Incident 1: #503 — island bond-renewal storm

**Source:** `silt-reviews/research/research-outcome/503-island-bond-renewal-storm-RESEARCH-CERTIFICATION-*.md`;
session MEMORY.md entry "silt-503-island-bond-renewal-storm".

**Shape:** A partitioned island replays all bond renewals in one burst at re-join. The per-
renewal memory footprint (large bond proof in-flight + chain state) exhausted the box
before any renewal committed. The fix is TTL re-denomination of bond proof sizes — but the
research cert ruled Q2 TTL re-denomination CONTRAINDICATED (changes security parameter).
Confirmed field.

**Key lesson:** A storm of concurrent large-proof operations = OOM. The safety is not in the
proof size; it is in serializing/rate-limiting the burst.

## Incident 2: retain-from-checkpoint OOM (in-memory SMT at 2M keys)

**Source:** MEMORY.md session-7 entry — in-memory SMT spike (`internal/smtspike`) OOM-killed
at 2M keys on the measurement box.

**Shape:** `AllEntries` replay to rebuild the SMT in-process → heap exhausted. The node
store is mandatory disk-backed for any realistic key count.

**Key lesson:** "Rebuild on startup" is not free when state grows. The Q6 resolution:
PERSIST, don't rebuild (bbolt reopen 7ms vs 18-min rebuild at 1M; OOM at 2M).

## Incident 3: coexistence run CONFIRMED thrash-to-unusable (2026-08-27)

**Source:** `integration/cloudtest/coexist-20260827T212244-citev/` (committed 01ab4d9 on
keystone-probes-bonded-epochset branch). Tester re-attach at 22:43Z after owning agent ended
~77 min into run.

**Shape:** bbolt 1M-key insert + 1060 MB anonymous balloon on 2 GB no-swap e2-custom-1-2048.
- Balloon guard PASSED: 1060 MiB resident, delta 1064.1 MB confirmed (early in run)
- free -m snapshot mid-build: used=1927/1976, available=48 MB (still marginal)
- free -m snapshot later: used=1968/1976, available=8 MB (SEVERE)
- At 8 MB free: GuestAgentCorePlugin CRASHED (21:38Z), GuestTelemetryExtension CRASHED
  (21:40Z), SSH dead (sshd cannot fork), ens4 network DHCP timeout (22:03Z), ens4 FAILED
  (22:05Z)
- Serial frozen since 22:05Z; SSH unreachable via direct AND IAP tunnel at 22:43Z
- NO kernel OOM killer event in serial console (grep confirmed zero oom|killed|out of memory)
- The box does NOT OOM-kill the test process; it THRASHES: bbolt mmap pages (file-backed)
  are evicted by kernel but re-faulted by process on every insert, creating I/O thrash
- In-process `-timeout 60m` fired at 22:22Z (launch + 60m) but produced NO external signal
  visible in serial or via SSH — the timeout goroutine may not get scheduled under severe
  kernel memory starvation

**Confirmed finding:** Floor box at 1060 MB coexistence pressure = UNUSABLE. Not OOM-killed;
functionally dead. The 305 MB bbolt heap floor measured at no-pressure does NOT hold under
this load because the evictable page cache gets thrashed (re-faulted continuously).

**Status:** This gate is now COMPLETE. The result is definitive: bbolt at 1060 MB pressure
on a 2 GB no-swap box is NOT viable for production floor-box use without a memory budget
that accounts for this thrash regime. The PE/Planner must decide the remediation path before
locking the bbolt backend.

**Evidence path:** `/Users/andrewedmond/Claude/claude/silt/integration/cloudtest/coexist-20260827T212244-citev/`
- `run-manifest.txt` — launch spec and balloon guard confirmation
- `intermediate-state-22m22-UTC.txt` — 49 min state (owning agent's last capture)
- `serial-console-full.txt` — complete 1513-line serial log
- `final-state-22m43-UTC.txt` — this tester's final capture at 81 min

## New orchestration scar: external watchdog is MANDATORY

**Source:** Incident 3 directly; the in-process `-timeout` did not produce an externally
observable signal on a thrashing box.

**Shape:** In-process `go test -timeout` fires a goroutine. Under severe kernel memory
starvation, goroutine scheduling degrades to near-zero. Even if the goroutine fires, it
calls `t.Fail()` and exits the binary — but the exit produces no serial output and is not
observable until SSH is restored. The test framework's timeout is not a wall-clock watchdog.

**Required remediation (Tester mandate — encode before next billable run):**
An EXTERNAL wall-clock watchdog must be armed before any billable run that might hit memory
pressure. Options: a pre-launched `sleep <N>; gcloud compute ssh ... --command 'kill -9 <PID>'`
command OS-detached alongside the test, or a Cloud Scheduler job. The in-process timeout
is NOT a substitute.

## Pattern (all three incidents)

A correct operation on a small box without realistic co-resident load silently conceals
the failure mode that appears at scale or pressure. The common trap: measure at no-pressure,
assume it holds under pressure.

**What to watch:**
- Any "load all X into memory" pattern in core (AllEntries, AllBonds, full-epoch snapshots).
  What is X at 1M entries? At 10M? Does it fit the floor box WITH a co-resident daemon?
- Any background proof operation that can storm (concurrent bond renewals, concurrent shard
  fetches). Is the burst rate bounded?
- Any billable run without an EXTERNAL watchdog armed before launch.

**Links:** [[scar-one-defect-four-costumes]], [[scar-depth-war-lineage]]
