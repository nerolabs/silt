package bond

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// TestMeasure_VerifyCPUBreakdown299: where does the per-answer verify CPU go?
// This is the term that prices every new block on the event loop (the #528
// knee's ~1s/block was dominated by bond re-verification during replay; the
// replay is gone, but each freshly synced/committed block still pays one
// verify on the loop). Measurement only — read with -v.
func TestMeasure_VerifyCPUBreakdown299(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement only")
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const size = 64 << 20
	c := Seal(pub, size)
	a, ok := c.Answer(12345, DefaultLabelSamples)
	if !ok {
		t.Fatal("answer failed")
	}
	const rounds = 20
	t0 := time.Now()
	for i := 0; i < rounds; i++ {
		if !Verify(pub, c.Root, size, 12345, a, DefaultLabelSamples) {
			t.Fatal("verify failed")
		}
	}
	per := time.Since(t0) / rounds
	t.Logf("space-only Verify (64MiB, k=%d): %v per answer", DefaultLabelSamples, per)
}
