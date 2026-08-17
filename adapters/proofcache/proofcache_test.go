package proofcache_test

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memproofs"
	"github.com/nerolabs/silt/adapters/proofcache"
	"github.com/nerolabs/silt/adapters/prooftest"
	"github.com/nerolabs/silt/ports"
)

// A proof cache is still a ProofStore: it must honor the whole contract.
func TestConformance(t *testing.T) {
	prooftest.Run(t, func(t *testing.T) ports.ProofStore {
		return proofcache.Open(memproofs.New(), 1<<20)
	})
}

// mkProof builds a proof of a known resident size: 2 Path hashes (64 B) + 2
// 32-byte PorTags (64 B) = 128 B of variable payload, plus proofcache's fixed
// per-entry overhead.
func mkProof(b byte) (ports.ChunkID, ports.StorageProof) {
	var id, root, p0, p1 ports.Hash
	id[0], id[1] = b, b>>4 // spread ids so none collide
	root[0] = b + 1
	tag0, tag1 := make([]byte, 32), make([]byte, 32)
	return id, ports.StorageProof{
		Root: root, Index: int(b), Total: 8, Column: int(b) % 4,
		Path: []ports.Hash{p0, p1}, PorTags: [][]byte{tag0, tag1},
	}
}

// THE MEMORY WALL (failing-first): resident proof RAM must stay O(hot), not
// O(N). Store N proofs into a cache far smaller than N proofs' worth and read
// every one; the resident bytes must never exceed the budget, no matter how
// large N grows. This is the property that turns the OOM (O(total held) resident)
// into O(hot).
func TestResidentRAMStaysBounded(t *testing.T) {
	const N = 5000
	inner := memproofs.New()
	_, sample := mkProofN(0)
	perProof := proofcache.SizeOf(sample) // one proof's resident cost
	budget := perProof * 16               // room for ~16 hot proofs, not N
	cache := proofcache.Open(inner, budget)

	for i := 0; i < N; i++ {
		id, pr := mkProofN(i)
		if err := cache.Put(id, pr); err != nil { // write-through to inner
			t.Fatal(err)
		}
	}
	// Read all N back (each a miss that admits + may evict). The cache must
	// bound itself the whole way.
	for i := 0; i < N; i++ {
		id, _ := mkProofN(i)
		if _, ok, err := cache.Get(id); err != nil || !ok {
			t.Fatalf("Get(%d): ok=%v err=%v — inner lost the write-through", i, ok, err)
		}
		if _, _, used := cache.Stats(); used > budget {
			t.Fatalf("resident RAM blew the budget at i=%d: used=%d > budget=%d (O(N), not O(hot))", i, used, budget)
		}
	}
	if _, _, used := cache.Stats(); used > budget {
		t.Fatalf("final resident=%d > budget=%d", used, budget)
	}
}

func mkProofN(i int) (ports.ChunkID, ports.StorageProof) {
	var id, root, p0, p1 ports.Hash
	id[0], id[1], id[2], id[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
	root[0] = byte(i)
	tag0, tag1 := make([]byte, 32), make([]byte, 32)
	return id, ports.StorageProof{
		Root: root, Index: i, Total: 8, Column: i % 4,
		Path: []ports.Hash{p0, p1}, PorTags: [][]byte{tag0, tag1},
	}
}

// Scan resistance: a Put must NOT warm the cache (mirrors cachestore's property).
// A bulk publish of never-served proofs must not evict genuinely hot ones.
func TestPutDoesNotWarmCache(t *testing.T) {
	cache := proofcache.Open(memproofs.New(), 1<<20)
	id, pr := mkProof(1)
	if err := cache.Put(id, pr); err != nil {
		t.Fatal(err)
	}
	if _, _, used := cache.Stats(); used != 0 {
		t.Fatalf("cache-on-read only: a Put must not warm the cache, used=%d", used)
	}
}

func TestHitMissAndReadThrough(t *testing.T) {
	inner := memproofs.New()
	id, pr := mkProof(7)
	if err := inner.Put(id, pr); err != nil {
		t.Fatal(err)
	}
	cache := proofcache.Open(inner, 1<<20)

	if _, ok, err := cache.Get(id); err != nil || !ok { // miss: fills
		t.Fatalf("first get: ok=%v err=%v", ok, err)
	}
	if _, ok, err := cache.Get(id); err != nil || !ok { // hit
		t.Fatalf("second get: ok=%v err=%v", ok, err)
	}
	if h, m, _ := cache.Stats(); h != 1 || m != 1 {
		t.Fatalf("hits=%d misses=%d, want 1/1", h, m)
	}
}

func TestLRUEviction(t *testing.T) {
	inner := memproofs.New()
	ida, pa := mkProof(1)
	idb, pb := mkProof(2)
	idc, pc := mkProof(3)
	for _, kv := range []struct {
		id ports.ChunkID
		p  ports.StorageProof
	}{{ida, pa}, {idb, pb}, {idc, pc}} {
		if err := inner.Put(kv.id, kv.p); err != nil {
			t.Fatal(err)
		}
	}
	per := proofcache.SizeOf(pa)
	cache := proofcache.Open(inner, per*2+per/2) // room for two, not three

	cache.Get(ida) // [a]
	cache.Get(idb) // [b a]
	cache.Get(ida) // touch a -> [a b]
	cache.Get(idc) // admit c, evict LRU (b) -> [c a]

	h0, m0, _ := cache.Stats()
	if _, ok, _ := cache.Get(ida); !ok { // a resident -> hit
		t.Fatal("a get failed")
	}
	h1, m1, _ := cache.Stats()
	if h1 != h0+1 || m1 != m0 {
		t.Fatalf("a should be a hit (h %d->%d, m %d->%d)", h0, h1, m0, m1)
	}
	if _, ok, _ := cache.Get(idb); !ok { // b evicted -> miss, re-read from inner
		t.Fatal("b get failed")
	}
	if _, m2, _ := cache.Stats(); m2 != m1+1 {
		t.Fatalf("b was evicted, expected a miss (m %d->%d)", m1, m2)
	}
}

func TestDeleteEvictsFromCacheAndInner(t *testing.T) {
	inner := memproofs.New()
	id, pr := mkProof(4)
	if err := inner.Put(id, pr); err != nil {
		t.Fatal(err)
	}
	cache := proofcache.Open(inner, 1<<20)
	cache.Get(id) // cache it

	if err := cache.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := cache.Get(id); ok {
		t.Fatal("Delete must remove from both the cache and the inner store")
	}
	if _, _, used := cache.Stats(); used != 0 {
		t.Fatalf("Delete must free the cached bytes, used=%d", used)
	}
}
