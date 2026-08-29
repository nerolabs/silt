---
name: billable-run-orchestration-playbook
description: LEARNING DOC (evolving, not immutable) for operating silt cloud runs under multi-agent orchestration — evergreen fleet-owner, default-PRESERVE, durable sink, EXTERNAL watchdog. Revised after every run's retro.
metadata:
  type: project
---

# Billable-Run Orchestration Playbook (silt cloudtest)

> **STATUS: LEARNING DOCUMENT — evolving tier, NOT an immutable.** Current best understanding; expect
> parts to be wrong. **Revise after every run's retro** (see Retros at the bottom). The only fixed
> thing is the goal: nothing is lost, every failure teaches. Version it in the commit log.

**Purpose.** Operate a local or billable cloud run so **nothing is lost** and every failure produces
evidence, not a torn-down fleet with the logs gone. Grounds in silt canon: the waste is HOURS not
dollars; runs cost pennies; capture evidence FIRST; a field run confirms, never discovers.

## The three lifetimes — keep them independent
An agent's death must take down none of the others.
1. **The fleet** — OS-detached, self-sufficient; survives the Tester/Planner dropping.
2. **The evidence** — durably committed to `integration/cloudtest/<run-id>/` (`git add -f`); NEVER only
   in an agent's context or a temp worktree.
3. **The evergreen Tester** — persistent observer/owner-of-record; **re-attachable** from the run-id +
   evidence dir. The run never depends on one agent's pulse. *(Validated 2026-08-27: the owner hit its
   turn limit mid-run; a replacement re-attached, captured evidence, preserved the box. The failsafe works.)*

## Roles — ONE owner of the fleet
- **Evergreen Tester** — the ONLY seat that touches the live fleet. Armed BEFORE launch; owns the
  preserved-fleet registry; keeps a tight live-state, offloads detail to artifacts.
- **PE / Researcher** — root-cause from banked artifacts, read-only. Never poke the fleet.
- **Builder** — fixes in an ISOLATED worktree; never edits the live harness mid-run.
- **Planner** — routes; owns the teardown gate + the in-run-fix-vs-deep-dive call. Does not execute.

## Before launch — arm, don't react late
- **Local proof first** (`RUN_LOCAL_PROOF` exits 0); the exact integration green locally. Model-check
  tier covering the regime green.
- **Branch first**; billable launch needs Andrew's UNAMBIGUOUS go-word.
- **★ EXTERNAL WALL-CLOCK WATCHDOG (mandatory for any memory-pressure run).** An in-process
  `go test -timeout` is NOT a watchdog under kernel memory starvation — the timeout goroutine can't be
  scheduled when the box thrashes (bitten 2026-08-27: `-timeout 60m` never fired; box thrashed 80+ min).
  Arm a SEPARATE OS-detached watchdog that issues `kill -9` (not SIGTERM) to the test at the wall-clock
  deadline, from a context that CANNOT be starved by the test's memory — i.e. from the local machine via
  `gcloud ssh`, not from inside the box.
- **Arm monitoring + verify-pid + nohup OS-detach** the run on the box (survives agent reaping).
- **Record the baseline:** run-id, seed, topology, steady-state RSS/heap. And budget realistically —
  **under memory pressure, op throughput can collapse ~1000×** (thrash), so the idle-box runtime estimate
  is worthless; size the watchdog deadline for the pressured case, not the idle case.

## DEFAULT ON FAILURE: PRESERVE, not teardown
Teardown is a deliberate, gated act AFTER extraction. "Idle box = pennies; a lost failure = hours."
1. **Freeze the moment** — snapshot to the durable sink the instant failure fires.
2. **Keep the patient alive** — disk-state failures: *stop* the instance (cheap). Memory/liveness
   failures (the silt-common class): keep the process FROZEN; do NOT SIGTERM (triggers teardown/cleanup);
   `kill -9` to strand; never let it be reaped/restarted — the live heap IS the evidence.
   - **Note:** a memory-pressure box may go NETWORK-DEAD (sshd can't fork at ~8 MB free; DHCP lease
     expires → NIC fails). Then the in-box nohup log is unreachable live but SURVIVES on the pd-ssd —
     recover it by **mounting the disk on a fresh instance** before teardown. Serial + gcloud metrics
     are the live evidence when SSH is dead.
3. **Register it** in the preserved-fleet registry (run-id → failure → still-needed?).

## The evidence manifest — what "ALL evidence" means for silt
Crash journals; per-node **LOG FILES** (not stdout); `results.jsonl` + `report.md`; RSS/heap telemetry;
`chain.cbor` + the `Regime()` save/restore lines; the RANDOMIZE seed; netem/topology; equivocation-island
state; **serial-console-full + `free -m`/`vmstat` history + `dmesg | grep -i oom`** for memory runs. Note
`report-<id>.md` stamps HEAD at report time — record the launch commit separately. *(Grows each retro.)*

## In-run fix vs bank-and-deep-dive — Planner classifies
Hot-apply IF isolated/reversible/no-gated-surface/validated-in-worktree (live scripts never mutated).
Bank-and-deep-dive IF gated surface, non-reproducible, or not obviously safe. Never re-run a known
failure to mask it.

## Teardown gate — Planner owns
Teardown ONLY when evidence banked + Tester confirms complete + deep-dive done + (for a memory run) the
in-box log recovered if it matters + Andrew's OK for direction changes.

## Rehearse before the first billable run — off the critical path
Dress rehearsal on a cheap/local run: evergreen Tester → arm → synthetic failure → freeze+preserve+bank →
kill the Tester and prove re-attach → teardown gate holds.

## Learn from it — two failure logs, and revise THIS doc
Capture silt failures AND orchestration failures. After each run, an orchestration retro → and the
retro's lessons EDIT this playbook. A rule that failed a real run is rewritten, not defended.

## Retros (learning log)
- **2026-08-27 — first billable run (coexistence #600 measurement).** Answered the qualitative question
  (floor box at 1060 MB coexistence pressure THRASHES to unusable — no OOM, bbolt pages evict+re-fault at
  ~1–5 inserts/sec — the no-pressure RSS does NOT predict production). Three learnings, now folded above:
  (1) **external wall-clock watchdog is mandatory** — the in-process `-timeout` never fired under thrash;
  (2) **pressure collapses throughput ~1000×** — budget the deadline for the pressured case;
  (3) **re-attach failsafe validated** — owner dropped, replacement captured evidence + preserved the box.
  Also: a memory-pressure box goes network-dead (sshd can't fork); recover the in-box log by mounting the
  pd-ssd on a fresh instance. Evidence: `integration/cloudtest/coexist-20260827T212244-citev/`.

## Deployment / source of truth
The GENERIC pattern → propagate to `../agent-orchestra/` as source of truth, deploy per
`deploy/silt/DEPLOY.md`; never fork by editing `silt/.claude/` directly. Related:
[[validated-verify-before-merge-and-measure-first]], [[planner-isolate-mutating-seats]],
[[standing-deep-run-loop]].
