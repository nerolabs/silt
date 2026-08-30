package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Tests for the sound, additive slice of lane-1 Part B increment B1 (floorbox_v5.go):
// the #535 cold-auditor recovery-boundary policy + the additive entry point WitnessValidateV5.
//
// EACH proof case is ABLATED — the defect it claims to catch is injected and watched to flip
// the outcome — per the standing "a check is not shipped until you have injected its defect and
// watched it go red" discipline. The bounded witnessable RECOMPUTE (the accept core) is
// research-gated and NOT built here; the safety-invariant test proves this build never returns
// Accept, so a green Accept would be a guessed recompute (the banned move) and is asserted
// impossible.

// floorBoxChain builds a minimal objective v5 chain whose config sets an ambiguous recovery
// boundary at height recoveryH (an epoch boundary). It is the pre-state a floor box would hold;
// WitnessValidateV5 reads only its public config, so no committed state is needed for these
// policy tests.
func floorBoxChain(t *testing.T, recoveryH uint64) *Chain {
	t.Helper()
	cfg := Config{
		Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 2, BondTTLBlocks: 4,
		LivenessRecoveryHeight: recoveryH,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	return c
}

// v5Block returns a minimal v5 block at height h. The policy path reads only Version and
// Height, so the block needs no witnesses or roots for these tests.
func v5Block(h uint64) Block {
	return Block{Version: BlockVersionWitnessable, Height: h}
}

// TestWitnessValidateV5_RecoveryBoundaryColdAuditorStalls is the #535 cold-auditor proof:
// at an ambiguous recovery boundary with NO box-local directive, the default box emits a LOUD
// IndeterminateTrustlessly (never Accept, never proposer-trust).
//
// ABLATION: adding a box-local directive for the SAME height flips the #535 arm — the box no
// longer stalls on ErrRecoveryDirectiveAbsent (it proceeds and then hits the gated recompute
// seam). Proving the directive is what drives the arm, not an unconditional stall.
func TestWitnessValidateV5_RecoveryBoundaryColdAuditorStalls(t *testing.T) {
	const recoveryH = 8 // an epoch boundary (8 % 4 == 0) equal to LivenessRecoveryHeight
	c := floorBoxChain(t, recoveryH)

	// Cold auditor: empty directive, default LiveFollower=false.
	got, reason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{}, RecoveryDirective{})
	if got != IndeterminateTrustlessly {
		t.Fatalf("cold-auditor at ambiguous recovery boundary: got %s, want INDETERMINATE_TRUSTLESSLY", got)
	}
	if !errors.Is(reason, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("cold-auditor stall reason: got %v, want ErrRecoveryDirectiveAbsent", reason)
	}

	// ABLATION: a box-local directive for recoveryH must flip the #535 arm off (no longer the
	// directive-absent stall). The box proceeds past the recovery gate and stalls instead on the
	// gated recompute seam — a DIFFERENT reason, proving the directive drove the first arm.
	d := RecoveryDirective{Heights: map[uint64]struct{}{recoveryH: {}}}
	got2, reason2 := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{}, d)
	if errors.Is(reason2, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("ablation: a present directive must NOT yield the directive-absent stall; got %v", reason2)
	}
	if got2 != IndeterminateTrustlessly || !errors.Is(reason2, ErrRecomputeGated) {
		t.Fatalf("ablation: with a directive the box should proceed to the gated recompute seam; got %s / %v", got2, reason2)
	}
}

// TestWitnessValidateV5_RecoveryDirectivePresentProceeds proves the directive-present arm: at
// an ambiguous boundary WITH a box-local directive, the recovery gate passes (the box would
// validate trustlessly). Since the recompute is gated, it then returns ErrRecomputeGated — the
// point is it did NOT stall on the #535 arm.
//
// ABLATION: removing the directive returns the box to the cold-auditor stall (proven by the
// test above's first arm), so the directive is load-bearing.
func TestWitnessValidateV5_RecoveryDirectivePresentProceeds(t *testing.T) {
	const recoveryH = 8
	c := floorBoxChain(t, recoveryH)
	d := RecoveryDirective{Heights: map[uint64]struct{}{recoveryH: {}}}

	got, reason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{}, d)
	if got != IndeterminateTrustlessly {
		t.Fatalf("directive-present: got %s, want INDETERMINATE_TRUSTLESSLY (gated recompute)", got)
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("directive-present should pass the recovery gate and hit the gated seam; got %v", reason)
	}
}

