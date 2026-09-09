package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Tests for the #535 cold-auditor recovery-boundary policy and the pre-structure entry point
// WitnessValidateV5 (floorbox_v5.go), as D0 leaves them: the stall at an ambiguous recovery
// boundary is UNCONDITIONAL — there is no directive, no live-follower opt-in and no fall-through.
//
// EACH proof case is ABLATED — the defect it claims to catch is injected and watched to flip the
// outcome — per the standing "a check is not shipped until you have injected its defect and watched
// it go red" discipline (simplicity rule 7). With the knob deleted, the ablation for the boundary
// arms is necessarily the CONFIG, not a box-local flag: the same height is driven with and without
// LivenessRecoveryHeight pointing at it.
//
// The whole-class never-Accept suite lives in floorbox_coldauditor_v5_test.go; what is pinned here
// is the policy unit itself.

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

// TestWitnessValidateV5_RecoveryBoundaryStallsUnconditionally is D0's first deliverable at the
// scaffold entry: at an ambiguous recovery boundary the box emits a LOUD IndeterminateTrustlessly,
// and there is no input that changes that. The three paths (a') removed — a box-local directive for
// the height, the live-follower opt-in, the un-gated fall-through — are gone from the SIGNATURE, so
// their absence is a compile-time property and not something a test could still express.
//
// ABLATION: point LivenessRecoveryHeight at a different epoch boundary; the same height and the
// same block then proceed to the gated recompute seam. The boundary predicate is what drives the
// arm, not an unconditional stall on everything.
func TestWitnessValidateV5_RecoveryBoundaryStallsUnconditionally(t *testing.T) {
	const recoveryH = 8 // an epoch boundary (8 % 4 == 0) equal to LivenessRecoveryHeight
	c := floorBoxChain(t, recoveryH)

	got, reason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{})
	if got != IndeterminateTrustlessly {
		t.Fatalf("cold auditor at an ambiguous recovery boundary: got %s, want INDETERMINATE_TRUSTLESSLY", got)
	}
	if !errors.Is(reason, ErrRecoveryBoundaryStall) {
		t.Fatalf("cold-auditor stall reason: got %v, want ErrRecoveryBoundaryStall", reason)
	}

	// The stall is a property of the HEIGHT and the chain's own config, so it repeats: there is no
	// per-call input that could carry an authorization.
	for i := 0; i < 3; i++ {
		if _, again := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{0xAA}); !errors.Is(again, ErrRecoveryBoundaryStall) {
			t.Fatalf("call %d: the stall must not depend on any per-call input; got %v", i, again)
		}
	}

	// ABLATION: a chain whose recovery boundary is elsewhere does NOT stall this height.
	elsewhere := floorBoxChain(t, 4)
	if _, reason := elsewhere.WitnessValidateV5(v5Block(recoveryH), [32]byte{}); errors.Is(reason, ErrRecoveryBoundaryStall) {
		t.Fatalf("ablation: with LivenessRecoveryHeight=4 height %d must not stall on the boundary arm; got %v", recoveryH, reason)
	}
}

// TestWitnessValidateV5_NonBoundaryHeightNotAmbiguous proves the policy does NOT stall at a
// height that is NOT the recovery boundary: the qualification set there is the frozen,
// witnessable epochSet, so the box proceeds (no false indeterminate that would needlessly stall
// an honest box).
//
// ABLATION: setting LivenessRecoveryHeight to the tested height (making it the ambiguous
// boundary) flips the same height to a stall — proving the non-stall is because the height is
// NOT the configured recovery boundary, not an unconditional proceed.
func TestWitnessValidateV5_NonBoundaryHeightNotAmbiguous(t *testing.T) {
	const h = 8

	// LivenessRecoveryHeight = 4 (a different boundary), so h=8 is NOT ambiguous.
	c := floorBoxChain(t, 4)
	_, reason := c.WitnessValidateV5(v5Block(h), [32]byte{})
	if errors.Is(reason, ErrRecoveryBoundaryStall) {
		t.Fatalf("a non-recovery-boundary height must not stall on the #535 arm; got %v", reason)
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("a non-ambiguous height should proceed to the gated recompute seam; got %v", reason)
	}

	// ABLATION: make h itself the recovery boundary → now it stalls.
	c2 := floorBoxChain(t, h)
	_, reason2 := c2.WitnessValidateV5(v5Block(h), [32]byte{})
	if !errors.Is(reason2, ErrRecoveryBoundaryStall) {
		t.Fatalf("ablation: with LivenessRecoveryHeight=%d the same height must stall; got %v", h, reason2)
	}
}

