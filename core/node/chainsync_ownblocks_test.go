package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// A PEER ONE BLOCK BEHIND THAT CATCHES UP MID-SWEEP.
//
// The sweep probes a peer's head, sees it differ from ours because the peer is a
// block behind, and asks for everything from our head up. By the time the request
// arrives the peer has committed that block, so it serves back exactly the block we
// already hold. That window extends nothing, so it used to fall through to the slow
// path, which rebuilds the whole chain from genesis and replays it on the event
// loop. The replay can adopt nothing and slash nothing, and its cost grows with the
// chain's whole history: on an aged chain under steady load each one held the loop
// for tens of seconds, the rounds behind it timed out, more peers fell a block
// behind, and commits stopped.

func TestSyncSkipsAWindowOfBlocksWeAlreadyHold(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())
	a1, a2 := identity.FromSeed(46801), identity.FromSeed(46802)
	anchors := map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-468")}}
	chain.Sign(g, a1.Signer())
	ccfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20)
		ch := chain.New(ccfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	us, peer := mk(a1), mk(a2)
	peer.Bootstrap([]ports.NodeID{a1.NodeID()}, func() {})
	sched.Run()

	// Both hold the same history; we hold one block more.
	growChain(t, us, a1, a2, 13)
	head, next := us.Chain().Head()
	for _, b := range us.Chain().Blocks(1)[:next-2] {
		if err := peer.Chain().Append(b); err != nil {
			t.Fatalf("peer append h%d: %v", b.Height, err)
		}
	}
	tip := us.Chain().Blocks(next - 1)

	net.EnableHeldDelivery()
	done := false
	us.SyncChain([]ports.NodeID{peer.ID()}, func(int, error) { done = true })
	caughtUp := false
	for steps := 0; steps < 100; steps++ {
		q := net.Pending()
		if len(q) == 0 {
			break
		}
		m := q[0]
		if m.Kind == ports.MsgGetChain && m.To == peer.ID() && !caughtUp {
			if err := peer.Chain().Append(tip[0]); err != nil {
				t.Fatalf("peer commits the tip: %v", err)
			}
			caughtUp = true
		}
		net.Deliver(m.ID)
	}
	if !done || !caughtUp {
		t.Fatalf("rig: the sweep must finish (%v) after asking the peer for a window (%v)", done, caughtUp)
	}
	if got, _ := us.Chain().Head(); got != head {
		t.Fatalf("our head moved from %x to %x on a window of our own blocks", head[:4], got[:4])
	}
	if got := us.Stats.ChainSyncFullReconciles; got != 0 {
		t.Fatalf("a window of blocks we already hold ran %d genesis replay(s); it must run none", got)
	}
}
