package chain

// TESTER PROBE 3 — blast radius. Is era-3 (v4) REACHABLE on a network the shipped
// binary builds? The coupling only exists on a v4 block, so this bounds it.

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestProbeE_ProductionReadinessStampNeverActivatesEra3 drives the whole production
// route: NewBondReg's stamp -> regVersion -> the rotateEpoch tally -> era3Active ->
// MintVersion. No activation override is set (the daemon sets none: cmd/silt/daemon.go's
// chain.Config literal has no Era3ActivationHeight / Era4ActivationHeight field).
func TestProbeE_ProductionReadinessStampNeverActivatesEra3(t *testing.T) {
	// 1. The stamp the SHIPPED constructor puts on every registration.
	r := NewBondReg(key(91001), ports.Hash{}, twoMiB, []byte("valid"), ports.Hash{}, 0)
	t.Logf("NewBondReg stamp: Version=%d (era-3 tally needs >=%d, era-4 needs >=%d)",
		r.Version, BlockVersionStateRoot, BlockVersionWitnessable)

	// 2. A chain with epochs ON (the latch route's precondition) and NO override.
	keys := []ed25519.PrivateKey{key(91002), key(91003), key(91004), key(91005)}
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, BondTTLBlocks: 400, EpochBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// 3. Every validator re-registers through the PRODUCTION constructor, then the chain
	//    runs past several epoch boundaries so every tally gets its chance.
	for h := 1; h <= 16; h++ {
		prev, next := c.Head()
		b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
		if h <= len(keys) {
			b.BondRegs = []BondReg{NewBondReg(keys[h-1], ports.HashBytes(keys[h-1].Public().(ed25519.PublicKey)), twoMiB, []byte("valid"), prev, 0)}
		}
		switch mv := c.MintVersion(next); {
		case mv >= BlockVersionWitnessable:
			t.Fatalf("h%d: MintVersion returned v5 — era-4 activated on the production route", next)
		case mv >= BlockVersionStateRoot:
			t.Fatalf("h%d: MintVersion returned v4 — era-3 activated on the production route", next)
		default:
			b.Version = BlockVersionRounds
		}
		twoPhaseSign(b, keys)
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d: %v", next, err)
		}
	}
	t.Logf("after 16 heights across 4 epochs: era3LockedIn=%v era3Height=%d era4LockedIn=%v era4Height=%d",
		c.era3LockedIn, c.era3Height, c.era4LockedIn, c.era4Height)
	t.Logf("regVersion committed for each validator:")
	for _, k := range keys {
		id := idOf(k)
		t.Logf("  %x -> %d", id[:4], c.regVersion[id])
	}
	for h := uint64(0); h <= 64; h++ {
		if c.era3Active(h) {
			t.Fatalf("era3Active(%d) is TRUE on the production route", h)
		}
	}
	t.Logf("era3Active is FALSE at every height 0..64 — no v4 block can exist on this chain")

	// 4. The ONE thing that would flip it: a reg stamped >= 4. Not producible by
	//    NewBondReg, and no daemon flag sets the override.
	if r.Version >= BlockVersionStateRoot {
		t.Fatalf("NewBondReg stamps %d — the era-3 tally WOULD fire in production", r.Version)
	}
}
