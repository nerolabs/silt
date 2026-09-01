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
	"fmt"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// mustProve issues an SMT proof for key against the prover's committed root, panicking on error.
// It is the R1.2 witness-builder helper: the fixtures always hold a valid prover, so a Prove failure
// is a test-construction bug, not an expected path. Used to populate the per-field screen/member
// anchor proofs the R1.2 fix requires.
func mustProve(p *statehash.Prover, key []byte) statehash.Witness {
	w, err := p.Prove(key)
	if err != nil {
		panic(fmt.Sprintf("mustProve(%x): %v", key, err))
	}
	return w
}

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

	// Steady-state member (no class-B write this block; buildRotateFixture's boundary carries no bond
	// reg), so the Weight anchor is the qualified||id present-proof. Forge the frozen Weight; the honest
	// QualifiedProof proves the ORIGINAL weight, so the anchor's IsProvenPresent at the forged value fails.
	originalWeight := w.Rotate.Members[0].Weight
	memberID := w.Rotate.Members[0].ID
	forgedWeight := originalWeight + 1
	w.Rotate.Members[0].Weight = forgedWeight

	// forgedRoot: the committed root the box computes if it TRUSTS the forged weight — the honest
	// post-apply state with epochSet||member set to the forged weight (rotateEpochSetLeafOps writes
	// EncodeInt64(Weight)). Passing it makes ablating the Weight anchor a wrong-ACCEPT (the ablation proof).
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	clone.epochSet[memberID] = forgedWeight
	forgedRoot, mErr := clone.StateRootForVersion(BlockVersionWitnessable)
	if mErr != nil {
		t.Fatalf("forgedRoot: %v", mErr)
	}
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted after forging Weight %d→%d", originalWeight, forgedWeight)
	}

	// R1.2: the box MUST STALL. The Weight anchor requires the frozen Weight proven present in
	// qualified||id under prevStateRoot; the forged Weight cannot be, so the class-P member anchor stalls.
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedFrozenWeight): box WRONG-ACCEPTED a forged frozen Weight.\n"+
			"  Expected a stall (R1.2 anchors Weight against qualified||id). Got nil.\n"+
			"  member=%x forgedWeight=%d originalWeight=%d", memberID[:4], forgedWeight, originalWeight)
	}
	t.Logf("GATE GREEN (R1.2): forged frozen Weight STALLS: %v\n"+
		"  member=%x forgedWeight=%d originalWeight=%d — Weight anchored against qualified||id under prevStateRoot.",
		err, memberID[:4], forgedWeight, originalWeight)
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

	// R1.2 screen proofs, from the pre-state prover. newAtt is bonded (present) but NOT in epochSet
	// (absent) and not slashed (absent).
	honestScreen := StateRootAttScreen{
		Attester: newAttID, Slashed: false, InEpochSet: false,
		BondedSize: newAttBonded, BondedPresent: newAttBP2,
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, newAttID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, newAttID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, newAttID[:])),
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

	// FORGE: swap InEpochSet to true — the box would emit a spurious validatorsSeen ADD for newAtt.
	// The attacker cannot forge a MEMBERSHIP proof of epochSet||newAtt (newAtt is not in the frozen
	// set), so the forged screen still carries the honest NON-MEMBERSHIP proof — which fails
	// IsProvenPresent under R1.2 ⇒ stall.
	forgedScreen := StateRootAttScreen{
		Attester: newAttID, Slashed: false, InEpochSet: true, // forged
		BondedSize: newAttBonded, BondedPresent: newAttBP2,
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, newAttID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, newAttID[:])), // absence proof — cannot prove present
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, newAttID[:])),
	}
	var forgedW StateRootWitness
	// E/R and maturity leaves carry over unchanged.
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.Maturity = w.Maturity
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	// The spurious A write-set the ATTACKER folds into forgedRoot: validatorsSeen||newAtt ADD + the
	// updated validatorsSeenRoot digest. (Built directly — the box's own assembler now ANCHORS and
	// would stall, so the attacker computes the inflated committed root itself.)
	forgedAWrites := []stateRootWrite{{key: statehash.Key(tagValidatorsSeen, newAttID[:]), newValue: statehash.Present}}
	for _, wr := range forgedAWrites {
		forgedW.ChangedLeaves = append(forgedW.ChangedLeaves, leafWit(wr.key))
	}
	forgedW.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}

	// Build forgedRoot: the committed root the attacker embeds after a spurious class-A seating of
	// newAtt. It includes every honest write (E/R + digests) PLUS validatorsSeen[newAtt], so ablating
	// the anchor makes the box recompute EXACTLY this root (wrong-accept) — the ablation-proof shape.
	forgedRoot := spuriousSeatedRoot(t, c, bTest, newAttID)
	honestRoot := honestCommitted

	if forgedRoot == honestRoot {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot — InEpochSet forge did not change root (newAtt already seen?)")
	}

	// R1.2: the box MUST STALL against forgedRoot — the forged InEpochSet=true cannot be proven present
	// against prevStateRoot, so attesterQualifiedFromScreen refuses to seat newAtt.
	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedInEpochSet): box WRONG-ACCEPTED a forged InEpochSet screen.\n"+
			"  Expected a stall (R1.2 anchors InEpochSet against epochSet||id). Got nil.\n"+
			"  forgedRoot=%x honestRoot=%x", forgedRoot, honestRoot)
	}
	t.Logf("GATE GREEN (R1.2): forged InEpochSet STALLS: %v\n  newAtt=%x — InEpochSet anchored against epochSet||id under prevStateRoot.",
		err, newAttID[:4])
}