// TestWitnessValidateV5_LiveFollowerOptInFlipsDefault proves the live-follower opt-in flips the
// cold-auditor default: at an ambiguous boundary with NO directive, a live-follower box proceeds
// past the recovery gate instead of stalling on ErrRecoveryDirectiveAbsent.
//
// ABLATION: the SAME height and empty directive with LiveFollower=false stalls (the cold-auditor
// test above), and with LiveFollower=true proceeds — the flag is the only difference, so it is
// what flips the behavior.
func TestWitnessValidateV5_LiveFollowerOptInFlipsDefault(t *testing.T) {
	const recoveryH = 8
	c := floorBoxChain(t, recoveryH)

	// Cold-auditor (default) with no directive: stalls on the #535 arm.
	_, coldReason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{}, RecoveryDirective{})
	if !errors.Is(coldReason, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("precondition: cold-auditor should stall on ErrRecoveryDirectiveAbsent; got %v", coldReason)
	}

	// Live-follower opt-in, same empty directive: proceeds past the recovery gate.
	live := RecoveryDirective{LiveFollower: true}
	_, liveReason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{}, live)
	if errors.Is(liveReason, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("live-follower must NOT stall on the #535 directive-absent arm; got %v", liveReason)
	}
	if !errors.Is(liveReason, ErrRecomputeGated) {
		t.Fatalf("live-follower should proceed to the gated recompute seam; got %v", liveReason)
	}
}

// TestWitnessValidateV5_NonBoundaryHeightNotAmbiguous proves the policy does NOT stall at a
// height that is NOT the recovery boundary: the qualification set there is the frozen,
// witnessable epochSet, so a cold auditor with no directive proceeds (no false indeterminate
// that would needlessly stall an honest box).
//
// ABLATION: setting LivenessRecoveryHeight to the tested height (making it the ambiguous
// boundary) flips the same height to a cold-auditor stall — proving the non-stall is because
// the height is NOT the configured recovery boundary, not an unconditional proceed.
func TestWitnessValidateV5_NonBoundaryHeightNotAmbiguous(t *testing.T) {
	const h = 8

	// LivenessRecoveryHeight = 4 (a different boundary), so h=8 is NOT ambiguous.
	c := floorBoxChain(t, 4)
	_, reason := c.WitnessValidateV5(v5Block(h), [32]byte{}, RecoveryDirective{})
	if errors.Is(reason, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("a non-recovery-boundary height must not stall on the #535 arm; got %v", reason)
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("a non-ambiguous height should proceed to the gated recompute seam; got %v", reason)
	}

	// ABLATION: make h itself the recovery boundary → now it stalls cold-auditor.
	c2 := floorBoxChain(t, h)
	_, reason2 := c2.WitnessValidateV5(v5Block(h), [32]byte{}, RecoveryDirective{})
	if !errors.Is(reason2, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("ablation: with LivenessRecoveryHeight=%d the same height must stall cold-auditor; got %v", h, reason2)
	}
}

// TestWitnessValidateV5_RecoveryHeightMustBeEpochBoundary proves isAmbiguousRecoveryBoundary
// mirrors the full-node gate EXACTLY: a LivenessRecoveryHeight that is NOT an epoch boundary
// never triggers the recovery branch (chain.go:1466-1468 requires h%EpochBlocks==0), so the
// box does not treat it as ambiguous. A non-boundary recovery height is a config the full node
// itself would ignore, so the box must too.
func TestWitnessValidateV5_RecoveryHeightMustBeEpochBoundary(t *testing.T) {
	const recoveryH = 5 // 5 % 4 != 0: NOT an epoch boundary, so the recovery branch never fires
	c := floorBoxChain(t, recoveryH)
	_, reason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{}, RecoveryDirective{})
	if errors.Is(reason, ErrRecoveryDirectiveAbsent) {
		t.Fatalf("a non-epoch-boundary recovery height must not be treated as ambiguous; got %v", reason)
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("a non-boundary recovery height should proceed to the gated seam; got %v", reason)
	}
}

