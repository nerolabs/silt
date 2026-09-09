package genesis_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/genesis"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// The load-bearing property: every node, building genesis independently
// from the embedded manifesto, gets the byte-identical block and link.
// That is what lets genesis be "declared, not agreed".
func TestGenesisIsDeterministic(t *testing.T) {
	b1, h1, _, err := genesis.Build(memstore.New())
	if err != nil {
		t.Fatal(err)
	}
	b2, h2, _, err := genesis.Build(memstore.New())
	if err != nil {
		t.Fatal(err)
	}
	if b1.Hash() != b2.Hash() {
		t.Fatal("genesis block hash differs between builds — not deterministic")
	}
	if h1 != h2 {
		t.Fatal("genesis link differs between builds")
	}
	if b1.Height != 0 {
		t.Fatalf("genesis height %d, want 0", b1.Height)
	}
}

// TestGenesisBlockHashIsPinned holds height-0 IDENTITY across binaries, which
// TestGenesisIsDeterministic cannot see (it compares two builds in one process). The
// literals are the values R-SHORT-FINAL-STRIPE produces, the SECOND accepted genesis move
// in this window and on the same ground as the first: no live network exists and every
// development chain is wiped on upgrade.
//
//	pre-4′ (padded manifest frame)   hash 7becf754…32ce · manifest chunk 8063c7a3…4610
//	4′     (true-length manifest)    hash f428d0a8…0951 · manifest chunk 5478750c…d107 · root fce9eeeb…20d6
//	now    (true-length DATA frame)  hash e44344ea…72c0 · manifest chunk f761f80b…fcf6 · root 31768fb4…7dd1
//
// The ROOT moves this time and did not before, and that difference is the whole point of
// pinning three literals rather than one: 4′ re-framed the manifest, which the root does
// not cover, so only the entry and the block hash moved. R-SHORT-FINAL-STRIPE re-frames
// the manifesto's own 2,042 bytes — a single-frame object — so its data and parity chunk
// IDs move, and the root is built out of exactly those. From here the genesis hash moves
// ONLY by an explicit, recorded decision: any drift turns this RED. ABLATION: set
// ManifestFrameBytes: 64 << 10 in genesis.Options → RED on the manifest chunk ID and on
// the block hash, GREEN on the root.
func TestGenesisBlockHashIsPinned(t *testing.T) {
	const (
		wantHash  = "e44344eafa258c64904d88337e72ad7a904bd3c16a3058abb8d58557740272c0"
		wantRoot  = "31768fb45fcf6e7fbf5568f916c790ea905d85220d20717bfe91442934867dd1"
		wantChunk = "f761f80bdc29952ef1a25c2136a35fb34e4feaf1b6873f7e03dc8a1fce67fcf6"
	)
	b, h, entry, err := genesis.Build(memstore.New())
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(h.Root[:]); got != wantRoot {
		t.Fatalf("genesis root %s, want %s — the manifesto's chunking or erasure geometry moved", got, wantRoot)
	}
	if len(entry.ManifestChunks) != 1 {
		t.Fatalf("genesis manifest is %d chunks, want exactly 1 (one true-length frame)", len(entry.ManifestChunks))
	}
	if got := hex.EncodeToString(entry.ManifestChunks[0][:]); got != wantChunk {
		t.Fatalf("genesis manifest chunk %s, want %s — the manifest FRAME moved (the root did not), and with it the block hash", got, wantChunk)
	}
	hh := b.Hash()
	if got := hex.EncodeToString(hh[:]); got != wantHash {
		t.Fatalf("genesis block hash %s, want %s — a fresh node and a node with a persisted chain now disagree at height 0", got, wantHash)
	}
}

// A fresh chain seeded with genesis has the manifesto at height 0 with
// ZERO reputation available — proving genesis bypasses the quorum gate —
// and real blocks then build at height 1.
func TestSeedsChainAtHeightZeroWithoutQuorum(t *testing.T) {
	block, _, _, err := genesis.Build(memstore.New())
	if err != nil {
		t.Fatal(err)
	}
	c := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	if err := c.AppendGenesis(block); err != nil {
		t.Fatalf("genesis rejected despite zero reputation: %v", err)
	}
	if c.Len() != 1 {
		t.Fatalf("chain length %d after genesis, want 1", c.Len())
	}
	_, nextHeight := c.Head()
	if nextHeight != 1 {
		t.Fatalf("next block height %d, want 1", nextHeight)
	}
	// A second genesis is refused.
	if err := c.AppendGenesis(block); err == nil {
		t.Fatal("second genesis must be rejected")
	}
	// A tampered genesis fails its signature check.
	bad := block
	bad.Entries = nil
	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	if err := fresh.AppendGenesis(bad); err == nil {
		t.Fatal("tampered genesis must be rejected")
	}
}

// The genesis file is retrievable: the manifesto comes back bit-perfect
// from the seeded store using the genesis link.
func TestGenesisFileRetrievable(t *testing.T) {
	store := memstore.New()
	block, h, _, err := genesis.Build(store)
	if err != nil {
		t.Fatal(err)
	}
	reg := oneEntryReg{entry: block.Entries[0]}
	var out bytes.Buffer
	if err := pipeline.Get(context.Background(), store, reg, h, &out); err != nil {
		t.Fatalf("retrieving genesis file: %v", err)
	}
	if !bytes.Equal(out.Bytes(), genesis.Manifesto) {
		t.Fatal("retrieved genesis file does not match the manifesto")
	}
}

// oneEntryReg is a minimal read-only registry holding just the genesis
// entry, so pipeline.Get can resolve the manifest.
type oneEntryReg struct{ entry ports.Entry }

func (r oneEntryReg) Publish(context.Context, ports.Entry) error { return nil }
func (r oneEntryReg) Lookup(_ context.Context, root ports.Hash) (ports.Entry, bool, error) {
	if root == r.entry.Root {
		return r.entry, true, nil
	}
	return ports.Entry{}, false, nil
}
func (r oneEntryReg) All(context.Context) ([]ports.Entry, error) {
	return []ports.Entry{r.entry}, nil
}
