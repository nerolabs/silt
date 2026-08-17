# Boundedness audit — every resident/accumulation path (the memory-safety discipline)

**Date:** 2026-08-18 · **Trigger:** PE ruling on the inbound-queue OOM — *"this is
the SECOND unbounded-resident-memory instance (proof map, then the inbound queue);
silt has no systematic memory-boundedness discipline."* This is the follow-through:
the memory analogue of the invariant-map/model-check discipline — name every
accumulation path and ask of each: **can a remote party (or unbounded honest load)
drive it without bound?** Every "no" needs a bound + a resident-memory-scaling
regression (RAM = f(hot/in-flight), never f(stored/received)).

**Tenet:** *bounded-then-fast* — resource boundedness under adversarial input is a
security floor (M0), efficiency is M1. Efficiency presumes boundedness.

## The audit (node accumulation surface)

| path | keyed/grows by | remote-drivable? | bounded today? | priority |
|---|---|---|---|---|
| **inbound message queue** (`eventloop.queue` via `tcpnet.readLoop`) | inbound decode rate | **YES** (any sender) | **FIXED v1** — `-inbound-cap` backpressure | done (v2: per-peer + priority) |
| **PoR proof map** (`proofMeta`+`proofCache`) | held chunks | via stored content | **FIXED #464** — O(hot) cache; meta O(N)×~120 B | done |
| `tokenIssued` | issued publish tokens | yes (token requests) | **YES** — `maxTokenIssued` cap + evict | ok |
| **`pendingEntries`** (mempool, `MsgSubmitEntry`) | distinct submitted roots | **YES** — but publish-token-gated (rate-limited) | **FIXED (A2)** — `maxMempool` cap, reject-when-full (preserves FIFO seniority) | done |
| **`pendingBondRegs`** (mempool, `MsgSubmitBondReg`) | distinct bond regs | yes — bond-gated | **FIXED (A2)** — same `maxMempool` cap (defense-in-depth; already validator-bounded) | done |
| **peer-keyed maps** — `peerBonds`, `peerCaps`, `peerDomains` | distinct NodeIDs gossiped | **YES** (sybil-ID flood) | **FIXED (A1)** — `maxPeerInfo` cap via `evictPeerInfoIfFull` (sweep-past-threshold idiom) | done |
| `peerIssuerKeys` | canonical issuers | via issuer-set | issuer-set-bounded (chain-committed validators) | ok |
| `pendingSlashes` | detected equivocations | via equivocation | bounded by real detections (each is signed proof) | low |
| **chain block history** (`chain.blocks`) | committed height | consensus-gated (block rate) | **retains ALL blocks** (~23 KiB/height in-sim; the field multiplier is bond-reg-laden blocks) | **MED** — needs pruning/snapshot eventually (unbounded over runtime) |
| bond plot (`diskplot.Load`) | own bond | no (self) | fixed per-validator (~63 MiB resident) | low — pageable |
| `pending`, `reachProbes` | in-flight own RPCs | self (our request rate) | bounded by our own outstanding + timeouts | ok |
| `serveLoad`, `leases` | held/leased chunks | via demand | bounded by held content + lease TTL | ok |

## The two the red team will likely reach first (candidate #183 findings)

1. **Peer-keyed maps under a sybil-ID flood.** `peerBonds`/`peerCaps`/`peerIssuerKeys`/
   `peerDomains` populate from gossip keyed by NodeID and (per the grep) are not
   pruned when a peer leaves the routing table. An adversary minting cheap NodeIDs and
   gossiping a bond/cap/issuer-key from each grows them without bound. Each entry is
   small, but millions of sybil IDs → hundreds of MB. **Bound:** evict alongside
   routing-table eviction (a peer we no longer route to, we no longer remember), or a
   hard cap keyed to `K × buckets`.
2. **Mempool pools (`pendingEntries`/`pendingBondRegs`) under sustained submit.** Both
   are publish/bond-token-gated (rate-limited), so not a raw flood — but if submit
   outruns per-block drain (`MaxEntryBytesPerBlock`/`MaxBondRegBytesPerBlock`) the pool
   grows without a hard ceiling. **Bound:** cap the pool by bytes; on overflow drop the
   oldest/lowest-priority (a dropped submission is re-sent by the client's retry loop —
   the #441 contract already tolerates loss).

## Method for the sweep (not just these findings)

For each path, add a **resident-memory-scaling regression** that drives the input
dimension and asserts RAM stays f(hot/in-flight/held), never f(received/submitted/
distinct-peers-seen). The proof-cache memory wall and the inbound-backpressure memory
wall are the templates. This turns "find the next OOM by profiling a crash" into
"prove no input path is unbounded" — the memory twin of the consensus invariant set.

## Staging

- **Before red-team #183:** the two candidate findings above (peer-map sybil-inflation,
  mempool pool caps) — they are remote-drivable and the red team floods.
- **Follow-on:** chain-history pruning/snapshotting (unbounded over runtime, but
  consensus-gated, not a fast flood); diskplot paging (efficiency, not safety).
- Each fix ships with its memory-scaling regression.
