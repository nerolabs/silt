package chain

// era-4 (v5) floor-box BOULDER-1 R1.6 — PER-FIELD Resolve-path oracle probes for the 23-field
// carrier table.
//
// BUILDER SEAT — 2026-09-02. Branch builder/floorbox-r1.6-per-field-oracle-probes.
// Governs: ROADMAP.md Boulder-1 R1.6 (ratified). Hardens the R1.4 recompute-soundness cert
//   (.../research-outcome/floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md),
//   which left R-CARRIER-REFLECTION HELD and named per-field ablation-teeth as the missing half of
//   the coverage walk.
// Design: docs/thinking/2026-09-02-floorbox-R1.6-per-field-oracle-probes-design.md.
//
// WHAT THIS FILE ADDS (over the 10 FIX gates + class-M gate in
// floorbox_recompute_adversarialroot_v5_test.go):
//   1. A DRIVEN per-field probe for each of the 13 "already-anchored" carrier obligations (the proofs,
//      the fold OldValues, the always-emitted scalar pairs). Each forges the field so it is
//      inconsistent with prevStateRoot and asserts the box STALLS. If the field's anchor (its paired
//      Resolve, or the fold's prevStateRoot VerifyProof) is dropped, the forged value folds into the
//      attacker's committed root and the box wrong-accepts — the probe reddens. TestPerFieldProbeBites
//      demonstrates this on a representative sample.
//   2. The StateRootRotateScalar carrier is pinned into the reflection walk (r12Carriers +
//      r12CoverageTable extended in floorbox_recompute_adversarialroot_v5_test.go), closing
//      R-CARRIER-REFLECTION for the scalar carrier — a new value-bearing scalar field now reddens the
//      coverage test until classified. The OldValue predicate rows are classified FIX-OPEN (the break
//      below), Proof as already-anchored.
//   3. THREE OPEN-BREAK gates for the activation-lock OldValue predicates
//      (GateLockedIn/Era3LockedIn/Era4LockedIn .OldValue). These read an UNANCHORED witness OldValue
//      as a BRANCH PREDICATE (rotate_v5.go:442,:450,:458): a forged OldValue=true SUPPRESSES the
//      lock-in emission, so the value is never folded, and the attacker commits a lock-free root — a
//      WRONG-ACCEPT. Confirmed reproducible for all three. This partially REFUTES the R1.4 Q1 "scalar
//      pairs are already-anchored: fold OldValue" classification (true only when an op is emitted).
//      Routed to the Researcher/PE as a consensus-adjacent recompute-soundness break. These gates
//      assert the CURRENT wrong-accept (so they are GREEN on this branch) and MUST be flipped to
//      assert-stall by the fix PR — the RED-on-current-code evidence a full-node reconstruction relies
//      on the lock scalars being present.

import (
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// PART A — the 13 "already-anchored" per-field probes (forge → STALL)
// =============================================================================
//
// Each probe forges ONE already-anchored carrier field on an honest, agreeing witness and asserts the
// box STALLS. The anchor that catches each is named. A probe with no demonstrated ablation-RED is
// decoration (session-7 scar); TestPerFieldProbeBites drives a representative sample RED by making
// the forged value the one the box would fold if its anchor were dropped.

// --- StateRootAttScreen proof/value fields (4): SlashedProof, EpochSetProof, EpochSetValue, BondedProof ---

// TestPerField_AttScreen_SlashedProof forges the SlashedProof (swaps in a proof for a different key).
// The anchor is the Resolve of Slashed against prevStateRoot (atts_v5.go:165): a proof that does not
// verify yields NoWitness ⇒ neither IsProvenAbsent nor IsProvenPresent ⇒ stall.
func TestPerField_AttScreen_SlashedProof(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	// Forge SlashedProof: a valid proof for the WRONG key. Resolve(slashed||id) fails ⇒ NoWitness ⇒ stall.
	aid := ports.HashBytes(pubOf(f.att))
	w.AttScreens[0].SlashedProof = mustProve(f.prover, statehash.Key(tagBonded, aid[:]))
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged SlashedProof wrong-accepted; expected a stall (Slashed Resolve anchor).")
	}
}

