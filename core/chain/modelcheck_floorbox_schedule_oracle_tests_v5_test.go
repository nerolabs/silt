package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// R1.5 scheduling-oracle tests. See modelcheck_floorbox_schedule_oracle_v5_test.go for the
// design header and the shared fixture builders.

// signBoundary builds a signed v5 boundary block on chain c (head is one below a boundary),
// optionally carrying a bond reg. proposer is c's proposer private key.
func signBoundary(t *testing.T, c *Chain, proposer ed25519.PrivateKey, entryByte byte, bondReg *BondReg) Block {
	t.Helper()
	prev, h := c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(entryByte)}}
	if bondReg != nil {
		b.BondRegs = append(b.BondRegs, *bondReg)
	}
	Sign(&b, proposer)
	return b
}

// =============================================================================
// SANITY — the honest recompute AGREES (the oracle's baseline; a stall here means the
// witness builder is wrong, not that a break was found).
// =============================================================================

func TestScheduleOracle_HonestBaselineAgrees(t *testing.T) {
	c, prop := unfiredLockInChain(t)
	prover, prevRoot := proverFor(t, c)
	b := signBoundary(t, c, prop, 52, nil)
	honest := committedRoot(t, c, b)
	w := boundaryWitnessFor(t, c, prover, b)
	if err := c.RecomputeStateRootEntriesRevocations(prevRoot, honest, b, w); err != nil {
		t.Fatalf("honest witness must AGREE with apply(): %v", err)
	}
}

// =============================================================================
// KNOWN COMPOUND-SHAPE BREAK (a) — class-P activation-lock LockedIn.OldValue.
// rotate_v5.go:442,:450,:458 read rw.GateLockedIn/Era3LockedIn/Era4LockedIn.OldValue as the
// "already locked in" tally gate. scalarFoldOp (:473) folds a scalar ONLY when it CHANGES, so
// a forged OldValue=true that SUPPRESSES a tally emits NO op and is never fold-checked. The box
// then computes a root that OMITS the mandatory lock-in write. An attacker who commits that
// suppressed root gets a wrong-accept.
//
// DRIVEN RED / documented-open: the recompute returns nil (wrong-accept) TODAY. When the box
// anchors the lock-in OldValue against prevStateRoot (the R1.x fix), this gate reddens with the
// fix-pointer below.
// =============================================================================

func TestScheduleOracle_OpenBreak_A_ForgedLockInOldValueSuppression(t *testing.T) {
	c, prop := unfiredLockInChain(t)
	prover, prevRoot := proverFor(t, c)
	b := signBoundary(t, c, prop, 52, nil)

	// The honest recompute agrees with apply() (which FIRES the gate+era3 lock-ins).
	honest := committedRoot(t, c, b)
	hw := boundaryWitnessFor(t, c, prover, b)
	if decodeBoolLeaf(hw.Rotate.GateLockedIn.OldValue) {
		t.Fatalf("fixture: honest GateLockedIn.OldValue must be false (tally unfired pre-boundary)")
	}
	if err := c.RecomputeStateRootEntriesRevocations(prevRoot, honest, b, hw); err != nil {
		t.Fatalf("baseline: honest witness must AGREE: %v", err)
	}

	// FORGE: claim the gate/era3 tallies are ALREADY locked in. rotateTallyOps skips the tally,
	// so no lock-in op is emitted, and the forged OldValue is never fold-checked.
	fw := boundaryWitnessFor(t, c, prover, b)
	fw.Rotate.GateLockedIn.OldValue = statehash.EncodeBool(true)
	fw.Rotate.Era3LockedIn.OldValue = statehash.EncodeBool(true)

	// The attacker commits the SUPPRESSED root — the block WITHOUT the lock-in writes.
	forgedRoot := suppressedLockInRoot(t, c, b)
	if forgedRoot == honest {
		t.Fatalf("GATE VACUOUS: forged suppressed root == honest root (the lock-in did not move the root)")
	}

	err := c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, b, fw)
	if err != nil {
		t.Fatalf("OPEN-BREAK (a) CLOSED: the box now STALLS a forged LockedIn.OldValue "+
			"suppression (%v).\n"+
			"  If this is the intended R1.x fix (anchor rw.*LockedIn.OldValue against prevStateRoot\n"+
			"  at rotate_v5.go:442,:450,:458 — fold the scalar EVEN when unchanged, or Resolve it),\n"+
			"  DELETE this documented-open gate and REPLACE it with a stall-assertion (err != nil).", err)
	}
	t.Logf("OPEN-BREAK (a) DRIVEN RED (documented-open): forged LockedIn.OldValue=true SUPPRESSES the "+
		"activation tally and WRONG-ACCEPTS a block that omitted the lock-in.\n"+
		"  site: rotate_v5.go:442,:450,:458 (gate: !decodeBoolLeaf(OldValue)) × scalarFoldOp:473 "+
		"(unchanged scalar not folded → OldValue never fold-checked).\n"+
		"  forgedRoot=%x honest=%x", forgedRoot, honest)
}

