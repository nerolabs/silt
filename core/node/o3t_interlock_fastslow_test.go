package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// O3 DIRECTION T — gate (b3), the node-tier twin: fast-path / slow-path equivalence through the
// REAL functions (cert §8.2 (b3), the PE §5 coupling). The chain-tier statement of the same
// property is TestO3T_FastSlowPathSameHead in core/chain; this one drives appendExtension on one
// node and reconstructFork -> Reconcile on its twin, with the served suffix carrying a DIFFERENT
// valid certificate in each serving. Both must land on the same head.
//
// Today (59509b1) the doc comment at chainrole.go:1488 justifies the fast path with "strictly
// positive weight" — a property that has been false on every era-2 certificate since #432. Under
// T the justification is "strictly greater height", a theorem. GREEN now (the dead term is 0),
// GREEN after T; RED under a non-extension-monotone certificate term (see the Tester's memory).

// o3tEra2Chain builds an objective 4-anchor launch chain (roundsWorld's config, node-side keys)
// with an era-1 genesis. Returns the chain, the anchor identities, and the genesis.
func o3tEra2Chain(t *testing.T) (*chain.Chain, []*identity.Identity, *chain.Block) {
	t.Helper()
	ids := make([]*identity.Identity, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(63000 + i))
		anchors[ids[i].NodeID()] = true
	}
	c := chain.New(chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(mcStubVerify)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("o3t-genesis")}}
	chain.Sign(g, ids[0].Signer())
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	return c, ids, g
}

// o3tEra2Block builds an era-2 block with proposer ids[0] and prepare+precommit signatures from
// the given non-proposer signers at round 0.
func o3tEra2Block(ids []*identity.Identity, signers []*identity.Identity, h uint64, prev ports.Hash, name string) *chain.Block {
	b := &chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(name)}}
	chain.Sign(b, ids[0].Signer())
	b.PrepareQC = append(b.PrepareQC, chain.AttestAt(b, ids[0].Signer(), 0, chain.PhasePrepare))
	for _, s := range signers {
		b.PrepareQC = append(b.PrepareQC, chain.AttestAt(b, s.Signer(), 0, chain.PhasePrepare))
	}
	b.Atts = append(b.Atts, chain.AttestAt(b, ids[0].Signer(), 0, chain.PhasePrecommit))
	for _, s := range signers {
		b.Atts = append(b.Atts, chain.AttestAt(b, s.Signer(), 0, chain.PhasePrecommit))
	}
	return b
}

func TestO3T_NodeFastSlowPathSameHead(t *testing.T) {
	fastChain, ids, g := o3tEra2Chain(t)
	slowChain, _, g2 := o3tEra2Chain(t)
	if g.Hash() != g2.Hash() {
		t.Fatal("fixture: shared genesis")
	}
	b1 := o3tEra2Block(ids, ids[1:], 1, g.Hash(), "o3t-b1")
	for _, c := range []*chain.Chain{fastChain, slowChain} {
		if err := c.Append(*b1); err != nil {
			t.Fatalf("b1: %v", err)
		}
	}
	if _, ok := fastChain.FinalizedHeight(); !ok {
		t.Fatal("fixture: the fast path requires BFT finality active")
	}
	b2Thick := o3tEra2Block(ids, ids[1:], 2, b1.Hash(), "o3t-b2")
	b2Thin := o3tEra2Block(ids, ids[1:3], 2, b1.Hash(), "o3t-b2")
	if b2Thick.Hash() != b2Thin.Hash() {
		t.Fatal("fixture: same block under two certificates")
	}

	fast := nodeFor528(t, ids[1], fastChain)
	k, ext, err := fast.appendExtension([]chain.Block{*b2Thick})
	if !ext || err != nil || k != 1 {
		t.Fatalf("fast path appendExtension: k=%d ext=%v err=%v", k, ext, err)
	}

	slow := nodeFor528(t, ids[2], slowChain)
	full, err := slow.reconstructFork([]chain.Block{*b2Thin})
	if err != nil {
		t.Fatalf("reconstructFork: %v", err)
	}
	adopted, err := slow.Chain().Reconcile(full)
	if err != nil {
		t.Fatalf("slow path Reconcile: %v", err)
	}
	if !adopted {
		t.Fatal("I4 VIOLATION — reconstructFork -> Reconcile REFUSED the strict extension appendExtension adopted: the two sync paths disagree on the same served window")
	}
	hf, nf := fast.Chain().Head()
	hs, ns := slow.Chain().Head()
	if hf != hs || nf != ns {
		t.Fatalf("fast/slow heads differ: fast=%x@%d slow=%x@%d", hf[:4], nf, hs[:4], ns)
	}
}
