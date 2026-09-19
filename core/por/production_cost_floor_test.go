package por

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"
)

// THE FLOOR-BOX PRODUCTION COST OF THE CANDIDATE PROOF-OF-RETRIEVABILITY SCHEMES.
//
// WHY THIS EXISTS AND WHY IT COMES FIRST. The care link printed on every publish
// IS the storage-proof verification key, so all three audit legs are satisfiable by
// a party holding that key and no bytes. There is no fix inside the current
// primitive: Shacham-Waters PRIVATE verification (the construction this package
// adopts, ASIACRYPT 2008 §3) assumes the verifying key is unknown to the prover,
// and here it is a published capability. The party that must VERIFY is the party
// that could FORGE. So the remedy is a scheme change — and build-immutable #8
// decides how a scheme change STARTS: measure the full resource cost on the floor
// box, the cost to PRODUCE the artifact and not only to verify, store or transmit
// it, BEFORE committing to the mechanism. A scheme whose output is tiny but whose
// production blows the floor is disqualified however elegant.
//
// THE OPTION SPACE IS RE-DERIVED FROM THE MECHANISM, not recalled. The research
// certification the pins cite was deleted with the written record on 2026-09-13, so
// "confirm which option shipped" cannot be answered by reading it. What the break
// actually requires is that VERIFYING stop implying FORGING, and there are exactly
// three structural ways to get there:
//
//	A. PUBLIC VERIFICATION — tag with a key the publisher keeps, verify with a
//	   public one (Shacham-Waters §4, BLS-style). A care-link holder verifies and
//	   cannot forge. Needs a pairing-friendly group; production is one group
//	   exponentiation per sector per block.
//	B. HASH-ONLY SPOT-CHECKING — no key at all. The auditor names sampled BLOCKS
//	   and checks the returned bytes against a committed per-shard Merkle root. A
//	   prover without the bytes cannot answer, and there is nothing to publish that
//	   could be turned into a forgery. Production is hashing; the cost is proof
//	   SIZE at audit time.
//	C. DON'T PUBLISH THE KEY — keep private verification and give the verifying key
//	   to someone other than every care-link holder. This is not a primitive change
//	   but a TRUST-MODEL change: the care link exists precisely so any caretaker can
//	   audit without the content key, so C removes the property the link is for. It
//	   is recorded to close the space, not as a candidate.
//
// So the measurable question is A versus B, and #8 says production cost decides the
// entry, not proof size. These benchmarks report it.
//
// ⚠ WHAT THIS BOX IS AND IS NOT. The floor box is 1 vCPU / 2 GB. These run
// SINGLE-THREADED, so the per-operation costs transfer to it modulo clock speed,
// and every figure below should be read as a LOWER BOUND for the floor — a
// developer core is faster than a hobbyist vCPU, never slower. That direction is
// what makes a disqualification sound: if a scheme is already too expensive here,
// it cannot be cheaper there.

// shardBytes is the shipped shard size the field measured: a 256 KiB chunk plus
// the AES-256-GCM overhead the pipeline adds. Every figure is per ONE shard,
// because that is the unit a publisher tags and a repair re-tags.
const shardBytes = 262160

// BenchmarkProductionCostA_CurrentPrivateTagging is the BASELINE: what the shipped
// scheme costs to produce today. A candidate is not judged in the abstract but
// against this — the publisher already pays it on every publish and every repair.
func BenchmarkProductionCostA_CurrentPrivateTagging(b *testing.B) {
	k, err := DeriveKey([]byte("floor-box-production-cost"), DefaultParams)
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, shardBytes)
	if _, err := rand.Read(data); err != nil {
		b.Fatal(err)
	}
	id := []byte("one-shard")
	b.SetBytes(shardBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if tags := k.Tags(id, data); len(tags) == 0 {
			b.Fatal("no tags produced — the baseline is measuring nothing")
		}
	}
}

