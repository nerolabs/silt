package chain

import (
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Every root any test in this package takes from a kept tree is also rebuilt
// from every leaf and compared.
func init() { checkKeptRoots = true }

// keptTreeChain is an objective v5 chain past its genesis, holding n committed
// entries each with a spent serial.
func keptTreeChain(t testing.TB, n int) *Chain {
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
	for i := 0; i < n; i++ {
		c.apply(keptTreeBlock(c, i))
	}
	return c
}

func keptTreeEntry(i int) ports.Entry {
	r := ports.HashBytes([]byte(fmt.Sprintf("kept-%d", i)))
	return ports.Entry{Root: r, ManifestChunks: []ports.ChunkID{r},
		Token: &ports.PublishToken{Serial: []byte(fmt.Sprintf("serial-%d", i))}}
}

func keptTreeBlock(c *Chain, i int) Block {
	prev, h := c.Head()
	return Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{keptTreeEntry(i)}}
}

func rebuiltRootV5(t testing.TB, c *Chain) ports.Hash {
	t.Helper()
	r, err := statehash.Root(c.stateRootLeavesV5())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A block's post-apply root from the kept tree equals the rebuild over the
// post-apply leaves, for entries, takedowns and their reversal, through clones
// of clones, while the live chain's tree follows the committed blocks.
func TestKeptTreeRootsMatchRebuild(t *testing.T) {
	c := keptTreeChain(t, 20)
	check := func(step string, b Block) {
		t.Helper()
		got, _, err := c.postApplyRoots(b)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		s := c.cloneForDryRun()
		s.rootCache = nil
		s.apply(b)
		if want := rebuiltRootV5(t, s); got != want {
			t.Fatalf("%s: kept-tree root %s, rebuild %s", step, got, want)
		}
		c.apply(b)
		if got, err := c.StateRootForVersion(BlockVersionWitnessable); err != nil || got != rebuiltRootV5(t, c) {
			t.Fatalf("%s: live root %s (%v) differs from the rebuild", step, got, err)
		}
	}
	for i := 20; i < 30; i++ {
		check(fmt.Sprintf("entry %d", i), keptTreeBlock(c, i))
	}
	if c.rootCache == nil || !c.rootCache.owned {
		t.Fatal("the live chain should keep its own tree after checking blocks")
	}

	prev, h := c.Head()
	gone := keptTreeEntry(3).Root
	check("revoke", Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Revocations: []ports.Hash{gone}})
	prev, h = c.Head()
	check("unrevoke", Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Unrevocations: []ports.Hash{gone}})

	// A clone of a clone builds on its parent's unapplied changes.
	s1 := c.cloneForDryRun()
	s1.apply(keptTreeBlock(s1, 100))
	s2 := s1.cloneForDryRun()
	s2.apply(keptTreeBlock(s2, 101))
	r2, ok := s2.keptRootV5()
	if !ok || r2 != rebuiltRootV5(t, s2) {
		t.Fatalf("clone of a clone: kept %s (ok=%v), rebuild %s", r2, ok, rebuiltRootV5(t, s2))
	}
}

// A clone whose parent has moved its tree on, and a write that bypassed the
// journal, both fall back to the rebuild rather than giving a wrong root.
func TestKeptTreeFallsBackWhenItCannotAccount(t *testing.T) {
	c := keptTreeChain(t, 5)
	s := c.cloneForDryRun()
	c.apply(keptTreeBlock(c, 5))
	if _, err := c.StateRootForVersion(BlockVersionWitnessable); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.keptRootV5(); ok {
		t.Fatal("a clone must not build on a tree its parent has moved on")
	}

	r := ports.HashBytes([]byte("written around the journal"))
	c.byRoot.Set(r, ports.Entry{Root: r})
	got, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil || got != rebuiltRootV5(t, c) {
		t.Fatalf("after a write the journal missed: root %s (%v), rebuild %s", got, err, rebuiltRootV5(t, c))
	}
}

// Checking a small block's roots costs about the same however many entries the
// chain holds; rebuilding from every leaf cost about 0.25 s at 40,000.
func BenchmarkPostApplyRootsKeptTree(b *testing.B) {
	defer func(was bool) { checkKeptRoots = was }(checkKeptRoots)
	checkKeptRoots = false
	for _, n := range []int{1_000, 10_000, 40_000} {
		c := keptTreeChain(b, n)
		blk := keptTreeBlock(c, n)
		if _, _, err := c.postApplyRoots(blk); err != nil { // builds the tree once
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, _, err := c.postApplyRoots(blk); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// byRoot, spent and blocks are shared between a chain and its dry-run clone rather
// than copied, so this is the property the drift gate's shared-backing check cannot
// see for them: applying a block on the clone leaves the source's committed entries,
// spent serials and history exactly as they were, and the clone holds all three.
func TestDryRunCloneWritesNeverReachTheSource(t *testing.T) {
	c := keptTreeChain(t, 50)
	before := rebuiltRootV5(t, c)
	clone := c.cloneForDryRun()
	b := keptTreeBlock(c, 50)
	clone.apply(b)

	e := b.Entries[0]
	if c.byRoot.Has(e.Root) || c.spent.Has(string(e.Token.Serial)) {
		t.Fatal("applying a block on the clone wrote its entry or serial into the source")
	}
	if !clone.byRoot.Has(e.Root) || !clone.spent.Has(string(e.Token.Serial)) {
		t.Fatal("the clone does not hold the block it applied")
	}
	if got := rebuiltRootV5(t, c); got != before {
		t.Fatal("the source's state root changed when the clone applied a block")
	}
	if c.byRoot.Len() != 50 || clone.byRoot.Len() != 51 {
		t.Fatalf("entry counts: source %d (want 50), clone %d (want 51)", c.byRoot.Len(), clone.byRoot.Len())
	}
	if c.blocks.Len() != 51 || clone.blocks.Len() != 52 {
		t.Fatalf("block counts: source %d (want 51), clone %d (want 52)", c.blocks.Len(), clone.blocks.Len())
	}
	if head, _ := c.Head(); head != keptTreeBlock(c, 50).Prev {
		t.Fatal("the source's head moved when the clone applied a block")
	}
}
