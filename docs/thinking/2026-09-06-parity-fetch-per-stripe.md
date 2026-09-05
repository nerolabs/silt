# 2026-09-06 — R-PARITY-AMPLIFICATION: fetch parity per STRIPE, not per whole object

- **Seat:** BUILDER (overnight, autonomous) · **Branch:** `builder/r-parity-amplification-per-stripe`
- **Base:** `origin/main` = `0348108`
- **Residual:** `R-PARITY-AMPLIFICATION` (Researcher, G-BB-19 certification 2026-09-05 §6): *"One missing
  data chunk makes the fetcher pull EVERY parity column of the WHOLE object … a 1.6× draw amplification
  triggerable by one withheld chunk. The corrected floor absorbs one round; it does not absorb repeats.
  Closable only by making the parity fetch per-stripe — a real and probably cheap fix, and a Builder
  question, not a pin question."* It is also the fact that turned the owner's 32 GiB into 44.7 GiB.

**Context / trigger.** `core/node/file.go` NetGet, coded path: fetch the K data columns; if `allData()` is
false — ANY data chunk of ANY stripe missing — fetch ALL N−K parity columns (`fetchCols(parityCols, …)`),
i.e. every parity shard of every stripe, then let the pipeline reconstruct. One withheld chunk in a
1,000-stripe object costs the fetcher (N−K)/K = 60 % of the object again, from the parity providers.

**Evidence (per build-immutable #7):**
- `core/node/file.go` NetGet: `allData()` walks `m.ChunkIDs()` (all data leaves) and the parity fetch is
  `fetchCols(parityCols, finish)` over `columnsOf(m)` — whole columns, all stripes.
- `columnsOf(m)` groups `m.Leaves()` by column IN STRIPE ORDER: `cols[j][s]` is stripe `s`'s shard in
  column `j`. `manifest.Leaves()` = all data chunk ids in file order, then all parity ids in stripe order;
  `columnAt(leafIdx, dataN, k, n)`: data leaf `i` → column `i % k`, stripe `i / k`; parity leaf `p` →
  column `k + p % (n−k)`, stripe `p / (n−k)`.
- `fetchColumn(root, col, ids, done)` resolves the column's providers ONCE (`colKey(root, col)`) and pulls
  each id in `ids` from them — it already takes an id LIST, so a per-stripe subset is the same call with
  fewer ids. The repair path (`fetchStripeByColumn`) already works per stripe.
- The pipeline reconstructs per stripe from any K of N shards (`core/erasure`, `DefaultParams K=10,
  N=16`), so parity is only ever USEFUL for the stripes that lost a data shard.

**Options weighed:**
- **(A) Per-stripe parity, same column lookups — RECOMMENDED.** After the data pass, compute the set of
  stripes with a missing data shard (`missing[s]` from `allData`'s walk, by `i / K`). If empty, finish.
  Otherwise, for each parity column `j ∈ [K, N)`, `fetchColumn(root, j, ids)` with `ids` = that column's
  shards for the missing stripes only. Same provider lookups (one per parity column), a strict subset of
  the bytes. Worst case (every stripe damaged) equals today's behaviour. No protocol or manifest change.
- **(B) Fetch exactly the number of parity shards needed per damaged stripe (K − present).** Fewer bytes
  still, but a second round trip when a chosen parity shard is itself missing, and it changes retrieval
  latency shape under loss (build-immutable #5: retry, don't multiply round trips on a lossy path).
  Rejected for this build; (A) already removes the amplification's object-size term.
- **(C) Leave it; raise the floor instead.** That is what the 44.7 GiB floor did — it prices the
  amplification into every honest grant. Rejected: the residual says it absorbs one round, not repeats.

**Decision + rationale:** (A). It changes the fetcher's per-server draw on a partially withheld object from
`S · N/K` to `S + (damaged stripes) · (N−K) · shard` — the object-size term of the amplification is gone
and a single withheld chunk costs one stripe's parity, not 60 % of the object. It is a fetcher-side
behaviour change only (what the fetcher ASKS for), so it changes no consensus rule, no manifest, no
placement, no ledger accounting; the serve path is untouched. The floor's `N/K` factor stays certified as
the WORST case (all stripes damaged) — this build does not re-open the 64 GiB pin, it makes the typical
case cheaper. Not research-gated (the Researcher named it a Builder question); a blind PE reviews.

**Gates planned (`core/node`, untagged):** with one data shard of one stripe withheld from a swarm, NetGet
completes bit-perfect AND the bytes pulled from parity providers equal that one stripe's parity shards
(not the whole parity columns); with no shard missing, no parity is fetched (today's behaviour, pinned);
with every stripe damaged, the fetch equals the whole-column fetch; the uncoded (`K == 0`) path is
unchanged. Existing: `TestNetGet*` in `netget_retention_500_test.go`, the e2e swarm retrieval and the
node-death e2e.

**Status:** proposed — to blind PE review before code.
