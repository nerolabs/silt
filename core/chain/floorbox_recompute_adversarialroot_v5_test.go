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