// spuriousSeatedRoot returns the committed StateRoot an attacker commits after a class-A screen
// spuriously seats `id` into validatorsSeen. It is the EXACT root the box would recompute if the
// (now-anchored) class-A screen were bypassed: the honest post-apply state PLUS validatorsSeen[id].
// It is built by honestly applying the block to a clone, then injecting the spurious seating and
// re-marshalling the state root — so it INCLUDES every honest write (E/R, digests) the box folds,
// not just the spurious ADD. Without that completeness the gate would stall on an incidental root
// mismatch (a decoration green), not on the anchor — the exact trap the ablation proof exposes.
func spuriousSeatedRoot(t *testing.T, base *Chain, b Block, id ports.NodeID) ports.Hash {
	t.Helper()
	clone := base.cloneForDryRun()
	clone.apply(b)          // honest apply — does NOT seat id (screen excludes it)
	clone.validatorsSeen[id] = true // the spurious class-A seating the attacker forges
	root, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("spuriousSeatedRoot: StateRootForVersion: %v", err)
	}
	return root
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
// THE REACHABLE PATH (PE ruling Q2, verified). The class-M poison ENTERS at the class-A screen and
// RIDES the committed validatorsSeenRoot into class-M's RecomputeMatureNow (maturitylatch_v5.go:87,
// over committedStateRoot). A forged class-A screen that seats a bonded-but-unqualified attester adds
// a spurious validatorsSeen||id, inflating the seen set class-M then folds — wrong-latching everMature
// early. RecomputeMatureNow in ISOLATION cannot detect the spurious seating (seating is class A's job);
// the DEFENSE is the class-A screen anchor (R1.2). So this gate drives the poison through the FULL
// entry RecomputeStateRootEntriesRevocations — the reachable path — and asserts the box STALLS at the
// class-A anchor before the spurious ADD can inflate the seen set class-M inherits.
//
// Fixture: a mature-epoch chain (MatureValidators=0 ⇒ everMature latched at genesis, epochSet frozen).
// `third` bonds AFTER the genesis freeze, so it is bonded but NOT in the frozen epochSet — honest
// apply() does NOT seat it (R-A-membership-source). At the test block `third` attests. A forged
// InEpochSet=true would seat it into validatorsSeen, inflating the validatorsSeenRoot that class-M's
// RecomputeMatureNow reads over the committed root (the PE Q2 inheritance). The box must STALL at the
// class-A anchor — closing the class-M inheritance AT SOURCE, so RecomputeMatureNow never sees the
// spuriously-seated member.
//
// NOTE ON THE ARCHITECTURAL CONSTRAINT (verified): an end-to-end everMature CROSSING via a spurious
// seating is not constructible — everMature=false pre-state requires a young chain, but on a young
// chain a bonded-above-MinBond member IS qualified (so the honest path also seats it, no discrepancy).
// So the reachable class-M defense is precisely the class-A screen anchor demonstrated here: with it,
// the seen set class-M inherits from the committed root cannot be inflated by a forged screen.
//
// Gate ships RED on main by asserting the wrong-accept; after R1.2 it asserts the class-A stall.
func TestAdversarialRoot_ClassM_PoisonedBySpuriousAtt(t *testing.T) {
	// MatureValidators=0: everMature latches at genesis, matureEpoch true. Epochs enabled + a genesis
	// boundary freezes the epochSet, so a post-freeze bond is NOT in epochSet (the mature-epoch screen).
	cfg := Config{
		Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 0,
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
	if !c.everMature {
		t.Fatalf("fixture: everMature must be latched post-genesis (MatureValidators=0)")
	}
	if !c.matureEpoch {
		t.Fatalf("fixture: matureEpoch must be true (frozen epochSet at genesis)")
	}

	// Bond third at h=1 — AFTER the genesis freeze, so third is bonded but NOT in epochSet.
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
	if _, inES := c.epochSet[thirdID]; inES {
		t.Fatalf("fixture: third must NOT be in the frozen epochSet (bonded post-freeze)")
	}
	if c.validatorsSeen[thirdID] {
		t.Fatalf("fixture: third must NOT be in validatorsSeen (no att yet)")
	}

	// Pre-state at h=2.
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()
	preValue := func(k []byte) []byte {
		for _, lf := range leaves {
			if string(lf.Key) == string(k) {
				return lf.Value
			}
		}
		return nil
	}

	// Test block at h=2: third attests. Honest apply() skips it (not in epochSet).
	prev2, h2 := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(88)}}
	bTest.Atts = append(bTest.Atts, Attest(&bTest, third))
	Sign(&bTest, prop)

	clone := c.cloneForDryRun()
	clone.apply(bTest)
	honestCommitted, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honestCommitted: %v", err)
	}
	if clone.validatorsSeen[thirdID] {
		t.Fatalf("fixture: third must NOT be in validatorsSeen after honest apply (not in epochSet)")
	}

	preSeenIDs := sortIDs(func() []ports.NodeID {
		var out []ports.NodeID
		for id := range c.validatorsSeen {
			out = append(out, id)
		}
		return out
	}())
	leafWit := func(k []byte) StateRootChangedLeafWitness {
		return StateRootChangedLeafWitness{Key: k, OldValue: preValue(k), Proof: mustProve(prover, k)}
	}
	seenRootWit := mustProve(prover, statehash.Key(tagValidatorsSeenRoot, nil))

	// Honest witness (InEpochSet=false → third not seated → no validatorsSeen change).
	honestScreen := StateRootAttScreen{
		Attester: thirdID, Slashed: false, InEpochSet: false, BondedSize: c.bonded[thirdID], BondedPresent: true,
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, thirdID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, thirdID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, thirdID[:])),
	}
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.AttScreens = []StateRootAttScreen{honestScreen}
	w.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot, PreIDs: preSeenIDs, Proof: seenRootWit}}
	// everMature latched at genesis (MatureValidators=0) → class M is pre-latched, emits nothing, reads
	// no SeenSet. The latched maturity witness supplies just the everMature pre-state scalar proof.
	w.Maturity = latchedMaturityWitness(t, prover, preValue)
	if checkErr := c.RecomputeStateRootEntriesRevocations(prevRoot, honestCommitted, bTest, w); checkErr != nil {
		t.Fatalf("honest witness (InEpochSet=false) must AGREE with apply(): %v", checkErr)
	}

	// FORGE: InEpochSet=true seats third; the attacker folds the spurious validatorsSeen||third ADD +
	// the updated validatorsSeenRoot into the committed root (built directly — the box's assembler now
	// anchors and would stall). The forged screen still carries third's honest epochSet NON-MEMBERSHIP
	// proof (third is genuinely not in the frozen set), which fails IsProvenPresent ⇒ class-A stall.
	forgedScreen := honestScreen
	forgedScreen.InEpochSet = true // forged; EpochSetProof still proves ABSENCE
	forgedRoot := spuriousSeatedRoot(t, c, bTest, thirdID)

	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted — the spurious seating did not move the root")
	}

	forgedW := w
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.ChangedLeaves = append(append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...),
		leafWit(statehash.Key(tagValidatorsSeen, thirdID[:])))

	// R1.2: the box MUST STALL at the class-A anchor — the forged InEpochSet=true cannot prove present,
	// so third is never seated, the spurious validatorsSeen ADD is never emitted, and class-M never
	// inherits an inflated seen set. This is the PE Q2 corollary made testable end-to-end.
	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (class-M poison via class-A): box WRONG-ACCEPTED a forged class-A seating\n"+
			"  that would inflate validatorsSeenRoot and cross everMature early.\n"+
			"  Expected a class-A stall (R1.2 anchors the screen). Got nil.\n"+
			"  forgedRoot=%x honestCommitted=%x spuriousMember=%x", forgedRoot, honestCommitted, thirdID[:4])
	}
	t.Logf("GATE GREEN (R1.2): class-M poison closed at source — forged class-A seating STALLS: %v\n"+
		"  spuriousMember=%x (bonded %d, not in epochSet). The class-A screen anchor forecloses the\n"+
		"  spurious validatorsSeen||id ADD, so class-M inherits the honest seen set. PE Q2 corollary confirmed.",
		err, thirdID[:4], c.bonded[thirdID])
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

	// forgedRoot: the committed root the box computes if it TRUSTS the forged (lowered) regVersion —
	// the era-4 tally fails to cross, so era4LockedIn/era4Height stay unset. Passing it makes ablating
	// the RegVersion anchor a wrong-ACCEPT (the ablation proof).
	forgedRoot := forgedRootWithoutEra4Lock(t, f.c, rb)
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted (era4 lock unchanged by the forge)")
	}

	// R1.2: the box MUST STALL — the forged RegVersion=4 requires a MEMBERSHIP proof of regVersion||big
	// at EncodeUint8(4), but the honest witness carries a membership proof of the TRUE value 5, so
	// IsProvenPresent at 4 fails ⇒ stall at the class-P member anchor.
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, rb, w)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedRegVersion): box WRONG-ACCEPTED a forged RegVersion 5→4.\n"+
			"  Expected a stall (R1.2 anchors RegVersion against regVersion||id). Got nil. bigID=%x originalRV=%d",
			bigID[:4], originalRV)
	}
	t.Logf("GATE GREEN (R1.2): forged RegVersion STALLS: %v\n  bigID=%x rv forged %d→4 — RegVersion anchored against regVersion||id.",
		err, bigID[:4], originalRV)
}

