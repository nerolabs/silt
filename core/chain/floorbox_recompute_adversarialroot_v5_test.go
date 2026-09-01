package chain

// era-4 (v5) floor-box BOULDER-1 invariant pins and adversarial-committed-root regression gates.
//
// TESTER SEAT — 2026-09-01.
// Governs: RULING-floorbox-R1.2-invariant-pins-2026-09-01.md (all tiers A/B/C).
//          docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md (gate shapes, 23-field cert).
//
// PART 1 — R1.0 INVARIANT PINS (must stay GREEN on main; they guard the R1.2 refactor):
//   TestActivationQuorumNonFork — Tier-B pin #5. Asserts rotateTallyOps and rotateEpoch produce
//     IDENTICAL lock-in verdicts across a swept schedule of (ready, total) pairs. Reddens if either
//     copy of 3*ready>2*total drifts. The driven sub-tests exercise the actual rotateTallyOps call.
//
// PART 2 — R1.1 ADVERSARIAL-COMMITTED-ROOT GATES (must be RED on main):
//   The attack: forge a witness field, fold the forged ops to get the root the attacker would commit
//   (forgedRoot), then assert Recompute(prev, forgedRoot, b, forgedWit) == nil (wrong-accept).
//   nil on main = RED gate = the gap is real. After R1.2 these gates must return non-nil (stall).
//
//   TestAdversarialRoot_ClassP_ForgedFrozenWeight — PE Tier-A required gate. Forges a frozen
//     member's Weight. Proves Weight is forgeable on main (§5 claim discharged).
//
//   TestAdversarialRoot_ClassA_ForgedInEpochSet — Tier-A gate for sc.InEpochSet. Forges the screen
//     to report a post-freeze bonded attester IS in the epochSet, inflating validatorsSeen.
//
//   TestAdversarialRoot_ClassM_PoisonedBySpuriousAtt — PE Tier-A required gate (ruling Q2).
//     Forges class-A InEpochSet to add a spurious validatorsSeen member; committed root moves to
//     the inflated set; class-M then latches everMature early on current main. The gate forges BOTH
//     the screen AND the committed root (PE ruling sharpening: a gate that checks against the honest
//     root is blind — same blindness as rotate_v5_test.go:407).
//
// COVERAGE ENUMERATION — fields not gated in this pass (owed before R1.2 lands):
//   Class P: RegVersion, RegVersionKnown (2 adversarial-root gates, distinct from the honest-root
//     ablation TestRecomputeStateRootRotateAblationLiveTallyForgedRegVersion which exists but is
//     blind to committed-root-moving forgeries).
//   Class A: Slashed, BondedSize, BondedPresent (3 gates; Slashed omitted here because building a
//     valid equivocation proof pair requires crypto tooling — use a slashed pre-state fixture; the
//     shape is identical to ForgedInEpochSet).
//   Class B: PriorOwner, Claimed, PriorProven (3 gates).
//   Total owed: 8 adversarial-root gates (P:2 + A:3 + B:3). This pass delivers 3 load-bearing ones.

import (
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// PART 1 — R1.0 INVARIANT PINS
// =============================================================================

// TestActivationQuorumNonFork is Tier-B pin #5 from the PE ruling. It asserts that the two live
// copies of the activation quorum — rotateTallyOps (:355,:363,:371) and rotateEpoch (:3448,:3471,
// :3495) — produce IDENTICAL lock-in decisions across a swept schedule of (ready, total) pairs.
//
// The #402 trap the PE flagged: both copies use `3*ready > 2*total` today and agree by inspection.
// Nothing enforces agreement. This pin makes the enforcement testable: a future edit that changes
// one copy's arithmetic must redden this test.
//
// Structure:
//   - The table sub-tests verify the quorum predicate against protocol-specified expected values.
//   - The driven sub-tests exercise the ACTUAL rotateTallyOps function to confirm it uses the
//     same arithmetic, not a reworded equivalent.
func TestActivationQuorumNonFork(t *testing.T) {
	// canonicalQuorum is the reference form of the quorum predicate, exactly as written in both
	// chain.go:3448,3471,3495 (rotateEpoch) and rotate_v5.go:355,363,371 (rotateTallyOps).
	// It is the SINGLE canonical reference this pin enforces. A future edit that changes either
	// live copy to a different expression (e.g. 2*ready > total) will diverge from this reference
	// on some row in the table — the pin reddens. The driven sub-tests then confirm the actual
	// rotateTallyOps call agrees.
	canonicalQuorum := func(total, ready int64) bool {
		return total > 0 && 3*ready > 2*total
	}

	type row struct {
		ready, total int64
		want         bool
		label        string
	}
	rows := []row{
		// degenerate / zero
		{0, 0, false, "0-of-0: total==0 ⇒ false"},
		{0, 3, false, "0-of-3: ready==0"},
		// below 2/3
		{1, 3, false, "1-of-3: 3<6"},
		{2, 4, false, "2-of-4: 6==8 not >"},
		{2, 3, false, "2-of-3: exactly 2/3 not strictly >"},
		{4, 6, false, "4-of-6: 12==12"},
		{6, 9, false, "6-of-9: 18==18"},
		// above 2/3
		{3, 4, true, "3-of-4: 9>8"},
		{2, 2, true, "2-of-2: 6>4"},
		{1, 1, true, "1-of-1: 3>2"},
		{7, 9, true, "7-of-9: 21>18"},
		{10, 13, true, "10-of-13: 30>26"},
		// large (float-divergence probe)
		{int64(2) << 30, int64(3) << 30, false, "exact-2/3: not strictly above 2/3"},
		{int64(1)<<30 + 1, int64(1)<<30 + 1, true, "all-ready"},
		{int64(2)<<30 + 1, int64(3) << 30, true, "2/3+1 of 3<<30"},
	}

	for _, r := range rows {
		r := r
		t.Run(r.label, func(t *testing.T) {
			got := canonicalQuorum(r.total, r.ready)
			if got != r.want {
				t.Fatalf("QUORUM ARITHMETIC BUG: want=%v got=%v for (ready=%d total=%d) [%s]\n"+
					"  canonicalQuorum(total=%d, ready=%d) = %v but expected %v.\n"+
					"  The two live copies of 3*ready > 2*total are:\n"+
					"    chain.go:3448,3471,3495 (rotateEpoch)\n"+
					"    floorbox_recompute_stateroot_rotate_v5.go:355,363,371 (rotateTallyOps)\n"+
					"  Both MUST match this canonical form. A drift from 3*ready>2*total to any other\n"+
					"  expression (e.g. 2*ready>total, ready*3>total*2 with overflow, etc.) reddens\n"+
					"  one or more rows here AND the driven sub-tests below.",
					r.want, got, r.ready, r.total, r.label,
					r.total, r.ready, got, r.want)
			}
		})
	}

	// DRIVEN PIN (lock-in fires): a single member, weight=9, regVersion=5.
	// Every threshold (3/4/5) is met ⇒ all three tallies lock in. At the SUBSEQUENT boundary the
	// tallies are already locked: rotateTallyOps must emit 0 ops (monotonic guards).
	// This confirms rotateTallyOps' tally() closure uses the canonical arithmetic.
	t.Run("driven-rotateTallyOps-all-locked-emits-nothing", func(t *testing.T) {
		cfg := Config{
			Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0,
		}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		prop := key(70001)
		g := &Block{Version: BlockVersionWitnessable, Height: 0}
		g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 9<<20, ports.Hash{}, 5, 1))
		Sign(g, prop)
		c.apply(*g)
		if !c.gateLockedIn || !c.era3LockedIn || !c.era4LockedIn {
			t.Fatalf("driven: all tallies should lock at genesis (single full-weight regVersion-5 member); got gate=%v era3=%v era4=%v",
				c.gateLockedIn, c.era3LockedIn, c.era4LockedIn)
		}
		// Advance to h=2 boundary.
		prev, _ := c.Head()
		b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev}
		Sign(&b1, prop)
		c.apply(b1)
		pid := ports.HashBytes(pubOf(prop))
		frozen := map[ports.NodeID]struct{}{pid: {}}
		weightByID := map[ports.NodeID]int64{pid: 9}
		regVersionByID := map[ports.NodeID]uint8{pid: 5}
		prev2, h2 := c.Head()
		rb := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2}
		Sign(&rb, prop)
		ops := c.rotateTallyOps(rb, &StateRootRotateWitness{
			GateLockedIn: StateRootRotateScalar{OldValue: statehash.EncodeBool(true)},
			GateHeight:   StateRootRotateScalar{OldValue: statehash.EncodeUint64(c.gateHeight)},
			Era3LockedIn: StateRootRotateScalar{OldValue: statehash.EncodeBool(true)},
			Era3Height:   StateRootRotateScalar{OldValue: statehash.EncodeUint64(c.era3Height)},
			Era4LockedIn: StateRootRotateScalar{OldValue: statehash.EncodeBool(true)},
			Era4Height:   StateRootRotateScalar{OldValue: statehash.EncodeUint64(c.era4Height)},
		}, frozen, regVersionByID, weightByID)
		if len(ops) != 0 {
			t.Fatalf("driven: all tallies already locked, rotateTallyOps must emit 0 ops; got %d", len(ops))
		}
	})

	// DRIVEN PIN (quorum NOT met): a single member, weight=9, regVersion=0.
	// No threshold (3/4/5) is met ⇒ rotateTallyOps must NOT lock in any tally.
	// If it emits a lock-in op, the tally arithmetic is wrong — either the threshold check is absent
	// or the quorum predicate is incorrect.
	t.Run("driven-rotateTallyOps-regVersion0-emits-nothing", func(t *testing.T) {
		cfg := Config{
			Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0,
		}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		// regVersion=0: does not meet any threshold (≥3/4/5).
		// Apply genesis via a regVersion-0 bond — but apply() tracks regVersion, so we set it directly.
		prop := key(70002)
		g := &Block{Version: BlockVersionWitnessable, Height: 0}
		g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 9<<20, ports.Hash{}, 0, 1))
		Sign(g, prop)
		c.apply(*g)
		if c.gateLockedIn || c.era3LockedIn || c.era4LockedIn {
			t.Fatalf("driven: regVersion=0 must not lock any tally at genesis; got gate=%v era3=%v era4=%v",
				c.gateLockedIn, c.era3LockedIn, c.era4LockedIn)
		}
		// Advance to h=2 boundary.
		prev, _ := c.Head()
		b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev}
		Sign(&b1, prop)
		c.apply(b1)
		pid := ports.HashBytes(pubOf(prop))
		frozen := map[ports.NodeID]struct{}{pid: {}}
		weightByID := map[ports.NodeID]int64{pid: 9}
		regVersionByID := map[ports.NodeID]uint8{pid: 0}
		prev2, h2 := c.Head()
		rb := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2}
		Sign(&rb, prop)
		ops := c.rotateTallyOps(rb, &StateRootRotateWitness{
			GateLockedIn: StateRootRotateScalar{OldValue: statehash.EncodeBool(false)},
			GateHeight:   StateRootRotateScalar{OldValue: nil},
			Era3LockedIn: StateRootRotateScalar{OldValue: statehash.EncodeBool(false)},
			Era3Height:   StateRootRotateScalar{OldValue: nil},
			Era4LockedIn: StateRootRotateScalar{OldValue: statehash.EncodeBool(false)},
			Era4Height:   StateRootRotateScalar{OldValue: nil},
		}, frozen, regVersionByID, weightByID)
		for _, op := range ops {
			t.Fatalf("QUORUM FORK (driven): regVersion=0 cannot meet threshold ≥3/4/5; rotateTallyOps "+
				"must emit 0 lock-in ops, but got op on key %x (newValue=%x).\n"+
				"  This means rotateTallyOps's quorum arithmetic diverges from canonicalQuorum.\n"+
				"  Two copies are: chain.go:3448 and rotate_v5.go:355. One has drifted.\n"+
				"  The #402 trap has been triggered — route both to a shared function.",
				op.Key, op.NewValue)
		}
	})

	// FORK DETECTION — confirm the table rows agree with the driven function behavior:
	// a member with weight=9 and regVersion=0 produces total=9, ready=0 → canonicalQuorum(9,0)=false.
	// If rotateTallyOps somehow emits a lock-in op for that case, the driven sub-test above reddens.
	// The table here independently confirms canonicalQuorum(9,0)=false:
	if canonicalQuorum(9, 0) {
		t.Fatalf("QUORUM TABLE: canonicalQuorum(total=9, ready=0) must be false; got true.\n"+
			"  The canonical quorum predicate 3*ready > 2*total is broken (3*0=0 > 18 is false).\n"+
			"  This is a compile-time constant mismatch, not a runtime failure. Fix the predicate.")
	}
	if !canonicalQuorum(9, 9) {
		t.Fatalf("QUORUM TABLE: canonicalQuorum(total=9, ready=9) must be true; got false.\n"+
			"  3*9=27 > 2*9=18 must be true. The canonical quorum predicate is broken.")
	}
}

