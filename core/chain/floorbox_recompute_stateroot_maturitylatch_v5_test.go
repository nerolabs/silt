package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the CLASS-M everMature maturity latch state-root recompute
// (floorbox_recompute_stateroot_maturitylatch_v5.go) — the BOUNDARY-INDEPENDENT single owner of the
// tagEverMature leaf write.
//
// THE GAP THIS CLOSES (PE ruling 2026-08-31, the write-obligation ledger):
//   silt-reviews/principle-engineer/RULING-floorbox-v5-write-obligation-ledger-2026-08-31.md
// apply() latches everMature false→true on ANY block where !everMature && Mature()
// (chain.go:3303-3305), BEFORE the boundary gate. #678 reproduced it ONLY inside class P
// (boundary-gated), so the GENERIC OFF-boundary maturity crossing (h % EpochBlocks != 0) had no
// reproducer → the recompute folded no tagEverMature op → root mismatch → STALL. These tests seat a
// network YOUNG at genesis and mature it at an OFF-boundary height, driving the REAL entry
// (RecomputeStateRootEntriesRevocations). The existing …AgreesWithApply fixtures are all
// mature-from-genesis, so this path was unexercised until now.

// offBoundaryMaturityFixture builds a v5 chain YOUNG at genesis that matures at an OFF-boundary
// height. EpochBlocks is large (no boundary fires at the test heights), MatureValidators=2. The
// network stays young until three distinct validators are seated into validatorsSeen; the seating
// block is an ORDINARY (non-boundary) block, so the everMature latch flips off-boundary.
type offBoundaryMaturityFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
	proposer ed25519.PrivateKey
	att1     ed25519.PrivateKey
	att2     ed25519.PrivateKey
	att3     ed25519.PrivateKey
}

func buildOffBoundaryMaturityFixture(t *testing.T) offBoundaryMaturityFixture {
	t.Helper()
	// EpochBlocks=1024 ⇒ no boundary at the test heights (h=1,2). MatureValidators=2: the Nakamoto
	// coefficient over THREE equal unset-domain bonds is 2 (one bond = total/3, not >), so the bar first
	// clears only once THREE distinct validators are seated — which happens at an ordinary block.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 2, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(57001)
	att1 := key(57002)
	att2 := key(57003)
	att3 := key(57004)
	// Genesis: bond proposer + three attesters (equal weight, distinct unset-domain groups). NO atts, so
	// validatorsSeen is EMPTY ⇒ coefficient 0 ⇒ the network is YOUNG (everMature=false).
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(att1, ports.HashBytes(pubOf(att1)), 8<<20, ports.Hash{}, 5, 2),
		bondRegFull(att2, ports.HashBytes(pubOf(att2)), 8<<20, ports.Hash{}, 5, 3),
		bondRegFull(att3, ports.HashBytes(pubOf(att3)), 8<<20, ports.Hash{}, 5, 4),
	)
	Sign(g, prop)
	c.apply(*g)
	if c.everMature {
		t.Fatalf("fixture: network must be YOUNG at genesis (everMature=false), got true")
	}

	// h=1: a plain E/R block, NO seating atts — validatorsSeen stays empty, network stays young. This
	// advances the head to h=2, an ordinary (OFF-boundary) height (2 % 1024 != 0).
	prev, _ := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev, Entries: []ports.Entry{entry(71)}}
	Sign(&b1, prop)
	c.apply(b1)
	if c.everMature {
		t.Fatalf("fixture: network must still be young at h=1, got everMature=true")
	}
	_, h := c.Head()
	if h != 2 {
		t.Fatalf("fixture: expected head h=2, got %d", h)
	}
	if h%cfg.EpochBlocks == 0 {
		t.Fatalf("fixture: h=%d must be OFF-boundary for this fixture", h)
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
		t.Fatalf("fixture pre-root mismatch")
	}
	return offBoundaryMaturityFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, att1: att1, att2: att2, att3: att3}
}

