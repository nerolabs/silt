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

// #382 — chain-sync must not re-fetch + re-validate every peer's WHOLE chain
// every sweep when the network already agrees. A cheap head probe elides the
// full fetch on an identical head; the full fetch (catch-up / reorg / slash)
// runs only on a real head difference. These tests pin both halves: the
// steady-state elision (the M1 cost win) AND that every correctness path the
// full fetch owns still fires.

// twoAgreeingValidators builds two objective validators sharing a genesis, with
// a1 the anchor proposer. Returns them plus the shared genesis.
func twoAgreeingValidators(t *testing.T) (*Node, *Node, *identity.Identity, *identity.Identity, *chain.Block, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())
	a1, a2 := identity.FromSeed(8201), identity.FromSeed(8202)
	anchors := map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-382")}}
	chain.Sign(g, a1.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	mk := func(id *identity.Identity) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	n1, n2 := mk(a1), mk(a2)
	n2.Bootstrap([]ports.NodeID{a1.NodeID()}, func() {})
	sched.Run()
	return n1, n2, a1, a2, g, sched
}

func TestChainSyncHeadProbeElidesFullFetchOnAgreement382(t *testing.T) {
	n1, n2, _, a2, _, sched := twoAgreeingValidators(t)

	// n2 syncs against n1 while they already share only genesis (identical head).
	// Every sweep must be a HEAD MATCH — zero full fetches — because the heads agree.
	before := n2.Stats.ChainSyncFullFetches
	for i := 0; i < 5; i++ {
		done := false
		n2.SyncChain([]ports.NodeID{n1.ID()}, func(int, error) { done = true })
		sched.Run()
		if !done {
			t.Fatal("SyncChain did not complete")
		}
	}
	if got := n2.Stats.ChainSyncFullFetches - before; got != 0 {
		t.Fatalf("#382: 5 sweeps against an AGREEING peer must elide every full fetch, got %d full fetches "+
			"(the head probe should have matched and skipped)", got)
	}
	if n2.Stats.ChainSyncHeadMatches < 5 {
		t.Fatalf("#382: expected ≥5 head-match elisions, got %d", n2.Stats.ChainSyncHeadMatches)
	}
	_ = a2
}

func TestChainSyncHeadProbeStillCatchesUp382(t *testing.T) {
	n1, n2, a1, a2, g, sched := twoAgreeingValidators(t)

	// n1 commits a block that n2 has not seen — their heads now DIFFER, so n2's
	// head probe must fall through to a full fetch and catch up.
	prev, _ := n1.Chain().Head()
	b1 := &chain.Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{mkEntry("b1")}}
	chain.Sign(b1, a1.Signer())
	b1.Atts = []chain.Attestation{chain.Attest(b1, a2.Signer())}
	if err := n1.Chain().Append(*b1); err != nil {
		t.Fatalf("n1 commit: %v", err)
	}
	_ = g

	fetchBefore := n2.Stats.ChainSyncFullFetches
	done := false
	var added int
	n2.SyncChain([]ports.NodeID{n1.ID()}, func(a int, _ error) { added, done = a, true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	if added != 1 {
		t.Fatalf("#382: n2 must catch up the 1 block it lacks via the full-fetch fallback, added=%d", added)
	}
	if n2.Stats.ChainSyncFullFetches-fetchBefore != 1 {
		t.Fatalf("#382: a head difference must trigger exactly one full fetch, got %d", n2.Stats.ChainSyncFullFetches-fetchBefore)
	}
	_, h1 := n1.Chain().Head()
	_, h2 := n2.Chain().Head()
	if h1 != h2 {
		t.Fatalf("#382: heads must converge after catch-up (n1=%d n2=%d)", h1, h2)
	}

	// And now that they agree again, the next sweep must go back to eliding.
	matchBefore := n2.Stats.ChainSyncHeadMatches
	done = false
	n2.SyncChain([]ports.NodeID{n1.ID()}, func(int, error) { done = true })
	sched.Run()
	if n2.Stats.ChainSyncHeadMatches-matchBefore != 1 {
		t.Fatal("#382: once heads re-agree, the sweep must elide the full fetch again")
	}
}