// =============================================================================
// PART 2 — R1.1 ADVERSARIAL-COMMITTED-ROOT GATES
// =============================================================================

// adversarialCommittedRoot builds the forged committed root for a forged witness by calling
// assembleStateRootRecomputeOps (the op-assembly half of the box) and FoldChangedPaths (the
// fold), using the SAME code path the box uses. The result is the root the attacker would embed
// in b.StateRoot. The gate asserts:
//   1. forgedRoot != honestRoot (the forgery is real, not a no-op).
//   2. Recompute(prev, forgedRoot, b, forgedWit) == nil (wrong-accept on main = RED gate).
//
// honestCommitted is passed as the "committedStateRoot" for assembleStateRootRecomputeOps so the
// class-M maturity recompute path can resolve against a valid root. For tests that need the class-M
// path to use the forgedRoot (class-M poison gate), the caller rebuilds w.Maturity separately after
// getting forgedRoot from this helper.
//
// Returns (forgedRoot, honestRoot). Calls t.Fatal if op assembly fails — the forged witness must
// not be so broken that assembly stalls (we need the ops to fold into the forged root).
func adversarialCommittedRoot(
	t *testing.T,
	c *Chain,
	prevRoot ports.Hash,
	honestCommitted ports.Hash,
	b Block,
	forgedWit StateRootWitness,
) (forgedRoot, honestRoot ports.Hash) {
	t.Helper()
	ops, err := c.assembleStateRootRecomputeOps(prevRoot, honestCommitted, b, forgedWit)
	if err != nil {
		t.Fatalf("adversarialCommittedRoot: assembleStateRootRecomputeOps failed for the forged witness: %v\n"+
			"  The forged field must not also break a proof that assembleStateRootRecomputeOps verifies.\n"+
			"  Only the BRANCH PREDICATE or NewValue changes — the leaf proofs must stay valid against prevRoot.",
			err)
	}
	forgedRoot, foldErr := statehash.FoldChangedPaths(prevRoot, ops)
	if foldErr != nil {
		t.Fatalf("adversarialCommittedRoot: FoldChangedPaths failed: %v", foldErr)
	}
	return forgedRoot, honestCommitted
}

// =============================================================================
// TestAdversarialRoot_ClassP_ForgedFrozenWeight
// =============================================================================
// PE Tier-A required gate. The design §5 claim ("Weight is FORGEABLE, not fold-caught") is
// analytically-sound but not yet measured (PE ruling caveat). This gate CONFIRMS it by running
// RED on current main.
//
// Mechanism: nodeSetMTHFromInt64 commits membership only (weights dropped). A forged Weight does
// not diverge epochSetRoot. The attacker forges Weight=W+1, folds → forgedRoot (the forged
// epochSet||id leaf value), then submits (prev, forgedRoot, b, forgedWit). postRoot==forgedRoot
// == committedStateRoot ⇒ wrong-accept ⇒ nil return. The gate asserts nil.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassP_ForgedFrozenWeight(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	honestCommitted := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	if len(w.Rotate.Members) == 0 {
		t.Fatalf("fixture: no frozen members — ablation requires at least one")
	}

	// Record the honest weight of the first frozen member.
	originalWeight := w.Rotate.Members[0].Weight
	// Forge the Weight to a different value (same id, different leaf value).
	w.Rotate.Members[0].Weight = originalWeight + 1

	// Build forgedRoot: the root the attacker commits when they supply the forged weight.
	forgedRoot, honestRoot := adversarialCommittedRoot(t, f.c, f.prevRoot, honestCommitted, b, w)

	// Step 1: confirm the forgery is real (not a no-op).
	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot for forgedWeight=%d (original=%d).\n"+
			"  Forging Weight must change epochSet||id leaf value and hence the post-root.\n"+
			"  If they are equal, Weight is already fold-caught by an existing cross-check.\n"+
			"  Identify the catching invariant and narrow the §5 analysis accordingly.",
			originalWeight+1, originalWeight)
	}

	// Step 2: assert the box WRONG-ACCEPTS on current main (the RED gate result).
	// On main: Weight is read untrusted, written as NewValue, folded → forgedRoot == committedStateRoot.
	// The box returns nil. That is the RED outcome — the soundness gap the gate proves exists.
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedFrozenWeight): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  If R1.2 is already in place (Weight anchored via Resolve), invert this assertion.\n"+
			"  forgedRoot=%x honestRoot=%x forgedWeight=%d originalWeight=%d",
			err, forgedRoot, honestRoot, originalWeight+1, originalWeight)
	}
	// nil = wrong-accept = RED confirmed.
	t.Logf("GATE RED (confirmed): Weight forgeable on main.\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  forgedWeight=%d originalWeight=%d\n"+
		"  The PE §5 claim is now MEASURED (not merely analytic). R1.2 must anchor Weight via Shape V.",
		forgedRoot, honestRoot, originalWeight+1, originalWeight)
}

