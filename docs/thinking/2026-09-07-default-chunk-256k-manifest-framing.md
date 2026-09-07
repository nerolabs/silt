# 2026-09-07 — The publish default moves to 256 KiB and manifests are framed at true length (one content-addressing break)

**Context / trigger:** D-R2.9-NODE-HALF-CALLS call 4 (2026-09-06) ratified moving `pipeline.DefaultChunkSize` to
262,144 B; the Economist's advisory (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-default-chunk-size-256KiB-2026-09-06.md`)
corrected the premise (the 64 KiB default paid a repair-bounty base of 2, not zero — a shard is a whole ciphertext
chunk) and measured an unpriced cost of the move on the first production store: 87.6 % of objects are 1.4 KB
manifests padded to the chunk size, so at 256 KiB the store grows 3.9× for no content. Call 4′ (2026-09-07)
ratified doing both in ONE PR — one content-addressing break — before the economy-on flip.

**Evidence:** `core/pipeline/pipeline.go:179` split the sealed manifest at `opts.ChunkSize` (manifests carry no
parity, so the padding bought nothing); `core/chunk/chunk.go` `Join` accepts frames of any size ≥ `MinChunkSize`
and requires only that non-final frames be full relative to their own length, so a single true-length manifest
frame round-trips unchanged; data frames must stay equal-length within a stripe (erasure), so a small FILE still
pays the padding to the chunk size; `core/genesis/genesis.go:57` pins its own 64 KiB for reproducibility and is
untouched; `RepairBountyBase(10, 262,144 + 16) = 10` exactly (the certified D-S7 threshold of 36 retrievals per
repair), vs 2 of an exact 2.5 at 64 KiB; the PoR audit samples every block at 256 KiB (p = 1.0) and 0.76 % of them
at the old comment's "64 MiB production minimum" (unenforced folklore — 29 holders per 1 GiB object vs 6,557).

**Options weighed:**
- **(A) Move the default alone.** Restores the D-S7 threshold; grows the flixz store 3.9× in padded manifests.
- **(B) Move the default AND frame manifests at true length, one PR.** Both change the root a NEW publish of the
  same bytes produces (convergent dedup does not span the boundary), so crossing once is the whole point.
  Ratified (4′).
- **(C) Frame data files at true length too.** Refuted: erasure shards must be equal-length within a stripe.
  Small files pay the padding; disclosed, with the Economist's `store.paddingRatio` telemetry as the follow-up.

**Decision:** (B). `ManifestFrameSize(blobLen, chunkSize) = min(blobLen + HeaderSize, chunkSize)`; the default
pinned to the delivery increment in `cmd/silt` (`TestDefaultChunkIsOneDeliveryCredit`); the framing gated
(`TestManifestIsFramedAtTrueLength`, ablation: split at the chunk size ⇒ a 1.4 KB manifest frames at 262,144).

**What this does to existing content — the merge is the owner's.** Already-published objects keep their roots
(their chunks are stored). A NEW publish of bytes already published produces a DIFFERENT root on either side of
the boundary; `R-MANIFEST-PADDING` and `R-BOUNTY-TRUNCATION` close (the truncation at the new default is 0.006 %).
The first production user's re-publish and link behaviour across the boundary is an outward-facing consequence:
the PR is built and reviewed, and merged only on the owner's go.
