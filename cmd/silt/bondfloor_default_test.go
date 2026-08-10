package main

import (
	"testing"
	"time"

	"github.com/nerolabs/silt/core/bond"
)

// Build-immutable #3/#4 (docs/TENETS.md): the anti-release floor is sized against
// a COMPUTE window (re-seal time × plot throughput), DECOUPLED from any transport
// timeout — so raising -request-timeout for durability (#288) never moves it and
// never prices out small validators. Lock the derivation in: the floor must equal
// the compute-window arithmetic exactly, proving it is not sourced from a network
// deadline. If someone re-couples it to RequestTimeout, this fails.
func TestAntiReleaseFloorIsComputeSourcedNotTransport(t *testing.T) {
	wantSecs := int64(AntiReleaseComputeWindow / time.Second)
	want := int64(2) * (wantSecs * bond.PlotSealThroughput) // 2× margin over window×throughput
	if DerivedBondFloor != want {
		t.Fatalf("anti-release floor must be derived from the compute window (2 × %ds × %d B/s = %d), got %d — is it (wrongly) coupled to a transport timeout?",
			wantSecs, int64(bond.PlotSealThroughput), want, DerivedBondFloor)
	}
	if AntiReleaseComputeWindow <= 0 {
		t.Fatal("the anti-release compute window must be a positive, explicit budget")
	}
}

// Retest G4-RESIDUAL INVERTED as a regression: the anti-release floor must be ON
// BY DEFAULT for an untrusted (objective) validator. #163 shipped the mechanism
// but defaulted both knobs to 0, so a stock/doc-following open M0 validator still
// admitted a sub-floor, releasable bond to full objective standing —
// "fixed but off by default" is not fixed.
func TestAntiReleaseFloorDefaultsOnForUntrustedValidator(t *testing.T) {
	// The stock untrusted-validator posture (no -min-bond-floor passed): the floor
	// is derived, not left off.
	got, defaulted := effectiveBondFloor(false, 0, true)
	if !defaulted || got != DerivedBondFloor {
		t.Fatalf("G4-residual regression: an untrusted validator that sets no floor must GET one: got %d (defaulted=%v), want %d", got, defaulted, DerivedBondFloor)
	}
	// The derived floor must actually exceed what re-plots inside a challenge
	// window (~270 MB/s x ~2s ≈ 540 MiB) — otherwise it denies nothing.
	const replotInWindow = int64(540) << 20
	if got <= replotInWindow {
		t.Fatalf("the derived floor (%d MiB) must exceed the ~%d MiB re-plottable inside one challenge window", got>>20, replotInWindow>>20)
	}
	// It must also deny the daemon's own default 64M bond, the exact posture the
	// red team's PoC exercised.
	if int64(64)<<20 >= got {
		t.Fatal("the derived floor must deny the default 64M bond on an untrusted swarm")
	}

	// An EXPLICIT choice always wins — including 0, the trusted/demo opt-out.
	if got, defaulted := effectiveBondFloor(true, 0, true); got != 0 || defaulted {
		t.Fatalf("an explicit -min-bond-floor 0 must opt out of the floor: got %d (defaulted=%v)", got, defaulted)
	}
	if got, _ := effectiveBondFloor(true, 2<<30, true); got != 2<<30 {
		t.Fatalf("an explicit floor must be honored verbatim: got %d", got)
	}

	// A TRUSTED swarm (or any non-objective path) is unaffected: no floor is
	// imposed, so demos and single-box deployments keep working.
	if got, defaulted := effectiveBondFloor(false, 0, false); got != 0 || defaulted {
		t.Fatalf("a trusted/non-objective node must get no imposed floor: got %d (defaulted=%v)", got, defaulted)
	}
}
