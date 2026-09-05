//go:build bbootstrap

package main

// D-BB-BUILD-TAG (ratified 2026-09-05), second half — INSIDE a tagged build, -bbootstrap
// gates the RECORDING, not just the publication.
//
// THE DEFECT THIS IS THE REGRESSION GATE FOR. cmd/silt/daemon.go called
// ledger.SetObservabilityClock UNCONDITIONALLY, and said so deliberately: the flag was
// documented as controlling "exactly ONE thing: publication", so that flipping it on at
// the next restart would find an already-stamped population. The consequence was that
// every default-flags silt node recorded (identity, cumulative bytes, first-seen
// wall-clock nanosecond) for every requester, in RAM, with no flag to disable it. That
// tuple did not exist before R2.9a.
//
// THE COST OF CLOSING IT, stated because it is a real loss: a tagged operator must now
// restart WITH -bbootstrap and then wait for the population to re-stamp. Arm B below is
// what that wait buys back, and it is the positive control that keeps arm A honest.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// bbSeamServer builds the fixture the DAEMON builds: it drives bbootstrapInject and
// bbootstrapWireUI with one `on` value, exactly as daemon.go does, instead of injecting
// the clock by hand the way r29aServer does. That difference is the whole test.
func bbSeamServer(t *testing.T, on bool) (*uiServer, *credit.Ledger, *r29aClock) {
	t.Helper()
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 } // nil func in this fixture; /api/status segfaults without it
	clk := &r29aClock{}
	bbootstrapInject(led, clk, on)
	bbootstrapWireUI(s, on)
	return s, led, clk
}

// TestR29aTheFlagGatesTheRecordingNotJustThePublication is the gate.
func TestR29aTheFlagGatesTheRecordingNotJustThePublication(t *testing.T) {
	// --- Arm A: the flag is OFF. Nothing is recorded. ---------------------------------
	s, led, clk := bbSeamServer(t, false)
	for i := 0; i < credit.BBootstrapMinRequesters; i++ {
		r29aFetch(led, i, 4096)
	}
	clk.now = ports.Time(bbStatusHour) // an hour of simulated time, had there been a clock

	h := led.BBootstrapPublish()
	if h.Requesters != credit.BBootstrapMinRequesters {
		t.Fatalf("arm A: requesters = %d, want %d — the fixture must actually produce a census, or every assertion below is vacuous", h.Requesters, credit.BBootstrapMinRequesters)
	}
	// The block must report itself correctly as having NO LIVE AGE AXIS. "Off" and
	// "idle" stay different objects: the census is still counted, it is simply not
	// age-conditioned, and a reader is told so rather than handed an all-zero age column.
	if h.ClockSource != "none" || h.AgeAxisLive {
		t.Fatalf("arm A: clockSource = %q, ageAxisLive = %v with -bbootstrap unset, want \"none\"/false — the flag must gate the INJECTION, so an unasked-for instrument has no clock to age against", h.ClockSource, h.AgeAxisLive)
	}
	if h.Cells != nil {
		t.Fatalf("arm A: cells published with no clock injected — an all-zero grid is indistinguishable from a genuinely young population")
	}
	// aged AND unstamped are both ZERO here, and that is the DEAD-CLOCK payload, not a
	// third failure: with no clock the snapshot counts a requester in the census and
	// places it in NEITHER age counter (BB-1 pins exactly this shape). `unstamped` is
	// the different, LIVE-clock reading — "the age axis works and this account predates
	// it" — which is what the mid-life flip in the next test produces. Asserting the
	// wrong one of those two is how this gate would go vacuous, so both are pinned.
	if h.Aged != 0 || h.Unstamped != 0 {
		t.Fatalf("arm A: aged = %d, unstamped = %d, want 0 / 0. With -bbootstrap unset no observability clock was injected, so Register wrote no first-touch time, there is no `when` on any identity, and the census is age-free rather than age-zero (D-BB-BUILD-TAG)", h.Aged, h.Unstamped)
	}
	if h.MaxOccupiedAgeEdgeNanos != 0 {
		t.Fatalf("arm A: maxOccupiedAgeEdgeNanos = %d with nothing stamped, want 0", h.MaxOccupiedAgeEdgeNanos)
	}
	// And nothing reaches the wire, because the renderer was never installed.
	if s.statusExtra != nil {
		t.Fatalf("arm A: uiServer.statusExtra was installed with -bbootstrap unset — there must be no path from /api/status to the census at all")
	}
	if body := bbRawStatus(t, s); strings.Contains(body, "bBootstrap") {
		t.Fatalf("arm A: GET /api/status carries a bBootstrap key with the flag unset: %s", body)
	}

	// --- Arm B: the flag is ON. The positive control. ---------------------------------
	// Without this arm, arm A would pass on a build where the instrument is simply
	// broken. This is what proves the difference is the FLAG.
	s2, led2, clk2 := bbSeamServer(t, true)
	for i := 0; i < credit.BBootstrapMinRequesters; i++ {
		r29aFetch(led2, i, 4096)
	}
	clk2.now = ports.Time(bbStatusHour)

	h2 := led2.BBootstrapPublish()
	if h2.ClockSource != "injected" || !h2.AgeAxisLive {
		t.Fatalf("arm B: clockSource = %q, ageAxisLive = %v with -bbootstrap SET, want \"injected\"/true — the positive control must show the flag doing something", h2.ClockSource, h2.AgeAxisLive)
	}
	if h2.MonotonicSource != "injected" {
		t.Fatalf("arm B: monotonicSource = %q, want \"injected\" — bbootstrapInject must still wire BOTH sources in one call (G-BB-4)", h2.MonotonicSource)
	}
	if h2.Aged != credit.BBootstrapMinRequesters || h2.Unstamped != 0 {
		t.Fatalf("arm B: aged = %d, unstamped = %d, want %d / 0", h2.Aged, h2.Unstamped, credit.BBootstrapMinRequesters)
	}
	if h2.Cells == nil {
		t.Fatalf("arm B: no cells with the age axis live")
	}
	if body := bbRawStatus(t, s2); !strings.Contains(body, "bBootstrap") {
		t.Fatalf("arm B: GET /api/status carries no bBootstrap key with the flag set: %s", body)
	}
}

