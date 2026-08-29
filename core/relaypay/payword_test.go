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
	v := NewVerifier(c.Root())
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
	v := NewVerifier(c.Root())
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
	v := NewVerifier(c.Root())
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
