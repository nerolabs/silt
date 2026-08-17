# Fix plan — bound inbound message memory with read backpressure (the MATURING OOM)

**Date:** 2026-08-17 (night) · **Root cause:**
`silt-reviews/principle-engineer/silt-oom-ROOT-CAUSE-unbounded-inbound-queue-2026-08-17.md`
· **Evidence:** run `e03f80d-heapprof` heap profiles (val-a 1020 MB RSS / 493 MB
heap; `cbor.fillByteString` 252 MB under `tcpnet.readLoop → eventloop.run →
node.handle`, 266 MB / 54%; 35 goroutines).

## The bug in one line

`tcpnet.readLoop` (per-connection goroutine) decodes every frame and
`t.loop.Post`s a closure capturing the decoded `msg` onto the event loop's
**unbounded** queue (`eventloop.Post` "never blocks"). Inbound decode outruns the
single loop (B2) → queue backs up → decoded payloads pin RAM → OOM. It is a
resource-exhaustion **DoS** (M0), not an efficiency knob.

## Design — admission-controlled inbound, everything else unchanged

**Principle:** backpressure belongs on the **network read path only**, NOT in the
event loop. The loop must stay non-blocking for INTERNAL posts (timers, API,
self-posts from within a running task) — blocking those would deadlock the single
thread. So the gate lives in `tcpnet`, sized in bytes, released when the loop
finishes the message.

### The gate

A counting semaphore over *outstanding inbound bytes* in the transport:

```go
// adapters/tcpnet: bytes admitted (decoded-but-not-yet-processed). Bounds the
// inbound working set so a fast/adversarial sender can't OOM the single loop.
type inboundGate struct {
    mu   sync.Mutex
    cond *sync.Cond
    used int64
    cap  int64
}
func (g *inboundGate) acquire(n int64) { // blocks the READER (safe: its own goroutine)
    g.mu.Lock(); for g.used+n > g.cap && g.used > 0 { g.cond.Wait() }
    g.used += n; g.mu.Unlock()
}
func (g *inboundGate) release(n int64) {
    g.mu.Lock(); g.used -= n; g.cond.Broadcast(); g.mu.Unlock()
}
```
`g.used > 0` in the wait guard guarantees a single over-cap message (a frame
bigger than the whole cap) still makes progress rather than deadlocking — it is
admitted alone.

### The seam (`tcpnet.go:~648-678`)

```go
// frame already read (bounded by the existing max-frame-size)
cost := int64(len(frame))
t.inbound.acquire(cost)               // ← reader blocks here when over cap → TCP backpressure
if err := cbor.Unmarshal(frame, &env); err != nil { t.inbound.release(cost); return }
...
msg := fromWire(env.Msg)
t.loop.Post(msg.Kind.String(), func() {
    defer t.inbound.release(cost)     // ← released when the LOOP finishes this message
    if t.handler != nil { t.handler(from, msg) }
})
```
(Acquire *before* `Unmarshal` so the decode itself is gated; release on every exit
path incl. the forgery/parse drops.)

### Why no deadlock

The loop only ever *runs* tasks (releasing capacity); it never *acquires*. Readers
acquire on their own per-connection goroutines. So the loop always drains →
releases → unblocks readers. A genuinely stuck handler blocks readers (correct:
stop accepting when you can't process) and the existing hang-watchdog fires. An
internal self-post never touches the gate, so a task that posts more work can't
self-block.

### What it changes behaviorally

Converts a **fatal OOM** into a **survivable throughput limit**: when the loop
falls behind, sockets stop being drained, TCP windows close, senders slow. The
node stays UP (alive > crashed) and catches up when load eases. Improving the
loop's throughput so the gate rarely engages is the M1-efficiency follow-up —
*bounded-then-fast*.

## Cap sizing (derive, don't guess — docs/network-durability.md discipline)

Bound = `max_frame_bytes × expected_concurrent_inbound × safety`, and must (a) not
engage under honest MATURING load (no happy-path latency), (b) leave headroom on a
2 GB box under GOMEMLIMIT. Starting proposal to sanity-check with research: a
global cap ~**256 MiB** (≈ the observed 266 MB backlog, so we cap near where OOM
began, well under 2 GB). Configurable via a daemon flag; 0 = unbounded (legacy).
**Flagged for derivation, not shipped as a magic number.**

## Fairness / priority (staging)

A single global cap prevents OOM but a flooding peer could fill it and starve
consensus messages behind it. Staging:
- **v1:** global inbound-bytes cap — kills the OOM (the M0 blocker). Ship first.
- **v2 (hardening):** per-peer share of the cap (no single peer monopolizes) +
  optionally a small reserved lane for consensus-critical kinds (vote/QC/round-change)
  so a gossip/publish flood can't starve the round. This is the full DoS-resistance
  story; open question to PE on whether v1 suffices for the M0 gate or v2 is required.

## Tests (failing-first)

1. **Memory wall (the regression):** a mock/loopback transport fed inbound frames
   faster than a deliberately-slow handler drains — assert outstanding inbound
   bytes stay ≤ cap (RED against today's unbounded `append`, GREEN after). This is
   the OOM regression, expressible in-process (no cloud).
2. **Liveness under flood:** a flooding peer + an honest consensus peer — assert
   the honest peer's messages still get processed (v1: eventually; v2: within a
   bound) and the node does not OOM.
3. **No happy-path regression:** under normal load the gate never blocks (used <
   cap throughout) — the full existing e2e/sim/integration suite stays green.
4. **Field re-confirm:** a `DEBUG_PROFILE=1` MATURING re-run — `infra-node-liveness`
   PASS (no OOM), and re-derive the (OOM-inflated) computed bounds on a live cohort.

## Then

`infra-node-liveness` PASS unblocks: (a) flixz (a memory-bounded head — this fix +
the shipped `-mem-limit`), and (b) the M0 gate (verdicts stop being provisional) →
red-team #183. The proof-map fix (#464) stays; it was correct, just not this.
