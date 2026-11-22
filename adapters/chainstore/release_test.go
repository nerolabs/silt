package chainstore

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// objectiveWorld is an objective v5 chain with one bonded validator that renews its
// bond, with a heavy proof, every few blocks: the shape whose old blocks a running
// node sheds and releases.
type objectiveWorld struct {
	cfg  chain.Config
	prop ed25519.PrivateKey
	// answer is the bond proof each renewal carries.
	answer []byte
}

const renewEvery = 7

func verifyValid(_ []byte, _ ports.Hash, _ int64, _ uint64, answer []byte) bool {
	return bytes.HasPrefix(answer, []byte("valid"))
}

func newObjectiveWorld() objectiveWorld {
	return objectiveWorld{
		cfg: chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			BondTTLBlocks: 16, EpochBlocks: 0},
		prop:   testKey(41),
		answer: []byte("valid"),
	}
}

func (w objectiveWorld) fresh() *chain.Chain {
	c := chain.New(w.cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(verifyValid)
	return c
}

func (w objectiveWorld) bondRoot() ports.Hash {
	return ports.HashBytes(w.prop.Public().(ed25519.PublicKey))
}

func (w objectiveWorld) genesis(t *testing.T, c *chain.Chain) {
	t.Helper()
	g := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("genesis")), FileSize: 1,
			ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("genesis-manifest"))}}},
		BondRegs: []chain.BondReg{chain.NewBondReg(w.prop, w.bondRoot(), 8<<20, w.answer, ports.Hash{}, 1)}}
	if err := c.PopulateEra4Roots(g); err != nil {
		t.Fatal(err)
	}
	chain.Sign(g, w.prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
}

// commit appends the next block, as a running node would: commit, shed heavy
// proofs below the horizon, persist, release.
func (w objectiveWorld) commit(t *testing.T, c *chain.Chain, wr *Writer, i int) {
	t.Helper()
	prev, h := c.Head()
	b := chain.Block{Version: chain.BlockVersionWitnessable, Height: h, Prev: prev,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte(fmt.Sprintf("file-%d", i))), FileSize: int64(i + 1),
			ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte(fmt.Sprintf("manifest-%d", i)))},
			Token:          &ports.PublishToken{Serial: []byte(fmt.Sprintf("serial-%d", i))}}}}
	if i%renewEvery == 0 {
		b.BondRegs = []chain.BondReg{chain.NewBondReg(w.prop, w.bondRoot(), 8<<20, w.answer, prev, 1)}
	}
	if err := c.PopulateEra4Roots(&b); err != nil {
		t.Fatalf("roots at %d: %v", h, err)
	}
	chain.Sign(&b, w.prop)
	b.PrepareQC = append(b.PrepareQC, chain.AttestAt(&b, w.prop, 0, chain.PhasePrepare, c.ChainID()))
	b.Atts = append(b.Atts, chain.AttestAt(&b, w.prop, 0, chain.PhasePrecommit, c.ChainID()))
	if err := c.Append(b); err != nil {
		t.Fatalf("append %d: %v", h, err)
	}
	c.PruneBelowHorizon()
	if err := wr.Sync(c); err != nil {
		t.Fatalf("sync at %d: %v", h, err)
	}
	if _, err := c.ReleaseBlocks(wr); err != nil {
		t.Fatalf("release at %d: %v", h, err)
	}
}

// grow builds a chain of n blocks after genesis through a Writer at path.
func (w objectiveWorld) grow(t *testing.T, path string, n int) (*chain.Chain, *Writer, []ports.Hash) {
	t.Helper()
	c := w.fresh()
	wr := NewWriter(path)
	w.genesis(t, c)
	hashes := []ports.Hash{hashAt(c, 0)}
	for i := 0; i < n; i++ {
		w.commit(t, c, wr, i)
		hashes = append(hashes, hashAt(c, c.Len()-1))
	}
	return c, wr, hashes
}