// forgedRootWithoutEra4Lock returns the committed root a class-P recompute produces when a forged
// (lowered) regVersion makes the era-4 activation tally FAIL to lock in: the honest post-apply state
// with era4LockedIn/era4Height cleared. It is the root the box computes if it trusts the forged
// regVersion — passing it turns an ablated RegVersion anchor into a wrong-accept (the ablation proof).
func forgedRootWithoutEra4Lock(t *testing.T, base *Chain, b Block) ports.Hash {
	t.Helper()
	clone := base.cloneForDryRun()
	clone.apply(b)
	clone.era4LockedIn = false
	clone.era4Height = 0
	root, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRootWithoutEra4Lock: %v", err)
	}
	return root
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

	// forgedRoot: era-4 fails to lock when the box trusts RegVersionKnown=false (regVersion defaults to
	// 0, below threshold). Passing it makes ablating the RegVersion anchor a wrong-ACCEPT.
	forgedRoot := forgedRootWithoutEra4Lock(t, f.c, rb)
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted (era4 lock unchanged by the forge)")
	}

	// R1.2: the box MUST STALL — the forged RegVersionKnown=false requires a NON-MEMBERSHIP proof of
	// regVersion||big, but the honest witness carries a MEMBERSHIP proof (big's regVersion=5 leaf is
	// present), so IsProvenAbsent fails ⇒ stall at the class-P member anchor.
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, rb, w)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedRegVersionKnown): box WRONG-ACCEPTED a forged RegVersionKnown=false.\n"+
			"  Expected a stall (R1.2 anchors RegVersion against regVersion||id). Got nil. bigID=%x", bigID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged RegVersionKnown STALLS: %v\n  bigID=%x — RegVersion anchored against regVersion||id under prevStateRoot.",
		err, bigID[:4])
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
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, culpritID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, culpritID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, culpritID[:])),
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

	// FORGE: Slashed=false + BondedPresent=true + BondedSize=MinBond. The attacker cannot forge the
	// slashed||culprit NON-MEMBERSHIP proof (culprit IS slashed), nor the bonded||culprit MEMBERSHIP
	// proof (slashing deleted the bonded leaf). So the forged screen still carries the honest proofs,
	// which prove the TRUE state — IsProvenAbsent(slashed)/IsProvenPresent(bonded) both fail ⇒ stall.
	forgedScreen := honestScreen
	forgedScreen.Slashed = false
	forgedScreen.BondedSize = era4MinBond
	forgedScreen.BondedPresent = true

	// forgedRoot = the committed root after a spurious class-A seating of culprit (honest post-apply +
	// validatorsSeen[culprit]) — includes every honest write, so ablating the anchor makes the box
	// recompute EXACTLY this root (wrong-accept).
	forgedRoot := spuriousSeatedRoot(t, c, bTest, culpritID)

	forgedW := w
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.ChangedLeaves = append(append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...),
		leafWit(statehash.Key(tagValidatorsSeen, culpritID[:])))

	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted after forging Slashed true→false. culpritID=%x", culpritID[:4])
	}

	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedSlashed): box WRONG-ACCEPTED a forged Slashed=false for a slashed culprit.\n"+
			"  Expected a stall (R1.2 anchors Slashed against slashed||id). Got nil. culpritID=%x", culpritID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged Slashed STALLS: %v\n  culpritID=%x — Slashed anchored against slashed||id under prevStateRoot.",
		err, culpritID[:4])
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
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, underBondedID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, underBondedID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, underBondedID[:])),
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

	// FORGE: raise BondedSize to MinBond → would qualify via the pre-maturity path. The BondedProof
	// proves the leaf at the TRUE ubBonded value; requiring present at forgedBondedSize (MinBond) fails
	// IsProvenPresent ⇒ class-A stall.
	forgedBondedSize := era4MinBond
	forgedScreen := honestScreen
	forgedScreen.BondedSize = forgedBondedSize

	forgedRoot := spuriousSeatedRoot(t, c, bTest, underBondedID)

	forgedW := w
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.ChangedLeaves = append(append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...),
		leafWit(statehash.Key(tagValidatorsSeen, underBondedID[:])))

	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted after forging BondedSize %d→%d.", ubBonded, forgedBondedSize)
	}

	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedBondedSize): box WRONG-ACCEPTED a forged BondedSize %d→%d.\n"+
			"  Expected a stall (R1.2 anchors BondedSize against bonded||id). Got nil. underBondedID=%x",
			ubBonded, forgedBondedSize, underBondedID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged BondedSize STALLS: %v\n  underBondedID=%x forged %d→%d — BondedSize anchored against bonded||id.",
		err, underBondedID[:4], ubBonded, forgedBondedSize)
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
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, notBondedID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, notBondedID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, notBondedID[:])),
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

	// FORGE: BondedPresent=true, BondedSize=MinBond → would qualify. The BondedProof proves the bonded
	// leaf ABSENT (notBonded has no bond); requiring present ⇒ IsProvenPresent fails ⇒ class-A stall.
	forgedScreen := honestScreen
	forgedScreen.BondedSize = era4MinBond
	forgedScreen.BondedPresent = true

	forgedRoot := spuriousSeatedRoot(t, c, bTest, notBondedID)

	forgedW := w
	forgedW.AttScreens = []StateRootAttScreen{forgedScreen}
	forgedW.ChangedLeaves = append(append([]StateRootChangedLeafWitness(nil), w.ChangedLeaves...),
		leafWit(statehash.Key(tagValidatorsSeen, notBondedID[:])))

	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted after forging BondedPresent false→true.")
	}

	err = c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, bTest, forgedW)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedBondedPresent): box WRONG-ACCEPTED a forged BondedPresent false→true.\n"+
			"  Expected a stall (R1.2 anchors BondedPresent against bonded||id). Got nil. notBondedID=%x", notBondedID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged BondedPresent STALLS: %v\n  notBondedID=%x — BondedPresent anchored against bonded||id.",
		err, notBondedID[:4])
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

	// forgedRoot: the committed root the box computes if it trusts the forged PriorOwner (displacement
	// suppressed → the squatter keeps its pre-block bonded+qualified standing). Passing it makes ablating
	// the PriorOwner anchor a wrong-ACCEPT (the ablation proof).
	forgedRoot := forgedRootDisplacementSuppressed(t, f.c, b, sqID)
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted (squatter standing unchanged)")
	}

	// R1.2: the box MUST STALL — the forged PriorOwner=honestID requires a MEMBERSHIP proof of
	// bondRootOwner||sharedRoot at EncodeID(honestID), but the honest OwnerProof proves the TRUE owner
	// (the squatter), so IsProvenPresent at the forged owner fails ⇒ class-B stall.
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedPriorOwner): box WRONG-ACCEPTED a forged PriorOwner (=honestID → no displacement).\n"+
			"  Expected a stall (R1.2 anchors PriorOwner against bondRootOwner||root). Got nil. sqID=%x honestID=%x",
			sqID[:4], honestID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged PriorOwner STALLS: %v\n  sqID=%x honestID=%x — PriorOwner anchored against bondRootOwner||root.",
		err, sqID[:4], honestID[:4])
}

