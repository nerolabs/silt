package diskplot

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/core/bond"
	"github.com/nerolabs/silt/core/vdf"
	"github.com/nerolabs/silt/ports"
)

func testID(b byte) ports.NodeID {
	var id ports.NodeID
	id[0] = b
	return id
}

// TestStreamedAndResidentPlotsAreTheSameFile pins that sealing a plot block by
// block to disk and sealing it in memory produce the same stored plot. The two
// paths exist so an operator can choose between paying the plot's size in memory
// and paying a few sparse reads per challenge — that is a resource choice, and it
// must not become a choice about WHICH BOND the identity holds. If the files
// diverged, an operator who re-plotted by the other path would commit to a
// different root and fail its own audit.
func TestStreamedAndResidentPlotsAreTheSameFile(t *testing.T) {
	const size = int64(1 << 20)
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("an identity key of some length!!")

	// Resident: seal in memory, Save the whole plot.
	resident := bond.Seal(key, size)
	if err := s.Save(testID(1), resident.Root, resident.Blocks()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Streamed: seal straight into the store.
	blocks, err := s.OpenBlocks(testID(2), bond.NumBlocks(size))
	if err != nil {
		t.Fatalf("OpenBlocks: %v", err)
	}
	streamed, err := bond.SealInto(key, size, blocks)
	if err != nil {
		t.Fatalf("SealInto: %v", err)
	}
	if err := s.CommitBlocks(testID(2), streamed.Root); err != nil {
		t.Fatalf("CommitBlocks: %v", err)
	}
	if streamed.Root != resident.Root {
		t.Fatalf("the two seals committed different bonds: %x streamed, %x resident",
			streamed.Root[:8], resident.Root[:8])
	}

	// Either file reads back through either path, with the same bytes.
	rootA, viaWhole, okA, err := s.Load(testID(2))
	if err != nil || !okA {
		t.Fatalf("Load of the streamed plot: ok=%v err=%v", okA, err)
	}
	rootB, viaBlocks, n, okB, err := s.LoadBlocks(testID(1))
	if err != nil || !okB {
		t.Fatalf("LoadBlocks of the resident plot: ok=%v err=%v", okB, err)
	}
	if rootA != resident.Root || rootB != resident.Root {
		t.Fatalf("headers disagree on the root: %x and %x, want %x", rootA[:8], rootB[:8], resident.Root[:8])
	}
	if n != bond.NumBlocks(size) {
		t.Fatalf("LoadBlocks reports %d blocks, want %d", n, bond.NumBlocks(size))
	}
	buf := make([]byte, len(viaWhole[0]))
	for i := range viaWhole {
		if err := viaBlocks.ReadBlock(i, buf); err != nil {
			t.Fatalf("ReadBlock %d: %v", i, err)
		}
		if !bytes.Equal(buf, viaWhole[i]) {
			t.Fatalf("block %d differs between the whole-file and random-access reads", i)
		}
	}

	// A plot reloaded from disk must still answer a live challenge, which is the
	// only thing holding it is for.
	reloaded, err := bond.ReconstructFrom(key, size, viaBlocks)
	if err != nil {
		t.Fatalf("ReconstructFrom: %v", err)
	}
	if reloaded.Root != resident.Root {
		t.Fatalf("a plot reloaded for random access re-derives root %x, want %x", reloaded.Root[:8], resident.Root[:8])
	}
	const nonce = uint64(77)
	a, ok := reloaded.AnswerSpaceTime(nonce, vdf.Default(), 300, 8)
	if !ok {
		t.Fatal("a disk-backed plot could not answer a challenge")
	}
	if !bond.VerifySpaceTime(key, reloaded.Root, size, nonce, a, vdf.Default(), 300, 8) {
		t.Fatal("an answer from a disk-backed plot does not verify against its committed root")
	}
}

// TestACommittedPlotStillAnswers pins that committing a streamed plot leaves the
// commitment that produced it able to read its own blocks.
//
// A bond is not a file, it is the ability to answer a challenge. Sealing streams the
// plot into the store and hands back a commitment that reads blocks from it on
// demand, so anything the commit step does to that storage happens underneath a live
// bond. If committing severed it, the node would hold a plot it had just written and
// be unable to prove it — indistinguishable from a validator that never earned
// standing, and visible only once real daemons try to reach consensus.
func TestACommittedPlotStillAnswers(t *testing.T) {
	const size = int64(1 << 20)
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("an identity key of some length!!")
	id := testID(3)

	blocks, err := s.OpenBlocks(id, bond.NumBlocks(size))
	if err != nil {
		t.Fatalf("OpenBlocks: %v", err)
	}
	c, err := bond.SealInto(key, size, blocks)
	if err != nil {
		t.Fatalf("SealInto: %v", err)
	}

	// The challenge must be answerable BEFORE the commit, so a failure after it is
	// attributable to the commit and not to the seal.
	const nonce = uint64(31337)
	if _, ok := c.AnswerSpaceTime(nonce, vdf.Default(), 300, 8); !ok {
		t.Fatal("GATE VACUOUS: the freshly sealed plot could not answer, so this says nothing about committing")
	}

	if err := s.CommitBlocks(id, c.Root); err != nil {
		t.Fatalf("CommitBlocks: %v", err)
	}

	a, ok := c.AnswerSpaceTime(nonce, vdf.Default(), 300, 8)
	if !ok {
		t.Fatal("COMMITTING THE PLOT BROKE THE BOND — the commitment that sealed this plot can no longer read " +
			"its own blocks, so the node holds a bond it cannot prove. On a real network that is a validator " +
			"that silently never earns standing.")
	}
	if !bond.VerifySpaceTime(key, c.Root, size, nonce, a, vdf.Default(), 300, 8) {
		t.Fatal("an answer from a committed plot does not verify against its own root")
	}
}
