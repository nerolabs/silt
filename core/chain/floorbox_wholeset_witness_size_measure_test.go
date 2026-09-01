package chain

// MEASUREMENT TEST — era-4 (v5) floor-box whole-set BONDED witness size
//
// Gate-4 unfiled-measurement scar closure (2026-08-31).
//
// The "~106MB" estimate for the whole-bonded witness at 1M members was cited
// once but never filed, so it cannot ground the R-membership / pony-2GB budget.
// This test produces the durable filed numbers.
//
// WHAT IS MEASURED. A floor-box processing a block that touches the bondedRoot
// whole-set digest (a bond-reg or slash block) must receive a
// StateRootDigestWitness for `bonded`. That witness has two parts:
//
//   PreIDs  []ports.NodeID   — the complete pre-state member list: N × 32 bytes.
//   Proof   statehash.Witness — one SMT inclusion proof for the digest leaf:
//                              len(SideNodes) × 32 bytes + small fixed overhead.
//
// The box builds the whole-set SMT (over the bonded keyspace leaves only: N
// bonded||id leaves + one bondedRoot digest leaf) and proves the bondedRoot
// digest leaf against its root. It does NOT build the full state root (that
// would require all 23 keyspaces); only the bonded sub-tree is instrumented
// here, which is what the cost doc discusses.
//
// WHY ONLY BONDED. The O(registry) cost is symmetric across bonded/qualified/
// slashed (same MTH structure, same SMT proof depth over the same sub-tree). The
// filed numbers use bonded as the representative case; the qualified and slashed
// witnesses are bounded by the same formula (they are subsets of bonded).
//
// REPRODUCE COMMAND (from repo root, worktree or main checkout):
//
//	go test ./core/chain/ -run TestMeasureFloorBoxWholeSetWitnessSize -v -count=1 -timeout=600s
//
// N=1M is the load-bearing data point for the pony-2GB budget verdict.
// Use -short to skip this test during routine CI (N=1M takes ~3s; the structural
// check at N=100 always runs).

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
)

// wholeSetWitnessRow is one row of the measurement table (one N value).
type wholeSetWitnessRow struct {
	N               int
	idListBytes     int     // N × 32 (flat byte count for PreIDs)
	sideNodes       int     // number of SMT proof sidenodes
	proofBytes      int     // marshaled proof size (gob)
	totalBytes      int     // idListBytes + proofBytes
	cumulAllocMB    float64 // cumulative bytes allocated (TotalAlloc delta) during SMT build
	liveHeapDeltaMB float64 // HeapInuse delta after GC (live heap contribution)
}

// TestMeasureFloorBoxWholeSetWitnessSize measures the marshaled byte size of the
// StateRootDigestWitness a floor box needs to verify the bonded whole-set digest
// for one touched bondedRoot leaf. It runs at N = 10k, 100k, and 1M.
//
// For each N it reports:
//   - idListBytes: N × 32 (PreIDs flat bytes, the dominating term)
//   - proofBytes: marshaled gob bytes for the SMT inclusion proof (O(log N × 32))
//   - totalBytes: idListBytes + proofBytes (the wire transfer size)
//   - cumulAllocMB: cumulative heap bytes allocated during SMT build (includes
//     GC-collected intermediates — this is NOT the peak live heap)
//   - liveHeapDeltaMB: HeapInuse delta after GC (approximates live heap growth)
func TestMeasureFloorBoxWholeSetWitnessSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-N witness measurement in short mode; use -run TestMeasureWholeSetWitnessProofStructure for the fast structural check")
	}

	// N=1M is the key budget-verdict point.
	// N=10k and 100k provide the extrapolation anchor.
	sizes := []int{10_000, 100_000, 1_000_000}

	rows := make([]wholeSetWitnessRow, 0, len(sizes))
	for _, N := range sizes {
		r := measureWholeSetWitnessBonded(t, N)
		rows = append(rows, r)
	}

	// Print the table.
	t.Logf("Measurement: wire size of StateRootDigestWitness.bonded (PreIDs + SMT proof)")
	t.Logf("┌──────────┬──────────────┬───────────┬────────────┬──────────────────┬─────────────────┐")
	t.Logf("│ N        │ PreIDs (KB)  │ Proof (B) │ Total (MB) │ cumulAlloc (MB)  │ liveHeap (MB)   │")
	t.Logf("├──────────┼──────────────┼───────────┼────────────┼──────────────────┼─────────────────┤")
	for _, r := range rows {
		t.Logf("│ %8d │ %12.1f │ %9d │ %10.3f │ %16.1f │ %15.1f │",
			r.N,
			float64(r.idListBytes)/1024,
			r.proofBytes,
			float64(r.totalBytes)/(1024*1024),
			r.cumulAllocMB,
			r.liveHeapDeltaMB,
		)
	}
	t.Logf("└──────────┴──────────────┴───────────┴────────────┴──────────────────┴─────────────────┘")
	t.Logf("cumulAlloc = TotalAlloc delta (includes GC-collected intermediates; not peak RSS)")
	t.Logf("liveHeap   = HeapInuse delta after GC (approximates retained heap growth)")

	// Print sidenode depths (extrapolation audit — confirms log growth).
	for _, r := range rows {
		t.Logf("N=%d: sideNodes=%d (SHA-256 sparse trie depth at N leaves; log₂(%d)≈%.0f)",
			r.N, r.sideNodes, r.N, log2fApprox(r.N))
	}

	// Budget verdict: total wire bytes at 1M must be < 2 GB (pony budget).
	const ponyBudget2GB = 2 * 1024 * 1024 * 1024
	for _, r := range rows {
		if r.N == 1_000_000 {
			totalMB := float64(r.totalBytes) / (1024 * 1024)
			verdict := "FITS"
			if r.totalBytes >= ponyBudget2GB {
				verdict = "EXCEEDS"
			}
			t.Logf("PONY-2GB BUDGET VERDICT (N=1M): %.1f MB wire size %s the 2048 MB budget (headroom=%.0f MB)",
				totalMB, verdict, float64(ponyBudget2GB)/(1024*1024)-totalMB)
			t.Logf("NOTE: the SMT build for N=1M costs %.0f MB cumulative heap (%.0f MB live); this is the",
				r.cumulAllocMB, r.liveHeapDeltaMB)
			t.Logf("      provider-side cost (the prover that HOLDS the committed set), not the box-side cost.")
			t.Logf("      A coexistence test vs a ~1GB flixz-sized daemon is owed (see MEMORY.md session-7 note).")
		}
	}
}