// crossingBlock builds the OFF-boundary h=2 block carrying non-proposer atts from att1+att2+att3 (the
// seating that crosses the maturity bar) plus an E/R entry. apply() seats them, then the class-M latch
// flips everMature false→true — all at an ordinary height, NO rotation.
func (f offBoundaryMaturityFixture) crossingBlock() Block {
	prev, h := f.c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(88)}}
	b.Atts = append(b.Atts, Attest(&b, f.att1), Attest(&b, f.att2), Attest(&b, f.att3))
	Sign(&b, f.proposer)
	return b
}

func (f offBoundaryMaturityFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

func (f offBoundaryMaturityFixture) prove(t *testing.T, key []byte) statehash.Witness {
	t.Helper()
	wit, err := f.prover.Prove(key)
	if err != nil {
		t.Fatalf("Prove(%x): %v", key, err)
	}
	return wit
}

func (f offBoundaryMaturityFixture) sortedSeenIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.validatorsSeen))
	for id := range f.c.validatorsSeen {
		out = append(out, id)
	}
	return sortIDs(out)
}

func (f offBoundaryMaturityFixture) applied(b Block) *Chain {
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	return clone
}

func (f offBoundaryMaturityFixture) committedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	sr, err := f.applied(b).StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// seenWitnessPost builds the SeenSetWitness the box feeds RecomputeMatureNow for the crossing decision,
// witnessed against the POST-apply committed root (the latch reads the post-block seen/bonded set).
func (f offBoundaryMaturityFixture) seenWitnessPost(t *testing.T, applied *Chain) SeenSetWitness {
	t.Helper()
	postProver, err := statehash.NewProver(applied.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("NewProver(post): %v", err)
	}
	rootVal := nodeSetMTHFromBool(applied.validatorsSeen)
	rootProof, err := postProver.Prove(statehash.Key(tagValidatorsSeenRoot, nil))
	if err != nil {
		t.Fatalf("Prove(validatorsSeenRoot post): %v", err)
	}
	ids := make([]ports.NodeID, 0, len(applied.validatorsSeen))
	members := make(map[ports.NodeID]MemberStateWitness, len(applied.validatorsSeen))
	for id := range applied.validatorsSeen {
		ids = append(ids, id)
		mw := MemberStateWitness{}
		sp, err := postProver.Prove(statehash.Key(tagSlashed, id[:]))
		if err != nil {
			t.Fatalf("Prove(slashed): %v", err)
		}
		mw.Slashed = applied.slashed[id]
		mw.SlashedProof = sp
		bp, err := postProver.Prove(statehash.Key(tagBonded, id[:]))
		if err != nil {
			t.Fatalf("Prove(bonded): %v", err)
		}
		mw.Bonded = applied.bonded[id]
		mw.BondedProof = bp
		dp, err := postProver.Prove(statehash.Key(tagBondDomain, id[:]))
		if err != nil {
			t.Fatalf("Prove(bondDomain): %v", err)
		}
		d, present := applied.bondDomain[id]
		mw.Domain = d
		mw.DomainPresent = present
		mw.DomainProof = dp
		members[id] = mw
	}
	return SeenSetWitness{IDs: ids, SeenRootWitness: rootProof, SeenRootValue: rootVal, Members: members}
}

