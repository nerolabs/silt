package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — TIER 2 (over the REAL node loop, via simnet held-delivery).
//
// Tier 1 (core/chain/modelcheck*_test.go) asserts I1/I3 over the Chain finality
// predicate directly. Tier 2 drives the REAL propose→gather→attest→commit→broadcast
// loop of several `*Node`s over the held-delivery network, where the DRIVER chooses
// the delivery order — so it reaches what tier 1 can't: the invariants that only exist
// once messages race through the actual gather and persist across a restart (I2, and
// I5's honest-never-slashed / the #397 delivery-race face).
//
// This file establishes the SUBSTRATE: a real round commits + broadcasts entirely over
// driver-controlled held-delivery. The adversarial invariant oracles that build on it
// (I2-across-restart; the genuine I5/#397 catch, which needs a pre-#402 baseline so the
// both-commit fork is reachable) are the documented next increment — see
// docs/thinking/2026-08-15-406-tier2-substrate-and-the-i5-honesty-catch.md for why the
// first I5 draft was withheld (it could not be shown failing-first: #402's I1 shields
// the #397 fork in the current codebase).

// mcStubVerify makes objective() true without real space-time proofs — anchors qualify
// via launchAnchor (zero-bond, pre-handoff), so the model-check needs no VDF work.
func mcStubVerify(_ []byte, _ ports.Hash, _ int64, _ uint64, _ []byte) bool { return true }

// tier2Net wires n anchor Nodes to a HELD-DELIVERY network sharing one genesis, each
// with its own persisted sign-mark store. Returns the nodes and the network. No
// StartChainSync (its self-rescheduling timers would never quiesce); the driver calls
// proposeBlock directly and moves the resulting messages by hand.
func tier2AnchorNet(t *testing.T, nAnchors int) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block, chain.Config) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8500 + i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}

	nodes := make([]*Node, nAnchors)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	return nodes, ids, net, g, cfg
}

// drainHeld delivers parked messages until the network is quiescent (no message in
// flight) or a step bound is hit, picking the next message via pick(pending)→index.
// Returns the number of deliveries. Since the driver never advances the clock, request
// timeouts never fire — a gather completes purely by delivered replies.
func drainHeld(t *testing.T, net *simnet.Network, pick func([]simnet.HeldMsg) int) int {
	t.Helper()
	const bound = 10000
	steps := 0
	for {
		p := net.Pending()
		if len(p) == 0 {
			return steps
		}
		if steps++; steps > bound {
			t.Fatalf("held-delivery did not quiesce within %d steps (livelock?)", bound)
		}
		net.Deliver(p[pick(p)].ID)
	}
}

func fifo(p []simnet.HeldMsg) int { return 0 }

// TestModelCheckTier2_RoundCommitsOverHeldDelivery is the substrate proof: a real
// proposer gathers a real quorum and commits a block entirely over driver-controlled
// delivery (no clock advance, no auto-timers), and the commit broadcasts to every
// replica. If this works, the held-delivery layer genuinely drives the node loop —
// the precondition for every adversarial tier-2 schedule.
func TestModelCheckTier2_RoundCommitsOverHeldDelivery(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

	b := &chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("blk1")}}
	var done bool
	var commitErr error
	nodes[0].proposeBlock(b, attesters, all, 1, func(err error) { done, commitErr = true, err })

	drainHeld(t, net, fifo)

	if !done {
		t.Fatal("proposeBlock never completed over held-delivery (gather did not finish)")
	}
	if commitErr != nil {
		t.Fatalf("round must commit over held-delivery: %v", commitErr)
	}
	for i, nd := range nodes {
		// Head() returns the NEXT height, so a committed height-1 block ⇒ Head()==2.
		if hh, h := nd.Chain().Head(); h != 2 || hh != b.Hash() {
			t.Fatalf("node %d must hold the committed block-1 (broadcast): head height=%d hash=%x want height 2 hash %x", i, h, hh, b.Hash())
		}
	}
}
