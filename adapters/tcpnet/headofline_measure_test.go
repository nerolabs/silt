// White-box: reaches Transport internals, so it lives in package tcpnet.
package tcpnet

import (
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// HEAD-OF-LINE BLOCKING ON ONE ORDERED CONNECTION, REPRODUCED BELOW THE FIELD TIER.
//
// The impairment wedge has survived four mechanisms built above the transport —
// retry de-duplication, proposer/attester alignment, the digest relay, and a
// held-registration inventory. Each moved its own number; none moved the wedge. Six
// field reproductions.
//
// The reason the last field run finally isolated is that consensus and its payload
// share ONE ORDERED PER-PEER TLS CONNECTION. On run 7eaf3bd-75421 a 121-byte
// chain-sync probe was DROPPED by admission control, and the outbound reserve fixed
// that: run 1b0b933-52703 dropped zero control frames. And the probes still failed —
// `probe-fails=4` on every seat, every sweep, with `last-err` EMPTY, because the
// failure is now a TIMEOUT. The frame is admitted, written to the socket, and waits
// behind megabytes already in the stream. An application-level budget decides what
// to hand a socket; it cannot reorder what the socket has accepted.
//
// THAT COST THREE BILLABLE RUNS TO LEARN AND IS NOT REPRODUCED ANYWHERE CHEAPER,
// which is the gap this file closes. Build-immutable #7 wants the repro below the
// field tier before anything is spent on a fix, and whichever of the three remedies
// is chosen — succinct proofs, a separate control stream, or QUIC — has to be graded
// against a failing local measurement rather than against another cloud run.
//
// The sim could never have shown it: adapters/simnet delivers a message atomically
// after a latency draw, so there is no stream to be behind. Loopback could not show
// it either while it had unlimited capacity. What makes it reproducible here is a
// reader that drains at a FIXED RATE, which is the one property of the impaired
// field link that matters: bytes leave the socket slower than the sender offers them.

// slowReader accepts one TLS conn as `seed`'s identity and then drains it at a fixed
// byte rate, reporting when each framed message completes. It is the field link
// reduced to its essential: a wire that moves bytes slower than the sender writes
// them, so what is already in the stream decides when the next thing can arrive.
type slowReader struct {
	id       ports.NodeID
	addr     string
	arrivals chan arrival
}

type arrival struct {
	size int
	at   time.Time
}

func newSlowReader(t *testing.T, seed int64, bytesPerSec int) *slowReader {
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
	sr := &slowReader{id: ident.NodeID(), addr: ln.Addr().String(), arrivals: make(chan arrival, 64)}
	done := make(chan struct{})
	// ACCEPT IN A LOOP. This fixture offers no ALPN, so it is a pre-lane peer: the
	// transport dials a control lane, finds none negotiated, closes it and falls back
	// to the shared conn. A single Accept would be consumed by that refused dial and
	// the real conn would never be taken — which is what this fixture is for, a peer
	// that has only one ordered stream.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				go func() {
					<-done
					c.Close()
				}()
				// Drain at the configured rate, one small slice at a time, and report a
				// message the moment its last byte has been read.
				const slice = 4 << 10
				pause := time.Duration(int64(time.Second) * int64(slice) / int64(bytesPerSec))
				buf := make([]byte, slice)
				var hdr [4]byte
				for {
					if _, err := io.ReadFull(c, hdr[:]); err != nil {
						return
					}
					n := int(binary.BigEndian.Uint32(hdr[:]))
					for left := n; left > 0; {
						take := slice
						if left < take {
							take = left
						}
						if _, err := io.ReadFull(c, buf[:take]); err != nil {
							return
						}
						left -= take
						time.Sleep(pause)
					}
					select {
					case sr.arrivals <- arrival{size: n, at: time.Now()}:
					default:
					}
				}
			}(c)
		}
	}()
	t.Cleanup(func() { close(done); ln.Close() })
	return sr
}

