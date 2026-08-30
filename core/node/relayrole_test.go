package node

// PoD relay lane — the relay-accept role and its two M0 guards
// (docs/design/pod.md §7.3, certified 2026-08-30). A relay opts into accepting
// sender-funded PayWord chains behind a flag (mirror of --accept-delivery-
// receipts / EnableDemandBank). The two guards are bright-line, non-negotiable
// M0 access-privacy constraints (immutable Don't-#3): the relay must reject any
// chain funded by a durable-account credit, and any session that reuses an
// ephemeral identity or a chain across sessions.

import (
	"net"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

func relayTestID(b byte) ports.NodeID { return ports.HashBytes([]byte{b}) }

// setRelayEpochFnForTest overrides the relay eviction epoch source so a test can
// drive epoch rotation without wiring a full chain (#645).
func (n *Node) setRelayEpochFnForTest(f func() uint64) { n.relayEpochFn = f }

// relaySeenEntryCountForTest returns the total live seen-map entries across both
// relay maps — the bound the #645 eviction test asserts stays small.
func (n *Node) relaySeenEntryCountForTest() int {
	return len(n.relaySeenEph) + len(n.relaySeenRoot)
}

// relayEvictionFloorForTest exposes the monotonic eviction floor so the #645 test
// can assert it never lowers on a reorg.
func (n *Node) relayEvictionFloorForTest() uint64 { return n.relayEvictionFloor }

// relayLiveSessionCountForTest returns the number of live paid-relay sessions in
// the table — the bound the Batch-2 leak-fix flood test asserts stays capped.
func (n *Node) relayLiveSessionCountForTest() int { return len(n.relaySessions) }

// newRelayTestNode builds a node with a sim transport so New does not panic on a
// nil transport. The relay guards are pure per-node state logic; the transport
// is only there to satisfy the constructor.
func newRelayTestNode(idSeed int64) *Node {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(idSeed)
	return New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
}

// TestRelayAcceptOffByDefault: a node does not accept PayWord chains until the
// gate is enabled — the mirror of the delivery-receipt gate being off by
// default. OpenRelaySession returns an error while the gate is off.
func TestRelayAcceptOffByDefault(t *testing.T) {
	n := newRelayTestNode(1)
	c, _ := relaypay.BuildChain([]byte("a-fresh-random-tip-for-the-session"), 8)
	eph := relayTestID(9)
	_, err := n.OpenRelaySession(eph, c.Root(), 8, FundingEphemeralBlind)
	if err == nil {
		t.Fatalf("OpenRelaySession succeeded with the relay-accept gate OFF — it must be gated")
	}
}

// TestRelayGuardI_DurableFundingRejected is M0 guard (i), failing-first: a chain
// root funded by a DURABLE-account credit (not the ephemeral blind path) must be
// REJECTED. Binding a chain to a durable identity turns the relay into a party
// that can tie the fetcher's durable identity to what it fetched — an M0
// access-privacy violation, not permitted at any performance price.
func TestRelayGuardI_DurableFundingRejected(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()
	c, _ := relaypay.BuildChain([]byte("a-fresh-random-tip-for-the-session"), 8)
	eph := relayTestID(9)

	// A durable-funded chain MUST be rejected.
	if _, err := n.OpenRelaySession(eph, c.Root(), 8, FundingDurableAccount); err == nil {
		t.Fatalf("guard (i) missing: a chain funded by a DURABLE-account credit was accepted — M0 Don't-#3 violation")
	}
	// The ephemeral-blind path is the ONLY accepted funding source.
	if _, err := n.OpenRelaySession(eph, c.Root(), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("the ephemeral-blind funding path must be accepted: %v", err)
	}
}

// TestRelayGuardII_EphemeralReuseRejected is M0 guard (ii), failing-first: a
// session-open reusing an ephemeral identity OR a chain across sessions must be
// REJECTED. Reuse upgrades the relay from a per-session observer to a
// longitudinal one — a real Don't-#3 regression.
func TestRelayGuardII_EphemeralReuseRejected(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	eph := relayTestID(9)
	c1, _ := relaypay.BuildChain([]byte("session-one-fresh-random-tip-value"), 8)
	c2, _ := relaypay.BuildChain([]byte("session-two-fresh-random-tip-value"), 8)

	if _, err := n.OpenRelaySession(eph, c1.Root(), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("first session for a fresh ephemeral identity must open: %v", err)
	}
	// Reusing the SAME ephemeral identity for a second session must be rejected.
	if _, err := n.OpenRelaySession(eph, c2.Root(), 8, FundingEphemeralBlind); err == nil {
		t.Fatalf("guard (ii) missing: a REUSED ephemeral identity opened a second session — longitudinal-linkage regression")
	}

	// And reusing the SAME chain root under a fresh ephemeral identity must also
	// be rejected — a chain must not span sessions.
	fresh := relayTestID(10)
	if _, err := n.OpenRelaySession(fresh, c1.Root(), 8, FundingEphemeralBlind); err == nil {
		t.Fatalf("guard (ii) missing: a REUSED chain root opened a second session — a chain must not span sessions")
	}
}

// TestOpenRelaySessionClampsChainLength is the #644 open-side clamp, failing-first:
// a fetcher cannot open a session claiming a chain LONGER than the relay will ever
// forward (S > S_max = MaxSessionBytes / RelayIncrementBytes = 262,144). S is
// derived relay-side from the relay's own config and the protocol increment, never
// trusted from the fetcher. Rejecting an oversized S at open is what bounds the
// stored S the AdvanceTo walk is clamped against.
//
// Removing the `S > relaypay.MaxChainLength` clamp lets a fetcher open a session
// with an arbitrarily large S, re-opening the unbounded-walk DoS — this test turns
// RED.
func TestOpenRelaySessionClampsChainLength(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	// A chain the fetcher CLAIMS is one past the relay's ceiling. The root value is
	// irrelevant to the clamp — the reject happens on S before any chain work.
	eph := relayTestID(9)
	root := make([]byte, 32)
	oversized := relaypay.MaxChainLength + 1
	if _, err := n.OpenRelaySession(eph, root, oversized, FundingEphemeralBlind); err == nil {
		t.Fatalf("OpenRelaySession accepted S=%d > S_max=%d: the #644 open-side clamp is missing", oversized, relaypay.MaxChainLength)
	}

	// Exactly S_max is accepted (inclusive ceiling).
	eph2 := relayTestID(10)
	if _, err := n.OpenRelaySession(eph2, root, relaypay.MaxChainLength, FundingEphemeralBlind); err != nil {
		t.Fatalf("OpenRelaySession rejected S == S_max=%d, which must be accepted: %v", relaypay.MaxChainLength, err)
	}
}

// TestRelaySessionPayCannotExceedChainLength is the #644 pay-side budget cap on the
// live session path, failing-first. It exercises the exact hole the exhaustion
// guard closes: after the chain is exhausted at count == S, the held preimage is
// x_S = H(tip), so a fetcher who reveals the raw TIP hashes to x_S and — without
// the guard — pushes count to S+1, an unfunded increment past the committed budget.
// The count must never exceed S: count is the monotonic settlement accumulator, so
// count * increment <= S * increment <= the fetcher's paid-in credit. If a tip
// reveal could push count to S+1, close-time settlement would over-redeem.
//
// Removing the `count >= S` exhaustion guard in Verifier.Advance turns this RED
// (the tip reveal advances count to S+1).
func TestRelaySessionPayCannotExceedChainLength(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	const S = 4
	// A 32-byte tip so the tip itself is a valid-length preimage to reveal past S.
	tip := []byte("a-fresh-32-byte-tip-for-exhaustcap")[:32]
	c, _ := relaypay.BuildChain(tip, S)
	eph := relayTestID(9)
	sess, err := n.OpenRelaySession(eph, c.Root(), S, FundingEphemeralBlind)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Pay all S increments; each is valid.
	for k := 1; k <= S; k++ {
		if err := sess.Pay(c.Preimage(k)); err != nil {
			t.Fatalf("increment %d: %v", k, err)
		}
	}
	if sess.Count() != S {
		t.Fatalf("count %d after S increments, want %d", sess.Count(), S)
	}
	// The fetcher now reveals the raw TIP. H(tip) == x_S == the held preimage, so
	// without the exhaustion guard this advances count to S+1 (an unfunded increment).
	if err := sess.Pay(tip); err == nil {
		t.Fatalf("Pay(tip) past the committed chain length S=%d was accepted — the budget cap is open (count now %d)", S, sess.Count())
	}
	if sess.Count() != S {
		t.Fatalf("a rejected past-S Pay moved the count to %d (want %d)", sess.Count(), S)
	}
}

// TestRelaySeenMapEvictsOnEpoch is the #645 failing-first test: the relay-seen
// maps must be EPOCH-tied and MONOTONIC, not raw FIFO. It pins three properties
// with an ablatable RED for each:
//
//  1. BOUNDED — entries admitted two-or-more epochs ago are swept, so the maps do
//     not grow unboundedly across epochs. (Ablation: remove the sweep → the map
//     keeps every entry and the size assertion reddens.)
//  2. GUARD (ii) HELD ACROSS THE BOUNDARY — a root/identity admitted in the
//     PREVIOUS epoch is STILL rejected on reuse after one epoch advance. Eviction
//     that is too eager (drops the previous epoch) re-opens the longitudinal
//     linkage the guard forecloses. (Ablation: retention window 0 → a
//     previous-epoch reuse slips through and reddens.)
//  3. MONOTONIC FLOOR (reorg-safe) — after advancing the epoch and then a reorg
//     that moves the epoch back, the eviction floor does NOT lower, so an entry
//     evicted at the higher epoch is not re-admitted below the floor. (Ablation:
//     let the floor track the epoch down → the floor lowers on reorg.)
//
// The epoch is injected via the test seat's relayEpochFn override so the test does
// not need a full chain wired; production reads the epoch from the chain.
func TestRelaySeenMapEvictsOnEpoch(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	// Drive the epoch from a test-controlled variable.
	var epoch uint64
	n.setRelayEpochFnForTest(func() uint64 { return epoch })

	root := func(b byte) []byte { r := make([]byte, 32); r[0] = b; return r }

	// Epoch 0: admit eph#1 / root A.
	epoch = 0
	if _, err := n.OpenRelaySession(relayTestID(1), root(0xA1), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("epoch 0 open: %v", err)
	}

	// Epoch 1: admit eph#2 / root B. Nothing evicted yet (retention keeps current +
	// previous). Both A and B are still on file.
	epoch = 1
	if _, err := n.OpenRelaySession(relayTestID(2), root(0xB2), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("epoch 1 open: %v", err)
	}

	// (2) GUARD (ii) across the boundary: root A (epoch 0) is the PREVIOUS epoch at
	// epoch 1 — reusing it must still be REJECTED, not evicted-then-accepted.
	if _, err := n.OpenRelaySession(relayTestID(99), root(0xA1), 8, FundingEphemeralBlind); err == nil {
		t.Fatalf("guard (ii) regression: a PREVIOUS-epoch chain root was re-admitted after one epoch advance")
	}

	// Epoch 2: admit eph#3 / root C. Now epoch 0 (root A, eph#1) is two epochs back
	// and must be SWEPT — this is the bound.
	epoch = 2
	if _, err := n.OpenRelaySession(relayTestID(3), root(0xC3), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("epoch 2 open: %v", err)
	}

	// (1) BOUNDED: the maps must not retain the epoch-0 entries. With retention =
	// current + previous, at epoch 2 only epochs 1 and 2 survive: eph#2, eph#3 and
	// root B, root C — four entries total across the two maps' live window, never
	// the full admit history.
	if got := n.relaySeenEntryCountForTest(); got > 4 {
		t.Fatalf("relay-seen maps hold %d entries after 3 epochs; epoch-0 entries were not swept (unbounded growth)", got)
	}

	// (1)/(2) The swept epoch-0 root A can now be re-admitted (it aged out past the
	// retention window — the certified epoch-TTL boundary). This is the intended
	// eviction, and it confirms the sweep actually happened.
	if _, err := n.OpenRelaySession(relayTestID(100), root(0xA1), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("epoch-0 root A should have aged out and be re-admittable at epoch 2: %v", err)
	}

	// (3) MONOTONIC FLOOR: at epoch 2 the floor advanced to 1 (= epoch - retention).
	// A reorg moves the epoch back to 1. The eviction floor must NOT lower — the
	// safe rule is "the floor only moves forward" (epochStart is reorg-swapped, §4
	// reorg caveat). A lowered floor would let a later sweep evict less and could
	// re-admit a root a forward chain already committed to aging.
	floorBefore := n.relayEvictionFloorForTest()
	if floorBefore != 1 {
		t.Fatalf("eviction floor is %d after epoch 2, want 1", floorBefore)
	}
	epoch = 1 // reorg backward
	if _, err := n.OpenRelaySession(relayTestID(101), root(0xD4), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("open at reorged epoch 1: %v", err)
	}
	if floorAfter := n.relayEvictionFloorForTest(); floorAfter < floorBefore {
		t.Fatalf("eviction floor LOWERED from %d to %d on a backward reorg — the monotonic guard is missing (guard-(ii) reorg regression)", floorBefore, floorAfter)
	}
	// Root B (epoch 1) is still on file and must still be REJECTED across the reorg.
	if _, err := n.OpenRelaySession(relayTestID(102), root(0xB2), 8, FundingEphemeralBlind); err == nil {
		t.Fatalf("guard (ii) regression on reorg: root B (epoch 1) was re-admitted after the epoch moved backward")
	}
}

// TestReapedSessionStopsLivePump is the Batch-3 reaper-teardown failing-first test
// (design §3b), closing the TODO(Batch-3) in sweepRelaySeen. Once the daemon binding
// wires SplicePaid, a reaped session may have a LIVE pump goroutine blocked in
// paidPump on auth.Wait(), waiting for a ceiling that will never rise (the fetcher is
// gone). Deleting the table entry alone leaks that goroutine — it blocks forever.
//
// The fix is one line at the reap site: sweepRelaySeen must call sess.closeSession()
// so the pump's Done() check trips and the goroutine drains to its ceiling and
// returns. RED before that line (the pump goroutine never exits — the timeout below
// fires); green after.
//
// Ablation: remove sess.closeSession() from the reap loop → the pump stays blocked on
// auth.Wait() and the "pump exited" assertion times out RED.
func TestReapedSessionStopsLivePump(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	// Drive the epoch from a test-controlled variable so the sweep is deterministic.
	var epoch uint64
	n.setRelayEpochFnForTest(func() uint64 { return epoch })

	const S = 8
	c, _ := relaypay.BuildChain([]byte("reaped-session-live-pump-fresh!!!")[:32], S)
	eph := relayTestID(9)
	epoch = 0
	sess, err := n.OpenRelaySession(eph, c.Root(), S, FundingEphemeralBlind)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	handle := uint64(1)
	n.relaySessions[handle] = sess

	// Start a LIVE pump against the session's authorizer. The ceiling is 0 (no Pay
	// has raised it), so the origin's first chunk cannot be delivered and the pump
	// blocks in paidPump on auth.Wait() — exactly a live session with a vanished
	// fetcher. Real TCP conns so the pump's reads/writes behave like the live path.
	ao, originEnd := relayTCPPair(t) // relay↔origin
	bf, fetchEnd := relayTCPPair(t)  // relay↔fetcher
	defer ao.Close()
	defer originEnd.Close()
	defer bf.Close()
	defer fetchEnd.Close()

	pumpDone := make(chan struct{})
	go func() {
		relaypaidPumpForTest(bf, ao, sess, int64(S)*relaypay.RelayIncrementBytes)
		close(pumpDone)
	}()

	// The origin streams a chunk so the pump reaches the gate and blocks on the
	// unmet ceiling (authBytes == 0).
	go func() { originEnd.Write([]byte("first-increment-chunk-that-cannot-yet-be-delivered")) }()
	// Give the pump a moment to reach and block on the gate.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-pumpDone:
		t.Fatalf("pump exited before the reap — the test did not set up a live blocked pump")
	default:
	}

	// Advance the epoch past the retention window and trigger the sweep. The reaper
	// must reap this stale session AND stop its pump.
	epoch = relayRetentionEpochs + 1
	n.sweepRelaySeen(epoch)

	if _, ok := n.relaySessions[handle]; ok {
		t.Fatalf("reaped session still in the table after the sweep")
	}
	// The pump goroutine MUST exit now (closeSession woke it and Done() is set). If
	// the reaper did not close the session, the pump stays blocked forever.
	select {
	case <-pumpDone:
		// pump drained and exited — correct
	case <-time.After(2 * time.Second):
		t.Fatalf("reaped session's pump goroutine did not exit — the reaper did not tear it down (goroutine leak: the TODO(Batch-3) close is missing)")
	}
}

