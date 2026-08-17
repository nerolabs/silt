// Package prooftest is a conformance suite every ports.ProofStore adapter must
// pass — one behavioral contract, however many implementations (memproofs,
// diskproofs, and the proofcache that fronts them). It exercises the Get/Keys
// per-id surface the OOM fix relies on, not just the whole-store Load.
package prooftest

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// mkProof builds a distinct, fully-populated proof (Root, Path, PorTags, Column)
// so a round-trip that drops any field is caught.
func mkProof(b byte, col int) (ports.ChunkID, ports.StorageProof) {
	var id, root, p0, p1 ports.Hash
	id[0], root[0], p0[0], p1[0] = b, b+1, b+2, b+3
	tag0, tag1 := make([]byte, 32), make([]byte, 32)
	tag0[0], tag1[0] = b+4, b+5
	return id, ports.StorageProof{
		Root: root, Index: int(b), Total: 8,
		Path: []ports.Hash{p0, p1}, Column: col, PorTags: [][]byte{tag0, tag1},
	}
}

func eq(a, b ports.StorageProof) bool {
	if a.Root != b.Root || a.Index != b.Index || a.Total != b.Total || a.Column != b.Column {
		return false
	}
	if len(a.Path) != len(b.Path) {
		return false
	}
	for i := range a.Path {
		if a.Path[i] != b.Path[i] {
			return false
		}
	}
	if len(a.PorTags) != len(b.PorTags) {
		return false
	}
	for i := range a.PorTags {
		if !bytes.Equal(a.PorTags[i], b.PorTags[i]) {
			return false
		}
	}
	return true
}

// Run drives the ProofStore contract. newStore returns a fresh, empty store.
func Run(t *testing.T, newStore func(t *testing.T) ports.ProofStore) {
	t.Run("PutGetRoundtrip", func(t *testing.T) {
		s := newStore(t)
		id, pr := mkProof(1, 3)
		if err := s.Put(id, pr); err != nil {
			t.Fatal(err)
		}
		got, ok, err := s.Get(id)
		if err != nil || !ok {
			t.Fatalf("Get(stored): ok=%v err=%v", ok, err)
		}
		if !eq(got, pr) {
			t.Fatalf("Get returned a different proof:\n got %+v\nwant %+v", got, pr)
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		s := newStore(t)
		id, _ := mkProof(9, -1)
		if _, ok, err := s.Get(id); ok || err != nil {
			t.Fatalf("Get(missing): want (_, false, nil), got ok=%v err=%v", ok, err)
		}
	})

	t.Run("Keys", func(t *testing.T) {
		s := newStore(t)
		id1, pr1 := mkProof(1, 3)
		id2, pr2 := mkProof(2, -1) // uncoded/manifest-style proof (column -1)
		if err := s.Put(id1, pr1); err != nil {
			t.Fatal(err)
		}
		if err := s.Put(id2, pr2); err != nil {
			t.Fatal(err)
		}
		keys, err := s.Keys()
		if err != nil {
			t.Fatal(err)
		}
		seen := map[ports.ChunkID]bool{}
		for _, k := range keys {
			seen[k] = true
		}
		if len(keys) != 2 || !seen[id1] || !seen[id2] {
			t.Fatalf("Keys = %v, want exactly {id1,id2}", keys)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		s := newStore(t)
		id, pr := mkProof(5, 0)
		if err := s.Put(id, pr); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(id); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := s.Get(id); ok {
			t.Fatal("Get after Delete: still present")
		}
		if err := s.Delete(id); err != nil {
			t.Fatalf("Delete of absent id must be a no-op, got %v", err)
		}
	})

	t.Run("Load", func(t *testing.T) {
		s := newStore(t)
		id1, pr1 := mkProof(1, 3)
		id2, pr2 := mkProof(2, -1)
		if err := s.Put(id1, pr1); err != nil {
			t.Fatal(err)
		}
		if err := s.Put(id2, pr2); err != nil {
			t.Fatal(err)
		}
		m, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		if len(m) != 2 || !eq(m[id1], pr1) || !eq(m[id2], pr2) {
			t.Fatalf("Load = %+v, want both proofs intact", m)
		}
	})
}