// measureWholeSetWitnessBonded builds an N-member bonded sub-tree SMT
// (bonded||id leaves + the bondedRoot digest leaf), proves the bondedRoot
// leaf, and returns the wire byte counts and heap measurements.
func measureWholeSetWitnessBonded(t *testing.T, N int) wholeSetWitnessRow {
	t.Helper()
	t.Logf("measuring N=%d ...", N)

	// -- Generate N synthetic NodeIDs (deterministic, 32 bytes each). --
	// Each is distinct: 8-byte big-endian index prefix, remaining 24 bytes zero.
	ids := make([]ports.NodeID, N)
	for i := 0; i < N; i++ {
		var b8 [8]byte
		binary.BigEndian.PutUint64(b8[:], uint64(i+1)) // 1-indexed to avoid all-zero ID
		var id ports.NodeID
		copy(id[:], b8[:])
		ids[i] = id
	}

	// -- Build the bonded sub-tree leaf set. --
	// One bonded||id leaf per member (value = EncodeInt64(1<<20) — 1 MiB bond).
	// One bondedRoot digest leaf (value = nodeSetMTH(ids)).
	leaves := make([]statehash.Leaf, 0, N+1)
	for _, id := range ids {
		leaves = append(leaves, statehash.Leaf{
			Key:   statehash.Key(tagBonded, id[:]),
			Value: statehash.EncodeInt64(1 << 20),
		})
	}
	bondedMTH := nodeSetMTH(ids)
	bondedRootKey := statehash.Key(tagBondedRoot, nil)
	leaves = append(leaves, statehash.Leaf{
		Key:   bondedRootKey,
		Value: bondedMTH,
	})

	// -- Measure heap before SMT build. --
	var ms0, ms1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms0)

	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("N=%d: NewProver: %v", N, err)
	}
	// Prove the bondedRoot digest leaf (membership proof).
	wit, err := prover.Prove(bondedRootKey)
	if err != nil {
		t.Fatalf("N=%d: Prove(bondedRootKey): %v", N, err)
	}

	runtime.GC()
	runtime.ReadMemStats(&ms1)
	cumulAllocMB := float64(ms1.TotalAlloc-ms0.TotalAlloc) / (1024 * 1024)
	// HeapInuse delta (live heap retained after GC):
	liveHeapDeltaMB := float64(int64(ms1.HeapInuse)-int64(ms0.HeapInuse)) / (1024 * 1024)

	// -- Count sidenode depth. --
	sideNodes := wit.SideNodeCount()

	// -- Verify the proof round-trips (ablation: proves the proof is real). --
	result := statehash.Resolve(prover.Root(), bondedRootKey, bondedMTH, wit)
	if !result.IsProvenPresent() {
		t.Fatalf("N=%d: Resolve not ProvenPresent for bondedRootKey (Outcome=%v)", N, result.Outcome())
	}

	// -- Marshal a proxy proof for gob wire size. --
	// The Witness type wraps *smt.SparseMerkleProof privately; we build a
	// structurally-equivalent proxy proof for gob marshal sizing.
	// A membership proof: SideNodes has `sideNodes` entries (each 32 bytes),
	// NonMembershipLeafData is nil, SiblingData is a 32-byte preimage.
	proxyProof := &smt.SparseMerkleProof{
		SideNodes: make([][]byte, sideNodes),
	}
	for i := range proxyProof.SideNodes {
		sn := make([]byte, 32)
		sn[0] = byte((i + 1) & 0xFF)
		proxyProof.SideNodes[i] = sn
	}
	proxyProof.SiblingData = make([]byte, 32)
	proxyProof.SiblingData[0] = 0xAB
	marshaledProof, err := proxyProof.Marshal()
	if err != nil {
		t.Fatalf("N=%d: Marshal proxy proof: %v", N, err)
	}

	idListBytes := N * 32 // PreIDs: N × 32 bytes (NodeID = Hash = [sha256.Size]byte = [32]byte)
	proofBytes := len(marshaledProof)
	totalBytes := idListBytes + proofBytes

	t.Logf("N=%d done: idList=%d B (%.2f MB), proof=%d B (sideNodes=%d), total=%.3f MB | cumulAlloc=%.1f MB, liveHeap=%.1f MB",
		N,
		idListBytes, float64(idListBytes)/(1024*1024),
		proofBytes, sideNodes,
		float64(totalBytes)/(1024*1024),
		cumulAllocMB, liveHeapDeltaMB,
	)

	return wholeSetWitnessRow{
		N:               N,
		idListBytes:     idListBytes,
		sideNodes:       sideNodes,
		proofBytes:      proofBytes,
		totalBytes:      totalBytes,
		cumulAllocMB:    cumulAllocMB,
		liveHeapDeltaMB: liveHeapDeltaMB,
	}
}

