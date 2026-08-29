---
name: ruling-keystone-node-store-backend
description: PE ruling — lock bbolt (not pebble) for the SMT node store, but ship-gate on the un-run coexistence test
metadata:
  type: project
---

Ruling filed 2026-08-27:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-keystone-node-store-backend-lock-2026-08-27.md`

**Verdict:** Lock **bbolt**, not pebble. But **backend choice ≠ ship gate** — the store does not
ship until the un-run coexistence test passes.

**Why bbolt (the crux):** On the axis that decides OOM survival — unevictable Go heap — bbolt and
pebble TIE at 1M (305 vs 304 MB). pebble's only edge (lower RSS) is kernel-evictable page cache,
not the deciding axis. With performance neutralized, pebble's supply-chain cost decides: I verified
`go mod graph` shows **41 modules reachable from pebble, a 12-module cockroachdb family**; bbolt's
runtime requirement reduces to `golang.org/x/sys`. Both already in go.mod (bbolt direct, pebble
indirect from the spike).

**The coupling the consult must not miss:** the 1M "no OOM" run had **NO competing daemon** and RSS
was measured under **NO memory pressure**. Production is bbolt beside ~1060 MB flixz on a 1976 MB
no-swap box. Survival depends on the kernel reclaiming bbolt's clean mmap'd pages — exactly what
was not tested. Backend = bbolt; ship = coexistence test passes (hold a ~1 GB balloon, drive
steady-state applies, PASS = no OOM + RSS sheds + apply within block budget).

**Premises I verified in code (`internal/smtspike/`):** batching is correct and one-fsync-per-block
(`boltstore_test.go:116-140`); root is byte-identical to the in-memory reference every block
(`boltstore_correctness_test.go:53-56`); RSS reads `/proc/self/statm` so it captures mmap page cache
`HeapAlloc` can't see (`rss_test.go:22-36`); pebble comparison is FAIR — WAL kept in both modes so
NoSync reopen is honest (`pebblestore_test.go:48-56`), a trap the builder caught, not skipped.

**Methodology caveats (neither flips it):** apply latency is best-case (warm cache, same process,
`store_profile_test.go:150-162`); single pd-ssd run, one sample per cell (pd-standard confound
correctly excluded).

**Not research-gated** — no I1–I5, no published claim, no economic/security parameter. Boundary
named: IF the SMT root becomes consensus-committed, store determinism becomes a correctness
property; the byte-identical-root test is the permanent gate for any backend swap.

**The one call left to Andrew (scope, trades against floor-box immutable #8):** must the floor box
hold a 1M tree beside a ~1 GB daemon at all, given the end-state is tree-free witness validation
(#600)? My rec: run the coexistence test at 1M ONCE before narrowing scope — narrow on a measured
FAIL, not on prediction.

**How to apply:** when the node-store build or the era-3 format freeze comes up, bbolt is the
locked backend but the coexistence test is an open owed gate. Related:
[[ruling-keystone-node-store-dependency]] (bbolt-behind-a-port, validate-by-proof — the prior ruling
this builds on).
