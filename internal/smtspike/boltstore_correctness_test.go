package smtspike

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// The disk store is only worth measuring if it is correct. These tests are the
// local proof (build-immutable: LOCAL PROOF BEFORE BILLABLE) — the floor-box
// run confirms COST, never correctness, so correctness must be green here first.

// TestBoltStoreRootMatchesInMemory is the load-bearing correctness assertion: a
// trie backed by the batching disk store must produce the BYTE-IDENTICAL root
// to one backed by the reference in-memory store, over the same key set. If the
// batching (pending buffer + per-block flush) corrupted read-your-writes, the
// roots would diverge here rather than in the field.
func TestBoltStoreRootMatchesInMemory(t *testing.T) {
	const n, perBlock = 2000, 100

	memStore := simplemap.NewSimpleMap()
	mem := smt.NewSparseMerkleTrie(memStore, sha256.New())

	bs, err := newBoltStore(t.TempDir(), true)
	if err != nil {
		t.Fatalf("newBoltStore: %v", err)
	}
	defer bs.Close()
	disk := smt.NewSparseMerkleTrie(bs, sha256.New())

	// Apply the same keys in the same block-sized batches to both, flushing the
	// disk store once per block — the real per-block commit shape.
	for i := 0; i < n; i++ {
		if err := mem.Update(keyAt(i), present); err != nil {
			t.Fatalf("mem Update: %v", err)
		}
		if err := disk.Update(keyAt(i), present); err != nil {
			t.Fatalf("disk Update: %v", err)
		}
		if (i+1)%perBlock == 0 {
			if err := mem.Commit(); err != nil {
				t.Fatalf("mem Commit: %v", err)
			}
			if err := disk.Commit(); err != nil {
				t.Fatalf("disk Commit: %v", err)
			}
			if err := bs.Flush(); err != nil {
				t.Fatalf("bolt Flush: %v", err)
			}
			if !bytes.Equal(mem.Root(), disk.Root()) {
				t.Fatalf("roots diverged after block %d: mem=%x disk=%x",
					(i+1)/perBlock, mem.Root(), disk.Root())
			}
		}
	}

	// Len() is the NODE count (~2.24 nodes/key, PR #596), not the key count.
	// The disk store must agree with the reference store on it.
	if bs.Len() != memStore.Len() {
		t.Errorf("disk store Len()=%d, reference store Len()=%d — node counts must match",
			bs.Len(), memStore.Len())
	}
	t.Logf("%d keys → %d nodes (%.2f nodes/key), roots byte-identical throughout",
		n, bs.Len(), float64(bs.Len())/float64(n))
}

// TestBoltStoreRebuildFromDisk is the Q6 boot-rebuild property in miniature: a
// committed root, persisted, must be recoverable by REOPENING the store — the
// snapshot/persist path (as opposed to a full replay rebuild). A trie imported
// against the reopened store must serve the same membership proofs.
func TestBoltStoreRebuildFromDisk(t *testing.T) {
	const n = 1000
	dir := t.TempDir()

	// Build and flush, capturing the root, then close.
	bs, err := newBoltStore(dir, false) // fsync on: this is the durability path
	if err != nil {
		t.Fatalf("newBoltStore: %v", err)
	}
	trie := smt.NewSparseMerkleTrie(bs, sha256.New())
	for i := 0; i < n; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := bs.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	root := append([]byte(nil), trie.Root()...)
	if err := bs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen — a fresh process would see exactly this — and Import against the
	// persisted root.
	bs2, err := newBoltStore(dir, false)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer bs2.Close()
	reopened := smt.ImportSparseMerkleTrie(bs2, sha256.New(), root)

	if !bytes.Equal(reopened.Root(), root) {
		t.Fatalf("reopened root %x != persisted root %x", reopened.Root(), root)
	}
	// A membership proof must verify against the reopened tree — i.e. the nodes
	// genuinely came back from disk, not from a warm cache.
	k := keyAt(n / 2)
	proof, err := reopened.Prove(k)
	if err != nil {
		t.Fatalf("Prove after reopen: %v", err)
	}
	ok, err := smt.VerifyProof(proof, root, k, present, reopened.Spec())
	if err != nil || !ok {
		t.Fatalf("membership proof failed against reopened store (ok=%v, err=%v)", ok, err)
	}
	if bs2.Len() != 0 {
		// Len is process-local (the spike does not persist the count); a reopened
		// store reports 0 until keys are touched. Recorded so the number is not
		// mistaken for a bug — the real NodeStore would persist a count or derive
		// it, out of scope for the cost spike.
		t.Logf("note: reopened Len()=%d (count is process-local in the spike)", bs2.Len())
	}
}

// TestBoltStoreDeleteThenReadback covers the tombstone path through a flush: a
// key deleted in one block must read absent after the flush, and its absence
// proof must verify.
func TestBoltStoreDeleteThenReadback(t *testing.T) {
	bs, err := newBoltStore(t.TempDir(), true)
	if err != nil {
		t.Fatalf("newBoltStore: %v", err)
	}
	defer bs.Close()
	trie := smt.NewSparseMerkleTrie(bs, sha256.New())

	for i := 0; i < 500; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := bs.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	before := bs.Len()

	// Delete one key in a following block.
	victim := keyAt(42)
	if err := trie.Delete(victim); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit after delete: %v", err)
	}
	if err := bs.Flush(); err != nil {
		t.Fatalf("Flush after delete: %v", err)
	}

	root := trie.Root()
	proof, err := trie.Prove(victim)
	if err != nil {
		t.Fatalf("Prove(deleted): %v", err)
	}
	ok, err := smt.VerifyProof(proof, root, victim, emptyValue, trie.Spec())
	if err != nil || !ok {
		t.Fatalf("absence proof for a deleted key failed (ok=%v, err=%v)", ok, err)
	}
	// The absence proof above is the real correctness statement; Len is the
	// node count, which must drop when the delete restructures the trie.
	if bs.Len() >= before {
		t.Errorf("node count did not drop after delete: %d → %d", before, bs.Len())
	}
}