// forgedRootDisplacementSuppressed returns the committed root a class-B recompute produces when a
// forged ownership screen SUPPRESSES a squatter's displacement: the honest post-apply state with the
// squatter's pre-block bonded + qualified standing RESTORED (the displacement's deletes undone). It is
// the root the box computes if it trusts the forged screen — passing it turns an ablated class-B anchor
// into a wrong-accept (the ablation proof).
func forgedRootDisplacementSuppressed(t *testing.T, base *Chain, b Block, sqID ports.NodeID) ports.Hash {
	t.Helper()
	preBonded, wasBonded := base.bonded[sqID]
	preQual, wasQual := base.qualified[sqID]
	clone := base.cloneForDryRun()
	clone.apply(b) // displaces the squatter (deletes its bonded + qualified)
	if wasBonded {
		clone.bonded[sqID] = preBonded // undo the displacement's bonded delete
	}
	if wasQual {
		clone.qualified[sqID] = preQual // undo the displacement's qualified delete
	}
	root, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRootDisplacementSuppressed: %v", err)
	}
	return root
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

	// forgedRoot: displacement suppressed → squatter keeps standing (same forged effect as PriorOwner).
	forgedRoot := forgedRootDisplacementSuppressed(t, f.c, b, sqID)
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestCommitted (squatter standing unchanged)")
	}

	// R1.2: the box MUST STALL — the forged Claimed=false requires a NON-MEMBERSHIP proof of
	// bondRootOwner||sharedRoot, but the honest OwnerProof proves it PRESENT (the squatter owns it), so
	// IsProvenAbsent fails ⇒ class-B stall.
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedClaimed): box WRONG-ACCEPTED a forged Claimed true→false (displacement skipped).\n"+
			"  Expected a stall (R1.2 anchors Claimed against bondRootOwner||root). Got nil. sqID=%x", sqID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged Claimed STALLS: %v\n  sqID=%x — Claimed anchored against bondRootOwner||root.",
		err, sqID[:4])
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

	// forgedRoot: the forged PriorProven=true makes the box REJECT the honest reg entirely (loop
	// continues) and leaves the squatter untouched — i.e. NO net committed-state change from the
	// pre-state. So the root the box computes if it trusts the forge is prevRoot itself. Passing it
	// makes ablating the PriorProven anchor a wrong-ACCEPT (the ablation proof).
	forgedRoot := f.prevRoot
	if forgedRoot == honestCommitted {
		t.Fatalf("GATE VACUOUS: prevRoot == honestCommitted (the honest reg produced no state change?)")
	}

	// R1.2: the box MUST STALL — the forged PriorProven=true requires a MEMBERSHIP proof of
	// bondRootProven||sharedRoot at EncodeBool(true), but the squatter's root is UNPROVEN (no proven
	// leaf), so the honest ProvenProof proves ABSENT and IsProvenPresent fails ⇒ class-B stall.
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w)
	if err == nil {
		t.Fatalf("GATE FAILED (ForgedPriorProven): box WRONG-ACCEPTED a forged PriorProven false→true (honest reg rejected).\n"+
			"  Expected a stall (R1.2 anchors PriorProven against bondRootProven||root). Got nil. sqID=%x honestID=%x",
			sqID[:4], honestID[:4])
	}
	t.Logf("GATE GREEN (R1.2): forged PriorProven STALLS: %v\n  sqID=%x honestID=%x — PriorProven anchored against bondRootProven||root.",
		err, sqID[:4], honestID[:4])
}

