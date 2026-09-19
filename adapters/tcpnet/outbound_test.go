// White-box tests for outbound admission control: they touch the unexported
// outboundGate + Transport.outbound, so they live in package tcpnet.
package tcpnet

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// The GLOBAL cap must bound the total charge across peers, and a refusal must
// not block the caller — Send runs on the single serialized loop, so admission
// answers immediately either way.
func TestOutboundGateGlobalCapRefusesWithoutBlocking(t *testing.T) {
	const cap = 100 << 10 // perPeer = 25 KiB
	g := newOutboundGate(cap)
	const n = 20 << 10 // charged at n + frameGoroutineCost = 28 KiB
	for i := 1; i <= 3; i++ {
		if !g.admit(peerID(byte(i)), n) {
			t.Fatalf("peer %d refused with an empty budget", i)
		}
	}
	if used := g.usedBytes(); used != 3*frameCost(n) {
		t.Fatalf("used=%d want %d", used, 3*frameCost(n))
	}
	// A 4th peer's frame would take the total to 4*28 KiB = 112 KiB > 100 KiB.
	if g.admit(peerID(4), n) {
		t.Fatalf("a frame over the global cap was admitted: used=%d cap=%d", g.usedBytes(), cap)
	}
	if g.refusedFrames() != 1 {
		t.Fatalf("refused=%d want 1", g.refusedFrames())
	}
	// Releasing makes room again: the gate is a budget, not a fuse.
	g.release(peerID(1), n)
	if !g.admit(peerID(4), n) {
		t.Fatal("a release did not make room for the refused frame")
	}
}

// PER-PEER FAIRNESS: one slow peer's backlog cannot consume the whole budget and
// starve the frames owed to everyone else. This is the property that matters in
// the field, where the backlog measured was on the links carrying one large
// payload while every other link sat empty.
func TestOutboundGatePerPeerShareLeavesOtherPeersRoom(t *testing.T) {
	const cap = 100 << 10 // perPeer = 25 KiB
	g := newOutboundGate(cap)
	slow := peerID(1)
	if !g.admit(slow, 10<<10) { // 18 KiB charged; under its 25 KiB share
		t.Fatal("first frame to an idle peer refused")
	}
	if g.admit(slow, 10<<10) { // 36 KiB would exceed the share, with global room
		t.Fatal("a peer over its share was admitted while the global budget had room")
	}
	if !g.admit(peerID(2), 10<<10) {
		t.Fatal("a peer with an empty backlog was starved by another peer's")
	}
}

// A frame larger than a limit must still be admitted when that peer has nothing
// in flight — progress over a permanent refusal. Its other half is the property
// that keeps this gate off the honest path: an idle link is never refused.
func TestOutboundGateAdmitsOversizeToAnIdlePeer(t *testing.T) {
	g := newOutboundGate(100 << 10)
	if !g.admit(peerID(1), 5<<20) {
		t.Fatal("an oversized frame to an idle peer must be admitted, not refused forever")
	}
	if g.admit(peerID(1), 1) {
		t.Fatal("the same peer must be refused while that frame is still in flight")
	}
}

// cap 0 = unbounded: nothing is ever refused (the sim/test default, and the
// pre-bound behaviour this file's control arm relies on).
func TestOutboundGateUnboundedNeverRefuses(t *testing.T) {
	g := newOutboundGate(0)
	for i := 0; i < 1000; i++ {
		if !g.admit(peerID(1), 1<<20) {
			t.Fatalf("cap 0 refused frame %d", i)
		}
	}
}

// stalledPeer listens as `seed`'s identity, completes the TLS handshake on every
// inbound conn and then NEVER READS. That is the field condition reduced to a
// fixture: a socket whose far end is not draining, so the sender's kernel buffer
// fills and every subsequent write parks. The conns are closed at cleanup, which
// releases the writers.
func stalledPeer(t *testing.T, seed int64) (ports.NodeID, string) {
	t.Helper()
	ident := identity.FromSeed(seed)
	cert, err := ident.Certificate()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS13,
	})
	if err != nil {
		t.Fatal(err)
	}
	held := make(chan net.Conn, 16)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				// Handshake so the dialer's pinned dial succeeds, then hold the
				// conn open and read nothing at all.
				_ = c.(*tls.Conn).Handshake()
				select {}
			}()
			select {
			case held <- c:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		for {
			select {
			case c := <-held:
				c.Close()
			default:
				return
			}
		}
	})
	return ident.NodeID(), ln.Addr().String()
}

