package repairproof

import (
	"testing"

	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/ports"
)

// makeStripe builds a full, honestly-encoded stripe of `size`-byte shards and
// returns every shard's bytes (n positions: data 0..k-1, parity k..n-1) and its
// content-addressed ID.
func makeStripe(t *testing.T, p erasure.Params, size int) (shards [][]byte, ids []ports.ChunkID) {
	t.Helper()
	data := make([][]byte, p.K)
	for i := range data {
		d := make([]byte, size)
		for j := range d {
			d[j] = byte((i*31 + j*7 + 1) % 251) // deterministic, no RNG
		}
		data[i] = d
	}
	parity, err := erasure.EncodeStripe(p, data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	shards = make([][]byte, p.N)
	copy(shards, data)
	copy(shards[p.K:], parity)
	ids = make([]ports.ChunkID, p.N)
	for i, s := range shards {
		ids[i] = ports.HashBytes(s)
	}
	return shards, ids
}

// survivorsExcluding picks the first `count` stripe positions that are not
// `target`, returning their bytes keyed by position.
func survivorsExcluding(shards [][]byte, target, count int) map[int][]byte {
	out := make(map[int][]byte, count)
	for pos := 0; pos < len(shards) && len(out) < count; pos++ {
		if pos == target {
			continue
		}
		out[pos] = shards[pos]
	}
	return out
}

// TestVerifyByRecompute_HonestRepairVerifies: for both a data shard and a parity
// shard, reconstructing the "lost" position from exactly k survivors matches the
// manifest-committed ID.
func TestVerifyByRecompute_HonestRepairVerifies(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 64)

	for _, target := range []int{1 /* a data shard */, p.K + 1 /* a parity shard */} {
		surv := survivorsExcluding(shards, target, p.K)
		ok, err := VerifyByRecompute(p, surv, p.K, target, ids[target])
		if err != nil {
			t.Fatalf("target %d: unexpected error %v", target, err)
		}
		if !ok {
			t.Fatalf("target %d: honest repair failed to verify", target)
		}
	}
}

// TestVerifyByRecompute_ExactlyKSurvivorsSuffice and a full n-1 survivor set both
// verify — the recompute is over ANY k of the survivors.
func TestVerifyByRecompute_SurvivorSetSizes(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 96)
	target := 2

	for _, count := range []int{p.K, p.N - 1} {
		surv := survivorsExcluding(shards, target, count)
		ok, err := VerifyByRecompute(p, surv, p.K, target, ids[target])
		if err != nil || !ok {
			t.Fatalf("survivor count %d: ok=%v err=%v, want ok=true", count, ok, err)
		}
	}
}

// TestVerifyByRecompute_WrongClaimRejected: a claim whose committed target ID is
// some OTHER shard's ID (a repairer trying to pass off the wrong bytes) fails.
func TestVerifyByRecompute_WrongClaimRejected(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 64)
	target := 1
	surv := survivorsExcluding(shards, target, p.K)

	// want = a different shard's ID → the recomputed target won't match it.
	ok, err := VerifyByRecompute(p, surv, p.K, target, ids[target+1])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("a claim against the wrong committed ID must be rejected")
	}

	// want = a garbage ID → also rejected.
	garbage := ports.HashBytes([]byte("not a real shard"))
	ok, _ = VerifyByRecompute(p, surv, p.K, target, garbage)
	if ok {
		t.Fatal("a claim against a garbage ID must be rejected")
	}
}

// TestVerifyByRecompute_CorruptedSurvivorRejected: one survivor with a flipped
// byte reconstructs a different target, so the honest committed ID no longer
// matches — the false repair is caught, not silently accepted.
func TestVerifyByRecompute_CorruptedSurvivorRejected(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 64)
	target := 3
	surv := survivorsExcluding(shards, target, p.K)

	// Flip a byte in one survivor (copy first so we don't mutate the shared slice).
	for pos, b := range surv {
		c := append([]byte(nil), b...)
		c[0] ^= 0xff
		surv[pos] = c
		break
	}
	ok, err := VerifyByRecompute(p, surv, p.K, target, ids[target])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("a corrupted survivor set must not verify to the honest target ID")
	}
}

