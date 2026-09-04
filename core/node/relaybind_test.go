package node

// PoD §7.3 Batch 3 — the node half of the adapter/node seam, failing-first + -race.
//
// ResolveRelayAuthorizer and SettleRelaySessionForHandle are called from the relay
// Server's accept/pump goroutines, OFF the node's event loop. relaySessions is a
// loop-only table, so these methods MARSHAL onto the loop (clock.AfterFunc(0, …)) and
// block on a reply. This file drives them from off-loop goroutines against a node
// backed by a REAL running event loop (walltime), under -race, to prove:
//
//   - the resolver resolves an owned handle to its live session and REFUSES an unknown
//     handle or a non-owner (the ephID-ownership check, certified residual #2);
//   - the crossing is -race clean (the map is read on the loop, the caller blocks off
//     it) — this is the design-flagged concurrency seam.
//
// A walltime clock is used (not simclock) because the marshal-onto-loop-and-block
// pattern needs the loop to run concurrently with the blocked caller; simclock's
// scheduler is single-goroutine and would deadlock the pattern. Production uses
// walltime, so this is the faithful tier for the seam.
//
// R2.14 (2026-09-04): TestNoDoubleSettleReaperAndPump is RE-SPECIFIED back to a
// PAYING first settle (min(count, Σ face) > 0, the anchored session) — the recorded
// goalpost move that retires the R0.7 interim's "pays 0". Its LOAD-BEARING property
// (single-settle-at-close: a second settle of the same handle is always a no-op)
// is unchanged and still asserted.

import (
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/relaypay"
)

// newLoopNode builds a node backed by a real running event loop (walltime), so
// clock.AfterFunc(0, …) fires on the loop goroutine — the production marshaling path.
func newLoopNode(t *testing.T, idSeed int64) *Node {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	ident := identity.FromSeed(idSeed)
	// simnet supplies only the transport endpoint (this test never sends messages);
	// the node's CLOCK is walltime so AfterFunc(0) fires on the real running loop.
	net := simnet.New(simclock.New(), 1, simnet.DefaultConfig())
	n := New(ident.NodeID(), DefaultConfig(), walltime.New(loop), net.Endpoint(ident.NodeID()), memstore.New())
	// R2.14: a committed demand key_0 + a ledger, so an anchored open is admissible.
	n.SetLedger(credit.New(50_000, 0))
	commitSelfDemandKey(t, n, ident, cachedRSAKey(t, int(idSeed%4)))
	return n
}

// openSessionOnLoop opens a relay session and inserts it into the table ON the loop
// (via the resolver's own marshaling primitive), returning the handle. This mirrors
// handleRelayOpen's loop-side insert without needing the full wire. R2.14: the
// session is anchored; the ephemeral is derived from ephSeed (its NodeID is what
// the caller resolves by) and the anchor is minted OFF the loop, statelessly, so
// concurrent callers race only through the loop-marshaled open itself.
func openSessionOnLoop(t *testing.T, n *Node, ephSeed int64, root []byte, S int) uint64 {
	t.Helper()
	e := newEphemeral(ephSeed)
	anchors := mintAnchorsFor(t, n, 0, 1)
	sig := ed25519.Sign(e.priv, relayOpenCommitment(n.id, root, S, anchors))
	done := make(chan uint64, 1)
	n.clock.AfterFunc(0, func() {
		sess, err := n.OpenRelaySession(e.id, root, S, FundingEphemeralBlind, anchors, e.pub, sig)
		if err != nil {
			done <- 0
			return
		}
		n.relaySessionSeq++
		h := n.relaySessionSeq
		n.relaySessions[h] = sess
		done <- h
	})
	select {
	case h := <-done:
		if h == 0 {
			t.Fatal("openSessionOnLoop: OpenRelaySession failed")
		}
		return h
	case <-time.After(2 * time.Second):
		t.Fatal("openSessionOnLoop: loop did not run the insert")
		return 0
	}
}

// TestResolveRelayAuthorizerOwnershipOffLoop pins the resolver seam: from an OFF-loop
// goroutine, an owned handle resolves to its live session; an unknown handle and a
// non-owner are both REFUSED (certified residual #2, the ephID-ownership check). Runs
// under -race to cover the accept-goroutine ↔ loop crossing.
func TestResolveRelayAuthorizerOwnershipOffLoop(t *testing.T) {
	n := newLoopNode(t, 8801)
	n.EnableRelayAccept()

	const S = 8
	owner := newEphemeral(0x11).id
	c, _ := relaypay.BuildChain([]byte("resolve-owner-fresh-random-tip-32")[:32], S)
	handle := openSessionOnLoop(t, n, 0x11, c.Root(), S)

	// (a) The OWNER resolves to a live session — called off the loop.
	sess, ok := n.ResolveRelayAuthorizer(owner, handle)
	if !ok || sess == nil {
		t.Fatalf("owner did not resolve its own handle (ok=%v)", ok)
	}
	// (b) An UNKNOWN handle is refused.
	if _, ok := n.ResolveRelayAuthorizer(owner, handle+999); ok {
		t.Fatalf("an unknown handle resolved — the lookup does not check existence")
	}
	// (c) A NON-OWNER of a real handle is refused (the ephID-ownership check).
	notOwner := relayTestID(0x22)
	if _, ok := n.ResolveRelayAuthorizer(notOwner, handle); ok {
		t.Fatalf("a non-owner resolved another fetcher's handle — the ownership check is missing (residual #2)")
	}
}