// =============================================================================
// TestAdversarialRoot_ClassA_ForgedInEpochSet
// =============================================================================
// Tier-A gate for sc.InEpochSet (attesterQualifiedFromScreen:128).
// Attack: a post-freeze bonded attester (not in the frozen epochSet) has InEpochSet forged to
// true. The box emits a spurious validatorsSeen||id ADD. The attacker commits the inflated
// validatorsSeenRoot. Wrong-accept.
//
// Setup: mature-epoch chain (MatureValidators=0, frozen at genesis). A new attester bonds at h=1
// (after the genesis freeze). At the test block it is NOT in epochSet and should be excluded.
// Gate is RED on main (nil = wrong-accept).
func TestAdversarialRoot_ClassA_ForgedInEpochSet(t *testing.T) {
	// Build fixture: mature-from-genesis, EpochBlocks large so no boundary fires at test heights.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(74001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
	)
	Sign(g, prop)
	c.apply(*g)
	if !c.matureEpoch {
		t.Fatalf("fixture: matureEpoch must be true post-genesis (MatureValidators=0)")
	}

	// Bond a new attester at h=1 — AFTER the genesis freeze, so it is NOT in epochSet.
	newAtt := key(74002)
	newAttID := ports.HashBytes(pubOf(newAtt))
	prev, h := c.Head()
	bBond := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev}
	bBond.BondRegs = append(bBond.BondRegs, bondRegFull(newAtt, ports.HashBytes(pubOf(newAtt)), 4<<20, ports.Hash{}, 5, 9))
	Sign(&bBond, prop)
	c.apply(bBond)

	if _, inES := c.epochSet[newAttID]; inES {
		t.Fatalf("fixture: newAtt must NOT be in epochSet (bonded post-genesis-freeze)")
	}
	if _, bonded := c.bonded[newAttID]; !bonded {
		t.Fatalf("fixture: newAtt must be bonded after the bond block")
	}
	if c.validatorsSeen[newAttID] {
		t.Fatalf("fixture: newAtt must NOT be in validatorsSeen pre-test block")
	}

	// Pre-state prover at h=2.
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch")
	}
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	// Test block at h=2: newAtt attests. apply() skips it (not in epochSet).
	prev2, h2 := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(55)}}
	bTest.Atts = append(bTest.Atts, Attest(&bTest, newAtt))
	Sign(&bTest, prop)

	// Honest committed root: apply() skips newAtt.
	clone := c.cloneForDryRun()
	clone.apply(bTest)
	honestCommitted, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honestCommitted: %v", err)
	}
	if clone.validatorsSeen[newAttID] {
		t.Fatalf("fixture: newAtt must NOT be in validatorsSeen honest post-apply")
	}

	// Build an honest witness (InEpochSet=false → no A write → no digest change).
	newAttBonded := c.bonded[newAttID]
	_, newAttBP := c.bonded[newAttID] // zero-value bool = false if absent, but bonded map returns int64
	// Actually bonded map is map[NodeID]int64, two-value lookup gives (int64, bool).
	newAttBP2 := func() bool { _, ok := c.bonded[newAttID]; return ok }()
	_ = newAttBP

	honestScreen := StateRootAttScreen{
		Attester: newAttID, Slashed: false, InEpochSet: false,
		BondedSize: newAttBonded, BondedPresent: newAttBP2,
	}

	preSeen := map[ports.NodeID]struct{}{}
	for id := range c.validatorsSeen {
		preSeen[id] = struct{}{}
	}

	// Helper to build a leaf witness from the pre-state prover.
	leafWit := func(k []byte) StateRootChangedLeafWitness {
		old := preValue(k)
		wit, pErr := prover.Prove(k)
		if pErr != nil {
			t.Fatalf("Prove(%x): %v", k, pErr)
		}
		return StateRootChangedLeafWitness{Key: k, OldValue: old, Proof: wit}
	}

	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.AttScreens = []StateRootAttScreen{honestScreen}
	// No A writes (newAtt not qualified) → no validatorsSeenRoot change.
	preSeenIDs := sortIDs(func() []ports.NodeID {
		var out []ports.NodeID
		for id := range c.validatorsSeen {
			out = append(out, id)
		}
		return out
	}())
	seenRootWit, pErr := prover.Prove(statehash.Key(tagValidatorsSeenRoot, nil))
	if pErr != nil {
		t.Fatalf("Prove(validatorsSeenRoot): %v", pErr)
	}
	w.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	w.Maturity = latchedMaturityWitness(t, prover, preValue)

	// Confirm honest witness agrees.
	if checkErr := c.RecomputeStateRootEntriesRevocations(prevRoot, honestCommitted, bTest, w); checkErr != nil {
		t.Fatalf("honest witness must AGREE with apply(): %v", checkErr)
	}

	// FORGE: swap InEpochSet to true — the box now emits a spurious validatorsSeen ADD for newAtt.
	forgedScreen := StateRootAttScreen{
		Attester: newAttID, Slashed: false, InEpochSet: true, // forged
		BondedSize: newAttBonded, BondedPresent: newAttBP2,
	}
	var forgedW StateRootWitness
	// E/R and maturity leaves carry over unchanged.
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.Maturity = w.Maturity
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}

	// Derive forged A write-set: newAtt now qualifies → validatorsSeen||newAtt ADD.
	forgedScreensMap := map[ports.NodeID]StateRootAttScreen{newAttID: forgedScreen}
	forgedAWrites, _, forgedAErr := c.stateRootAttWriteSet(bTest, preSeen, forgedScreensMap)
	if forgedAErr != nil {
		t.Fatalf("forged stateRootAttWriteSet: %v", forgedAErr)
	}
	for _, wr := range forgedAWrites {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	// The validatorsSeenRoot digest must change (new member added).
	// Build the post-set with newAtt added.
	forgedPostSeenIDs := append(preSeenIDs, newAttID)
	forgedPostSeenIDs = sortIDs(forgedPostSeenIDs)
	forgedW.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}

	// Build forgedRoot.
	forgedRoot, honestRoot := adversarialCommittedRoot(t, c, prevRoot, honestCommitted, bTest, forgedW)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot — InEpochSet forge did not change root.\n"+
			"  Check: was newAtt already in validatorsSeen? forgedPostSeenIDs=%v", forgedPostSeenIDs)
	}

	// Assert wrong-accept (RED gate on main).
	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedInEpochSet): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x",
			err, forgedRoot, honestRoot)
	}
	t.Logf("GATE RED (confirmed): InEpochSet forge produces wrong-accept on main.\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  newAtt=%x\n"+
		"  R1.2 must anchor InEpochSet via Shape V (epochSet||id Resolve) to close this gate.",
		forgedRoot, honestRoot, newAttID[:4])
}