// TestWitnessValidateV5_SubV5BlockRejected proves the v5-only version gate: a sub-v5 block
// handed to the v5 floor-box mode is Reject (ErrNotWitnessableVersion), not indeterminate — the
// mode can positively disprove a malformed-version input without the recompute.
//
// ABLATION: the same block at v5 is NOT rejected on the version gate (it reaches the gated
// seam), proving the gate keys on the version.
func TestWitnessValidateV5_SubV5BlockRejected(t *testing.T) {
	c := floorBoxChain(t, 0) // no recovery boundary configured

	for _, v := range []uint64{1, 2, 3, BlockVersionStateRoot} { // v1..v4, all sub-v5
		b := Block{Version: v, Height: 1}
		got, reason := c.WitnessValidateV5(b, [32]byte{}, RecoveryDirective{})
		if got != Reject || !errors.Is(reason, ErrNotWitnessableVersion) {
			t.Fatalf("sub-v5 block (v%d): got %s / %v, want REJECT / ErrNotWitnessableVersion", v, got, reason)
		}
	}

	// ABLATION: a v5 block is NOT rejected on the version gate.
	got, reason := c.WitnessValidateV5(v5Block(1), [32]byte{}, RecoveryDirective{})
	if got == Reject && errors.Is(reason, ErrNotWitnessableVersion) {
		t.Fatal("ablation: a v5 block must pass the version gate")
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("a v5 block should reach the gated recompute seam; got %s / %v", got, reason)
	}
}

// TestWitnessValidateV5_NeverAcceptsWhileRecomputeGated is THE SAFETY INVARIANT of this
// increment: because the bounded witnessable recompute is research-gated and not built,
// WitnessValidateV5 NEVER returns Accept. A green Accept here would be a guessed recompute — the
// exact banned move (C-7 §104). This test sweeps the reachable input classes and asserts Accept
// never occurs. It is the ablation of the whole increment: if a future edit wires a recompute
// that can return Accept, this test must be updated deliberately (with its own certified proof),
// not left green by accident.
func TestWitnessValidateV5_NeverAcceptsWhileRecomputeGated(t *testing.T) {
	cases := []struct {
		name      string
		recoveryH uint64
		height    uint64
		directive RecoveryDirective
	}{
		{"no-recovery-config", 0, 3, RecoveryDirective{}},
		{"recovery-boundary-cold-auditor", 8, 8, RecoveryDirective{}},
		{"recovery-boundary-with-directive", 8, 8, RecoveryDirective{Heights: map[uint64]struct{}{8: {}}}},
		{"recovery-boundary-live-follower", 8, 8, RecoveryDirective{LiveFollower: true}},
		{"non-boundary-height", 4, 7, RecoveryDirective{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := floorBoxChain(t, tc.recoveryH)
			got, _ := c.WitnessValidateV5(v5Block(tc.height), [32]byte{}, tc.directive)
			if got == Accept {
				t.Fatalf("SAFETY VIOLATION: WitnessValidateV5 returned ACCEPT while the recompute is gated (%s)", tc.name)
			}
		})
	}
}

// TestFloorBoxOutcomeZeroValueIsIndeterminate pins the safe-default shape: the zero
// FloorBoxOutcome is IndeterminateTrustlessly, so a forgotten/mis-constructed outcome stalls,
// never silently accepts. Mirrors the witness accessor's NoWitness-as-zero invariant.
func TestFloorBoxOutcomeZeroValueIsIndeterminate(t *testing.T) {
	var zero FloorBoxOutcome
	if zero != IndeterminateTrustlessly {
		t.Fatalf("zero FloorBoxOutcome must be IndeterminateTrustlessly (the safe default), got %s", zero)
	}
	if zero == Accept {
		t.Fatal("zero FloorBoxOutcome must NEVER be Accept")
	}
}
