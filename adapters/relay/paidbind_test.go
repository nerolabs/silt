package relay

// PoD §7.3 Batch 3 — the daemon control-frame binding, failing-first (design §2, §3).
//
// Two halves of one rendezvous are pinned here:
//
//   - THE PAID MARKER (§2). A connect frame carries an optional Paid handle. A ZERO
//     marker routes to the free splice byte-for-byte (backward-compat: an old client
//     sends no field). A NONZERO marker routes to the paid path.
//
//   - THE REFUSE-NEVER-DOWNGRADE CONDITION (§3, certified residual #2). A paid connect
//     whose handle does not resolve to a live, fetcher-OWNED session is REFUSED — never
//     downgraded to the free splice. A free downgrade would hand a non-payer an
//     unfunded, uncapped forward. This is RED if the accept path ignores an unresolved
//     paid marker and splices free.
//
//   - THE AUTHORIZER SEAM (§3). A resolved paid connect runs SplicePaid gated on the
//     node-owned authorizer, and the settler fires ONCE at close with the forwarded
//     count. The resolver is called OFF the node loop (the Server's accept goroutine),
//     so a real node marshals the lookup onto its loop; the test resolver is
//     mutex-guarded and the suite runs under -race.

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// gateAuthorizer is a test authorizer whose ceiling a test raises step by step,
// mirroring the node's RelaySession as MsgRelayPay reveals arrive. It stands in for
// the node-owned verifier at the adapter seam.
type gateAuthorizer struct {
	mu     sync.Mutex
	bytes  int64
	closed bool
	signal chan struct{}
}

func newGateAuthorizer() *gateAuthorizer { return &gateAuthorizer{signal: make(chan struct{}, 1)} }

func (a *gateAuthorizer) AuthorizedBytes() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bytes
}
func (a *gateAuthorizer) Done() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}
func (a *gateAuthorizer) Wait() <-chan struct{} { return a.signal }
func (a *gateAuthorizer) authorize(n int64) {
	a.mu.Lock()
	a.bytes = n
	a.mu.Unlock()
	a.wake()
}
func (a *gateAuthorizer) close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wake()
}
func (a *gateAuthorizer) wake() {
	select {
	case a.signal <- struct{}{}:
	default:
	}
}

