package chainstore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// commitNext appends one committed block carrying entry tag to c.
func commitNext(t *testing.T, c *chain.Chain, tag string) chain.Block {
	t.Helper()
	prev, height := c.Head()
	b := &chain.Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{{
		Root:           ports.HashBytes([]byte(tag)),
		ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte(tag + "/m"))},
		FileSize:       1,
	}}}
	chain.Sign(b, testKey(1))
	for i := int64(2); i <= 5; i++ {
		b.Atts = append(b.Atts, chain.Attest(b, testKey(i)))
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("commit %s: %v", tag, err)
	}
	return *b
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

// sameChain fails unless the file at path holds exactly c's blocks.
func sameChain(t *testing.T, path string, c *chain.Chain) {
	t.Helper()
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := c.Blocks(0)
	if len(got) != len(want) {
		t.Fatalf("file holds %d blocks, chain %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Hash() != want[i].Hash() {
			t.Fatalf("block %d differs between the file and the chain", i)
		}
	}
}

// Persisting a commit writes that block's frame and nothing else, so its cost is
// the block's size however long the chain is. A Sync with nothing new writes
// nothing.
func TestWriterAppendsOnlyTheNewBlocks(t *testing.T) {
	c := persistedChain(t)
	path := filepath.Join(t.TempDir(), "chain.cbor")
	w := NewWriter(path)
	defer w.Close()
	if err := w.Sync(c); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	appends := 0
	for i := 0; i < 20; i++ {
		before, compactAt := fileSize(t, path), w.compactAt
		b := commitNext(t, c, fmt.Sprintf("append-%d", i))
		if err := w.Sync(c); err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
		if w.compactAt != compactAt {
			continue // this sync was a compaction
		}
		appends++
		if grew, frame := fileSize(t, path)-before, int64(len(appendFrames(nil, []chain.Block{b}))); grew != frame {
			t.Fatalf("commit %d grew the file by %d bytes; its frame is %d", i, grew, frame)
		}
	}
	if appends < 15 {
		t.Fatalf("only %d of 20 commits appended; the rest rewrote the file", appends)
	}
	before := fileSize(t, path)
	if err := w.Sync(c); err != nil || fileSize(t, path) != before {
		t.Fatalf("a sync with nothing new must write nothing: err=%v size %d → %d", err, before, fileSize(t, path))
	}
	sameChain(t, path, c)

	fresh := chain.New(chain.DefaultConfig(), func(ports.NodeID) int64 { return 0 })
	if n, err := Replay(path, fresh); err != nil || n != c.Len() {
		t.Fatalf("replay: n=%d err=%v, want %d", n, err, c.Len())
	}
}

// When the chain stops extending what the file holds (a reorg replaced its
// head), the file is rewritten to the chain as it now is.
func TestWriterRewritesAfterAReorg(t *testing.T) {
	ours, theirs := persistedChain(t), persistedChain(t)
	commitNext(t, ours, "ours-2")
	commitNext(t, ours, "ours-3")
	commitNext(t, theirs, "theirs-2")
	commitNext(t, theirs, "theirs-3")
	commitNext(t, theirs, "theirs-4")

	path := filepath.Join(t.TempDir(), "chain.cbor")
	w := NewWriter(path)
	defer w.Close()
	if err := w.Sync(ours); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(theirs); err != nil {
		t.Fatal(err)
	}
	sameChain(t, path, theirs)
}

// A file in the legacy whole-array format is read, and the first Sync rewrites
// it as a log that later commits append to.
func TestWriterMigratesALegacyFile(t *testing.T) {
	c := persistedChain(t)
	path := filepath.Join(t.TempDir(), "chain.cbor")
	if err := os.WriteFile(path, chain.EncodeBlocks(c.Blocks(0)), 0o644); err != nil {
		t.Fatal(err)
	}
	sameChain(t, path, c)

	w := NewWriter(path)
	defer w.Close()
	commitNext(t, c, "after-upgrade")
	if err := w.Sync(c); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, logMagic) {
		t.Fatal("the first sync must rewrite a legacy file as a log")
	}
	sameChain(t, path, c)
}

// Appends past twice the file's size at its last rewrite trigger a rewrite, so
// the file sheds what the replica has since pruned. The amortized cost per block
// stays constant: each rewrite follows appends at least as large as itself.
func TestWriterCompactsOnceAppendsDoubleTheFile(t *testing.T) {
	c := persistedChain(t)
	path := filepath.Join(t.TempDir(), "chain.cbor")
	w := NewWriter(path)
	defer w.Close()
	if err := w.Sync(c); err != nil {
		t.Fatal(err)
	}
	rewrites, last := 0, w.compactAt
	for i := 0; i < 200; i++ {
		commitNext(t, c, fmt.Sprintf("grow-%d", i))
		if err := w.Sync(c); err != nil {
			t.Fatal(err)
		}
		if w.compactAt != last {
			rewrites++
			if w.compactAt != 2*fileSize(t, path) {
				t.Fatalf("a rewrite must set the next one at twice the new size: %d vs %d", w.compactAt, fileSize(t, path))
			}
			last = w.compactAt
		}
	}
	if rewrites == 0 || rewrites > 12 {
		t.Fatalf("200 commits from a 2-block chain must rewrite a few times, doubling each time; got %d", rewrites)
	}
	sameChain(t, path, c)
}
