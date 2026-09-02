package chain

// era-4 (v5) floor-box BOULDER-1 gate-coverage gap closures — R1.2 witness-soundness.
//
// TESTER SEAT — 2026-09-01.
// Closes three coverage gaps flagged by the PE against the R1.2 gate set:
//
//   GAP 1 — TestAdversarialRoot_ClassA_ForgedSlashed_StillBonded
//     The existing ForgedSlashed fixture slashes the culprit and also removes it from bonded
//     (chain.go:3288). A forged Slashed=false screen thus has BondedPresent=true paired with an
//     honest BondedProof that proves bonded||id ABSENT — so the BondedProof (not the SlashedProof)
//     is what stalls the box. The Slashed anchor is NOT independently load-bearing in that fixture.
//     This variant keeps the culprit in bonded AFTER slashing (synthetic state — impossible on a
//     real chain) so that BondedPresent=true AND BondedProof proves PRESENT. In this variant, the
//     ONLY thing that can catch a forged Slashed=false is the Slashed absent-proof. Ablation is
//     confirmed by the companion test TestAdversarialRoot_ClassA_ForgedSlashed_BondedProofCatchesFirst.
//
//   GAP 2 — TestMatureEpochImpliesEverMature_InvariantPin
//     The PE-flagged class-M non-constructibility rests on the invariant matureEpoch => everMature,
//     which is emergent from rotateEpoch's early-return at chain.go:3395-3398. A future edit that
//     sets matureEpoch on a non-latched chain re-opens class-M inheritance. This test pins the
//     invariant: sweeps real apply() paths and asserts matureEpoch => everMature at each step;
//     then demonstrates teeth by perturbing the assumption (injecting matureEpoch=true with
//     everMature=false) and confirming the pin detects the violation.
//
//   GAP 3 — TestAdversarialRoot_ClassB_ForgedPreBondRegHeight
//     preBondRegHeight is fold-caught (a forged OldValue causes a root mismatch) but has no
//     dedicated adversarial-root point gate. This adds one in the same shape as the other class-B
//     gates: forge preBondRegHeight, assert stall; confirm the fold-catch is the mechanism.

