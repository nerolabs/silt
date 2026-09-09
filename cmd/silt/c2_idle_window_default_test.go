package main

// Lane C2 — the RED-first half of the delivery idle window (`R-REAPER-FORFEIT`, ROADMAP
// row C2; owner call 4 of `D-TRUE-UP-CALLS-2026-09-07`). The property half is driven in
// core/node/c2_idle_window_gates_test.go; this half binds the derived floor to the values
// the shipped daemon and the shipped harness actually carry.
//
// THE DERIVATION, in one line:
//
//	the reaper's stamp is coarsened to buckets of window/deliveryStampDivisor
//	(core/node/deliverysession.go), so the GUARANTEED survival — the shortest gap since a
//	real settlement at which a reap can fire — is window·(divisor−1)/divisor, MEASURED at
//	0.751× on a 1000 s window (core/node TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow).
//	The ratified sentence puts the default ABOVE Lane A's bound, so:
//
//	   window ≥ bound · divisor/(divisor−1) = 430 s · 4/3 = 573.34 s
//
// The bound is 430 s: `D-H43-WORKLESS-DESIGNEE` (21) publishes a LOST entry forward as
// bounded by the re-keyed takeover at ≤ (N+2)·ChainSyncInterval + G = 14·30 + 10 at N = 12.
// It dominates the 190 s modal tier (`D-CONSENSUS-ARMING` (19)) and the 380 s cloudtest
// hard cap, both field-confirmed on `integration/cloudtest/report-97e3101-deep.md`.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	// c2StampDivisor mirrors core/node's unexported deliveryStampDivisor. cmd/silt cannot
	// import it; core/node/TestC2StampDivisorIsFour pins the two together and fails if it
	// moves.
	c2StampDivisor = 4
	// c2GoverningBound: D-H43-WORKLESS-DESIGNEE (21), (N+2)·ChainSyncInterval + G at N=12.
	c2GoverningBound = 430 * time.Second
)

// c2DerivedIdleFloor is the smallest window whose GUARANTEED survival clears the bound.
// Ceiling division: nothing below this can be sized "above" the bound in the sense the
// ratified sentence uses, because a quarter of the window is spent by the stamp coarsening.
func c2DerivedIdleFloor() time.Duration {
	num := int64(c2GoverningBound) * c2StampDivisor
	den := int64(c2StampDivisor - 1)
	return time.Duration((num + den - 1) / den)
}

// G-C2-8 — the daemon's ACCEPTED FLOOR must clear the derived floor. RED today:
// deliveryIdleFloor is 1 s, so the daemon accepts a window whose guaranteed survival is
// 0.75 s against a 430 s admitted stall. This is the axis, not a string: it runs the
// shipped constant through the shipped coarsening.
func TestC2AcceptedIdleFloorClearsTheLivenessBound(t *testing.T) {
	want := c2DerivedIdleFloor()
	survives := deliveryIdleFloor - deliveryIdleFloor/c2StampDivisor
	if deliveryIdleFloor < want {
		t.Fatalf("deliveryIdleFloor = %v: the daemon accepts a -delivery-idle-window whose GUARANTEED survival is %v, "+
			"against a worst admitted stall of %v (D-H43-WORKLESS-DESIGNEE (21)). Derived floor = bound × %d/%d = %v. "+
			"Owner call 4 of D-TRUE-UP-CALLS-2026-09-07 sets the window ABOVE the bound; a floor of %v does not enforce that.",
			deliveryIdleFloor, survives, c2GoverningBound, c2StampDivisor, c2StampDivisor-1, want, deliveryIdleFloor)
	}
}

// G-C2-9 — the shipped flag DEFAULT is no longer the refuse-until-set 0. RED today. This
// is a source-text pin and it verifies exactly one thing: that the literal in the
// fs.Duration call is not 0. It does NOT evaluate the default; G-C2-8 is the arm that runs
// a number through the derivation.
func TestC2DeliveryIdleWindowDefaultIsSet(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`fs\.Duration\("delivery-idle-window",\s*([^,]+),`)
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatal("daemon.go no longer declares -delivery-idle-window with fs.Duration — this gate has lost its subject")
	}
	got := strings.TrimSpace(string(m[1]))
	if got == "0" {
		t.Fatalf("-delivery-idle-window default is still %q (REFUSE-UNTIL-SET). Owner call 4 of "+
			"D-TRUE-UP-CALLS-2026-09-07 releases it now that the bound is field-confirmed "+
			"(integration/cloudtest/report-97e3101-deep.md, rows 6-fault-tolerance and 10a-stall-drill). "+
			"The derived floor is %v; the Tester's recommendation is 24m (guaranteed survival 18m = 2.51× "+
			"the 430 s bound, and above the 1040 s h43 field stall on run c450985-deep).", got, c2DerivedIdleFloor())
	}
}

// G-C2-10 — the SHIPPED HARNESS value must clear the derived floor too. RED today: the
// cloudtest graded run sets `-delivery-idle-window 90s`, whose guaranteed survival is
// 67.5 s — under even the 190 s modal tier the same run confirms. A harness that grades the
// paid lane under a window the liveness model can break is grading the wrong thing.
func TestC2CloudtestIdleWindowClearsTheLivenessBound(t *testing.T) {
	re := regexp.MustCompile(`-delivery-idle-window\s+([0-9]+[a-z]+)`)
	found := map[string]string{}
	for _, f := range []string{
		"../../integration/cloudtest/scenarios.sh",
		"../../integration/cloudtest/README.md",
		"../../integration/cloudtest/topology.py",
	} {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range re.FindAllSubmatch(src, -1) {
			found[f] = string(m[1])
		}
	}
	if len(found) == 0 {
		t.Skip("no -delivery-idle-window value found in the cloudtest harness — nothing to grade")
	}
	want := c2DerivedIdleFloor()
	files := make([]string, 0, len(found))
	for f := range found {
		files = append(files, f)
	}
	sort.Strings(files)
	bad := 0
	for _, f := range files {
		d, err := time.ParseDuration(found[f])
		if err != nil {
			t.Fatalf("%s: -delivery-idle-window %q does not parse: %v", f, found[f], err)
		}
		if d < want {
			bad++
			t.Errorf("%s sets -delivery-idle-window %v: guaranteed survival %v, against a worst admitted stall of %v "+
				"(and a 190 s modal tier the SAME run confirms). Derived floor %v.",
				f, d, d-d/c2StampDivisor, c2GoverningBound, want)
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d cloudtest sites set a -delivery-idle-window below the derived floor %v", bad, len(files), want)
	}
}