// sameBlocks fails unless every block of c reads back as the block committed.
func sameBlocks(t *testing.T, step string, c *chain.Chain, hashes []ports.Hash) {
	t.Helper()
	if c.Len() != len(hashes) {
		t.Fatalf("%s: chain holds %d blocks, %d were committed", step, c.Len(), len(hashes))
	}
	for h := range hashes {
		b := c.BlockAt(uint64(h))
		if b.Hash() != hashes[h] || b.Height != uint64(h) {
			t.Fatalf("%s: block %d reads back as height %d hash %s, committed %s", step, h, b.Height, b.Hash(), hashes[h])
		}
		if h < c.ReleasedBlocks()+32 && h >= 32 {
			for _, r := range b.BondRegs {
				if r.Answer != nil {
					t.Fatalf("%s: released block %d came back with the heavy proof it had shed", step, h)
				}
			}
		}
	}
}

// A running node releases the blocks it has made final and durable, and every one
// of them reads back from the log as the block committed, with the proofs it shed
// still shed. Serving a peer, a lookup, and the era census all see the same chain.
func TestReleasedBlocksReadBackFromTheLog(t *testing.T) {
	w := newObjectiveWorld()
	c, wr, hashes := w.grow(t, filepath.Join(t.TempDir(), "chain.cbor"), 300)
	defer wr.Close()
	if c.ReleasedBlocks() == 0 {
		t.Fatal("no block was released: the test proves nothing about reading one back")
	}
	sameBlocks(t, "after release", c, hashes)
	served := c.Blocks(0)
	for h := range served {
		if served[h].Hash() != hashes[h] {
			t.Fatalf("Blocks(0) serves a different block %d", h)
		}
	}
	if got, want := c.EraState().Census, chain.CensusOf(served); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("era census %+v, a walk of the blocks gives %+v", got, want)
	}
	if e, ok := c.LookupRoot(ports.HashBytes([]byte("file-5"))); !ok || e.FileSize != 6 || e.Token != nil {
		t.Fatalf("lookup of a file in a released block: %+v, %v", e, ok)
	}
}

// A restart replays the log block by block and releases as it goes, so it holds no
// more of the history in memory than the node did before it stopped; and the first
// save after it appends instead of rewriting the file.
func TestReplayReleasesAsItGoesAndTheNextSaveAppends(t *testing.T) {
	w := newObjectiveWorld()
	path := filepath.Join(t.TempDir(), "chain.cbor")
	c, wr, hashes := w.grow(t, path, 600)
	wr.Close()
	released := c.ReleasedBlocks()

	again := w.fresh()
	wr2 := NewWriter(path)
	defer wr2.Close()
	n, loss, refused := wr2.Recover(again, false)
	if refused != nil || loss != nil || n != len(hashes) {
		t.Fatalf("recover: %d blocks, loss %v, refused %v", n, loss, refused)
	}
	if again.ReleasedBlocks() < released-releaseEvery {
		t.Fatalf("the replay released %d blocks; the node it restarts had released %d", again.ReleasedBlocks(), released)
	}
	sameBlocks(t, "after replay", again, hashes)

	before, compactAt := fileSize(t, path), wr2.compactAt
	w.commit(t, again, wr2, 600)
	if wr2.compactAt != compactAt {
		t.Fatal("the first save after a restart rewrote the file")
	}
	hashes = append(hashes, hashAt(again, again.Len()-1))
	if grew := fileSize(t, path) - before; grew <= 0 || grew > 64<<10 {
		t.Fatalf("the first save after a restart grew the file by %d bytes, not one block", grew)
	}
	sameBlocks(t, "after the next save", again, hashes)
	sameChain(t, path, again)
}