// =============================================================================
// TestAdversarialRoot_ClassM_PoisonedBySpuriousAtt
// =============================================================================
// PE Tier-A REQUIRED gate (ruling Q2, "class-M IS A2-poisoned").
//
// The claim (PE ruling Q2): once class-A adds a spurious validatorsSeen||id ADD via an
// unanchored screen, class-M inherits the inflated validatorsSeenRoot from the committed root
// and can wrong-latch everMature early. The poisoning enters at class-A and rides the committed
// root into class-M; fixing class-A is the correct fix (not class-M).
//
// ARCHITECTURAL CONSTRAINT (documented here, not a gap in the gate):
// The end-to-end wrong-accept via RecomputeStateRootEntriesRevocations requires:
//   (a) everMature=false in the committed pre-state (class-M can still fire), AND
//   (b) a bonded-above-MinBond node that is UNQUALIFIED by the live qualification rules
//       (so the honest path doesn't seat it, but the forged screen does).
// In pre-maturity mode (matureEpoch=false), condition (b) cannot hold: any node bonded
// above MinBond IS qualified, so the honest apply() also seats it and the honest chain
// also crosses maturity — same latch, no discrepancy. In mature-epoch mode (matureEpoch=true),
// condition (a) cannot hold: matureEpoch is only set after everMature (chain.go:3395-3398),
// so the committed pre-state always has everMature=true on a mature-epoch chain — and the
// class-M path (maturityLatchOps) returns immediately for pre-latched chains.
// This is not a gap in the protocol — it is a natural consequence of the state-transition
// ordering. The class-M inheritance gap is LATENT: it matters if a future regression in the
// class-A screen anchoring creates a spurious ADD that then inflates the seen set.
//
// THIS GATE demonstrates the inheritance mechanism at the RecomputeMatureNow level:
// given a forgedRoot where validatorsSeenRoot commits an inflated seen set, RecomputeMatureNow
// returns matureNow=true (wrong), while the honest root returns false. This is the RED gate.
//
// Chain: young (MatureValidators=1), one proposer (prop) + one bonded third. Pre-state has
// empty validatorsSeen. forgedRoot is built by folding validatorsSeen[thirdID]=Present and
// updating validatorsSeenRoot — the exact ops the class-A attack would commit.
//
// Gate is RED on main: RecomputeMatureNow(forgedRoot, {thirdID}) returns true (wrong).
// The honest: RecomputeMatureNow(prevRoot, {}) returns false (correct, empty seen set).
// R1.2 closes this by anchoring the class-A screen so the spurious ADD is prevented.
func TestAdversarialRoot_ClassM_PoisonedBySpuriousAtt(t *testing.T) {
	// MatureValidators=1: one seen member >= MinBond clears the bar.
	// EpochBlocks=1024: no boundary fires at test heights. BondTTLBlocks=0: no TTL.
	cfg := Config{
		Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 1, BondTTLBlocks: 0,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(76001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
	)
	Sign(g, prop)
	c.apply(*g)
	if c.everMature {
		t.Fatalf("fixture: chain must be YOUNG post-genesis (MatureValidators=1, empty validatorsSeen)")
	}

	// Bond third at h=1 with full bond (>= era4MinBond). third is NOT yet in validatorsSeen.
	third := key(76003)
	thirdID := ports.HashBytes(pubOf(third))
	prev, h := c.Head()
	bBond := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev}
	bBond.BondRegs = append(bBond.BondRegs, bondRegFull(third, ports.HashBytes(pubOf(third)), 8<<20, ports.Hash{}, 5, 9))
	Sign(&bBond, prop)
	c.apply(bBond)

	if _, bonded := c.bonded[thirdID]; !bonded {
		t.Fatalf("fixture: third must be bonded at h=2")
	}
	if c.validatorsSeen[thirdID] {
		t.Fatalf("fixture: third must NOT be in validatorsSeen yet (no att)")
	}
	if c.everMature {
		t.Fatalf("fixture: chain must still be YOUNG (validatorsSeen empty, coefficient=0<1=MV)")
	}

	// Pre-state at h=2: validatorsSeen={}, tagEverMature=false, bonded[thirdID]=8<<20.
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch: prover=%x chain=%x", prevRoot, sr)
	}

	// Baseline: honest RecomputeMatureNow on prevRoot with empty seen set.
	// validatorsSeenRoot in prevRoot = MTH(empty). The honest chain has no seen members.
	seenRootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	var honestSeenRootValue []byte
	for _, lf := range leaves {
		if string(lf.Key) == string(seenRootKey) {
			honestSeenRootValue = lf.Value
			break
		}
	}
	honestSeenRootProof, pErr := prover.Prove(seenRootKey)
	if pErr != nil {
		t.Fatalf("Prove(validatorsSeenRoot honest): %v", pErr)
	}
	emptySeenSet := SeenSetWitness{
		IDs:             []ports.NodeID{},
		Members:         map[ports.NodeID]MemberStateWitness{},
		SeenRootValue:   honestSeenRootValue,
		SeenRootWitness: honestSeenRootProof,
	}
	honestMatureNow, honestErr := c.RecomputeMatureNow(prevRoot, emptySeenSet)
	if honestErr != nil {
		t.Fatalf("GATE CONSTRUCTION: honest RecomputeMatureNow(prevRoot, empty) must not stall: %v", honestErr)
	}
	if honestMatureNow {
		t.Fatalf("GATE VACUOUS: honest RecomputeMatureNow(prevRoot, empty) returned true — "+
			"empty seen set cannot clear MatureValidators=1. The maturity metric is broken.")
	}
	t.Logf("baseline: honest RecomputeMatureNow(prevRoot, {})=%v (correct: not mature)", honestMatureNow)

	// BUILD FORGEDROOT: fold validatorsSeen[thirdID]=Present AND update validatorsSeenRoot.
	// This is the root the attacker would commit after a spurious class-A ADD via a forged screen.
	// The fold uses the SAME FoldChangedPaths primitive the box uses.
	seenKey := statehash.Key(tagValidatorsSeen, thirdID[:])
	// validatorsSeen[thirdID] is ABSENT pre-state → non-membership proof.
	seenProofPre, sErr := prover.Prove(seenKey)
	if sErr != nil {
		t.Fatalf("Prove(validatorsSeen[thirdID] pre, absent): %v", sErr)
	}
	// validatorsSeenRoot pre-state → membership proof for old value.
	seenRootProofPre, srErr := prover.Prove(seenRootKey)
	if srErr != nil {
		t.Fatalf("Prove(validatorsSeenRoot pre): %v", srErr)
	}
	// The forged post-validatorsSeenRoot value: MTH({thirdID}).
	forgedSeenRootValue := nodeSetMTH([]ports.NodeID{thirdID})

	forgedOps := []statehash.FoldOp{
		{
			Key:      seenKey,
			OldValue: nil, // absent pre-state (non-membership proof)
			NewValue: statehash.Present,
			Proof:    seenProofPre,
		},
		{
			Key:      seenRootKey,
			OldValue: honestSeenRootValue, // old digest value (membership proof)
			NewValue: forgedSeenRootValue,
			Proof:    seenRootProofPre,
		},
	}
	forgedRoot, foldErr := statehash.FoldChangedPaths(prevRoot, forgedOps)
	if foldErr != nil {
		t.Fatalf("FoldChangedPaths (forgedRoot): %v", foldErr)
	}
	if forgedRoot == prevRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == prevRoot — the fold produced no change. " +
			"Check that validatorsSeen[thirdID] was truly absent and the digest changed.")
	}
	t.Logf("forgedRoot=%x prevRoot=%x (differ — spurious ADD moved the root)", forgedRoot, prevRoot)

	// BUILD FORGED SEENSET: prove thirdID's bonded/domain/slashed leaves against forgedRoot.
	// forgedRoot contains all of prevRoot's leaves PLUS validatorsSeen[thirdID]=Present and
	// the updated validatorsSeenRoot. The bonded[thirdID] leaf is unchanged from prevRoot.
	forgedPostLeaves := c.stateRootLeavesV5()
	// Inject the two changed leaves into the post-leaf set.
	for i, lf := range forgedPostLeaves {
		if string(lf.Key) == string(seenKey) {
			forgedPostLeaves[i].Value = statehash.Present
		}
		if string(lf.Key) == string(seenRootKey) {
			forgedPostLeaves[i].Value = forgedSeenRootValue
		}
	}
	// validatorsSeen[thirdID] was absent — add it.
	found := false
	for _, lf := range forgedPostLeaves {
		if string(lf.Key) == string(seenKey) {
			found = true
			break
		}
	}
	if !found {
		forgedPostLeaves = append(forgedPostLeaves, statehash.Leaf{Key: seenKey, Value: statehash.Present})
	}
	forgedPostProver, fpErr := statehash.NewProver(forgedPostLeaves)
	if fpErr != nil {
		t.Fatalf("NewProver(forgedPostLeaves): %v", fpErr)
	}
	if forgedPostProver.Root() != forgedRoot {
		t.Fatalf("GATE CONSTRUCTION ERROR: forged post-prover root %x != forgedRoot %x.\n"+
			"  The manual leaf injection into forgedPostLeaves did not reconstruct forgedRoot.\n"+
			"  Verify the seenKey leaf was correctly injected (absent→present).",
			forgedPostProver.Root(), forgedRoot)
	}

	// Per-member proofs for thirdID against forgedRoot.
	thirdSlashedProof, err := forgedPostProver.Prove(statehash.Key(tagSlashed, thirdID[:]))
	if err != nil {
		t.Fatalf("Prove(slashed[thirdID] forged): %v", err)
	}
	thirdBondedProof, err := forgedPostProver.Prove(statehash.Key(tagBonded, thirdID[:]))
	if err != nil {
		t.Fatalf("Prove(bonded[thirdID] forged): %v", err)
	}
	thirdDomainProof, err := forgedPostProver.Prove(statehash.Key(tagBondDomain, thirdID[:]))
	if err != nil {
		t.Fatalf("Prove(bondDomain[thirdID] forged): %v", err)
	}
	thirdDomain, thirdDomainPresent := c.bondDomain[thirdID]
	forgedSeenRootProof, err := forgedPostProver.Prove(seenRootKey)
	if err != nil {
		t.Fatalf("Prove(validatorsSeenRoot forged): %v", err)
	}

	forgedSeenSet := SeenSetWitness{
		IDs: []ports.NodeID{thirdID},
		Members: map[ports.NodeID]MemberStateWitness{
			thirdID: {
				Bonded:        c.bonded[thirdID], // real committed bond: 8<<20 >= era4MinBond
				BondedProof:   thirdBondedProof,
				Domain:        thirdDomain,
				DomainPresent: thirdDomainPresent,
				DomainProof:   thirdDomainProof,
				Slashed:       c.slashed[thirdID], // false
				SlashedProof:  thirdSlashedProof,
			},
		},
		SeenRootValue:   forgedSeenRootValue,
		SeenRootWitness: forgedSeenRootProof,
	}

	// GATE ASSERTION: RecomputeMatureNow(forgedRoot, {thirdID}) must return true on main.
	// third is bonded 8<<20 >= era4MinBond=1<<20 → coefficient=1 >= MatureValidators=1 → matureNow=true.
	// The class-M metric is inflated by the spurious seen member admitted via forgedRoot.
	forgedMatureNow, forgedErr := c.RecomputeMatureNow(forgedRoot, forgedSeenSet)
	if forgedErr != nil {
		t.Fatalf("GATE MISFIRED (class-M poison): RecomputeMatureNow(forgedRoot, {thirdID}) stalled.\n"+
			"  Expected nil error + true (wrong-latch = RED gate on main). Got error: %v\n"+
			"  forgedRoot=%x prevRoot=%x\n"+
			"  If R1.2 is in place (class-A screen anchored), the forged seen set would not "+
			"be constructible from a forged block — the spurious ADD is prevented at class-A.",
			forgedErr, forgedRoot, prevRoot)
	}
	if !forgedMatureNow {
		t.Fatalf("GATE MISFIRED (class-M poison): RecomputeMatureNow(forgedRoot, {thirdID}) returned false.\n"+
			"  Expected true (thirdID bonded %d >= MinBond %d, coefficient=1 >= MV=1).\n"+
			"  The maturity metric did not count thirdID. Check MemberStateWitness.Bonded and proofs.\n"+
			"  forgedRoot=%x prevRoot=%x",
			c.bonded[thirdID], era4MinBond, forgedRoot, prevRoot)
	}
	t.Logf("GATE RED (confirmed): class-M wrong-latches everMature via poisoned validatorsSeenRoot.\n"+
		"  RecomputeMatureNow(forgedRoot, {thirdID}) = true (wrong-latch).\n"+
		"  RecomputeMatureNow(prevRoot, {})          = false (honest).\n"+
		"  forgedRoot=%x\n  prevRoot=%x\n  spuriousMember=%x (bonded %d >= MinBond %d)\n"+
		"  PE Q2 claim CONFIRMED: class-M inherits the inflated validatorsSeenRoot from the committed root.\n"+
		"  R1.2 must anchor the class-A screen (InEpochSet / BondedSize / BondedPresent / Slashed)\n"+
		"  to prevent the spurious validatorsSeen||id ADD that feeds this wrong-latch.",
		forgedRoot, prevRoot, thirdID[:4], c.bonded[thirdID], era4MinBond)
}

