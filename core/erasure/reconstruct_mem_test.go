package erasure

// §0.1 — the repair-path memory footprint at PRODUCTION chunk size (research cert
// 2026-08-19, RULING-repair-payee-fork §0.1 gate). Reconstructing one lost shard
// materializes the whole stripe in RAM: the node repair path holds ~k survivor
// shards (each = one chunk) plus the rebuilt slots as [][]byte
// (ReconstructStripe, erasure.go). At the 64 KiB SIM chunk size that is ~640 KiB
// and invisible; at the 64 MiB PRODUCTION minimum it is ~640 MiB–1 GiB — on a node
// whose entire budget is ~2 GB (build-immutable #8). This is measured LOCALLY,
// for $0, because it is a single-node property: build-immutables #6/#7 say
// reproduce locally before spending a cloud run, and a cloud run cannot see this
// anyway (the harness publishes at 64 KiB, hiding the spike ~1000×). If the
// production footprint doesn't fit the floor box, the mitigation is streaming /
// column-wise decode or a smaller hot-path chunk — a mechanism change, not a
// tuning knob. Plan: docs/thinking/2026-08-19-cloudtest-harness-improvement-plan.md.

import (
	"math/rand"
	"runtime"
	"testing"
)

// reconstructFootprint measures the repair reconstruction memory at the given
// shard (chunk) size two ways: (1) `resident` — the DETERMINISTIC bytes the stripe
// slice holds through the reconstruct (GC-independent, the honest resident
// footprint), and (2) `heapDelta` — the process HeapAlloc growth as a cross-check
// (GC-noisy, informational). DefaultParams (k=10, n=16), 2 lost — the modal
// loss-driven repair. The node repair path holds exactly this: an n-slot stripe
// with survivors fetched + the lost shards rebuilt, all resident at once
// (ReconstructStripe fills the missing slots in place).
func reconstructFootprint(t *testing.T, shardSize int) (resident, heapDelta int64) {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	p := DefaultParams
	data := makeStripe(rng, p.K, shardSize)
	parity, err := EncodeStripe(p, data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	runtime.GC()
	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)

	// Model the repair path: an n-slot stripe with survivors fetched (own copies,
	// as store.Get returns), 2 data shards lost, then reconstruct in place.
	shards := make([][]byte, p.N)
	for j, d := range data {
		shards[j] = append([]byte(nil), d...)
	}
	for j, par := range parity {
		shards[p.K+j] = append([]byte(nil), par...)
	}
	shards[3], shards[7] = nil, nil // two lost data shards

	if err := ReconstructStripe(p, shards, p.K); err != nil {
		t.Fatalf("reconstruct: %v", err)
	}

	// Deterministic resident: the actual bytes held by every slot after reconstruct
	// (all n filled) — this is what pins RAM, independent of GC timing.
	for _, s := range shards {
		resident += int64(len(s))
	}
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	heapDelta = int64(m1.HeapAlloc) - int64(m0.HeapAlloc)
	runtime.KeepAlive(shards)
	return resident, heapDelta
}

// TestReconstructMemoryFootprint_SimVsProd measures and RECORDS the §0.1 number at
// both the sim (64 KiB) and production-minimum (64 MiB) chunk sizes, so the ~1000×
// gap the cloud harness hides is a committed, citable measurement. It asserts the
// resident footprint holds at least k survivors' worth (a sanity floor); the point
// is the logged number, read from the test output.
func TestReconstructMemoryFootprint_SimVsProd(t *testing.T) {
	if testing.Short() {
		t.Skip("§0.1 footprint test allocates ~1 GiB at prod chunk size; explicit-run only")
	}
	const MiB = 1 << 20
	for _, shardSize := range []int{64 << 10, 64 << 20} { // 64 KiB sim, 64 MiB prod-min
		resident, heapDelta := reconstructFootprint(t, shardSize)
		floor := int64(DefaultParams.K) * int64(shardSize) // ≥ k survivors resident
		t.Logf("§0.1 shard=%dKiB (k=%d,n=%d): RESIDENT stripe=%.1fMiB (heapDelta≈%.1fMiB, GC-noisy); a 2GB box has %.0fMiB left after this",
			shardSize>>10, DefaultParams.K, DefaultParams.N,
			float64(resident)/MiB, float64(heapDelta)/MiB, 2048-float64(resident)/MiB)
		if resident < floor {
			t.Fatalf("shard=%dKiB: resident %d < floor %d — reconstruction must hold at least k survivors", shardSize>>10, resident, floor)
		}
	}
}

// BenchmarkReconstructStripe_ProdChunk reports B/op — the allocation footprint of
// one production-chunk-size reconstruction — for the §0.1 record. Run with:
//
//	go test ./core/erasure -run x -bench ReconstructStripe_ProdChunk -benchmem -benchtime 3x
func BenchmarkReconstructStripe_ProdChunk(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	p := DefaultParams
	const prodShard = 64 << 20 // 64 MiB production minimum
	data := makeStripe(rng, p.K, prodShard)
	parity, err := EncodeStripe(p, data)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(p.K) * prodShard)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shards := make([][]byte, p.N)
		for j, d := range data {
			shards[j] = append([]byte(nil), d...)
		}
		for j, par := range parity {
			shards[p.K+j] = append([]byte(nil), par...)
		}
		shards[3], shards[7] = nil, nil
		if err := ReconstructStripe(p, shards, p.K); err != nil {
			b.Fatal(err)
		}
	}
}