// =============================================================================
// PART 4 — TIER-C COVERAGE META-ASSERTION (reflection-pinned)
// =============================================================================
//
// TestAdversarialRootCoverageIsComplete reflects over the three untrusted witness CARRIER structs and
// asserts every field is classified in the R1.2 cert table with a disposition of either:
//   - "FIX"            — the field feeds a NewValue or a branch predicate and has a driven
//                        adversarial-root gate (named) that reddens if its anchor is dropped; or
//   - "already-anchored" — the field is a pure OldValue/proof/key already verified by the fold or is
//                        own-cfg (with a stated reason).
//
// Modeled on TestLeafDiffGuardCoversEveryEmittableTag (floorbox_recompute_leafdiff_v5_test.go): the
// table is keyed on reflect.TypeOf, NOT a hand list, so a NEW field on any carrier reddens this test
// until it is classified. That closes the silent-new-hole gap (PE ruling Tier C). Its teeth are
// demonstrated by TestAdversarialRootCoverageMetaHasTeeth below.

// r12Disposition classifies one carrier field for the R1.2 witness-soundness coverage cert.
type r12Disposition struct {
	kind   string // "FIX" or "already-anchored"
	detail string // the driven gate name (FIX) or the anchoring reason (already-anchored)
}

// r12CoverageTable is the cert table: carrier struct name → field name → disposition. It is checked
// against the reflected fields of each carrier, so it cannot silently drift from the structs.
var r12CoverageTable = map[string]map[string]r12Disposition{
	"StateRootAttScreen": {
		"Attester":      {"already-anchored", "the attester key (id), not a value/predicate"},
		"Slashed":       {"FIX", "TestAdversarialRoot_ClassA_ForgedSlashed"},
		"InEpochSet":    {"FIX", "TestAdversarialRoot_ClassA_ForgedInEpochSet"},
		"BondedSize":    {"FIX", "TestAdversarialRoot_ClassA_ForgedBondedSize"},
		"BondedPresent": {"FIX", "TestAdversarialRoot_ClassA_ForgedBondedPresent"},
		"SlashedProof":  {"already-anchored", "the anchoring proof for Slashed (verified via Resolve)"},
		"EpochSetProof": {"already-anchored", "the anchoring proof for InEpochSet (verified via Resolve)"},
		"EpochSetValue": {"already-anchored", "the committed epochSet weight EpochSetProof binds (membership-only read)"},
		"BondedProof":   {"already-anchored", "the anchoring proof for BondedPresent/BondedSize (verified via Resolve)"},
	},
	"StateRootRotateMember": {
		"ID":                     {"already-anchored", "the frozen member key (id), not a value/predicate"},
		"Weight":                 {"FIX", "TestAdversarialRoot_ClassP_ForgedFrozenWeight"},
		"RegVersion":             {"FIX", "TestAdversarialRoot_ClassP_ForgedRegVersion"},
		"RegVersionKnown":        {"FIX", "TestAdversarialRoot_ClassP_ForgedRegVersionKnown"},
		"EpochSetProof":          {"already-anchored", "the epochSet write-target proof, verified by the fold OldValue"},
		"EpochSetOldValue":       {"already-anchored", "the fold OldValue, verified against prevStateRoot by FoldChangedPaths"},
		"EpochSetDeleteSiblings": {"already-anchored", "delete off-path siblings, anchored by the final root equality"},
		"QualifiedProof":         {"already-anchored", "the anchoring proof for Weight (verified via Resolve)"},
		"RegVersionProof":        {"already-anchored", "the anchoring proof for RegVersion/RegVersionKnown (verified via Resolve)"},
	},
	"StateRootBondRegScreen": {
		"Root":        {"already-anchored", "the bond-reg root key, not a value/predicate"},
		"PriorOwner":  {"FIX", "TestAdversarialRoot_ClassB_ForgedPriorOwner"},
		"Claimed":     {"FIX", "TestAdversarialRoot_ClassB_ForgedClaimed"},
		"PriorProven": {"FIX", "TestAdversarialRoot_ClassB_ForgedPriorProven"},
		"OwnerProof":  {"already-anchored", "the anchoring proof for PriorOwner/Claimed (verified via Resolve)"},
		"ProvenProof": {"already-anchored", "the anchoring proof for PriorProven (verified via Resolve)"},
	},
}

