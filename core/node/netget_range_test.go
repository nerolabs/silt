package node

import (
	"bytes"
	"context"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/ports"
)

// putCounter counts the chunks a node writes into its store: every piece a fetch pulls.
type putCounter struct {
	ports.ChunkStore
	puts map[ports.ChunkID]int
}

func (p *putCounter) Put(ctx context.Context, c ports.Chunk) error {
	p.puts[c.ID]++
	return p.ChunkStore.Put(ctx, c)
}

func rangeConsumer(r *netgetRig, seed int64) (*Node, *putCounter) {
	store := &putCounter{ChunkStore: memstore.New(), puts: map[ports.ChunkID]int{}}
	id := identity.FromSeed(seed).NodeID()
	cfg := DefaultConfig()
	cfg.Replication = 1
	nd := New(id, cfg, r.sched, r.net.Endpoint(id), store)
	nd.Bootstrap([]ports.NodeID{r.nodes[0].ID(), r.nodes[1].ID(), r.nodes[2].ID()}, func() {})
	r.sched.Run()
	return nd, store
}

func (r *netgetRig) getRange(t *testing.T, nd *Node, off, n int64) []byte {
	t.Helper()
	var out bytes.Buffer
	var err error
	finished := false
	nd.NetGetRange(r.reg, r.h, off, n, &out, func(e error) { err, finished = e, true })
	r.sched.Run()
	if !finished || err != nil {
		t.Fatalf("NetGetRange(%d, %d): finished=%v err=%v", off, n, finished, err)
	}
	return out.Bytes()
}

// TestARangedFetchPullsOnlyItsChunks. Over the network, a range inside one frame pulls that
// one data chunk (plus the manifest, once) and nothing else; a second range reuses the
// cached manifest; and nothing the fetches pulled is kept.
func TestARangedFetchPullsOnlyItsChunks(t *testing.T) {
	r := newNetgetRig(t)
	nd, store := rangeConsumer(r, 3991)
	payload := r.m.ChunkSize - chunk.HeaderSize
	entry, _, _ := r.reg.Lookup(bg(), r.h.Root)

	off := 13*payload + 7
	if got := r.getRange(t, nd, off, 50); !bytes.Equal(got, r.data[off:off+50]) {
		t.Fatal("the range came back wrong")
	}
	leaves := 0
	for id := range store.puts {
		for _, l := range r.m.Leaves() {
			if l == id {
				leaves++
			}
		}
	}
	if leaves != 1 || store.puts[r.m.ChunkIDs()[13]] != 1 {
		t.Fatalf("a range inside frame 13 pulled %d file shard(s); want only chunk 13 (puts: %d)", leaves, len(store.puts))
	}
	for _, id := range entry.ManifestChunks {
		if store.puts[id] != 1 {
			t.Fatalf("the manifest chunk was pulled %d times on the first range, want 1", store.puts[id])
		}
	}

	// A second range crossing the stripe boundary (chunks 9 and 10): the manifest comes from
	// the cache, so no manifest chunk is pulled again.
	off = 10*payload - 20
	if got := r.getRange(t, nd, off, 40); !bytes.Equal(got, r.data[off:off+40]) {
		t.Fatal("the stripe-crossing range came back wrong")
	}
	for _, id := range entry.ManifestChunks {
		if store.puts[id] != 1 {
			t.Fatalf("the second range pulled the manifest again (%d puts) instead of using the cache", store.puts[id])
		}
	}
	if held := r.heldOf(nd, r.candidates(t)); held != 0 {
		t.Fatalf("a ranged fetch kept %d pieces it pulled", held)
	}
}

