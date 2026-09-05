package credit

import "testing"

// TestR212FaucetBucketArithmetic pins the bucket under a fake monotonic source: a fresh
// bucket is full; each take drains one; a drained bucket denies; a whole interval refills
// by `refill`, never above capacity; a partial interval refills nothing and is not lost;
// a source that does not advance refills nothing. Deterministic, no goroutine, no timer.
func TestR212FaucetBucketArithmetic(t *testing.T) {
	now := int64(1_000)
	f := newFaucet(3, 2, 100, func() int64 { return now })
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
	now += 99 // a partial interval: nothing refills, nothing is lost
	if f.take() {
		t.Fatalf("partial interval refilled the bucket")
	}
	now += 1 // exactly one whole interval since the last fill point
	if got := f.Level(); got != 2 {
		t.Fatalf("after one interval level = %d, want refill 2", got)
	}
	now += 250 // two more whole intervals (+4) saturates at capacity 3
	if got := f.Level(); got != 3 {
		t.Fatalf("level = %d, want saturation at capacity 3", got)
	}
	// The fill point advanced by WHOLE intervals only: 1000 → 1100 → 1300; the 50 ns
	// remainder is carried, so 50 more nanoseconds complete an interval.
	f.take()
	f.take()
	f.take()
	now += 50
	if got := f.Level(); got != 2 {
		t.Fatalf("carried remainder lost: level = %d, want 2", got)
	}
	// A source that steps BACKWARD (impossible for a monotonic source, but a fake can)
	// refills nothing and never panics.
	now -= 10_000
	if got := f.Level(); got != 2 {
		t.Fatalf("a backward source changed the level: %d", got)
	}
}