// TestVerifyByRecompute_TooFewSurvivors: below k survivors, the target cannot be
// reconstructed at all, so the claim is unverifiable — never accepted by default.
func TestVerifyByRecompute_TooFewSurvivors(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 64)
	target := 0
	surv := survivorsExcluding(shards, target, p.K-1) // one short

	ok, err := VerifyByRecompute(p, surv, p.K, target, ids[target])
	if err != ErrUnrecoverable {
		t.Fatalf("err = %v, want ErrUnrecoverable", err)
	}
	if ok {
		t.Fatal("an unrecoverable stripe must not verify")
	}
}

// TestVerifyByRecompute_TargetAsOwnSurvivor: supplying the target position as its
// own survivor is rejected — else a claimant could "prove" any bytes by handing
// them in as the survivor.
func TestVerifyByRecompute_TargetAsOwnSurvivor(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 64)
	target := 2
	surv := survivorsExcluding(shards, target, p.K)
	surv[target] = shards[target] // sneak the target in

	_, err := VerifyByRecompute(p, surv, p.K, target, ids[target])
	if err == nil {
		t.Fatal("target supplied as its own survivor must be a structural error")
	}
}

// TestVerifyByRecompute_MalformedInputs: out-of-range target, bad realData, and a
// claim to have repaired an implicit-zero padding position are all structural
// errors, distinct from a well-formed claim that fails to verify.
func TestVerifyByRecompute_MalformedInputs(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	shards, ids := makeStripe(t, p, 64)
	surv := survivorsExcluding(shards, 0, p.K)

	// target out of range.
	if _, err := VerifyByRecompute(p, surv, p.K, p.N, ids[0]); err == nil {
		t.Fatal("out-of-range target must error")
	}
	// bad realData.
	if _, err := VerifyByRecompute(p, surv, 0, 0, ids[0]); err == nil {
		t.Fatal("realData=0 must error")
	}
	// A padding position (realData..k-1) is not a repairable shard.
	if _, err := VerifyByRecompute(p, surv, 3 /* realData */, 3 /* target = pad */, ids[3]); err == nil {
		t.Fatal("claiming to repair an implicit-zero padding position must error")
	}
	// bad code params.
	if _, err := VerifyByRecompute(erasure.Params{K: 0, N: 8}, surv, 1, 0, ids[0]); err == nil {
		t.Fatal("invalid params must error")
	}
}

