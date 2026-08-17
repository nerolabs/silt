package tcpnet

import (
	"sync"

	"github.com/nerolabs/silt/ports"
)

// inboundGate bounds the transport's in-flight INBOUND working set — the bytes a
// node has read off the wire and not yet finished processing on its single
// serialized loop (B2). It exists because the loop's queue is unbounded and each
// queued task pins its decoded message: under load (or an adversarial flood) a
// fast sender's messages pile up faster than the loop drains them, the decoded
// payloads accumulate, and the node OOM-crash-loops. This is a resource-exhaustion
// DoS on a remote-controlled input path — a security floor (build-immutables #4/#5,
// personas #13/#14; the memory twin of the #424 CPU-flood), not an efficiency knob.
//
// The reader (per-connection goroutine) acquires this budget BEFORE reading a
// frame's body and releases it when the LOOP finishes handling that message. When
// the budget is exhausted the reader BLOCKS — which stops draining that socket, so
// TCP flow-control pushes back on the sender. A fatal OOM becomes a survivable
// throughput limit (alive > crashed). The loop only ever RELEASES (never acquires),
// so it always drains and wakes readers — no deadlock.
//
// v2a — PER-PEER FAIRNESS (PE ruling 2026-08-17): beyond the global budget, each
// peer is confined to a SHARE (perPeerFrac of the cap), so one flooding peer (or a
// single sybil) can't monopolize the budget and starve everyone else — it fills its
// own share, blocks, and TCP pushes back on IT while other peers proceed.
//
// v2b PENDING before #183: a consensus-kind RESERVE so that even when a SYBIL COHORT
// collectively fills the general budget, consensus-critical frames (prepare/precommit/
// round-change/commit) from honest validators still have room. Per-peer fairness
// bounds a single flooder; the reserve is what a many-peer flood still can't touch.
// Design: docs/thinking/2026-08-17-inbound-backpressure-fix-plan.md (needs
// decode-then-classify with a bounded speculative-decode slot for small frames).
type inboundGate struct {
	mu      sync.Mutex
	cond    *sync.Cond
	used    int64
	cap     int64                  // 0 = unbounded (sims/tests; the daemon sets a real cap)
	perPeer int64                  // per-peer share of the cap; 0 = no per-peer limit
	perUsed map[ports.NodeID]int64 // in-flight bytes per peer (only non-zero entries)
}

// perPeerFrac: a single peer may hold at most this fraction of the global budget.
// 1/4 leaves ≥ 3/4 for the rest, so no single flooder can wedge the majority.
const perPeerNum, perPeerDen = 1, 4

func newInboundGate(capBytes int64) *inboundGate {
	g := &inboundGate{perUsed: make(map[ports.NodeID]int64)}
	g.cond = sync.NewCond(&g.mu)
	g.setCapLocked(capBytes)
	return g
}

func (g *inboundGate) setCapLocked(c int64) {
	g.cap = c
	if c > 0 {
		g.perPeer = c * perPeerNum / perPeerDen
	} else {
		g.perPeer = 0
	}
}

// setCap changes the budget (the daemon wires it from -inbound-cap after New).
func (g *inboundGate) setCap(c int64) {
	g.mu.Lock()
	g.setCapLocked(c)
	g.cond.Broadcast() // a raised cap may admit waiters
	g.mu.Unlock()
}

// acquire reserves n bytes for peer, blocking the caller until they fit under BOTH
// the global cap and this peer's share. Callable only from a reader goroutine (never
// the loop). cap 0 = unbounded. The "used > 0" / "perUsed > 0" guards admit a lone
// frame bigger than a limit when that pool is empty, so an oversized-but-legal frame
// makes progress instead of deadlocking.
func (g *inboundGate) acquire(peer ports.NodeID, n int64) {
	g.mu.Lock()
	for (g.cap > 0 && g.used > 0 && g.used+n > g.cap) ||
		(g.perPeer > 0 && g.perUsed[peer] > 0 && g.perUsed[peer]+n > g.perPeer) {
		g.cond.Wait()
	}
	g.used += n
	g.perUsed[peer] += n
	g.mu.Unlock()
}

func (g *inboundGate) release(peer ports.NodeID, n int64) {
	g.mu.Lock()
	g.used -= n
	if g.perUsed[peer] -= n; g.perUsed[peer] <= 0 {
		delete(g.perUsed, peer)
	}
	g.cond.Broadcast()
	g.mu.Unlock()
}

// usedBytes reports the current in-flight inbound bytes (for tests/observability).
func (g *inboundGate) usedBytes() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.used
}
