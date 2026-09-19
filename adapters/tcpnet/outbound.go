package tcpnet

import (
	"sync"

	"github.com/nerolabs/silt/ports"
)

// outboundGate bounds the transport's in-flight OUTBOUND working set — the
// marshalled frames Send has handed to a delivery goroutine that have not yet
// reached the peer's socket. It is the dual of inboundGate (inbound.go) and
// exists for the same reason at the other end of the wire: Send hands every
// frame to its own goroutine (`go t.deliver`), and that goroutine RETAINS the
// whole marshalled frame while it waits on the per-peer write mutex and then on
// a socket whose send buffer is already full. Nothing counted those frames, so
// a node producing faster than a link drains accumulated them without limit and
// OOM-crashed — an unbounded system on a small box is unsafe, not slow
// (build-immutable #8).
//
// The measured shape: under composed network impairment a validator's resident
// set grew monotonically with no plateau until the kernel killed it, while an
// unshaped control on the same box stayed flat; the goroutine population WAS the
// backlog (over 90% of them parked in deliver → peerConn.write), and the frame
// encoder held the majority of the live heap.
//
// REFUSING, NOT BLOCKING, is the difference from the inbound gate, and it is
// forced by which thread calls in. The inbound gate blocks a per-connection
// reader, which is safe: it stops draining one socket and TCP flow-control
// pushes back on that sender. Send runs on the node's single serialized loop
// (B2). Blocking there would stall every peer, every timer and every API call
// behind one slow socket — trading a bounded memory failure for a whole-node
// liveness failure. So a frame that does not fit is DROPPED, which is the
// transport's documented loss semantics: a failed write drops the message and
// the core's timeout machinery owns recovery; there is no retransmit here.
//
// Dropping is also the right answer on the merits rather than merely the
// available one. The budget is full only when this peer's socket is already
// backed up, and the field evidence is that a re-sent copy queued behind an
// undelivered one arrives long after the deadline that asked for it has passed —
// while the bytes it adds to the queue slow down everything already in it.
type outboundGate struct {
	mu      sync.Mutex
	used    int64
	cap     int64                  // 0 = unbounded (sims/tests; the daemon sets a real cap)
	perPeer int64                  // per-peer share of the cap; 0 = no per-peer limit
	perUsed map[ports.NodeID]int64 // in-flight bytes per peer (only non-zero entries)
	refused int64                  // frames dropped for want of budget (observability)
}

// frameGoroutineCost prices the delivery goroutine each admitted frame owns: its
// stack (Go starts a goroutine small and grows it on the first call) plus the
// gate's own bookkeeping, rounded generously up. It is charged alongside the
// frame's bytes so that ONE budget bounds the whole outbound footprint. Without
// it a flood of tiny frames would sit comfortably inside a byte cap while the
// goroutine stacks carrying them — the larger cost at that size — grew without
// bound, which is the same unboundedness in a different allocation.
const frameGoroutineCost = 8 << 10

// DefaultOutboundCap is the budget the daemon ships (-outbound-cap). It mirrors
// the inbound cap, because the two bound the same quantity at the two ends of
// the wire and an operator sizing one should not have to reason differently
// about the other. The sizing has to clear an honest burst and stay well under
// the floor box:
//
//   - The per-peer share is a quarter of it — 64 MiB — which holds hundreds of
//     default-sized (256 KiB) chunk pushes or dozens of multi-megabyte bond
//     proofs in flight to one peer. A frame larger than the share still goes,
//     alone, under the empty-pool rule, so the legal-maximum chunk is never
//     refused for its size; it is serialized per peer, which a single write
//     mutex does anyway.
//   - Against the declared floor spec (2 GiB) the worst case this admits is
//     12.5% of the box, on top of the same again inbound, against a measured
//     honest working set of ~260 MiB. Bounded, and an operator on a smaller box
//     can lower it.
//
// It is a memory budget, not a security parameter: no security property may
// rest on it, and lowering it costs delivery on a backed-up link rather than a
// defence (build-immutable #3).
const DefaultOutboundCap = 256 << 20

func newOutboundGate(capBytes int64) *outboundGate {
	g := &outboundGate{perUsed: make(map[ports.NodeID]int64)}
	g.setCapLocked(capBytes)
	return g
}

func (g *outboundGate) setCapLocked(c int64) {
	g.cap = c
	if c > 0 {
		g.perPeer = c * perPeerNum / perPeerDen
	} else {
		g.perPeer = 0
	}
}

// setCap changes the budget (the daemon wires it from -outbound-cap after New).
func (g *outboundGate) setCap(c int64) {
	g.mu.Lock()
	g.setCapLocked(c)
	g.mu.Unlock()
}

// frameCost is what an n-byte frame charges against the budget.
func frameCost(n int64) int64 { return n + frameGoroutineCost }

// admit reserves an n-byte frame's cost for peer and reports whether it fits
// under BOTH the global cap and this peer's share. It never blocks; see the type
// comment for why a refusal rather than backpressure is the only sound answer on
// the loop's thread. cap 0 = unbounded.
//
// The "used > 0" / "perUsed > 0" guards admit a lone frame bigger than a limit
// when that pool is empty, so an oversized-but-legal frame still moves instead of
// being refused forever — the same rule the inbound gate uses. Their other effect
// is what keeps this off the honest path entirely: a peer with nothing in flight
// is never refused, so the gate bites only a link that is already backed up.
func (g *outboundGate) admit(peer ports.NodeID, n int64) bool {
	cost := frameCost(n)
	g.mu.Lock()
	defer g.mu.Unlock()
	if (g.cap > 0 && g.used > 0 && g.used+cost > g.cap) ||
		(g.perPeer > 0 && g.perUsed[peer] > 0 && g.perUsed[peer]+cost > g.perPeer) {
		g.refused++
		return false
	}
	g.used += cost
	g.perUsed[peer] += cost
	return true
}

func (g *outboundGate) release(peer ports.NodeID, n int64) {
	cost := frameCost(n)
	g.mu.Lock()
	g.used -= cost
	if g.perUsed[peer] -= cost; g.perUsed[peer] <= 0 {
		delete(g.perUsed, peer)
	}
	g.mu.Unlock()
}

// usedBytes reports the current in-flight outbound charge (for tests/observability).
func (g *outboundGate) usedBytes() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.used
}

// peerBytes reports one peer's in-flight charge and the share it is held to —
// the two numbers a refusal has to name to be actionable.
func (g *outboundGate) peerBytes(peer ports.NodeID) (inFlight, share int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.perUsed[peer], g.perPeer
}

// refusedFrames counts frames dropped for want of budget since start.
func (g *outboundGate) refusedFrames() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.refused
}