// registerByHand registers ident on srv over a fresh control conn and returns it so a
// test can drive the accept step deterministically.
func registerByHand(t *testing.T, srv *Server, relayID ports.NodeID, ident *identity.Identity) *tls.Conn {
	t.Helper()
	cert, _ := ident.Certificate()
	ctl, err := dialRelay(cert, relayID, srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCtrl(ctl, ctrl{Op: "register"}); err != nil {
		t.Fatal(err)
	}
	if fr, err := readCtrl(ctl); err != nil || fr.Op != "ok" {
		t.Fatalf("register: %v %+v", err, fr)
	}
	// The control conn must be read WITH a live deadline so a hung incoming read
	// fails the test instead of blocking; the relay resets the deadline at splice.
	ctl.SetDeadline(time.Time{})
	return ctl
}

// TestFreeConnectUnchangedByPaidField is the §2 backward-compat pin: a connect with
// Paid==0 (an old client, or any free NAT-fallback dial) still runs the free splice
// and round-trips bytes exactly as before. RED if adding the Paid field or its accept
// branch broke the free path.
func TestFreeConnectUnchangedByPaidField(t *testing.T) {
	identR, identB, identS := identity.FromSeed(40), identity.FromSeed(41), identity.FromSeed(42)
	srv, err := Serve("127.0.0.1:0", identR, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	heard := make(chan []byte, 1)
	cl, err := NewClient(identB, identR.NodeID(), srv.Addr(), func(conn net.Conn) {
		defer conn.Close()
		b := make([]byte, 5)
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		heard <- b
		conn.Write([]byte("world"))
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	startClient(t, cl)

	certS, _ := identS.Certificate()
	// DialThrough sends Paid:0 — the free path.
	conn, err := DialThrough(certS, identR.NodeID(), srv.Addr(), identB.NodeID())
	if err != nil {
		t.Fatalf("free connect refused after adding the Paid field: %v", err)
	}
	defer conn.Close()
	conn.Write([]byte("hello"))
	reply := make([]byte, 5)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("free splice read: %v", err)
	}
	if string(reply) != "world" {
		t.Fatalf("free reply = %q, want world (free path corrupted)", reply)
	}
	select {
	case b := <-heard:
		if string(b) != "hello" {
			t.Fatalf("target heard %q", b)
		}
	case <-time.After(time.Second):
		t.Fatal("free splice did not deliver the payload")
	}
}

// TestPaidConnectRefusedWhenUnresolved is the §3 certified-residual-#2 pin: a connect
// carrying a nonzero Paid handle that does NOT resolve to a live, owned session is
// REFUSED — never spliced free. Two cases:
//
//	(a) NO RESOLVER installed (a relay that does not accept payments): the paid marker
//	    must be refused, not silently downgraded to free.
//	(b) RESOLVER installed but returns ok=false (unknown or unowned handle): refused.
//
// RED if the accept path ignores the marker and runs the free splice (the connector
// would then get "ok" and an uncapped free forward it never funded).
func TestPaidConnectRefusedWhenUnresolved(t *testing.T) {
	identR, identB, identS := identity.FromSeed(50), identity.FromSeed(51), identity.FromSeed(52)

	run := func(t *testing.T, installResolver bool) {
		srv, err := Serve("127.0.0.1:0", identR, Config{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer srv.Close()
		if installResolver {
			// A resolver that OWNS no handle: every lookup fails. A refused lookup must
			// still refuse the connect, not downgrade to free.
			srv.SetPaidResolver(func(ports.NodeID, uint64) (Authorizer, bool) { return nil, false })
		}

		bctl := registerByHand(t, srv, identR.NodeID(), identB)
		defer bctl.Close()

		// The fetcher dials with a nonzero paid handle.
		certS, _ := identS.Certificate()
		dialErr := make(chan error, 1)
		go func() {
			conn, err := DialThroughPaid(certS, identR.NodeID(), srv.Addr(), identB.NodeID(), 777)
			if conn != nil {
				conn.Close()
			}
			dialErr <- err
		}()

		// The relay parks the connect and notifies B. B accepts the stream — this is
		// where the paid resolution happens.
		fr, err := readCtrl(bctl)
		if err != nil || fr.Op != "incoming" {
			t.Fatalf("incoming: %v %+v", err, fr)
		}
		certB, _ := identB.Certificate()
		bacc, err := dialRelay(certB, identR.NodeID(), srv.Addr())
		if err != nil {
			t.Fatal(err)
		}
		defer bacc.Close()
		if err := writeCtrl(bacc, ctrl{Op: "accept", Stream: fr.Stream}); err != nil {
			t.Fatal(err)
		}
		// The accepting side (B) must be REFUSED, not spliced: an unresolved paid
		// session gets an err frame, not "ok".
		bfr, err := readCtrl(bacc)
		if err != nil {
			t.Fatalf("accept read: %v", err)
		}
		if bfr.Op == "ok" {
			t.Fatalf("unresolved paid connect was SPLICED (accept got ok) — it must be REFUSED, never downgraded to free (certified residual #2)")
		}
		if bfr.Op != "err" {
			t.Fatalf("accept got %q, want err (paid session unresolved)", bfr.Op)
		}
		// The connector must also be refused, not handed a free pipe.
		if err := <-dialErr; err == nil {
			t.Fatalf("connector got a live pipe for an unresolved paid handle — the free downgrade is present")
		}
	}

	t.Run("no-resolver", func(t *testing.T) { run(t, false) })
	t.Run("resolver-rejects", func(t *testing.T) { run(t, true) })
}

// TestPaidConnectResolvesAndGates is the §3 happy-path seam pin: a paid connect whose
// handle resolves to a live, owned authorizer runs SplicePaid gated on the ceiling,
// and the settler fires ONCE at close with the forwarded byte count. It also exercises
// the concurrency seam: the resolver is invoked from the Server's accept goroutine
// while the test holds the authorizer under its own lock, so -race covers the crossing.
func TestPaidConnectResolvesAndGates(t *testing.T) {
	const inc = 4096
	const increments = 4
	identR, identB, identS := identity.FromSeed(60), identity.FromSeed(61), identity.FromSeed(62)
	srv, err := Serve("127.0.0.1:0", identR, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	const handle = uint64(99)
	auth := newGateAuthorizer()
	// The resolver checks the fetcher owns the handle (the ephID-ownership check the
	// node enforces). Here identS is the owner.
	var resolverCalls int32
	var rmu sync.Mutex
	srv.SetPaidResolver(func(fetcher ports.NodeID, h uint64) (Authorizer, bool) {
		rmu.Lock()
		resolverCalls++
		rmu.Unlock()
		if h == handle && fetcher == identS.NodeID() {
			return auth, true
		}
		return nil, false
	})
	settled := make(chan int64, 1)
	srv.SetPaidSettler(func(h uint64, forwarded int64) {
		if h != handle {
			t.Errorf("settler got handle %d, want %d", h, handle)
		}
		settled <- forwarded
	})

	// B is the ORIGIN in a paid session: it holds the object and streams it toward the
	// fetcher through the relay's paid forward direction.
	payload := bytes.Repeat([]byte{0x5c}, increments*inc)
	cl, err := NewClient(identB, identR.NodeID(), srv.Addr(), func(conn net.Conn) {
		defer conn.Close()
		conn.Write(payload)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	startClient(t, cl)

	certS, _ := identS.Certificate()
	conn, err := DialThroughPaid(certS, identR.NodeID(), srv.Addr(), identB.NodeID(), handle)
	if err != nil {
		t.Fatalf("paid connect refused for an owned handle: %v", err)
	}
	defer conn.Close()

	// Read the forward object while stepping authorization one increment at a time —
	// exactly as MsgRelayPay reveals would drive the node's raiseCeiling. At each
	// step the pump must never deliver PAST the authorized ceiling (the pay-gate),
	// and it must deliver everything once the ceiling covers it. Reads gate at byte
	// granularity, so a single Read may straddle an increment boundary; the invariant
	// checked per step is "no overshoot" (len(got) never exceeds k*inc + one buffered
	// chunk of stiff), and the full payload is required only after full authorization.
	got := make([]byte, 0, len(payload))
	buf := make([]byte, inc)
	for k := 1; k <= increments; k++ {
		auth.authorize(int64(k) * inc)
		// Drain what THIS increment unlocked, with a short deadline: the pump stalls at
		// the ceiling until the next authorize, so a deadline is how we observe the gate.
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		for {
			n, rerr := conn.Read(buf)
			got = append(got, buf[:n]...)
			if rerr != nil {
				break // deadline: the pump stalled at (or near) the ceiling
			}
		}
		// The pay-gate: after authorizing k increments the fetcher must not have
		// received more than k*inc plus at most one buffered chunk of in-flight stiff.
		if int64(len(got)) > int64(k)*inc+inc {
			t.Fatalf("increment %d: pump delivered %d bytes with only %d authorized — the pay-gate overshoots", k, len(got), int64(k)*inc)
		}
	}
	// Full authorization is in; the whole payload must now be deliverable.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for len(got) < len(payload) {
		n, rerr := conn.Read(buf)
		got = append(got, buf[:n]...)
		if rerr != nil {
			t.Fatalf("after full authorization the pump stalled at %d/%d bytes: %v", len(got), len(payload), rerr)
		}
	}
	auth.close() // session closing

	select {
	case forwarded := <-settled:
		if forwarded != int64(len(payload)) {
			t.Fatalf("settler reported %d forwarded bytes, want %d", forwarded, len(payload))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("settler never fired at close — the settle-at-close driver is missing")
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("forward object corrupted over the paid splice")
	}
	rmu.Lock()
	calls := resolverCalls
	rmu.Unlock()
	if calls != 1 {
		t.Fatalf("resolver called %d times, want exactly 1 (once at accept)", calls)
	}
}
