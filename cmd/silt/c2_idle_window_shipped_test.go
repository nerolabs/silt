package main

// Lane C2 — the SHIPPED half of the delivery idle window: the constants the daemon runs
// on, pinned to the derivation the RED-first gates in c2_idle_window_default_test.go
// judge. Those gates ask "does the shipped value clear the bound"; these ask "is the
// shipped value the DERIVED one, and is it the one the flag actually hands the node".
//
// The distinction matters because a floor of a year clears the bound too. The floor is
// EXACT here: the smallest window whose guaranteed survival dominates the worst stall the
// liveness model admits. Anything larger refuses windows the model says are safe.

import (
	"os"
	"testing"
	"time"
)

// G-C2-13 — the shipped constants ARE the derivation. Every input is cross-pinned to the
// Tester's independently-written copy, so a change to either side is a RED test and not a
// silent divergence: the divisor to core/node's (via c2StampDivisor, itself pinned by
// core/node TestC2StampDivisorIsFour), the bound to D-H43-WORKLESS-DESIGNEE (21), the
// floor to the predicate, and the default to the flag's own source literal.
func TestC2ShippedFloorIsDerivedFromTheBound(t *testing.T) {
	if deliveryIdleStampDivisor != c2StampDivisor {
		t.Fatalf("deliveryIdleStampDivisor = %d but the derivation's divisor is %d — the production floor and the gate that judges it no longer share the reaper's coarsening",
			deliveryIdleStampDivisor, c2StampDivisor)
	}
	if deliveryIdleBound != c2GoverningBound {
		t.Fatalf("deliveryIdleBound = %v, want %v (D-H43-WORKLESS-DESIGNEE (21): (N+2)·ChainSyncInterval + G = 14·30 + 10 at N = 12)",
			deliveryIdleBound, c2GoverningBound)
	}
	// EXACT, not ">=": the floor is the smallest safe window. A larger floor refuses
	// windows the model admits; a smaller one accepts windows it does not.
	if want := c2DerivedIdleFloor(); deliveryIdleFloor != want {
		t.Fatalf("deliveryIdleFloor = %v, want exactly %v (bound × %d/%d, settled against the predicate). Guaranteed survival %v vs bound %v",
			deliveryIdleFloor, want, c2StampDivisor, c2StampDivisor-1,
			deliveryIdleFloor-deliveryIdleFloor/c2StampDivisor, deliveryIdleBound)
	}
	// The endpoints of the floor itself, in the arithmetic the daemon refuses on.
	if c2ClearsBound(deliveryIdleFloor - time.Nanosecond) {
		t.Fatal("one nanosecond under deliveryIdleFloor still clears the bound — the floor is not the smallest safe window, so the daemon refuses windows that are in fact safe")
	}
	if !c2ClearsBound(deliveryIdleFloor) {
		t.Fatalf("deliveryIdleFloor %v does not clear the bound", deliveryIdleFloor)
	}
	// The old floor's job survives the raise: the daemon starts an idle/2 wall-clock
	// ticker (daemon.go), and time.NewTicker panics on a non-positive interval.
	if deliveryIdleFloor/2 <= 0 {
		t.Fatal("deliveryIdleFloor/2 is not positive — the delivery-sweep ticker would panic (blind PE item 3)")
	}
}