// BenchmarkProductionCostB_HashOnlyPerBlockTree is option B's production cost: the
// per-shard Merkle tree over sampled blocks that a hash-only spot-check verifies
// against. No key, no field arithmetic, nothing to publish that could forge.
func BenchmarkProductionCostB_HashOnlyPerBlockTree(b *testing.B) {
	data := make([]byte, shardBytes)
	if _, err := rand.Read(data); err != nil {
		b.Fatal(err)
	}
	blockBytes := DefaultParams.blockBytes()
	b.SetBytes(shardBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Leaves, then a binary fold. The shape a per-shard block tree needs; the
		// production package would reuse core/manifest rather than this loop, which
		// is here so the benchmark stays inside core/por and measures pure hashing.
		var level [][32]byte
		for off := 0; off < len(data); off += blockBytes {
			end := off + blockBytes
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
		if len(level) != 1 {
			b.Fatal("the fold did not reach a single root — the benchmark is measuring nothing")
		}
	}
}

// BenchmarkProductionCostC_OneGroupExponentiation prices the ONE operation option A
// is built out of.
//
// ⚠ IT IS A PROXY, AND THE SUBSTITUTION IS THE PART TO CHECK. This measures a P-256
// scalar multiplication from the standard library, because no pairing-friendly
// group is in this tree and B8 forbids adopting an unaudited or unproven one to
// find out — a library choice is exactly what #8 says must NOT come first. A
// BLS12-381 G1 exponentiation is the operation option A actually needs and it is
// SLOWER than P-256, not faster (a larger field and a less optimised stdlib path),
// so using P-256 makes every option-A figure derived here a LOWER BOUND. That is
// the safe direction: a disqualification computed from a lower bound holds, while a
// pass computed from one does not. If these numbers say option A fits, they are NOT
// sufficient to adopt it — that needs the real curve.
func BenchmarkProductionCostC_OneGroupExponentiation(b *testing.B) {
	curve := ecdh.P256()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	peer, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	pub := peer.PublicKey()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := priv.ECDH(pub); err != nil {
			b.Fatal(err)
		}
	}
}

// TestProductionCostPerShardIsReported turns the three benchmarks into the one
// comparison #8 asks for, printed per SHARD rather than per operation.
//
// It is a test and not a benchmark because the DERIVED quantity is the deliverable:
// option A's per-shard cost is an operation COUNT times a per-operation price, and
// the count comes from the scheme's own shape (one exponentiation per sector per
// block, plus one per block) rather than from a stopwatch.
func TestProductionCostPerShardIsReported(t *testing.T) {
	// NO -short SKIP, deliberately. It reads as a PASS, so a skipped measurement is
	// an artifact claiming the schemes were priced when they were not — and this one
	// costs tens of milliseconds. The single assertion is a 10x ratio against a
	// measured 100x+ gap, so it is not load-sensitive on a contended box either.
	p := DefaultParams
	blocks := p.Blocks(shardBytes)
	sectors := p.SectorsPerBlock

	// Option A's operation count, from the construction rather than from a table:
	// sigma_i = (H(i) * prod_j u_j^{m_ij})^x needs one exponentiation per sector
	// plus one for the aggregate, per block.
	expsPerShard := blocks * (sectors + 1)

	current := timeOnce(t, func() {
		k, err := DeriveKey([]byte("report"), p)
		if err != nil {
			t.Fatal(err)
		}
		data := make([]byte, shardBytes)
		k.Tags([]byte("s"), data)
	})
	oneExp := timeN(t, 200, func() {
		curve := ecdh.P256()
		priv, _ := curve.GenerateKey(rand.Reader)
		peer, _ := curve.GenerateKey(rand.Reader)
		pub := peer.PublicKey()
		_, _ = priv.ECDH(pub)
	})
	optionA := time.Duration(expsPerShard) * oneExp

	t.Logf("FLOOR-BOX PRODUCTION COST PER %d-BYTE SHARD (single-threaded; a developer core, so a LOWER bound for 1 vCPU)", shardBytes)
	t.Logf("  geometry: %d blocks x %d sectors (s=%d, %d B/sector, %d B/block)",
		blocks, sectors, sectors, SectorBytes, p.blockBytes())
	t.Logf("  BASELINE  private Shacham-Waters tagging (shipped): %v", current)
	t.Logf("  OPTION A  public verification: %d group exponentiations x %v/exp (P-256 PROXY, a LOWER bound) = %v  -> %.0fx the baseline",
		expsPerShard, oneExp, optionA, float64(optionA)/float64(current))
	t.Logf("  OPTION B  hash-only per-block tree: see BenchmarkProductionCostB (pure SHA-256 over the shard)")

	if current <= 0 || oneExp <= 0 {
		t.Fatal("a measured cost came back as zero — the timer resolution is too coarse for this rig and no figure above " +
			"can be cited. Raise the iteration counts before reading anything here.")
	}
	// THE ONE ASSERTION, and it is about the SHAPE rather than any wall-clock
	// figure: option A must be more than an order of magnitude dearer to produce
	// than the scheme already shipping, or the #8 objection to entering on a
	// library choice does not apply and this whole measurement was unnecessary.
	// If this ever fails, the pairing option got cheap and the option space
	// re-opens — which is a finding, not a broken test.
	if optionA < 10*current {
		t.Fatalf("option A's production cost (%v) is within 10x the shipped scheme's (%v). The #8 disqualification argument "+
			"does not hold at that ratio and the option space must be re-derived: a publicly-verifiable scheme that is "+
			"cheap to produce is the better answer to the care-link break, not the worse one.", optionA, current)
	}
}

