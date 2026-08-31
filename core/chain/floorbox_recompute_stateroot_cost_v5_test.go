package chain

import (
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// TestRecomputeStateRootCostIsFlat proves the O(payload) claim (the whole point): the SAME small
// E/R payload block, recomputed against a 100-entry vs a 10,000-entry pre-state, uses a witness
// bundle whose size scales with the PAYLOAD + O(log N), NOT with the total state. The superseded
// whole-state P1-a witnessed ALL 100 vs ALL 10,000 leaves; the flat result here is the win.
//
// The measured quantity is the witness's total sidenode count (the actual bytes a box fetches):
// Σ over changed-leaf proofs of len(SideNodes) + the dueBucket proof's sidenodes. It grows only
// with the changed-leaf count (fixed by the payload) and log(state size) — a ~+7% bump from 100 →
// 10,000 (log₂ growth), NOT a 100× bump.
func TestRecomputeStateRootCostIsFlat(t *testing.T) {
	small := measureRecomputeWitness(t, 100)
	large := measureRecomputeWitness(t, 10000)

	t.Logf("witness sidenode count: 100-entry pre-state = %d ; 10,000-entry pre-state = %d",
		small.sidenodes, large.sidenodes)
	t.Logf("changed-leaf witnesses: 100 = %d ; 10,000 = %d (identical — payload-fixed)",
		small.changedLeaves, large.changedLeaves)

	// The changed-leaf COUNT is payload-fixed: identical across state sizes.
	if small.changedLeaves != large.changedLeaves {
		t.Fatalf("changed-leaf count must be payload-fixed, got %d vs %d", small.changedLeaves, large.changedLeaves)
	}
	// The sidenode count must NOT scale with total state. A 100× state increase must not cause
	// anything close to a 100× witness increase — the growth is log(N), so require the large
	// witness is well under 2× the small one (log₂(10000/100) ≈ 6.6 extra levels per proof, a
	// small fraction of the ~256-deep proof).
	if large.sidenodes > 2*small.sidenodes {
		t.Fatalf("O(payload) VIOLATED: 100× state grew the witness %dx (small=%d large=%d) — cost is not flat",
			large.sidenodes/max1(small.sidenodes), small.sidenodes, large.sidenodes)
	}
	// And the recompute must AGREE at both sizes (correctness under the cost claim).
	if !small.agreed || !large.agreed {
		t.Fatalf("recompute must agree at both sizes: small=%v large=%v", small.agreed, large.agreed)
	}
}

type recomputeCost struct {
	sidenodes     int
	changedLeaves int
	agreed        bool
}

// measureRecomputeWitness builds an objective v5 chain, pads its byRoot map to `padTo` entries,
// then recomputes a fixed small E/R block and measures the witness bundle size.
func measureRecomputeWitness(t *testing.T, padTo int) recomputeCost {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(61001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	c.apply(*g)

	// Pad the committed byRoot set to padTo distinct entries (directly, to size the tree without
	// minting thousands of blocks). This grows the pre-state N without changing the payload.
	for i := 0; i < padTo; i++ {
		r := ports.HashBytes([]byte(fmt.Sprintf("pad-%d", i)))
		c.byRoot[r] = ports.Entry{Root: r}
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch: %x != %x", sr, prevRoot)
	}
	f := stateRootFixture{c: c, prevRoot: prevRoot, prover: prover}

	// A FIXED small E/R block: two adds + one token entry. Same payload at every state size.
	prev, h := c.Head()
	tok := &ports.PublishToken{Serial: []byte("cost-serial")}
	e := entry(30)
	e.Token = tok
	b := Block{
		Version: BlockVersionWitnessable, Height: h, Prev: prev,
		Entries: []ports.Entry{entry(31), e},
	}

	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBlock(t, b)

	cost := recomputeCost{changedLeaves: len(w.ChangedLeaves)}
	for _, cl := range w.ChangedLeaves {
		cost.sidenodes += cl.Proof.SideNodeCount()
	}
	cost.sidenodes += w.DueBucketProof.SideNodeCount()
	cost.agreed = f.c.RecomputeStateRootEntriesRevocations(prevRoot, committed, b, w) == nil
	return cost
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