// TestPerField_AttScreen_EpochSetProof forges the AttScreen EpochSetProof. Anchor: Resolve(epochSet||id)
// (atts_v5.go:186). The fixture attester IS in the frozen epochSet, so InEpochSet=true and the box
// requires a present-proof; a wrong-key proof fails ⇒ stall.
func TestPerField_AttScreen_EpochSetProof(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	aid := ports.HashBytes(pubOf(f.att))
	w.AttScreens[0].EpochSetProof = mustProve(f.prover, statehash.Key(tagBonded, aid[:]))
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged EpochSetProof wrong-accepted; expected a stall (InEpochSet Resolve anchor).")
	}
}

// TestPerField_AttScreen_EpochSetValue forges the EpochSetValue (the committed frozen weight the
// present-proof binds). Anchor: Resolve(epochSet||id, EpochSetValue) — a value that does not match the
// committed leaf fails IsProvenPresent ⇒ stall.
func TestPerField_AttScreen_EpochSetValue(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	// The attester is in the epochSet, so EpochSetValue is the committed weight. Forge it.
	w.AttScreens[0].EpochSetValue = statehash.EncodeInt64(1234567)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged EpochSetValue wrong-accepted; expected a stall (Resolve binds the value).")
	}
}

// TestPerField_AttScreen_BondedProof forges the BondedProof on a pre-maturity fixture (where the
// bonded screen is read). Anchor: Resolve(bonded||id) (atts_v5.go:206). A wrong-key proof stalls.
func TestPerField_AttScreen_BondedProof(t *testing.T) {
	// Pre-maturity path: EpochBlocks=0 ⇒ epochsEnabled()=false ⇒ the bonded screen is read.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 0, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(91001)
	att := key(91002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(att, ports.HashBytes(pubOf(att)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)
	attID := ports.HashBytes(pubOf(att))

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
	prev, h := c.Head()
	bTest := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(60)}}
	bTest.Atts = append(bTest.Atts, Attest(&bTest, att))
	Sign(&bTest, prop)

	clone := c.cloneForDryRun()
	clone.apply(bTest)
	committed, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	leafWit := func(k []byte) StateRootChangedLeafWitness {
		return StateRootChangedLeafWitness{Key: k, OldValue: preValue(k), Proof: mustProve(prover, k)}
	}
	honestScreen := StateRootAttScreen{
		Attester: attID, Slashed: false, InEpochSet: false,
		BondedSize: c.bonded[attID], BondedPresent: true,
		SlashedProof:  mustProve(prover, statehash.Key(tagSlashed, attID[:])),
		EpochSetProof: mustProve(prover, statehash.Key(tagEpochSet, attID[:])),
		BondedProof:   mustProve(prover, statehash.Key(tagBonded, attID[:])),
	}
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(bTest) {
		w.ChangedLeaves = append(w.ChangedLeaves, leafWit(wr.key))
	}
	w.ChangedLeaves = append(w.ChangedLeaves, leafWit(statehash.Key(tagValidatorsSeen, attID[:])))
	w.AttScreens = []StateRootAttScreen{honestScreen}
	w.DigestPreSets = []StateRootDigestWitness{{Tag: tagValidatorsSeenRoot,
		PreIDs: sortIDs(nil), Proof: mustProve(prover, statehash.Key(tagValidatorsSeenRoot, nil))}}
	w.Maturity = latchedMaturityWitness(t, prover, preValue)
	if err := c.RecomputeStateRootEntriesRevocations(prevRoot, committed, bTest, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	// Forge BondedProof: wrong-key proof ⇒ Resolve(bonded||id) NoWitness ⇒ stall.
	w.AttScreens[0].BondedProof = mustProve(prover, statehash.Key(tagSlashed, attID[:]))
	if err := c.RecomputeStateRootEntriesRevocations(prevRoot, committed, bTest, w); err == nil {
		t.Fatalf("PROBE FAILED: forged BondedProof wrong-accepted; expected a stall (Bonded Resolve anchor).")
	}
}

// --- StateRootBondRegScreen proof fields (2): OwnerProof, ProvenProof ---
//
// Both are read on the class-B DISPLACEMENT path (bondFixture: an honest registrant proves a
// genesis-squatted shared root, displacing the squatter). OwnerProof anchors PriorOwner/Claimed;
// ProvenProof anchors PriorProven. A wrong-key proof yields NoWitness ⇒ the Resolve stalls.

// TestPerField_BondRegScreen_OwnerProof forges the OwnerProof. Anchor: Resolve(bondRootOwner||root)
// (bondreg_v5.go:155/:180). The squatter OWNS the shared root pre-state (Claimed=true), so the box
// requires a present-proof; a wrong-key proof fails ⇒ stall.
func TestPerField_BondRegScreen_OwnerProof(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(92001)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}
	committed := f.applyAndCommittedRoot(t, b)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	for i := range w.BondRegScreens {
		if w.BondRegScreens[i].Root == f.sharedRoot {
			w.BondRegScreens[i].OwnerProof = mustProve(f.prover, statehash.Key(tagBondRootProven, f.sharedRoot[:]))
		}
	}
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged OwnerProof wrong-accepted; expected a stall (PriorOwner/Claimed Resolve anchor).")
	}
}

