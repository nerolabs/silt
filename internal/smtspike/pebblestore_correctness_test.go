package smtspike

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// The pebble adapter must clear the same correctness bar as bbolt before it can
// join the billable floor-box run (LOCAL PROOF BEFORE BILLABLE). These mirror
// the boltStore tests exactly, so the two backends are held to one standard.

func TestPebbleStoreRootMatchesInMemory(t *testing.T) {
	const n, perBlock = 2000, 100

	memStore := simplemap.NewSimpleMap()
	mem := smt.NewSparseMerkleTrie(memStore, sha256.New())

	ps, err := newPebbleStore(t.TempDir(), false)
	if err != nil {
		t.Fatalf("newPebbleStore: %v", err)
	}
	defer ps.Close()
	disk := smt.NewSparseMerkleTrie(ps, sha256.New())

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
			if err := ps.Flush(false); err != nil {
				t.Fatalf("pebble Flush: %v", err)
			}
			if !bytes.Equal(mem.Root(), disk.Root()) {
				t.Fatalf("roots diverged after block %d: mem=%x disk=%x",
					(i+1)/perBlock, mem.Root(), disk.Root())
			}
		}
	}
	if ps.Len() != memStore.Len() {
		t.Errorf("pebble Len()=%d, reference Len()=%d — node counts must match",
			ps.Len(), memStore.Len())
	}
	t.Logf("%d keys → %d nodes (%.2f nodes/key), roots byte-identical throughout",
		n, ps.Len(), float64(ps.Len())/float64(n))
}

func TestPebbleStoreRebuildFromDisk(t *testing.T) {
	const n = 1000
	dir := t.TempDir()

	ps, err := newPebbleStore(dir, true)
	if err != nil {
		t.Fatalf("newPebbleStore: %v", err)
	}
	trie := smt.NewSparseMerkleTrie(ps, sha256.New())
	for i := 0; i < n; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := ps.Flush(true); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	root := append([]byte(nil), trie.Root()...)
	if err := ps.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ps2, err := newPebbleStore(dir, true)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer ps2.Close()
	reopened := smt.ImportSparseMerkleTrie(ps2, sha256.New(), root)

	if !bytes.Equal(reopened.Root(), root) {
		t.Fatalf("reopened root %x != persisted %x", reopened.Root(), root)
	}
	k := keyAt(n / 2)
	proof, err := reopened.Prove(k)
	if err != nil {
		t.Fatalf("Prove after reopen: %v", err)
	}
	if ok, err := smt.VerifyProof(proof, root, k, present, reopened.Spec()); err != nil || !ok {
		t.Fatalf("membership proof failed against reopened pebble (ok=%v, err=%v)", ok, err)
	}
}

func TestPebbleStoreDeleteThenReadback(t *testing.T) {
	ps, err := newPebbleStore(t.TempDir(), false)
	if err != nil {
		t.Fatalf("newPebbleStore: %v", err)
	}
	defer ps.Close()
	trie := smt.NewSparseMerkleTrie(ps, sha256.New())

	for i := 0; i < 500; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := ps.Flush(false); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	before := ps.Len()

	victim := keyAt(42)
	if err := trie.Delete(victim); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit after delete: %v", err)
	}
	if err := ps.Flush(false); err != nil {
		t.Fatalf("Flush after delete: %v", err)
	}

	root := trie.Root()
	proof, err := trie.Prove(victim)
	if err != nil {
		t.Fatalf("Prove(deleted): %v", err)
	}
	if ok, err := smt.VerifyProof(proof, root, victim, emptyValue, trie.Spec()); err != nil || !ok {
		t.Fatalf("absence proof for a deleted key failed (ok=%v, err=%v)", ok, err)
	}
	if ps.Len() >= before {
		t.Errorf("node count did not drop after delete: %d → %d", before, ps.Len())
	}
}
