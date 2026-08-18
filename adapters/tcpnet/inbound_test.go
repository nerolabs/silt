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

func peerID(b byte) ports.NodeID { var id ports.NodeID; id[0] = b; return id }

// The GLOBAL cap must bound total outstanding bytes across peers, block a
// would-be over-cap acquire, and unblock it once a release makes room. Distinct
// peers (each under its per-peer share) so the global cap is the binding limit.
func TestInboundGateGlobalCapBoundsAndUnblocks(t *testing.T) {
	g := newInboundGate(100) // perPeer = 25
	g.acquire(peerID(1), 20)
	g.acquire(peerID(2), 20)
	g.acquire(peerID(3), 20)
	g.acquire(peerID(4), 20) // used = 80, each peer 20 < 25
	if g.usedBytes() != 80 {
		t.Fatalf("used=%d want 80", g.usedBytes())
	}
	// A 5th peer's 30 would push total to 110 > 100 → must block on the global cap.
	unblocked := make(chan struct{})
	go func() { g.acquire(peerID(5), 30); close(unblocked) }()
	select {
	case <-unblocked:
		t.Fatal("acquire over the global cap must block, not admit")
	case <-time.After(50 * time.Millisecond):
	}
	g.release(peerID(1), 20) // used 60 → 60+30 = 90 ≤ 100 → unblocks
	select {
	case <-unblocked:
	case <-time.After(time.Second):
		t.Fatal("acquire did not unblock after a release made room")
	}
}

// PER-PEER FAIRNESS (v2a): one peer cannot exceed its share (cap/4) even when the
// global budget has room — and a DIFFERENT peer proceeds meanwhile. This is what
// keeps a single flooder from monopolizing the budget and starving everyone else.
func TestInboundGatePerPeerFairness(t *testing.T) {
	g := newInboundGate(100) // perPeer = 25
	flooder := peerID(1)
	g.acquire(flooder, 25) // flooder at its share; global used = 25 (plenty of room)

	// The flooder asking for more must BLOCK on its per-peer share (not the global cap).
	blocked := make(chan struct{})
	go func() { g.acquire(flooder, 10); close(blocked) }()
	select {
	case <-blocked:
		t.Fatal("a peer over its per-peer share must block even with global room")
	case <-time.After(50 * time.Millisecond):
	}
	// A DIFFERENT peer proceeds immediately — the flooder didn't starve it.
	other := make(chan struct{})
	go func() { g.acquire(peerID(2), 40); close(other) }()
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("a well-behaved peer must proceed while a flooder is throttled")
	}
	// The flooder unblocks once it releases back under its share.
	g.release(flooder, 25)
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("flooder did not unblock after releasing under its share")
	}
}

// A single frame larger than a limit must be admitted when that pool is empty
// (progress over deadlock) — a legal-but-oversized chunk still moves.
func TestInboundGateAdmitsSingleOversize(t *testing.T) {
	g := newInboundGate(100) // both cap and perPeer are smaller than 500
	done := make(chan struct{})
	go func() { g.acquire(peerID(1), 500); close(done) }()
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
			g.acquire(peerID(byte(i)), 1<<20)
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