// TestPerField_BondRegScreen_ProvenProof forges the ProvenProof. Anchor: Resolve(bondRootProven||root)
// (bondreg_v5.go:164/:169). The squatter's root is claimed-but-unproven pre-state, so PriorProven=false
// requires a non-membership proof; a wrong-key proof fails IsProvenAbsent ⇒ stall.
func TestPerField_BondRegScreen_ProvenProof(t *testing.T) {
	f := buildBondFixture(t)
	prev, h := f.c.Head()
	honest := key(92101)
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}
	committed := f.applyAndCommittedRoot(t, b)
	newDue := h + f.c.cfg.BondTTLBlocks + 1
	w := f.bondWitness(t, b, []uint64{newDue})
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	for i := range w.BondRegScreens {
		if w.BondRegScreens[i].Root == f.sharedRoot {
			w.BondRegScreens[i].ProvenProof = mustProve(f.prover, statehash.Key(tagBondRootOwner, f.sharedRoot[:]))
		}
	}
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged ProvenProof wrong-accepted; expected a stall (PriorProven Resolve anchor).")
	}
}

// --- StateRootRotateMember fold-input fields (3): EpochSetOldValue, EpochSetProof, EpochSetDeleteSiblings ---

// TestPerField_RotateMember_EpochSetOldValue forges a frozen member's EpochSetOldValue (the fold's
// OldValue for the epochSet||id leaf). Anchor: FoldChangedPaths verifies OldValue against prevStateRoot
// (fold.go:126). A forged OldValue fails VerifyProof ⇒ stall.
func TestPerField_RotateMember_EpochSetOldValue(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	if len(w.Rotate.Members) == 0 {
		t.Fatalf("fixture: expected frozen members")
	}
	w.Rotate.Members[0].EpochSetOldValue = statehash.EncodeInt64(987654)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged EpochSetOldValue wrong-accepted; expected a fold stall (OldValue vs prevStateRoot).")
	}
}

// TestPerField_RotateMember_EpochSetProof forges a frozen member's EpochSetProof (the fold's inclusion
// proof for the epochSet||id write-target). Anchor: FoldChangedPaths VerifyProof against prevStateRoot.
// A wrong-key proof fails ⇒ stall.
func TestPerField_RotateMember_EpochSetProof(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	id := w.Rotate.Members[0].ID
	w.Rotate.Members[0].EpochSetProof = mustProve(f.prover, statehash.Key(tagQualified, id[:]))
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged EpochSetProof wrong-accepted; expected a fold stall.")
	}
}