// A rewrite of the log, by compaction or after a reorg, reads the released blocks
// from the old file while it writes the new one, and the chain reads them from the
// new file afterwards.
func TestRewriteCarriesReleasedBlocksAcross(t *testing.T) {
	w := newObjectiveWorld()
	path := filepath.Join(t.TempDir(), "chain.cbor")
	c, wr, hashes := w.grow(t, path, 300)
	defer wr.Close()
	if c.ReleasedBlocks() == 0 {
		t.Fatal("no block was released")
	}
	before := fileSize(t, path)
	wr.compactAt = 0 // the next sync compacts
	if err := wr.Sync(c); err != nil {
		t.Fatalf("compaction: %v", err)
	}
	if after := fileSize(t, path); after >= before {
		t.Fatalf("compaction left the file at %d bytes from %d: the shed proofs did not leave the disk", after, before)
	}
	sameBlocks(t, "after compaction", c, hashes)
	sameChain(t, path, c)
	w.commit(t, c, wr, 300)
}

// A torn final frame, an append a crash interrupted, is cut off at restart, and
// the next save's frame lands where it was.
func TestReplayCutsATornFrameAndAppendsInItsPlace(t *testing.T) {
	w := newObjectiveWorld()
	path := filepath.Join(t.TempDir(), "chain.cbor")
	_, wr, _ := w.grow(t, path, 40)
	wr.Close()
	// A length header and part of a long payload: longer than the next block's
	// frame, so writing over it would not hide it.
	appendBytes(t, path, append([]byte{0, 1, 0, 0}, make([]byte, 32<<10)...))

	again := w.fresh()
	wr2 := NewWriter(path)
	defer wr2.Close()
	if _, loss, refused := wr2.Recover(again, false); loss != nil || refused != nil {
		t.Fatalf("a torn final frame: loss %v, refused %v", loss, refused)
	}
	w.commit(t, again, wr2, 40)
	sameChain(t, path, again)
	if got := fileSize(t, path); got != wr2.size {
		t.Fatalf("the file is %d bytes and its frames end at %d: the torn frame was left behind", got, wr2.size)
	}
}

func appendBytes(t *testing.T, path string, b []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
}

func hashAt(c *chain.Chain, h int) ports.Hash {
	b := c.BlockAt(uint64(h))
	return b.Hash()
}

// The proofs a node sheds leave the file too, not only memory. Each renewal here
// carries a proof the size of a real one, so the file is mostly proofs, as a young
// network's is; by the time the horizon has passed most of them, a reader of the
// file must see them shed, and the shed proofs the file still carries must stay
// under a quarter of it. The saves that rewrite the file to get there stay few.
func TestShedProofsLeaveTheFile(t *testing.T) {
	w := newObjectiveWorld()
	w.answer = append([]byte("valid"), make([]byte, 120<<10)...)
	path := filepath.Join(t.TempDir(), "chain.cbor")
	c := w.fresh()
	wr := NewWriter(path)
	defer wr.Close()
	w.genesis(t, c)
	const n = 300
	rewrites := 0
	for i := 0; i < n; i++ {
		before := wr.compactAt
		w.commit(t, c, wr, i)
		if wr.compactAt != before {
			rewrites++
		}
	}
	onDisk, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var shedOnDisk int
	var stale int64
	for h := range onDisk {
		b := &onDisk[h]
		if b.HeavyProofsShed() {
			shedOnDisk++
		}
		if b.CarriesHeavyProofs() && !c.CarriesHeavyProofsAt(uint64(h)) {
			stale += proofBytes(b)
		}
	}
	size := fileSize(t, path)
	if shedOnDisk == 0 {
		t.Fatalf("the file holds no block with its proofs shed (file %d bytes, chain released %d)", size, c.ReleasedBlocks())
	}
	if 4*stale >= size {
		t.Fatalf("the file still carries %d bytes of proofs the chain has shed, of %d", stale, size)
	}
	if rewrites > n/5 {
		t.Fatalf("%d of %d saves rewrote the file", rewrites, n)
	}
	t.Logf("file %d bytes, %d blocks shed on disk, %d stale proof bytes, %d rewrites in %d saves", size, shedOnDisk, stale, rewrites, n)
}
