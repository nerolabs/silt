# 2026-08-27 — The coexistence balloon: making the floor-box RSS measurement mean something

**Trigger:** the floor-box store profile (`2026-08-27-disk-backed-mapstore-options.md`)
recorded bbolt at 1M keys as **heap 305 MB / RSS 1328 MB**, and closed with one owed gap:

> RSS was measured with **no memory pressure**. The decisive real-world question — does the
> store shed its evictable page cache fast enough to coexist with a ~1 GB flixz daemon
> without OOM — was **not** tested (it needs a balloon process holding ~1 GB during the run).

Until that gap is closed, the numbers answer nothing about coexistence. The heap-floor tie
(~305 MB) says both backends *should* survive beside a 1060 MB daemon under 1976 MB — but
"should" is doing all the work, because the 1328 MB RSS was recorded with the kernel under
no reason to reclaim the clean mmap'd page cache. This deliberation designs the instrument
that turns "should" into a measured number.

## The mechanism paragraph (attribute before you ship)

The failure is: **the recorded RSS over-counts the coexistence risk by exactly its evictable
portion, because** the profile ran on an otherwise-idle box, so the kernel never reclaimed
bbolt's clean file-backed page cache — 1328 MB is heap (305, unevictable) plus ~1023 MB of
cache that a real daemon's demand for physical RAM would push out. This change addresses that
**by** allocating a `SILT_COEXIST_BALLOON_MB`-sized anonymous buffer, writing every page to
fault it fully resident, and holding a live reference for the entire profile run. On a
no-swap box anonymous memory cannot be paged out, so the balloon genuinely competes for
physical RAM against bbolt's page cache. The RSS reported at each scale step then reveals the
real behavior: either bbolt's cache sheds toward the ~305 MB heap floor (coexistence holds,
bbolt is locked) or the box OOMs (bbolt's evictable footprint does not shed cleanly under
pressure, and pebble returns, per the prior doc's closing clause).

Evidence this is the right axis: the store-profile doc already separates unevictable heap
(305 MB, "the true floor") from "clean, file-backed, kernel-evictable page cache ... high
because the measurement ran with no memory pressure." The balloon supplies the missing
pressure.

## Options for the pressure source

**(A) Anonymous buffer, every page written, live package-level reference (chosen).**
A `[]byte` of `MB<<20`, striding every `os.Getpagesize()` bytes with a non-zero write so the
kernel faults in a real physical page per stride. A package-level var holds the reference so
GC never frees it. On a no-swap box these pages are pinned in RAM for the run. Simplest thing
that creates *real* pressure; no product code; no external process to orchestrate.

**(B) A second OS process ballooning ~1 GB.** Closer to the flixz-daemon shape, but needs
process orchestration inside a Go test, and the coexistence question is about *physical RAM
contention*, which anonymous pages in-process reproduce exactly. Rejected: more moving parts,
same physics.

**(C) `cgroup` memory limit + let the box OOM naturally.** Tests the kernel's OOM-killer path
faithfully but couples the harness to cgroup plumbing and root. Rejected for the spike; the
balloon already forces the reclaim-or-OOM decision.

## The trap this must not fall into

A `make([]byte, n)` that is never touched is **not resident** — Go zero-fills lazily via
demand paging, and an untouched buffer creates no pressure. A fake balloon is a false
measurement is a wrong #600 call. So the balloon MUST write every page and MUST prove it did:

- **Linux:** `residentMB()` (reads `/proc/self/statm`) jumps by ~balloon-size with the
  balloon on vs off. This is the load-bearing proof on the floor box.
- **Cross-platform (macOS dev box has no `/proc`):** compute a checksum over one byte per
  page after writing, and report the touched-page count. A dev-box run cannot show the RSS
  jump, but it can prove the pages were allocated and written — so the mechanism is verifiable
  before the billable run.

## What ships

1. `SILT_COEXIST_BALLOON_MB` env — 0/unset preserves current behavior byte-for-byte.
2. `inflateBalloon(mb)` — allocates, writes every page, returns (touchedPages, checksum),
   stores the buffer in a package-level `heldBalloon` so GC cannot reclaim it.
3. The profile logs a balloon banner (requested MB, touched pages, checksum, and the RSS
   delta the balloon itself caused) before the scale loop, and each scale row is measured
   with the balloon held — so RSS-under-pressure is directly comparable to the prior
   no-pressure table.
4. `TestBalloonResident` — a fast, always-on unit test proving `inflateBalloon` touches every
   page (checksum + count), and on Linux asserts the `residentMB()` jump. This is the
   failing-first proof that the balloon is real, catchable locally in seconds.

## The runs

- **RUN_LOCAL_PROOF** (functional, exits 0 on any box):
  `go test ./internal/smtspike/ -run TestBalloonResident -v`
  plus a small-scale profile smoke with the balloon on:
  `SILT_STORE_PROFILE=1 SILT_STORE_BACKEND=bbolt SILT_SMT_SCALES=1000 SILT_COEXIST_BALLOON_MB=64 go test ./internal/smtspike/ -run TestStoreProfile -v`

- **Billable floor-box invocation** (2 GB no-swap `e2-custom-1-2048`, bbolt, matching the
  prior methodology — NoSync path, per-scale isolation):
  `SILT_STORE_PROFILE=1 SILT_STORE_BACKEND=bbolt SILT_SMT_SCALES=1000000 SILT_COEXIST_BALLOON_MB=1060 go test ./internal/smtspike/ -run TestStoreProfile -v -timeout 60m`

  Balloon 1060 MB matches the flixz occupancy the prior doc cites (1060 MB of a 1909 MB box).
  Read: does the 1M-scale RSS row shed toward ~305 MB (coexistence holds) or does the box OOM
  (`OOM_LINES > 0` / kernel kills the process)?
