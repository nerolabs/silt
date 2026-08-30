package e2e

// PoD §7.3 Batch 3 — the paid relay session, end to end over real TCP.
//
// This is the daemon-integration proof for the rendezvous the design flagged as
// having "no unit home": the paid marker on the relay control frame resolving to the
// node-owned authorizer at splice time. It wires the REAL pieces — three nodes over
// real tcpnet transports, a real relay.Server with the paid seam installed, and the
// real byte legs over TCP — and drives the full flow: open (MsgRelayOpen) → paid
// connect (DialThroughPaid) → pay-as-you-go (MsgRelayPay) → settle-at-close.
//
// It is an in-process integration test (not spawned daemons) because the CLI has no
// scriptable paid-relay fetch; the seam under test is the adapter/node rendezvous,
// which this drives directly through the production code (the same shape as tcpnet's
// own cross-network integration tests).
//
// The five assertions (design §4):
//
//	(a) FORWARD INTEGRITY — the fetcher reconstructs the exact object the origin holds.
//	(b) LIVE PAY-GATE — the relay stops forwarding when reveals stop (bounded stiff).
//	(c) CONSERVED SETTLE + firewall — the operator BALANCE rose by exactly
//	    count × increment, and Reputation() (standing) is UNCHANGED (Invariant-A).
//	(d) FREE RELAY STILL WORKS with payments on — a non-paying peer reaches the origin
//	    through free relay under the same caps (the Option-B witness, D-POD-RELAY-COEXIST).
//	(e) M0 LOG AUDIT — the settlement log line carries no cross-session-correlating field.

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/relay"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// capLog collects a node's log lines so the M0 audit can inspect the settlement line.
type capLog struct {
	mu      sync.Mutex
	records []string
}

func (c *capLog) Enabled(ports.LogLevel) bool { return true }
func (c *capLog) Log(_ ports.LogLevel, event string, kv ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	line := event
	for i := 0; i+1 < len(kv); i += 2 {
		line += fmt.Sprintf(" %v=%v", kv[i], kv[i+1])
	}
	c.records = append(c.records, line)
}
func (c *capLog) lines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.records...)
}

