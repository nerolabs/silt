package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// padBlock builds a linked block of ~2 KB (64 manifest chunks) so a window byte-cap
// admits a controllable number of them.
func padBlock(h uint64, prev ports.Hash) Block {
	chunks := make([]ports.ChunkID, 64)
	for i := range chunks {
		chunks[i] = ports.HashBytes([]byte{byte(h), byte(i)})
	}
	return Block{Version: 1, Height: h, Prev: prev,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte{byte(h)}), ManifestChunks: chunks}}}
}

func padChain(n int) []Block {
	bs := make([]Block, n)
	var prev ports.Hash
	for h := 0; h < n; h++ {
		bs[h] = padBlock(uint64(h), prev)
		prev = bs[h].Hash()
	}
	return bs
}

// EncodeBlocksUpTo must return a bounded PREFIX (never the whole chain when it exceeds
// the cap), always ≥ 1 block, and always a valid round-trip — the serve-buffer bound
// that turns the 144 MB whole-chain encode into a windowed one.
func TestEncodeBlocksUpToBoundsTheWindow(t *testing.T) {
	const N = 20
	blocks := padChain(N)
	full := EncodeBlocks(blocks)
	perBlock := len(full) / N
	cap := perBlock * 5 // room for ~5 blocks, far fewer than N

	win := EncodeBlocksUpTo(blocks, cap)
	if len(win) > cap+perBlock*2 {
		t.Fatalf("window %d B far exceeds cap %d B — not bounded", len(win), cap)
	}
	got, err := DecodeBlocks(win)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || len(got) >= N {
		t.Fatalf("window holds %d blocks, want a bounded prefix 1..%d", len(got), N-1)
	}
	for i := range got { // it must be the PREFIX, in order
		if got[i].Height != blocks[i].Height {
			t.Fatalf("window block %d is height %d, want the prefix height %d", i, got[i].Height, blocks[i].Height)
		}
	}

	// A cap smaller than one block still sends exactly one (progress over stall).
	if g1, _ := DecodeBlocks(EncodeBlocksUpTo(blocks, 1)); len(g1) != 1 {
		t.Fatalf("a tiny cap must still send exactly 1 block, got %d", len(g1))
	}
	// maxBytes <= 0 is the legacy whole-chain encode.
	if len(EncodeBlocksUpTo(blocks, 0)) != len(full) {
		t.Fatal("maxBytes<=0 must encode the whole chain (legacy)")
	}
}

// The requester's reassembly logic: fetching successive windows (server does
// Blocks(from=h) → EncodeBlocksUpTo) and appending must rebuild the EXACT chain. This
// pins the core of the paginated fetch loop deterministically, without the async wiring.
func TestWindowedReassemblyEqualsFullChain(t *testing.T) {
	const N = 20
	blocks := padChain(N)
	cap := len(EncodeBlocks(blocks)) / N * 4 // ~4 blocks per window → several windows

	var got []Block
	h := uint64(0)
	windows := 0
	for int(h) < N {
		// server side: the window from height h
		win, err := DecodeBlocks(EncodeBlocksUpTo(blocks[h:], cap))
		if err != nil {
			t.Fatal(err)
		}
		if len(win) == 0 {
			break
		}
		got = append(got, win...)
		h = win[len(win)-1].Height + 1
		windows++
	}
	if windows < 2 {
		t.Fatalf("expected the fetch to need MULTIPLE windows, took %d", windows)
	}
	if len(got) != N {
		t.Fatalf("reassembled %d blocks, want %d", len(got), N)
	}
	for i := range got {
		if got[i].Hash() != blocks[i].Hash() {
			t.Fatalf("reassembled block %d differs from the source chain", i)
		}
	}
}
