package relaypay

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// The PayWord primitive tests (docs/design/pod.md §7.3, certified 2026-08-30).
// PayWord is a sender-funded hash chain: the fetcher picks a random tip and
// hashes it S+1 times; the most-hashed value is the ROOT committed once to the
// relay. To authorize increment k the fetcher reveals x_k, and the relay checks
// H(x_k) = x_{k-1} against the preimage it currently holds — one SHA-256 per
// increment. Forgery is cryptographically excluded (one-way hash); the relay
// can redeem only preimages the fetcher revealed.

// TestChainRootIsMostHashedValue: the root is H^{S+1}(tip). Revealing preimages
// in order x_1, x_2, … walks BACK toward the tip, each one hashing to the last.
func TestChainRootIsMostHashedValue(t *testing.T) {
	const S = 16
	tip := []byte("a-random-32-byte-tip-for-testing")
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	if c.Len() != S {
		t.Fatalf("chain length %d, want %d", c.Len(), S)
	}
	// The root must equal hashing the tip S+1 times.
	want := tip
	for i := 0; i < S+1; i++ {
		h := sha256.Sum256(want)
		want = h[:]
	}
	if !bytes.Equal(c.Root(), want) {
		t.Fatalf("root is not H^{S+1}(tip)")
	}
	// The root is the value reached by hashing the MOST times: H(preimage(1)) = root.
	h := sha256.Sum256(c.Preimage(1))
	if !bytes.Equal(h[:], c.Root()) {
		t.Fatalf("H(x_1) must equal the root x_0")
	}
}

// TestRelayVerifyAdvancesOneHash: the relay holds the root, then advances one
// preimage per increment, each verified with a single H(x_k) = x_{k-1}.
func TestRelayVerifyAdvancesOneHash(t *testing.T) {
	const S = 8
	tip := []byte("another-random-tip-value-for-test")
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	v := NewVerifier(c.Root(), S)
	if v.Count() != 0 {
		t.Fatalf("fresh verifier count %d, want 0", v.Count())
	}
	for k := 1; k <= S; k++ {
		if err := v.Advance(c.Preimage(k)); err != nil {
			t.Fatalf("increment %d rejected a valid preimage: %v", k, err)
		}
		if v.Count() != k {
			t.Fatalf("after increment %d count is %d", k, v.Count())
		}
	}
}

// TestForgedPreimageRejected: the relay cannot advance past what the fetcher
// revealed. A random value that does not hash to the held preimage is rejected,
// and the count does not move — the one-way-hash forgery exclusion.
func TestForgedPreimageRejected(t *testing.T) {
	const S = 8
	tip := []byte("tip-for-the-forgery-rejection-test")
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	v := NewVerifier(c.Root(), S)
	if err := v.Advance([]byte("not-a-valid-preimage-at-all-nope!!")); err == nil {
		t.Fatalf("verifier accepted a forged preimage")
	}
	if v.Count() != 0 {
		t.Fatalf("a rejected preimage moved the count to %d", v.Count())
	}
	// A real advance, then a forged one: count stays where the last valid was.
	if err := v.Advance(c.Preimage(1)); err != nil {
		t.Fatalf("valid advance: %v", err)
	}
	if err := v.Advance(c.Preimage(3)); err == nil {
		t.Fatalf("verifier accepted a skip-ahead (x_3 does not hash to x_1)")
	}
	if v.Count() != 1 {
		t.Fatalf("count moved past the last valid preimage: %d", v.Count())
	}
}

// TestSkippedIncrementsAllowedInOrder: the fetcher may reveal x_2 directly if
// the relay is willing to walk two hashes; but the default Advance is strictly
// one hash. AdvanceTo walks up to a bounded number of hashes to reach the
// claimed count, so a fetcher can pay several increments at once.
func TestAdvanceToWalksToClaimedCount(t *testing.T) {
	const S = 10
	tip := []byte("tip-for-the-advance-to-batch-paying")
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	v := NewVerifier(c.Root(), S)
	// Jump straight to increment 5 by revealing x_5; the verifier walks 5 hashes.
	if err := v.AdvanceTo(c.Preimage(5), 5); err != nil {
		t.Fatalf("AdvanceTo(5) rejected a valid preimage: %v", err)
	}
	if v.Count() != 5 {
		t.Fatalf("AdvanceTo left count at %d, want 5", v.Count())
	}
	// A claimed count that the preimage does not actually reach is rejected.
	if err := v.AdvanceTo(c.Preimage(6), 8); err == nil {
		t.Fatalf("AdvanceTo accepted a preimage that does not reach the claimed count")
	}
	// Cannot go backward.
	if err := v.AdvanceTo(c.Preimage(3), 3); err == nil {
		t.Fatalf("AdvanceTo accepted a backward move")
	}
	if v.Count() != 5 {
		t.Fatalf("a rejected AdvanceTo moved the count to %d", v.Count())
	}
}

