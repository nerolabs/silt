// TESTER PROBE — NOT FOR MERGE. Reachability: is era 4 (and therefore the v5 signature
// FORM) reachable at all on a chain a real daemon builds?
package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestProbe_Era4Reachability(t *testing.T) {
	// (1) The readiness signal a real node emits. NewBondReg is the ONLY non-test
	// producer of a BondReg (core/node/objectivechain.go:70 is its only caller).
	r := NewBondReg(key(701), ports.Hash{}, twoMiB, []byte("a"), ports.Hash{}, 0)
	t.Logf("REACH NewBondReg().Version = %d (era-3 latch needs >=%d, era-4 latch needs >=%d)",
		r.Version, BlockVersionStateRoot, BlockVersionWitnessable)
	if r.Version >= BlockVersionWitnessable {
		t.Fatal("REACH: a real node DOES signal era-4 readiness — the latch is live")
	}

	// (2) A chain shaped like cmd/silt/daemon.go:905 — no Era3/Era4ActivationHeight,
	// EpochBlocks set. Register five bonds and run past several epoch boundaries.
	prop, v1, v2, v3, v4 := key(51), key(52), key(53), key(54), key(55)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4}
	if cfg.Era3ActivationHeight != 0 || cfg.Era4ActivationHeight != 0 {
		t.Fatal("PROBE SETUP WRONG: the daemon sets neither override; this probe must not either")
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range []interface{ Public() interface{} }{} {
		_ = k
	}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}), bondReg(v1, twoMiB, ports.Hash{}),
		bondReg(v2, twoMiB, ports.Hash{}), bondReg(v3, twoMiB, ports.Hash{}),
		bondReg(v4, twoMiB, ports.Hash{}))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
	prev := g.Hash()
	for h := uint64(1); h <= 20; h++ {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, prop)
		b.Atts = []Attestation{Attest(b, v1), Attest(b, v2), Attest(b, v3), Attest(b, v4)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h=%d: %v", h, err)
		}
		prev = b.Hash()
	}
	for _, h := range []uint64{1, 4, 8, 12, 20, 100, 1 << 30} {
		t.Logf("REACH h=%-12d era3Active=%-5v era4Active=%-5v MintVersion=%d",
			h, c.era3Active(h), c.era4Active(h), c.MintVersion(h))
	}
	if c.era4Active(1 << 30) {
		t.Fatal("REACH: era 4 DID activate on a daemon-shaped chain")
	}
	t.Log("REACH VERDICT: on a daemon-shaped chain, era 4 NEVER activates and the proposer " +
		"mints v2 forever — so NO node ever produces a v5-FORM signature, and the (v2,v5) pair " +
		"has no live source.")
}
