package chain

import (
	"crypto/ed25519"
	"runtime"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// scaleChain builds an objective v5 chain of nblocks blocks of perBlock entries
// through the full Append path, as committed blocks arrive, and returns it with the
// bytes allocated per entry while building it.
func scaleChain(t *testing.T, nblocks, perBlock int) (*Chain, float64) {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(61001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	before := allocated()
	k := 0
	for i := 0; i < nblocks; i++ {
		prev, h := c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev}
		for j := 0; j < perBlock; j++ {
			b.Entries = append(b.Entries, keptTreeEntry(k))
			k++
		}
		if err := c.PopulateEra4Roots(&b); err != nil {
			t.Fatalf("roots at %d: %v", h, err)
		}
		twoPhaseSign(&b, []ed25519.PrivateKey{prop}, c.ChainID())
		if err := c.Append(b); err != nil {
			t.Fatalf("append %d: %v", h, err)
		}
	}
	return c, float64(allocated()-before) / float64(k)
}

func allocated() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

// Committing a block, and re-validating history when a validator restarts, must cost
// work in proportion to the block, not to the chain behind it. Validating a block
// applies it to a dry-run copy of the chain; a copy that duplicated the history or
// the per-file sets made each block cost the whole chain and a restart cost its
// square. Allocation per entry is the deterministic witness: it stays flat as the
// chain grows fourfold when the copy is shared, and grows with the chain when it is
// not. (Timing would say the same, noisily.) Small enough to run under -short, so
// every merge-gating job carries it.
func TestCommitAndReloadCostStayFlatAsTheChainGrows(t *testing.T) {
	defer func(was bool) { checkKeptRoots = was }(checkKeptRoots)
	checkKeptRoots = false // the cross-check rebuilds the tree per block, which is the cost under test

	const perBlock = 10
	var commit, reload [2]float64
	for i, nblocks := range []int{500, 2000} {
		c, perEntry := scaleChain(t, nblocks, perBlock)
		commit[i] = perEntry
		blocks := c.Blocks(0)
		fresh := New(c.cfg, c.rep)
		fresh.SetBondVerifier(objectiveVerify)
		before := allocated()
		if n, err := fresh.Reload(blocks); err != nil {
			t.Fatalf("reload at %d: %v", n, err)
		}
		reload[i] = float64(allocated()-before) / float64(nblocks*perBlock)
		t.Logf("%d blocks: commit %.0f B/entry, reload %.0f B/entry", nblocks, commit[i], reload[i])
	}
	// Four times the chain: a per-block copy of history would roughly quadruple both.
	// The state tree's depth grows with the log of its size, so allow half again.
	if commit[1] > 1.5*commit[0] {
		t.Errorf("committing a block allocates %.0f B/entry on a chain four times as long as one at %.0f B/entry: its cost grows with the chain", commit[1], commit[0])
	}
	if reload[1] > 1.5*reload[0] {
		t.Errorf("reloading allocates %.0f B/entry for a chain four times as long as one at %.0f B/entry: a restart costs more than linear time", reload[1], reload[0])
	}
}