// TestPaidRelaySessionEndToEnd is the five-assertion Batch-3 proof.
func TestPaidRelaySessionEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e drives real TCP + loops; skipped under -short")
	}
	const inc = relaypay.RelayIncrementBytes
	const S = 6 // the object is S increments long

	// ---- Loops + transports for the fetcher and relay NODES (the payment lane).
	fLoop, rLoop := eventloop.New(), eventloop.New()
	go fLoop.Run()
	go rLoop.Run()
	t.Cleanup(fLoop.Stop)
	t.Cleanup(rLoop.Stop)

	fID := identity.FromSeed(9101) // fetcher's FRESH EPHEMERAL identity
	rID := identity.FromSeed(9102) // relay operator
	oID := identity.FromSeed(9103) // origin (holds the object)

	fTr, err := tcpnet.New(fLoop, fID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fTr.Close() })
	rTr, err := tcpnet.New(rLoop, rID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rTr.Close() })

	fNode := node.New(fID.NodeID(), node.DefaultConfig(), walltime.New(fLoop), fTr, memstore.New())
	rNode := node.New(rID.NodeID(), node.DefaultConfig(), walltime.New(rLoop), rTr, memstore.New())

	// The relay's ledger + captured log (for assertions c and e).
	ledger := credit.New(50_000, 0)
	// Seed the fetcher's paid-in blind credit (stands in for the blind withdrawal it
	// already funded under its ephemeral identity — the conservation SOURCE).
	ledger.RecordServe(fID.NodeID(), rID.NodeID(), ports.ChunkID{}, 20_000)
	rNode.SetLedger(ledger)
	log := &capLog{}
	rNode.SetLogger(log)
	rNode.EnableRelayAccept()

	// Route the fetcher ↔ relay so MsgRelayOpen / MsgRelayPay flow.
	fTr.AddPeer(rID.NodeID(), rTr.Addr())
	rTr.AddPeer(fID.NodeID(), fTr.Addr())

	// ---- The relay.Server (the byte-forwarding lane) with the PAID SEAM installed.
	relaySrv, err := relay.Serve("127.0.0.1:0", identity.FromSeed(9102), relay.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relaySrv.Close() })
	// Wire the seam exactly as the daemon does (cmd/silt/daemon.go).
	relaySrv.SetPaidResolver(func(fetcher ports.NodeID, handle uint64) (relay.Authorizer, bool) {
		return rNode.ResolveRelayAuthorizer(fetcher, handle)
	})
	relaySrv.SetPaidSettler(rNode.SettleRelaySessionForHandle)

	// ---- The ORIGIN registers on the relay Server and streams the object on connect.
	object := bytes.Repeat([]byte{0xA7}, S*inc)
	freeObject := []byte("free-relay-still-works-under-payments")
	origin, err := relay.NewClient(identity.FromSeed(9103), identity.FromSeed(9102).NodeID(), relaySrv.Addr(), func(conn net.Conn) {
		defer conn.Close()
		// The origin distinguishes the paid fetch from the free fetch by the first
		// byte the connector sends (a tiny handshake standing in for the real e2e TLS
		// handshake): 'P' → stream the paid object, 'F' → stream the free object.
		hdr := make([]byte, 1)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		if hdr[0] == 'F' {
			conn.Write(freeObject)
			return
		}
		conn.Write(object)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go origin.Run(func(e error) { ready <- e })
	t.Cleanup(origin.Close)
	select {
	case e := <-ready:
		if e != nil {
			t.Fatalf("origin registration: %v", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("origin registration timed out")
	}
	_ = oID

	// ---- Build the PayWord chain the fetcher funds (fresh, per-session).
	chain, _ := relaypay.BuildChain([]byte("paid-relay-e2e-fresh-random-tip!!")[:32], S)

	// ---- Open the paid session over the wire (MsgRelayOpen), on the fetcher's loop.
	handleCh := make(chan uint64, 1)
	errCh := make(chan error, 1)
	fLoop.Post("open", func() {
		fNode.OpenRelaySessionRemote(rID.NodeID(), chain.Root(), S, node.FundingEphemeralBlind, func(h uint64, e error) {
			if e != nil {
				errCh <- e
				return
			}
			handleCh <- h
		})
	})
	var handle uint64
	select {
	case handle = <-handleCh:
	case e := <-errCh:
		t.Fatalf("open paid session: %v", e)
	case <-time.After(10 * time.Second):
		t.Fatal("open paid session timed out")
	}

	// ---- Dial the paid byte leg through the relay Server (the marker connect).
	certF, _ := fID.Certificate()
	pipe, err := relay.DialThroughPaid(certF, identity.FromSeed(9102).NodeID(), relaySrv.Addr(), identity.FromSeed(9103).NodeID(), handle)
	if err != nil {
		t.Fatalf("paid connect refused: %v", err)
	}
	t.Cleanup(func() { pipe.Close() })
	pipe.Write([]byte{'P'}) // tell the origin: stream the paid object

	// ---- (b) LIVE PAY-GATE: read the object while revealing preimages increment by
	// increment. Before the first reveal, no bytes past the origin's buffered stiff
	// may cross. After each reveal the next increment unlocks.
	got := make([]byte, 0, len(object))
	buf := make([]byte, inc)

	// First, prove the gate holds with ZERO reveals: nothing (beyond one buffered
	// chunk of stiff) is delivered.
	pipe.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	for {
		n, rerr := pipe.Read(buf)
		got = append(got, buf[:n]...)
		if rerr != nil {
			break
		}
		if len(got) > inc { // more than one buffered increment escaped without payment
			t.Fatalf("live pay-gate broken: %d bytes forwarded with zero reveals (stiff exceeds one increment)", len(got))
		}
	}
	if len(got) > inc {
		t.Fatalf("live pay-gate broken: %d bytes forwarded before any payment", len(got))
	}

	// Now reveal increment by increment; each unlocks one more increment of forward.
	for k := 1; k <= S; k++ {
		revealed := make(chan error, 1)
		kk := k
		fLoop.Post("pay", func() {
			fNode.SubmitRelayPay(rID.NodeID(), handle, chain.Preimage(kk), kk, func(_ int, e error) { revealed <- e })
		})
		select {
		case e := <-revealed:
			if e != nil {
				t.Fatalf("reveal increment %d: %v", k, e)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("reveal increment %d timed out", k)
		}
		// Drain what this increment unlocked (deadline observes the gate stall).
		pipe.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		for {
			n, rerr := pipe.Read(buf)
			got = append(got, buf[:n]...)
			if rerr != nil {
				break
			}
		}
		if int64(len(got)) > int64(k)*inc+inc {
			t.Fatalf("increment %d: %d bytes forwarded, ceiling is %d (+1 buffered) — pay-gate overshoots", k, len(got), int64(k)*inc)
		}
	}
	// All increments revealed: the full object must now be deliverable.
	pipe.SetReadDeadline(time.Now().Add(3 * time.Second))
	for len(got) < len(object) {
		n, rerr := pipe.Read(buf)
		got = append(got, buf[:n]...)
		if rerr != nil {
			t.Fatalf("after full payment the forward stalled at %d/%d bytes: %v", len(got), len(object), rerr)
		}
	}

	// (a) FORWARD INTEGRITY.
	if !bytes.Equal(got, object) {
		t.Fatalf("forward object corrupted through the paid splice (got %d bytes)", len(got))
	}

	// Close the byte leg → the pump returns → the settler fires at close.
	pipe.Close()

	// ---- (c) CONSERVED SETTLE + firewall. The ledger is LOOP-ONLY by design (no
	// mutex): the event-loop goroutine writes Balance via RedeemRelayCredit at settle.
	// Reading it directly from THIS test goroutine would race that write (Tester
	// finding). So every ledger read below is MARSHALED onto the relay node's loop and
	// the value returned over a cap-1 reply channel — the same single-thread discipline
	// the production seam uses. This is a TEST-ONLY fix; the production Ledger stays
	// mutex-free (the loop-only invariant is not weakened).
	readBalance := func() int64 {
		ch := make(chan int64, 1)
		rLoop.Post("check-balance", func() { ch <- ledger.Balance(rID.NodeID()) })
		return <-ch
	}
	readReputation := func() int64 {
		ch := make(chan int64, 1)
		rLoop.Post("check-reputation", func() { ch <- ledger.Reputation(rID.NodeID()) })
		return <-ch
	}

	// Capture standing BEFORE settle so we can assert it is unchanged, then wait for
	// the balance to reflect the settle.
	repBefore := readReputation()
	balStart := int64(0) // the relay operator started at zero balance
	wantCredit := int64(S) * relaypay.RelayIncrementCredit
	deadline := time.Now().Add(5 * time.Second)
	for readBalance() < balStart+wantCredit && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if bal := readBalance(); bal != balStart+wantCredit {
		t.Fatalf("operator balance = %d after settle, want %d (= %d increments × %d) — settle not conserved", bal, balStart+wantCredit, S, relaypay.RelayIncrementCredit)
	}
	if rep := readReputation(); rep != repBefore {
		t.Fatalf("relay Reputation() moved from %d to %d on a relay settlement — the Invariant-A firewall is breached (relay credit must be balance-only)", repBefore, rep)
	}

	// ---- (e) M0 LOG AUDIT: the settlement line carries no cross-session-correlating
	// field (no ephemeral/durable identity, no chain root). It logs only per-session
	// values (increments, credit).
	var settleLine string
	for _, ln := range log.lines() {
		if strings.Contains(ln, "relay session settled") {
			settleLine = ln
		}
	}
	if settleLine == "" {
		t.Fatal("no settlement log line found")
	}
	for _, forbidden := range []string{
		fID.NodeID().String(),            // fetcher ephemeral identity
		rID.NodeID().String(),            // relay identity
		hex.EncodeToString(chain.Root()), // chain root
	} {
		if forbidden != "" && strings.Contains(settleLine, forbidden) {
			t.Fatalf("settlement log line leaks a cross-session-correlating field %q: %q", forbidden, settleLine)
		}
	}

	// ---- (d) FREE RELAY STILL WORKS with payments on (the Option-B witness). A
	// second connector reaches the origin through FREE relay (no paid marker), under
	// the same caps, while --accept-relay-payments is on.
	certFree, _ := identity.FromSeed(9199).Certificate()
	freePipe, err := relay.DialThrough(certFree, identity.FromSeed(9102).NodeID(), relaySrv.Addr(), identity.FromSeed(9103).NodeID())
	if err != nil {
		t.Fatalf("FREE relay refused while payments are on — Option B violated (free must be unchanged): %v", err)
	}
	defer freePipe.Close()
	freePipe.Write([]byte{'F'})
	freeGot := make([]byte, len(freeObject))
	freePipe.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(freePipe, freeGot); err != nil {
		t.Fatalf("free relay fetch failed with payments on: %v", err)
	}
	if !bytes.Equal(freeGot, freeObject) {
		t.Fatalf("free relay object corrupted: got %q", freeGot)
	}
}
