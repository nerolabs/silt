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

**Blind PE ruling on this record**
(`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-parity-fetch-per-stripe-design-2026-09-06.md`,
PROCEED-WITH-CHANGES — "direction right, option wrong"). Built as **(A′), not (A)**:
- The record rejected (B) on a false premise: `fetchCols` already walks parity columns SEQUENTIALLY,
  one lookup each, and `fetchColumn`'s per-id `missing` list was thrown away. A DEFICIT COUNTER on that
  same walk — per stripe, deficit = real data shards − present; pull from each parity column only the
  shards of stripes still in deficit; decrement as they land; stop at the first column that clears every
  deficit — costs at most the same N−K lookups, typically ONE, and zero extra round trips. Measured by
  the PE: (A) over-fetches 6× per damaged stripe and leaves a 1.5× adversarial ceiling (withhold one
  chunk per stripe); (A′) is 1.0× for every withholding strategy — the fetcher draws exactly K shards per
  stripe by construction.
- `parityForMissing` already existed — a verbatim (A) — wired only inside `if m.K == 0`, where it returns
  nil: vacuous unconditionally. Deleted; the uncoded path is behaviourally unchanged (G-PS-7).
- **The pin's citation moves with the code.** `D-R2.9a-RUN-CALLS` named `core/node/file.go:750-760` as
  the decisive artifact for the 44.7 GiB floor's N/K factor; this build rewrites those lines. The floor's
  factor SURVIVES by a mechanism the certification did not name: `fetchFrom` transfers the bytes and
  verifies AFTER — a CORRUPTING provider forces `S·N/K` under any fetch policy including (A′). The
  ledger's citation is corrected to say so; the pin's VALUE is untouched.
- **Not claimed:** whether `R-PARITY-AMPLIFICATION` is DISCHARGED is research-gated, then the owner's. This
  build changes what an honest or withholding provider can make the fetcher draw; it does not close the
  corrupting case, and it does not touch the 64 GiB pin.
- **Filed, not fixed:** `R-SPARSE-COLUMN-PROVIDER` — `NetGetRetain` is live, and per-stripe parity makes a
  retainer a 1-of-T holder of a parity column; `probeShard` walks providers sequentially and corpse gating
  does not skip live nodes. #500 is not changed here.
- Gates G-PS-1…6 built on the existing 80 KiB / 4 KiB rig (21 chunks, 3 stripes, final stripe = 1 real data
  shard); G-PS-7 is the unchanged uncoded suite. Ablation run in a scratch copy: reverting the walk to
  whole-column parity turns four of the five behavioural gates RED (the healthy-object pin holds under
  both, by design). Two node-wide counters added for the gates: `Stats.ParityColumnLookups`,
  `Stats.ParityShardsPulled` (withheld with the other counters under `-privacy`).

**Status:** built; whole `core/node` suite (non-`-short`) and full e2e green locally; to blind PE code review.
