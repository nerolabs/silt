package credit

import "testing"

// TestR212FaucetBucketArithmetic pins the bucket under a fake monotonic source: a fresh
// bucket is full; each take drains one; a drained bucket denies; tokens accrue
// CONTINUOUSLY at refill/interval with the sub-token remainder carried exactly; the level
// saturates at capacity; a source that does not advance or steps backward accrues nothing.
func TestR212FaucetBucketArithmetic(t *testing.T) {
	now := int64(1_000)
	f := newFaucet(3, 2, 100, func() int64 { return now }) // 2 tokens per 100 ns = one per 50 ns
	if f.Level() != 3 {
		t.Fatalf("fresh bucket level = %d, want capacity 3", f.Level())
	}
	for i := 0; i < 3; i++ {
		if !f.take() {
			t.Fatalf("take %d denied on a full bucket", i)
		}
	}
	if f.take() {
		t.Fatalf("4th take admitted on an empty bucket")
	}
	now += 49 // 49 ns × 2 / 100 = 0.98 of a token: nothing yet, remainder carried
	if f.take() {
		t.Fatalf("0.98 of a token was spent")
	}
	now += 1 // carried 98 + 2 = 100 ⇒ exactly one token; CONTINUOUS, not at a boundary
	if got := f.Level(); got != 1 {
		t.Fatalf("after 50 ns level = %d, want 1 (continuous accrual with the remainder carried)", got)
	}
	now += 75 // +1.5 tokens ⇒ level 2, carry 50
	if got := f.Level(); got != 2 {
		t.Fatalf("level = %d, want 2", got)
	}
	now += 25 // carry 50 + 50 = 100 ⇒ +1 ⇒ 3 = capacity
	if got := f.Level(); got != 3 {
		t.Fatalf("level = %d, want saturation at 3", got)
	}
	now += 10_000 // a long idle: still capacity, and the carry is dropped, not banked
	if got := f.Level(); got != 3 || f.carry != 0 {
		t.Fatalf("idle bucket level = %d carry = %d, want 3 / 0", got, f.carry)
	}
	f.take()
	f.take()
	f.take()
	now += 25 // half a token after a full drain: nothing banked from the idle period
	if got := f.Level(); got != 0 {
		t.Fatalf("banked tokens from an idle full bucket: level = %d", got)
	}
	now -= 10_000 // a backward source (impossible for a monotonic source; a fake can) changes nothing
	if got := f.Level(); got != 0 {
		t.Fatalf("a backward source changed the level: %d", got)
	}
}

// TestR212FaucetNeverBuildsABucketThatCannotRefill is the PE's S5: a non-positive capacity,
// refill or interval — or no time source — yields nil (UNLIMITED), never a bucket that drains
// once and denies for 104 simulated days.
func TestR212FaucetNeverBuildsABucketThatCannotRefill(t *testing.T) {
	now := func() int64 { return 0 }
	for _, c := range [][3]int64{{0, 1, 1}, {1, 0, 1}, {1, 1, 0}, {-5, 1, 1}, {1, -1, 1}, {1, 1, -1}} {
		if f := newFaucet(c[0], c[1], c[2], now); f != nil {
			t.Fatalf("newFaucet%v built a bucket that can never refill", c)
		}
	}
	if f := newFaucet(1, 1, 1, nil); f != nil {
		t.Fatalf("newFaucet with no time source built a bucket")
	}
	if f := newFaucet(1, 1, 1, now); f == nil {
		t.Fatalf("a valid configuration returned nil")
	}
}

// TestR212FaucetRecoversFromEmptyAfterALongIdle pins the recovery the PE's probe wanted: an
// empty bucket left alone for many intervals is full again — a denial is never permanent.
func TestR212FaucetRecoversFromEmptyAfterALongIdle(t *testing.T) {
	now := int64(0)
	f := newFaucet(256, 256, 3_600_000_000_000, func() int64 { return now }) // 256 per hour
	for i := 0; i < 256; i++ {
		f.take()
	}
	if f.take() {
		t.Fatalf("257th take admitted")
	}
	now += 14_062_500_000 // one token's worth: 3.6e12 / 256 ns
	if got := f.Level(); got != 1 {
		t.Fatalf("after one token-interval level = %d, want 1", got)
	}
	now += 104 * 24 * 3_600_000_000_000 // 104 days
	if got := f.Level(); got != 256 {
		t.Fatalf("after 104 days level = %d, want full", got)
	}
}