// TestWitnessValidateV5_RecoveryHeightMustBeEpochBoundary proves isAmbiguousRecoveryBoundary
// mirrors the full-node gate EXACTLY: a LivenessRecoveryHeight that is NOT an epoch boundary
// never triggers the recovery branch (chain.go requires h%EpochBlocks==0), so the box does not
// treat it as ambiguous. A non-boundary recovery height is a config the full node itself would
// ignore, so the box must too. This is the STRICTER of the two forms H-1 unified on.
func TestWitnessValidateV5_RecoveryHeightMustBeEpochBoundary(t *testing.T) {
	const recoveryH = 5 // 5 % 4 != 0: NOT an epoch boundary, so the recovery branch never fires
	c := floorBoxChain(t, recoveryH)
	_, reason := c.WitnessValidateV5(v5Block(recoveryH), [32]byte{})
	if errors.Is(reason, ErrRecoveryBoundaryStall) {
		t.Fatalf("a non-epoch-boundary recovery height must not be treated as ambiguous; got %v", reason)
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("a non-boundary recovery height should proceed to the gated seam; got %v", reason)
	}

	// ABLATION: the same height on a chain whose EpochBlocks divides it IS ambiguous — so the
	// epoch-boundary conjunct, not the height alone, is what withheld the stall above.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 5, MatureValidators: 2, BondTTLBlocks: 4, LivenessRecoveryHeight: recoveryH}
	c2 := New(cfg, func(ports.NodeID) int64 { return 0 })
	c2.SetBondVerifier(objectiveVerify)
	if _, reason2 := c2.WitnessValidateV5(v5Block(recoveryH), [32]byte{}); !errors.Is(reason2, ErrRecoveryBoundaryStall) {
		t.Fatalf("ablation: with EpochBlocks=5 height 5 IS an epoch boundary and must stall; got %v", reason2)
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
		got, reason := c.WitnessValidateV5(b, [32]byte{})
		if got != Reject || !errors.Is(reason, ErrNotWitnessableVersion) {
			t.Fatalf("sub-v5 block (v%d): got %s / %v, want REJECT / ErrNotWitnessableVersion", v, got, reason)
		}
	}

	// ABLATION: a v5 block is NOT rejected on the version gate.
	got, reason := c.WitnessValidateV5(v5Block(1), [32]byte{})
	if got == Reject && errors.Is(reason, ErrNotWitnessableVersion) {
		t.Fatal("ablation: a v5 block must pass the version gate")
	}
	if !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("a v5 block should reach the gated recompute seam; got %s / %v", got, reason)
	}
}

// TestWitnessValidateV5_RefusesAPrunedBlock is H-3 at the SECOND exported box entry. A pruned
// block's Hash() short-circuits to a stored token bound to no struct field — StateRoot included
// (certification §2.4) — so nothing downstream can be anchored to it. The door already refused;
// this makes "the box refuses pruned blocks" true of the box rather than of one of its two
// functions.
//
// ABLATION: delete the b.IsPruned() clause from WitnessValidateV5 ⇒ the pruned block reaches the
// gated seam and this arm reports ErrRecomputeGated instead ⇒ RED.
func TestWitnessValidateV5_RefusesAPrunedBlock(t *testing.T) {
	c := floorBoxChain(t, 0)
	full := v5Block(3)

	// The honest twin: the SAME block unpruned reaches the gated seam, so the refusal below is the
	// pruning and not the block.
	if _, reason := c.WitnessValidateV5(full, [32]byte{}); !errors.Is(reason, ErrRecomputeGated) {
		t.Fatalf("NON-VACUITY BROKEN: the unpruned twin must reach the gated seam; got %v", reason)
	}

	got, reason := c.WitnessValidateV5(full.Prune(), [32]byte{})
	if got != IndeterminateTrustlessly || !errors.Is(reason, ErrPrunedBlockUnreproducible) {
		t.Fatalf("a pruned block must be refused (ErrPrunedBlockUnreproducible); got %s / %v", got, reason)
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
