// White-box: reaches Transport internals, so it lives in package tcpnet.
package tcpnet

import (
	"crypto/tls"
	"encoding/binary"
	"io"
	"testing"
	"time"

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
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
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