// =============================================================================
// KNOWN COMPOUND-SHAPE BREAK (b) — RegVersion in-block cross-check gap.
// apply()'s rotate tally (chain.go:3444) reads the JUST-WRITTEN c.regVersion[id] of an in-block
// bond (rotate runs LAST, after the block's bonds). The box anchors regVersion against PRE-state
// (anchorRotateMember, rotate_v5.go:355-367): a fresh in-block bond has no pre-state regVersion
// leaf, so its honest witness sets RegVersionKnown=false and the box EXCLUDES it from the tally.
// The box's tally therefore diverges from apply()'s. When the in-block bond's weight is decisive,
// apply() locks in but the box does not — so the box AGREES with an attacker who commits the
// suppressed (no-lock-in) root: a wrong-accept.
//
// DRIVEN RED / documented-open: nil (wrong-accept) TODAY. Fix pointer in the failure message.
// =============================================================================

func TestScheduleOracle_OpenBreak_B_InBlockRegVersionTallyDivergence(t *testing.T) {
	// Gate tally live; era3/era4 guarded off so the gate(>=3) tally is the only moving part.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0,
		Era4ActivationHeight: 1, Era3ActivationHeight: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(78001)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 2<<20, ports.Hash{}, 2, 1), // rv 2 < gate 3
	)
	Sign(g, prop)
	c.apply(*g)

	prev, h := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(61)}}
	Sign(&b1, prop)
	c.apply(b1)
	if c.gateLockedIn {
		t.Fatalf("fixture: gate must be UNFIRED before the boundary")
	}

	// Boundary block carrying a FRESH in-block bond at gate-ready regVersion 3, large weight —
	// so apply()'s tally (counting the in-block regVersion) locks in, but the box (excluding it)
	// does not.
	newv := key(78002)
	newvID := ports.HashBytes(pubOf(newv))
	reg := bondRegFull(newv, newvID, 16<<20, ports.Hash{}, BlockVersionRegGate, 9)
	b2 := signBoundary(t, c, prop, 62, &reg)

	clone := c.cloneForDryRun()
	clone.apply(b2)
	if !clone.gateLockedIn {
		t.Fatalf("fixture: apply(b2) must lock in the gate (in-block bond weight decisive)")
	}

	prover, prevRoot := proverFor(t, c)
	w := boundaryWitnessFor(t, c, prover, b2)
	// Confirm the box's witness excludes the in-block regVersion (RegVersionKnown=false).
	foundNew := false
	for _, m := range w.Rotate.Members {
		if m.ID == newvID {
			foundNew = true
			if m.RegVersionKnown {
				t.Fatalf("fixture: the in-block bond newv must witness RegVersionKnown=false "+
					"(no pre-state regVersion leaf); got known=%v rv=%d", m.RegVersionKnown, m.RegVersion)
			}
		}
	}
	if !foundNew {
		t.Fatalf("fixture: newv must be in the reconstructed frozen set (rotate runs LAST, freezes post-apply qualified)")
	}

	honest, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("honest root: %v", err)
	}

	// The suppressed root: post-apply state with ONLY the gate lock-in undone (era3 is guarded off).
	sup := c.cloneForDryRun()
	sup.apply(b2)
	sup.gateLockedIn = false
	sup.gateHeight = 0
	forgedRoot, err := sup.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRoot: %v", err)
	}
	if forgedRoot == honest {
		t.Fatalf("GATE VACUOUS: suppressed root == honest root")
	}

	rerr := c.RecomputeStateRootEntriesRevocations(prevRoot, forgedRoot, b2, w)
	if rerr != nil {
		t.Fatalf("OPEN-BREAK (b) CLOSED: the box now STALLS the in-block-regVersion tally "+
			"divergence (%v).\n"+
			"  If this is the intended R1.x fix (cross-check the in-block bond's regVersion against\n"+
			"  the class-B write — the just-registered version — the way Weight is cross-checked at\n"+
			"  anchorRotateMember:335-345, so the box's tally matches apply()'s at chain.go:3444),\n"+
			"  DELETE this documented-open gate and REPLACE it with a stall-assertion (err != nil).", rerr)
	}
	t.Logf("OPEN-BREAK (b) DRIVEN RED (documented-open): the box EXCLUDES an in-block bond's just-written "+
		"regVersion from the activation tally (RegVersionKnown=false), diverging from apply() "+
		"(chain.go:3444) and WRONG-ACCEPTING a block that omitted the lock-in.\n"+
		"  newv=%x forgedRoot=%x honest=%x", newvID[:4], forgedRoot, honest)
}