// floodStalled sends `frames` frames of `payload` bytes at a peer that never
// reads, and reports the peak in-flight outbound charge and how many Send calls
// were refused. The conversation is established first so every flooded frame
// rides the ONE live conn — which is the field shape, where the backlog builds
// behind a single peer's write mutex rather than across fresh dials.
func floodStalled(t *testing.T, tr *Transport, to ports.NodeID, payload, frames int) (peak, refused int64) {
	t.Helper()
	if err := tr.Send(to, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
		t.Fatalf("opening send failed: %v", err)
	}
	for deadline := time.Now().Add(5 * time.Second); tr.liveConn(to) == nil; {
		if time.Now().After(deadline) {
			t.Fatal("no live conversation with the stalled peer")
		}
		time.Sleep(10 * time.Millisecond)
	}
	data := make([]byte, payload)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < frames; i++ {
			if err := tr.Send(to, ports.Message{Kind: ports.MsgStoreChunk, Data: data}); err != nil {
				refused++
			}
		}
	}()
	for {
		if u := tr.outbound.usedBytes(); u > peak {
			peak = u
		}
		select {
		case <-done:
			if u := tr.outbound.usedBytes(); u > peak {
				peak = u
			}
			return peak, refused
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// THE MEMORY WALL AT THE SENDING END. A node writing to a peer that is not
// draining retains every frame it has handed to a delivery goroutine. Unbounded,
// that working set grows with whatever the node decides to send — the shape
// measured in the field, where a validator's resident set climbed monotonically
// until the kernel killed it while an unshaped control stayed flat.
//
// Both arms run on the same fixture, minutes apart in the same test, because a
// bound that is never reached passes for free: the UNBOUNDED arm has to blow past
// the cap or the BOUNDED arm proves nothing.
func TestOutboundBacklogIsBoundedAgainstAPeerThatNeverReads(t *testing.T) {
	const (
		payload = 1 << 20 // 1 MiB frames, the order of a bond proof
		frames  = 48      // 48 MiB if nothing bounds it
		cap     = 8 << 20 // 8 MiB budget; a 2 MiB per-peer share
	)
	// One frame may be admitted against an almost-full budget, and the envelope
	// adds its own bytes on top of the payload; allow that much slack and no more.
	slack := 2 * frameCost(payload)

	// CONTROL — the pre-bound behaviour, reached through the documented
	// "0 = unbounded" sentinel rather than a second code path.
	unbounded, _ := newTransport(t, 420)
	defer unbounded.Close()
	idA, addrA := stalledPeer(t, 421)
	unbounded.AddPeer(idA, addrA)
	ctlPeak, ctlRefused := floodStalled(t, unbounded, idA, payload, frames)
	if ctlRefused != 0 {
		t.Fatalf("the unbounded control refused %d frames — it is not the pre-bound behaviour", ctlRefused)
	}
	if ctlPeak <= cap {
		t.Fatalf("the fixture never built a backlog: unbounded peak=%d bytes, under the %d-byte cap the other arm asserts — this gate cannot fail and proves nothing", ctlPeak, cap)
	}

	// THE RULE — the same flood at the same peer, with a budget.
	bounded, _ := newTransport(t, 422)
	defer bounded.Close()
	bounded.SetOutboundCap(cap)
	idB, addrB := stalledPeer(t, 423)
	bounded.AddPeer(idB, addrB)
	peak, refused := floodStalled(t, bounded, idB, payload, frames)
	if peak > cap+slack {
		t.Fatalf("outbound backlog blew the cap: peak=%d bytes, cap=%d (the unbounded control reached %d)", peak, cap, ctlPeak)
	}
	if refused == 0 {
		t.Fatalf("no frame was refused, so the bound never bit — peak=%d against cap=%d", peak, cap)
	}
	t.Logf("unbounded peak %d bytes (%d frames refused) vs bounded peak %d bytes (%d refused), cap %d",
		ctlPeak, ctlRefused, peak, refused, cap)
}

// THE POSITIVE CONTROL, on the gate's own axis: at the SHIPPED budget, a peer
// that drains must receive everything. A bound that also dropped frames on a
// healthy link would be a liveness regression wearing a memory fix's clothes,
// and it would pass the test above perfectly.
//
// This is what ties DefaultOutboundCap to a measured burst rather than to taste.
// The producer here writes 200 frames back to back with no pacing at all, which
// is faster than any core path offers them: the loop hands the transport one
// message per event. A budget that clears this clears the honest path. Driven at
// an 8 MiB cap instead, the same burst loses frames from the 28th on — so the number
// is load-bearing and the failure it guards is real, not hypothetical.
func TestOutboundBoundDropsNothingWhenThePeerDrains(t *testing.T) {
	const (
		payload = 64 << 10 // 13 MiB of frames, back to back
		frames  = 200
		cap     = DefaultOutboundCap
	)
	trA, loopA := newTransport(t, 424)
	defer func() { trA.Close(); loopA.Stop() }()
	trB, loopB := newTransport(t, 425)
	defer func() { trB.Close(); loopB.Stop() }()
	trA.SetOutboundCap(cap)

	got := make(chan struct{}, frames)
	trB.SetHandler(func(_ ports.NodeID, _ ports.Message) { got <- struct{}{} })

	idB := identity.FromSeed(425).NodeID()
	trA.AddPeer(idB, trB.Addr())
	data := make([]byte, payload)
	for i := 0; i < frames; i++ {
		if err := trA.Send(idB, ports.Message{Kind: ports.MsgStoreChunk, Data: data}); err != nil {
			t.Fatalf("frame %d was dropped against a draining peer: %v", i, err)
		}
	}
	deadline := time.After(30 * time.Second)
	for i := 0; i < frames; i++ {
		select {
		case <-got:
		case <-deadline:
			t.Fatalf("only %d/%d frames arrived — the bound cost delivery on a healthy link", i, frames)
		}
	}
	// The budget must return to zero — a gate that leaks a frame's charge on
	// every send eventually refuses everything on a healthy link. It is polled
	// rather than read once: arrival at the RECEIVER does not order the SENDER's
	// release, which runs when the delivery goroutine returns from its write, so
	// a single read here is a race that fails on one frame's charge.
	for wait := time.Now().Add(10 * time.Second); ; {
		used := trA.outbound.usedBytes()
		if used == 0 {
			break
		}
		if time.Now().After(wait) {
			t.Fatalf("outbound budget not fully released after the drain: used=%d", used)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