// relaypaidPumpForTest drives the adapter paidPump against a node RelaySession as the
// live path would. It lives here (not the relay package) so the node test can exercise
// the reaper-teardown seam without importing test helpers across packages. It mirrors
// what SplicePaid's forward lane does: read from the origin, gate on the session's
// authorizer, write to the fetcher.
func relaypaidPumpForTest(dst net.Conn, src net.Conn, sess *RelaySession, maxBytes int64) {
	buf := make([]byte, 4096)
	var forwarded int64
	for forwarded < maxBytes {
		n, rerr := src.Read(buf)
		if n > 0 {
			target := forwarded + int64(n)
			for sess.AuthorizedBytes() < target {
				if sess.Done() {
					return
				}
				<-sess.Wait()
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
			forwarded += int64(n)
		}
		if rerr != nil {
			return
		}
	}
}

// relayTCPPair returns a connected pair of loopback TCP conns (the node-test analog of
// the relay package's tcpPair).
func relayTCPPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
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
		c, aerr := ln.Accept()
		ch <- res{c, aerr}
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

// TestRelaySessionAdvancesAndSettles: the happy path. A session opened under a
// fresh ephemeral identity + fresh chain accepts preimages in order and the
// verifier tracks the settled count.
func TestRelaySessionAdvancesAndSettles(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	const S = 8
	c, _ := relaypay.BuildChain([]byte("the-happy-path-session-random-tip!"), S)
	eph := relayTestID(9)
	sess, err := n.OpenRelaySession(eph, c.Root(), S, FundingEphemeralBlind)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for k := 1; k <= S; k++ {
		if err := sess.Pay(c.Preimage(k)); err != nil {
			t.Fatalf("increment %d: %v", k, err)
		}
	}
	if sess.Count() != S {
		t.Fatalf("settled count %d, want %d", sess.Count(), S)
	}
}
