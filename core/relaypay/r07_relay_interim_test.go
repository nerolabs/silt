package relaypay

// R0.7 relay interim — RED-first gate G-RI-3 (Tester, 2026-09-03).
//
// Binding spec: RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §8 (RT-RELAY-3: "NOT closed by the anchor; it needs its own bound... Promote
// [Verifier.walkSteps] from instrumentation to an enforced budget"); red-team
// RED-TEAM-relay-lane-session-grant-and-byte-price-2026-09-03.md RT-RELAY-3
// ("each bogus pay = ~53 ms relay CPU from a ~48-byte wire message...
// replayable forever on one open session").
//
// NOTE ON NAMING: both tests below are named with a `Relay` prefix (rather
// than a bare `TestAdvanceTo...`) so they are caught by the run filter the
// task specified (`-run 'Relay|RI_|relay'`). Neither
// `TestAdvanceToClampsToChainLength` (the pre-existing #644 test) nor a bare
// `TestAdvanceTo...` name would match that filter — flagged in the report,
// not silently worked around.
//
// TWO DISTINCT CLAIMS, checked separately:
//
//  1. "A single claimedCount > S is refused BEFORE the walk." This is the
//     EXISTING #644 clamp (payword.go:220-224) and is ALREADY GREEN on main —
//     see TestAdvanceToClampsToChainLength (payword_test.go:139). This file
//     does not re-test it; TestRelayPayPerCallOverSClampAlreadyEnforced below
//     is a one-line pointer/witness, not a new property.
//
//  2. "The CUMULATIVE walk across an entire session is bounded by S, and once
//     spent, a further claim is refused BEFORE walking." This is what cert §8
//     asks to promote from instrumentation to enforcement, and it is NOT
//     implemented anywhere on main: nothing reads v.walkSteps except the
//     ablation test itself. A single open session accepts an UNBOUNDED number
//     of full-S bogus walks, each ~S hashes, forever — the live RT-RELAY-3
//     CPU-DoS. TestRelayPayAdvanceToCumulativeWalkBudgetEnforced is RED on
//     main.

import "testing"

// TestRelayPayPerCallOverSClampAlreadyEnforced is a witness, not a new gate:
// it confirms (independently of TestAdvanceToClampsToChainLength) that a
// SINGLE claimedCount > S is refused before any walk occurs on a fresh
// verifier. This half of G-RI-3 is ALREADY GREEN on main
// (payword.go:220-224) — see TestAdvanceToClampsToChainLength
// (payword_test.go:139-186) for the authoritative ablation-backed test. Kept
// here only so a reader of the R0.7 interim gates does not have to go hunting
// for which half is already closed.
func TestRelayPayPerCallOverSClampAlreadyEnforced(t *testing.T) {
	const S = 6
	tip := []byte("g-ri-3-witness-fresh-random-tip-32")[:32]
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	v := NewVerifier(c.Root(), S)

	bogus := make([]byte, hashLen)
	before := v.walkSteps
	if err := v.AdvanceTo(bogus, S+1); err == nil {
		t.Fatalf("AdvanceTo accepted claimedCount S+1=%d > S=%d", S+1, S)
	}
	if got := v.walkSteps; got != before {
		t.Fatalf("an over-S claim walked %d steps before refusing (want 0 delta) — payword.go:222-224's pre-walk clamp regressed", got-before)
	}
}

// TestRelayPayAdvanceToCumulativeWalkBudgetEnforced is G-RI-3's substantive
// half: the TOTAL hash-walk work a session's Verifier will ever perform is
// bounded by S (the committed chain length — an honest session's cumulative
// walk across all its calls is at most S, since count only ever moves
// forward and never past S). Once that budget is spent, a further AdvanceTo
// claim must be REFUSED BEFORE walking, not walk again and fail on the
// bytesEqual check afterward.
//
// TODAY (main): nothing enforces this. A bogus preimage claiming count=S
// always passes both live guards (claimedCount > v.count, since count never
// advances on a bytesEqual failure; claimedCount <= v.s, since S is not > S),
// so it walks the full S hashes every single call, forever, on one open
// session — the exact RT-RELAY-3 CPU-DoS the red-team measured
// (~53ms/bogus pay, replayable). RED.
//
// Ablation (once a fix lands): removing the cumulative-budget check must
// redden this test again (walkSteps grows past S on the second call).
func TestRelayPayAdvanceToCumulativeWalkBudgetEnforced(t *testing.T) {
	const S = 8
	tip := []byte("g-ri-3-cumulative-fresh-random-tip")[:32]
	c, err := BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	v := NewVerifier(c.Root(), S)

	// bogus never hashes to anything the chain produced, so bytesEqual always
	// fails and v.count never advances — the claim is replayable indefinitely
	// under today's code, each replay walking S hashes.
	bogus := make([]byte, hashLen)

	// First claim: an honest session's cumulative walk is at most S, so
	// spending the WHOLE budget on one (here: rejected) claim is in-budget.
	if err := v.AdvanceTo(bogus, S); err == nil {
		t.Fatalf("bogus preimage was accepted (count moved to %d)", v.Count())
	}
	afterFirst := v.walkSteps
	if afterFirst > uint64(S) {
		t.Fatalf("first bogus AdvanceTo walked %d steps, want <= S=%d", afterFirst, S)
	}
	if v.Count() != 0 {
		t.Fatalf("a rejected AdvanceTo moved count to %d, want 0", v.Count())
	}

	// Second claim on the SAME session: count is still 0 (nothing was ever
	// authorized), so this is functionally a REPLAY of the same bogus claim.
	// The session's cumulative walk budget (S) is already spent by the first
	// call, so this must be refused BEFORE walking — walkSteps must not move.
	err2 := v.AdvanceTo(bogus, S)
	if err2 == nil {
		t.Fatalf("a second bogus claim was accepted")
	}
	if got := v.walkSteps; got != afterFirst {
		t.Fatalf("RT-RELAY-3 cumulative walk budget is NOT enforced: a second bogus AdvanceTo on the same session walked AGAIN (walkSteps %d -> %d, session budget S=%d) instead of being refused before walking — Verifier.walkSteps is still instrumentation only, not the enforced per-session budget cert §8 asks for (unbounded CPU-DoS: red-team measured ~53ms/bogus pay, replayable forever on one open session)", afterFirst, got, S)
	}
}