// TestAdvanceToClampsToChainLength is the #644 failing-first test: an adversarial
// oversized claimedCount (far past the committed chain length S) must be REJECTED
// BEFORE the hash walk runs, so a single bogus MsgRelayPay cannot spin the relay
// for millions of hashes. The Verifier carries S (NewVerifier(root, S)); AdvanceTo
// rejects claimedCount > S before walking.
//
// The ablation: the per-verifier walk-step counter (v.walkSteps) must never exceed
// S for this call. Removing the `claimedCount > S` clamp lets the walk run the full
// attacker-chosen (claimedCount - count) hashes — the ~5M-hash spin the PE measured
// — and the counter assertion turns RED.
func TestAdvanceToClampsToChainLength(t *testing.T) {
	const S = 8
	tip := []byte("a-random-32-byte-tip-for-the-clamp")
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	v := NewVerifier(c.Root(), S)

	// An attacker sends a bogus preimage claiming to reach a count far past S.
	// The preimage MUST be a valid 32-byte length (otherwise AdvanceTo rejects on
	// the length check before the walk, and the ablation would not be exercised).
	// Without the clamp this walks 5,000,000 hashes before the equality check can
	// reject. With the clamp it is rejected before any walk.
	bogus := make([]byte, hashLen) // 32 bytes of zeros: valid length, bogus value
	const bogusCount = 5_000_000
	// walkSteps accumulates across calls on the same verifier, so each sub-check
	// measures the DELTA from a baseline taken just before its AdvanceTo call.
	before := v.walkSteps
	err = v.AdvanceTo(bogus, bogusCount)
	if err == nil {
		t.Fatalf("AdvanceTo accepted an oversized claimedCount %d > S=%d", bogusCount, S)
	}
	if steps := v.walkSteps - before; steps > uint64(S) {
		t.Fatalf("oversized claimedCount drove %d hash-walk steps (> S=%d): the S-clamp is missing, the CPU-DoS is open", steps, S)
	}
	// The count must not have moved.
	if v.Count() != 0 {
		t.Fatalf("a rejected oversized AdvanceTo moved the count to %d", v.Count())
	}

	// A legitimate in-bounds AdvanceTo still works and its walk is bounded by S.
	before = v.walkSteps
	if err := v.AdvanceTo(c.Preimage(5), 5); err != nil {
		t.Fatalf("in-bounds AdvanceTo(5) rejected: %v", err)
	}
	if steps := v.walkSteps - before; steps > uint64(S) {
		t.Fatalf("in-bounds walk ran %d steps (> S=%d)", steps, S)
	}
	// Claiming exactly S is allowed; claiming S+1 is rejected before the walk.
	before = v.walkSteps
	if err := v.AdvanceTo(c.Preimage(5), S+1); err == nil {
		t.Fatalf("AdvanceTo accepted claimedCount S+1 = %d > S=%d", S+1, S)
	}
	if steps := v.walkSteps - before; steps > uint64(S) {
		t.Fatalf("an over-S claim ran %d hash-walk steps (> S=%d) before rejecting", steps, S)
	}
}

// BenchmarkPayWordVerify times the REAL per-increment verify — Verifier.Advance,
// one SHA-256(32 B) plus the held-preimage advance — the cost §5 constraint (a)
// weighs against forwarding time. Run without -race (timing only). The chain is
// prebuilt so only the verify is timed; when the chain is exhausted the verifier
// is reset outside the timed section.
func BenchmarkPayWordVerify(b *testing.B) {
	const S = 4096
	tip := []byte("a-random-32-byte-tip-for-the-bench")
	c, err := BuildChain(tip, S)
	if err != nil {
		b.Fatalf("BuildChain: %v", err)
	}
	// Preimages are revealed in order x_1 … x_S; each Advance is one SHA-256.
	pre := make([][]byte, S+1)
	for k := 1; k <= S; k++ {
		pre[k] = c.Preimage(k)
	}
	v := NewVerifier(c.Root(), S)
	k := 1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := v.Advance(pre[k]); err != nil {
			b.Fatalf("Advance(%d) rejected a valid preimage: %v", k, err)
		}
		if k++; k > S {
			b.StopTimer()
			v = NewVerifier(c.Root(), S)
			k = 1
			b.StartTimer()
		}
	}
}