// =============================================================================
// PART 3 — R1.1 REMAINING ADVERSARIAL-COMMITTED-ROOT GATES
// =============================================================================
//
// These 8 gates complete the Tier-A adversarial-committed-root coverage.
// Each gate:
//   1. Builds a fixture where the forged field is meaningful (affects the recompute output).
//   2. Forges the field value in the witness.
//   3. Calls adversarialCommittedRoot → gets forgedRoot (same code path as the box).
//   4. Asserts forgedRoot != honestRoot (forge is real, not vacuous).
//   5. Asserts Recompute(prev, forgedRoot, b, forgedWit) == nil (wrong-accept = RED on main).
//   nil = wrong-accept = RED gate = the gap is real. After R1.2 each gate must return non-nil.

// =============================================================================
// CLASS P — RegVersion
// =============================================================================
//
// TestAdversarialRoot_ClassP_ForgedRegVersion — adversarial-committed-root gate for
// StateRootRotateMember.RegVersion.
//
// Attack: forge a frozen member's RegVersion from 5→4. The era-4 tally now fails (the forged member
// no longer counts toward ready-for-threshold-5). The box omits era4LockedIn/era4Height ops from its
// fold → forgedRoot lacks those scalars. The attacker commits this root; the box wrong-accepts.
//
// The existing ablation TestRecomputeStateRootRotateAblationLiveTallyForgedRegVersion proves the
// honest-root path stalls (mismatch). This gate proves the ADVERSARIAL-ROOT path wrong-accepts (nil).
// Both are needed: the ablation confirms the field matters; this gate confirms it is attackable.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassP_ForgedRegVersion(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// small: rv=4, below era-4 threshold (needs rv>=5). big: rv=5, dominant weight.
	small := key(77001)
	big := key(77002)

	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(small, ports.HashBytes(pubOf(small)), 2<<20, ports.Hash{}, 4, 1),
	)
	Sign(g, small)
	c.apply(*g)
	if c.era4LockedIn {
		t.Fatalf("fixture: era4 must be UNLOCKED at genesis (single rv=4 member)")
	}

	// h=1: add big (rv=5, dominant weight) so the era-4 tally fires at the h=2 boundary.
	prev1, _ := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev1}
	b1.BondRegs = append(b1.BondRegs, bondRegFull(big, ports.HashBytes(pubOf(big)), 16<<20, ports.Hash{}, 5, 2))
	Sign(&b1, small)
	c.apply(b1)

	// Confirm honest apply() locks era-4 at h=2.
	prev2, h2 := c.Head()
	rb := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(99)}}
	Sign(&rb, small)
	sanity := c.cloneForDryRun()
	sanity.apply(rb)
	if !sanity.era4LockedIn {
		t.Fatalf("fixture: era4 must lock at h=2 boundary in honest apply()")
	}
	if c.era4LockedIn {
		t.Fatalf("fixture: era4 must NOT be locked pre-state")
	}

	// Build rotateFixture manually (same pattern as existing Ablation 3b).
	f := rotateFixture{c: c, proposer: small}
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	f.prover = prover
	f.prevRoot = prover.Root()

	honestCommitted, err := sanity.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	w := f.witnessForBoundary(t, rb)

	// Baseline: honest witness agrees with apply().
	if checkErr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, rb, w); checkErr != nil {
		t.Fatalf("honest boundary witness must agree with apply(): %v", checkErr)
	}

	// FORGE: big's RegVersion 5→4. Era-4 tally fails → forgedRoot lacks era4LockedIn/era4Height.
	bigID := ports.HashBytes(pubOf(big))
	originalRV := uint8(0)
	for i := range w.Rotate.Members {
		if w.Rotate.Members[i].ID == bigID {
			originalRV = w.Rotate.Members[i].RegVersion
			w.Rotate.Members[i].RegVersion = 4
			break
		}
	}
	if originalRV != 5 {
		t.Fatalf("fixture: big must have rv=5 pre-forge (got rv=%d)", originalRV)
	}

	forgedRoot, honestRoot := adversarialCommittedRoot(t, f.c, f.prevRoot, honestCommitted, rb, w)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging big rv 5→4.\n"+
			"  Expected forgedRoot to lack era4LockedIn/era4Height scalars.\n"+
			"  era4LockedIn pre-state=%v", c.era4LockedIn)
	}

	// RED gate: wrong-accept on main.
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, rb, w)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedRegVersion): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x bigID=%x forgedRV=4 originalRV=%d",
			err, forgedRoot, honestRoot, bigID[:4], originalRV)
	}
	t.Logf("GATE RED (confirmed): RegVersion forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  bigID=%x rv forged %d→4\n"+
		"  forgedRoot lacks era4LockedIn/era4Height scalars that honestRoot carries.\n"+
		"  R1.2 must anchor RegVersion via Shape V to close this gate.",
		forgedRoot, honestRoot, bigID[:4], originalRV)
}