// TestVerifyByRecompute_ShortFinalStripe: a stripe with fewer than k real data
// chunks (the padded final stripe) still verifies an honest repair of a REAL data
// shard, with the implicit zeros supplied for free.
func TestVerifyByRecompute_ShortFinalStripe(t *testing.T) {
	p := erasure.Params{K: 4, N: 8}
	const realData = 2
	size := 48

	// Encode a short stripe: only realData real data shards.
	data := make([][]byte, realData)
	for i := range data {
		d := make([]byte, size)
		for j := range d {
			d[j] = byte((i*17 + j*5 + 3) % 251)
		}
		data[i] = d
	}
	parity, err := erasure.EncodeStripe(p, data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Full n-slot picture: realData real shards, (k-realData) implicit zeros, parity.
	shards := make([][]byte, p.N)
	copy(shards, data)
	for i := realData; i < p.K; i++ {
		shards[i] = make([]byte, size) // implicit zero shard
	}
	copy(shards[p.K:], parity)
	ids := make([]ports.ChunkID, p.N)
	for i, s := range shards {
		ids[i] = ports.HashBytes(s)
	}

	// Repair real data shard 1 from k survivors (the zeros count for free inside
	// erasure, so supply the real+parity shards).
	target := 1
	surv := map[int][]byte{}
	for pos := 0; pos < p.N && len(surv) < p.K; pos++ {
		if pos == target || (pos >= realData && pos < p.K) {
			continue // skip target and padding positions
		}
		surv[pos] = shards[pos]
	}
	ok, err := VerifyByRecompute(p, surv, realData, target, ids[target])
	if err != nil || !ok {
		t.Fatalf("short-stripe honest repair: ok=%v err=%v, want ok=true", ok, err)
	}
}

// shortFinalStripe builds a SHORT final stripe under p: realData real data shards,
// (k − realData) implicit-zero padding slots, and the full parity set. It returns
// the n-slot picture and each slot's content id. The padding slots carry the zero
// bytes erasure.ReconstructStripe would fill them with, which is exactly why they
// are never STORED: storedShards emits realData + (n − k) refs for such a stripe
// and nothing else.
func shortFinalStripe(t *testing.T, p erasure.Params, realData, size int) (shards [][]byte, ids []ports.ChunkID) {
	t.Helper()
	if realData < 1 || realData >= p.K {
		t.Fatalf("shortFinalStripe needs 1 <= realData < k, got realData=%d k=%d", realData, p.K)
	}
	data := make([][]byte, realData)
	for i := range data {
		d := make([]byte, size)
		for j := range d {
			d[j] = byte((i*23 + j*11 + 5) % 251)
		}
		data[i] = d
	}
	parity, err := erasure.EncodeStripe(p, data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	shards = make([][]byte, p.N)
	copy(shards, data)
	for i := realData; i < p.K; i++ {
		shards[i] = make([]byte, size)
	}
	copy(shards[p.K:], parity)
	ids = make([]ports.ChunkID, p.N)
	for i, s := range shards {
		ids[i] = ports.HashBytes(s)
	}
	return shards, ids
}

// storedSurvivorsExcluding supplies exactly what a JUDGE can supply for a short
// final stripe: every STORED position except the target. Padding positions are
// math, not storage, so no judge can ever hand them in — which is the whole point.
func storedSurvivorsExcluding(shards [][]byte, p erasure.Params, realData, target int) map[int][]byte {
	out := map[int][]byte{}
	for pos := 0; pos < p.N; pos++ {
		if pos == target || (pos >= realData && pos < p.K) {
			continue
		}
		out[pos] = shards[pos]
	}
	return out
}

// TestVerifyByRecompute_ShortFinalStripeJudgeableFromStoredSurvivorsAlone is the
// gate for the `present`-count fix (D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12,
// item 2). At the SHIPPED geometry k=10/n=16, a final stripe of 4 real data chunks
// stores 4 + 6 = 10 shards, so a judge that excludes the claimed position can
// supply at most 9 survivors — one short of k. Counting only supplied survivors
// therefore made the claim STRUCTURALLY unjudgeable forever: the paramedic repairs
// the position and no judge can ever judge it. Every object of four chunks or fewer
// is such an object, which at the default 256 KiB chunk size is every object of at
// most 1 MiB.
//
// The fix counts the implicit-zero padding slots erasure.ReconstructStripe will
// fill, making this function's recoverability predicate exactly ReconstructStripe's
// minus the target.
//
// DRIVEN RED FIRST: without the padding count this returns ErrUnrecoverable for
// every realData in 1..4 at k=10/n=16.
func TestVerifyByRecompute_ShortFinalStripeJudgeableFromStoredSurvivorsAlone(t *testing.T) {
	p := erasure.DefaultParams // the SHIPPED geometry, not a convenient one
	const size = 64
	target := p.K // the stripe's first parity position

	for realData := 1; realData < p.K; realData++ {
		shards, ids := shortFinalStripe(t, p, realData, size)
		surv := storedSurvivorsExcluding(shards, p, realData, target)

		// Derived anti-vacuity: for realData < 2k − n + 1 the supplied set is SHORT
		// of k, so this row is one the old predicate could never judge. Assert the
		// arithmetic rather than trusting the loop.
		wantSupplied := realData + (p.N - p.K) - 1
		if len(surv) != wantSupplied {
			t.Fatalf("realData=%d: a judge could supply %d survivors, want realData+(n−k)−1 = %d — the fixture is not modelling the stored set",
				realData, len(surv), wantSupplied)
		}
		ok, err := VerifyByRecompute(p, surv, realData, target, ids[target])
		if err != nil {
			t.Fatalf("realData=%d (%d supplied survivors, k=%d): err = %v, want nil. A stripe a judge CAN see every stored shard of must be judgeable; "+
				"ErrUnrecoverable here is the `present` count ignoring the %d implicit-zero padding slots that erasure.ReconstructStripe fills for free",
				realData, len(surv), p.K, err, p.K-realData)
		}
		if !ok {
			t.Fatalf("realData=%d: an honest repair of position %d did not verify", realData, target)
		}
	}
	t.Logf("short-final-stripe judgeable at k=%d n=%d for every realData in 1..%d; the tightest row supplies %d survivors against k=%d",
		p.K, p.N, p.K-1, 1+(p.N-p.K)-1, p.K)
}

// TestVerifyByRecompute_PaddingCountDoesNotDeleteTheUnrecoverableSplit is the
// REFUTED-placement guard. The certified fix counts padding; it does NOT delete the
// `present < k` pre-check, and deleting that pre-check is REFUTED
// (D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12): erasure.ReconstructStripe's below-k
// failure is a plain error, which this function maps to (false, nil), and
// Decide(false, …) SLASHES. A genuinely short survivor fetch is a TRANSIENT — the
// node judge defers and retries it — so routing it into (false, nil) would
// bond-slash an HONEST paramedic.
//
// So: even with the padding counted, a survivor set short enough that
// supplied + padding < k must still return ErrUnrecoverable, never (false, nil).
func TestVerifyByRecompute_PaddingCountDoesNotDeleteTheUnrecoverableSplit(t *testing.T) {
	p := erasure.DefaultParams
	const size, realData = 64, 4
	shards, ids := shortFinalStripe(t, p, realData, size)
	target := p.K
	padding := p.K - realData

	// Hand in fewer survivors than the padding can make up: supplied + padding < k.
	supplied := p.K - padding - 1 // 3 at k=10, realData=4
	surv := map[int][]byte{}
	for pos := 0; pos < p.N && len(surv) < supplied; pos++ {
		if pos == target || (pos >= realData && pos < p.K) {
			continue
		}
		surv[pos] = shards[pos]
	}
	if len(surv)+padding >= p.K {
		t.Fatalf("VACUOUS: %d supplied + %d padding >= k=%d — this arm must be BELOW k or it proves nothing", len(surv), padding, p.K)
	}

	ok, err := VerifyByRecompute(p, surv, realData, target, ids[target])
	if err != ErrUnrecoverable {
		t.Fatalf("err = %v, want ErrUnrecoverable. The `present < k` pre-check is what keeps 'I could not check you' distinct from 'you lied': "+
			"without it a short fetch reaches ReconstructStripe, returns (false, nil), and repairproof.Decide SLASHES an honest paramedic's bond", err)
	}
	if ok {
		t.Fatal("a set below k even after padding must not verify")
	}
}

// TestVerifyByRecompute_FullStripeSurvivorRequirementIsUnchanged pins that the
// padding count changes NOTHING on a full stripe. realData == k leaves the range
// [realData, k) empty, so the requirement stays the familiar k supplied survivors
// and k−1 stays unrecoverable. A fix that widened the full-stripe case would be
// widening the judge's acceptance on the path that carries every ordinary object.
func TestVerifyByRecompute_FullStripeSurvivorRequirementIsUnchanged(t *testing.T) {
	p := erasure.DefaultParams
	shards, ids := makeStripe(t, p, 64)
	target := p.K

	if _, err := VerifyByRecompute(p, survivorsExcluding(shards, target, p.K-1), p.K, target, ids[target]); err != ErrUnrecoverable {
		t.Fatalf("k−1 survivors on a FULL stripe: err = %v, want ErrUnrecoverable", err)
	}
	ok, err := VerifyByRecompute(p, survivorsExcluding(shards, target, p.K), p.K, target, ids[target])
	if err != nil || !ok {
		t.Fatalf("k survivors on a FULL stripe: ok=%v err=%v, want ok=true err=nil", ok, err)
	}
}
