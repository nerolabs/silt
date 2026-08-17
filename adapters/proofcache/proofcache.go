// Package proofcache is a read-through LRU cache in front of any
// ports.ProofStore, the proof analogue of adapters/cachestore. It exists to
// bound a daemon's RESIDENT proof RAM: the node keeps tiny metadata for every
// held chunk always-resident, but the big fields of a StorageProof (the Merkle
// Path and the per-block PoR tags, ~5 KB each) live in the backing store and are
// paged into this cache only when a proof is actually served or audited. So
// resident proof RAM is O(hot), not O(total held chunks) — the daemon OOM fix.
//
// Bounded by a byte budget; least-recently-used proofs evict when it is
// exceeded. Cache-on-read only: a Put writes straight through to the backing
// without warming the cache, so a bulk publish of just-stored-but-never-served
// proofs cannot evict genuinely hot ones (basic scan resistance). Delete evicts,
// so a purged or denylisted proof is never served from RAM.
package proofcache

import (
	"container/list"
	"sync"

	"github.com/nerolabs/silt/ports"
)

// SizeOf is the resident cost proofcache charges a proof against its budget: the
// variable payload (Path hashes + PoR tags) plus a fixed per-entry overhead for
// the map/list bookkeeping and the small scalar fields. Exported so callers and
// tests can size a budget in terms of "how many hot proofs".
func SizeOf(p ports.StorageProof) int64 {
	const fixed = 96 // scalars + map element + list node + key, approx
	n := int64(fixed)
	n += int64(len(p.Path)) * 32
	for _, tag := range p.PorTags {
		n += int64(len(tag))
	}
	return n
}

type entry struct {
	id   ports.ChunkID
	p    ports.StorageProof
	size int64
}

type Store struct {
	inner  ports.ProofStore
	budget int64

	mu    sync.Mutex
	items map[ports.ChunkID]*list.Element // id -> *entry element
	ll    *list.List                      // front = most recently used
	used  int64
	hits  int64
	miss  int64
}

var _ ports.ProofStore = (*Store)(nil)

// Open wraps inner with a proof cache of at most budget bytes. A budget of zero
// or less is a programming error — callers pass a sane default.
func Open(inner ports.ProofStore, budget int64) *Store {
	return &Store{
		inner:  inner,
		budget: budget,
		items:  make(map[ports.ChunkID]*list.Element),
		ll:     list.New(),
	}
}

// Get returns a proof, serving from RAM on a hit and paging from the backing on
// a miss (then admitting it). ok is false only if the backing has no such proof.
func (s *Store) Get(id ports.ChunkID) (ports.StorageProof, bool, error) {
	s.mu.Lock()
	if el, ok := s.items[id]; ok {
		s.ll.MoveToFront(el)
		s.hits++
		out := copyProof(el.Value.(*entry).p) // don't hand out the cached aliasable slices
		s.mu.Unlock()
		return out, true, nil
	}
	s.miss++
	s.mu.Unlock()

	p, ok, err := s.inner.Get(id)
	if err != nil || !ok {
		return ports.StorageProof{}, ok, err
	}
	s.admit(id, p)
	return copyProof(p), true, nil
}

// admit inserts a freshly-paged proof, evicting LRU entries until the budget
// holds. A proof larger than the whole budget is never cached (it would evict
// everything and still not fit) — it just pages on every serve.
func (s *Store) admit(id ports.ChunkID, p ports.StorageProof) {
	size := SizeOf(p)
	if size > s.budget {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; ok {
		return // raced with another reader; keep the incumbent
	}
	el := s.ll.PushFront(&entry{id: id, p: copyProof(p), size: size})
	s.items[id] = el
	s.used += size
	for s.used > s.budget {
		s.evictLRU()
	}
}

// evictLRU drops the least-recently-used entry. Caller holds s.mu.
func (s *Store) evictLRU() {
	el := s.ll.Back()
	if el == nil {
		return
	}
	s.drop(el)
}

// drop removes el from the cache. Caller holds s.mu.
func (s *Store) drop(el *list.Element) {
	e := el.Value.(*entry)
	s.ll.Remove(el)
	delete(s.items, e.id)
	s.used -= e.size
}

// Put write-throughs to the backing WITHOUT warming the cache (scan resistance):
// a flood of stored-but-never-served proofs cannot evict hot ones.
func (s *Store) Put(id ports.ChunkID, p ports.StorageProof) error {
	return s.inner.Put(id, p)
}

// Delete evicts from the cache and deletes from the backing, so a purged or
// denylisted proof is gone from both RAM and disk.
func (s *Store) Delete(id ports.ChunkID) error {
	s.mu.Lock()
	if el, ok := s.items[id]; ok {
		s.drop(el)
	}
	s.mu.Unlock()
	return s.inner.Delete(id)
}

// Keys delegates: the backing is authoritative about what exists; the cache is
// only a read accelerator over a hot subset.
func (s *Store) Keys() ([]ports.ChunkID, error) { return s.inner.Keys() }

// Load delegates: whole-store reads bypass the cache (they'd blow it anyway).
func (s *Store) Load() (map[ports.ChunkID]ports.StorageProof, error) { return s.inner.Load() }

// Stats reports cache effectiveness: cumulative hits and misses, and the bytes
// currently resident. For observability — is the proof cache earning its RAM?
func (s *Store) Stats() (hits, misses, usedBytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits, s.miss, s.used
}

// copyProof deep-copies the aliasable slices so neither a caller nor the backing
// can mutate what the cache holds (and vice-versa).
func copyProof(p ports.StorageProof) ports.StorageProof {
	if p.Path != nil {
		p.Path = append([]ports.Hash(nil), p.Path...)
	}
	if p.PorTags != nil {
		tags := make([][]byte, len(p.PorTags))
		for i, t := range p.PorTags {
			tags[i] = append([]byte(nil), t...)
		}
		p.PorTags = tags
	}
	return p
}
