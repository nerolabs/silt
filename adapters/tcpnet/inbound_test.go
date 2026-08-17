// White-box tests for inbound backpressure (the MATURING OOM fix): they touch the
// unexported inboundGate + Transport.inbound, so they live in package tcpnet.
package tcpnet

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// The gate must bound outstanding bytes to the cap, block a would-be over-cap
// acquire, and unblock it once a release makes room. This is the OOM-prevention
// property in miniature: outstanding inbound bytes never grow past the budget.
func TestInboundGateBoundsAndUnblocks(t *testing.T) {
	g := newInboundGate(100)
	g.acquire(60)
	g.acquire(30) // used = 90, still under cap
	if g.usedBytes() != 90 {
		t.Fatalf("used=%d want 90", g.usedBytes())
	}

	// A 20-byte acquire would push used to 110 > 100 → must block.
	unblocked := make(chan struct{})
	go func() { g.acquire(20); close(unblocked) }()
	select {
	case <-unblocked:
		t.Fatal("acquire over the cap must block, not admit")
	case <-time.After(50 * time.Millisecond):
	}

	// Free 60: used drops to 30, so the pending 20 now fits → it must proceed.
	g.release(60)
	select {
	case <-unblocked:
	case <-time.After(time.Second):
		t.Fatal("acquire did not unblock after a release made room")
	}
	if g.usedBytes() != 50 { // 90 - 60 + 20
		t.Fatalf("used=%d want 50", g.usedBytes())
	}
}

// A single frame larger than the whole cap must be admitted when the gate is
// empty (progress over deadlock) — a legal-but-oversized chunk still moves.
func TestInboundGateAdmitsSingleOversize(t *testing.T) {
	g := newInboundGate(100)
	done := make(chan struct{})
	go func() { g.acquire(500); close(done) }() // 500 > cap 100, but gate is empty
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("an oversized frame must be admitted alone, not deadlock")
	}
}

// cap 0 = unbounded: acquire never blocks (the sim/test default).
func TestInboundGateUnboundedNeverBlocks(t *testing.T) {
	g := newInboundGate(0)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			g.acquire(1 << 20)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cap 0 must never block")
	}
}

// THE MEMORY WALL (end to end): flood a receiver whose loop is WEDGED on the first
// message. Without admission control the decoded messages pile onto the unbounded
// queue without limit (the OOM). With the cap, the receiver's inbound working set
// stays ~cap no matter how many frames are thrown at it — the reader blocks and TCP
// pushes back — and every message still processes once the loop drains (alive, not
// crashed).
func TestInboundBackpressureBoundsBacklogUnderAWedgedLoop(t *testing.T) {
	trA, loopA := newTransport(t, 400)
	defer func() { trA.Close(); loopA.Stop() }()
	trB, loopB := newTransport(t, 401)
	defer func() { trB.Close(); loopB.Stop() }()

	const (
		payload = 8 << 10  // ~8 KiB/message
		cap     = 40 << 10 // room for ~5 in flight
		flood   = 60       // far more than the cap holds
	)
	trB.SetInboundCap(cap)

	wedge := make(chan struct{})
	var processed atomic.Int64
	trB.SetHandler(func(_ ports.NodeID, _ ports.Message) {
		if processed.Add(1) == 1 {
			<-wedge // the first message wedges the single loop until released
		}
	})

	idB := identity.FromSeed(401).NodeID()
	trA.AddPeer(idB, trB.Addr())
	data := make([]byte, payload)
	go func() {
		for i := 0; i < flood; i++ {
			_ = trA.Send(idB, ports.Message{Kind: ports.MsgStoreChunk, Data: data})
		}
	}()

	// While the loop is wedged and A floods, B's inbound budget must stay bounded —
	// NOT grow toward flood*payload (~480 KiB). Sample the peak over a settle window.
	deadline := time.Now().Add(3 * time.Second)
	var peak int64
	for time.Now().Before(deadline) {
		if u := trB.inbound.usedBytes(); u > peak {
			peak = u
		}
		time.Sleep(20 * time.Millisecond)
	}
	// One frame may be mid-acquire above the cap; allow generous slack for the
	// envelope, but well under the unbounded flood total.
	if peak > cap+2*payload {
		t.Fatalf("inbound backlog blew the cap: peak=%d bytes, cap=%d (unbounded would be ~%d)", peak, cap, flood*payload)
	}

	// Liveness: release the wedge; every flooded message must still be processed
	// (backpressure slows intake, it does not drop messages).
	close(wedge)
	live := time.Now().Add(10 * time.Second)
	for processed.Load() < flood && time.Now().Before(live) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := processed.Load(); got < flood {
		t.Fatalf("only %d/%d messages processed after unwedging — backpressure dropped messages", got, flood)
	}
	if u := trB.inbound.usedBytes(); u != 0 {
		t.Fatalf("inbound budget not fully released after drain: used=%d", u)
	}
}
