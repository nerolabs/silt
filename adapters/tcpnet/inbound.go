package tcpnet

import "sync"

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
// NOTE (v1 — OOM stopped, DoS-resistance PENDING per PE ruling 2026-08-17): this is
// a single GLOBAL budget. It prevents the OOM, but a flooding peer can fill it and
// stall consensus messages queued behind it. Per-peer fairness + a reserved lane
// for consensus-critical kinds are REQUIRED before the red team (#183) so a flood
// can't convert the memory-DoS into a liveness-DoS.
type inboundGate struct {
	mu   sync.Mutex
	cond *sync.Cond
	used int64
	cap  int64 // 0 = unbounded (sims/tests; the daemon sets a real cap)
}

func newInboundGate(capBytes int64) *inboundGate {
	g := &inboundGate{cap: capBytes}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// setCap changes the budget (the daemon wires it from -inbound-cap after New).
func (g *inboundGate) setCap(c int64) {
	g.mu.Lock()
	g.cap = c
	g.cond.Broadcast() // a raised cap may admit waiters
	g.mu.Unlock()
}

// acquire reserves n bytes, blocking the caller until they fit. Callable only
// from a reader goroutine (never the loop). cap 0 = unbounded (never blocks). A
// frame larger than the whole cap is admitted alone when the gate is empty, so a
// single oversized-but-legal frame makes progress instead of deadlocking.
func (g *inboundGate) acquire(n int64) {
	g.mu.Lock()
	for g.cap > 0 && g.used > 0 && g.used+n > g.cap {
		g.cond.Wait()
	}
	g.used += n
	g.mu.Unlock()
}

func (g *inboundGate) release(n int64) {
	g.mu.Lock()
	g.used -= n
	g.cond.Broadcast()
	g.mu.Unlock()
}

// usedBytes reports the current in-flight inbound bytes (for tests/observability).
func (g *inboundGate) usedBytes() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.used
}