// G-C2-14 — the shipped DEFAULT, and the reason it is 24m rather than the tighter 10m that
// also clears the floor. The margin datum is the ONE stall the field produced (1040 s, run
// c450985-deep h42→h43); the survival claim about it is driven in core/node
// TestC2SessionSurvivesTheWorstAdmittedStall, both candidates, both directions.
func TestC2ShippedDefaultClearsTheFloorAndTheObservedFieldStall(t *testing.T) {
	if deliveryIdleDefault != 24*time.Minute {
		t.Fatalf("deliveryIdleDefault = %v, want 24m — core/node's c2CandidateMargin (1440 s), the value the survival arms are driven at, moves with this",
			deliveryIdleDefault)
	}
	if deliveryIdleDefault < deliveryIdleFloor {
		t.Fatalf("deliveryIdleDefault %v is below deliveryIdleFloor %v — the shipped default would be refused by the shipped daemon", deliveryIdleDefault, deliveryIdleFloor)
	}
	if !c2ClearsBound(deliveryIdleDefault) {
		t.Fatalf("the shipped default %v does not clear the bound %v (guaranteed survival %v)",
			deliveryIdleDefault, deliveryIdleBound, deliveryIdleDefault-deliveryIdleDefault/c2StampDivisor)
	}
	if got := deliveryIdleDefault - deliveryIdleDefault/deliveryIdleStampDivisor; got < deliveryIdleFieldStall {
		t.Fatalf("the shipped default guarantees %v of survival, under the %v stall the field actually produced — the tighter candidate (10m, guaranteed 7m30s) was rejected for exactly this",
			got, deliveryIdleFieldStall)
	}
	// The margin datum itself, pinned two ways. It is the ENTIRE justification for 24m
	// over 10m, and an unpinned datum can be zeroed: at deliveryIdleFieldStall = 0 the
	// clause above is vacuously satisfied by any window, guard 3 in numeraire.go becomes
	// uint(default − default/4), and a later drop to 10m would pass everything here
	// except the bare literal below.
	//
	// (i) the VALUE, with its provenance.
	if deliveryIdleFieldStall != 1040*time.Second {
		t.Fatalf("deliveryIdleFieldStall = %v, want 1040 s = 17m20s — block 43 committed 17 min 20 s after block 42 on run c450985-deep "+
			"(evidence: integration/cloudtest/h43-stall-evidence-c450985-deep/README.md; the same figure is core/node's c2FieldStallObserved, "+
			"a separate literal in a package cmd/silt cannot import, so the two are held together by this pin and its twin there)",
			deliveryIdleFieldStall)
	}
	// (ii) the JOB the datum does: it must reject the tighter candidate. This is the arm
	// that reddens when the datum is zeroed, because zero rejects nothing.
	const tighter = 10 * time.Minute
	if tighter-tighter/deliveryIdleStampDivisor >= deliveryIdleFieldStall {
		t.Fatalf("the 10m candidate guarantees %v and deliveryIdleFieldStall is %v, so the datum no longer rejects it — 24m over 10m rests on this comparison and nothing else",
			tighter-tighter/deliveryIdleStampDivisor, deliveryIdleFieldStall)
	}
}

// G-C2-19 — the daemon's periodic sweep cadence, derived from the INSTALLED window.
// The interval must stay strictly positive at every window the daemon can accept: at the
// old 1 s floor `idle/2` was 500 ms and at anything sub-second it rounds toward zero,
// which panics time.NewTicker.
// UNGATED: R-DELIVERY-SWEEP-TICKER-UNFIRED — this pins the arithmetic, not the firing.
// No tier observes the goroutine in cmd/silt/daemon.go actually posting a sweep; at the
// shipped 24m window it fires at 12m, longer than any graded cloud flow lives.
func TestC2SweepIntervalIsHalfTheInstalledWindow(t *testing.T) {
	for _, w := range []time.Duration{deliveryIdleFloor, deliveryIdleDefault, time.Hour, 100 * time.Hour} {
		if got, want := deliverySweepInterval(w), w/2; got != want {
			t.Fatalf("deliverySweepInterval(%v) = %v, want %v", w, got, want)
		}
		if deliverySweepInterval(w) <= 0 {
			t.Fatalf("deliverySweepInterval(%v) is not positive — time.NewTicker panics", w)
		}
	}
	// The floor is what keeps this true: one nanosecond of window rounds to zero.
	if deliverySweepInterval(time.Nanosecond) > 0 {
		t.Fatal("fixture: a 1 ns window no longer rounds the sweep interval to zero, so the floor's ticker rationale has lost its subject")
	}
}

// G-C2-15 — the flag hands the node the DERIVED CONSTANT, not a literal that happens to
// match it today. This gate sees STRINGS ONLY: it reads daemon.go and checks the
// expression inside the fs.Duration call, so it can tell `deliveryIdleDefault` from
// `24*time.Minute` but cannot tell you what either evaluates to at run time.
// RUNTIME GATE: e2e TestDeliveryIdleWindowDefaultBootsThePaidLane — the daemon booted
// with -accept-delivery-receipts and NO -delivery-idle-window announces `idle window
// 24m0s` on its affordability line and is not refused.
func TestC2FlagDefaultIsTheDerivedConstant_Source(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := c2DefaultLiteral(src)
	if !ok {
		t.Fatal("SOURCE GATE: daemon.go no longer declares -delivery-idle-window with fs.Duration — the string this gate matches on is gone")
	}
	if got != "deliveryIdleDefault" {
		t.Fatalf("SOURCE GATE: the -delivery-idle-window default expression reads %q, want the identifier deliveryIdleDefault — a literal drifts from the derivation in cmd/silt/numeraire.go silently", got)
	}
}
