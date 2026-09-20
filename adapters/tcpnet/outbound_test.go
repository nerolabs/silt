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

// A BULK BACKLOG MUST NOT BLIND THE NODE TRYING TO RECOVER BEHIND IT.
//
// The budget above is fair between PEERS and was blind between FRAME SIZES, so a
// link carrying multi-megabyte payload consumed its whole share and every small
// frame behind it was refused. Measured in the field (run 7eaf3bd-75421): of the
// 23 frames the budget dropped inside an impaired window, TWENTY were 121 bytes —
// chain-sync window requests and head probes, not payload. All four validators
// reported the same thing at the same moment:
//
//	chain sync sweep made NO progress while behind our-next=47 max-peer-head=0
//	  peers=4 probe-fails=4
//	  last-err="window@46 ...: tcpnet: outbound budget full ..."
//
// max-peer-head=0 on every seat: the whole validator set went blind to each
// other's heads, by the nodes' own attribution, while 1.57 MB frames held the
// budget. A node that cannot send a 121-byte probe cannot discover it is behind,
// cannot fetch the window that would catch it up, and cannot refuse fast either —
// a refusal reply is 121 bytes too. That is head-of-line blocking INSIDE the
// bound that closed the OOM.
//
// The rule: a slice of every share is reserved for small frames, so bulk is held
// short of it and a control frame always has somewhere to go. It is a SIZE rule
// and deliberately NOT a message-kind rule — the transport is an adapter (B1) and
// must not learn the core's message taxonomy in order to stay live.

// fillShareWithBulk drives one peer's share to the brim using BULK frames only,
// the way payload fills it in the field, and reports the headroom left.
//
// THE EXACT TOP-UP IS WHAT MAKES THIS A TEST RATHER THAN A COINCIDENCE. Admitting
// uniform 1 MiB frames until one is refused stops an average of half a frame short
// — hundreds of kilobytes of slack, which a 121-byte frame fits into comfortably,
// so the assertion below would pass on the very behaviour it exists to catch. The
// field had no such slack: 67,103,304 bytes in flight against a 67,108,864-byte
// share, 5,560 bytes short. So the last frame is sized to the headroom that is
// actually left.
func fillShareWithBulk(t *testing.T, g *outboundGate, p ports.NodeID) (headroom, share int64) {
	t.Helper()
	const bulk = 1 << 20
	for n := 0; g.admit(p, bulk); n++ {
		if n > 10000 {
			t.Fatal("PREMISE BROKEN: the share never filled, so nothing below sits behind a backlog")
		}
	}
	// Top up with one exactly-sized BULK frame, repeatedly, until even that is
	// refused. After the fix the first attempt is refused (bulk is held short of
	// the reserve), which is the point.
	for {
		inFlight, sh := g.peerBytes(p)
		want := sh - inFlight - frameGoroutineCost - 1 // cost lands one byte under the share
		if want <= smallFrameBytes || !g.admit(p, want) {
			break
		}
	}
	inFlight, sh := g.peerBytes(p)
	return sh - inFlight, sh
}

// THE RULE: with bulk unable to take another byte, a control-sized frame still goes.
func TestASmallFrameStillFitsBehindABulkBacklog(t *testing.T) {
	const cap = 64 << 20 // per-peer share = 16 MiB
	g := newOutboundGate(cap)
	p := peerID(1)

	headroom, share := fillShareWithBulk(t, g, p)
	t.Logf("MEASURED — bulk filled the share to within %d B of %d B", headroom, share)

	if !g.admit(p, 121) {
		t.Fatalf("a 121-byte frame was refused behind a bulk backlog (%d B of headroom in a %d B share). "+
			"That is the field failure exactly: the node cannot send the probe that would tell it it is "+
			"behind, cannot fetch the window that would catch it up, and cannot refuse fast either", headroom, share)
	}

	// AND THE RESERVE IS A BOUND, NOT A BYPASS. Small frames draw on a budget of
	// their own; they do not get an unlimited one, or the unbounded outbound
	// working set this gate exists to prevent returns through a smaller door.
	admitted := 1
	for g.admit(p, 121) {
		admitted++
		if admitted > 200000 {
			t.Fatal("small frames are admitted without limit — the reserve is a bypass, and the OOM is back " +
				"by another route")
		}
	}
	t.Logf("  small frames admitted before the reserve refused in turn: %d", admitted)
}

// THE CONTROL THAT KEEPS THE RESERVE HONEST: it comes OUT of the share rather
// than sitting on top of it.
//
// If it were additive the gate's real ceiling would quietly exceed the cap the
// operator set, which is the bound failing at the exact quantity it promises —
// build-immutable #8 asks for a bound, not a generous one.
func TestTheSmallFrameReserveComesOutOfTheShareRatherThanOnTopOfIt(t *testing.T) {
	const cap = 64 << 20
	g := newOutboundGate(cap)
	p := peerID(1)

	headroom, share := fillShareWithBulk(t, g, p)
	// A reserve of one byte is not a reserve. What it has to hold is a stalled
	// round's small traffic to this peer — probes, window requests, attestations,
	// round-changes and the refusals that end a round early — with room to spare.
	const mustHold = 64
	if want := mustHold * frameCost(121); headroom < want {
		t.Fatalf("bulk left %d B of a %d B share, room for %d control frames. A stalled round sends more than "+
			"that to one peer, so the node is blinded again at a slightly larger backlog — want room for at "+
			"least %d (%d B)", headroom, share, headroom/frameCost(121), mustHold, want)
	}
	for g.admit(p, 121) { // now fill the reserve too
	}
	total, _ := g.peerBytes(p)

	t.Logf("MEASURED — bulk stopped %d B short of the %d B share; with the reserve also full, %d B in flight",
		headroom, share, total)
	if total > share {
		t.Fatalf("the peer holds %d B against a %d B share: the reserve is ADDITIVE, so the gate's real ceiling "+
			"exceeds the cap the operator set", total, share)
	}
}