// TestR29aFlippingTheFlagOnDoesNotRecoverThePastIsTheACCEPTEDCOST states the trade in a
// test rather than only in a comment, so that a future reader who thinks the old
// unconditional injection was better finds the property asserted, not merely described.
//
// An account registered while the flag was off carries NO stamp. Injecting the clock
// afterwards does not retroactively age it: it is counted in the census and reported as
// `unstamped`, and BBootstrapRunPrecondition VOIDS a run that carries any such account.
// So the operator's real recovery path is a restart plus a wait, which is what
// D-BB-BUILD-TAG accepts.
func TestR29aFlippingTheFlagOnDoesNotRecoverThePastIsTheACCEPTEDCOST(t *testing.T) {
	_, led, clk := bbSeamServer(t, false)
	for i := 0; i < credit.BBootstrapMinRequesters; i++ {
		r29aFetch(led, i, 4096)
	}
	// The operator now flips the flag on WITHOUT restarting — the case the old
	// unconditional injection existed to avoid.
	bbootstrapInject(led, clk, true)
	clk.now = ports.Time(bbStatusHour)

	h := led.BBootstrapPublish()
	if h.Unstamped != credit.BBootstrapMinRequesters {
		t.Fatalf("unstamped = %d, want %d — every account that predates the injection must be counted as unstamped, never dumped into age bucket 0, which would make them look brand new", h.Unstamped, credit.BBootstrapMinRequesters)
	}
	if h.Aged != 0 {
		t.Fatalf("aged = %d, want 0 — the past is not recoverable and must not be invented", h.Aged)
	}
	bad := credit.BBootstrapRunPrecondition(credit.BBootstrapHistogram{}, h, bbStatusHour)
	found := false
	for _, r := range bad {
		if strings.Contains(r, "unstamped requesters present") {
			found = true
		}
	}
	if !found {
		t.Fatalf("BBootstrapRunPrecondition did not void a run carrying unstamped requesters; it returned %v. The cost of gating the injection is only acceptable because the run refuses the half-stamped population instead of fitting it", bad)
	}
}

func bbRawStatus(t *testing.T, s *uiServer) string {
	t.Helper()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/status", nil)
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d, body %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}