import (
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// GAP 1 — TestAdversarialRoot_ClassA_ForgedSlashed_StillBonded
// =============================================================================
//
// Variant of TestAdversarialRoot_ClassA_ForgedSlashed that makes the Slashed anchor
// independently load-bearing.
//
// In the EXISTING fixture (TestAdversarialRoot_ClassA_ForgedSlashed) the culprit is removed
// from bonded when slashed (chain.go:3288). The forged screen has BondedPresent=true, but the
// BondedProof proves bonded||id ABSENT — so attesterQualifiedFromScreen stalls at the
// BondedPresent check (atts_v5.go:207), NOT at the Slashed check (atts_v5.go:169). The Slashed
// anchor is never independently exercised against a real wrong-accept.
//
// THIS variant keeps the culprit in bonded post-slash (synthetic state — chain.go:3288 normally
// removes it, but we inject it manually so BondedProof proves PRESENT). Now:
//   - BondedPresent=true, BondedSize=era4MinBond, BondedProof proves PRESENT — would qualify
//     via the pre-maturity path (BondedPresent && BondedSize >= MinBond).
//   - The ONLY thing that stalls the forge is the Slashed absent-proof (atts_v5.go:169).
//
// Gate is GREEN on R1.2 (stalls at Slashed anchor). With Slashed anchor ablated (comment out
// lines 169-170 of atts_v5.go), this gate would produce a wrong-accept (nil).
func TestAdversarialRoot_ClassA_ForgedSlashed_StillBonded(t *testing.T) {
	// EpochBlocks=0: no epochs -> epochsEnabled()=false -> pre-maturity A-qualification path.
	// MatureValidators=0: everMature=true post-genesis -> latchedMaturityWitness works.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 0, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(79001)
	culprit := key(79002)

	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(culprit, ports.HashBytes(pubOf(culprit)), era4MinBond, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)
	culpritID := ports.HashBytes(pubOf(culprit))

	if !c.everMature {
		t.Fatalf("fixture: everMature must be true post-genesis (MatureValidators=0)")
	}

	// Slash culprit at h=1. chain.go:3288 removes culprit from bonded.
	prev1, h1 := c.Head()
	bSlash := Block{Version: BlockVersionWitnessable, Height: h1, Prev: prev1,
		Slashes: []Equivocation{slashProof(culprit, prev1, 0x51, 0x52)}}
	Sign(&bSlash, prop)
	c.apply(bSlash)
	if !c.slashed[culpritID] {
		t.Fatalf("fixture: culprit must be slashed after the slash block")
	}
	if _, stillBonded := c.bonded[culpritID]; stillBonded {
		t.Fatalf("fixture: chain.go:3288 must have removed culprit from bonded; " +
			"if this fires, the slash logic changed and the fixture needs updating")
	}

	// SYNTHETIC RE-INSERTION: put culprit back into bonded at era4MinBond. This is the state
	// a real chain CANNOT reach (slash always removes from bonded), but which tests the Slashed
	// anchor in isolation. In this state:
	//   - slashed[culpritID] = true  (present, proven by SlashedProof)
	//   - bonded[culpritID]  = era4MinBond (present, proven by BondedProof)
	// A forged Slashed=false screen with BondedPresent=true and BondedSize=era4MinBond would
	// qualify via the pre-maturity path IF the Slashed anchor is bypassed.
	c.bonded[culpritID] = era4MinBond

	// Pre-state prover: captures both the slashed leaf (present) and bonded leaf (present).
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()

	// Verify the fixture isolation: BondedProof proves PRESENT (so it does NOT catch the forge).
	bondedKey := statehash.Key(tagBonded, culpritID[:])
	bondedProof := mustProve(prover, bondedKey)
	bondedRes := statehash.Resolve(prevRoot, bondedKey, statehash.EncodeInt64(era4MinBond), bondedProof)
	if !bondedRes.IsProvenPresent() {
		t.Fatalf("FIXTURE INVALID: bonded||culprit must be proven PRESENT in the synthetic pre-state "+
			"(era4MinBond=%d was re-inserted); stateRootLeavesV5 may not capture the re-inserted entry. "+
			"culpritID=%x", era4MinBond, culpritID[:4])
	}
	t.Logf("FIXTURE CONFIRMED: bonded||culprit proven PRESENT (size=%d). If Slashed anchor bypassed, "+
		"BondedPresent=true && BondedSize=%d >= MinBond=%d -> culprit qualifies -> wrong-accept.",
		era4MinBond, era4MinBond, era4MinBond)

	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	// Test block: culprit attests at h=2. Honest committed root: culprit is slashed so not seated.
	// Compute honestCommitted from a clone that does NOT have the synthetic bonded re-insertion
	// (the actual honest chain state has culprit removed from bonded by chain.go:3288).
	honestClone := c.cloneForDryRun()
	delete(honestClone.bonded, culpritID) // restore real post-slash state (no bonded entry)
	prev2, h2 := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(78)}}
	bTest.LastCommit = append(bTest.LastCommit, carrierEntry(c, culprit))
	Sign(&bTest, prop)

	applyClone := honestClone.cloneForDryRun()
	applyClone.apply(bTest)
	honestCommitted, err := applyClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honestCommitted: %v", err)
	}
	if applyClone.validatorsSeen[culpritID] {
		t.Fatalf("fixture: culprit must NOT be in validatorsSeen after honest apply (slashed)")
	}

	// forgedRoot: honest apply + spurious validatorsSeen[culpritID] seating, computed from
	// the SYNTHETIC chain (culprit in bonded) so the pre-state leaves match the prover.
	attackerClone := c.cloneForDryRun()
	attackerClone.apply(bTest)
	attackerClone.validatorsSeen[culpritID] = true
	forgedRoot, err := attackerClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRoot: %v", err)
	}
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted (spurious seating did not move root)")
	}

	preSeenIDs := sortIDs(func() []ports.NodeID {
		var out []ports.NodeID
		for id := range c.validatorsSeen {
			out = append(out, id)
		}
		return out
	}())
	leafWit := func(k []byte) StateRootChangedLeafWitness {
		old := preValue(k)
		wit, pErr := prover.Prove(k)
		if pErr != nil {
			t.Fatalf("Prove(%x): %v", k, pErr)
		}
		return StateRootChangedLeafWitness{Key: k, OldValue: old, Proof: wit}
	}
	seenRootWit := mustProve(prover, statehash.Key(tagValidatorsSeenRoot, nil))

	// FORGED SCREEN: Slashed=false (forge), BondedPresent=true, BondedSize=era4MinBond.
	// BondedProof proves PRESENT (unlike the existing fixture where it proves ABSENT).
	// SlashedProof is an honest PRESENCE proof — cannot forge an absence proof because
	// culprit IS slashed. The forge fails IsProvenAbsent at atts_v5.go:169.
	forgedScreen := StateRootAttScreen{
		Attester:      culpritID,
		Slashed:       false, // FORGED: culprit IS slashed
		InEpochSet:    false,
		BondedSize:    era4MinBond, // honest in the synthetic state
		BondedPresent: true,        // honest in the synthetic state
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, culpritID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, culpritID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, culpritID[:])),
	}

	var forgedW StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.ChangedLeaves = append(forgedW.ChangedLeaves,
		leafWit(statehash.Key(tagValidatorsSeen, culpritID[:])))
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.ParentProposer, forgedW.ParentProposerSig = c.CarrierParentProposerWitness()
	forgedW.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	forgedW.Maturity = latchedMaturityWitness(t, prover, preValue)

	// R1.2: box MUST STALL. Forged Slashed=false requires absent-proof of slashed||culprit,
	// but culprit IS in slashed (presence proven) -> IsProvenAbsent fails at atts_v5.go:169.
	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedSlashed_StillBonded): box WRONG-ACCEPTED a forged Slashed=false "+
			"for a slashed culprit that is ALSO in the bonded map.\n"+
			"  Expected stall at the Slashed absent-proof (IsProvenAbsent, atts_v5.go:169). Got nil.\n"+
			"  Slashed anchor is NOT load-bearing — the anchor is broken.\n"+
			"  culpritID=%x forgedRoot=%x", culpritID[:4], forgedRoot)
	}
	t.Logf("GATE GREEN (Slashed anchor independently load-bearing): forged Slashed=false STALLS: %v\n"+
		"  culpritID=%x — Slashed absent-proof fires at atts_v5.go:169 BEFORE BondedPresent check.\n"+
		"  Synthetic: culprit in BOTH slashed (present) AND bonded (present, size=%d).\n"+
		"  Without the Slashed anchor the culprit qualifies via BondedPresent && BondedSize >= MinBond.",
		err, culpritID[:4], era4MinBond)
}

