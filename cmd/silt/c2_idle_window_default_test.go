package main

// The RED-first half of the delivery idle window. The property half is driven in core/node/c2_idle_window_gates_test.go; this half
// binds the derived floor to the values the shipped daemon and the shipped harness
// actually carry.
//
// THE DERIVATION, in one line:
//
//	the reaper's stamp is coarsened to buckets of window/deliveryStampDivisor
//	(core/node/deliverysession.go), so the GUARANTEED survival — the shortest gap since a
//	real settlement at which a reap can fire — is window·(divisor−1)/divisor, MEASURED at
//	0.751× on a 1000 s window (core/node TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow).
//	The sentence puts the default ABOVE this lane's bound, so:
//
//	 window ≥ bound · divisor/(divisor−1) = 430 s · 4/3 = 573.34 s
//
// The bound is 430 s: `` (21) publishes a LOST entry forward as bounded by the re-keyed
// takeover at ≤ (N+2)·ChainSyncInterval + G = 14·30 + 10 at N = 12. It dominates the 190 s
// modal tier (`` (19)) and the 380 s cloudtest hard cap, both field-confirmed on
// the field report under integration/cloudtest/.

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
	// c2GoverningBound: (21), (N+2)·ChainSyncInterval + G at N=12.
	c2GoverningBound = 430 * time.Second
)

// c2DerivedIdleFloor is the smallest window whose GUARANTEED survival clears the bound.
// Ceiling division: nothing below this can be sized "above" the bound in the sense the
// sentence uses, because a quarter of the window is spent by the stamp coarsening. The
// closed form is bound × divisor/(divisor−1); the exact answer is one step lower whenever
// integer truncation of window/divisor rounds in the window's favour, so the floor is
// settled against the PREDICATE itself rather than against the closed form.
func c2DerivedIdleFloor() time.Duration {
	num := int64(c2GoverningBound) * c2StampDivisor
	den := int64(c2StampDivisor - 1)
	w := time.Duration((num + den - 1) / den)
	for w > 0 && c2ClearsBound(w-1) {
		w--
	}
	return w
}

// c2ClearsBound is the ONE predicate all three gates below judge on: does a configured
// window's guaranteed survival dominate the worst admitted stall?
func c2ClearsBound(window time.Duration) bool {
	return window-window/c2StampDivisor >= c2GoverningBound
}

// c2DefaultLiteral extracts the -delivery-idle-window default expression from daemon.go
// source. Factored out so can exercise the same extraction at both polarities.
func c2DefaultLiteral(src []byte) (string, bool) {
	m := regexp.MustCompile(`fs\.Duration\("delivery-idle-window",\s*([^,]+),`).FindSubmatch(src)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(string(m[1])), true
}

// BOTH POLARITIES of the predicate and of the extraction the three gates below judge on.
// A gate that has only ever been observed RED is a gate whose GREEN is a guess; this is
// the arm that shows what the one-line default has to reach. It carries no dependency
// on a product value, so it stays GREEN before and after the fix.
func TestC2GatePredicateFlipsAtTheDerivedFloor(t *testing.T) {
	want := c2DerivedIdleFloor()
	for _, tc := range []struct {
		window time.Duration
		clears bool
		why    string
	}{
		{time.Second, false, "today's deliveryIdleFloor"},
		{90 * time.Second, false, "today's cloudtest harness value"},
		{190 * time.Second, false, "the window sized at the MODAL tier"},
		{430 * time.Second, false, "the window sized naively AT the governing bound"},
		{want - time.Nanosecond, false, "one nanosecond under the derived floor"},
		{want, true, "the derived floor exactly"},
		{want + time.Nanosecond, true, "one nanosecond over the derived floor"},
		{10 * time.Minute, true, "candidate A (tight)"},
		{24 * time.Minute, true, "candidate B (margin)"},
	} {
		if got := c2ClearsBound(tc.window); got != tc.clears {
			t.Fatalf("c2ClearsBound(%v) = %v, want %v (%s); guaranteed survival %v vs bound %v, derived floor %v",
				tc.window, got, tc.clears, tc.why, tc.window-tc.window/c2StampDivisor, c2GoverningBound, want)
		}
		t.Logf("%-46s window=%-12v guaranteed=%-12v clears=%v", tc.why, tc.window, tc.window-tc.window/c2StampDivisor, tc.clears)
	}
	// The source extraction, both polarities.
	for _, tc := range []struct{ src, want string }{
		{`x := fs.Duration("delivery-idle-window", 0, "help")`, "0"},
		{`x := fs.Duration("delivery-idle-window", 24*time.Minute, "help")`, "24*time.Minute"},
		{`x := fs.Duration("delivery-idle-window", deliveryIdleDefault, "help")`, "deliveryIdleDefault"},
	} {
		got, ok := c2DefaultLiteral([]byte(tc.src))
		if !ok || got != tc.want {
			t.Fatalf("c2DefaultLiteral(%q) = %q/%v, want %q", tc.src, got, ok, tc.want)
		}
	}
	if _, ok := c2DefaultLiteral([]byte("no flag here")); ok {
		t.Fatal("c2DefaultLiteral matched a source with no flag declaration")
	}
}

