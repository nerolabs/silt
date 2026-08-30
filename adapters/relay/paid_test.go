package relay

// PoD §7.3 transport Batch 2 — the paid forwarding pump (step 4).
//
// Two hazards this file pins failing-first:
//
//  1. PAY-THEN-FORWARD (the gate). The relay forwards increment k only after the
//     fetcher authorizes it (a preimage reveal, modeled here as an authorizer's
//     byte ceiling rising). If the fetcher stops authorizing, the relay stops
//     forwarding — the irreducible stiff is bounded to ONE increment.
//
//  2. SPLICE-EOF SURVIVAL (the sharpest transport hazard, design §2). The existing
//     splice closes BOTH conns on the FIRST EOF because swarm exchanges are
//     short-lived (server.go). A paid ≤1 GiB relay session is NOT short-lived: a
//     reverse-direction EOF must NOT tear down the paid forward stream. The naive
//     splice reddens this test.

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/ports"
)

// staticAuthorizer is a test authorizer whose ceiling a test raises step by step,
// signalling waiters each time. It stands in for the node-side verifier that the
// live path updates on each MsgRelayPay (design §2 Option A: the node drives, the
// adapter gates).
type staticAuthorizer struct {
	mu     sync.Mutex
	bytes  int64
	closed bool
	signal chan struct{}
}

func newStaticAuthorizer() *staticAuthorizer {
	return &staticAuthorizer{signal: make(chan struct{}, 1)}
}

func (a *staticAuthorizer) AuthorizedBytes() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bytes
}

func (a *staticAuthorizer) Done() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

// Wait blocks until the ceiling may have advanced or the session closed.
func (a *staticAuthorizer) Wait() <-chan struct{} { return a.signal }

func (a *staticAuthorizer) authorize(n int64) {
	a.mu.Lock()
	a.bytes = n
	a.mu.Unlock()
	a.wake()
}

func (a *staticAuthorizer) close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wake()
}

func (a *staticAuthorizer) wake() {
	select {
	case a.signal <- struct{}{}:
	default:
	}
}

// TestPaidPumpForwardsOnlyAuthorizedBytes is failing-first test (1): the paid
// pump forwards up to the authorizer's ceiling and no further. With the ceiling
// at one increment, exactly one increment of payload crosses; the pump then
// blocks (stiff bounded to one increment) until the ceiling rises. When the
// fetcher stops authorizing, the reader on the far side sees no more than the
// authorized bytes.
//
// paidPump(dst, src, auth, cap) reads from the origin (src) and writes to the
// fetcher (dst), gated on auth's authorized-byte ceiling.
func TestPaidPumpForwardsOnlyAuthorizedBytes(t *testing.T) {
	const inc = 4096
	// origin (src of forwarded content) and fetcher (dst) sides, over net.Pipes.
	originR, originW := net.Pipe() // origin writes payload into originW; pump reads originR
	fetchR, fetchW := net.Pipe()   // pump writes into fetchW; fetcher reads fetchR
	defer originR.Close()
	defer originW.Close()
	defer fetchR.Close()
	defer fetchW.Close()

	auth := newStaticAuthorizer()
	auth.authorize(inc) // authorize exactly one increment

	go paidPump(fetchW, originR, auth, 8*inc)

	// The origin has 8 increments ready to send.
	payload := bytes.Repeat([]byte{0x5a}, 8*inc)
	go func() { originW.Write(payload); originW.Close() }()

	// The fetcher reads. With one increment authorized, it must receive exactly
	// one increment and then stall (no more bytes until authorization rises).
	got := make([]byte, 0, 8*inc)
	buf := make([]byte, inc)
	fetchR.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	for {
		n, err := fetchR.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break // deadline: the pump stalled at the ceiling
		}
		if len(got) >= 8*inc {
			break
		}
	}
	// The stiff is bounded to ONE increment: with one authorized, the fetcher
	// receives one increment of payload and no more.
	if len(got) > inc {
		t.Fatalf("paid pump forwarded %d bytes with only %d authorized — the pay-gate is missing (stiff exceeds one increment)", len(got), inc)
	}
	if len(got) != inc {
		t.Fatalf("paid pump forwarded %d bytes, want exactly one increment %d", len(got), inc)
	}

	// Now authorize the rest; the pump must deliver the remaining increments.
	auth.authorize(8 * inc)
	fetchR.SetReadDeadline(time.Now().Add(2 * time.Second))
	for len(got) < 8*inc {
		n, err := fetchR.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			t.Fatalf("after full authorization the pump stalled at %d/%d bytes: %v", len(got), 8*inc, err)
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload corrupted through the paid pump")
	}
}

