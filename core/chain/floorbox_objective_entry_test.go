package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The box entry must assert objective, not merely that a verifier is wired.
//
// This is the precondition on freeze-manifest item 2 (retiring `slashedRoot` and
// `validatorsSeenRoot` from the v5 digest set five → three). The freeze manifest states it: "an
// explicit objective guard at the box entry that stalls loudly rather than silently taking the
// legacy matureNow branch." Until it lands, the leaf retirement is not verifies — and a leaf
// removal is FORMAT-deadline, so missing it means the box carries two grow-only whole-set folds
// for era-4's entire life.
//
// THE DIVERGENCE, verified at source rather than taken from the research:
//
//	objective = c.cfg.MinBond > 0 && c.verifyBond != nil (chain.go)
//
// But the box entry asserted only `c.verifyBond == nil`. So on a box with a WIRED verifier and
// cfg.MinBond == 0 the entry passes, and the maturity recompute runs the OBJECTIVE predicate —
// floorbox_recompute_maturity_v5.go documents that assumption in its own header, calling
// objective "true for any untrusted deployment" — while a full node at the SAME config takes
// matureNow's LEGACY branch, counting non-anchor validatorsSeen against MatureValidators. Two
// different verdicts from one config, silently. That is exactly the shape this gate names.
//
// RED-FIRST, and driven: with the entry asserting only `verifyBond == nil` this row FAILS. What it
// returned in the ablation is worth recording, because it bounds what this row proves — the class-A
// screen caught this particular fixture ("class-A screen requires objective mode"), exactly as the
// own parenthetical says it does. So this row drives THE ENTRY ASSERTION, which fires before any
// screen and therefore covers every path into the box; it does NOT by itself drive the maturity
// recompute's legacy/objective divergence on a POST-LATCH block, which is the shape §4.11 names as
// the live risk. The entry closes that path too — it returns before the recompute is reached — but
// a row that DRIVES the post-latch maturity case is owed and is filed, not claimed here. Asserting
// more than the fixture exercises is the failure mode this repo has a lint for.
func TestColdBoxWiredVerifierWithZeroMinBondStallsAtEntry(t *testing.T) {
	f := buildMidEpochJoiner(t)

	cfg := f.cfg
	cfg.MinBond = 0 // the second arm of objective, the one the entry did not cover

	// Built directly, NOT via coldBox: that helper asserts objective is true, which is
	// precisely the corner under test. rep returns a value that WOULD qualify, so if the
	// legacy branch is reached the box produces a wrong write-set rather than a
	// coincidental zero — the same discipline coldBox uses.
	box := New(cfg, func(ports.NodeID) int64 { return 1 << 30 })
	box.SetBondVerifier(objectiveVerify)

	// The fixture must actually sit in the uncovered corner, or the row proves
	// nothing: a WIRED verifier (coldBox calls SetBondVerifier) and objective FALSE.
	if box.verifyBond == nil {
		t.Fatalf("FIXTURE BROKEN: the verifier must be WIRED — the old entry assertion already " +
			"catches an unwired box, and that corner is covered by TestColdBox_UnwiredBondVerifierStallsAtEntry")
	}
	if box.objective() {
		t.Fatalf("FIXTURE BROKEN: objective() must be FALSE here (MinBond=0), or this row re-tests the happy path")
	}

	err := recomputeViaHead(box, f.prevRoot, f.honestRoot(t), f.b, f.witness(t, false))
	if !errors.Is(err, ErrRecomputeBoxWiring) {
		t.Fatalf("OPEN: a box with a WIRED verifier and MinBond=0 did not stall at the entry.\n"+
			"  objective() is FALSE, so a full node at this config takes matureNow's LEGACY branch, while the\n"+
			"  box's maturity recompute reproduces the OBJECTIVE branch unconditionally — a SILENT divergence,\n"+
			"  which is the replay shape the entry assertion exists to name. The entry must assert\n"+
			"  !objective(), not verifyBond == nil.\n"+
			"  got: %v", err)
	}
}