// TestARangedFetchRebuildsAChunkNoOneHolds: the only holder of a covered chunk is gone, so
// the range is rebuilt from its stripe's other shards and parity, and is exact.
func TestARangedFetchRebuildsAChunkNoOneHolds(t *testing.T) {
	r := newNetgetRig(t)
	target := r.m.ChunkIDs()[4]
	for _, nd := range r.nodes {
		if ok, _ := nd.Store().Has(bg(), target); ok {
			nd.Store().Delete(bg(), target)
		}
	}
	nd, _ := rangeConsumer(r, 3992)
	payload := r.m.ChunkSize - chunk.HeaderSize
	off := 4*payload + 100
	if got := r.getRange(t, nd, off, 300); !bytes.Equal(got, r.data[off:off+300]) {
		t.Fatal("a range over a chunk no one holds came back wrong")
	}
	if nd.Stats.ParityColumnLookups == 0 {
		t.Fatal("CONTROL: no parity was used, so the lost chunk was not actually lost")
	}
}

func TestARangedFetchPastTheEndIsRefused(t *testing.T) {
	r := newNetgetRig(t)
	nd, _ := rangeConsumer(r, 3993)
	var err error
	nd.NetGetRange(r.reg, r.h, int64(len(r.data)), 1, &bytes.Buffer{}, func(e error) { err = e })
	r.sched.Run()
	if err == nil {
		t.Fatal("a range starting at the end of the file was accepted")
	}
}

// lookupCounter counts registry lookups.
type lookupCounter struct {
	ports.Registry
	n int
}

func (l *lookupCounter) Lookup(ctx context.Context, root ports.Hash) (ports.Entry, bool, error) {
	l.n++
	return l.Registry.Lookup(ctx, root)
}

// TestASecondRangeSkipsTheRegistry: once a file's manifest is open, later ranges and size
// queries make no registry round trip — a player reads one file many times a minute.
func TestASecondRangeSkipsTheRegistry(t *testing.T) {
	r := newNetgetRig(t)
	reg := &lookupCounter{Registry: r.reg}
	r.reg = reg
	nd, _ := rangeConsumer(r, 3994)
	r.getRange(t, nd, 0, 100)
	first := reg.n
	if first == 0 {
		t.Fatal("CONTROL: the first range never looked the file up")
	}
	r.getRange(t, nd, 5000, 100)
	var size int64
	nd.NetFileSize(r.reg, r.h, func(n int64, err error) { size = n })
	r.sched.Run()
	if reg.n != first {
		t.Fatalf("later reads made %d more registry lookups; a cached manifest needs none", reg.n-first)
	}
	if size != int64(len(r.data)) {
		t.Fatalf("NetFileSize = %d, want %d", size, len(r.data))
	}
}

// TestTheManifestCacheIsBoundedByBytes: it holds many small manifests, evicts oldest first
// when the byte budget is reached, and never caches one larger than the whole budget.
func TestTheManifestCacheIsBoundedByBytes(t *testing.T) {
	var c manifestCache
	mk := func(chunks int) *manifest.Manifest {
		m := &manifest.Manifest{Chunks: make([][]byte, chunks)}
		return m
	}
	h := func(i int) link.Handle { return link.Handle{Root: ports.HashBytes([]byte{byte(i), byte(i >> 8)})} }
	for i := 0; i < 100; i++ {
		c.put(h(i), mk(100))
	}
	if len(c.order) != 100 || c.get(h(0)) == nil {
		t.Fatalf("100 small manifests: %d cached; all should fit", len(c.order))
	}
	big := (manifestCacheBytes / 56) / 3 // a third of the budget each
	for i := 1000; i < 1004; i++ {
		c.put(h(i), mk(big))
	}
	if c.bytes > manifestCacheBytes {
		t.Fatalf("cache holds %d bytes, over its %d budget", c.bytes, manifestCacheBytes)
	}
	if c.get(h(0)) != nil {
		t.Fatal("the oldest entry survived a byte-budget eviction")
	}
	if c.get(h(1003)) == nil {
		t.Fatal("the newest entry was evicted")
	}
	c.put(h(9999), mk(2*manifestCacheBytes/56))
	if c.get(h(9999)) != nil {
		t.Fatal("a manifest larger than the whole budget was cached")
	}
}