// log2fApprox returns an approximation of log₂(n) by counting right-shifts to 1.
func log2fApprox(n int) float64 {
	if n <= 1 {
		return 0
	}
	r := 0.0
	for n > 1 {
		n >>= 1
		r++
	}
	return r
}

// TestMeasureWholeSetWitnessProofStructure is a FAST structural check (N=100,
// no -short guard) that the measurement method is faithful:
//   - the real proof issued by statehash.NewProver has a nonzero sidenode count,
//   - Resolve verifies it (the proof is a real membership proof, not a dummy), and
//   - the proxy marshal returns a finite nonzero byte count.
//
// This always runs in CI so the regression coverage is always-on.
func TestMeasureWholeSetWitnessProofStructure(t *testing.T) {
	const N = 100
	ids := make([]ports.NodeID, N)
	for i := 0; i < N; i++ {
		var b8 [8]byte
		binary.BigEndian.PutUint64(b8[:], uint64(i+1))
		var id ports.NodeID
		copy(id[:], b8[:])
		ids[i] = id
	}
	leaves := make([]statehash.Leaf, 0, N+1)
	for _, id := range ids {
		leaves = append(leaves, statehash.Leaf{
			Key:   statehash.Key(tagBonded, id[:]),
			Value: statehash.EncodeInt64(1 << 20),
		})
	}
	bondedMTH := nodeSetMTH(ids)
	bondedRootKey := statehash.Key(tagBondedRoot, nil)
	leaves = append(leaves, statehash.Leaf{
		Key:   bondedRootKey,
		Value: bondedMTH,
	})

	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	wit, err := prover.Prove(bondedRootKey)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	result := statehash.Resolve(prover.Root(), bondedRootKey, bondedMTH, wit)
	if !result.IsProvenPresent() {
		t.Fatalf("Resolve not ProvenPresent: Outcome=%v", result.Outcome())
	}
	sn := wit.SideNodeCount()
	if sn == 0 {
		t.Errorf("sideNodeCount=0 for N=%d: proof has no depth — measurement would undercount", N)
	}
	t.Logf("N=%d: sideNodeCount=%d (SHA-256 sparse trie depth), idListBytes=%d B, proofSideBytes=%d B",
		N, sn, N*32, sn*32)

	// Proxy-marshal to confirm the marshal path works.
	proxyProof := &smt.SparseMerkleProof{
		SideNodes: make([][]byte, sn),
	}
	for i := range proxyProof.SideNodes {
		sn32 := make([]byte, 32)
		sn32[0] = byte((i + 1) & 0xFF)
		proxyProof.SideNodes[i] = sn32
	}
	proxyProof.SiblingData = make([]byte, 32)
	proxyProof.SiblingData[0] = 0xAB
	marshaled, err := proxyProof.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(marshaled) == 0 {
		t.Errorf("marshal produced 0 bytes — the proxy proof is empty")
	}
	t.Logf("N=%d: proxy proof marshaled=%d B, total wire (idList+proof)=%d B",
		N, len(marshaled), N*32+len(marshaled))
}