// =============================================================================
// ADDITION 1 — Box-as-I1-participant SCHEDULING ORACLE.
// An adversarial scheduler delivers honest witnesses to some boxes and forged witnesses to
// others, under adversarial delivery order / partition. It asserts:
//   I1 — no two disjoint boxes emit Accept for CONFLICTING committed roots at one height, when
//        the boxes are given HONEST witnesses. (This is the safety property the accept-flip must
//        preserve.)
//   I5 — an honest box (honest witness, honest committed root) is never wrongly refused/"slashed"
//        (it always Accepts the honest root, under every delivery order).
// The oracle ALSO records the pre-flip diagnostic: a box fed a FORGED witness for a suppressed
// root DOES emit Accept (the OPEN-BREAKs above), which an honest box refuses — the fork the flip
// would expose if the break is not closed first. That divergence is asserted by the OPEN-BREAK
// gates; here we assert the honest quorum does not fork.
// =============================================================================

// box is one floor-box participant: a Chain (cfg-only for the recompute) plus the pre-state root
// it holds (the attester-signed parent root).
type oracleBox struct {
	name string
	c    *Chain
	prev ports.Hash
}

// deliver is one adversarial delivery: a (box, committedRoot, witness) triple. verdict() runs the
// box's Resolve (the recompute) and returns nil (Accept) or a stall.
type oracleDelivery struct {
	box       *oracleBox
	committed ports.Hash
	witness   StateRootWitness
	block     Block
}

func (d oracleDelivery) verdict() error {
	return d.box.c.RecomputeStateRootEntriesRevocations(d.box.prev, d.committed, d.block, d.witness)
}