// TestPaidPumpSurvivesReverseEOF is failing-first test (2): the SPLICE-EOF
// GOTCHA. In a paid session the reverse direction (fetcher → origin) may close
// while the paid forward direction (origin → fetcher) still has authorized bytes
// to deliver. The paid pump must NOT tear down the forward stream when the
// reverse direction hits EOF. The naive splice (both-close-on-first-EOF) reddens
// this: the forward payload is truncated the instant the reverse side closes.
func TestPaidPumpSurvivesReverseEOF(t *testing.T) {
	const inc = 4096
	// The relay holds two conns: one to the origin (ao) and one to the fetcher
	// (bf). paidSession forwards ao→bf (PAID, gated) and bf→ao (reverse control).
	// Real TCP loopback conns so the reverse side can HALF-close (CloseWrite) —
	// the exact shape a fetcher makes when it finishes its reverse chatter early
	// while still downloading on the forward direction.
	ao, originEnd := tcpPair(t) // relay↔origin
	bf, fetchEnd := tcpPair(t)  // relay↔fetcher
	defer ao.Close()
	defer originEnd.Close()
	defer bf.Close()
	defer fetchEnd.Close()

	auth := newStaticAuthorizer()
	auth.authorize(4 * inc) // authorize the full forward payload

	// Run the bidirectional paid session (forward = ao→bf gated, reverse = bf→ao).
	go paidSession(ao, bf, auth, 8*inc)

	// The origin drains the reverse lane so the reverse pump can complete.
	go func() { io.Copy(io.Discard, originEnd) }()
	// The reverse side (fetcher → origin) sends a byte then HALF-CLOSES its write
	// side: reverse EOF while the forward download is still live.
	fetchEnd.Write([]byte{0x01})
	fetchEnd.CloseWrite()

	// The forward payload: 4 increments the origin streams AFTER the reverse EOF.
	payload := bytes.Repeat([]byte{0x77}, 4*inc)
	go func() {
		time.Sleep(100 * time.Millisecond) // let the reverse EOF land first
		originEnd.Write(payload)
	}()

	// The fetcher must still receive the FULL forward payload despite the reverse
	// EOF. With the naive both-close-on-first-EOF pump, the forward stream is torn
	// down when the reverse side closes and this read is truncated.
	got := make([]byte, 0, len(payload))
	buf := make([]byte, inc)
	fetchEnd.SetReadDeadline(time.Now().Add(2 * time.Second))
	for len(got) < len(payload) {
		n, err := fetchEnd.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break
		}
	}
	if len(got) != len(payload) {
		t.Fatalf("reverse-EOF tore down the paid forward stream: got %d/%d bytes (the splice-EOF gotcha is unhandled)", len(got), len(payload))
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("forward payload corrupted after reverse EOF")
	}
}

// TestSplicePaidFullSessionOverTCP is the integration path (feasible-e2e): a real
// paid relay session over TCP through the Server's SplicePaid, driven by a stepped
// authorizer that mimics the node raising the ceiling on each verified MsgRelayPay.
// It forwards a multi-increment object pay-as-you-go, survives a reverse-EOF
// mid-session, and reports exactly the forwarded byte count as the settlement basis.
func TestSplicePaidFullSessionOverTCP(t *testing.T) {
	const inc = 4096
	const increments = 5
	srv := &Server{cfg: Config{}.withDefaults(), perPeer: make(map[ports.NodeID]int)}

	// The relay's two conns: to the origin (ao) and to the fetcher (bf).
	ao, originEnd := tcpPair(t)
	bf, fetchEnd := tcpPair(t)
	defer originEnd.Close()
	defer fetchEnd.Close()

	auth := newStaticAuthorizer()
	var forwarded int64
	pumpDone := make(chan struct{})
	go func() {
		forwarded = srv.SplicePaid(ports.NodeID{0x01}, ao, bf, auth)
		close(pumpDone)
	}()

	// The origin streams the whole object up-front; the relay releases it only as
	// the fetcher pays.
	payload := bytes.Repeat([]byte{0x33}, increments*inc)
	go func() { originEnd.Write(payload); originEnd.CloseWrite() }()

	// The fetcher's reverse control lane sends an ack then half-closes early — the
	// forward object must keep flowing past this reverse EOF.
	fetchEnd.Write([]byte("ack"))
	fetchEnd.CloseWrite()

	// Read the forward object while stepping authorization one increment at a time,
	// exactly as MsgRelayPay reveals would drive the node's raiseCeiling.
	got := make([]byte, 0, len(payload))
	buf := make([]byte, inc)
	for k := 1; k <= increments; k++ {
		auth.authorize(int64(k) * inc) // pay increment k
		fetchEnd.SetReadDeadline(time.Now().Add(2 * time.Second))
		for int64(len(got)) < int64(k)*inc {
			n, err := fetchEnd.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil {
				t.Fatalf("increment %d: forward read stalled at %d bytes: %v", k, len(got), err)
			}
		}
	}
	auth.close() // session closing
	<-pumpDone

	if !bytes.Equal(got, payload) {
		t.Fatalf("forward object corrupted over the paid TCP splice")
	}
	if forwarded != int64(len(payload)) {
		t.Fatalf("SplicePaid reported %d forwarded bytes, want %d (the settlement basis)", forwarded, len(payload))
	}
}

// tcpPair returns a connected pair of TCP conns over loopback. Both support
// CloseWrite (half-close), which net.Pipe does not — needed to model a reverse
// EOF that leaves the forward direction live.
func tcpPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	return client.(*net.TCPConn), r.c.(*net.TCPConn)
}
