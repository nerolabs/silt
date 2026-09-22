package node

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/memproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/proofcache"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/core/por"
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
// guarantee: moving the full proof out of a resident map and into a bounded cache
// over a backing store must NOT change the audit answer. A proof long evicted from
// the hot cache pages back from the backing and produces the byte-identical opened
// leaves it would have when resident. This is a where-it-LIVES change, not a
// where-it-VERIFIES change.
func TestColdProofAnswersAuditIdentically(t *testing.T) {
	// One honest shard with real bytes.
	data := make([]byte, 6000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	chunk := ports.NewChunk(data)
	var root ports.Hash
	root[0] = 0xA1
	proof := ports.StorageProof{Root: root, Index: 0, Total: 1, Column: -1, LeafBytes: por.SpotLeafBytes}

	// The challenge both nodes answer.
	var seed [32]byte
	seed[0] = 0x9
	msg := ports.Message{Kind: ports.MsgChallenge, ChunkID: chunk.ID, PorSeed: seed[:], PorCount: porSampleCount}

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
				Path: []ports.Hash{{}, {}}, LeafBytes: por.SpotLeafBytes})
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
		t.Fatalf("cold-paged proof differs from resident: leaves %d vs %d", hot.PorBlocks, cold.PorBlocks)
	}
	if len(hot.PorOpen) == 0 {
		t.Fatal("the resident answer opened no leaves — the comparison below would hold vacuously")
	}
	if len(hot.PorOpen) != len(cold.PorOpen) || len(hot.PorPaths) != len(cold.PorPaths) {
		t.Fatalf("opened-leaf count differs hot=%d/%d cold=%d/%d",
			len(hot.PorOpen), len(hot.PorPaths), len(cold.PorOpen), len(cold.PorPaths))
	}
	for i := range hot.PorOpen {
		if !bytes.Equal(hot.PorOpen[i], cold.PorOpen[i]) {
			t.Fatalf("opened leaf %d differs between resident and cold-paged proof", i)
		}
		if !bytes.Equal(hot.PorPaths[i], cold.PorPaths[i]) {
			t.Fatalf("path %d differs between resident and cold-paged proof", i)
		}
	}
}

// TestColdLiarProofStillCaught is the liar twin of the honest cold-page test. The
// OOM fix moves the liar's proof out of a resident map and into the bounded cache
// over the backing — so a liar's proof CAN be evicted. The write-through keeps it
// durable in the backing precisely so that a cold-paged liar proof still produces
// the intended caught-as-liar signal: `Found=true` with an answer that FAILS
// verification — NOT the degenerate `Found=false` ("I don't have it") a lost proof
// would give, which would silently test the wrong path.
func TestColdLiarProofStillCaught(t *testing.T) {
	// The liar keeps the receipt but never the data (empty store).
	data := make([]byte, 6000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	chunk := ports.NewChunk(data)
	shardRoot := por.ShardRoot(data, por.SpotLeafBytes)
	leaves := por.SpotLeaves(len(data), por.SpotLeafBytes)
	var root ports.Hash
	root[0] = 0xB2
	proof := ports.StorageProof{Root: root, Index: 0, Total: 1, Column: -1, LeafBytes: por.SpotLeafBytes}

	// Proof durable in the backing; the cache is 1-proof and under eviction
	// pressure, so our target is COLD (paged from the backing on the audit).
	backingMem := memproofs.New()
	backingMem.Put(chunk.ID, proof)
	pc := proofcache.Open(backingMem, proofcache.SizeOf(proof)+1)
	for i := 0; i < 64; i++ {
		var id ports.ChunkID
		id[0], id[1] = byte(i), byte(i>>8)
		filler := ports.StorageProof{Total: 1, Column: -1,
			Path: []ports.Hash{{}, {}}, LeafBytes: por.SpotLeafBytes}
		backingMem.Put(id, filler)
		pc.Get(id) // warm+evict, pushing our target out of the hot cache
	}

	// Empty store: the liar has no bytes. answerChallenge's liar branch returns
	// before it ever reads the store, so this is the pure "keep the receipt, ditch
	// the goods" prover.
	n := mkNode(t, memstore.New(), pc)
	n.SetLiar(true)

	var seed [32]byte
	seed[0] = 0x7
	msg := ports.Message{Kind: ports.MsgChallenge, ChunkID: chunk.ID, PorSeed: seed[:], PorCount: porSampleCount}
	reply := n.answerChallenge(msg)

	// 1. The cold-paged liar proof still ANSWERS (Found=true) — not the degenerate
	// Found=false a lost/evicted proof would give. This is what the durable
	// write-through buys: the bad-μ catch path keeps running.
	if !reply.Found {
		t.Fatal("cold-paged liar proof returned Found=false — the write-through failed and the drill would silently test the wrong path")
	}
	// 2. And that answer FAILS verification: the liar is CAUGHT. It has no bytes to
	// open, so the answer carries no leaves at all and the auditor's committed root
	// has nothing to check — the same outcome as when the proof was resident.
	if por.VerifyOpenings(shardRoot, leaves, por.SpotLeafBytes, seed, porSampleCount,
		parseOpenings(reply, min(porSampleCount, leaves))) {
		t.Fatal("liar's cold-paged answer VERIFIED — it must fail (caught as a liar)")
	}
	if reply.PorBlocks == leaves {
		t.Fatalf("the liar reported the true leaf count %d — it holds no bytes and cannot know the committed "+
			"geometry, so this fixture is handing it the auditor's own number", leaves)
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
			Path: []ports.Hash{{}, {}}, LeafBytes: por.SpotLeafBytes})
	}

	// A cache far smaller than N proofs' worth: room for ~16 hot, not 2000.
	perProof := proofcache.SizeOf(ports.StorageProof{Column: -1, Path: []ports.Hash{{}, {}}})
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
		if p.Root != r || len(p.Path) != 2 || p.LeafBytes != por.SpotLeafBytes {
			t.Fatalf("paged proof mismatched: root %x vs %x, path %d, leaf width %d",
				p.Root, r, len(p.Path), p.LeafBytes)
		}
	}

	// The hot-proof cache never exceeded its budget: resident RAM is O(hot),
	// not O(N) — the wall.
	if _, _, used := pc.Stats(); used > budget {
		t.Fatalf("resident hot-proof RAM %d blew the O(hot) budget %d", used, budget)
	}
}
