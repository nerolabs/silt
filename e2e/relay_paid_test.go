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
//	    min(count × increment, Σ face) of the anchors the fetcher's DURABLE identity
//	    bought from the relay (R2.14), the ledger total moved by settled − Σ face ≤ 0
//	    (the unconsumed remainder is burned, never re-minted), and Reputation()
//	    (standing) is UNCHANGED (Invariant-A).
//	(d) FREE RELAY STILL WORKS with payments on — a non-paying peer reaches the origin
//	    through free relay under the same caps (the Option-B witness, D-POD-RELAY-COEXIST).
//	(e) M0 LOG AUDIT — the settlement log line carries no cross-session-correlating field.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
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
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// anchorChainFor builds a chain whose genesis commits key as relay's demand key_0
// (a v5 IssuerKeyReg). Each node gets its OWN instance of the same genesis so no
// chain object is shared across event loops.
func anchorChainFor(t *testing.T, relay *identity.Identity, key *rsa.PrivateKey) *chain.Chain {
	t.Helper()
	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{{Root: ports.HashBytes([]byte("paid-relay-e2e-genesis"))}},
		IssuerKeys: []chain.IssuerKeyReg{chain.SignIssuerKeyReg(relay.Signer(), 0, demand.KeyFingerprint(&key.PublicKey))},
	}
	chain.Sign(&g, relay.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis committing the relay's key_0: %v", err)
	}
	return c
}

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
	const k = 1 // anchors bought for the session; face = the fee = 50,000 increments
	const fee = int64(50_000)

	// ---- Loops + transports for the fetcher's DURABLE identity, the session
	// EPHEMERAL and the relay NODES (the payment lane).
	dLoop, fLoop, rLoop := eventloop.New(), eventloop.New(), eventloop.New()
	go dLoop.Run()
	go fLoop.Run()
	go rLoop.Run()
	t.Cleanup(dLoop.Stop)
	t.Cleanup(fLoop.Stop)
	t.Cleanup(rLoop.Stop)

	dID := identity.FromSeed(9100) // fetcher's DURABLE identity — buys the anchors
	fID := identity.FromSeed(9101) // fetcher's FRESH EPHEMERAL identity — spends them
	rID := identity.FromSeed(9102) // relay operator
	oID := identity.FromSeed(9103) // origin (holds the object)

	dTr, err := tcpnet.New(dLoop, dID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dTr.Close() })
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

	dNode := node.New(dID.NodeID(), node.DefaultConfig(), walltime.New(dLoop), dTr, memstore.New())
	fNode := node.New(fID.NodeID(), node.DefaultConfig(), walltime.New(fLoop), fTr, memstore.New())
	rNode := node.New(rID.NodeID(), node.DefaultConfig(), walltime.New(rLoop), rTr, memstore.New())

	// The relay's chain-committed demand key_0 (R2.14: the anchor lane's precondition
	// — the same v5 IssuerKeyReg the delivery lane needs). The durable fetcher holds
	// its own replica of the same genesis and pins the relay's key against it.
	relayKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rNode.SetSigner(rID.Signer())
	rNode.EnableChain(anchorChainFor(t, rID, relayKey), rID.Signer())
	rNode.SetDemandIssuerKey(rand.Reader, 0, relayKey)
	dNode.SetSigner(dID.Signer())
	dNode.EnableChain(anchorChainFor(t, rID, relayKey), dID.Signer())
	fNode.SetSigner(fID.Signer()) // the ephemeral signs the session-open commitment

	// The relay's ledger + captured log (for assertions c and e). The shipped grant
	// funds the durable buyer's anchor purchase; nothing is pre-funded for the
	// ephemeral, which this ledger never sees as an account.
	ledger := credit.New(fee, 500_000)
	ledger.Register(rID.NodeID())
	ledger.Register(dID.NodeID())
	rNode.SetLedger(ledger)
	log := &capLog{}
	rNode.SetLogger(log)
	rNode.EnableRelayAccept()

	// Route durable ↔ relay (the anchor purchase) and ephemeral ↔ relay (the
	// session) so MsgDemandTokenRequest / MsgRelayOpen / MsgRelayPay flow.
	dTr.AddPeer(rID.NodeID(), rTr.Addr())
	rTr.AddPeer(dID.NodeID(), dTr.Addr())
	fTr.AddPeer(rID.NodeID(), rTr.Addr())
	rTr.AddPeer(fID.NodeID(), fTr.Addr())

	// The ledger is LOOP-ONLY (no mutex): every read is marshaled onto the relay
	// node's loop and returned over a cap-1 reply channel — the production seam's
	// single-thread discipline (Tester finding; a test-only fix, the Ledger stays
	// mutex-free).
	readBalance := func(id ports.NodeID) int64 {
		ch := make(chan int64, 1)
		rLoop.Post("check-balance", func() { ch <- ledger.Balance(id) })
		return <-ch
	}
	readTotal := func() int64 {
		ch := make(chan int64, 1)
		rLoop.Post("check-total", func() {
			var sum int64
			for _, b := range ledger.Balances() {
				sum += b
			}
			ch <- sum
		})
		return <-ch
	}
	readReputation := func() int64 {
		ch := make(chan int64, 1)
		rLoop.Post("check-reputation", func() { ch <- ledger.Reputation(rID.NodeID()) })
		return <-ch
	}
	totalStart := readTotal()
	dStart := readBalance(dID.NodeID())

	// ---- The DURABLE identity pins the relay's committed key and buys k anchors
	// over the wire: the real blind withdrawal, charged by ChargePublish on the
	// RELAY's ledger (the paying ledger is the settling ledger).
	pinned := make(chan error, 1)
	dLoop.Post("pin-keys", func() {
		dNode.FetchDemandIssuerKeys(rID.NodeID(), func(n int, e error) {
			if e == nil && n != 1 {
				e = fmt.Errorf("pinned %d keys, want 1", n)
			}
			pinned <- e
		})
	})
	select {
	case e := <-pinned:
		if e != nil {
			t.Fatalf("durable fetcher could not pin the relay's committed key_0: %v", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pinning the relay's demand key timed out")
	}
	anchorsCh := make(chan []relaypay.Anchor, 1)
	anchorErr := make(chan error, 1)
	dLoop.Post("buy-anchors", func() {
		dNode.AcquireRelayAnchors(rand.Reader, rID.NodeID(), k, func(a []relaypay.Anchor, e error) {
			if e != nil {
				anchorErr <- e
				return
			}
			anchorsCh <- a
		})
	})
	var anchors []relaypay.Anchor
	select {
	case anchors = <-anchorsCh:
	case e := <-anchorErr:
		t.Fatalf("AcquireRelayAnchors: %v", e)
	case <-time.After(10 * time.Second):
		t.Fatal("anchor purchase timed out")
	}
	if len(anchors) != k {
		t.Fatalf("bought %d anchors, want %d", len(anchors), k)
	}
	if got := readBalance(dID.NodeID()); got != dStart-k*fee {
		t.Fatalf("durable buyer's balance on the RELAY's ledger is %d after buying %d anchors, want %d − k·fee = %d", got, k, dStart, dStart-k*fee)
	}
	// Baselines for (c), captured BEFORE the session: the settle fires when the
	// forward stream completes (the paid pump returns on the origin's EOF, not on
	// the fetcher's close), so any later read races a paying settle.
	repBefore := readReputation()
	balStart := readBalance(rID.NodeID())

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
		fNode.OpenRelaySessionRemote(rID.NodeID(), chain.Root(), S, node.FundingEphemeralBlind, anchors, func(h uint64, e error) {
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

	// Close the byte leg (the pump has already returned on the origin's EOF and
	// settled; this is the fetcher's side of the teardown).
	pipe.Close()

	// ---- (c) CONSERVED SETTLE + firewall. Wait for the S5 settlement line, then
	// assert the balance rose by exactly min(count × increment, Σ face) over the
	// pre-session baseline — the anchor-funded payout R2.14 restores (the R0.7
	// interim's wantCredit = 0 is retired; cert
	// R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md
	// §9 build-checked) — and the ledger TOTAL moved by settled − k·fee ≤ 0 across
	// purchase → open → pay → settle: the unconsumed face is burned, never re-minted.
	wantCredit := min(int64(S)*relaypay.RelayIncrementCredit, k*fee)

	deadline := time.Now().Add(5 * time.Second)
	settled := func() bool {
		for _, ln := range log.lines() {
			if strings.Contains(ln, "relay session settled") {
				return true
			}
		}
		return false
	}
	for !settled() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if bal := readBalance(rID.NodeID()); bal != balStart+wantCredit {
		t.Fatalf("operator balance = %d after settle, want %d (+min(count × inc, Σ face) = %d) — got a payout of %d for %d increments", bal, balStart+wantCredit, wantCredit, bal-balStart, S)
	}
	if delta := readTotal() - totalStart; delta != wantCredit-k*fee || delta > 0 {
		t.Fatalf("ledger total moved by %d across purchase → settle, want settled − k·fee = %d (≤ 0; the remainder is burned)", delta, wantCredit-k*fee)
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
		dID.NodeID().String(),            // fetcher DURABLE identity (the anchor buyer)
		rID.NodeID().String(),            // relay identity
		hex.EncodeToString(chain.Root()), // chain root
		hex.EncodeToString(anchors[0].Serial),
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
