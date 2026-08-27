package sim

import (
	"os"
	"testing"
)

// TestPerHeightCostLinear is the STANDING GATE for the depth-war failure class:
// per-height cost that grows with chain depth (#528, #535, #549, #555, #556,
// #558, #560, #561, #562, #563, #572 — all field-confirmed, one class). A unit
// test at a fixed height is ALWAYS green for an O(depth) bug (canonical: #555,
// AllEntries built an O(n) slice per block — green in every constant-height
// test, catastrophic at real chain depth). This gate measures the SLOPE of cost
// vs depth and fails on super-linear growth.
//
// Metric: baseline-subtracted runtime.MemStats.HeapObjects. It is deterministic
// across runs (seeded sim + a forced GC before each sample) — unlike HeapInuse,
// which steps by GC arena granularity and is logged but NOT asserted on.
//
// Bound: for linear cost, doubling the height doubles the depth-attributable
// object count, so growth(2H)/growth(H) → 2.0 (measured baseline: 1.998–1.999).
// A super-linear O(n)-per-block defect (O(n²) total) gives ratio → 4.0. The gate
// asserts the ratio stays below 2.6 at TWO doublings — 30% above the linear
// baseline, 35% below the super-linear signal. See
// docs/thinking/2026-08-27-o-depth-ci-gate.md for the full derivation and the
// false-positive analysis.
//
// Cost: ~6s to h=2000 in the full CI go-test job; ~1.5s to h=1000 under -short.
// Both ladders span two doublings and catch the O(n²) class.
func TestPerHeightCostLinear(t *testing.T) {
	// The ladder: three heights spanning two doublings. Under -short (the default
	// dev/CI-race suite) use the smaller ladder to keep the fast path fast; the
	// full go-test job runs the wider ladder. Both assert the same 2.6 bound.
	ladder := []int{500, 1000, 2000}
	if testing.Short() {
		ladder = []int{250, 500, 1000}
	}

	// Failing-first proof: SILT_ODEPTH_INJECT=1 turns on a synthetic
	// O(n)-per-block accumulator (see injectSuperLinear), which must drive the
	// gate RED. Off in all normal runs. This is the "watch the defect go red"
	// mechanism, not a production knob.
	inject := os.Getenv("SILT_ODEPTH_INJECT") == "1"
	var perHeight func(h int)
	if inject {
		injected = nil
		perHeight = injectSuperLinear
	}

	top := ladder[len(ladder)-1]
	want := map[int]bool{0: true}
	for _, h := range ladder {
		want[h] = true
	}
	got := map[int]uint64{}
	onSample := func(h int, s heapSample) {
		if want[h] {
			got[h] = s.HeapObjects
			t.Logf("gate sample: h=%-4d HeapObjects=%-9d HeapInuse=%.1f MiB chainLen=%d",
				h, s.HeapObjects, float64(s.HeapInuse)/(1<<20), s.ChainLen)
		}
	}

	// sampleEvery is the GCD of the ladder heights (all multiples of 250), so
	// every ladder height lands on a sample; h=0 and the final height are always
	// sampled by driveMeasureHeaps.
	sampleEvery := ladderStep(ladder)
	driveMeasureHeaps(t, top, heapsOpts{sampleEvery: sampleEvery, perHeight: perHeight}, onSample, nil)
	if inject {
		t.Logf("injected accumulator parked %d per-height slices", len(injected))
	}

	for h := range want {
		if _, ok := got[h]; !ok {
			t.Fatalf("missing heap sample at h=%d (ladder=%v, sampleEvery=%d)",
				h, ladder, sampleEvery)
		}
	}

	// growth(h) = objects attributable to depth = HeapObjects(h) - HeapObjects(0).
	// Baseline subtraction removes the fixed h=0 overhead (identities, endpoints,
	// genesis) that would otherwise dilute the ratio below 2.0 even for a purely
	// linear system.
	base := got[0]
	growth := func(h int) float64 {
		g := got[h]
		if g < base {
			t.Fatalf("h=%d HeapObjects %d < baseline %d (impossible)", h, g, base)
		}
		return float64(g - base)
	}

	const bound = 2.6 // linear baseline 1.998; super-linear O(n²) ≈ 4.0.
	// Two-stage doubling test: check every adjacent doubling in the ladder. Both
	// must pass. A single-point fluke cannot fail both at 2.6; a real O(n²)
	// defect fails both.
	for i := 0; i+1 < len(ladder); i++ {
		lo, hi := ladder[i], ladder[i+1]
		if hi != 2*lo {
			t.Fatalf("ladder stage %d→%d is not a doubling; the ratio bound assumes 2×", lo, hi)
		}
		glo, ghi := growth(lo), growth(hi)
		if glo <= 0 {
			t.Fatalf("h=%d has non-positive depth-attributable growth %.0f", lo, glo)
		}
		ratio := ghi / glo
		t.Logf("doubling %d→%d: growth %.0f→%.0f  ratio=%.3f (linear=2.0, bound=%.1f)",
			lo, hi, glo, ghi, ratio, bound)
		if ratio >= bound {
			t.Errorf("SUPER-LINEAR per-height cost: growth(%d)/growth(%d) = %.3f ≥ %.1f. "+
				"Per-height object cost is growing faster than linear with chain depth — "+
				"the depth-war failure class (#555 shape). A hot-path structure is "+
				"accumulating per-block super-linearly. Make it linear (incremental "+
				"update), do not loosen the bound. See docs/thinking/2026-08-27-o-depth-ci-gate.md.",
				hi, lo, ratio, bound)
		}
	}
}

// ladderStep returns the largest sample interval that hits every ladder height
// (their GCD). All ladder heights are multiples of 250, so this is 250.
func ladderStep(ladder []int) int {
	g := ladder[0]
	for _, h := range ladder[1:] {
		g = gcd(g, h)
	}
	return g
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// injectSuperLinear parks `h` fresh heap objects at height h. Wired as the
// per-height hook under SILT_ODEPTH_INJECT=1, the total object count grows as
// sum(1..h) ≈ h²/2 — a synthetic O(n)-per-block regression (the #555 shape) used
// to prove the gate goes RED. The objects are held in the package-level `injected`
// ring so the GC cannot reclaim them and HeapObjects (the asserted metric) climbs
// quadratically. NOT reachable in any normal run.
func injectSuperLinear(h int) {
	for i := 0; i < h; i++ {
		injected = append(injected, make([]byte, 16))
	}
}

// injected parks the synthetic per-height allocations so the injected regression
// is real object growth the GC keeps (only populated under SILT_ODEPTH_INJECT=1).
var injected [][]byte