// TestScheduleOracle_I1_DisjointBoxesNoConflictingAccept builds a set of disjoint boxes that all
// hold the SAME pre-state (the same parent root) and are asked to Resolve the SAME height. Two
// candidate committed roots exist: the honest root and a conflicting forged (suppressed-lock-in)
// root. The scheduler delivers, under an adversarially-permuted order, honest witnesses to the
// honest quorum. I1: no two honest boxes Accept CONFLICTING roots.
func TestScheduleOracle_I1_DisjointBoxesNoConflictingAccept(t *testing.T) {
	c, prop := unfiredLockInChain(t)
	prover, prevRoot := proverFor(t, c)
	b := signBoundary(t, c, prop, 52, nil)
	honest := committedRoot(t, c, b)
	conflicting := suppressedLockInRoot(t, c, b)
	if honest == conflicting {
		t.Fatalf("fixture: honest and conflicting roots must differ")
	}

	// Three disjoint honest boxes, identical cfg, same pre-state. Each is handed the honest
	// witness + honest root, but the scheduler varies WHICH root each is ASKED to Accept:
	// the adversary tries to make one honest box Accept the conflicting root.
	boxes := []*oracleBox{
		{name: "box-A", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRoot},
		{name: "box-B", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRoot},
		{name: "box-C", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRoot},
	}
	for _, bx := range boxes {
		bx.c.SetBondVerifier(objectiveVerify)
	}

	// Adversarial schedule: box-A and box-B get honest root; box-C is ATTACKED with the
	// conflicting root (the adversary hopes the honest witness Accepts it). Delivery order is
	// permuted (C first, then A, then B) to probe order-sensitivity.
	deliveries := []oracleDelivery{
		{box: boxes[2], committed: conflicting, witness: boundaryWitnessFor(t, c, prover, b), block: b},
		{box: boxes[0], committed: honest, witness: boundaryWitnessFor(t, c, prover, b), block: b},
		{box: boxes[1], committed: honest, witness: boundaryWitnessFor(t, c, prover, b), block: b},
	}

	acceptedRoot := map[string]ports.Hash{}
	for _, d := range deliveries {
		if err := d.verdict(); err == nil {
			acceptedRoot[d.box.name] = d.committed
			t.Logf("%s ACCEPTED root %x", d.box.name, d.committed)
		} else {
			t.Logf("%s stalled: %v", d.box.name, err)
		}
	}

	// I1: no two boxes Accept CONFLICTING roots. Collect the set of accepted roots; it must be
	// a single value (or empty).
	seen := map[ports.Hash]string{}
	for name, r := range acceptedRoot {
		if other, ok := seen[r]; ok {
			_ = other
		}
		seen[r] = name
	}
	if len(seen) > 1 {
		t.Fatalf("I1 VIOLATED: disjoint honest boxes Accepted CONFLICTING roots at one height: %v\n"+
			"  Two honest boxes given honest witnesses must never fork — a divergent Accept here is a\n"+
			"  safety break the accept-flip would ship.", acceptedRoot)
	}
	// The honest quorum must Accept the honest root; box-C (attacked with the conflicting root)
	// must STALL (an honest witness cannot Accept a suppressed-lock-in root).
	if acceptedRoot["box-A"] != honest || acceptedRoot["box-B"] != honest {
		t.Fatalf("I1 baseline: box-A and box-B must Accept the honest root; got %v", acceptedRoot)
	}
	if _, cAccepted := acceptedRoot["box-C"]; cAccepted {
		t.Fatalf("I1: box-C was attacked with the CONFLICTING root and Accepted it via an HONEST witness — "+
			"an honest witness must never Accept a suppressed-lock-in root. accepted=%v", acceptedRoot)
	}
	t.Logf("I1 HELD (honest-witness quorum): the honest quorum Accepted only the honest root; the "+
		"attacked box stalled. accepted=%v", acceptedRoot)

	// DIAGNOSTIC — the fork the OPEN-BREAK enables. If box-C is fed a FORGED witness (the
	// suppression of OPEN-BREAK (a)), it DOES Accept the conflicting root. Under the accept-flip
	// that is a live I1 fork: box-C finalizes the conflicting root while box-A/box-B finalize the
	// honest root at the SAME height. The oracle asserts this fork is REACHABLE via a forged
	// witness (so the break is not benign) — and that closing OPEN-BREAK (a) is what removes it.
	forgedC := New(c.cfg, func(ports.NodeID) int64 { return 0 })
	forgedC.SetBondVerifier(objectiveVerify)
	forkErr := forgedC.RecomputeStateRootEntriesRevocations(prevRoot, conflicting, b, forgedSuppressionWitness(t, c, prover, b))
	if forkErr == nil {
		t.Logf("I1 FORK REACHABLE (pre-flip diagnostic): a FORGED witness makes box-C Accept the "+
			"CONFLICTING root %x while the honest quorum Accepts %x — the fork OPEN-BREAK (a) enables. "+
			"Closing rotate_v5.go:442/:450/:458 removes it.", conflicting, honest)
	} else {
		t.Fatalf("I1 diagnostic expected the OPEN-BREAK (a) fork to be reachable via a forged witness, "+
			"but box-C stalled (%v). If OPEN-BREAK (a) is CLOSED, update this diagnostic and the (a) gate together.", forkErr)
	}
}

// TestScheduleOracle_I5_HonestNeverSlashed asserts an honest box (honest witness + honest root)
// Accepts under EVERY adversarial delivery permutation — it is never wrongly refused. "Slashed"
// here is the box-level analogue: an honest participant's correct verdict is never suppressed by
// delivery order or by an adversary's concurrent forged deliveries to other boxes.
func TestScheduleOracle_I5_HonestNeverSlashed(t *testing.T) {
	c, prop := unfiredLockInChain(t)
	prover, prevRoot := proverFor(t, c)
	b := signBoundary(t, c, prop, 52, nil)
	honest := committedRoot(t, c, b)
	conflicting := suppressedLockInRoot(t, c, b)

	honestBox := &oracleBox{name: "honest", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRoot}
	honestBox.c.SetBondVerifier(objectiveVerify)

	// Interleave the honest delivery with adversarial forged deliveries to a scratch box, in
	// several orders. The honest box's verdict must be Accept in every case (order-independent,
	// and independent of what the adversary feeds other boxes).
	scratch := &oracleBox{name: "scratch", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRoot}
	scratch.c.SetBondVerifier(objectiveVerify)

	orderings := [][]oracleDelivery{
		{
			{box: honestBox, committed: honest, witness: boundaryWitnessFor(t, c, prover, b), block: b},
			{box: scratch, committed: conflicting, witness: forgedSuppressionWitness(t, c, prover, b), block: b},
		},
		{
			{box: scratch, committed: conflicting, witness: forgedSuppressionWitness(t, c, prover, b), block: b},
			{box: honestBox, committed: honest, witness: boundaryWitnessFor(t, c, prover, b), block: b},
		},
	}
	for i, ordering := range orderings {
		honestAccepted := false
		for _, d := range ordering {
			err := d.verdict()
			if d.box == honestBox && err == nil {
				honestAccepted = true
			}
		}
		if !honestAccepted {
			t.Fatalf("I5 VIOLATED (ordering %d): the honest box did NOT Accept its honest root — "+
				"a correct honest verdict was suppressed by delivery order or a concurrent adversarial delivery.", i)
		}
	}
	t.Logf("I5 HELD: the honest box Accepted its honest root under every adversarial delivery order.")
}