// THE MEASUREMENT: how long a control-sized frame waits when a bulk frame is
// already in the stream ahead of it, against how long it waits on its own.
//
// The control arm is what makes the number mean anything: the same frame, the same
// socket, the same rate, with nothing in front of it. Without that arm a slow
// arrival could be the rate, the handshake, or the fixture.
//
// THE BULK HAS TO BE BIGGER THAN THE KERNEL WILL SWALLOW, and the first cut of this
// file was not — which the assertion caught rather than passing quietly. Send hands
// every frame to its own goroutine, so the order two frames reach the wire in is
// decided by whichever wins the per-peer write mutex. A 512 KiB payload disappears
// into a loopback socket buffer instantly, releasing the mutex before the control
// frame even queues, and the control frame then arrives FIRST: 64 ms either way, no
// blocking, nothing measured. The field's bulk frame was mid-write on a socket that
// had stopped accepting, which is the state this has to reach. So the payload is
// sized past the buffers and the control frame is offered only once the bulk write
// is demonstrably parked.
func TestAControlFrameWaitsBehindBulkOnOneOrderedConnection(t *testing.T) {
	const (
		rate  = 8 << 20  // fast enough to keep the test seconds long, slow enough to back the writer up
		bulk  = 16 << 20 // past any loopback send+receive buffer, so the writer genuinely blocks
		small = 121      // the chain-sync probe the field measured being starved
		wait  = 120 * time.Second
	)

	send := func(t *testing.T, seed int64, withBulkAhead bool) time.Duration {
		t.Helper()
		tr, loop := newTransport(t, seed)
		defer func() { tr.Close(); loop.Stop() }()
		sr := newSlowReader(t, seed+1, rate)
		tr.AddPeer(sr.id, sr.addr)

		// Open the conversation and let the handshake settle, so neither arm pays
		// for it inside the measurement.
		if err := tr.Send(sr.id, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
			t.Fatalf("opening send: %v", err)
		}
		select {
		case <-sr.arrivals:
		case <-time.After(wait):
			t.Fatal("the opening frame never arrived — the fixture is not connected")
		}

		if withBulkAhead {
			if err := tr.Send(sr.id, ports.Message{Kind: ports.MsgStoreChunk, Data: make([]byte, bulk)}); err != nil {
				t.Fatalf("bulk send: %v", err)
			}
			// Let the bulk write take the write mutex and block on the socket. Without
			// this the two frames simply race for the mutex and the measurement is of
			// the race, not of the stream.
			time.Sleep(500 * time.Millisecond)
		}
		start := time.Now()
		if err := tr.Send(sr.id, ports.Message{Kind: ports.MsgGetChainHead, Data: make([]byte, small)}); err != nil {
			t.Fatalf("control send: %v", err)
		}
		// Wait for the CONTROL frame: with bulk ahead, the bulk arrives first.
		for deadline := time.After(wait); ; {
			select {
			case a := <-sr.arrivals:
				if a.size < bulk/2 { // the control frame, not the bulk one
					return a.at.Sub(start)
				}
			case <-deadline:
				t.Fatal("the control frame never arrived inside the window")
			}
		}
	}

	alone := send(t, 600, false)
	behind := send(t, 610, true)

	t.Logf("MEASURED — a %d-byte control frame on a connection draining at %d B/s:", small, rate)
	t.Logf("  with nothing ahead of it:            %v", alone.Round(time.Millisecond))
	t.Logf("  with a %d-byte bulk frame ahead:  %v", bulk, behind.Round(time.Millisecond))
	t.Logf("  the bulk frame's own wire time:      %v", time.Duration(int64(time.Second)*int64(bulk)/int64(rate)))

	// THE PROPERTY, asserted so it cannot rot: the control frame is delayed by
	// essentially the whole bulk payload, because one ordered stream has no way to
	// let it past. This is the wedge's remaining cause, reduced to a local test.
	wire := time.Duration(int64(time.Second) * int64(bulk) / int64(rate))
	if behind < wire/2 {
		t.Fatalf("the control frame arrived in %v with %d bytes of bulk (%v of wire) ahead of it. Either the "+
			"fixture is not actually serializing them or something now lets a small frame past a large one — "+
			"if the latter, the wedge's remaining cause has changed and the sheet must be re-derived", behind, bulk, wire)
	}
	if alone > wire/4 {
		t.Fatalf("the control frame took %v even with NOTHING ahead of it, against %v of bulk wire time. The "+
			"fixture is measuring its own overhead rather than head-of-line blocking, so the comparison above "+
			"means nothing", alone, wire)
	}
	t.Logf("  => head-of-line blocking on one ordered connection: %.0fx", float64(behind)/float64(alone))
}

