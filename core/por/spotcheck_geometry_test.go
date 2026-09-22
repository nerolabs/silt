package por

import (
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/manifest"
)

// THE GEOMETRY OF THE HASH-ONLY SPOT CHECK: which leaf size, and how many samples.
//
// WHY THIS EXISTS. production_cost_floor_test.go answered WHICH scheme (option B,
// hash-only spot-checking) by pricing PRODUCTION on the floor box, which is what
// build-immutable #8 asks of a mechanism choice. It did not answer the scheme's two
// free parameters, and those are not free in the way a tuning knob is free: the leaf
// size sets BOTH the production cost and the audit's wire cost, and it sets them in
// OPPOSITE directions. A smaller leaf makes a sample cheaper to send and a shard
// dearer to commit, because the tree over it has twice as many nodes to hash.
//
// So there are three quantities and they do not all improve together:
//
//	PRODUCTION  hashing the shard once, plus ~2*(S/L) node hashes for the tree.
//	            Rises as L falls. It is what #8 measures and what picked option B.
//	AUDIT BYTES per sample, L + 32*ceil(log2(S/L)). The Merkle path dominates below
//	            ~512 B, so shrinking L past that buys almost nothing and keeps
//	            costing production.
//	DETECTION   1-(1-f)^l against a holder that dropped fraction f. It is bought
//	            with the sample count l, at AUDIT BYTES each.
//
// THE BASELINE EVERY ROW IS READ AGAINST is the scheme shipping today: a
// Shacham-Waters aggregate response is 128 field elements plus the aggregate tag,
// so its audit costs a FIXED 4128 B per shard no matter how large the shard is.
// That is the number the hash-only scheme has to live beside, because an audit that
// costs more wire than the one it replaces has moved the cost rather than removed
// it. Production is where it wins; wire is where it must not lose.

// spotShardBytes is the same shipped shard the production-cost measurement used: a
// 256 KiB frame plus the AES-256-GCM overhead the pipeline adds.
const spotShardBytes = shardBytes

// spotProduceIters is how many iterations each timed batch runs. It is sized so a
// batch lasts tens of milliseconds rather than a few, because the quantity under
// test — committing one shard by hashing — takes under a millisecond, and a batch
// that short is decided by whether the scheduler happened to interrupt it. Timing a
// sub-millisecond operation over a handful of repetitions measures the machine's
// mood, not the operation: the same unchanged code reported production ratios from
// 4.0x to 8.8x that way, a band wide enough to cross the 4x bar this file asserts.
// Longer batches plus timeN's floor over several of them put the spread well inside
// the margin.
const spotProduceIters = 100

// swAuditBytes is the shipped scheme's per-shard audit response: SectorsPerBlock mu
// elements plus one sigma, each ElemBytes wide. Independent of the shard size,
// which is the property the hash-only scheme gives up.
var swAuditBytes = (DefaultParams.SectorsPerBlock + 1) * ElemBytes

func pathLenFor(leaves int) int {
	n := 0
	for k := leaves; k > 1; k = (k + 1) / 2 {
		n++
	}
	return n
}

// TestSpotCheckGeometryIsReported prints the three-way trade and asserts the two
// properties the shipped constants have to have.
func TestSpotCheckGeometryIsReported(t *testing.T) {
	// NO -short SKIP, for the reason the production-cost measurement gives: a skipped
	// measurement reads as a pass, and this one costs tens of milliseconds.
	data := make([]byte, spotShardBytes)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	swProduce := timeN(t, spotProduceIters, func() {
		k, err := DeriveKey([]byte("geometry"), DefaultParams)
		if err != nil {
			t.Fatal(err)
		}
		k.Tags([]byte("s"), data)
	})

	t.Logf("SPOT-CHECK GEOMETRY over a %d-byte shard (single-threaded; a developer core, so a LOWER bound for 1 vCPU)", spotShardBytes)
	t.Logf("  BASELINE shipped Shacham-Waters: produce %v, audit %d B/shard (size-independent)", swProduce, swAuditBytes)
	t.Logf("  leaf     leaves  path  sample B   produce   l=5 audit B   l=8 audit B   l@50%%/99.9%%")

	type row struct {
		leaf, sample int
		produce      time.Duration
	}
	var rows []row
	for _, leaf := range []int{64, 128, 256, 512, 1024, 2048, 3968} {
		leaves := (spotShardBytes + leaf - 1) / leaf
		path := pathLenFor(leaves)
		sample := leaf + path*32
		produce := timeN(t, spotProduceIters, func() { hashOnlyRoot(data, leaf) })
		l := samplesFor(0.5, 0.999)
		t.Logf("  %5d   %6d  %4d  %8d   %7v   %11d   %11d   %6d",
			leaf, leaves, path, sample, produce, 5*sample, 8*sample, l*sample)
		rows = append(rows, row{leaf: leaf, sample: sample, produce: produce})
	}

	// THE FIRST ASSERTION: at the shipped leaf size, producing the commitment must
	// stay materially cheaper than the scheme it replaces. That cheapness is the
	// whole of #8's case for option B, and a leaf small enough to erase it would
	// have re-opened the choice without saying so.
	shipped := row{}
	for _, r := range rows {
		if r.leaf == SpotLeafBytes {
			shipped = r
		}
	}
	if shipped.leaf == 0 {
		t.Fatalf("SpotLeafBytes = %d is not in the measured table above, so the shipped constant is not the one "+
			"this measurement priced", SpotLeafBytes)
	}
	if shipped.produce*4 > swProduce {
		t.Fatalf("producing the hash-only commitment at leaf=%d costs %v against the shipped scheme's %v — less than 4x "+
			"cheaper. The #8 argument that picked option B was a production-cost argument; at this ratio it no longer "+
			"holds and the leaf size must come back down.", shipped.leaf, shipped.produce, swProduce)
	}

	// THE SECOND ASSERTION: a full sweep at the shipped sample count must not cost
	// more wire than the scheme it replaces. Option B trades a size-independent
	// proof for a sampled one; if the sample also costs more bytes, the trade is a
	// pure loss on the wire and the leaf/sample pair is wrong.
	audit := SpotSampleCount * shipped.sample
	if audit > swAuditBytes {
		t.Fatalf("one spot-check audit costs %d B (%d samples x %d B) against the shipped aggregate's %d B. The hash-only "+
			"scheme would be MOVING the cost from production to wire rather than removing it; lower SpotSampleCount or "+
			"the leaf size until it fits.", audit, SpotSampleCount, shipped.sample, swAuditBytes)
	}
	t.Logf("  SHIPPED leaf=%d, samples=%d -> %d B/shard against the baseline's %d B; production %v against %v",
		SpotLeafBytes, SpotSampleCount, audit, swAuditBytes, shipped.produce, swProduce)
	t.Logf("  detection at %d samples: dropped 100%% -> 100.00%%, 50%% -> %.2f%%, 25%% -> %.2f%%, 10%% -> %.2f%%",
		SpotSampleCount, 100*detect(0.5, SpotSampleCount), 100*detect(0.25, SpotSampleCount), 100*detect(0.10, SpotSampleCount))
}

