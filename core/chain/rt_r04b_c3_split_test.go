package chain

// R0.4b C3 re-break — BREAK 1 (F1), THE TWO-TIER AGREEMENT GATE. Inversion of the red-team
// probe rt_c3b_split_test.go (RT-C3B-18 / 18b), which measured a consensus SPLIT in BOTH
// directions at every epoch turn carrying a committed issuer key:
//
//	box verdict      : <nil>                      (AGREES with a forged root)
//	full-node verdict: era-3 block StateRoot does not equal the recomputed post-apply root
//
// and, the other way,
//
//	an HONEST block carrying ZERO IssuerKeys is ACCEPTED by the full node and read by the
//	box as a FORGED ROOT.
//
// This gate drives the identical scenario through every validation tier and requires the
// tiers to AGREE. It is the entry-criteria gate for the R1.8 accept-flip: the split was
// latent only because the box never returns Accept.

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// tierVerdicts is one block's verdict at each validation tier the R1.8 flip spans.
type tierVerdicts struct {
	box      error // the O(payload) floor-box recompute — the R1.8 accept surface
	fullNode error // the LIVE full-node path (validateEra3Roots)
}

// D0 REDUCED THIS GATE FROM FOUR TIERS TO TWO, and the two that remain are the two that carry its
// invariant. There used to be a coldBox tier and a liveBox tier — Chain.WitnessValidateV5 with and
// without RecoveryDirective{LiveFollower: true} — because a box could be TOLD to proceed past an
// ambiguous recovery boundary. Owner call 2 deleted the directive, which collapsed those two into
// one; the PE's simplicity ruling then deleted the scaffold itself, which removed the last.
//
// Nothing this gate exists for is lost. Its invariant is the SPLIT invariant — the box's recompute
// may stall where the node decides, but must never agree with a block the node rejects, nor report a
// forged root for one the node accepts — and both directions live in the box/fullNode pair. The
// deleted tiers asserted only "the box did not return Accept", on a function with four returns and
// no Accept among them; that property is now held where a flip could actually happen, at the R1.8
// downgrade in (*Box).Validate, driven on real node-accepted blocks of every v5 class by
// TestColdAuditor_NeverAcceptsAnyV5BlockClass.

// eachTier runs one candidate block through every tier against the same fixture pre-state.
// The chain is cloned per tier so no tier's dry-run apply can leak into another's.
func eachTier(t *testing.T, f rotateFixture, b Block, committed ports.Hash) tierVerdicts {
	t.Helper()
	var v tierVerdicts
	v.box = recomputeViaHead(f.c, f.prevRoot, committed, b, f.witnessForBoundary(t, b))
	live := f.c.cloneForDryRun()
	blk := b
	v.fullNode = live.validateEra3Roots(&blk)
	return v
}

// assertTiersAgree is the invariant: the box may STALL (name a class it does not reproduce)
// where the full node decides, but it may never AGREE with a block the full node rejects, and
// it may never report a FORGED ROOT for a block the full node accepts. And the box must never
// Accept at all until R1.8 flips it.
func assertTiersAgree(t *testing.T, label string, v tierVerdicts) {
	t.Helper()
	switch {
	case v.box == nil && v.fullNode != nil:
		t.Fatalf("%s: SPLIT (wrong-accept direction) — the box AGREES (nil) with a block the "+
			"full node REJECTS: %v", label, v.fullNode)
	case v.box != nil && v.fullNode == nil && !errors.Is(v.box, ErrRecomputeStateRootScopeStall):
		t.Fatalf("%s: SPLIT (false-forgery direction) — the full node ACCEPTS the block and "+
			"the box reports %v, which is not an out-of-scope stall", label, v.box)
	}
}

// TestRTC3_EpochTurnHonestBlockDoesNotSplitTheTiers is RT-C3B-18b closed: the honest
// zero-registration block at the epoch turn, over a pre-state holding a committed issuer key.
func TestRTC3_EpochTurnHonestBlockDoesNotSplitTheTiers(t *testing.T) {
	for _, withReg := range []bool{false, true} {
		label := "no-committed-issuer-key"
		if withReg {
			label = "committed-issuer-key-in-pre-state"
		}
		t.Run(label, func(t *testing.T) {
			f := buildPruneFixture(t, withReg)
			b := f.boundaryBlock(nil)
			honest := f.applyAndCommittedRoot(t, b)
			hc := f.c.cloneForDryRun()
			hc.apply(b)
			lr := hc.LogRoot()
			b.StateRoot, b.LogRoot = &honest, &lr
			Sign(&b, f.proposer)

			v := eachTier(t, f, b, honest)
			if v.fullNode != nil {
				t.Fatalf("fixture: the full node must ACCEPT the honest block, got %v", v.fullNode)
			}
			if v.box != nil {
				t.Fatalf("the box read an HONEST full-node-accepted block as %v", v.box)
			}
			assertTiersAgree(t, label, v)
		})
	}
}

// TestRTC3_EpochTurnForgedRootDoesNotSplitTheTiers is RT-C3B-18 closed: the forged root — the
// post-state the OLD height-driven prune produced, with the committed issuerKeyCommit leaf
// deleted by a block that carries no registrations — must be rejected by BOTH tiers.
func TestRTC3_EpochTurnForgedRootDoesNotSplitTheTiers(t *testing.T) {
	f := buildPruneFixture(t, true)
	b := f.boundaryBlock(nil)

	forge := f.c.cloneForDryRun()
	forge.apply(b)
	delete(forge.issuerKeyCommit, 0) // the pre-fix prune's effect, now a forgery
	forged, err := forge.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatal(err)
	}
	honestClone := f.c.cloneForDryRun()
	honestClone.apply(b)
	lr := honestClone.LogRoot()
	b.StateRoot, b.LogRoot = &forged, &lr
	Sign(&b, f.proposer)

	v := eachTier(t, f, b, forged)
	if v.fullNode == nil {
		t.Fatalf("fixture: the full node must REJECT the forged root")
	}
	if v.box == nil {
		t.Fatalf("BREAK RT-C3B-18 REOPENED: the box AGREES with a forged root the full node "+
			"rejects (%v)", v.fullNode)
	}
	assertTiersAgree(t, "forged-pruned-root", v)
	t.Logf("both tiers reject: box=%v", v.box)
}

// TestRTC3_RegistrationCarryingBlockStallsRatherThanSplits keeps the other half honest. When
// the block DOES carry registrations the class is genuinely out of the box's scope, so the box
// must STALL BY NAME — not agree, and not report a forged root.
func TestRTC3_RegistrationCarryingBlockStallsRatherThanSplits(t *testing.T) {
	f := buildPruneFixture(t, true)
	b := f.boundaryBlock(nil)
	b.IssuerKeys = []IssuerKeyReg{
		SignIssuerKeyReg(f.proposer, f.c.blockEpoch(b.Height), ports.Hash{0x88}),
	}
	committed := f.applyAndCommittedRoot(t, b)
	hc := f.c.cloneForDryRun()
	hc.apply(b)
	lr := hc.LogRoot()
	b.StateRoot, b.LogRoot = &committed, &lr
	Sign(&b, f.proposer)

	v := eachTier(t, f, b, committed)
	if v.fullNode != nil {
		t.Fatalf("fixture: the full node must accept the honest registration block, got %v", v.fullNode)
	}
	if !errors.Is(v.box, ErrRecomputeStateRootScopeStall) {
		t.Fatalf("a registration-carrying block must stall the box BY NAME, got %v", v.box)
	}
	assertTiersAgree(t, "registration-carrying", v)
}