// A SECOND CONNECTION IS WHAT THE MEASUREMENT SAYS WOULD HELP, and this prices it
// on the same fixture rather than in the abstract — the arm any remedy has to beat.
//
// It is NOT a proposed implementation. Opening a second conn inside the transport
// means a role negotiated at connection setup (otherwise the peer's adopt() replaces
// its general conversation with the control conn), and that reaches into the relay
// splice, the hole-punch upgrade and NATed reply routing. The number here is what
// decides whether that work is worth it.
func TestASecondConnectionCarriesTheControlFrameImmediately(t *testing.T) {
	const (
		rate = 8 << 20
		bulk = 16 << 20
		wait = 120 * time.Second
	)
	tr, loop := newTransport(t, 620)
	defer func() { tr.Close(); loop.Stop() }()

	bulkPeer := newSlowReader(t, 621, rate)
	ctrlPeer := newSlowReader(t, 622, rate)
	tr.AddPeer(bulkPeer.id, bulkPeer.addr)
	tr.AddPeer(ctrlPeer.id, ctrlPeer.addr)
	for _, p := range []*slowReader{bulkPeer, ctrlPeer} {
		if err := tr.Send(p.id, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
			t.Fatalf("opening send: %v", err)
		}
		select {
		case <-p.arrivals:
		case <-time.After(wait):
			t.Fatal("a fixture never connected")
		}
	}

	if err := tr.Send(bulkPeer.id, ports.Message{Kind: ports.MsgStoreChunk, Data: make([]byte, bulk)}); err != nil {
		t.Fatalf("bulk send: %v", err)
	}
	time.Sleep(500 * time.Millisecond) // the bulk write is parked on its own socket
	start := time.Now()
	if err := tr.Send(ctrlPeer.id, ports.Message{Kind: ports.MsgGetChainHead, Data: make([]byte, 121)}); err != nil {
		t.Fatalf("control send: %v", err)
	}
	var got time.Duration
	select {
	case a := <-ctrlPeer.arrivals:
		got = a.at.Sub(start)
	case <-time.After(wait):
		t.Fatal("the control frame never arrived on its own connection")
	}

	wire := time.Duration(int64(time.Second) * int64(bulk) / int64(rate))
	t.Logf("MEASURED — the same control frame on a SEPARATE connection, with %v of bulk in flight elsewhere: %v",
		wire, got.Round(time.Millisecond))

	if got > wire/4 {
		t.Fatalf("a control frame on its own connection still took %v against %v of unrelated bulk wire. Then "+
			"separating the streams does NOT buy what the field failure needs, and the remedy has to be the "+
			"one that removes the payload instead", got, wire)
	}
}

// THE LANE, DRIVEN AGAINST THE FIXTURE THAT MEASURES THE DEFECT.
//
// Both frames go to the SAME peer, which is the field shape and the arm the two
// measurements above could not make: the bulk payload and the control frame are
// addressed to one node, and the only question is whether they share a stream.
//
// laneReader accepts BOTH lanes from one dialer and timestamps arrivals on each, so
// a control frame that overtakes a bulk frame is visible as an ordering fact rather
// than inferred from two separate peers.
func TestTheControlLaneCarriesASmallFramePastABulkOneToTheSamePeer(t *testing.T) {
	const (
		rate = 8 << 20
		bulk = 16 << 20
		wait = 120 * time.Second
	)
	lr := newLaneReader(t, 701, rate)
	tr, loop := newTransport(t, 700)
	defer func() { tr.Close(); loop.Stop() }()
	tr.AddPeer(lr.id, lr.addr)

	// Open the bulk conversation and let its handshake settle.
	if err := tr.Send(lr.id, ports.Message{Kind: ports.MsgStoreChunk, Data: make([]byte, 1<<10)}); err != nil {
		t.Fatalf("opening send: %v", err)
	}
	select {
	case <-lr.arrivals:
	case <-time.After(wait):
		t.Fatal("the opening frame never arrived")
	}

	if err := tr.Send(lr.id, ports.Message{Kind: ports.MsgStoreChunk, Data: make([]byte, bulk)}); err != nil {
		t.Fatalf("bulk send: %v", err)
	}
	time.Sleep(500 * time.Millisecond) // the bulk write is parked on the bulk socket
	start := time.Now()
	if err := tr.Send(lr.id, ports.Message{Kind: ports.MsgGetChainHead, Data: make([]byte, 121)}); err != nil {
		t.Fatalf("control send: %v", err)
	}

	var got time.Duration
	var lane string
	for deadline := time.After(wait); ; {
		select {
		case a := <-lr.arrivals:
			if a.size < bulk/2 {
				got, lane = a.at.Sub(start), a.lane
				goto done
			}
		case <-deadline:
			t.Fatal("the control frame never arrived")
		}
	}
done:
	wire := time.Duration(int64(time.Second) * int64(bulk) / int64(rate))
	t.Logf("MEASURED — bulk and control to the SAME peer, %v of bulk in flight:", wire)
	t.Logf("  the control frame arrived in %v, on the %q lane", got.Round(time.Millisecond), lane)

	if lane != alpnCtrl {
		t.Fatalf("the control frame rode the %q lane, not %q — the size rule or the dial did not route it, so "+
			"the measurement below is of the shared stream", lane, alpnCtrl)
	}
	if got > wire/4 {
		t.Fatalf("the control frame took %v against %v of bulk wire time even on its own lane. The lanes are "+
			"not independent — check that the bulk conn is not being reused for it", got, wire)
	}
}