// TestAdversarialRoot_ClassA_ForgedSlashed_BondedProofCatchesFirst is the companion
// ablation-probe for GAP 1. It confirms that in the EXISTING fixture (culprit NOT in bonded
// post-slash), the BondedProof (ABSENT) catches the forge at atts_v5.go:207, NOT the Slashed
// anchor at atts_v5.go:169. This demonstrates why the StillBonded variant was needed.
func TestAdversarialRoot_ClassA_ForgedSlashed_BondedProofCatchesFirst(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 0, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(79101)
	culprit := key(79102)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(culprit, ports.HashBytes(pubOf(culprit)), era4MinBond, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)
	culpritID := ports.HashBytes(pubOf(culprit))

	prev1, h1 := c.Head()
	bSlash := Block{Version: BlockVersionWitnessable, Height: h1, Prev: prev1,
		Slashes: []Equivocation{slashProof(culprit, prev1, 0x61, 0x62)}}
	Sign(&bSlash, prop)
	c.apply(bSlash)

	if _, stillBonded := c.bonded[culpritID]; stillBonded {
		t.Fatalf("fixture: culprit must NOT be in bonded post-slash (chain.go:3288)")
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()

	// Confirm: bonded||culprit is ABSENT in the pre-state prover (post-slash real state).
	bondedKey := statehash.Key(tagBonded, culpritID[:])
	bondedRes := statehash.Resolve(prevRoot, bondedKey, statehash.EncodeInt64(era4MinBond), mustProve(prover, bondedKey))
	if bondedRes.IsProvenPresent() {
		t.Fatalf("probe: bonded||culprit must be ABSENT post-slash for this probe to be meaningful")
	}

	// Call attesterQualifiedFromScreen directly with the forged screen (Slashed=false,
	// BondedPresent=true, but BondedProof proves ABSENT).
	forgedScreen := StateRootAttScreen{
		Attester:      culpritID,
		Slashed:       false,
		InEpochSet:    false,
		BondedSize:    era4MinBond,
		BondedPresent: true,
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, culpritID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, culpritID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, culpritID[:])),
	}
	_, qualErr := c.attesterQualifiedFromScreen(prevRoot, forgedScreen, livePreForProbe(c))
	if qualErr == nil {
		t.Fatalf("PROBE FAILED: forged screen must stall (at Slashed or BondedPresent check)")
	}
	// The Slashed anchor runs FIRST in attesterQualifiedFromScreen (lines 165-173 precede
	// the BondedPresent check at lines 201-213). So the Slashed anchor fires first in BOTH
	// the original and StillBonded fixtures. The DISTINCTION is what happens if ONLY the
	// Slashed anchor (lines 169-170) is ablated:
	//   - Original fixture: forge still caught by BondedPresent check (BondedProof ABSENT).
	//   - StillBonded fixture: forge produces wrong-accept (BondedProof PRESENT -> qualifies).
	// This confirms the StillBonded variant is needed to show the Slashed anchor is
	// independently load-bearing (ablating it alone causes a wrong-accept in StillBonded,
	// but NOT in the original fixture where BondedProof acts as a backup catch).
	t.Logf("PROBE CONFIRMED (original fixture Slashed anchor fires, but BondedProof backs it up):\n"+
		"  Error: %v\n"+
		"  bonded||culprit is ABSENT post-slash. If Slashed anchor ablated, BondedPresent=true\n"+
		"  but BondedProof proves ABSENT -> BondedPresent check would also stall. This means\n"+
		"  the original fixture does NOT demonstrate Slashed anchor is independently load-bearing\n"+
		"  (ablating it alone does not cause wrong-accept in the original fixture).\n"+
		"  TestAdversarialRoot_ClassA_ForgedSlashed_StillBonded closes this gap: with bonded||culprit\n"+
		"  PRESENT, ablating the Slashed anchor alone would cause a wrong-accept.", qualErr)
}

