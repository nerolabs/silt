package chain

import "testing"

// TestRetentionHorizonAt pins the pure arithmetic of the rolling retention horizon
// (the read-only half of the H2 payload-selective-prune OOM fix). Blocks strictly
// BELOW the returned height are prune-eligible (their heavy BondReg.Answer may be
// dropped, header+sigs kept); blocks at/above it retain the full proof and are still
// re-verifiable. Research-certified safetyDepth = 2·BondTTL, epoch-aligned; the
// horizon is FLOORED to an epoch boundary so it lands on a validator-set snapshot
// (#357 Cond A) and retains AT LEAST safetyDepth (err long, per the cert).
func TestRetentionHorizonAt(t *testing.T) {
	const ttl = 32
	const safety = 2 * ttl // 64

	cases := []struct {
		name                            string
		finalizedHeight, safety, epochs uint64
		want                            uint64
	}{
		// Not enough history to retain a full safetyDepth window → prune nothing.
		{"shallow-zero", 0, safety, 0, 0},
		{"exactly-safety", safety, safety, 0, 0},
		{"one-below-safety", safety - 1, safety, 0, 0},
		// Deep enough, no epochs: horizon = finalizedHeight − safetyDepth.
		{"deep-no-epochs", 200, safety, 0, 200 - safety}, // 136
		// Deep with epochs: floor (finalizedHeight − safetyDepth) to an epoch boundary.
		//   raw = 300 − 64 = 236; epochs=50 → floor(236/50)*50 = 200.
		{"deep-epoch-floored", 300, safety, 50, 200},
		//   raw = 264 − 64 = 200; already on a 50-boundary → 200.
		{"deep-epoch-on-boundary", 264, safety, 50, 200},
		// Flooring must never raise the horizon above raw (retain ≥ safetyDepth).
		{"epoch-floor-conservative", 249, safety, 50, 150}, // raw=185 → floor→150
	}
	for _, c := range cases {
		if got := retentionHorizonAt(c.finalizedHeight, c.safety, c.epochs); got != c.want {
			t.Errorf("%s: retentionHorizonAt(%d,%d,%d) = %d, want %d",
				c.name, c.finalizedHeight, c.safety, c.epochs, got, c.want)
		}
	}
}

// TestRetentionHorizonPrunesNothingWithoutFinality guards the safety precondition:
// without BFT finality (a trusted/legacy config with no super-quorum) there is no
// finalized anchor to prune below, so the horizon is 0 (prune nothing). A default
// Config has EpochBlocks=0 / no objective finality, so a fresh Chain must report 0.
func TestRetentionHorizonPrunesNothingWithoutFinality(t *testing.T) {
	c := New(Config{BondTTLBlocks: 32}, nil)
	if got := c.RetentionHorizon(); got != 0 {
		t.Fatalf("RetentionHorizon on a non-final chain = %d, want 0 (no finality anchor → prune nothing)", got)
	}
}
