# Design — paginate chain sync (bound the `EncodeBlocks` serve spike)

**Date:** 2026-08-18 · **Driver:** the `880d993-memlimit` heap profile — `chain.EncodeBlocks`
= 144 MB LIVE, a node **serving** its whole chain (bond-reg-laden blocks) into ONE CBOR
buffer on `MsgGetChain{Height:0}`. The chain-serve driver (#3) of the multi-driver OOM.
**Status:** DESIGN for PE sign-off on the approach (delicate: consensus/sync path). The
server helper + its test are implemented here; the requester loop is flagged for review
BEFORE landing (PE guardrail: sync-correctness change gets review, not blind gating).

## The safety hinge (verified in code)

`chain.Reconcile(fork)` replays the fork and validates **full block linkage** — every
block must have `Prev == parent.Hash()` (`ErrWrongParent`, chain.go:1589) — plus: fork
starts from OUR genesis, contains the WS-checkpoint (F-1), and contains our committed
head (quorum-finality). So a paginated reassembly that **splices inconsistent windows**
(e.g. the peer reorged mid-fetch) **fails Reconcile and is rejected** — sync fails closed
for that peer this sweep and retries next. **Pagination cannot corrupt the chain**; the
existing validation backstops it. This is why it is tractable.

## What pagination changes (and does NOT)

- **Bounds:** the SERVER's `EncodeBlocks` transient buffer + the per-message wire size
  (144 MB → a window byte-cap). This is the observed driver (a node serving its chain).
- **Does NOT bound:** the REQUESTER's reassembled `full` chain (it still holds the whole
  fetched chain for `slashEquivocators` + `Reconcile`). That is the chain-RETENTION driver
  (#4), addressed separately by pruning/#299 — out of scope here.

## The scheme

### Server — `MsgGetChain` handler (chainrole.go:399)
Replace `EncodeBlocks(n.chain.Blocks(msg.Height))` (whole chain) with a byte-bounded
WINDOW from `msg.Height`:
```go
window := chain.EncodeBlocksUpTo(n.chain.Blocks(msg.Height), maxChainReplyBytes)
n.reply(from, msg, ports.Message{Kind: ports.MsgChainReply, OK: true, Data: window})
```
`EncodeBlocksUpTo(blocks, maxBytes)` encodes the longest PREFIX whose encoding fits in
maxBytes, always **≥ 1 block** (a lone oversized block still moves — never stall). The
requester detects the window's end by the last block's height and loops. `maxChainReplyBytes`
~= 8 MiB (WAN-movable per #286/#313; ~18 round-trips for a 144 MB chain — a sync is not
latency-critical like a consensus round).

### Requester — `fetchFull` loop (chainrole.go:1097)
The `MsgGetChainHead` probe already returns the peer's head **Height** (chainrole.go:429);
thread it into `fetchFull`. Then loop windows instead of one fetch:
```
full = []; h = 0
repeat:
  resp = request(MsgGetChain{Height: h})       // async; chain the callback like ask(i+1)
  window = DecodeBlocks(resp.Data)
  if window empty: break                        // peer gave nothing more (error / done)
  full = append(full, window...)
  h = window.last.Height + 1
until h > peerHeadHeight
# unchanged from here — operate on the reassembled full chain:
slashEquivocators(old, full)
Reconcile(full)
```
The async loop chains callbacks (the codebase idiom — see `ask`). On any window
error/timeout mid-loop, abort THIS peer (partial `full` is discarded, not reconciled) and
`next()` to the following peer — same failure semantics as today's single-fetch timeout.

## Failure modes (all fail closed)

| case | outcome |
|---|---|
| peer reorgs mid-fetch → spliced `full` | `Reconcile` `ErrWrongParent` → rejected, retry next sweep |
| peer serves a bad/short window | linkage break or `h` doesn't advance → abort peer, `next()` |
| peer never reaches `peerHeadHeight` (stalls windows) | bounded by the request timeout per window → abort peer |
| a single block > maxChainReplyBytes | `EncodeBlocksUpTo` sends it alone (≥1) → progress |
| old (un-paginated) requester vs new server | gets a truncated chain → `Reconcile` sees missing head → not adopted (safe, but sync stalls → **server+requester must ship together**, atomic) |

## Test plan

1. **`EncodeBlocksUpTo`** (unit, implemented here): a prefix that fits; ≥1 on oversize;
   round-trips through `DecodeBlocks`.
2. **Paginated sync end-to-end** (sim): a node N blocks ahead syncs a laggard via MANY
   windows; the laggard's final chain == the leader's; assert the server never encoded > cap
   per reply (a serve-size wall — the memory regression) AND > 1 window was needed.
3. **Reorg-mid-fetch safety**: inject a peer that changes its chain between windows → the
   spliced assembly is rejected, chain uncorrupted.
4. **No regression:** the full e2e/sim/integration sync suite green (head-match skip,
   equivocation-detection, reorg, catch-up all preserved).

## PE questions before I land the requester loop

1. `maxChainReplyBytes` = 8 MiB — endorse, or derive differently from network-durability?
2. The atomic server+requester rollout (a mixed fleet during deploy: new server + old
   requester stalls sync for that pair) — acceptable for a coordinated upgrade, or need a
   capability negotiation / fallback-to-full for old peers?
3. Is bounding the SERVE spike (not the requester's reassembled chain) the right scope for
   this PR, with retention/#299 as the follow-on? (I believe yes — it's the observed 144 MB
   driver.)
