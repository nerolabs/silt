# Implementation plan — bound the PoR proof map to O(hot) (the daemon OOM fix)

**Date:** 2026-08-17 · **Trigger:** the proof-map OOM (flixz report → PE triage →
field corroboration: it crash-looped the whole MATURING cohort). Now on the
critical path — `proof-map fix + harness OOM detection → clean MATURING re-run →
red team #183`. Harness OOM detection is DONE (PR #463, `scan_node_liveness`). This
plan is the other half.

**Design authority:**
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/silt-proof-oom-triage-PE-2026-08-17.md`
(the spec) +
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/proof-oom-field-corroboration-PE-2026-08-17.md`
(critical-path + memory-scaling wall). This plan grounds that spec in the actual
code and corrects one under-specification.

## The bug (confirmed at HEAD)

`Node.proofs map[ChunkID]StorageProof` is fully resident, one entry per held
chunk, never evicted (`node.go:456`; `LoadProofs` loads the whole store at startup
`:727-737`; store path pins `:1065/:1079`). `StorageProof` carries `Path []Hash` +
`PorTags [][]byte` (`ports/net.go:168`) ≈ 5.4 KB/proof → steady-state RAM is
**O(total held chunks)**. `cachestore` bounds chunk *data*; there is no analogue
for proofs.

## THE DESIGN CORRECTION (verified — the PE spec's key-set is not enough)

The triage says: replace `n.proofs` with a **key-set** `map[ChunkID]struct{}` +
bounded hot-proof cache. But **two iterate-all sites and re-announce need proof
CONTENT, not just existence** — verified:
- `capacity.go:60 HeldRoots`: `out[p.Root]++` — needs `Root`.
- `denylist.go:64 EnforceDenylist`: `n.isDenied(p.Root)` — needs `Root`.
- `LoadProofs`/`AnnounceHeld` re-announce under the column key `hash(root‖Column)`
  — needs `Root` + `Column` (`node.go:718-726`, `net.go:173-177`).

A bare key-set would force these to page every proof → defeats the fix. **So the
resident structure is a TINY-METADATA set, not a key-set:**

```
type proofMeta struct { Root ports.Hash; Index, Total, Column int } // ~56 B + map overhead ≈ 80-100 B
n.proofMeta  map[ChunkID]proofMeta   // resident: the small fields every iterate-all/announce/denylist/existence site needs
n.proofCache *proofcache.Store       // bounded, over diskproofs: the BIG fields (Path + PorTags), paged for serve/audit only
```

Split of `StorageProof`: **small/resident** = `Root, Index, Total, Column`;
**big/paged** = `Path, PorTags`. Memory: ~80-100 B/chunk resident vs ~5.4 KB full
→ ~55-70× smaller (matches the PE's ~70×), restoring O(hot) + a bounded hot cache.

## The build (5 steps, failing-first memory wall first)

**0. Failing-first memory-scaling regression** (the wall, write it RED first):
store N chunks (N large), assert resident proof RAM stays **O(hot), not O(N)** —
model on `cachestore`'s budget test. RED against today's full-map; GREEN after.

**1. `ports.ProofStore`** (currently Put/Delete/Load-whole, `ports/ports.go:100`):
add `Get(id) (StorageProof, bool, error)` + `Keys() ([]ChunkID, error)`. Keep
Put/Delete; Load can stay (compat) or retire once LoadProofs uses Keys+Get.

**2. `diskproofs`** (per-id layout confirmed: `<root>/<hex[:2]>/<hex>`,
`diskproofs.go:46`): `Get` = one file read; `Keys` = walk the two-level tree. Both
cheap. Add to the ProofStore conformance suite (V2 — one suite per seam).

**3. `adapters/proofcache`** — mirror `cachestore` exactly: byte-budget,
LRU/heat-evicted RAM cache over a `ProofStore`; `Get` fills on miss; `Put`
write-through **without warming** (the `cachestore.TestPutDoesNotWarmCache`
scan-resistance property — a bulk publish must not blow the cache).

**4. `node.go` refactor** (14 access sites mapped):
- field: `n.proofs` → `n.proofMeta` (resident tiny) + `n.proofCache` (bounded).
- **store** (`:1065/:1079`): set `proofMeta`, `diskproofs.Put(full)`, cache
  write-through-no-warm — do NOT pin hot.
- **LoadProofs** (`:727`): populate `proofMeta` from `Keys()` + `Get()` per chunk
  (bounded RAM). ⚠️ v1 startup reads N proofs to extract metadata (O(N) I/O, NOT
  O(N) RAM — the OOM is fixed); a compact metadata sidecar is the noted fast-follow
  to cut startup I/O. Flag in the PR.
- **serve/audit** (`:1112` serve, `por.go:133` audit, `repair.go:82`): read the
  full proof via `proofCache.Get` (pages from diskproofs on miss — same tradeoff
  as serving a cold chunk).
- **existence** (`:1123` "of course I have it"): check `proofMeta` (resident).
- **iterate-all** (`capacity.go:60 HeldRoots`, `denylist.go:64 EnforceDenylist`):
  iterate `proofMeta` (Root available — no paging).
- **delete** (`demand.go:72`, `node.go:747`): drop from `proofMeta` + `diskproofs`
  + cache.
- **demand/por lookups** (`demand.go:93`, `por.go:133`): the ones needing full
  content page via the cache; the ones needing only Root use `proofMeta`.

**5. Tests** (V5/V2): the step-0 memory wall (GREEN now); ProofStore conformance
(Get/Keys/Put/Delete round-trip); behavioral — a served/audited COLD chunk pages
its proof and answers **identically** to the pre-change resident path (proof
content + verification unchanged — this is a *where-it-lives* change, not a
security-rule change; I1–I5 / PoR verification untouched).

## Scope / risk

- **Silt-team, node + adapter only.** No consensus, no PoR verification logic, no
  security rule — the proof content and its audit are byte-identical; only where it
  RESIDES changes. Independent of the red-team track (verified).
- **Delicate because it's 14 sites on the PoR/audit path** — hence the behavioral
  test asserting identical answers, and the memory wall asserting the fix holds.
- **Deliberately NOT fatigue-built at the tail of the 2026-08-17 marathon session**
  (a security-adjacent 14-site refactor deserves fresh focus). This plan is the
  pace-before-code artifact so the build starts correct — the design (incl. the
  resident-metadata correction) is settled; the build is mechanical from here.

## Then

Clean MATURING re-run on the fixed build: expect `infra-node-liveness` PASS (no
OOM), the 2 chaos FAILs + 10b clincher resolved, and **re-derive the computed
bounds** (220s/360s may TIGHTEN — the PE flagged they were OOM-inflated). Then
#183.
