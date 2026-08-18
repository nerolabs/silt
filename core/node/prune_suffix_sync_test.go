package node

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// Slice 5 — suffix-sync makes pruning safe: a behind node catches up AROUND a peer's
// pruned window by requesting from its OWN finalized head (M1), and a deep-cold node
// beyond the weak-subjectivity window is told to use a checkpoint/archive (ErrNeedCheckpoint)
// rather than silently failing. Fixture uses the mcStubVerify bond verifier + stub-answer
// bond regs (like matureWorld) so blocks carry a heavy payload the prune can shed, with a
// small BondTTL/head-window so a short chain crosses a positive prune floor.

// prunableNet builds two anchor validators sharing a genesis on one simnet, in objective
// mode with finality active and a small retention window (BondTTL 2, head-window 2 ⇒
// pruneFloor = finalized − max(4, 2+margin) = finalized − 6).
func prunableNet(t *testing.T) (n1, n2 *Node, a1, a2 *identity.Identity, g *chain.Block, net *simnet.Network, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	net = simnet.New(sched, 4, simnet.DefaultConfig())
	a1, a2 = identity.FromSeed(9500), identity.FromSeed(9501)
	anchors := map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true}
	g = &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("prune-genesis")}}
	chain.Sign(g, a1.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		AnchorQuorum: 1, MatureValidators: 99, BondTTLBlocks: 2, BondRegHeadWindow: 2}
	n1 = newPrunableNode(t, a1, cfg, g, sched, net)
	n2 = newPrunableNode(t, a2, cfg, g, sched, net)
	return n1, n2, a1, a2, g, net, sched
}

func newPrunableNode(t *testing.T, id *identity.Identity, cfg chain.Config, g *chain.Block, sched *simclock.Scheduler, net *simnet.Network) *Node {
	t.Helper()
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	nd.SetLedger(credit.New(50_000, 0))
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	ch.SetBondVerifier(mcStubVerify) // stub answers accepted; the prune sheds them regardless
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	nd.EnableChain(ch, id.Signer())
	return nd
}

// commitTo appends one committed block (a1 proposer, a2 attester) carrying a stub bond reg —
// a heavy payload the prune will shed — to each target chain, keeping them identical.
func commitTo(t *testing.T, h uint64, prev ports.Hash, a1, a2 *identity.Identity, targets ...*chain.Chain) ports.Hash {
	t.Helper()
	v := identity.FromSeed(int64(6500 + h)) // a fresh bonded identity per height
	pub := append([]byte(nil), v.Signer().Public().(ed25519.PublicKey)...)
	reg := chain.NewBondReg(v.Signer(), ports.HashBytes(pub), 2<<20, []byte("stub"), prev, h)
	b := &chain.Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(fmt.Sprintf("blk-%d", h))}, BondRegs: []chain.BondReg{reg}}
	chain.Sign(b, a1.Signer())
	b.Atts = []chain.Attestation{chain.Attest(b, a2.Signer())}
	for _, c := range targets {
		if err := c.Append(*b); err != nil {
			t.Fatalf("commit h%d: %v", h, err)
		}
	}
	return b.Hash()
}

