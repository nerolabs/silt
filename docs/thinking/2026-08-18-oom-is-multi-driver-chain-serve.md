# The MATURING OOM is multi-driver — inbound FIXED, chain-serve encoding is next

**Date:** 2026-08-18 (early AM) · Evidence: three wire runs tonight, heap-profiled.
The OOM is not one bug; it's a **sequence of drivers**, each revealed by fixing the
one before it (classic memory whack-a-mole — but every step is real: 90→74→50 kills).

## The driver sequence (all heap-attributed on the wire)

1. **Inbound message queue (unbounded)** — `tcpnet.readLoop` → unbounded `eventloop`
   queue, 266 MB+ growing. **FIXED** (v1 backpressure): profiled bounded at 212 MB < the
   256 MB cap on `a9cfc06-reconfirm`. The gate holds.
2. **Go GC 2× amplification** — heap 379 MB → RSS 840 MB with no `-mem-limit`.
   **Mitigated** by `GOMEMLIMIT` (`880d993-memlimit`, 1500 M): **74 → 50 kills**. But
   1500 M is too high for a 2 GB box: under coordinated 10-handoff pressure all heavy
   nodes sat at ~1500 M at once (RSS ≈ heap + stacks + spikes → 2 GB). GOMEMLIMIT can't
   evict LIVE memory, and the live set during the drain is large →
3. **Chain-serve encoding (`chain.EncodeBlocks`) — the current dominant, 144 MB.** With
   inbound capped, the new top allocation on `880d993-memlimit` is
   `bytes.growSlice ← cbor.encMode.Marshal ← chain.EncodeBlocks` = 145 MB LIVE. Root:
   a syncing peer sends `MsgGetChain{Height:0}` (`chainrole.go:1099`) — the **whole
   chain** — and the server `EncodeBlocks(n.chain.Blocks(0))` (`:410`) marshals **every
   bond-reg-laden block (~1.5 MB each) into ONE buffer**. Both sides then hold the full
   chain (`DecodeBlocks` on the receiver too). During the maturing handoff many nodes
   sync at once → repeated 145 MB spikes → OOM. **This is a LIVE spike GOMEMLIMIT can't
   reclaim.**
4. **Chain retention** (all committed blocks resident, ~186 MB of decoded CBOR on
   sybils) — related; the audit's flagged unbounded chain-history path.

## Why the maturing regime specifically

The memory is dominated by **bond-reg proofs (~1.5 MB each)** — carried in blocks,
retained in the chain, and re-encoded in full on every sync. The MATURING regime mints
many of these (maturers registering). So a maturing consensus node's working set (full
chain of ~1.5 MB reg-blocks + inbound + plot + encode buffers) is **fundamentally large
for a 2 GB e2-small**.

## The fixes (both real, neither is a 3am change — flagged for PE + fresh focus)

1. **Paginate the chain sync** (the immediate next driver). Server: bound `EncodeBlocks`
   per `MsgGetChain` reply to a byte/height WINDOW; requester: loop over windows. DELICATE:
   the slash-on-detection scan + `Reconcile` fork-choice currently operate on the full
   fetched chain, so pagination touches sync correctness — PE review + careful tests.
2. **Don't re-serve full bond-reg proofs on every sync / prune them from retained blocks.**
   The proofs are the bulk; a synced peer often doesn't need the full space-time proof of
   every historical reg (it needs the committed result). This is the bigger structural win
   (cuts both retention #4 and serve #3) but a larger consensus-data change.

## Interim, to unblock the field test now

The memory reduction is the right long-term fix, but it's real work. To get a **clean
no-OOM MATURING sheet sooner** (so the flows can be graded), the pragmatic lever is a
**bigger box for the field test** — `e2-medium` (4 GB) instead of `e2-small` (2 GB) — with
a **lower `GOMEMLIMIT`** (e.g. 3000 M) for headroom. The field test's job is to grade the
CONSENSUS flows; a 2 GB box is an infrastructure constraint, not a product requirement
(a real operator sizes for their load). Pair with the paginate-sync fix landing over the
next days. **PE/Andrew call: fix-then-retest on 2 GB, or bigger-box-now + fix-in-parallel.**

## What's proven

- The inbound-queue OOM (the original attribution) is **fixed** (backpressure, wire-proven).
- `GOMEMLIMIT` is a necessary companion (74→50) but not sufficient alone at 1500 M/2 GB.
- The residual OOM is now **attributed to chain-serve encoding + chain retention of
  bond-reg proofs** — a maturing-regime data-size problem, not an unbounded-queue bug.
- Each fix is measurable progress; this is convergent, not stuck.