// TestNoDoubleSettleReaperAndPump pins the design §3b hazard: a reaper-triggered close
// and a pump-completion settle must NOT both settle one handle. SettleRelaySession
// deletes the handle on the FIRST call, so a second settle (whichever path fires
// second) finds it absent and pays 0 — single-settle. This test settles once (the
// pump-completion path), then settles the SAME handle again (the reaper path), and
// asserts the second settle is a no-op: it pays 0 and the operator balance does not
// move (twice, or at all — see the R0.7 interim note below).
//
// R2.14 (2026-09-04): the first settle PAYS again — min(count, Σ face) against an
// anchored session (the R0.7 interim's pay-0 re-specification is retired). The
// second settle of the same handle must pay 0 and move nothing.
//
// Ablation: remove the `delete(n.relaySessions, handle)` from SettleRelaySession →
// a SECOND settle finds the session and pays again (observable in the balance), and
// RelaySessionForTest reports the handle still present after the FIRST settle.
func TestNoDoubleSettleReaperAndPump(t *testing.T) {
	const S = 6
	fetcher, relay, ledger, sched := relayPairForTest(t, nil)
	c, _ := relaypay.BuildChain([]byte("no-double-settle-fresh-random-tip")[:32], S)

	var handle uint64
	fetcher.OpenRelaySessionRemote(relay.id, c.Root(), S, FundingEphemeralBlind, mintAnchorsFor(t, relay, 0, 1), func(h uint64, _ error) { handle = h })
	sched.Run()
	if handle == 0 {
		t.Fatal("anchored open over the wire failed")
	}

	// Pay all S increments so there would have been credit to settle, pre-interim.
	for k := 1; k <= S; k++ {
		fetcher.SubmitRelayPay(relay.id, handle, c.Preimage(k), k, func(int, error) {})
		sched.Run()
	}

	balBefore := ledger.Balance(relay.id)
	first := relay.SettleRelaySession(handle) // pump-completion settle
	if want := int64(S) * relaypay.RelayIncrementCredit; first != want {
		t.Fatalf("first settle paid %d for an anchored session with %d paid increments, want min(count, face) = %d (R2.14: the paid > 0 precondition is restored)", first, S, want)
	}
	balAfterFirst := ledger.Balance(relay.id)
	if balAfterFirst != balBefore+first {
		t.Fatalf("balance moved by %d on the first settle, want exactly the settled %d", balAfterFirst-balBefore, first)
	}
	// The single-settle property: the handle must be gone after the FIRST
	// settle regardless of what it paid — this is what the ablation (removing
	// the `delete` call) would break.
	if _, ok := relay.RelaySessionForTest(handle); ok {
		t.Fatalf("the session handle still exists after the first settle — single-settlement-at-close is violated (the ablation this test guards against)")
	}
	// The reaper (or a racing pump) settles the SAME handle again. It must be a no-op.
	second := relay.SettleRelaySession(handle)
	if second != 0 {
		t.Fatalf("second settle of the same handle paid %d — DOUBLE SETTLE (the handle was not removed on first settle)", second)
	}
	if bal := ledger.Balance(relay.id); bal != balAfterFirst {
		t.Fatalf("operator balance moved on the second settle (%d → %d) — double-pay", balAfterFirst, bal)
	}
}

// TestResolveRelayAuthorizerRaceUnderConcurrentOpens hammers the seam: many off-loop
// resolver calls run concurrently with on-loop session opens. It asserts nothing more
// than "no data race / no panic" — the -race detector is the assertion. Without the
// loop-marshaling (or a lock), reading relaySessions off the loop while the loop
// mutates it trips the race detector.
func TestResolveRelayAuthorizerRaceUnderConcurrentOpens(t *testing.T) {
	n := newLoopNode(t, 8802)
	n.EnableRelayAccept()

	const S = 4
	var wg sync.WaitGroup
	// Openers: insert sessions on the loop.
	handles := make(chan uint64, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tip := make([]byte, 32)
			tip[0], tip[1] = 0xEE, byte(i)
			c, _ := relaypay.BuildChain(tip, S)
			handles <- openSessionOnLoop(t, n, int64(0x40+i), c.Root(), S)
		}(i)
	}
	// Resolvers: read the table off the loop concurrently.
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			eph := newEphemeral(int64(0x40 + i)).id
			for j := 0; j < 8; j++ {
				n.ResolveRelayAuthorizer(eph, uint64(i+1))
			}
		}(i)
	}
	wg.Wait()
	close(handles)
	// Drain (no assertion beyond -race cleanliness + no panic).
	for range handles {
	}
}