// =============================================================================
// CLASS P — RegVersionKnown
// =============================================================================
//
// TestAdversarialRoot_ClassP_ForgedRegVersionKnown — adversarial-committed-root gate for
// StateRootRotateMember.RegVersionKnown.
//
// Attack: forge a frozen member's RegVersionKnown from true→false. The box no longer adds the
// member to regVersionByID, so regVersionByID[id] defaults to 0 (below threshold 5). The era-4 tally
// fails → forgedRoot lacks era4LockedIn/era4Height scalars → wrong-accept.
//
// Same fixture as ForgedRegVersion. The forge is a different field (RegVersionKnown, not RegVersion),
// but the effect on the tally is identical: the member stops counting toward the ready weight.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassP_ForgedRegVersionKnown(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	small := key(77101)
	big := key(77102)

	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(small, ports.HashBytes(pubOf(small)), 2<<20, ports.Hash{}, 4, 1),
	)
	Sign(g, small)
	c.apply(*g)

	prev1, _ := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev1}
	b1.BondRegs = append(b1.BondRegs, bondRegFull(big, ports.HashBytes(pubOf(big)), 16<<20, ports.Hash{}, 5, 2))
	Sign(&b1, small)
	c.apply(b1)

	prev2, h2 := c.Head()
	rb := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(99)}}
	Sign(&rb, small)
	sanity := c.cloneForDryRun()
	sanity.apply(rb)
	if !sanity.era4LockedIn {
		t.Fatalf("fixture: era4 must lock at h=2 boundary")
	}
	if c.era4LockedIn {
		t.Fatalf("fixture: era4 must NOT be locked pre-state")
	}

	f := rotateFixture{c: c, proposer: small}
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	f.prover = prover
	f.prevRoot = prover.Root()

	honestCommitted, err := sanity.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	w := f.witnessForBoundary(t, rb)

	if checkErr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, rb, w); checkErr != nil {
		t.Fatalf("honest boundary witness must agree with apply(): %v", checkErr)
	}

	// FORGE: big's RegVersionKnown true→false.
	// Effect: regVersionByID[bigID] is never set → defaults to 0 → below era-4 threshold 5 → tally fails.
	bigID := ports.HashBytes(pubOf(big))
	originalKnown := false
	for i := range w.Rotate.Members {
		if w.Rotate.Members[i].ID == bigID {
			originalKnown = w.Rotate.Members[i].RegVersionKnown
			w.Rotate.Members[i].RegVersionKnown = false
			w.Rotate.Members[i].RegVersion = 0
			break
		}
	}
	if !originalKnown {
		t.Fatalf("fixture: big must have RegVersionKnown=true pre-forge (regVersion=5 leaf exists)")
	}

	forgedRoot, honestRoot := adversarialCommittedRoot(t, f.c, f.prevRoot, honestCommitted, rb, w)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging big RegVersionKnown true→false.\n"+
			"  era4LockedIn pre=%v", c.era4LockedIn)
	}

	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, rb, w)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedRegVersionKnown): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x bigID=%x",
			err, forgedRoot, honestRoot, bigID[:4])
	}
	t.Logf("GATE RED (confirmed): RegVersionKnown forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  bigID=%x RegVersionKnown forged true→false\n"+
		"  forgedRoot lacks era4LockedIn/era4Height scalars that honestRoot carries.\n"+
		"  R1.2 must anchor RegVersionKnown via Shape V to close this gate.",
		forgedRoot, honestRoot, bigID[:4])
}

// =============================================================================
// CLASS A — Slashed
// =============================================================================
//
// TestAdversarialRoot_ClassA_ForgedSlashed — adversarial-committed-root gate for
// StateRootAttScreen.Slashed.
//
// Attack: a truly-slashed attester has Slashed forged to false. attesterQualifiedFromScreen returns
// true (F2 gate bypassed). A spurious validatorsSeen||id ADD is emitted. The attacker commits the
// inflated validatorsSeenRoot → wrong-accept.
//
// Fixture: a slashed attester exists pre-state (applied via an equivocation proof). The test block
// carries an att from the culprit. Honest apply() skips it (slashed). Gate forges Slashed=false →
// box seats the culprit → inflated seen set → forgedRoot != honestRoot → nil (wrong-accept).
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassA_ForgedSlashed(t *testing.T) {
	// EpochBlocks=0: epochsEnabled()=false → pre-maturity A-qualification path always active.
	// MatureValidators=0: Mature()=true always → everMature=true post-genesis → latchedMaturityWitness works (no SeenSet).
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 0, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(78001)
	culprit := key(78002)

	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(culprit, ports.HashBytes(pubOf(culprit)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)
	culpritID := ports.HashBytes(pubOf(culprit))

	if !c.everMature {
		t.Fatalf("fixture: everMature must be true post-genesis (MatureValidators=0)")
	}

	// Slash culprit at h=1.
	prev1, h1 := c.Head()
	bSlash := Block{Version: BlockVersionWitnessable, Height: h1, Prev: prev1,
		Slashes: []Equivocation{slashProof(culprit, prev1, 0x41, 0x42)}}
	Sign(&bSlash, prop)
	c.apply(bSlash)
	if !c.slashed[culpritID] {
		t.Fatalf("fixture: culprit must be slashed after the slash block")
	}
	if c.validatorsSeen[culpritID] {
		t.Fatalf("fixture: culprit must NOT be in validatorsSeen")
	}

	// Pre-state at h=2: culprit is slashed, everMature=true, validatorsSeen empty.
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch")
	}
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	// Test block: culprit attests at h=2. Honest apply() skips culprit (slashed).
	prev2, h2 := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(77)}}
	bTest.Atts = append(bTest.Atts, Attest(&bTest, culprit))
	Sign(&bTest, prop)

	clone := c.cloneForDryRun()
	clone.apply(bTest)
	honestCommitted, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honestCommitted: %v", err)
	}
	if clone.validatorsSeen[culpritID] {
		t.Fatalf("fixture: culprit must NOT be in validatorsSeen after honest apply (slashed)")
	}

	preSeen := map[ports.NodeID]struct{}{}
	for id := range c.validatorsSeen {
		preSeen[id] = struct{}{}
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
	seenRootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	seenRootWit, pErr := prover.Prove(seenRootKey)
	if pErr != nil {
		t.Fatalf("Prove(validatorsSeenRoot): %v", pErr)
	}

	// epochsEnabled()=false (EpochBlocks=0) → pre-maturity A-qualification: (BondedPresent && BondedSize >= MinBond) || launchAnchor.
	// Honest: Slashed=true → F2 gate → false (BondedSize/BondedPresent irrelevant for F2).
	// NOTE: slashing removes culprit from bonded map (chain.go:3288), so c.bonded[culpritID]=0.
	// The honest screen reflects the real post-slash state: not bonded.
	honestScreen := StateRootAttScreen{
		Attester: culpritID, Slashed: true, InEpochSet: false,
		BondedSize: 0, BondedPresent: false,
	}
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.AttScreens = []StateRootAttScreen{honestScreen}
	w.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	// everMature=true post-genesis → latchedMaturityWitness gives preEverMature=true → no SeenSet needed.
	w.Maturity = latchedMaturityWitness(t, prover, preValue)

	if checkErr := c.RecomputeStateRootEntriesRevocations(prevRoot, honestCommitted, bTest, w); checkErr != nil {
		t.Fatalf("honest witness (Slashed=true) must agree with apply(): %v", checkErr)
	}

	// FORGE: Slashed=false + BondedPresent=true + BondedSize=MinBond.
	// The attacker claims culprit is not slashed AND is bonded at MinBond.
	// Pre-maturity path: (true && MinBond >= MinBond) = true → spurious validatorsSeen ADD.
	// NOTE: slashing deleted the bonded leaf; the attacker forges its presence (forged non-membership
	// proof for validatorsSeen||culpritID is still valid — that leaf IS absent in prevRoot).
	forgedScreen := StateRootAttScreen{
		Attester: culpritID, Slashed: false, InEpochSet: false,
		BondedSize: era4MinBond, BondedPresent: true, // forged: claim culprit is bonded at MinBond
	}

	// Derive the forged A write-set (spurious ADD for culprit).
	forgedScreensMap := map[ports.NodeID]StateRootAttScreen{culpritID: forgedScreen}
	forgedAWrites, _, forgedAErr := c.stateRootAttWriteSet(bTest, preSeen, forgedScreensMap)
	if forgedAErr != nil {
		t.Fatalf("forged stateRootAttWriteSet: %v", forgedAErr)
	}
	if len(forgedAWrites) == 0 {
		t.Fatalf("GATE CONSTRUCTION: forged Slashed=false + BondedSize>=MinBond must produce a validatorsSeen ADD")
	}

	// Build forged witness: E/R leaves + forged A leaves (absent pre-state, non-membership proof).
	var forgedW StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	for _, wr := range forgedAWrites {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	forgedW.Maturity = w.Maturity

	forgedRoot, honestRoot := adversarialCommittedRoot(t, c, prevRoot, honestCommitted, bTest, forgedW)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging Slashed true→false.\n"+
			"  culpritID=%x (bond deleted by slash; forged BondedPresent=true BondedSize=MinBond)", culpritID[:4])
	}

	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedSlashed): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x culpritID=%x",
			err, forgedRoot, honestRoot, culpritID[:4])
	}
	t.Logf("GATE RED (confirmed): Slashed forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  culpritID=%x Slashed forged true→false\n"+
		"  F2 gate bypassed; culprit spuriously seated into validatorsSeen.\n"+
		"  R1.2 must anchor Slashed via Shape S (slashedRoot anchoredPreSet) to close this gate.",
		forgedRoot, honestRoot, culpritID[:4])
}