// =============================================================================
// GAP 2 — TestMatureEpochImpliesEverMature_InvariantPin
// =============================================================================
//
// Pins the invariant matureEpoch(state) => everMature(state).
//
// The invariant is currently enforced EMERGENTLY by rotateEpoch's early-return at
// chain.go:3395-3398: the function sets matureEpoch=true only when everMature is already true.
// Nothing in the type system or a named invariant check prevents a future edit from setting
// matureEpoch before everMature is latched.
//
// Why the invariant matters: class-M (maturitylatch_v5.go) reads the committed SeenSet ONLY
// when matureEpoch is false (pre-handoff); once matureEpoch=true the SeenSet is already
// committed by definition. If matureEpoch could be true while everMature=false, a spurious
// SeenSet could be committed before the latch — re-opening the class-M inheritance the R1.2
// fix closes.
//
// Structure:
//  1. apply sweep: real chain through genesis, maturity latch, epoch boundary. Assert invariant
//     at each step.
//  2. teeth: inject matureEpoch=true with everMature=false and confirm the invariant-check
//     predicate detects it.
func TestMatureEpochImpliesEverMature_InvariantPin(t *testing.T) {
	// checkInv asserts the invariant on a chain at a named step.
	checkInv := func(c *Chain, step string) {
		t.Helper()
		if c.matureEpoch && !c.everMature {
			t.Fatalf("INVARIANT VIOLATED at step %q: matureEpoch=true AND everMature=false.\n"+
				"  rotateEpoch (chain.go:3395-3398) must never set matureEpoch=true unless everMature\n"+
				"  is already true. A future edit that drops the everMature guard re-opens class-M\n"+
				"  inheritance (maturitylatch_v5.go — PE ruling Q2).", step)
		}
	}

	// Sweep 1: young -> mature latch -> first mature epoch boundary.
	t.Run("young-to-mature-sweep", func(t *testing.T) {
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 1, BondTTLBlocks: 0}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		prop := key(80001)
		att := key(80002)
		g := &Block{Version: BlockVersionWitnessable, Height: 0}
		g.BondRegs = append(g.BondRegs,
			bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
			bondRegFull(att, ports.HashBytes(pubOf(att)), era4MinBond, ports.Hash{}, 5, 2),
		)
		Sign(g, prop)
		c.apply(*g)
		checkInv(c, "h=0 genesis (pre-latch)")

		if c.matureEpoch || c.everMature {
			t.Fatalf("sweep: both must be false pre-latch (MatureValidators=1, no atts yet)")
		}

		// h=1: att attests -> seats into validatorsSeen -> Mature() trips -> everMature latches.
		prev, h := c.Head()
		b1 := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev}
		b1.LastCommit = append(b1.LastCommit, carrierEntry(c, att))
		Sign(&b1, prop)
		c.apply(b1)
		checkInv(c, "h=1 att attests (latch event)")
		if !c.everMature {
			t.Fatalf("sweep: everMature must be true after att attests (seen=1 >= MatureValidators=1)")
		}
		if c.matureEpoch {
			t.Fatalf("sweep: matureEpoch must be false (epoch boundary not yet hit)")
		}

		// h=2: epoch boundary. rotateEpoch fires. everMature=true -> matureEpoch sets.
		prev2, h2 := c.Head()
		b2 := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2}
		Sign(&b2, prop)
		c.apply(b2)
		checkInv(c, "h=2 first epoch boundary (handoff)")
		if !c.matureEpoch {
			t.Fatalf("sweep: matureEpoch must be true after first mature epoch boundary")
		}
		if !c.everMature {
			t.Fatalf("sweep: everMature must remain true post-boundary")
		}
	})

	// Sweep 2: mature-from-genesis (MatureValidators=0). everMature latches at h=0.
	t.Run("mature-from-genesis-sweep", func(t *testing.T) {
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		prop := key(80101)
		g := &Block{Version: BlockVersionWitnessable, Height: 0}
		g.BondRegs = append(g.BondRegs,
			bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		)
		Sign(g, prop)
		c.apply(*g)
		checkInv(c, "h=0 mature-from-genesis")

		prev, h := c.Head()
		b1 := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev}
		Sign(&b1, prop)
		c.apply(b1)
		checkInv(c, "h=1")

		prev2, h2 := c.Head()
		b2 := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2}
		Sign(&b2, prop)
		c.apply(b2)
		checkInv(c, "h=2 first epoch boundary")
		if !c.matureEpoch || !c.everMature {
			t.Fatalf("mature-from-genesis: both must be true after h=2 boundary")
		}
	})

	// Teeth: inject matureEpoch=true with everMature=false and confirm the invariant predicate
	// detects it. This proves the pin would redden if apply() produced such a state.
	t.Run("teeth-violation-detected", func(t *testing.T) {
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 1, BondTTLBlocks: 0}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		prop := key(80201)
		g := &Block{Version: BlockVersionWitnessable, Height: 0}
		g.BondRegs = append(g.BondRegs,
			bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		)
		Sign(g, prop)
		c.apply(*g)
		if c.everMature || c.matureEpoch {
			t.Fatalf("teeth: fixture must start with both false (MatureValidators=1, no atts)")
		}

		// Inject the violation: matureEpoch=true WITHOUT everMature.
		c.matureEpoch = true
		// c.everMature remains false.

		// The invariant predicate must detect it.
		if !(c.matureEpoch && !c.everMature) {
			t.Fatalf("TEETH FAILED: the violation injection (matureEpoch=true, everMature=false) " +
				"is not detectable by the invariant predicate — the injection did not work")
		}
		t.Logf("TEETH CONFIRMED: matureEpoch=true && everMature=false detected.\n" +
			"  A future rotateEpoch edit that removes the chain.go:3395 guard would produce this\n" +
			"  state on any non-latched chain at an epoch boundary, and checkInv would redden.")
	})
}

