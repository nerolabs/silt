# 2026-08-27 — The disk-backed MapStore: four options, and the one that needs your call

**Context / trigger:** PR #596 disqualified the in-memory SMT backend *by kernel OOM* — the
floor box killed it at 2M keys (~1.68 GiB anon-rss), and flixz already occupies 1060 MB of
a 1909 MB box, so even 1M entries would not fit alongside a real daemon. The keystone
therefore needs a **disk-backed `kvstore.MapStore`**, and the certification's owed
measurement (boot rebuild) cannot be re-run until one exists — every timing number so far
came from an in-memory store, so `~22 s per 1M` is a **lower bound, not an estimate.**

**This is written before any code**, because the decision is a *dependency* decision and
silt's dependency profile is deliberately lean. Today, after adding the SMT:

```
github.com/fxamacker/cbor/v2      github.com/klauspost/reedsolomon
github.com/pokt-network/smt       golang.org/x/sys
(+ 2 indirect: cpuid, float16)
```

Six modules total. #596 counted "**zero new indirect dependencies**" as a point in the
SMT's favour. Whatever backs the node store is the seventh, and it will hold consensus-
critical state.

## What the store must actually do

`kvstore.MapStore` is five methods — `Get` / `Set` / `Delete` / `Len` / `ClearAll` — so the
interface is not the constraint. The workload is:

- **Measured shape (PR #596, floor box):** 2.24 nodes per key, **218 stored bytes per
  key**, keys are 32-byte hashes, values ~65–97 bytes. A 10M-entry registry is ~2.2 GB on
  disk and ~22M records.
- **Hot path:** ~4 ms/block for 100 changed keys at 100k state, which is `changed × ~2·log₂ n`
  random point reads plus the same order of writes, committed once per block.
- **Boot rebuild:** `O(state)` sequential writes — the number Q6's rebuild-vs-persist
  choice turns on.
- **The hard constraint:** it must do this inside what remains of a **2 GB box after a
  silt daemon is already running.** That is the constraint that killed the in-memory
  backend, and any candidate whose own cache/index scales with total keys re-creates it.

## Options

**(A) One file per node, sharded directory tree (the `diskstore` shape, no new dependency).**
Reuses a pattern already in the repo. But 22M records at 10M entries means **22M inodes**;
per-file overhead (a 4 KiB block minimum for a ~100-byte value) inflates 2.2 GB toward
~90 GB, and boot rebuild becomes 22M `create+fsync` calls. Cost is dominated by the
filesystem, not by silt. **Rejected on arithmetic** — this is the #299 shape (an elegant
artifact whose *production* cost blows the floor), and it does not need a spike to see.

