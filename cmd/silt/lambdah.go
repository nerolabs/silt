package main

// λ_H arrival-rate instrumentation — the measurement the CT-1 conditional theorem is
// owed. See research cert C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27,
// §6 ("the one owed input"): RECORD the honest-arrival RATE at launch and alarm on a
// floor exit. Design: docs/thinking/2026-08-27-lambda-h-arrival-rate-instrumentation.md.
//
// λ_H is the operator/domain-distinct bonded-arrival rate per block-height. The honest-
// arrival COUNT A(t) is the shipped shed metric min(NakamotoOperators, NakamotoDomains)
// (chain.MatureCoefficient) — the exact distinctness the maturity shed gates on, so the
// floor parameterizes the same quantity the theorem binds (T_mature ≤ M_req / λ_H).
// λ_H over a window [t0,t1] is ΔA/Δheight — a NET rate (a lapsed TTL bond drops A, so it
// can be ≤ 0, which is precisely the stalled/reversed arrival a floor-exit surfaces).
//
// This is OBSERVABILITY over the committed ledger: it reads A(t) via a pure chain getter
// and holds only ephemeral windowing state in the observer. It changes no validity
// predicate, no consensus rule (I1–I5), no security parameter.

// lambdaHTracker keeps a trailing ring of (height, coefficient) samples and computes the
// realized honest-arrival rate λ_H = ΔA/Δheight over the window. It lives in the daemon's
// commit observer, beside the C2 concentration alarm — the chain stays a pure reader.
type lambdaHTracker struct {
	window  uint64 // trailing window width in block-heights
	heights []uint64
	coeffs  []int // A(height) = min(NakamotoOperators, NakamotoDomains) at that height
}

func newLambdaHTracker(window uint64) *lambdaHTracker {
	if window == 0 {
		window = 1
	}
	return &lambdaHTracker{window: window}
}

// observe records the committed distinctness count A at a height and evicts samples that
// have fallen outside the trailing window [height-window, height].
func (t *lambdaHTracker) observe(height uint64, coeff int) {
	t.heights = append(t.heights, height)
	t.coeffs = append(t.coeffs, coeff)
	// Evict samples older than the trailing window. Keep the oldest that still anchors a
	// full-width span so the rate is measured over ~window heights.
	var lo uint64
	if height > t.window {
		lo = height - t.window
	}
	i := 0
	for i < len(t.heights)-1 && t.heights[i] < lo {
		i++
	}
	if i > 0 {
		t.heights = t.heights[i:]
		t.coeffs = t.coeffs[i:]
	}
}

// ready reports whether the window holds at least two samples spanning a positive height
// interval — the minimum to compute a rate.
func (t *lambdaHTracker) ready() bool {
	return len(t.heights) >= 2 && t.heights[len(t.heights)-1] > t.heights[0]
}

// rate returns the measured λ_H = (A(t1) − A(t0)) / (t1 − t0) over the trailing window,
// the realized net operator/domain-distinct bonded-arrival rate per block-height. Only
// meaningful when ready().
func (t *lambdaHTracker) rate() float64 {
	if !t.ready() {
		return 0
	}
	last := len(t.heights) - 1
	dA := float64(t.coeffs[last] - t.coeffs[0])
	dH := float64(t.heights[last] - t.heights[0])
	return dA / dH
}

// span returns the height interval the current rate is measured over.
func (t *lambdaHTracker) span() uint64 {
	if !t.ready() {
		return 0
	}
	return t.heights[len(t.heights)-1] - t.heights[0]
}

// belowFloor reports whether the measured λ_H has fallen strictly below the configured
// floor — the CT-1 hypothesis-H exit. Only fires once the window is ready() and the floor
// is enabled (> 0). The CALLER additionally gates on !EverMature(): after the one-way
// latch the arrival floor is moot (P4 — post-maturity concentration cannot re-arm anchors),
// so the alarm only matters in the pre-maturity window the theorem must order.
func (t *lambdaHTracker) belowFloor(floor float64) bool {
	return floor > 0 && t.ready() && t.rate() < floor
}