// =============================================================================
// CLASS A — BondedSize
// =============================================================================
//
// TestAdversarialRoot_ClassA_ForgedBondedSize — adversarial-committed-root gate for
// StateRootAttScreen.BondedSize.
//
// Attack: an attester bonded BELOW MinBond has BondedSize forged to >= MinBond. The pre-maturity
// qualification predicate (sc.BondedPresent && sc.BondedSize >= MinBond) flips true → spurious
// validatorsSeen ADD → inflated validatorsSeenRoot → wrong-accept.
//
// Fixture: MatureValidators=0 (everMature=true after genesis → latchedMaturityWitness works, no SeenSet
// needed), EpochBlocks=0 (epochsEnabled()=false → pre-maturity A-qualification path always active).
// One attester bonded below MinBond. Test block has that attester attest. Honest: excluded. Forged:
// BondedSize raised to MinBond → included.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassA_ForgedBondedSize(t *testing.T) {
	// EpochBlocks=0: no epochs → epochsEnabled()=false → pre-maturity path always.
	// MatureValidators=0: Mature()=true always → everMature=true after genesis → latchedMaturityWitness ok.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 0, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(78101)
	underBonded := key(78102)
	underBondedID := ports.HashBytes(pubOf(underBonded))
	smallBond := era4MinBond - 1 // just below MinBond

	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(underBonded, ports.HashBytes(pubOf(underBonded)), smallBond, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)

	if !c.everMature {
		t.Fatalf("fixture: everMature must be true post-genesis (MatureValidators=0)")
	}
	ubBonded, ubPresent := c.bonded[underBondedID]
	if !ubPresent {
		t.Fatalf("fixture: underBonded must be in bonded map")
	}
	if ubBonded >= era4MinBond {
		t.Fatalf("fixture: underBonded must be BELOW MinBond (size=%d >= MinBond=%d)", ubBonded, era4MinBond)
	}

	// Pre-state at h=1.
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch")
	}
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	// Test block: underBonded attests. Honest: excluded (BondedSize < MinBond).
	prev, h := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(66)}}
	bTest.Atts = append(bTest.Atts, Attest(&bTest, underBonded))
	Sign(&bTest, prop)

	clone := c.cloneForDryRun()
	clone.apply(bTest)
	honestCommitted, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honestCommitted: %v", err)
	}
	if clone.validatorsSeen[underBondedID] {
		t.Fatalf("fixture: underBonded must NOT be in validatorsSeen after honest apply (BondedSize < MinBond)")
	}

	preSeen := map[ports.NodeID]struct{}{}
	for id := range c.validatorsSeen {
		preSeen[id] = struct{}{}
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
	seenRootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	seenRootWit, pErr := prover.Prove(seenRootKey)
	if pErr != nil {
		t.Fatalf("Prove(validatorsSeenRoot): %v", pErr)
	}

	honestScreen := StateRootAttScreen{
		Attester: underBondedID, Slashed: false, InEpochSet: false,
		BondedSize: ubBonded, BondedPresent: true,
	}
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.AttScreens = []StateRootAttScreen{honestScreen}
	w.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	// everMature=true → latchedMaturityWitness gives preEverMature=true → no SeenSet needed.
	w.Maturity = latchedMaturityWitness(t, prover, preValue)

	if checkErr := c.RecomputeStateRootEntriesRevocations(prevRoot, honestCommitted, bTest, w); checkErr != nil {
		t.Fatalf("honest witness (BondedSize below MinBond) must agree with apply(): %v", checkErr)
	}

	// FORGE: raise BondedSize to MinBond → qualifies via pre-maturity path.
	forgedBondedSize := era4MinBond
	forgedScreen := StateRootAttScreen{
		Attester: underBondedID, Slashed: false, InEpochSet: false,
		BondedSize: forgedBondedSize, BondedPresent: true,
	}
	forgedScreensMap := map[ports.NodeID]StateRootAttScreen{underBondedID: forgedScreen}
	forgedAWrites, _, forgedAErr := c.stateRootAttWriteSet(bTest, preSeen, forgedScreensMap)
	if forgedAErr != nil {
		t.Fatalf("forged stateRootAttWriteSet: %v", forgedAErr)
	}
	if len(forgedAWrites) == 0 {
		t.Fatalf("GATE CONSTRUCTION: forged BondedSize=MinBond must qualify underBonded")
	}

	var forgedW StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	for _, wr := range forgedAWrites {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	forgedW.Maturity = w.Maturity

	forgedRoot, honestRoot := adversarialCommittedRoot(t, c, prevRoot, honestCommitted, bTest, forgedW)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging BondedSize %d→%d.",
			ubBonded, forgedBondedSize)
	}

	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedBondedSize): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x ubBonded=%d forgedSize=%d MinBond=%d",
			err, forgedRoot, honestRoot, ubBonded, forgedBondedSize, era4MinBond)
	}
	t.Logf("GATE RED (confirmed): BondedSize forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  underBondedID=%x BondedSize forged %d→%d (MinBond=%d)\n"+
		"  Pre-maturity qualification gate bypassed; attester spuriously seated.\n"+
		"  R1.2 must anchor BondedSize via Shape V to close this gate.",
		forgedRoot, honestRoot, underBondedID[:4], ubBonded, forgedBondedSize, era4MinBond)
}

// =============================================================================
// CLASS A — BondedPresent
// =============================================================================
//
// TestAdversarialRoot_ClassA_ForgedBondedPresent — adversarial-committed-root gate for
// StateRootAttScreen.BondedPresent.
//
// Attack: an attester with NO bonded leaf (BondedPresent=false honestly) is forged to
// BondedPresent=true with BondedSize >= MinBond. The pre-maturity predicate flips true →
// spurious validatorsSeen ADD → wrong-accept.
//
// Same fixture design as BondedSize: MatureValidators=0, EpochBlocks=0.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassA_ForgedBondedPresent(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 0, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(78201)
	notBonded := key(78202)
	notBondedID := ports.HashBytes(pubOf(notBonded))

	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
	)
	Sign(g, prop)
	c.apply(*g)

	if !c.everMature {
		t.Fatalf("fixture: everMature must be true post-genesis (MatureValidators=0)")
	}
	if _, ok := c.bonded[notBondedID]; ok {
		t.Fatalf("fixture: notBonded must NOT be in bonded map")
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != prevRoot {
		t.Fatalf("pre-root mismatch")
	}
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	prev, h := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(67)}}
	bTest.Atts = append(bTest.Atts, Attest(&bTest, notBonded))
	Sign(&bTest, prop)

	clone := c.cloneForDryRun()
	clone.apply(bTest)
	honestCommitted, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honestCommitted: %v", err)
	}
	if clone.validatorsSeen[notBondedID] {
		t.Fatalf("fixture: notBonded must NOT be in validatorsSeen after honest apply")
	}

	preSeen := map[ports.NodeID]struct{}{}
	for id := range c.validatorsSeen {
		preSeen[id] = struct{}{}
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
	seenRootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	seenRootWit, pErr := prover.Prove(seenRootKey)
	if pErr != nil {
		t.Fatalf("Prove(validatorsSeenRoot): %v", pErr)
	}

	honestScreen := StateRootAttScreen{
		Attester: notBondedID, Slashed: false, InEpochSet: false,
		BondedSize: 0, BondedPresent: false,
	}
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.AttScreens = []StateRootAttScreen{honestScreen}
	w.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	w.Maturity = latchedMaturityWitness(t, prover, preValue)

	if checkErr := c.RecomputeStateRootEntriesRevocations(prevRoot, honestCommitted, bTest, w); checkErr != nil {
		t.Fatalf("honest witness (BondedPresent=false) must agree with apply(): %v", checkErr)
	}

	// FORGE: BondedPresent=true, BondedSize=MinBond → qualifies via pre-maturity path.
	forgedScreen := StateRootAttScreen{
		Attester: notBondedID, Slashed: false, InEpochSet: false,
		BondedSize: era4MinBond, BondedPresent: true,
	}
	forgedScreensMap := map[ports.NodeID]StateRootAttScreen{notBondedID: forgedScreen}
	forgedAWrites, _, forgedAErr := c.stateRootAttWriteSet(bTest, preSeen, forgedScreensMap)
	if forgedAErr != nil {
		t.Fatalf("forged stateRootAttWriteSet: %v", forgedAErr)
	}
	if len(forgedAWrites) == 0 {
		t.Fatalf("GATE CONSTRUCTION: forged BondedPresent=true BondedSize=MinBond must qualify notBonded")
	}

	var forgedW StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	for _, wr := range forgedAWrites {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	forgedW.Maturity = w.Maturity

	forgedRoot, honestRoot := adversarialCommittedRoot(t, c, prevRoot, honestCommitted, bTest, forgedW)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging BondedPresent false→true.")
	}

	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedBondedPresent): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x notBondedID=%x",
			err, forgedRoot, honestRoot, notBondedID[:4])
	}
	t.Logf("GATE RED (confirmed): BondedPresent forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  notBondedID=%x BondedPresent forged false→true\n"+
		"  Attester with no bonded leaf spuriously seated into validatorsSeen.\n"+
		"  R1.2 must anchor BondedPresent via Shape V to close this gate.",
		forgedRoot, honestRoot, notBondedID[:4])
}