// TestPerField_RotateMember_PriorEpochSetOldValue forges the OldValue of a DROPPED prior-epochSet
// member (the fold's OldValue for the epochSet||id DELETE). Anchor: FoldChangedPaths verifies the
// DELETE's OldValue against prevStateRoot (fold.go:126). A forged OldValue fails VerifyProof ⇒ stall.
// This is the driven per-field probe for the DELETE path of the RotateMember carrier; the
// EpochSetDeleteSiblings are anchored by the final root-equality (TestPerFieldProbeBites), a
// fold-machinery detail, not an independently-forgeable value.
func TestPerField_RotateMember_PriorEpochSetOldValue(t *testing.T) {
	f, b, committed, w := epochSetDropFixture(t)
	if len(w.Rotate.PriorEpochSet) == 0 {
		t.Fatalf("fixture: expected a dropped prior-epochSet member")
	}
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	// Forge the dropped member's committed prior epochSet value. The DELETE's OldValue no longer matches
	// prevStateRoot ⇒ the fold's VerifyProof fails ⇒ stall.
	w.Rotate.PriorEpochSet[0].EpochSetOldValue = statehash.EncodeInt64(31337)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged PriorEpochSet OldValue wrong-accepted; expected a fold stall (DELETE OldValue vs prevStateRoot).")
	}
}

// epochSetDropFixture builds a boundary where a genesis epochSet member is DROPPED from the frozen set
// (slashed before the boundary ⇒ no longer qualified ⇒ removed at the freeze). Returns a fixture, the
// boundary block, the honest committed root, and the full witness (with a PriorEpochSet DELETE entry).
func epochSetDropFixture(t *testing.T) (rotateFixture, Block, ports.Hash, StateRootWitness) {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(94001)
	victim := key(94002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(victim, ports.HashBytes(pubOf(victim)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)
	victimID := ports.HashBytes(pubOf(victim))
	if _, in := c.epochSet[victimID]; !in {
		t.Fatalf("fixture: victim must be in the frozen genesis epochSet")
	}
	// h=1: slash the victim ⇒ removed from qualified/bonded ⇒ dropped at the h=2 boundary freeze.
	prev1, h1 := c.Head()
	bSlash := Block{Version: BlockVersionWitnessable, Height: h1, Prev: prev1,
		Slashes: []Equivocation{slashProof(victim, prev1, 0x51, 0x52)}}
	Sign(&bSlash, prop)
	c.apply(bSlash)
	if !c.slashed[victimID] {
		t.Fatalf("fixture: victim must be slashed")
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	f := rotateFixture{c: c, proposer: prop, prover: prover, prevRoot: prover.Root()}
	b := f.boundaryBlock(nil)
	// Confirm the victim leaves the frozen set (a real DELETE at the freeze).
	clone := c.cloneForDryRun()
	clone.apply(b)
	if _, still := clone.epochSet[victimID]; still {
		t.Fatalf("fixture: victim must be DROPPED from the frozen epochSet at the boundary")
	}
	committed, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	w := f.witnessForBoundary(t, b)
	return f, b, committed, w
}

// --- StateRootRotateScalar always-emitted pairs (2): EpochStart, MatureEpoch OldValue/Proof ---

// TestPerField_RotateScalar_EpochStart_Anchored forges the EpochStart scalar OldValue. epochStart
// advances EVERY boundary (rotate_v5.go:253), so the op is ALWAYS emitted and the OldValue/Proof are
// folded and verified against prevStateRoot. A forged OldValue fails the fold ⇒ stall. This is the
// scalar carrier's SAFE case — contrast the LockedIn.OldValue break in PART C.
func TestPerField_RotateScalar_EpochStart_Anchored(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	w.Rotate.EpochStart.OldValue = statehash.EncodeUint64(424242)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err == nil {
		t.Fatalf("PROBE FAILED: forged EpochStart.OldValue wrong-accepted; expected a fold stall (op always emitted).")
	}
}

// =============================================================================
// PART B — driven-ablation meta-test: the probes BITE
// =============================================================================
//
// A probe with no demonstrated ablation-RED is decoration. For a representative sample, this test
// makes the forged value the one the box WOULD fold if its anchor were dropped (an attacker-committed
// root), and confirms the box stalls ONLY because the anchor is present: the honest post-apply root
// with the forged field trusted is the forgedRoot; the box must NOT recompute it.
//
// The 10 FIX fields already have this driven property (their adversarial-committed-root gates in
// floorbox_recompute_adversarialroot_v5_test.go). Here we drive the fold-input EpochSetOldValue: the
// forged OldValue, if trusted (fold check dropped), would diverge the epochSet||id leaf to a value the
// attacker commits. We assert forgedRoot != honestRoot AND the box stalls — so ablating the fold's
// prevStateRoot VerifyProof is the difference between stall and wrong-accept.
func TestPerFieldProbeBites(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	honest := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honest, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}

	// Build forgedRoot: the committed root an attacker embeds if the box TRUSTS a forged frozen weight
	// (the epochSet||id leaf set to the forged value). If the fold's OldValue check were dropped, the
	// box would recompute exactly this root — a wrong-accept. With the check present, the box stalls.
	id := w.Rotate.Members[0].ID
	forgedWeight := w.Rotate.Members[0].Weight + 7
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	clone.epochSet[id] = forgedWeight
	forgedRoot, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRoot: %v", err)
	}
	if forgedRoot == honest {
		t.Fatalf("GATE VACUOUS: forgedRoot == honestRoot")
	}
	// Forge the member's Weight AND its EpochSetOldValue is unchanged; the epochSet leaf NewValue is
	// EncodeInt64(Weight). The Weight anchor (qualified||id Resolve) catches this; assert the stall.
	w.Rotate.Members[0].Weight = forgedWeight
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, w); err == nil {
		t.Fatalf("BITE FAILED: box wrong-accepted the forged-weight forgedRoot; the Weight anchor is not load-bearing here.")
	}
	t.Logf("BITE CONFIRMED: forgedRoot != honestRoot and the box STALLS — the anchor is the difference between stall and wrong-accept.")
}