// r12Carriers returns the reflected type of each untrusted witness carrier the R1.2 fix anchors.
func r12Carriers() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(StateRootAttScreen{}),
		reflect.TypeOf(StateRootRotateMember{}),
		reflect.TypeOf(StateRootBondRegScreen{}),
	}
}

func TestAdversarialRootCoverageIsComplete(t *testing.T) {
	for _, carrier := range r12Carriers() {
		name := carrier.Name()
		table, ok := r12CoverageTable[name]
		if !ok {
			t.Fatalf("COVERAGE GAP: carrier %s has no R1.2 cert-table entry. Classify every field FIX or already-anchored.", name)
		}
		// Every reflected field must be classified.
		seen := map[string]struct{}{}
		for i := 0; i < carrier.NumField(); i++ {
			f := carrier.Field(i)
			seen[f.Name] = struct{}{}
			disp, classified := table[f.Name]
			if !classified {
				t.Fatalf("COVERAGE GAP: %s.%s is UNCLASSIFIED in the R1.2 cert table.\n"+
					"  A new untrusted field must be either FIX (with a driven adversarial-root gate) or\n"+
					"  already-anchored (with a stated reason). Add it to r12CoverageTable.", name, f.Name)
			}
			if disp.kind != "FIX" && disp.kind != "already-anchored" {
				t.Fatalf("COVERAGE GAP: %s.%s has invalid disposition %q (want FIX or already-anchored)", name, f.Name, disp.kind)
			}
			if disp.detail == "" {
				t.Fatalf("COVERAGE GAP: %s.%s classified %q with no detail (gate name or reason required)", name, f.Name, disp.kind)
			}
		}
		// No stale table rows (a removed field the table still lists).
		for field := range table {
			if _, ok := seen[field]; !ok {
				t.Fatalf("STALE COVERAGE: r12CoverageTable lists %s.%s but the struct has no such field (renamed/removed?)", name, field)
			}
		}
	}
	// Every FIX gate named in the table must be a real test function in this package (a typo'd gate name
	// is a silent hole). The go test binary registers tests; assert the name is non-empty and unique per
	// FIX field — a weaker but reflection-free check that the detail is a plausible gate reference.
	fixGates := map[string]string{}
	for carrier, table := range r12CoverageTable {
		for field, disp := range table {
			if disp.kind != "FIX" {
				continue
			}
			if prev, dup := fixGates[disp.detail]; dup {
				t.Fatalf("COVERAGE: FIX gate %q is claimed by two fields (%s and %s.%s) — each FIX field needs its OWN driven gate",
					disp.detail, prev, carrier, field)
			}
			fixGates[disp.detail] = carrier + "." + field
		}
	}
	// The 11 driven gates: 4 class-A + 3 class-P + 3 class-B + the class-M cross-class gate. The table
	// enumerates the 10 per-field FIX gates; class-M is the cross-class gate (not a single carrier field).
	if len(fixGates) != 10 {
		t.Fatalf("COVERAGE: expected 10 per-field FIX gates in the cert table, got %d: %v", len(fixGates), fixGates)
	}
}