// forgedSuppressionWitness returns a boundary witness with GateLockedIn/Era3LockedIn.OldValue
// forged to true (the OPEN-BREAK (a) suppression), used as the adversary's forged delivery.
func forgedSuppressionWitness(t *testing.T, c *Chain, prover *statehash.Prover, b Block) StateRootWitness {
	t.Helper()
	w := boundaryWitnessFor(t, c, prover, b)
	w.Rotate.GateLockedIn.OldValue = statehash.EncodeBool(true)
	w.Rotate.Era3LockedIn.OldValue = statehash.EncodeBool(true)
	return w
}

// =============================================================================
// ADDITION 2 — MULTI-BLOCK Resolve schedule.
// Consecutive epoch-boundary blocks under adversarially-ordered witness delivery. Asserts:
//   - each box's Resolve verdict is STABLE under reorder (Resolve is a pure function of
//     prevStateRoot + block; permuting the delivery order of independent (root, block, witness)
//     triples cannot change any verdict).
//   - a FORGED witness at height h does NOT poison prevStateRoot for h+1's Resolve calls
//     (I3-adjacent: the box holds the attester-signed parent root for h+1 independently; a
//     forged h-witness the box stalls on never mutates the h+1 pre-state).
// =============================================================================

// TestScheduleOracle_MultiBlockResolveStableUnderReorder builds two consecutive boundary blocks
// (h and h+EpochBlocks) and confirms each box's Resolve verdict is identical whether the two
// deliveries arrive in-order or reversed. Resolve is pure over (prevStateRoot, block), so the
// order the scheduler delivers them cannot change either verdict.
func TestScheduleOracle_MultiBlockResolveStableUnderReorder(t *testing.T) {
	c, prop := unfiredLockInChain(t)

	// Block at height h (the first boundary).
	proverH, prevRootH := proverFor(t, c)
	bH := signBoundary(t, c, prop, 52, nil)
	honestH := committedRoot(t, c, bH)
	wH := boundaryWitnessFor(t, c, proverH, bH)

	// Advance the chain PAST bH to build the NEXT boundary's pre-state. apply bH, then a filler
	// block, so the head sits one below the next boundary.
	c.apply(bH)
	prevF, hF := c.Head()
	bFiller := Block{Version: BlockVersionWitnessable, Height: hF, Prev: prevF, Entries: []ports.Entry{entry(53)}}
	Sign(&bFiller, prop)
	c.apply(bFiller)

	proverH2, prevRootH2 := proverFor(t, c)
	bH2 := signBoundary(t, c, prop, 54, nil)
	honestH2 := committedRoot(t, c, bH2)
	wH2 := boundaryWitnessFor(t, c, proverH2, bH2)

	// Two independent boxes, one per height (disjoint pre-states).
	boxH := &oracleBox{name: "h", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRootH}
	boxH2 := &oracleBox{name: "h2", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRootH2}
	boxH.c.SetBondVerifier(objectiveVerify)
	boxH2.c.SetBondVerifier(objectiveVerify)

	dH := oracleDelivery{box: boxH, committed: honestH, witness: wH, block: bH}
	dH2 := oracleDelivery{box: boxH2, committed: honestH2, witness: wH2, block: bH2}

	// In-order delivery.
	inOrderH := dH.verdict()
	inOrderH2 := dH2.verdict()
	// Reversed delivery (fresh boxes to avoid any residual state).
	boxH.c = New(c.cfg, func(ports.NodeID) int64 { return 0 })
	boxH2.c = New(c.cfg, func(ports.NodeID) int64 { return 0 })
	boxH.c.SetBondVerifier(objectiveVerify)
	boxH2.c.SetBondVerifier(objectiveVerify)
	revH2 := dH2.verdict()
	revH := dH.verdict()

	if (inOrderH == nil) != (revH == nil) {
		t.Fatalf("REORDER INSTABILITY at height h: in-order verdict=%v reversed verdict=%v — "+
			"Resolve must be pure over (prevStateRoot, block); delivery order changed the verdict.", inOrderH, revH)
	}
	if (inOrderH2 == nil) != (revH2 == nil) {
		t.Fatalf("REORDER INSTABILITY at height h+1: in-order verdict=%v reversed verdict=%v", inOrderH2, revH2)
	}
	if inOrderH != nil || inOrderH2 != nil {
		t.Fatalf("baseline: both honest boundary blocks must Accept; h=%v h2=%v", inOrderH, inOrderH2)
	}
	t.Logf("REORDER STABLE: both boundary Resolve verdicts are Accept regardless of delivery order.")
}

