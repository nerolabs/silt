// TESTER PROBE — NOT FOR MERGE. The certification scopes the defect to "every height
// >= H_era4". This drives the mixed-form slash through the ERA-4 validation path on a
// chain where era4Active is TRUE and the carrying block is v5.
package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestProbe_Era4PathAcceptsMixedFormSlash(t *testing.T) {
	prop, att1, victim, foreign := key(40301), key(40302), key(40399), key(40398)
	cfg := Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors:              map[ports.NodeID]bool{idOf(prop): true, idOf(att1): true},
		AnchorQuorum:         1,
		Era3ActivationHeight: 1,
		Era4ActivationHeight: 1, // era 4 governs from height 1
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(att1, twoMiB, ports.Hash{}),
		bondReg(victim, twoMiB, ports.Hash{}))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	cid := c.ChainID()
	prev, next := c.Head()
	if !c.era4Active(next) {
		t.Fatalf("PROBE SETUP WRONG: era 4 must be ACTIVE at h=%d or this probe proves nothing", next)
	}
	if mv := c.MintVersion(next); mv != BlockVersionWitnessable {
		t.Fatalf("PROBE SETUP WRONG: MintVersion(%d) = %d, want %d", next, mv, BlockVersionWitnessable)
	}
	t.Logf("SETUP era4Active(h=%d) = true, MintVersion = %d (v5)", next, c.MintVersion(next))

	mk := func(version uint64, e byte, bindCID ports.Hash) Block {
		b := Block{Version: version, Height: 40, Prev: ports.HashBytes([]byte("foreign-prev")),
			Entries: []ports.Entry{entry(e)}}
		if version >= BlockVersionWitnessable {
			b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
			setD3Digests(&b)
		}
		Sign(&b, foreign)
		b.Atts = []Attestation{AttestAt(&b, victim, 1, PhasePrecommit, bindCID)}
		return b
	}
	pub := append([]byte(nil), victim.Public().(ed25519.PublicKey)...)
	foreignCID := ports.HashBytes([]byte("a foreign silt network genesis"))

	cases := []struct {
		name string
		ev   Equivocation
	}{
		{"(v2 foreign, v5 THIS chain)", Equivocation{Culprit: pub,
			A: mk(BlockVersionStateRoot, 1, foreignCID), B: mk(BlockVersionWitnessable, 2, cid)}},
		{"(v2 foreign, v2 foreign)", Equivocation{Culprit: pub,
			A: mk(BlockVersionStateRoot, 1, foreignCID), B: mk(BlockVersionStateRoot, 2, foreignCID)}},
		{"(v5 foreign, v5 foreign) CONTROL", Equivocation{Culprit: pub,
			A: mk(BlockVersionWitnessable, 1, foreignCID), B: mk(BlockVersionWitnessable, 2, foreignCID)}},
	}
	for _, tc := range cases {
		b := &Block{Version: BlockVersionWitnessable, Height: next, Prev: prev,
			Entries: []ports.Entry{entry(9)}, Slashes: []Equivocation{tc.ev}}
		setD3Digests(b)
		state, log, err := c.postApplyRoots(*b)
		if err != nil {
			t.Fatalf("%s: postApplyRoots: %v", tc.name, err)
		}
		b.StateRoot, b.LogRoot = &state, &log
		Sign(b, prop)

		// The node's era-4 proposal path, and the floor-box mirror, one at a time.
		vpErr := c.ValidateProposal(b)
		vsErr := c.validateSlashes(b)
		t.Logf("ERA4 %-34s ValidateProposal -> %v", tc.name, vpErr)
		t.Logf("ERA4 %-34s validateSlashes  -> %v", tc.name, vsErr)
	}
}
