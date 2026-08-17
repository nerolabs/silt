package node

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/proofcache"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// mkNode builds a bare single-node harness on the in-package link net, with the
// given chunk store and proof backing injected — enough to drive answerChallenge
// and the meta-site sweeps directly.
func mkNode(t *testing.T, store ports.ChunkStore, backing ports.ProofStore) *Node {
	t.Helper()
	sched := simclock.New()
	var id ports.NodeID
	id[0] = 7
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
	end := &linkEnd{net: ln, id: id}
	ln.ends[id] = end
	n := New(id, DefaultConfig(), sched, end, store)
	n.SetProofStore(backing)
	n.LoadProofs()
	return n
}

// TestColdProofAnswersAuditIdentically is the refactor's core behavioral
// guarantee: moving the full proof (Path + PoR tags) out of a resident map and
// into a bounded cache over a backing store must NOT change the audit answer. A
// proof long evicted from the hot cache pages back from the backing and produces
// the byte-identical PoR proof it would have when resident. This is a
// where-it-LIVES change, not a where-it-VERIFIES change.
func TestColdProofAnswersAuditIdentically(t *testing.T) {
	// One honest shard with real bytes + real PoR tags.
	data := make([]byte, 6000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	chunk := ports.NewChunk(data)
	key := DerivePorKey(linkLayoutKey(t))
	tags := key.Tags(chunk.ID[:], data)
	var root ports.Hash
	root[0] = 0xA1
	proof := ports.StorageProof{Root: root, Index: 0, Total: 1, Column: -1, PorTags: tags}

	// The challenge both nodes answer.
	var seed [32]byte
	seed[0] = 0x9
	msg := ports.Message{Kind: ports.MsgChallenge, ChunkID: chunk.ID, PorSeed: seed[:], PorCount: len(tags)}

	answerWith := func(budget int64, evict int) ports.Message {
		store := memstore.New()
		store.Put(bg(), chunk)
		backingMem := memproofs.New()
		backingMem.Put(chunk.ID, proof)
		// Pre-load other proofs so the tiny cache is under pressure; then a
		// cold read of our target must page from the backing.
		for i := 0; i < evict; i++ {
			var id ports.ChunkID
			id[0], id[1] = byte(i), byte(i>>8)
			var r ports.Hash
			r[0] = byte(i + 1)
			backingMem.Put(id, ports.StorageProof{Root: r, Total: 1, Column: -1,
				PorTags: [][]byte{make([]byte, 32), make([]byte, 32)}})
		}
		var backing ports.ProofStore = backingMem
		if budget > 0 {
			pc := proofcache.Open(backingMem, budget)
			// Page every filler through the cache to evict our target if it were ever warm.
			for i := 0; i < evict; i++ {
				var id ports.ChunkID
				id[0], id[1] = byte(i), byte(i>>8)
				pc.Get(id)
			}
			backing = pc
		}
		n := mkNode(t, store, backing)
		return n.answerChallenge(msg)
	}

	// Roomy cache (proof effectively always resident) vs a 1-proof cache with
	// heavy eviction pressure (our target is cold, paged from backing).
	hot := answerWith(1<<20, 0)
	cold := answerWith(proofcache.SizeOf(proof)+1, 64)

	if !hot.Found || !cold.Found {
		t.Fatalf("both answers must produce a proof: hot.Found=%v cold.Found=%v", hot.Found, cold.Found)
	}
	if hot.PorBlocks != cold.PorBlocks {
		t.Fatalf("cold-paged proof differs from resident: blocks %d vs %d", hot.PorBlocks, cold.PorBlocks)
	}
	if !bytes.Equal(hot.PorSigma, cold.PorSigma) {
		t.Fatal("aggregated tag (sigma) differs between resident and cold-paged proof")
	}
	if len(hot.PorMu) != len(cold.PorMu) {
		t.Fatalf("mu length differs hot=%d cold=%d", len(hot.PorMu), len(cold.PorMu))
	}
	for i := range hot.PorMu {
		if !bytes.Equal(hot.PorMu[i], cold.PorMu[i]) {
			t.Fatalf("mu[%d] differs between resident and cold-paged proof", i)
		}
	}
}

// TestResidentMetaAtScale is the node-level memory wall: with a disk full of
// proofs behind a TINY hot-proof cache, every held chunk's small fields stay
// available at the meta sites (HeldRoots, chunkDenied) and its full proof still
// pages back identical — while the resident hot-cache RAM stays bounded, not
// O(held). This is the shape of the OOM fix in the node, not just the adapter.
func TestResidentMetaAtScale(t *testing.T) {
	const N = 2000
	store := memstore.New()
	backingMem := memproofs.New()
	roots := make(map[ports.ChunkID]ports.Hash, N)
	for i := 0; i < N; i++ {
		// Distinct chunk + distinct root, one shard per root.
		c := ports.NewChunk([]byte{byte(i), byte(i >> 8), byte(i >> 16), 0xEE})
		store.Put(bg(), c)
		var r ports.Hash
		r[0], r[1], r[2] = byte(i), byte(i>>8), byte(i>>16)
		roots[c.ID] = r
		backingMem.Put(c.ID, ports.StorageProof{Root: r, Index: 0, Total: 1, Column: -1,
			PorTags: [][]byte{make([]byte, 32), make([]byte, 32)}})
	}

	// A cache far smaller than N proofs' worth: room for ~16 hot, not 2000.
	perProof := proofcache.SizeOf(ports.StorageProof{Column: -1, PorTags: [][]byte{make([]byte, 32), make([]byte, 32)}})
	budget := perProof * 16
	pc := proofcache.Open(backingMem, budget)
	n := mkNode(t, store, pc)

	// Every held chunk's ROOT is available at the meta sites without paging.
	held := n.HeldRoots()
	total := 0
	for _, r := range roots {
		total += held[r]
	}
	if total != N {
		t.Fatalf("HeldRoots saw %d of %d roots — resident metadata is not the existence authority", total, N)
	}

	// The full proof of ANY chunk (including long-cold ones) pages back identical.
	for id, r := range roots {
		p, ok, err := n.proofs.Get(id)
		if err != nil || !ok {
			t.Fatalf("paging proof for a held chunk failed: ok=%v err=%v", ok, err)
		}
		if p.Root != r || len(p.PorTags) != 2 {
			t.Fatalf("paged proof mismatched: root %x vs %x, tags %d", p.Root, r, len(p.PorTags))
		}
	}

	// The hot-proof cache never exceeded its budget: resident RAM is O(hot),
	// not O(N) — the wall.
	if _, _, used := pc.Stats(); used > budget {
		t.Fatalf("resident hot-proof RAM %d blew the O(hot) budget %d", used, budget)
	}
}
