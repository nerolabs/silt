package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The #535 cold-auditor recovery-boundary POLICY UNIT (floorbox_v5.go), as D0 leaves it: the stall
// at an ambiguous recovery boundary is UNCONDITIONAL — no directive, no live-follower opt-in, no
// fall-through — and the predicate it keys on is the STRICTER of the two forms that used to exist
// (H-1), so an honest box is not stalled at a height the full node's recovery branch would never
// take.
//
// THIS FILE TESTS THE PREDICATE AND THE DECISION DIRECTLY, not through an entry point. It used to
// drive Chain.WitnessValidateV5, the pre-structure scaffold, which D0 deleted: there is one door
// and it is (*Box).Validate. What the door does with this decision — including that it keys on the
// BOX'S OWN head height rather than the block's declared one — is
// TestColdAuditor_TheBoundaryPostureIsThePositionOfTheBoxNotTheClaimOfTheBlock's job. What is
// pinned here is the rule the door consults.
//
// EACH proof case is ABLATED — the defect it claims to catch is injected and watched to flip the
// outcome (simplicity rule 7). With the knob deleted, the only ablation available is the CONFIG:
// the same height is driven with and without LivenessRecoveryHeight pointing at it, and with and
// without an epoch cadence that divides it.

// floorBoxChain builds a minimal objective v5 chain whose config sets an ambiguous recovery
// boundary at height recoveryH (an epoch boundary). It is the pre-state a floor box would hold;
// the policy unit reads only this public config, so no committed state is needed.
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

// TestRecoveryBoundaryDecision_StallsUnconditionally is D0's first deliverable at the policy unit.
// At an ambiguous recovery boundary the decision is a stall, and there is no second argument that
// could change it: the three paths (a′) removed — a box-local directive for the height, the
// live-follower opt-in, the un-gated fall-through — are gone from the SIGNATURE, so their absence
// is a compile-time property rather than something a test still has to assert.
//
// ABLATION: point LivenessRecoveryHeight at a different epoch boundary; the same height then
// proceeds. The boundary predicate drives the arm, not an unconditional stall on everything.
func TestRecoveryBoundaryDecision_StallsUnconditionally(t *testing.T) {
	const recoveryH = 8 // an epoch boundary (8 % 4 == 0) equal to LivenessRecoveryHeight
	c := floorBoxChain(t, recoveryH)

	proceed, reason := c.recoveryBoundaryDecision(recoveryH)
	if proceed {
		t.Fatal("cold auditor at an ambiguous recovery boundary must NOT proceed")
	}
	if !errors.Is(reason, ErrRecoveryBoundaryStall) {
		t.Fatalf("stall reason: got %v, want ErrRecoveryBoundaryStall", reason)
	}

	// The decision is a pure function of the height and the chain's own config, so it repeats.
	for i := 0; i < 3; i++ {
		if p, again := c.recoveryBoundaryDecision(recoveryH); p || !errors.Is(again, ErrRecoveryBoundaryStall) {
			t.Fatalf("call %d: the stall must not depend on anything but the height and the config; got %v / %v", i, p, again)
		}
	}

	// ABLATION: a chain whose recovery boundary is elsewhere does NOT stall this height.
	if p, r := floorBoxChain(t, 4).recoveryBoundaryDecision(recoveryH); !p || r != nil {
		t.Fatalf("ablation: with LivenessRecoveryHeight=4 height %d must proceed; got %v / %v", recoveryH, p, r)
	}
}

// TestRecoveryBoundaryDecision_NonBoundaryHeightNotAmbiguous proves the policy does NOT stall at a
// height that is NOT the recovery boundary: the qualification set there is the frozen, witnessable
// epochSet, so the box proceeds. A false indeterminate would needlessly stall an honest box, and
// that is the liveness regression H-1 rejected the looser predicate to avoid.
//
// ABLATION: set LivenessRecoveryHeight to the tested height and it flips to a stall.
func TestRecoveryBoundaryDecision_NonBoundaryHeightNotAmbiguous(t *testing.T) {
	const h = 8
	if p, r := floorBoxChain(t, 4).recoveryBoundaryDecision(h); !p || r != nil {
		t.Fatalf("a non-recovery-boundary height must proceed; got %v / %v", p, r)
	}
	if p, r := floorBoxChain(t, h).recoveryBoundaryDecision(h); p || !errors.Is(r, ErrRecoveryBoundaryStall) {
		t.Fatalf("ablation: with LivenessRecoveryHeight=%d the same height must stall; got %v / %v", h, p, r)
	}
}

