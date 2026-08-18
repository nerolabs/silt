package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Slice 4 — the DORMANT payload-selective prune (pruneBelowHorizon). Sheds the heavy
// BondReg.Answer from finalized blocks strictly below the prune floor, keeping header +
// consensus sigs (slice 2), bounding resident/durable/served heavy payload to a recent
// window. Nothing in production calls it yet — the enablement waits on the safe sync
// redirect (PE ruling slice4-sync-redirect-2026-08-18, Opt C). These tests bank the shed
// logic + the degenerate-BondTTL guard. Plan:
// docs/thinking/2026-08-18-slice4-prune-blocked-on-sync-redirect.md.

// TestPruneFloorAt pins the pure prune-floor arithmetic: retain max(2·BondTTL,
// headWindow+margin) below the finalized head, epoch-aligned; 0 when BondTTL is degenerate
// or there isn't a full retain window yet. Always <= RetentionHorizon() (retain >= 2·BondTTL).
func TestPruneFloorAt(t *testing.T) {
	const m = pruneGuardMargin
	cases := []struct {
		name                          string
		finalized, bondTTL, hw, epoch uint64
		want                          uint64
	}{
		{"degenerate-bondttl", 100, 0, 8, 0, 0},        // BondTTL 0 → never prune
		{"guard-governs", 100, 1, 8, 0, 100 - (8 + m)}, // 2·1=2 < 8+m → retain 8+m
		{"horizon-governs", 100, 32, 8, 0, 100 - 64},   // 2·32=64 > 8+m → retain 64
		{"underflow-zero", 10, 32, 8, 0, 0},            // finalized <= retain → 0
		{"exactly-retain", 64, 32, 8, 0, 0},            // finalized == retain → 0
		{"epoch-floored", 300, 32, 8, 50, 200},         // raw=236 → floor(236/50)*50=200
	}
	for _, c := range cases {
		if got := pruneFloorAt(c.finalized, c.bondTTL, c.hw, c.epoch); got != c.want {
			t.Errorf("%s: pruneFloorAt(%d,%d,%d,%d) = %d, want %d",
				c.name, c.finalized, c.bondTTL, c.hw, c.epoch, got, c.want)
		}
	}
}

// anchorChainWithRegs builds a 2-anchor objective chain to head topHeight, placing a heavy
// bond reg in each block whose height is in regHeights. BondTTLBlocks 1 + BondRegHeadWindow 2
// make the prune floor small (retain = max(2, 2+margin)) so a short chain exercises shedding.
func anchorChainWithRegs(t *testing.T, topHeight uint64, regHeights map[uint64]bool) (*Chain, []Block) {
	t.Helper()
	a1, a2 := key(9100), key(9101)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 99, BondTTLBlocks: 1, BondRegHeadWindow: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	blocks := []Block{*g}
	prev := g.Hash()
	for h := uint64(1); h <= topHeight; h++ {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		if regHeights[h] {
			v := key(int64(6100) + int64(h))
			b.BondRegs = []BondReg{bondReg(v, twoMiB, prev)}
		}
		Sign(b, a1)
		b.Atts = []Attestation{Attest(b, a2)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d: %v", h, err)
		}
		blocks = append(blocks, *b)
		prev = b.Hash()
	}
	return c, blocks
}

// TestPruneBelowHorizon_ShedsBelowRetainsAbove is the core shed: every heavy block strictly
// below the prune floor loses its Answer (and is marked pruned); every block at/above the
// floor keeps its full proof. RED against the stub (which sheds nothing).
func TestPruneBelowHorizon_ShedsBelowRetainsAbove(t *testing.T) {
	c, _ := anchorChainWithRegs(t, 9, map[uint64]bool{1: true, 2: true, 3: true, 6: true})
	floor := c.pruneFloor()
	if floor < 4 {
		t.Fatalf("precondition: prune floor = %d, want >= 4 (heights 1..3 below it)", floor)
	}

	n := c.pruneBelowHorizon()
	if n == 0 {
		t.Fatal("pruneBelowHorizon sheds nothing — the heavy regs below the floor were not pruned")
	}

	for _, b := range c.Blocks(0) {
		heavyBelow := b.Height < floor && len(b.BondRegs) > 0
		if heavyBelow {
			if !b.IsPruned() {
				t.Fatalf("block h%d is below the floor %d with a bond reg but was not pruned", b.Height, floor)
			}
			if b.hasHeavyBondProof() {
				t.Fatalf("block h%d was pruned but still carries a heavy Answer", b.Height)
			}
		}
		if b.Height >= floor && b.IsPruned() {
			t.Fatalf("block h%d is at/above the floor %d but was pruned (must retain full proof)", b.Height, floor)
		}
	}
}

// TestPruneBelowHorizon_DegenerateBondTTL: with BondTTLBlocks 0 the prune floor is 0 even
// under finality, so nothing is shed — the degenerate-config guard (PE Q4). RED-neutral
// (stub also sheds nothing), but load-bearing against the real shed over-pruning to the tip.
func TestPruneBelowHorizon_DegenerateBondTTL(t *testing.T) {
	a1, a2 := key(9200), key(9201)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 99, BondTTLBlocks: 0} // degenerate
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	prev := g.Hash()
	for h := uint64(1); h <= 8; h++ {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		if h == 1 {
			b.BondRegs = []BondReg{bondReg(key(6200), twoMiB, prev)}
		}
		Sign(b, a1)
		b.Atts = []Attestation{Attest(b, a2)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d: %v", h, err)
		}
		prev = b.Hash()
	}
	if got := c.pruneFloor(); got != 0 {
		t.Fatalf("degenerate BondTTL=0 must give prune floor 0, got %d", got)
	}
	if n := c.pruneBelowHorizon(); n != 0 {
		t.Fatalf("degenerate BondTTL=0 must prune nothing, pruned %d", n)
	}
}

// TestPruneBelowHorizon_PreservesLinkageAndReloads: after pruning, the chain still hash-links
// and replays through Reload (Block.Prune preserves Hash(); validateStructural verifies sigs
// against the stored hash and never re-verifies bonds — slice 3 one-site finding).
func TestPruneBelowHorizon_PreservesLinkageAndReloads(t *testing.T) {
	c, _ := anchorChainWithRegs(t, 9, map[uint64]bool{1: true, 2: true, 3: true})
	if n := c.pruneBelowHorizon(); n == 0 {
		t.Fatal("precondition: expected the prune to shed at least one block")
	}
	pruned := c.Blocks(0)

	// Replay the (now-pruned) own history into a fresh replica — must restore every block.
	fresh := New(c.cfg, func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(objectiveVerify)
	restored, err := fresh.Reload(pruned)
	if err != nil {
		t.Fatalf("a pruned own chain must Reload: %v", err)
	}
	if restored != len(pruned) {
		t.Fatalf("Reload restored %d of %d blocks", restored, len(pruned))
	}
	if _, a := c.Head(); func() uint64 { _, b := fresh.Head(); return b }() != a {
		t.Fatal("reloaded head height must match the source chain")
	}
}

// TestPruneBelowHorizon_Idempotent: a second prune sheds nothing (already-pruned + entry-only
// blocks are skipped).
func TestPruneBelowHorizon_Idempotent(t *testing.T) {
	c, _ := anchorChainWithRegs(t, 9, map[uint64]bool{1: true, 2: true, 3: true})
	first := c.pruneBelowHorizon()
	if first == 0 {
		t.Fatal("precondition: first prune must shed something")
	}
	if second := c.pruneBelowHorizon(); second != 0 {
		t.Fatalf("second prune must be a no-op, shed %d", second)
	}
}