// TestProverAnswerCostIsReported prices what answering ONE challenge costs the
// prover, which is a different number from producing the commitment and is the one a
// denial-of-service budget is sized from: MsgChallenge carries no signature and no
// standing requirement, so an unbounded challenger buys this much of another node's
// single serialized loop for free (core/node bondaudit.go porChallengeBurst).
//
// THE PREPARED TREE IS THE WHOLE DIFFERENCE, and it is why this is measured rather
// than assumed. manifest.Prove recomputes subtree hashes over half the leaves on
// every call, so drawing SpotSampleCount proofs the naive way is SpotSampleCount
// passes over the shard. manifest.BuildTree is O(n) once and O(log n) per proof.
// Both are measured here so the gap is a number and not a claim.
func TestProverAnswerCostIsReported(t *testing.T) {
	data := make([]byte, spotShardBytes)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	seed := [32]byte{0x31}
	leaves := SpotLeaves(len(data), SpotLeafBytes)

	prepared := timeN(t, 20, func() {
		if _, err := Open(data, SpotLeafBytes, seed, SpotSampleCount); err != nil {
			t.Fatal(err)
		}
	})
	naive := timeN(t, 5, func() {
		lh := LeafHashes(data, SpotLeafBytes)
		for _, i := range SpotIndices(seed, leaves, SpotSampleCount) {
			if _, err := manifest.Prove(lh, i); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Logf("PROVER COST for one challenge over a %d-byte shard (%d leaves, %d samples):", spotShardBytes, leaves, SpotSampleCount)
	t.Logf("  shipped (one prepared tree): %v", prepared)
	t.Logf("  naive (a standalone proof per sample): %v  -> %.1fx", naive, float64(naive)/float64(prepared))
	t.Logf("  the aggregate scheme this replaced was measured at 8.3 ms per answer, so the shipped path is ~%.1fx cheaper",
		8.3e6/float64(prepared.Nanoseconds()))

	// THE ASSERTION: the prepared tree must actually be the cheaper path. If a
	// refactor ever routes Open through the standalone proof, one challenge becomes
	// SpotSampleCount passes over the shard and the per-challenger budget that was
	// sized from this number is silently wrong by that factor.
	if prepared >= naive {
		t.Fatalf("opening a challenge costs %v with a prepared tree and %v without one — the tree is buying nothing, "+
			"so either Open stopped using manifest.BuildTree or the tree stopped caching. The DoS budget in "+
			"core/node bondaudit.go is sized from the prepared figure.", prepared, naive)
	}
}

// detect is 1-(1-f)^l: the probability l independent samples hit at least one of the
// leaves a holder dropped a fraction f of.
func detect(f float64, l int) float64 {
	miss := 1.0
	for i := 0; i < l; i++ {
		miss *= 1 - f
	}
	return 1 - miss
}

// hashOnlyRoot is the production cost being priced: leaf hashes over the shard, then
// a binary fold. It mirrors the shipped ShardRoot at an arbitrary leaf size so the
// table can sweep one, and is kept here rather than exported so the production path
// has exactly one geometry.
func hashOnlyRoot(data []byte, leaf int) [32]byte {
	var level [][32]byte
	for off := 0; off < len(data); off += leaf {
		end := off + leaf
		if end > len(data) {
			end = len(data)
		}
		level = append(level, sha256.Sum256(data[off:end]))
	}
	for len(level) > 1 {
		next := make([][32]byte, 0, (len(level)+1)/2)
		for j := 0; j < len(level); j += 2 {
			if j+1 == len(level) {
				next = append(next, level[j])
				continue
			}
			var pair [64]byte
			copy(pair[:32], level[j][:])
			copy(pair[32:], level[j+1][:])
			next = append(next, sha256.Sum256(pair[:]))
		}
		level = next
	}
	return level[0]
}