// TestIsAmbiguousRecoveryBoundary_IsTheStricterForm is H-1's gate. The box's predicate mirrors the
// full node's recovery branch EXACTLY — the height must equal LivenessRecoveryHeight AND epochs must
// be enabled AND EpochBlocks must be non-zero AND the height must be an epoch boundary — and D0
// folded rotateOps' looser copy onto it. A LivenessRecoveryHeight that is not an epoch boundary is a
// config the full node itself ignores, so the box must ignore it too; adopting the LOOSER form at
// both sites was the rejected direction, because it stalls at heights that are not boundaries.
//
// ABLATION: each conjunct dropped in turn flips exactly one row of the table below.
func TestIsAmbiguousRecoveryBoundary_IsTheStricterForm(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cfg      Config
		h        uint64
		ambiguos bool
	}{
		{"boundary height, epochs on, divides", Config{EpochBlocks: 4, LivenessRecoveryHeight: 8}, 8, true},
		{"height is not the configured one", Config{EpochBlocks: 4, LivenessRecoveryHeight: 8}, 4, false},
		{"no recovery height configured", Config{EpochBlocks: 4, LivenessRecoveryHeight: 0}, 8, false},
		{"height is not an epoch boundary", Config{EpochBlocks: 4, LivenessRecoveryHeight: 5}, 5, false},
		{"epochs disabled (EpochBlocks 0)", Config{EpochBlocks: 0, LivenessRecoveryHeight: 8}, 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.Quorum, cfg.MinBond, cfg.ByzantineQuorum, cfg.MatureValidators = 1, era4MinBond, true, 2
			c := New(cfg, func(ports.NodeID) int64 { return 0 })
			c.SetBondVerifier(objectiveVerify)
			if got := c.isAmbiguousRecoveryBoundary(tc.h); got != tc.ambiguos {
				t.Fatalf("isAmbiguousRecoveryBoundary(%d) = %v, want %v — the box's predicate must mirror the "+
					"full node's recovery branch exactly, or it stalls where the node would not (liveness) or "+
					"proceeds where the node re-bases (the ambiguity this policy governs)", tc.h, got, tc.ambiguos)
			}
		})
	}

	// The legacy fence rides on the same predicate: epochsEnabled() is EpochBlocks > 0 AND objective().
	lf := buildLegacyFixture(t)
	lf.c.cfg.EpochBlocks, lf.c.cfg.LivenessRecoveryHeight = 4, 8
	if lf.c.isAmbiguousRecoveryBoundary(8) {
		t.Fatal("a legacy (non-objective) chain has no epochs, so no height is an ambiguous recovery boundary")
	}
}

// TestFloorBox_SubV5BlockRejectedAtTheDoor is the v5-only version partition, driven where it now
// lives: composition step 0, reached through the ONE door. A sub-v5 block is Reject
// (ErrNotWitnessableVersion) and an above-era block is Reject (ErrAboveCurrentEraVersion) — both
// positive disproofs, distinct from a stall, and neither needs the recompute.
//
// ABLATION: a v5 block is NOT rejected on the version partition; it reaches the R1.8 downgrade.
// That is the honest twin, and it runs first.
func TestFloorBox_SubV5BlockRejectedAtTheDoor(t *testing.T) {
	f := buildStructFixture(t)
	src := newProverSource(t, f.c)
	box := boxOver(t, f, src)
	b := f.mkBlock(t, nil)
	w := structWitnessFor(t, f, src, b)
	assertBoxReachesTheDowngrade(t, box, b, w)

	for _, v := range []uint64{1, 2, 3, BlockVersionStateRoot} { // v1..v4, all sub-v5
		sub := b
		sub.Version = v
		sub.hashMemoSet = false
		got, reason := box.Validate(sub, w)
		if got != Reject || !errors.Is(reason, ErrNotWitnessableVersion) {
			t.Fatalf("sub-v5 block (v%d) at the door: got %s / %v, want REJECT / ErrNotWitnessableVersion", v, got, reason)
		}
	}
	above := b
	above.Version = BlockVersionWitnessable + 1
	above.hashMemoSet = false
	if got, reason := box.Validate(above, w); got != Reject || !errors.Is(reason, ErrAboveCurrentEraVersion) {
		t.Fatalf("above-era block at the door: got %s / %v, want REJECT / ErrAboveCurrentEraVersion", got, reason)
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