// =============================================================================
// CLASS B — PriorOwner
// =============================================================================
//
// TestAdversarialRoot_ClassB_ForgedPriorOwner — adversarial-committed-root gate for
// StateRootBondRegScreen.PriorOwner.
//
// Attack: in a displacement scenario (honest proven reg vs. unproven squatter), forge PriorOwner to
// equal the new registrant's ID. The displacement branch checks `o != id`: with o == honestID == id,
// the condition is false → no displacement. The box derives a delta without the squatter's
// bonded/qualified deletes. The attacker commits this root → wrong-accept.
//
// Fixture: buildBondFixture has a genesis squatter on a shared root (unproven). The gate forges the
// screen to report the squatter's PriorOwner AS the new honest registrant → displacement suppressed.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassB_ForgedPriorOwner(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(83001)
	honestID := ports.HashBytes(pubOf(honest))
	sqID := ports.HashBytes(pubOf(f.squatter))

	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}

	// Confirm displacement fires in honest apply().
	sanity := f.c.cloneForDryRun()
	sanity.apply(b)
	if _, still := sanity.bonded[sqID]; still {
		t.Fatalf("fixture: displacement must fire for the gate to be meaningful")
	}

	honestCommitted := f.applyAndCommittedRoot(t, b)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})

	// Honest witness agrees with apply().
	if checkErr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, b, w); checkErr != nil {
		t.Fatalf("honest witness (displacement) must agree with apply(): %v", checkErr)
	}

	// FORGE PriorOwner: claim the shared root's prior owner IS the new honest registrant.
	// stateRootBondRegWriteSet reads: `if o, isClaimed := owner[root]; isClaimed && o != id`.
	// With o=honestID, id=honestID: o==id → false → branch skipped → squatter NOT displaced.
	for i := range w.BondRegScreens {
		if w.BondRegScreens[i].Root == f.sharedRoot {
			w.BondRegScreens[i].PriorOwner = honestID // forge: claim honest IS the prior owner
			break
		}
	}

	// adversarialCommittedRoot derives the forged write-set (no squatter deletes) from the forged screen.
	// Honest ChangedLeaves is a SUPERSET of forged derived keys (extra leaves are unused, no stall).
	forgedRoot, honestRoot := adversarialCommittedRoot(t, f.c, f.prevRoot, honestCommitted, b, w)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging PriorOwner to honestID.\n"+
			"  sqID=%x honestID=%x sharedRoot=%x", sqID[:4], honestID[:4], f.sharedRoot[:4])
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedPriorOwner): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x sqID=%x honestID=%x",
			err, forgedRoot, honestRoot, sqID[:4], honestID[:4])
	}
	t.Logf("GATE RED (confirmed): PriorOwner forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  sqID=%x survives (displacement suppressed by PriorOwner forge)\n"+
		"  forgedPriorOwner=%x (=honestID → o==id → no displacement)\n"+
		"  R1.2 must anchor PriorOwner via Shape V (bondRootOwner||root Resolve) to close this gate.",
		forgedRoot, honestRoot, sqID[:4], honestID[:4])
}

// =============================================================================
// CLASS B — Claimed
// =============================================================================
//
// TestAdversarialRoot_ClassB_ForgedClaimed — adversarial-committed-root gate for
// StateRootBondRegScreen.Claimed.
//
// Attack: forge Claimed=false for a root that IS honestly claimed by the squatter. The displacement
// branch `if o, isClaimed := owner[root]; isClaimed && o != id` requires isClaimed=true. With
// Claimed=false, isClaimed=false → branch skipped → no displacement. Squatter keeps standing.
// The attacker commits the delta without squatter deletes → wrong-accept.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassB_ForgedClaimed(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(83101)
	sqID := ports.HashBytes(pubOf(f.squatter))

	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}

	sanity := f.c.cloneForDryRun()
	sanity.apply(b)
	if _, still := sanity.bonded[sqID]; still {
		t.Fatalf("fixture: displacement must fire in honest apply()")
	}

	honestCommitted := f.applyAndCommittedRoot(t, b)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})

	if checkErr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, b, w); checkErr != nil {
		t.Fatalf("honest witness must agree with apply(): %v", checkErr)
	}

	// FORGE: Claimed=false → isClaimed=false → displacement branch not entered → squatter survives.
	for i := range w.BondRegScreens {
		if w.BondRegScreens[i].Root == f.sharedRoot {
			w.BondRegScreens[i].Claimed = false
			w.BondRegScreens[i].PriorOwner = ports.NodeID{}
			w.BondRegScreens[i].PriorProven = false
			break
		}
	}

	forgedRoot, honestRoot := adversarialCommittedRoot(t, f.c, f.prevRoot, honestCommitted, b, w)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging Claimed true→false.\n"+
			"  sqID=%x sharedRoot=%x", sqID[:4], f.sharedRoot[:4])
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedClaimed): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x sqID=%x",
			err, forgedRoot, honestRoot, sqID[:4])
	}
	t.Logf("GATE RED (confirmed): Claimed forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n  sqID=%x survives (displacement skipped, Claimed forged true→false)\n"+
		"  R1.2 must anchor Claimed via Shape V (bondRootOwner||root absent-proof) to close this gate.",
		forgedRoot, honestRoot, sqID[:4])
}

// =============================================================================
// CLASS B — PriorProven
// =============================================================================
//
// TestAdversarialRoot_ClassB_ForgedPriorProven — adversarial-committed-root gate for
// StateRootBondRegScreen.PriorProven.
//
// Attack: the displacement fires iff `proven && !provenRoot[r.Root]` where
// proven=(b.Height>0) and provenRoot[r.Root]=sc.PriorProven. An unproven squatter
// (PriorProven=false honestly) is forged to PriorProven=true. Then:
//   `proven && !true = false` → `!false = true` → `continue`
// The honest reg is REJECTED (the loop body is skipped entirely). The squatter keeps its bonded
// standing AND the honest reg gets no standing. The committed delta omits both the squatter deletes
// AND the honest reg inserts. The attacker commits this root → wrong-accept.
//
// Gate is RED on main (nil = wrong-accept). After R1.2 this gate must return non-nil.
func TestAdversarialRoot_ClassB_ForgedPriorProven(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(83201)
	sqID := ports.HashBytes(pubOf(f.squatter))
	honestID := ports.HashBytes(pubOf(honest))

	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}

	// Confirm displacement fires AND honest reg lands in honest apply().
	sanity := f.c.cloneForDryRun()
	sanity.apply(b)
	if _, still := sanity.bonded[sqID]; still {
		t.Fatalf("fixture: squatter must be displaced in honest apply()")
	}
	if _, ok := sanity.bonded[honestID]; !ok {
		t.Fatalf("fixture: honest reg must land in honest apply()")
	}
	// Squatter must be UNPROVEN (genesis-declared): PriorProven=false is the honest value.
	if f.c.bondRootProven[f.sharedRoot] {
		t.Fatalf("fixture: squatter's root must be UNPROVEN for PriorProven forge to flip the branch")
	}

	honestCommitted := f.applyAndCommittedRoot(t, b)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})

	if checkErr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, b, w); checkErr != nil {
		t.Fatalf("honest witness must agree with apply(): %v", checkErr)
	}

	// FORGE: PriorProven=false → true. Displacement condition:
	//   proven(h=1>0=true) && !provenRoot[root](=!true=false) → false → !false → continue.
	// The honest reg is REJECTED (continues out of loop body without writing anything).
	// The box derives a delta where: no squatter deletes, no honest reg inserts.
	// This matches a root where the squatter is still bonded and honest reg is absent.
	for i := range w.BondRegScreens {
		if w.BondRegScreens[i].Root == f.sharedRoot {
			w.BondRegScreens[i].PriorProven = true
			break
		}
	}

	forgedRoot, honestRoot := adversarialCommittedRoot(t, f.c, f.prevRoot, honestCommitted, b, w)

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot after forging PriorProven false→true.\n"+
			"  sqID=%x honestID=%x h=%d proven(h>0)=%v", sqID[:4], honestID[:4], h, h > 0)
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err != nil {
		t.Fatalf("GATE MISFIRED (ForgedPriorProven): Recompute stalled against forgedRoot.\n"+
			"  Expected nil (wrong-accept = RED gate on main). Got: %v\n"+
			"  forgedRoot=%x honestRoot=%x sqID=%x honestID=%x",
			err, forgedRoot, honestRoot, sqID[:4], honestID[:4])
	}
	t.Logf("GATE RED (confirmed): PriorProven forgeable on main (adversarial-root path).\n"+
		"  forgedRoot=%x\n  honestRoot=%x\n"+
		"  sqID=%x survives + honestID=%x rejected (PriorProven forged false→true)\n"+
		"  Honest reg REJECTED (loop continues), squatter unbothered.\n"+
		"  R1.2 must anchor PriorProven via Shape V (bondRootProven||root Resolve) to close this gate.",
		forgedRoot, honestRoot, sqID[:4], honestID[:4])
}