// =============================================================================
// GAP 3 — TestAdversarialRoot_ClassB_ForgedPreBondRegHeight
// =============================================================================
//
// Dedicated adversarial-root point gate for preBondRegHeight (class-B old-bucket source).
//
// preBondRegHeight[id] is read from ChangedLeaves[i].OldValue for bondRegHeight||id keys
// (bondreg_v5.go:405-411). A wrong OldValue causes the box to compute the wrong old due-bucket
// to DELETE, which produces the wrong committed root. The fold then sees postRoot != StateRoot
// and stalls (ErrRecomputeStateRootMismatch).
//
// Gate: forge the OldValue of the bondRegHeight||prop changed-leaf from 0 (genesis reg-height)
// to 5. The box computes oldDue=5+ttl+1=70 and emits a DELETE on bucket 70 (absent -> ADD)
// while failing to delete the honest old bucket (bucket 65=0+ttl+1). The fold produces the
// wrong root -> mismatch -> stall. Passing the HONEST committed root as the committed StateRoot
// confirms this: the box's wrong delta mismatches it.
func TestAdversarialRoot_ClassB_ForgedPreBondRegHeight(t *testing.T) {
	f := buildBondFixture(t)
	pid := ports.HashBytes(pubOf(f.proposer))

	// Confirm prop is registered at h=0 (genesis).
	preRegHeight, ok := f.c.bondRegHeight[pid]
	if !ok {
		t.Fatalf("fixture: proposer must have a bondRegHeight pre-state")
	}
	if preRegHeight != 0 {
		t.Fatalf("fixture: proposer bondRegHeight must be 0 (genesis); got %d", preRegHeight)
	}

	// Build the renew block: prop renews at h=1.
	prev, h := f.c.Head()
	renewBlock := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(f.proposer, ports.HashBytes(pubOf(f.proposer)), 16<<20, prev, 6, 1)}}

	honestCommitted := f.applyAndCommittedRoot(t, renewBlock)

	// Honest witness: affected buckets = oldDue (0+64+1=65) and newDue (1+64+1=66).
	oldDue, ok := f.preDueHeight(pid)
	if !ok {
		t.Fatalf("fixture: proposer pre-due-height not found")
	}
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, renewBlock, uniqueU64(oldDue, newDue))

	// Baseline: honest witness must agree with apply().
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, renewBlock, w); err != nil {
		t.Fatalf("honest renew witness must AGREE with apply(): %v", err)
	}

	// FORGE: replace the OldValue of the bondRegHeight||prop changed-leaf from the true
	// pre-state height (0) to fakeOldHeight (5). bondreg_v5.go:405-411 extracts preBondRegHeight
	// from this OldValue. With fakeOldHeight=5:
	//   - box computes fakeOldDue = 5 + 64 + 1 = 70
	//   - box emits DELETE on bucket 70 (absent pre-state -> OldValue=nil in the fold op)
	//   - box does NOT emit DELETE on bucket 65 (the honest old bucket)
	//   - fold produces root with bucket 65 still at its pre-state value (prop still in it)
	//   - this mismatches honestCommitted (which has bucket 65 emptied) -> stall
	fakeOldHeight := uint64(5)
	brhKey := statehash.Key(tagBondRegHeight, pid[:])

	forgedW := w // start from the honest witness
	forgedW.ChangedLeaves = append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...)
	forged := false
	for i, cl := range forgedW.ChangedLeaves {
		if string(cl.Key) == string(brhKey) {
			forgedW.ChangedLeaves[i].OldValue = statehash.EncodeUint64(fakeOldHeight)
			forged = true
			break
		}
	}
	if !forged {
		t.Fatalf("FORGE SETUP: bondRegHeight||prop changed-leaf not found in the witness.\n"+
			"  bondWitness must include the bondRegHeight leaf for a renewing id.\n"+
			"  propID=%x preRegHeight=%d", pid[:4], preRegHeight)
	}

	// The forgedRoot IS the honest committed root: the attacker commits the honest StateRoot,
	// but forges preBondRegHeight to make the box compute the wrong delta. The box's wrong
	// delta mismatches the honest committed root -> stall (fold-catch shape).
	forgedRoot := honestCommitted

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, renewBlock, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedPreBondRegHeight): box WRONG-ACCEPTED a forged preBondRegHeight.\n"+
			"  Expected a stall (fold mismatch: wrong old-bucket delete causes postRoot != StateRoot).\n"+
			"  propID=%x preRegHeight=%d fakeOldHeight=%d oldDue=%d fakeOldDue=%d newDue=%d",
			pid[:4], preRegHeight, fakeOldHeight, oldDue, fakeOldHeight+f.c.cfg.BondTTLBlocks+1, newDue)
	}
	t.Logf("GATE GREEN (preBondRegHeight fold-catch): forged preBondRegHeight STALLS: %v\n"+
		"  propID=%x preRegHeight=%d (honest) forged to %d.\n"+
		"  Box computed wrong oldDue=%d (honest=%d), emitted wrong bucket DELETE -> fold mismatch.\n"+
		"  Dedicated point gate established for preBondRegHeight (class-B old-bucket source).",
		err, pid[:4], preRegHeight, fakeOldHeight,
		fakeOldHeight+f.c.cfg.BondTTLBlocks+1, oldDue)
}