// The daemon's ACCEPTED FLOOR must clear the derived floor. RED today:
// deliveryIdleFloor is 1 s, so the daemon accepts a window whose guaranteed survival is
// 0.75 s against a 430 s admitted stall. This is the axis, not a string: it runs the
// shipped constant through the shipped coarsening.
func TestC2AcceptedIdleFloorClearsTheLivenessBound(t *testing.T) {
	want := c2DerivedIdleFloor()
	survives := deliveryIdleFloor - deliveryIdleFloor/c2StampDivisor
	if !c2ClearsBound(deliveryIdleFloor) {
		t.Fatalf("deliveryIdleFloor = %v: the daemon accepts a -delivery-idle-window whose GUARANTEED survival is %v, "+
			"against a worst admitted stall of %v (the workless-designee bound). Derived floor = bound × %d/%d = %v. "+
			"A project decision of sets the window ABOVE the bound; a floor of %v does not enforce that.",
			deliveryIdleFloor, survives, c2GoverningBound, c2StampDivisor, c2StampDivisor-1, want, deliveryIdleFloor)
	}
}

// The shipped flag DEFAULT is no longer the refuse-until-set 0. This is a source-text
// pin and it verifies exactly one thing: that the literal in the fs.Duration call is not
// 0. It sees STRINGS ONLY — it does NOT evaluate the default; this gate is the arm that
// runs a number through the derivation, and this gate pins the expression to the derived
// constant. RUNTIME GATE: e2e TestDeliveryIdleWindowDefaultBootsThePaidLane — a daemon
// with
// -accept-delivery-receipts and no -delivery-idle-window boots and announces the window
// It got.
func TestC2DeliveryIdleWindowDefaultIsSet(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := c2DefaultLiteral(src)
	if !ok {
		t.Fatal("SOURCE GATE: daemon.go no longer declares -delivery-idle-window with fs.Duration — this gate has lost its subject")
	}
	if got == "0" {
		t.Fatalf("SOURCE GATE: the -delivery-idle-window default expression is still %q (REFUSE-UNTIL-SET). A project decision of "+
			"releases it now that the bound is field-confirmed "+
			"(integration/cloudtest/report-97e3101-deep.md, rows 6-fault-tolerance and 10a-stall-drill). "+
			"The derived floor is %v; the recommendation is 24m (guaranteed survival 18m = 2.51× "+
			"the 430 s bound, and above the 1040 s field stall on run c450985-deep).", got, c2DerivedIdleFloor())
	}
}

// The SHIPPED HARNESS value must clear the derived floor too. RED today: the cloudtest
// graded run sets `-delivery-idle-window 90s`, whose guaranteed survival is
// 67.5 s — under even the 190 s modal tier the same run confirms. A harness that grades the
// paid lane under a window the liveness model can break is grading the wrong thing.
func TestC2CloudtestIdleWindowClearsTheLivenessBound(t *testing.T) {
	re := regexp.MustCompile(`-delivery-idle-window\s+([0-9]+[a-z]+)`)
	// EVERY match in every file, not the last one per file: a harness file that sets
	// one compliant and one non-compliant window would otherwise grade on whichever
	// came last.
	found := map[string][]string{}
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
			found[f] = append(found[f], string(m[1]))
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
	bad, sites := 0, 0
	for _, f := range files {
		for _, raw := range found[f] {
			sites++
			d, err := time.ParseDuration(raw)
			if err != nil {
				t.Fatalf("%s: -delivery-idle-window %q does not parse: %v", f, raw, err)
			}
			if !c2ClearsBound(d) {
				bad++
				t.Errorf("%s sets -delivery-idle-window %v: guaranteed survival %v, against a worst admitted stall of %v "+
					"(and a 190 s modal tier the SAME run confirms). Derived floor %v.",
					f, d, d-d/c2StampDivisor, c2GoverningBound, want)
			}
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d cloudtest sites set a -delivery-idle-window below the derived floor %v", bad, sites, want)
	}
}

// The epoch cadence core/node's relay arithmetic is derived against. core/node cannot
// import package main, so its c2EpochBlocks literal is pinned here.
func TestC2DerivedEpochBlocksIsEight(t *testing.T) {
	if DerivedEpochBlocks != 8 {
		t.Fatalf("DerivedEpochBlocks = %d, want 8 — core/node's c2EpochBlocks and every relay "+
			"settle-inside-one-epoch figure derived from it move with this", DerivedEpochBlocks)
	}
}
