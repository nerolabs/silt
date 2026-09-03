//go:build !race

package blindtoken

// The C-3 COST measurement, split out behind `//go:build !race`.
//
// WHY THE BUILD TAG. This gate times 2048-bit big.Int arithmetic, and the race detector
// inflates it several-fold — measured 3.3 ms without -race and 9.7 ms with it, on the same
// machine and the same code. Under -race the measurement would be measuring the detector,
// not the function, on both sides of the ratio and not necessarily in step. The gate
// therefore runs in the NON-race full suite (`go test ./...`, which CI runs), and is
// skipped only in the -race job. Nothing else moves: every correctness gate in
// crypto_advisory_gates_test.go runs under both.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

// TestC3_ValidatePubCostBudget is a RATIO gate, not a wall-clock budget. What must hold is
// that ONE key admission stays within a constant factor of ONE RSA verify ON THE SAME
// MACHINE: 154x measured on an M4 (3.290916 ms / 21.454 µs), 159x in the crypto advisory.
// Both sides are 2048-bit big.Int work in the same process, so a uniform hardware slowdown
// cancels where an absolute millisecond figure does not — the same code costs 3.3 ms on the
// owner's M4 and 10.5 ms best / 30.3 ms worst on the CI runner, which is the closer analogue
// of a pony-class node. The milliseconds are still LOGGED, so a human reads the real cost;
// only the ASSERTION is hardware-independent.
//
// K = 1000 is ~6x headroom over the measured 154x. It trips on the regressions this gate
// exists to catch — ProbablyPrime(1) -> ProbablyPrime(20), or SmallFactorBound 2^16 -> 2^24 —
// without being a timing flake on a loaded box.
//
// THE OTHER DIRECTION IS ALREADY GATED. TestC3_HardnessRunsAtAdmissionNotOnEveryModexp
// asserts perVerify*10 <= admission, which trips if the hardness half moves back ONTO the
// modexp path. Upper bound here, lower bound there. Neither is calibrated to a machine.
//
// Ruling that set this shape (5 ms REFUTED as a security parameter, disposition (b)):
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-ValidatePub-cost-gate-RULING-2026-09-03.md
//
// IT ASSERTS ON THE BEST OF N, not the worst, and that is deliberate. `go test ./...` runs
// packages in parallel, so a wall-clock WORST case measures scheduler contention on a loaded
// box, not this function. The minimum is the least-contended sample and the closest estimate
// of the single-threaded cost the node loop actually pays. The spread is logged so a real
// regression is still visible.
func TestC3_ValidatePubCostBudget(t *testing.T) {
	// K: the ceiling on (one full key admission) / (one RSA verify), same machine, same process.
	const K = 1000
	// The primorial build is a ONE-TIME process cost, not a per-pin cost. Time it separately
	// and warm it before the measurement, so this gate does not silently become order-dependent
	// on whichever test ran first.
	warm := time.Now()
	PrewarmValidatePub()
	build := time.Since(warm)

	const samples = 20
	keys := make([]*rsa.PublicKey, samples)
	for i := range keys {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = &k.PublicKey
	}

	// The DENOMINATOR: one RSA verify under the same key, on this machine, right now.
	priv := testKey(t)
	vpub := &priv.PublicKey
	serial, _ := NewSerial(rand.Reader)
	blinded, secret, berr := Blind(rand.Reader, vpub, serial)
	if berr != nil {
		t.Fatal(berr)
	}
	sig, uerr := Unblind(vpub, serial, mustSign(t, priv, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	const verifies = 200
	vstart := time.Now()
	for i := 0; i < verifies; i++ {
		if !Verify(vpub, serial, sig) {
			t.Fatal("setup: the signature must verify")
		}
	}
	perVerify := time.Since(vstart) / verifies
	if perVerify <= 0 {
		t.Fatalf("verify measured as %v — the timer resolution cannot carry this ratio", perVerify)
	}

	best, worst := time.Duration(1<<62), time.Duration(0)
	for _, k := range keys {
		start := time.Now()
		if verr := ValidatePub(k); verr != nil {
			t.Fatalf("honest key refused: %v", verr)
		}
		d := time.Since(start)
		if d < best {
			best = d
		}
		if d > worst {
			worst = d
		}
	}
	t.Logf("MEASURED: ValidatePub on an honest 2048-bit key over %d samples — best %v, worst %v; "+
		"one RSA verify on this machine = %v; ratio best/verify = %dx (ceiling %dx). At most "+
		"W+1 = 5 pins per issuer per window (2W+1 = 9 across the staged band). One-time "+
		"primorial build = %v.", samples, best, worst, perVerify, int64(best/perVerify), K, build)
	if best > K*perVerify {
		t.Fatalf("ValidatePub costs %v at BEST against a %v RSA verify on this same machine — "+
			"a %dx ratio, over the %dx ceiling. Key admission must stay within a constant factor "+
			"of one verify: it runs on the single-threaded node loop at every distinct key pin, "+
			"at most W+1 = 5 per issuer per window.", best, perVerify, int64(best/perVerify), K)
	}
}
