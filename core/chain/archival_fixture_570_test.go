package chain

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #570 — the archival-format golden-fixture suite.
//
// WHY (the #558 scar): validateStructural verified attesters in era-1 form
// while era-2 signs consensusSigBytes(phase, round, hash) — so replay of ANY
// era-2 chain silently fell back to genesis, from #432 until 2026-08-25,
// masked by peer full-fetch. Every other test mints its fixtures at HEAD with
// HEAD's code, so nothing asserted the archival property: BYTES WRITTEN BY AN
// OLDER BINARY MUST REPLAY AT TODAY'S HEAD. These fixtures are that property,
// committed: real serialized chains (the exact EncodeBlocks representation
// chainstore.Save persists), checked into testdata/archival/, replayed by
// every future HEAD against pinned head-hash and derived-state constants.
//
// WRITE-ONCE DISCIPLINE: a committed fixture is NEVER regenerated — its bytes
// are the contract. A new rule era ADDS a fixture (era-3 must add one when the
// D-TIERING state root lands — consult Q5 makes this suite its RED home). If
// a fixture fails here, the fix is in the VALIDATION/DECODE path, not in the
// fixture. The generator below refuses to overwrite.

const archivalDir = "testdata/archival"

// archivalWorld returns the deterministic 4-anchor world both the generator
// and the replay test construct: same seeds, same config as roundsWorld, but
// WITHOUT testing.T so the generator can use it too. The config is part of
// each fixture's contract and must stay reconstructible forever.
func archivalWorld() (*Chain, []ed25519.PrivateKey) {
	keys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(11000 + i))
		anchors[idOf(keys[i])] = true
	}
	c := New(Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99},
		func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	return c, keys
}

// era1Genesis appends the shared deterministic genesis (era-1 form, proposer
// keys[0]) and returns it.
func era1Genesis(c *Chain, keys []ed25519.PrivateKey) *Block {
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondReg(keys[0], twoMiB, ports.Hash{}))
	for _, k := range keys[1:] {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		panic(err)
	}
	return g
}

// era1Block builds a committed era-1 block: bare-hash attestations from the
// three non-proposer anchors (strict anchor majority with the proposer).
func era1Block(c *Chain, keys []ed25519.PrivateKey, h uint64, prev ports.Hash, mutate func(*Block)) *Block {
	b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
	if mutate != nil {
		mutate(b)
	}
	Sign(b, keys[0])
	for _, k := range keys[1:] {
		b.Atts = append(b.Atts, Attest(b, k))
	}
	if err := c.Append(*b); err != nil {
		panic(err)
	}
	return b
}

// era2Block builds a committed era-2 block: two-phase certificate at round 0
// (proposer self-prepare + count-neutral self-precommit leading each set).
func era2Block(c *Chain, keys []ed25519.PrivateKey, h uint64, prev ports.Hash, mutate func(*Block)) *Block {
	b := &Block{Version: BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
	if mutate != nil {
		mutate(b)
	}
	Sign(b, keys[0])
	b.PrepareQC = append(b.PrepareQC, AttestAt(b, keys[0], 0, PhasePrepare))
	for _, k := range keys[1:] {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare))
	}
	b.Atts = append(b.Atts, AttestAt(b, keys[0], 0, PhasePrecommit))
	for _, k := range keys[1:] {
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit))
	}
	if err := c.Append(*b); err != nil {
		panic(err)
	}
	return b
}

// buildArchivalChains constructs the four fixture histories deterministically.
// Shared by the generator (to write them) and — deliberately NOT by the replay
// test, which reads only the committed bytes.
func buildArchivalChains() map[string][]Block {
	out := map[string][]Block{}

	// era1: launch-mode history, era-1 signatures throughout, carrying the
	// registry surface a real archive holds: entries, bond regs (renewal at
	// h2), a revocation of h1's entry (h3), and its unrevocation (h4).
	{
		c, keys := archivalWorld()
		g := era1Genesis(c, keys)
		b1 := era1Block(c, keys, 1, g.Hash(), nil)
		b2 := era1Block(c, keys, 2, b1.Hash(), func(b *Block) {
			b.BondRegs = append(b.BondRegs, bondReg(keys[1], twoMiB, b1.Hash()))
		})
		b3 := era1Block(c, keys, 3, b2.Hash(), func(b *Block) {
			b.Revocations = append(b.Revocations, entry(1).Root)
		})
		era1Block(c, keys, 4, b3.Hash(), func(b *Block) {
			b.Unrevocations = append(b.Unrevocations, entry(1).Root)
		})
		out["era1.cbor"] = c.Blocks(0)
	}

	// era2: era-1 genesis (the #432 flip happened after launch everywhere)
	// then era-2 two-phase blocks with a reg renewal.
	{
		c, keys := archivalWorld()
		g := era1Genesis(c, keys)
		b1 := era2Block(c, keys, 1, g.Hash(), func(b *Block) {
			b.BondRegs = append(b.BondRegs, bondReg(keys[2], twoMiB, g.Hash()))
		})
		era2Block(c, keys, 2, b1.Hash(), nil)
		out["era2.cbor"] = c.Blocks(0)
	}

	// era2-pruned: the at-depth on-disk shape — the reg-carrying era-2 block
	// payload-selectively pruned (Answer dropped, Pruned hash carried).
	{
		blocks := append([]Block(nil), out["era2.cbor"]...)
		blocks[1] = blocks[1].Prune()
		out["era2-pruned.cbor"] = blocks
	}

	// mixed: the real field history shape — era-1 committed blocks, then the
	// era flip, then era-2 blocks, one store.
	{
		c, keys := archivalWorld()
		g := era1Genesis(c, keys)
		b1 := era1Block(c, keys, 1, g.Hash(), nil)
		b2 := era1Block(c, keys, 2, b1.Hash(), func(b *Block) {
			b.BondRegs = append(b.BondRegs, bondReg(keys[3], twoMiB, b1.Hash()))
		})
		b3 := era2Block(c, keys, 3, b2.Hash(), nil)
		era2Block(c, keys, 4, b3.Hash(), func(b *Block) {
			b.Revocations = append(b.Revocations, entry(2).Root)
		})
		out["mixed-era1-era2.cbor"] = c.Blocks(0)
	}
	return out
}

