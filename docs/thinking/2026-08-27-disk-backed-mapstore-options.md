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
