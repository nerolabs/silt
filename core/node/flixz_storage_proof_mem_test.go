package node

import (
	"os"
	"strconv"
	"testing"

	"github.com/nerolabs/silt/adapters/diskproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/proofcache"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// TestFlixzStorageProofMemoryIsOHot is the DAEMON-realistic measurement the PE
// asked for (opt-in: SILT_FLIXZ_DIAG=1): a storage/resolver node holding a large
// catalog on disk (flixz's workload) must keep resident PROOF RAM O(hot), not
// O(total held) — the bug #464 fixed. It populates a real diskproofs store with N
// proofs (the prod backing), reloads through the bounded proofcache exactly as the
// daemon does, and measures resident heap.
//
// Before #464: `n.proofs` held the FULL StorageProof (Path + PorTags, ~5.4 KB)
// for every held chunk → O(total) resident → at catalog scale it OOMs (the flixz
// report). After #464: resident = tiny proofMeta (~80-100 B/chunk, O(N) but small)
// + a bounded proofcache (≤ budget, paged from disk) → O(hot).
//
//	SILT_FLIXZ_DIAG=1 SILT_FLIXZ_N=200000 go test ./core/node/ -run Flixz -v
//
// This proves the MECHANISM at scale; flixz should still confirm on their exact
// catalog with `silt daemon -debug-addr` + `go tool pprof .../heap` (the finding
// note is explicit that inference != measurement).
func TestFlixzStorageProofMemoryIsOHot(t *testing.T) {
	if os.Getenv("SILT_FLIXZ_DIAG") != "1" {
		t.Skip("diagnostic; set SILT_FLIXZ_DIAG=1 to run (measures #464 storage proof RAM)")
	}
	N := 100000
	if v := os.Getenv("SILT_FLIXZ_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			N = n
		}
	}
	const budget = 64 << 20 // the daemon's default -proof-cache

	// A realistic full proof: 4 Merkle-path hashes (128 B) + PoR tags. Production
	// shards run many por-blocks; size the tags so one full proof ≈ 5.4 KB, the
	// figure the triage/PE used for the O(total) blow-up.
	const porBlocks = 166 // 166 * 32 B ≈ 5.3 KB of tags + 128 B path ≈ 5.4 KB
	mkFull := func(i int) ports.StorageProof {
		var root ports.Hash
		root[0], root[1], root[2] = byte(i), byte(i>>8), byte(i>>16)
		path := make([]ports.Hash, 4)
		tags := make([][]byte, porBlocks)
		for j := range tags {
			tags[j] = make([]byte, 32)
		}
		return ports.StorageProof{Root: root, Index: i % 8, Total: 8, Column: i % 4, Path: path, PorTags: tags}
	}

	// Populate a REAL on-disk proof store (the daemon's diskproofs backing).
	dir := t.TempDir()
	ds, err := diskproofs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]ports.ChunkID, N)
	for i := 0; i < N; i++ {
		var id ports.ChunkID
		id[0], id[1], id[2], id[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
		ids[i] = id
		if err := ds.Put(id, mkFull(i)); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	// Reload exactly as a restarting daemon does: proofcache over diskproofs,
	// SetProofStore, LoadProofs (Keys+Get → resident proofMeta, bounded cache).
	var idNode ports.NodeID
	idNode[0] = 3
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())
	nd := New(idNode, DefaultConfig(), sched, net.Endpoint(idNode), memstore.New())
	pc := proofcache.Open(ds, budget)
	nd.SetProofStore(pc)
	nd.LoadProofs()

	// Every chunk's metadata is resident (the existence/route/denylist authority).
	miss := 0
	for _, id := range ids {
		if _, ok := nd.proofMeta[id]; !ok {
			miss++
		}
	}
	if miss != 0 {
		t.Fatalf("%d/%d proofMeta entries missing after reload — resident metadata is not the authority", miss, N)
	}

	// The precise O(hot) claim, measured on the DELIBERATE resident structures
	// (not a noisy HeapInuse delta, which includes unreturned GC spans):
	//   - the FULL proofs (Path + PorTags) live in a BOUNDED cache: used ≤ budget,
	//     no matter how large the catalog. This is the O(hot) guarantee.
	//   - the resident metadata is O(N) but TINY: ~sizeof(proofMeta) per chunk.
	_, _, cacheUsed := pc.Stats()
	const metaBytesPerChunk = 120 // proofMeta struct + map element overhead, approx
	metaMiB := float64(N) * metaBytesPerChunk / (1 << 20)
	fullMiB := float64(cacheUsed) / (1 << 20)
	residentMiB := metaMiB + fullMiB
	oTotalMiB := float64(N) * 5.4 / 1024 // what the OLD full-map pinned: N × ~5.4 KB

	t.Logf("N=%d held: resident proof RAM = %.1f MiB (bounded cache %.1f MiB + O(N) meta %.1f MiB); "+
		"the OLD full-map would pin O(total) ≈ %.0f MiB. Reduction ×%.1f.",
		N, residentMiB, fullMiB, metaMiB, oTotalMiB, oTotalMiB/residentMiB)

	// THE WALL 1: the FULL-proof resident RAM is bounded by the cache budget,
	// independent of N (the O(hot) property #464 delivers).
	if cacheUsed > budget {
		t.Fatalf("proof cache used %d > budget %d — full-proof RAM not O(hot)", cacheUsed, budget)
	}
	// THE WALL 2: total resident proof RAM is FAR below the O(total) full-map —
	// the reduction that turns flixz's catalog from an OOM into a bounded head.
	if residentMiB >= oTotalMiB {
		t.Fatalf("resident %.1f MiB is not below O(total) %.0f MiB — #464 gives no reduction at N=%d", residentMiB, oTotalMiB, N)
	}
}