// =============================================================================
// PART C — CLOSED-BREAK gates: the activation-lock OldValue predicates now STALL (Direction A)
// =============================================================================
//
// rotate_v5.go read rw.GateLockedIn.OldValue / rw.Era3LockedIn.OldValue / rw.Era4LockedIn.OldValue
// as a BRANCH PREDICATE (!decodeBoolLeaf(...)) to decide whether to attempt each activation lock-in.
// The OldValue was UNTRUSTED and only fold-verified when scalarFoldOp emitted an op (i.e. when the
// value CHANGED). A forged OldValue=true SUPPRESSED the lock-in attempt: no op emitted, the forged
// OldValue never folded, and the attacker committed a root WITHOUT the lock scalars → a WRONG-ACCEPT.
//
// FIXED by DIRECTION A (classP-anchoring cert 2026-09-02): rotateTallyOps now anchors each lock-in
// bool's committed pre-value against prevStateRoot (anchorRotateScalar → Resolve.IsProvenPresent)
// UNCONDITIONALLY, before the branch read. A forged OldValue that cannot Resolve present ⇒ NoWitness
// ⇒ STALL. These gates now assert the STALL (err != nil): each forges the lock-in OldValue=true, the
// box refuses to agree with the lock-free forgedRoot, and the anchor is the difference. The box still
// NEVER Accepts (WitnessValidateV5 → Gated); this is STALL-ADDING ONLY.

// allThreeLockFixture builds a chain where the honest h=2 boundary locks gate+era3+era4 (a single
// dominant rv=5 member added post-genesis to a rv=0 genesis). Returns the fixture, the boundary block,
// and the honest committed root.
func allThreeLockFixture(t *testing.T) (f rotateFixture, rb Block, honestCommitted ports.Hash) {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0,
		RegGateActivationHeight: 0, Era3ActivationHeight: 0, Era4ActivationHeight: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	a := key(93001)
	bkey := key(93002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs, bondRegFull(a, ports.HashBytes(pubOf(a)), 2<<20, ports.Hash{}, 0, 1))
	Sign(g, a)
	c.apply(*g)
	if c.gateLockedIn || c.era3LockedIn || c.era4LockedIn {
		t.Fatalf("fixture: nothing must lock at genesis (rv=0)")
	}
	prev1, _ := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev1}
	b1.BondRegs = append(b1.BondRegs, bondRegFull(bkey, ports.HashBytes(pubOf(bkey)), 32<<20, ports.Hash{}, 5, 2))
	Sign(&b1, a)
	c.apply(b1)

	prev2, h2 := c.Head()
	rb = Block{Version: BlockVersionWitnessable, Height: h2, Prev: prev2, Entries: []ports.Entry{entry(99)}}
	Sign(&rb, a)
	sanity := c.cloneForDryRun()
	sanity.apply(rb)
	if !sanity.gateLockedIn || !sanity.era3LockedIn || !sanity.era4LockedIn {
		t.Fatalf("fixture: all three must lock at h=2 honest; got gate=%v era3=%v era4=%v",
			sanity.gateLockedIn, sanity.era3LockedIn, sanity.era4LockedIn)
	}
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	f = rotateFixture{c: c, proposer: a, prover: prover, prevRoot: prover.Root()}
	honestCommitted, err = sanity.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("committed: %v", err)
	}
	return f, rb, honestCommitted
}