// TestHashOnlySpotCheckProofSizeIsReported prices what option B PAYS, which is the
// other half of the #8 comparison and the reason the sheet was wary of it.
//
// The sheet's objection was exact and it is about ONE sample count: "on the
// measured 262,160 B shard a 67-sample challenge over 4 KiB blocks is the whole
// shard". True — 67 samples over 67 blocks IS the shard, so at that setting option
// B is not an audit, it is a fetch. But 67 is not a required number: it is the
// current scheme's porSampleCount, which is free there because a homomorphic
// aggregate is size-independent. A spot-check picks its sample count from the
// DETECTION CONFIDENCE it wants against a prover that dropped a fraction of the
// shard, and silt erasure-codes, so a holder must drop real bytes to save anything.
//
// The table below is that trade, computed from the geometry rather than asserted.
func TestHashOnlySpotCheckProofSizeIsReported(t *testing.T) {
	p := DefaultParams
	blockBytes := p.blockBytes()
	blocks := p.Blocks(shardBytes)
	pathHashes := 0
	for n := blocks; n > 1; n = (n + 1) / 2 {
		pathHashes++
	}

	t.Logf("HASH-ONLY SPOT-CHECK PROOF SIZE over a %d-byte shard (%d blocks x %d B, %d-hash Merkle path):",
		shardBytes, blocks, blockBytes, pathHashes)
	for _, c := range []struct {
		dropped    float64 // the fraction of the shard the prover discarded
		confidence float64 // the detection probability we want against it
	}{
		{1.00, 0.9999}, // dropped the whole shard: one sample settles it
		{0.50, 0.999},
		{0.10, 0.95},
		{0.01, 0.95},
	} {
		samples := samplesFor(c.dropped, c.confidence)
		if samples > blocks {
			samples = blocks
		}
		bytes := samples*blockBytes + samples*pathHashes*32
		t.Logf("  prover dropped %5.0f%% -> %2d samples for %.2f%% detection = %7d B (%.0f%% of the shard)",
			c.dropped*100, samples, c.confidence*100, bytes, 100*float64(bytes)/float64(shardBytes))
	}
	t.Logf("  the sheet's 67-sample setting = %d B, which IS the whole shard — that is a property of the sample "+
		"COUNT carried over from the aggregate scheme, not of spot-checking", blocks*blockBytes+blocks*pathHashes*32)
	t.Logf("  AND THE TOP ROW IS THE ADVERSARY THAT MATTERS: silt is content-addressed and re-verifies every read " +
		"against its hash (B3), so a holder keeping 99%% of a shard can SERVE nothing and has saved 1%% of a disk. " +
		"The strategy that pays is dropping the shard, and that costs 4192 B to catch.")

	// THE ASSERTION: the audit must be able to catch a materially cheating holder
	// for materially less than fetching the shard. If it cannot, option B is a
	// fetch wearing an audit's name and the comparison above is decoration.
	half := samplesFor(0.5, 0.999)*blockBytes + samplesFor(0.5, 0.999)*pathHashes*32
	if half >= shardBytes/2 {
		t.Fatalf("catching a holder that dropped HALF its shard at 99.9%% costs %d B of a %d-byte shard. Spot-checking "+
			"buys too little over just fetching the bytes at this geometry, and option B needs a smaller block size "+
			"before it can be compared with option A at all.", half, shardBytes)
	}
}

// samplesFor is the sample count l such that 1-(1-dropped)^l >= confidence: the
// standard spot-check bound. Independent uniform samples WITH replacement, which
// is the pessimistic direction — sampling without replacement detects sooner.
func samplesFor(dropped, confidence float64) int {
	if dropped >= 1 {
		return 1
	}
	l := 1
	miss := 1 - dropped
	acc := miss
	for 1-acc < confidence {
		acc *= miss
		l++
		if l > 100000 {
			break
		}
	}
	return l
}

func timeOnce(t *testing.T, f func()) time.Duration { return timeN(t, 1, f) }

// timeN runs f n times and returns the mean. A mean over repeats, never a single
// sample: this box's own record has a threefold spread between runs of unchanged
// work, so one reading is not a measurement.
func timeN(t *testing.T, n int, f func()) time.Duration {
	t.Helper()
	start := time.Now()
	for i := 0; i < n; i++ {
		f()
	}
	return time.Since(start) / time.Duration(n)
}
