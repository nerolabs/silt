# Floor-box whole-set witness size measurement (2026-08-31)

**Gate-4 unfiled-measurement scar closure.**

The "~106 MB" estimate for the whole-bonded witness at 1M members was cited once
but never filed, so it could not ground the R-membership / pony-2GB budget. This
document files the durable, reproducible numbers.

## Background

A floor-box processing a block that touches the `bondedRoot` whole-set digest
(a bond-reg or slash block) must receive a `StateRootDigestWitness` for `bonded`.
That witness carries two parts (see `floorbox_recompute_stateroot_slash_v5.go`,
`StateRootDigestWitness`):

- `PreIDs []ports.NodeID` — the complete pre-state member list: **N × 32 bytes**.
- `Proof statehash.Witness` — one SMT inclusion proof for the digest leaf:
  `len(SideNodes) × 32 bytes` + small gob fixed overhead.

The cost is O(N) in the wire size and O(N) in the provider-side SMT build, NOT
O(payload). This is the R-cost-wholeset certification (class S P1-b, class B
P1-d).

## Measurement method

Test: `TestMeasureFloorBoxWholeSetWitnessSize` in
`core/chain/floorbox_wholeset_witness_size_measure_test.go` (this commit).

The test:
1. Generates N synthetic `NodeID`s (32-byte deterministic keys).
2. Builds a bonded sub-tree SMT: N `bonded||id` leaves + one `bondedRoot`
   digest leaf (via `statehash.NewProver`).
3. Proves the `bondedRoot` digest leaf (membership proof → `wit`).
4. Verifies the proof round-trips (`statehash.Resolve` returns `ProvenPresent`).
5. Builds a proxy `*smt.SparseMerkleProof` with the same sidenode count and
   calls `Marshal()` (gob) to get the exact wire-byte count.
6. Reports: `N × 32` (PreIDs) + marshal bytes (Proof) + heap metrics.

Why a proxy proof for marshaling: `statehash.Witness` wraps
`*smt.SparseMerkleProof` privately. The proxy has the same sidenode count and
SiblingData size (32 bytes), so its gob encoding is byte-equivalent in size.

## Results

Measured on: macOS Darwin 25.5.0, go1.26.5, commit `7d85a18`.

**Reproduce command:**

```
go test ./core/chain/ -run TestMeasureFloorBoxWholeSetWitnessSize -v -count=1 -timeout=600s
```

| N       | PreIDs (KB) | Proof (B) | Total (MB) | cumulAlloc (MB) | liveHeap (MB) |
|--------:|------------:|----------:|-----------:|----------------:|--------------:|
|  10,000 |       312.5 |       748 |      0.306 |            23.8 |          10.2 |
| 100,000 |     3,125.0 |       748 |      3.052 |           262.1 |          94.2 |
|1,000,000|    31,250.0 |       847 |     30.518 |         2,980.4 |         979.8 |

**Sidenode depths** (SHA-256 sparse trie — depth reflects actual key
distribution, not log₂(N) exactly):

- N=10,000: 18 sidenodes (log₂(10,000) ≈ 13)
- N=100,000: 18 sidenodes (log₂(100,000) ≈ 17)
- N=1,000,000: 21 sidenodes (log₂(1,000,000) ≈ 20)

SHA-256 key hashing produces a sparse trie; the sidenode count grows with the
number of shared path prefixes, not strictly with N. At 10k–1M the depth stays
in the 18–21 range (not 256, which is the trie height).

**Memory notes:**

- `cumulAlloc` = `TotalAlloc` delta (cumulative bytes allocated, includes
  GC-collected intermediates; NOT peak RSS).
- `liveHeap` = `HeapInuse` delta after GC (approximates retained live heap).
- The liveHeap at N=1M is ~980 MB. This is the **provider-side** cost (the full
  node that holds the committed set and issues the proof). The floor-box itself
  only holds the 30.5 MB wire bundle plus the two committed roots — a tiny
  fraction.

## Budget verdict

**Pony-2GB budget (N=1M): 30.5 MB wire size FITS with 2,017 MB headroom.**

The previously-cited "~106 MB" estimate was approximately 3.5× too high. The
dominating term is the PreIDs list (N × 32 bytes = 30.5 MB at N=1M); the SMT
proof is negligible (847 bytes = 8 sidenodes × 32 bytes + 175 bytes gob
overhead).

The wire budget for ONE digest witness (bonded) at N=1M is well within the 2 GB
pony limit. A block that touches all three digests (slashed + bonded + qualified)
sends three such witnesses; for a slash block at 1M membership the total wire
cost is ~3 × 30.5 MB = ~91.5 MB, still within budget. The 2 GB budget was set
for the worst-case full-registry scenario, and this measurement confirms it is
not binding.

## Open item (owed before backend lock)

The provider-side SMT build at N=1M uses **~980 MB live heap** (and ~3 GB
cumulative allocation). A coexistence test is owed: balloon ~1 GB to simulate
a flixz-sized daemon alongside the provider SMT build, confirm no OOM under
realistic memory pressure. This is the same owed test noted in
`MEMORY.md` (session-7, node-store coexistence).

## What this closes

This measurement closes the gate-4 unfiled-measurement scar:

- The "~106 MB" estimate is replaced by **30.5 MB** (N=1M, bonded digest, one
  touched digest per block).
- The R-membership / pony-2GB budget is now grounded in a filed, reproducible
  measurement, not an estimate.
- The measurement test is a permanent regression guard: it runs fast at N=100
  (structural check, always-on) and at N=10k/100k/1M under `-run
  TestMeasureFloorBoxWholeSetWitnessSize` (600s timeout, -short skips it).