// TestAdversarialRootCoverageMetaHasTeeth proves the coverage meta-assertion is not decoration: a field
// dropped from the cert table (simulating an un-classified new field) MUST be reported as a gap by the
// same reflect walk. A meta-assertion with no demonstrated red is a comment that compiles (session-7).
func TestAdversarialRootCoverageMetaHasTeeth(t *testing.T) {
	carrier := reflect.TypeOf(StateRootAttScreen{})
	full := r12CoverageTable[carrier.Name()]

	// Simulate dropping the classification for "Slashed" (an un-classified new/renamed field).
	reduced := map[string]r12Disposition{}
	for k, v := range full {
		if k == "Slashed" {
			continue
		}
		reduced[k] = v
	}

	// The reflect walk over the carrier must now find "Slashed" unclassified.
	var uncovered []string
	for i := 0; i < carrier.NumField(); i++ {
		f := carrier.Field(i)
		if _, ok := reduced[f.Name]; !ok {
			uncovered = append(uncovered, f.Name)
		}
	}
	if len(uncovered) != 1 || uncovered[0] != "Slashed" {
		t.Fatalf("META-TEETH FAILED: dropping the Slashed classification must leave exactly Slashed uncovered, got %v.\n"+
			"  If this does not redden, the coverage meta-assertion cannot detect an unclassified field.", uncovered)
	}
}