// laneReader accepts both ALPN lanes from one dialer and reports which lane each
// framed message arrived on.
type laneReader struct {
	id       ports.NodeID
	addr     string
	arrivals chan laneArrival
}

type laneArrival struct {
	size int
	at   time.Time
	lane string
}

func newLaneReader(t *testing.T, seed int64, bytesPerSec int) *laneReader {
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
		NextProtos:   []string{alpnBulk, alpnCtrl},
	})
	if err != nil {
		t.Fatal(err)
	}
	lr := &laneReader{id: ident.NodeID(), addr: ln.Addr().String(), arrivals: make(chan laneArrival, 64)}
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				go func() { <-done; c.Close() }()
				tc := c.(*tls.Conn)
				if err := tc.Handshake(); err != nil {
					return
				}
				lane := tc.ConnectionState().NegotiatedProtocol
				// The CONTROL lane drains immediately; the BULK lane drains at the
				// configured rate. That is the asymmetry the real wire has — a
				// congested link is congested for the payload riding it, and the
				// point of a second lane is that the small frame is not on it.
				slice := 4 << 10
				pause := time.Duration(int64(time.Second) * int64(slice) / int64(bytesPerSec))
				if lane == alpnCtrl {
					pause = 0
				}
				buf := make([]byte, slice)
				var hdr [4]byte
				for {
					if _, err := io.ReadFull(tc, hdr[:]); err != nil {
						return
					}
					n := int(binary.BigEndian.Uint32(hdr[:]))
					for left := n; left > 0; {
						take := slice
						if left < take {
							take = left
						}
						if _, err := io.ReadFull(tc, buf[:take]); err != nil {
							return
						}
						left -= take
						if pause > 0 {
							time.Sleep(pause)
						}
					}
					select {
					case lr.arrivals <- laneArrival{size: n, at: time.Now(), lane: lane}:
					default:
					}
				}
			}(c)
		}
	}()
	t.Cleanup(func() { close(done); ln.Close() })
	return lr
}