// TestScheduleOracle_ForgedWitnessDoesNotPoisonNextPrevRoot confirms that a FORGED witness the
// box stalls on at height h does NOT alter the pre-state (prevStateRoot) the box holds for h+1's
// Resolve. The box's Resolve is stateless over its inputs — it mutates no registry — so an h+1
// Accept against the honest h+1 root is unaffected by any forged h-delivery (I3-adjacent: a
// forged block at h cannot corrupt the h+1 read-set).
func TestScheduleOracle_ForgedWitnessDoesNotPoisonNextPrevRoot(t *testing.T) {
	c, prop := unfiredLockInChain(t)

	// Height-h boundary + its FORGED (suppressed-lock-in) delivery.
	proverH, prevRootH := proverFor(t, c)
	bH := signBoundary(t, c, prop, 52, nil)
	forgedRootH := suppressedLockInRoot(t, c, bH)
	forgedWH := forgedSuppressionWitness(t, c, proverH, bH)

	// Advance to the NEXT boundary's pre-state (apply the HONEST bH, then a filler).
	c.apply(bH)
	prevF, hF := c.Head()
	bFiller := Block{Version: BlockVersionWitnessable, Height: hF, Prev: prevF, Entries: []ports.Entry{entry(53)}}
	Sign(&bFiller, prop)
	c.apply(bFiller)

	proverH2, prevRootH2 := proverFor(t, c)
	bH2 := signBoundary(t, c, prop, 54, nil)
	honestH2 := committedRoot(t, c, bH2)
	wH2 := boundaryWitnessFor(t, c, proverH2, bH2)

	boxH := &oracleBox{name: "h", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRootH}
	boxH2 := &oracleBox{name: "h2", c: New(c.cfg, func(ports.NodeID) int64 { return 0 }), prev: prevRootH2}
	boxH.c.SetBondVerifier(objectiveVerify)
	boxH2.c.SetBondVerifier(objectiveVerify)

	// h+1's HONEST Accept, WITHOUT any prior delivery.
	baseline := boxH2.c.RecomputeStateRootEntriesRevocations(boxH2.prev, honestH2, bH2, wH2)
	if baseline != nil {
		t.Fatalf("baseline: honest h+1 boundary must Accept; got %v", baseline)
	}

	// Now deliver the FORGED h-witness to boxH FIRST (the adversary tries to poison the next
	// read-set), then re-run h+1's honest Accept on boxH2.
	_ = boxH.c.RecomputeStateRootEntriesRevocations(boxH.prev, forgedRootH, bH, forgedWH)
	after := boxH2.c.RecomputeStateRootEntriesRevocations(boxH2.prev, honestH2, bH2, wH2)
	if (baseline == nil) != (after == nil) {
		t.Fatalf("POISON: a forged h-delivery changed h+1's honest verdict (baseline=%v after=%v) — "+
			"prevStateRoot for h+1 must be independent of any h-delivery (I3-adjacent).", baseline, after)
	}
	// The h+1 pre-state root the box holds is unchanged by the forged h-delivery.
	if boxH2.prev != prevRootH2 {
		t.Fatalf("POISON: the h+1 prevStateRoot mutated after a forged h-delivery — %x != %x", boxH2.prev, prevRootH2)
	}
	t.Logf("NO POISON: a forged h-delivery left h+1's honest Accept and pre-state root unchanged.")
}