**(B) An embedded pure-Go KV store (new dependency).**
The settled answer, and B8 says buy settled corners rather than re-derive them. Candidates
differ in exactly the axis that matters here — resident memory:
- **bbolt** — single-file B+tree, mmap'd, pure Go, no compaction, very stable API, widely
  deployed (etcd's original store). Read-heavy point lookups are its strength; memory is
  page-cache-backed rather than a heap index, which is the right property for a 2 GB box.
  Write amplification on random-key inserts is its known weakness, and SMT node keys are
  hashes — i.e. maximally random.
- **pebble / badger** — LSM-tree, built for write throughput, which suits the rebuild path
  better than bbolt. But both carry **memtables, block caches and bloom filters whose
  default footprints are sized for servers**, and badger in particular is memory-hungry.
  On a 2 GB box shared with a daemon, that is the same failure mode we just disqualified,
  arriving through a different door.

**(C) Append-only log + in-memory index.**
Fast writes, simple. But the index is `O(keys)` **in RAM** — which is precisely what the
kernel killed at 2M. **Rejected**: it re-creates the disqualified shape.

**(D) Write our own on-disk B-tree.**
Full control of the memory profile. Disqualified by **B8**: a storage engine is a settled
corner, the novelty budget belongs to M0, and "consensus is boring by policy" applies with
even more force to the thing consensus state is stored in.

## Recommendation, and its honest status

**(B), and specifically `bbolt` as the leading candidate** — pure Go, single file, mmap'd
so its residency is page cache the kernel can evict under pressure rather than heap the GC
must hold, and no background compaction competing with a validator for one vCPU.

**This recommendation is DOCUMENTARY.** It is the same epistemic position the SMT library
call was in before PR #596 — reasoning from properties, not from executed code. #596 is
the precedent for what to do about that: the library call looked right on paper and the
*spike* is what turned it into a decision, including one finding (the residency ceiling)
that no amount of reading would have produced.

So the gate, mirroring #596 exactly:

1. Add the dependency **behind a spike test only**, importable by nothing.
2. Measure on the **1 vCPU / 2 GB floor box**, with a silt daemon-sized process resident so
   the measurement reflects the real constraint rather than an empty machine:
   **boot rebuild** (the Q6 number), **hot-path apply**, **resident memory**, and **on-disk
   size** at 1M / 10M keys.
3. Compare against bbolt's known weakness — random-key insert amplification — because SMT
   keys are hashes. **If rebuild time is dominated by it, the LSM candidates return**, and
   the question becomes whether their caches can be configured small enough to fit.

If the spike disagrees with the reasoning above, the reasoning is void, exactly as the JMT
port would have returned had the SMT spike failed.

## The call that is yours

**Adding a seventh module that holds consensus-critical state is a project-level decision,
not a builder's.** The alternatives are all worse on the evidence — (A) and (C) are
disqualified by arithmetic, (D) by B8 — but "we now depend on an embedded database" is a
standing commitment, and #596 treated the lean profile as something worth counting.

The narrow question: **do you want the spike to proceed on `bbolt`, or should the LSM
candidates be measured alongside it in the same run?** Measuring two costs little extra —
the harness is the same and the floor box is pennies — and it would settle the
random-key-insert question with numbers instead of my reading of it.

---

## PE ruling absorbed + the bbolt spike built (2026-08-27)

The PE ruled (`RULING-keystone-node-store-dependency-2026-08-27.md`) and Andrew concurred:
take the dependency behind **`ports.NodeStore`**; **measure `bbolt` + one tuned LSM in one
floor-box run** rather than pick by reading; and — the load-bearing call — the **floor-box
validator validates BY PROOF (witnesses), the tree is a tier above** (recorded in
`docs/decisions.md`, opened as #600).

### What is built (local proof — LOCAL PROOF BEFORE BILLABLE)

`internal/smtspike/` now carries a disk-backed spike, test-only, importable by nothing:

- **`boltStore`** — a batching `kvstore.MapStore` over bbolt. The load-bearing design point
  is write batching: the SMT calls `Set()` once per dirty node during `Commit()`, so a naive
  one-txn-per-`Set` adapter would fsync per node and make the measurement meaningless.
  `Set()` buffers into a pending map; `Flush()` commits a whole block in one bbolt
  transaction (one fsync). `Get()` reads the pending buffer first, so read-your-writes holds
  within a block.
- **Correctness proven before cost** (`boltstore_correctness_test.go`): the disk-backed trie
  produces **byte-identical roots** to the in-memory reference across every block; a
  committed root survives **close + reopen** and still serves membership proofs (the Q6
  boot-rebuild property in miniature); a deleted key reads absent through the tombstone path.
- **The measurement** (`store_profile_test.go`, `SILT_STORE_PROFILE=1`): build, per-key,
  Go-heap, **RSS**, on-disk, hot-path apply, and reopen — per scale, isolated so an OOM at
  the top scale cannot destroy lower results, fsync on/off via `SILT_STORE_NOSYNC`.

### The measurement gap I nearly shipped

The first harness reported **Go heap only** (`runtime.MemStats.HeapAlloc`). That is the wrong
axis: bbolt is mmap'd, so its pages live in the OS page cache *outside* the Go heap —
invisible to `HeapAlloc`, and precisely the residency the PE flagged as the reason to prefer
bbolt (kernel-evictable) over an LSM's server-sized heap caches. Measuring heap alone would
have compared the two backends on the axis where they look similar and hidden the axis where
they differ. `residentMB()` (reads `/proc/self/statm` on the Linux floor box) is the fix —
the number that actually answers the OOM question this whole exercise exists for.

### New evidence that shifts the "measure both" call — pebble is 127 modules

The PE said measure bbolt + one tuned LSM in one run because "the box is pennies and the
harness is identical." True on compute. But the LSM candidate, **`cockroachdb/pebble`, pulls
in 127 modules** — versus bbolt's ~1 net. The PE did not have that number.

It changes the trade in two ways the ruling's reasoning would itself follow:

1. **The bbolt-alone floor-box run already answers the question the LSM was for.** The PE
   wanted the LSM measured to test whether bbolt's random-insert weakness (SMT keys are
   hashes) is binding. But the bbolt profile on the floor box *shows that directly* — if its
   build/apply cost with fsync is acceptable, there is nothing for a faster-writing LSM to
   buy, because bbolt already wins on residency (Q1's decisive axis). The LSM only matters
   **if bbolt's write cost proves unacceptable**, and the bbolt run is what reveals that.
2. **127 test-only modules in `go.sum` is not free** — it is supply-chain surface and review
   burden on a project that counts its lean profile (PR #596). Paying it speculatively, to
   confirm a prior the residency argument already favors, inverts the #596 discipline.

**So the recommended sequence, offered for Andrew's call:** run **bbolt alone** on the floor
box first. If every axis is acceptable, the backend is decided and the 127-module tree is
never added. **Only if bbolt's write cost is the binding constraint** do we add pebble and
measure the trade — evidence-driven, not prior-driven. This does not contradict the PE; it
applies the PE's own "measure, don't assume" to a cost the PE was not shown. If Andrew wants
both in one run regardless, pebble goes in behind the same harness — it is a half-day of
adapter, not a design problem.

### Gate status

- **Local proof: GREEN.** Correctness done; the floor-box run confirms *cost*, never
  correctness (per the standing rule).
- **The floor-box run is billable** and needs Andrew's explicit go — it is the same
  `e2-custom-1-2048` recipe as #596 (dedicated 1 vCPU / 2 GB, no swap, per-scale processes,
  delete + verify 0 instances/disks after).
- The one open sub-decision inside it: **bbolt-alone-first (recommended) vs bbolt+pebble
  together.**

---

## Floor-box results (2026-08-27) — measured, both backends, and the prior did NOT hold

Measured on a dedicated GCP `e2-custom-1-2048` (1 vCPU, 1976 MB, **no swap**), NoSync path,
one process per cell. A first instance used **pd-standard** (20 GB, ~15 IOPS) and proved
only that bbolt's random-key inserts at 1M **exceed 30 minutes** on a throttled disk — a
real signal for cheap VPSs, but not representative. The numbers below are from a **pd-ssd**
box (Andrew authorized the second instance), which is a fair hobbyist VPS.

| backend | n | build | heap MB | **RSS MB** | disk MB | apply/block | reopen |
|---|---|---|---|---|---|---|---|
| bbolt | 100k | 39.9 s | 37 | 187 | 48 | 35 ms | **0.58 ms** |
| pebble | 100k | 36.7 s | 46 | 165 | 32 | 32 ms | 49.6 ms |
| bbolt | 1M | 18.5 min | **305** | 1328 | 418 | 98 ms | **7.4 ms** |
| pebble | 1M | 22.5 min | **304** | 981 | 234 | 66 ms | 87 ms |

**No OOM at 1M on the 2 GB box for either backend** (`OOM_LINES: 0`) — both fit without a
competing daemon, unlike the in-memory backend which the kernel killed at 2M (#596).

### The reading that matters: separate unevictable heap from evictable page cache

- **Unevictable (Go heap): bbolt 305 MB, pebble 304 MB at 1M — a TIE.** This is the memory
  that *cannot* be reclaimed under pressure, so it is the true floor. Both are bounded and
  fine on a 2 GB box.
- **RSS (heap + touched mmap pages): bbolt 1328 MB, pebble 981 MB.** The excess over heap is
  **clean, file-backed, kernel-evictable** page cache — high because the measurement ran
  with no memory pressure, so the kernel never reclaimed it. Under the pressure of a
  coexisting daemon it sheds toward the heap floor. So pebble's ~350 MB RSS advantage is
  **mostly in reclaimable cache, not in the must-hold floor** where they tie.

This is exactly the axis the PE flagged, and it cuts the opposite way to a naive RSS read:
the tuned-pebble RSS win is real but largely evictable, so it does not move the true memory
floor. On the floor that matters, the two are even.

### What each backend actually wins

- **pebble:** smaller on disk (234 vs 418 MB), lower total RSS, faster hot-path apply
  (66 vs 98 ms/block). More space-efficient at scale.
- **bbolt:** **1 net dependency vs pebble's 127 modules** (permanent, disk-independent
  supply-chain surface), and **reopen in 7 ms vs 87 ms** — which matters because Q6 now
  resolves to *persist* (below).

### Q6 resolves decisively: PERSIST, do not rebuild-at-boot

The rebuild-with-per-block-flush build is **18–22 minutes at 1M** — a non-starter for
rebuild-at-boot. But **reopen (the persist path) is milliseconds** (bbolt 7 ms, pebble
87 ms). So the disk-backed store makes *persist* trivially the winner, resolving Q6 in its
favour — the opposite of the pure-in-memory world where the tree had to be rebuilt because
it could not be trusted from disk. (The tree is still a derived cache; a persisted tree is
cross-checked against the committed root on load, per the certification's Q6 — a mismatch
triggers a loud rebuild.)

### The honest remaining gap

RSS was measured with **no memory pressure**. The decisive real-world question — does the
store shed its evictable page cache fast enough to coexist with a ~1 GB flixz daemon
without OOM — was **not** tested (it needs a balloon process holding ~1 GB during the run).
The heap-floor tie (~305 MB) says both *should* survive, since 1060 + 305 + working set
fits under 1976 MB, but "should" is doing work there. That coexistence test is the one thing
still owed before the backend is committed.

### Recommendation (builder, for Andrew — the dependency is his call)

**Lean bbolt.** The memory difference that looked decisive is mostly evictable page cache;
on the unevictable floor the two tie at ~305 MB. Against that, pebble's **127-module
supply-chain surface is a large, permanent, disk-independent cost** on a project that counts
its lean profile, and bbolt's faster reopen serves the persist path Q6 now favours. pebble's
real wins (disk size, apply latency) are genuine but do not outweigh the dependency weight
once the memory picture is read correctly. This **confirms the PE's original instinct** —
but only after measurement separated evictable from unevictable memory, which is why the run
was worth doing rather than deciding by prior. If the coexistence test later shows bbolt's
evictable footprint does not shed cleanly under daemon pressure, pebble returns.