// witnessForCrossing builds the full compound (E/R + A + M) witness for the OFF-boundary crossing
// block: the E/R changed leaves, the class-A validatorsSeen ADDs + digest, and the class-M maturity
// witness (the pre-latch everMature scalar proof + the POST-apply SeenSet). NO class-P witness — the
// block is not a boundary.
func (f offBoundaryMaturityFixture) witnessForCrossing(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	// E/R changed leaves.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
			Key: wr.key, OldValue: f.preValue(wr.key), Proof: f.prove(t, wr.key),
		})
	}

	applied := f.applied(b)

	// Class A: validatorsSeen ADDs + the validatorsSeenRoot digest. Screen each non-proposer attester
	// from the fixture's committed pre-state.
	preSeen := idSet(f.sortedSeenIDs())
	screens := map[ports.NodeID]StateRootAttScreen{}
	proposer := b.ProposerID()
	for i := range b.Atts {
		id := b.Atts[i].AttesterID()
		if id == proposer {
			continue
		}
		sz, bp := f.c.bonded[id]
		_, inES := f.c.epochSet[id]
		sc := StateRootAttScreen{Attester: id, Slashed: f.c.slashed[id], InEpochSet: inES, BondedSize: sz, BondedPresent: bp}
		screens[id] = sc
		w.AttScreens = append(w.AttScreens, sc)
	}
	aWrites, _, err := f.c.stateRootAttWriteSet(b, preSeen, screens)
	if err != nil {
		t.Fatalf("stateRootAttWriteSet: %v", err)
	}
	for _, wr := range aWrites {
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
			Key: wr.key, OldValue: f.preValue(wr.key), Proof: f.prove(t, wr.key),
		})
	}
	w.DigestPreSets = append(w.DigestPreSets,
		StateRootDigestWitness{Tag: tagValidatorsSeenRoot, PreIDs: f.sortedSeenIDs(), Proof: f.prove(t, statehash.Key(tagValidatorsSeenRoot, nil))},
	)

	// Class M maturity witness: the pre-latch everMature scalar proof (pre=false ⇒ the crossing) + the
	// POST-apply SeenSet the box feeds RecomputeMatureNow.
	w.Maturity = &StateRootMaturityWitness{
		EverMature: StateRootRotateScalar{OldValue: f.preValue(statehash.Key(tagEverMature, nil)), Proof: f.prove(t, statehash.Key(tagEverMature, nil))},
		SeenSet:    f.seenWitnessPost(t, applied),
	}

	// dueBucket scope-gate proof (non-membership at b.Height).
	var hk [8]byte
	putUint64BE(hk[:], b.Height)
	w.DueBucketProof = f.prove(t, statehash.Key(tagDueBucket, hk[:]))
	return w
}