// TestSuffixSync_CatchUpAroundPrunedPeer: a node behind by less than the retention window
// catches up from a peer that has PRUNED its old heavy proofs — the suffix-sync requests
// from the behind node's OWN finalized head, so it never pulls a pruned block, and its own
// pruned prefix (below its own floor) is accepted on replay. The old {Height:0} full-fetch
// would have hit the peer's pruned gap and been rejected.
func TestSuffixSync_CatchUpAroundPrunedPeer(t *testing.T) {
	n1, n2, a1, a2, g, _, sched := prunableNet(t)

	prev := g.Hash()
	for h := uint64(1); h <= 12; h++ { // shared history to height 12
		prev = commitTo(t, h, prev, a1, a2, n1.Chain(), n2.Chain())
	}
	for h := uint64(13); h <= 17; h++ { // n1 advances alone; n2 falls behind by 5
		prev = commitTo(t, h, prev, a1, a2, n1.Chain())
	}
	if p := n1.Chain().PruneBelowHorizon(); p == 0 { // what pruneOnCommit does on the real path
		t.Fatal("precondition: n1 must have heavy blocks to prune below its floor")
	}
	n2.Chain().PruneBelowHorizon()

	_, before := n2.Chain().Head()
	needCkpt := n2.Stats.ChainSyncNeedCheckpoint
	done := false
	n2.SyncChain([]ports.NodeID{n1.ID()}, func(int, error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	_, after := n2.Chain().Head()
	if _, want := n1.Chain().Head(); after != want {
		t.Fatalf("n2 did not catch up around the pruned peer: next-height %d→%d, want %d", before, after, want)
	}
	if n2.Stats.ChainSyncNeedCheckpoint != needCkpt {
		t.Fatal("a within-window catch-up must not raise the need-checkpoint signal")
	}
}

// TestSuffixSync_DeepColdSignalsNeedCheckpoint: a FRESH node (genesis only, trust floor 0)
// cannot sync from a pruned peer — the peer's pruned blocks are above the fresh node's floor
// and it must not trust them from a peer (the C1/long-range guard). It stalls and SIGNALS
// need-checkpoint (I4), never silently failing or adopting a forgery.
func TestSuffixSync_DeepColdSignalsNeedCheckpoint(t *testing.T) {
	n1, _, a1, a2, g, net, sched := prunableNet(t)
	prev := g.Hash()
	for h := uint64(1); h <= 17; h++ {
		prev = commitTo(t, h, prev, a1, a2, n1.Chain())
	}
	if p := n1.Chain().PruneBelowHorizon(); p == 0 {
		t.Fatal("precondition: n1 must have pruned blocks for the deep-cold gap")
	}

	// A fresh node with only the genesis, on n1's network — deep-cold relative to pruned n1.
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true},
		AnchorQuorum: 1, MatureValidators: 99, BondTTLBlocks: 2, BondRegHeadWindow: 2}
	cold := newPrunableNode(t, identity.FromSeed(9600), cfg, g, sched, net)

	before := cold.Stats.ChainSyncNeedCheckpoint
	done := false
	cold.SyncChain([]ports.NodeID{n1.ID()}, func(int, error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	if cold.Stats.ChainSyncNeedCheckpoint <= before {
		t.Fatal("a deep-cold node syncing from a pruned peer must raise the need-checkpoint signal (I4), not silently fail")
	}
	if _, h := cold.Chain().Head(); h != 1 {
		t.Fatalf("the deep-cold node must NOT adopt the pruned chain (next height %d, want 1 = genesis only)", h)
	}
}

// TestReconstructFork_PrependsOwnPrefix unit-tests the M1 prepend: it keys off the served
// start height, prepending our own [0, start) and using a genesis-rooted serve as-is.
func TestReconstructFork_PrependsOwnPrefix(t *testing.T) {
	n1, _, a1, a2, g, _, _ := prunableNet(t)
	prev := g.Hash()
	for h := uint64(1); h <= 5; h++ {
		prev = commitTo(t, h, prev, a1, a2, n1.Chain())
	}
	served := make([]chain.Block, 0, 5) // a run starting at height 3 ⇒ prepend our own [0,2]
	for h := uint64(3); h <= 7; h++ {
		served = append(served, chain.Block{Version: 1, Height: h})
	}
	full, err := n1.reconstructFork(served)
	if err != nil {
		t.Fatalf("reconstructFork: %v", err)
	}
	if len(full) != 8 || full[0].Height != 0 || full[len(full)-1].Height != 7 {
		t.Fatalf("reconstructed chain wrong: len %d, [0].Height %d, [last].Height %d", len(full), full[0].Height, full[len(full)-1].Height)
	}
	gRoot := []chain.Block{{Version: 1, Height: 0}, {Version: 1, Height: 1}} // genesis-rooted ⇒ as-is
	full2, err := n1.reconstructFork(gRoot)
	if err != nil || len(full2) != 2 {
		t.Fatalf("genesis-rooted serve must be used as-is: len %d err %v", len(full2), err)
	}
}