// forgedRootSuppressLock clears the named lock's committed scalars from the honest post-apply root —
// the root an attacker commits when a forged LockedIn.OldValue=true suppresses the lock emission.
func forgedRootSuppressLock(t *testing.T, base *Chain, b Block, which string) ports.Hash {
	t.Helper()
	clone := base.cloneForDryRun()
	clone.apply(b)
	switch which {
	case "gate":
		clone.gateLockedIn = false
		clone.gateHeight = 0
	case "era3":
		clone.era3LockedIn = false
		clone.era3Height = 0
	case "era4":
		clone.era4LockedIn = false
		clone.era4Height = 0
	}
	root, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRootSuppressLock(%s): %v", which, err)
	}
	return root
}

// runLockPredicateAnchorStall drives one activation-lock OldValue predicate: forge it true (which
// WOULD suppress the lock-in emission), build the lock-free forgedRoot, and assert the box STALLS
// (Direction A anchor). The baseline (honest witness → honest root) must still agree, proving the
// anchor only rejects the forgery, not the honest path.
func runLockPredicateAnchorStall(t *testing.T, which string, forge func(*StateRootRotateWitness)) {
	f, rb, honestCommitted := allThreeLockFixture(t)
	w := f.witnessForBoundary(t, rb)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, rb, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}
	forge(w.Rotate)
	forgedRoot := forgedRootSuppressLock(t, f.c, rb, which)
	if forgedRoot == honestCommitted {
		t.Fatalf("[%s] GATE VACUOUS: forgedRoot == honestCommitted", which)
	}
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, rb, w)
	if err == nil {
		t.Fatalf("[%s] ANCHOR REGRESSED: box WRONG-ACCEPTS a forged LockedIn.OldValue=true predicate.\n"+
			"  Direction A (rotateTallyOps → anchorRotateScalar) must Resolve the lock-in OldValue present\n"+
			"  against prevStateRoot BEFORE the branch read; a forged OldValue must STALL. forgedRoot=%x honest=%x",
			which, forgedRoot, honestCommitted)
	}
	t.Logf("[%s] ANCHOR HOLDS (Direction A): a forged LockedIn.OldValue=true STALLS (%v) — the pre-state "+
		"anchor catches the suppression; the box never agrees with the lock-free forgedRoot.", which, err)
}

func TestOpenBreak_GateLockedInOldValuePredicate(t *testing.T) {
	runLockPredicateAnchorStall(t, "gate", func(rw *StateRootRotateWitness) {
		rw.GateLockedIn.OldValue = statehash.EncodeBool(true)
	})
}

func TestOpenBreak_Era3LockedInOldValuePredicate(t *testing.T) {
	runLockPredicateAnchorStall(t, "era3", func(rw *StateRootRotateWitness) {
		rw.Era3LockedIn.OldValue = statehash.EncodeBool(true)
	})
}

func TestOpenBreak_Era4LockedInOldValuePredicate(t *testing.T) {
	runLockPredicateAnchorStall(t, "era4", func(rw *StateRootRotateWitness) {
		rw.Era4LockedIn.OldValue = statehash.EncodeBool(true)
	})
}