// TestGenerateArchivalFixtures_570 writes the fixture files. Guarded: runs
// only with SILT_REGEN_ARCHIVAL_FIXTURES=1, and REFUSES to overwrite an
// existing fixture (write-once — delete by hand only with a recorded reason,
// which should never happen; add new files for new eras instead).
func TestGenerateArchivalFixtures_570(t *testing.T) {
	if os.Getenv("SILT_REGEN_ARCHIVAL_FIXTURES") != "1" {
		t.Skip("generator; set SILT_REGEN_ARCHIVAL_FIXTURES=1 to write NEW fixtures (existing files are never overwritten)")
	}
	if err := os.MkdirAll(archivalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, blocks := range buildArchivalChains() {
		path := filepath.Join(archivalDir, name)
		if _, err := os.Stat(path); err == nil {
			t.Logf("%s: exists — SKIPPED (write-once)", name)
			continue
		}
		if err := os.WriteFile(path, EncodeBlocks(blocks), 0o644); err != nil {
			t.Fatal(err)
		}
		head := blocks[len(blocks)-1].Hash()
		t.Logf("%s: %d blocks, head %s", name, len(blocks), hex.EncodeToString(head[:]))
	}
}

// The pinned contract per fixture: committed bytes → Reload at HEAD → exactly
// this many blocks, this head hash, this derived state. Every constant below
// was recorded at generation (2026-08-25, HEAD 027c354, era-2 current) and is
// NEVER updated to make a failure pass — a mismatch means HEAD broke replay
// of an older archive (#558 class) or changed hash computation for committed
// bytes (a silent hard fork).
func TestArchivalFixturesReplayAtHead_570(t *testing.T) {
	cases := []struct {
		file     string
		blocks   int
		nextH    uint64
		headHash string
		state    func(t *testing.T, c *Chain)
	}{
		{"era1.cbor", 5, 5, "13d5393d9efaf363f42136de38c0ad540de1ab6b1fe10ff45ea104d5b001587d", func(t *testing.T, c *Chain) {
			if c.Revoked(entry(1).Root) {
				t.Error("era1: entry(1) must be UNrevoked after the h4 unrevocation")
			}
			if !c.IsBonded(idOf(key(11001))) {
				t.Error("era1: keys[1] must be bonded after the h2 renewal")
			}
		}},
		{"era2.cbor", 3, 3, "3bf13cc07f4b46304661d1344b17021c29ec0205fabeb75994f25e471ccceeb9", func(t *testing.T, c *Chain) {
			if !c.IsBonded(idOf(key(11002))) {
				t.Error("era2: keys[2] must be bonded after the h1 reg")
			}
		}},
		{"era2-pruned.cbor", 3, 3, "3bf13cc07f4b46304661d1344b17021c29ec0205fabeb75994f25e471ccceeb9", nil},
		{"mixed-era1-era2.cbor", 5, 5, "5a7e02e6c43da4925250951055999cf6a205bf0c69cfaaa589fd094b13d0ecc5", func(t *testing.T, c *Chain) {
			if !c.Revoked(entry(2).Root) {
				t.Error("mixed: entry(2) must be revoked by the h4 era-2 revocation")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(archivalDir, tc.file))
			if err != nil {
				t.Fatalf("committed fixture missing: %v (fixtures are write-once repo artifacts — never regenerate to fix a failure)", err)
			}
			blocks, err := DecodeBlocks(raw)
			if err != nil {
				t.Fatalf("#570: HEAD can no longer DECODE the committed archival bytes: %v", err)
			}
			if len(blocks) != tc.blocks {
				t.Fatalf("decoded %d blocks, want %d", len(blocks), tc.blocks)
			}
			c, _ := archivalWorld()
			n, err := c.Reload(blocks)
			if err != nil || n != tc.blocks {
				t.Fatalf("#570 (the #558 class): HEAD replays only %d of %d committed blocks (err=%v) — an archived store written by an older era no longer validates; fix the era-aware validation path, NEVER the fixture", n, tc.blocks, err)
			}
			head, next := c.Head()
			if next != tc.nextH {
				t.Fatalf("restored next height %d, want %d", next, tc.nextH)
			}
			if got := hex.EncodeToString(head[:]); got != tc.headHash {
				t.Fatalf("#570: head hash of the committed archive changed: got %s want %s — HEAD computes a different hash for identical committed bytes (a silent hard fork)", got, tc.headHash)
			}
			if tc.state != nil {
				tc.state(t, c)
			}
		})
	}
}