// A LARGE REPLY MUST REACH AN UNDIALABLE PEER WHOSE BULK CONN HAS DIED.
//
// This is the defect the control lane introduced, and the shape it left behind.
//
// Originally the lane could BE a peer's first contact, so a peer whose first frame
// was small had a control conn and no bulk conn — and a large reply to it, when it
// was also undialable, found no conn, no address, and was dropped. Run
// f34745d-56790 read that as four flows fetching the sha256 of the empty string.
// natted_test.go covers the same undialable-peer shape with SMALL messages and
// stayed green throughout, because the frames it sends are exactly the ones the lane
// carries correctly; only a payload exposes it.
//
// The lane is now supplementary — it is never a first contact, so that state does not
// arise from the normal path. What CAN still arise is the bulk conn dying while the
// lane survives, and the last-resort delivery has to hold there or the same data loss
// comes back through a smaller door. So this constructs that state directly rather
// than relying on an ordering that is now prevented.
func TestALargePayloadReachesAPeerWhoseBulkConnDied(t *testing.T) {
	// The "NATed" node advertises nothing, so the far end can never dial it.
	natted, loopN := newTransportAt(t, 800, "0.0.0.0:0")
	defer func() { natted.Close(); loopN.Stop() }()
	public, loopP := newTransport(t, 801)
	defer func() { public.Close(); loopP.Stop() }()

	got := make(chan int, 4)
	natted.SetHandler(func(_ ports.NodeID, m ports.Message) { got <- len(m.Data) })

	publicID := identity.FromSeed(801).NodeID()
	nattedID := identity.FromSeed(800).NodeID()
	natted.AddPeer(publicID, public.Addr())

	// Two small frames: the first establishes the bulk conversation, the second gets
	// the lane. That ordering is the fix, so the fixture has to honour it.
	for i := 0; i < 2; i++ {
		if err := natted.Send(publicID, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	for deadline := time.Now().Add(10 * time.Second); public.ctrlConn(nattedID) == nil; {
		if time.Now().After(deadline) {
			t.Fatal("PREMISE BROKEN: no control lane was established, so nothing below is about the lane")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Now kill the bulk conversation, leaving only the lane — the state a dropped
	// conn leaves behind for a peer nobody can dial.
	pc := public.liveConn(nattedID)
	if pc == nil {
		t.Fatal("PREMISE BROKEN: no bulk conn to drop, so the fixture is not in the state under test")
	}
	public.dropConn(nattedID, pc)
	if public.liveConn(nattedID) != nil {
		t.Fatal("PREMISE BROKEN: the bulk conn survived the drop")
	}

	// A PAYLOAD, the way a chunk fetch is answered.
	const payload = 256 << 10
	if err := public.Send(nattedID, ports.Message{Kind: ports.MsgFetchChunkReply, Data: make([]byte, payload)}); err != nil {
		t.Fatalf("the large reply was refused outright: %v", err)
	}
	select {
	case n := <-got:
		if n != payload {
			t.Fatalf("the reply arrived with %d bytes, want %d", n, payload)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a large reply to a peer reachable ONLY over the control lane was never delivered. The lane " +
			"must never be the reason a frame is not delivered: head-of-line blocking is a latency cost, and " +
			"dropping the frame is a silent-loss shape (S3)")
	}
}

// AND THE LANE IS NOT A FIRST CONTACT, which is what keeps the case above rare
// instead of routine. A NATed fetcher sends small requests and receives chunk-sized
// replies; if its opening frame took the lane it would never open a bulk conn at
// all, and every reply to it would ride the lane — putting 65 KiB frames on the
// connection whose whole purpose is to carry small ones past them. The field counted
// 26 of 40 sampled lane frames taking that path.
func TestTheFirstFrameToAPeerOpensTheBulkConversationNotTheLane(t *testing.T) {
	tr, loop := newTransport(t, 810)
	defer func() { tr.Close(); loop.Stop() }()
	peer, loopP := newTransport(t, 811)
	defer func() { peer.Close(); loopP.Stop() }()
	peerID := identity.FromSeed(811).NodeID()
	tr.AddPeer(peerID, peer.Addr())

	if err := tr.Send(peerID, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
		t.Fatalf("opening send: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); tr.liveConn(peerID) == nil; {
		if time.Now().After(deadline) {
			t.Fatal("the first small frame opened no BULK conversation — an undialable peer would then have no " +
				"path for a large reply, which is the data loss this ordering exists to prevent")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tr.ctrlConn(peerID) != nil {
		t.Fatal("the FIRST frame to a peer opened a control lane. The lane must be supplementary to an " +
			"established bulk conversation, or a peer that only ever sends small frames never opens one")
	}
	// The second small frame may take the lane: the bulk conversation now exists.
	if err := tr.Send(peerID, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
		t.Fatalf("second send: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); tr.ctrlConn(peerID) == nil; {
		if time.Now().After(deadline) {
			t.Fatal("no lane was ever established, so small frames never get off the payload's connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// newTransportAt is newTransport with an explicit listen address, so a test can
// build a node that advertises nothing dialable.
func newTransportAt(t *testing.T, seed int64, listen string) (*Transport, *eventloop.Loop) {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	tr, err := New(loop, identity.FromSeed(seed), listen)
	if err != nil {
		t.Fatal(err)
	}
	return tr, loop
}