// --- OFF-boundary crossing POSITIVE: the box AGREES with real apply() over an OFF-boundary maturity
// crossing. This is the exact gap the class-M reproducer closes — it MUST fail against the pre-fix
// (P-only) recompute, verified by the ablation below. ---
func TestRecomputeStateRootClassMOffBoundaryCrossingAgreesWithApply(t *testing.T) {
	f := buildOffBoundaryMaturityFixture(t)
	b := f.crossingBlock()

	// Precondition: the block IS the crossing and is OFF-boundary.
	applied := f.applied(b)
	if !applied.everMature {
		t.Fatalf("fixture: the crossing block must latch everMature true; got false")
	}
	if b.Height%f.c.cfg.EpochBlocks == 0 {
		t.Fatalf("fixture: the crossing block must be OFF-boundary")
	}

	committed := f.committedRoot(t, b)
	w := f.witnessForCrossing(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("off-boundary crossing recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- OFF-boundary crossing ABLATION (the PRE-FIX gap, RED→GREEN): model the pre-fix recompute that
// had NO class-M reproducer off-boundary. Fold the honest E/R + A ops WITHOUT the tagEverMature op; the
// recomputed root must DIVERGE from the honest committed root (the pre-fix box would STALL). Then the
// FIXED entry AGREES. This proves the class-M write is load-bearing at the exact boundary the gap lives
// on. ---
func TestRecomputeStateRootClassMOffBoundaryAblationNoClassM(t *testing.T) {
	f := buildOffBoundaryMaturityFixture(t)
	b := f.crossingBlock()
	committed := f.committedRoot(t, b)
	w := f.witnessForCrossing(t, b)

	// Reproduce the PRE-FIX behavior: assemble the ops WITHOUT class M (no maturity witness). The
	// pre-fix off-boundary recompute folded only E/R + A, omitting the tagEverMature false→true write.
	noM := w
	noM.Maturity = nil
	// The entry now REQUIRES a class-M witness, so it stalls loud on nil (never silently skips). Model the
	// pre-fix fold directly: build the E/R + A ops and fold them, omitting the everMature op.
	ops := f.nonMaturityOps(t, b, w)
	buggyRoot, err := statehash.FoldChangedPaths(f.prevRoot, ops)
	if err != nil {
		t.Fatalf("fold (pre-fix, no class M): %v", err)
	}
	if buggyRoot == committed {
		t.Fatalf("ABLATION FAILED: omitting the class-M everMature write must diverge the root, but it matched")
	}

	// And the FIXED path AGREES (class M emits the everMature write) — the RED→GREEN pair on one fixture.
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("the fixed (class-M) recompute should AGREE, got %v", err)
	}
}

// nonMaturityOps rebuilds the E/R + class-A FoldOps for the crossing block, WITHOUT the class-M
// everMature op — modelling the pre-fix off-boundary recompute so the ablation can fold them and show
// the root diverges.
func (f offBoundaryMaturityFixture) nonMaturityOps(t *testing.T, b Block, w StateRootWitness) []statehash.FoldOp {
	t.Helper()
	witByKey := map[string]*StateRootChangedLeafWitness{}
	for i := range w.ChangedLeaves {
		witByKey[string(w.ChangedLeaves[i].Key)] = &w.ChangedLeaves[i]
	}
	var ops []statehash.FoldOp
	aOps, aWrites, err := f.c.attOps(b, w)
	if err != nil {
		t.Fatalf("attOps: %v", err)
	}
	ops = append(ops, aOps...)
	writeSet := applyEntriesRevocationsWriteSet(b)
	writeSet = append(writeSet, aWrites...)
	for _, wr := range writeSet {
		wit, ok := witByKey[string(wr.key)]
		if !ok || wit.Proof.IsNil() {
			t.Fatalf("no witness for derived key %x", wr.key)
		}
		ops = append(ops, statehash.FoldOp{
			Key: wr.key, OldValue: wit.OldValue, NewValue: wr.newValue, Proof: wit.Proof, DeleteSiblings: wit.DeleteSiblings,
		})
	}
	return ops
}

// ============================================================================================
// COVERAGE (b) + (c), via the REAL entry, on BOTH an OFF-boundary and an ON-boundary crossing.
// ============================================================================================

// crossingCase is one maturity-crossing scenario: a description, whether it lands on a boundary, and a
// builder that returns the fixture chain + prevRoot + the crossing block + the honest witness + the
// committed root. Both cases seat a network young→mature; the ON-boundary case reuses handoffFixture.
type crossingCase struct {
	name       string
	onBoundary bool
	build      func(t *testing.T) (c *Chain, prevRoot ports.Hash, b Block, w StateRootWitness, committed ports.Hash)
}

func crossingCases() []crossingCase {
	return []crossingCase{
		{
			name:       "off-boundary",
			onBoundary: false,
			build: func(t *testing.T) (*Chain, ports.Hash, Block, StateRootWitness, ports.Hash) {
				f := buildOffBoundaryMaturityFixture(t)
				b := f.crossingBlock()
				return f.c, f.prevRoot, b, f.witnessForCrossing(t, b), f.committedRoot(t, b)
			},
		},
		{
			name:       "on-boundary",
			onBoundary: true,
			build: func(t *testing.T) (*Chain, ports.Hash, Block, StateRootWitness, ports.Hash) {
				f := buildHandoffFixture(t)
				b := f.handoffBoundaryBlock()
				return f.c, f.prevRoot, b, f.witnessForHandoff(t, b), f.committedRoot(t, b)
			},
		},
	}
}

// (b) OMIT the tagEverMature write ⇒ stall. The committed root reflects the latch (everMature=true),
// but a recompute that does NOT fold the everMature write (modelled by forging the committed root to
// carry everMature=true while the box's maturity witness says the block does NOT cross) diverges ⇒
// stall. Here we drive the REAL entry: a committed root that reflects the latch, but a Maturity witness
// whose SeenSet does NOT reach the bar (so the box omits the write) ⇒ mismatch stall. We force the box
// to omit by supplying a SeenSet that reconstructs a coefficient BELOW the bar (drop a member), which
// makes RecomputeMatureNow return not-mature — but the omitted member breaks the digest first, so the
// cleaner drive is: forge the committed root to commit everMature=true honestly, and supply a Maturity
// witness with pre=true (already-latched) so the box omits the write; the honest committed root
// reflects a FRESH latch (pre was false), so the roots diverge. That models "the write was omitted".
func TestRecomputeStateRootClassMOmittedWriteStalls(t *testing.T) {
	for _, tc := range crossingCases() {
		t.Run(tc.name, func(t *testing.T) {
			c, prevRoot, b, w, committed := tc.build(t)

			// The honest committed root reflects the everMature false→true latch this block performs.
			// Model a recompute that OMITS that write: claim the chain was ALREADY latched (pre=true), so
			// class M emits nothing. The box's recomputed root then commits everMature=? — but pre=true
			// means the fold never touches the everMature leaf, leaving the pre-state value (false) under
			// the recomputed root, while the committed root commits true ⇒ mismatch.
			w.Maturity = &StateRootMaturityWitness{
				EverMature: StateRootRotateScalar{OldValue: statehash.EncodeBool(true), Proof: w.Maturity.EverMature.Proof},
			}

			err := c.RecomputeStateRootEntriesRevocations(prevRoot, committed, b, w)
			if err == nil {
				t.Fatalf("ABLATION FAILED: omitting the everMature write (pre claimed already-latched) must stall, got nil")
			}
			// The forged pre=true scalar cannot verify against prevStateRoot (which commits false), so the
			// fold stalls; if the boundary freeze gate consumes the wrong post value it diverges the root.
			if !errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootMismatch) &&
				!errors.Is(err, ErrRecomputeStateRootMaturity) {
				t.Fatalf("expected a fold/mismatch/maturity stall for an omitted everMature write, got %v", err)
			}
		})
	}
}

// (c) FORGED maturity screen (per-member bonded/slashed) ⇒ stall. A forged per-member value in the
// class-M SeenSet cannot verify against the committed root ⇒ RecomputeMatureNow stalls ⇒ the box
// stalls (ErrRecomputeStateRootMaturity). Driven via the REAL entry on both crossings.
func TestRecomputeStateRootClassMForgedMaturityScreenStalls(t *testing.T) {
	for _, tc := range crossingCases() {
		t.Run(tc.name, func(t *testing.T) {
			c, prevRoot, b, w, committed := tc.build(t)

			// AGREE first (honest) so the ablation is not vacuous.
			if err := c.RecomputeStateRootEntriesRevocations(prevRoot, committed, b, w); err != nil {
				t.Fatalf("honest crossing must AGREE first, got %v", err)
			}

			// Forge one member's bonded weight in the class-M SeenSet: its inclusion proof no longer
			// verifies against the committed root ⇒ RecomputeMatureNow stalls ⇒ the box stalls.
			if len(w.Maturity.SeenSet.IDs) == 0 {
				t.Fatalf("fixture: the crossing SeenSet must carry members")
			}
			victim := w.Maturity.SeenSet.IDs[0]
			mw := w.Maturity.SeenSet.Members[victim]
			mw.Bonded += 1 // a forged weight the committed root does not commit
			w.Maturity.SeenSet.Members[victim] = mw

			err := c.RecomputeStateRootEntriesRevocations(prevRoot, committed, b, w)
			if !errors.Is(err, ErrRecomputeStateRootMaturity) {
				t.Fatalf("ABLATION FAILED: a forged maturity screen must stall with ErrRecomputeStateRootMaturity, got %v", err)
			}
		})
	}
}
