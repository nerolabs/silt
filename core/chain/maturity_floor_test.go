package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestMaturityWaitsForTheBondedStorageFloor is the defence against one operator tripping the
// anchor shed with many small bonds. The Nakamoto coefficient cannot tell 24 plots behind one
// operator, each under its own invented failure domain, from 24 operators — so the coefficient
// alone reports the network decentralized. The bonded-storage floor (Config.MatureBondedBytes)
// prices the attack: until the non-anchor bonded total reaches the floor the network stays young,
// whatever the coefficient says. The same chain with the floor at zero is the control: it
// matures on the coefficient alone, which is the shape a blind red team used to trip the latch.
// The floor-box composition (v5MatureNow) reaches the same verdict as the node on each arm.
func TestMaturityWaitsForTheBondedStorageFloor(t *testing.T) {
	const bond = int64(1) << 30
	build := func(floor int64, splitters int) *Chain {
		anchors := map[ports.NodeID]bool{}
		var anchorIDs []ports.NodeID
		for i := 0; i < 4; i++ {
			id := idOf(key(int64(9300 + i)))
			anchors[id] = true
			anchorIDs = append(anchorIDs, id)
		}
		cfg := Config{
			Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
			Anchors: anchors, AnchorQuorum: 1, MatureValidators: 4, OperatorMargin: 2,
			MatureBondedBytes: floor,
		}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		for _, aid := range anchorIDs {
			c.bonded[aid] = bond
			c.validatorsSeen[aid] = true
		}
		for i := 0; i < splitters; i++ { // one operator, many keys, one small plot each
			sid := idOf(key(int64(9400 + i)))
			c.bonded[sid] = bond
			c.validatorsSeen[sid] = true
			c.bondDomain[sid] = uint64(0xd0000 + i) // invented, distinct domains
		}
		return c
	}

	if c := build(0, 24); !c.Mature() {
		t.Fatalf("control: with no floor, 24 split bonds must trip the coefficient alone: %+v", c.C2Metric())
	}
	const floor = int64(16) << 40
	attack := build(floor, 24)
	if attack.Mature() {
		t.Fatalf("one operator's 24 GiB tripped maturity past a %d TiB floor: %+v", floor>>40, attack.C2Metric())
	}
	// The floor binds on the bonded total, not on the coefficient: enough real storage
	// clears it (here 24 × 1 TiB bonds).
	big := build(floor, 24)
	for id := range big.validatorsSeen {
		if !big.cfg.Anchors[id] {
			big.bonded[id] = 1 << 40
		}
	}
	if !big.Mature() {
		t.Fatalf("24 TiB of non-anchor bonded storage must clear a 16 TiB floor: %+v", big.C2Metric())
	}

	for name, c := range map[string]*Chain{"control": build(0, 24), "attack": attack, "big": big} {
		got, out, err := v5MatureNow(liveView{c: c})
		if out != Accept || err != nil {
			t.Fatalf("%s: the composition could not decide maturity: %v %v", name, out, err)
		}
		if got != c.matureNow() {
			t.Fatalf("%s: the composition says mature=%v, the node %v", name, got, c.matureNow())
		}
	}
}

// TestMaturityLatchWaitsForTheWindow: the latch trips only once the network has been mature at every
// block of a run as long as MatureWindowBlocks, and a block that is not mature restarts the run.
func TestMaturityLatchWaitsForTheWindow(t *testing.T) {
	c := New(Config{MatureWindowBlocks: 3}, func(ports.NodeID) int64 { return 0 }) // MatureValidators 0: Mature() is true
	for h := uint64(1); h <= 2; h++ {
		c.applyMaturityLatch(h)
		if c.everMature {
			t.Fatalf("latched at height %d, inside a 3-block window", h)
		}
	}
	c.applyMaturityLatch(3)
	if !c.everMature {
		t.Fatalf("not latched after 3 consecutive mature blocks (since=%d)", c.matureSince)
	}

	// A lapse restarts the run.
	d := New(Config{MatureWindowBlocks: 3, MatureValidators: 1}, func(ports.NodeID) int64 { return 0 })
	d.matureSince = 5       // a run began at height 4
	d.applyMaturityLatch(5) // MatureValidators 1 with nothing bonded: not mature
	if d.matureSince != 0 || d.everMature {
		t.Fatalf("a block that is not mature must end the run: since=%d latched=%v", d.matureSince, d.everMature)
	}
	for h, want := range map[uint64]bool{0: true, 1: true} {
		if got := maturityRunComplete(1, 0, h); got != want {
			t.Fatalf("a window of %d must complete at once", h)
		}
	}
}
