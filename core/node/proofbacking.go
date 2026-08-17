package node

import "github.com/nerolabs/silt/ports"

// proofMeta is the small, ALWAYS-RESIDENT slice of a StorageProof: the fields the
// hot-path proof sites need without paging the big Path+PorTags. Existence
// checks, the iterate-all sweeps (HeldRoots, EnforceDenylist, chunkDenied), and
// re-announce (placementKey needs Root+Column) all read only these. ~80-100 B per
// held chunk resident, versus ~5.4 KB for the full proof — so a node with a disk
// full of chunks keeps O(held) TINY metadata resident, and the full proofs live
// in the backing store, paged into a bounded cache only to serve or audit. That
// is the daemon OOM fix: resident proof RAM becomes O(hot), not O(total held).
type proofMeta struct {
	Root         ports.Hash
	Index, Total int
	Column       int
}

func metaOf(p ports.StorageProof) proofMeta {
	return proofMeta{Root: p.Root, Index: p.Index, Total: p.Total, Column: p.Column}
}

// memProofs is the DEFAULT ports.ProofStore for a node with no durable proof
// store injected (sims, ephemeral clients): the full StorageProofs live in RAM.
// That is fine because such nodes are small. A daemon injects a bounded,
// disk-backed store — adapters/proofcache over adapters/diskproofs — via
// SetProofStore, which is what makes resident proof RAM O(hot). Kept in core (not
// adapters/memproofs) so the hexagonal core takes no adapter dependency (B1);
// lock-free because it is only ever touched from the node's single serialized
// loop (B2).
type memProofs struct {
	m map[ports.ChunkID]ports.StorageProof
}

func newMemProofs() *memProofs {
	return &memProofs{m: make(map[ports.ChunkID]ports.StorageProof)}
}

var _ ports.ProofStore = (*memProofs)(nil)

func (s *memProofs) Put(id ports.ChunkID, p ports.StorageProof) error {
	s.m[id] = copyProof(p) // deep-copy so a later caller mutation can't alias
	return nil
}

func (s *memProofs) Get(id ports.ChunkID) (ports.StorageProof, bool, error) {
	p, ok := s.m[id]
	if !ok {
		return ports.StorageProof{}, false, nil
	}
	return copyProof(p), true, nil
}

func (s *memProofs) Keys() ([]ports.ChunkID, error) {
	out := make([]ports.ChunkID, 0, len(s.m))
	for id := range s.m {
		out = append(out, id)
	}
	return out, nil
}

func (s *memProofs) Load() (map[ports.ChunkID]ports.StorageProof, error) {
	out := make(map[ports.ChunkID]ports.StorageProof, len(s.m))
	for id, p := range s.m {
		out[id] = copyProof(p)
	}
	return out, nil
}

func (s *memProofs) Delete(id ports.ChunkID) error {
	delete(s.m, id)
	return nil
}
