package main

import "testing"

// The λ_H tracker is the measurement + floor-exit alarm the CT-1 conditional theorem is
// owed (research cert C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27, §6).
// These tests exercise the rate computation and — the load-bearing ablation — that the
// floor-exit alarm goes RED under stalled/reversed arrivals and stays quiet under a healthy
// climb. A green alarm test with no demonstrated red is a comment that compiles.

func TestLambdaHRateOverWindow(t *testing.T) {
	tr := newLambdaHTracker(10)
	// A climbs 0→1→2→3→4→5 over heights 1..6: 5 distinct arrivals across 5 heights = 1.0.
	for i, coeff := range []int{0, 1, 2, 3, 4, 5} {
		tr.observe(uint64(i+1), coeff)
	}
	if !tr.ready() {
		t.Fatal("tracker should be ready after 6 samples spanning 5 heights")
	}
	if got := tr.rate(); got != 1.0 {
		t.Fatalf("λ_H rate = %.3f, want 1.000 (ΔA=5 over Δheight=5)", got)
	}
	if got := tr.span(); got != 5 {
		t.Fatalf("span = %d, want 5", got)
	}
}

func TestLambdaHNotReadyBeforeTwoSamples(t *testing.T) {
	tr := newLambdaHTracker(10)
	if tr.ready() {
		t.Fatal("empty tracker must not be ready")
	}
	tr.observe(1, 0)
	if tr.ready() {
		t.Fatal("one sample cannot yield a rate")
	}
	// belowFloor must never fire before a rate exists, regardless of floor.
	if tr.belowFloor(1.0) {
		t.Fatal("floor-exit must not fire before the window is ready")
	}
}

func TestLambdaHWindowEvictsOldSamples(t *testing.T) {
	tr := newLambdaHTracker(3) // trailing window of 3 heights
	// Heights 1..10, A climbing by 1 each. With a width-3 window the rate is still 1.0
	// but measured over the recent span, not the whole history.
	for i := 1; i <= 10; i++ {
		tr.observe(uint64(i), i-1)
	}
	if got := tr.span(); got > 3 {
		t.Fatalf("span = %d, must not exceed the window width 3 (old samples not evicted)", got)
	}
	if got := tr.rate(); got != 1.0 {
		t.Fatalf("λ_H rate = %.3f over the trailing window, want 1.000", got)
	}
}

// The ablation: inject the defect the alarm claims to catch (a stalled or reversed honest-
// arrival sequence) and watch belowFloor go RED; a healthy climbing sequence keeps it quiet.
func TestLambdaHFloorExitFiresOnStalledArrivals(t *testing.T) {
	const floor = 0.5

	// Healthy: A climbs 1/height — rate 1.0 ≥ floor — alarm QUIET.
	healthy := newLambdaHTracker(10)
	for i, coeff := range []int{0, 1, 2, 3, 4, 5} {
		healthy.observe(uint64(i+1), coeff)
	}
	if healthy.belowFloor(floor) {
		t.Fatalf("healthy climb (rate %.3f ≥ floor %.3f) must NOT trip the floor-exit alarm", healthy.rate(), floor)
	}

	// Stalled: A flat at 3 — rate 0.0 < floor — alarm RED.
	stalled := newLambdaHTracker(10)
	for i := 0; i < 6; i++ {
		stalled.observe(uint64(i+1), 3)
	}
	if !stalled.belowFloor(floor) {
		t.Fatalf("stalled arrivals (rate %.3f < floor %.3f) MUST trip the floor-exit alarm — the CT-1 hypothesis exit went undetected", stalled.rate(), floor)
	}

	// Reversed: A falls (net attrition, e.g. TTL lapses) — rate < 0 < floor — alarm RED.
	reversed := newLambdaHTracker(10)
	for i, coeff := range []int{5, 4, 3, 2, 1, 0} {
		reversed.observe(uint64(i+1), coeff)
	}
	if !reversed.belowFloor(floor) {
		t.Fatalf("reversed arrivals (rate %.3f < floor %.3f) MUST trip the floor-exit alarm", reversed.rate(), floor)
	}

	// Floor disabled (0): never fires, even on a stall — the default-off contract.
	if stalled.belowFloor(0) {
		t.Fatal("a zero (disabled) floor must never trip the alarm")
	}
}
