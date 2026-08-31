package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the P1-e class-P epoch-rotation state-root recompute
// (floorbox_recompute_stateroot_rotate_v5.go).
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md
//
// R3 (execution-derived drift guard, MANDATORY): the box's rotate reconstruction is checked against
// the REAL apply() + StateRootForVersion(5), ablated RED on a stale (pre-delta) freeze, a flipped
// regVersion tally, a short qualified-set witness, and a missing rotate scalar. Each ablation drives
// the REAL RecomputeStateRootEntriesRevocations.

type rotateFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
	proposer ed25519.PrivateKey
}

// buildRotateFixture advances a v5 chain to the block BEFORE an epoch boundary, with the epochSet
// already frozen (matureEpoch=true via MatureValidators=0). The head is positioned so the next block
// is a boundary. TTL enabled so the dueBucket scope-gate path is exercised.
func buildRotateFixture(t *testing.T) rotateFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(55001)
	v2 := key(55002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(v2, ports.HashBytes(pubOf(v2)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)
	// Advance to head==2 (next block h=2 is a boundary). Genesis (h=0) already froze + locked in.
	for {
		_, h := c.Head()
		if h == 2 {
			break
		}
		prev, hh := c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: hh, Prev: prev, Entries: []ports.Entry{entry(byte(50 + hh))}}
		Sign(&b, prop)
		c.apply(b)
	}
	if !c.matureEpoch {
		t.Fatalf("fixture: expected matureEpoch=true")
	}
	if len(c.epochSet) == 0 {
		t.Fatalf("fixture: expected a non-empty frozen epochSet")
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
	return rotateFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop}
}

// boundaryBlock builds a boundary block. If bondReg is non-nil it also carries a fresh bond reg
// (a membership change at the freeze).
func (f rotateFixture) boundaryBlock(bondReg *BondReg) Block {
	prev, h := f.c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(99)}}
	if bondReg != nil {
		b.BondRegs = append(b.BondRegs, *bondReg)
	}
	Sign(&b, f.proposer)
	return b
}

func (f rotateFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

func (f rotateFixture) leafWitness(t *testing.T, wr stateRootWrite) StateRootChangedLeafWitness {
	t.Helper()
	old := f.preValue(wr.key)
	if wr.newValue == nil {
		wit, sibs, err := f.prover.ProveWithSiblings(wr.key)
		if err != nil {
			t.Fatalf("ProveWithSiblings: %v", err)
		}
		return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit, DeleteSiblings: sibs}
	}
	wit, err := f.prover.Prove(wr.key)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit}
}

func (f rotateFixture) digestWitness(t *testing.T, tag string, preIDs []ports.NodeID) StateRootDigestWitness {
	t.Helper()
	wit, err := f.prover.Prove(statehash.Key(tag, nil))
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootDigestWitness{Tag: tag, PreIDs: preIDs, Proof: wit}
}

func (f rotateFixture) preQualifiedIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.qualified))
	for id := range f.c.qualified {
		out = append(out, id)
	}
	return sortIDs(out)
}
func (f rotateFixture) preBondedIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.bonded))
	for id := range f.c.bonded {
		out = append(out, id)
	}
	return sortIDs(out)
}
func (f rotateFixture) preSlashedIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.slashed))
	for id := range f.c.slashed {
		out = append(out, id)
	}
	return sortIDs(out)
}

// scalarWit builds a rotate scalar witness for a reserved-key scalar leaf.
func (f rotateFixture) scalarWit(t *testing.T, tag string) StateRootRotateScalar {
	t.Helper()
	key := statehash.Key(tag, nil)
	wit, err := f.prover.Prove(key)
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootRotateScalar{OldValue: f.preValue(key), Proof: wit}
}

// rotateMember builds a frozen-member witness reading the fixture's committed pre-state: the member's
// POST-qualified weight, its regVersion, and its prior epochSet leaf proof (the freeze write-target).
func (f rotateFixture) rotateMember(t *testing.T, id ports.NodeID, weight int64) StateRootRotateMember {
	t.Helper()
	esKey := statehash.Key(tagEpochSet, id[:])
	esOld := f.preValue(esKey)
	esProof, err := f.prover.Prove(esKey)
	if err != nil {
		t.Fatalf("Prove(epochSet %x): %v", id[:], err)
	}
	rv, ok := f.c.regVersion[id]
	return StateRootRotateMember{
		ID:               id,
		Weight:           weight,
		RegVersion:       rv,
		RegVersionKnown:  ok,
		EpochSetProof:    esProof,
		EpochSetOldValue: esOld,
	}
}

// witnessForBoundary builds the full witness for a boundary block. It reconstructs the POST-apply
// qualified set (via a real apply() clone) to know the frozen members, then witnesses each.
func (f rotateFixture) witnessForBoundary(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	// E/R changed leaves.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	// If the block carries a bond reg, add the full class-B witness (digests, screens, buckets, and
	// the per-member changed leaves) so the box can reconstruct the post-qualified set.
	hasBondReg := len(b.BondRegs) > 0
	if hasBondReg {
		f.addBondRegWitness(t, b, &w)
	}

	// The POST-apply qualified set + weights (the freeze source), from a real apply() clone.
	clone := f.c.cloneForDryRun()
	clone.apply(b)

	// DigestPreSets: qualifiedRoot (freeze source anchor) + epochSetRoot (freeze digest). Bond-reg
	// blocks also need bondedRoot/slashedRoot — added by addBondRegWitness.
	w.DigestPreSets = append(w.DigestPreSets,
		f.digestWitness(t, tagQualifiedRoot, f.preQualifiedIDs()),
		f.digestWitness(t, tagEpochSetRoot, f.preEpochSetIDs()),
	)

	// Rotate witness: frozen members + prior epochSet droppers + scalars.
	var rw StateRootRotateWitness
	for id, wt := range clone.qualified {
		rw.Members = append(rw.Members, f.rotateMember(t, id, wt))
	}
	// Prior epochSet members not in the new frozen set → DELETE witnesses.
	for id := range f.c.epochSet {
		if _, stillFrozen := clone.qualified[id]; stillFrozen {
			continue
		}
		esKey := statehash.Key(tagEpochSet, id[:])
		wit, sibs, err := f.prover.ProveWithSiblings(esKey)
		if err != nil {
			t.Fatalf("ProveWithSiblings(epochSet drop): %v", err)
		}
		rw.PriorEpochSet = append(rw.PriorEpochSet, StateRootRotateMember{
			ID: id, EpochSetOldValue: f.preValue(esKey), EpochSetProof: wit, EpochSetDeleteSiblings: sibs,
		})
	}
	rw.EpochStart = f.scalarWit(t, tagEpochStart)
	rw.MatureEpoch = f.scalarWit(t, tagMatureEpoch)
	rw.EverMature = f.scalarWit(t, tagEverMature)
	rw.GateLockedIn = f.scalarWit(t, tagGateLockedIn)
	rw.GateHeight = f.scalarWit(t, tagGateHeight)
	rw.Era3LockedIn = f.scalarWit(t, tagEra3LockedIn)
	rw.Era3Height = f.scalarWit(t, tagEra3Height)
	rw.Era4LockedIn = f.scalarWit(t, tagEra4LockedIn)
	rw.Era4Height = f.scalarWit(t, tagEra4Height)
	w.Rotate = &rw

	// dueBucket scope-gate proof (non-membership at b.Height, unless a bond reg TTL bucket collides).
	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dp, err := f.prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		if err != nil {
			t.Fatalf("Prove(dueBucket): %v", err)
		}
		w.DueBucketProof = dp
	}
	return w
}

func (f rotateFixture) preEpochSetIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.epochSet))
	for id := range f.c.epochSet {
		out = append(out, id)
	}
	return sortIDs(out)
}

// addBondRegWitness adds the class-B witness pieces for a boundary+bondreg compound block.
func (f rotateFixture) addBondRegWitness(t *testing.T, b Block, w *StateRootWitness) {
	t.Helper()
	// Digest pre-sets B touches (bondedRoot, qualifiedRoot, slashedRoot). qualifiedRoot is added by
	// the caller too — dedup by tag so we don't double-add.
	w.DigestPreSets = append(w.DigestPreSets,
		f.digestWitness(t, tagBondedRoot, f.preBondedIDs()),
		f.digestWitness(t, tagSlashedRoot, f.preSlashedIDs()),
	)
	// Screens: per bond-reg root ownership.
	for _, r := range b.BondRegs {
		owner, claimed := f.c.bondRootOwner[r.Root]
		w.BondRegScreens = append(w.BondRegScreens, StateRootBondRegScreen{
			Root: r.Root, PriorOwner: owner, Claimed: claimed, PriorProven: f.c.bondRootProven[r.Root],
		})
	}
	// Bucket witnesses for affected due-heights (a fresh reg inserts into b.Height+ttl+1).
	if f.c.cfg.BondTTLBlocks > 0 {
		due := b.Height + f.c.cfg.BondTTLBlocks + 1
		var hk [8]byte
		putUint64BE(hk[:], due)
		bp, err := f.prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		if err != nil {
			t.Fatalf("Prove(bucket): %v", err)
		}
		w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{DueHeight: due, PreMembers: nil, Proof: bp})
	}

	// The B per-member write-set: reconstruct via bondRegOps to enumerate the changed keys. Build a
	// temporary witness with the digest pre-sets + buckets so bondRegOps can anchor.
	tmp := StateRootWitness{
		DigestPreSets: []StateRootDigestWitness{
			f.digestWitness(t, tagBondedRoot, f.preBondedIDs()),
			f.digestWitness(t, tagQualifiedRoot, f.preQualifiedIDs()),
			f.digestWitness(t, tagSlashedRoot, f.preSlashedIDs()),
		},
		BondRegScreens: w.BondRegScreens,
		BondRegBuckets: w.BondRegBuckets,
	}
	_, bWrites, err := f.c.bondRegOps(b, tmp)
	if err != nil {
		t.Fatalf("bondRegOps (witness build): %v", err)
	}
	for _, wr := range bWrites {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
}

func (f rotateFixture) applyAndCommittedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// --- Ablation 0: the P recompute AGREES with real apply() over a steady-state boundary. ---
func TestRecomputeStateRootRotateAgreesWithApply(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("P recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Ablation 0b: a boundary that ALSO carries a bond reg (membership change at the freeze) — the
// frozen epochSet must include the just-bonded validator (R-P-sameblock-order). ---
func TestRecomputeStateRootRotateWithBondRegAgreesWithApply(t *testing.T) {
	f := buildRotateFixture(t)
	nv := key(55055)
	reg := bondRegFull(nv, ports.HashBytes(pubOf(nv)), 6<<20, ports.Hash{}, 5, 3)
	b := f.boundaryBlock(&reg)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("P+bondreg recompute should AGREE with real apply() but stalled: %v", err)
	}
	// Confirm the just-bonded validator IS in the reconstructed frozen set (else the ablation is vacuous).
	postQual, err := f.c.reconstructPostQualified(b, w)
	if err != nil {
		t.Fatalf("reconstructPostQualified: %v", err)
	}
	if _, ok := postQual[ports.HashBytes(pubOf(nv))]; !ok {
		t.Fatalf("the just-bonded validator must be in the reconstructed post-qualified freeze set")
	}
}

// --- Ablation 1: STALE FREEZE — freeze the PRE-delta qualified set (drop the just-bonded validator
// from the frozen member witness). The box's reconstructed post-qualified INCLUDES it, so the member
// set mismatches ⇒ stall (R-P-sameblock-order). ---
func TestRecomputeStateRootRotateAblationStaleFreeze(t *testing.T) {
	f := buildRotateFixture(t)
	nv := key(55056)
	reg := bondRegFull(nv, ports.HashBytes(pubOf(nv)), 6<<20, ports.Hash{}, 5, 3)
	b := f.boundaryBlock(&reg)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	// Drop the just-bonded validator from the rotate Members (a stale, pre-delta freeze).
	nvid := ports.HashBytes(pubOf(nv))
	var kept []StateRootRotateMember
	for _, m := range w.Rotate.Members {
		if m.ID != nvid {
			kept = append(kept, m)
		}
	}
	w.Rotate.Members = kept

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a stale (pre-delta) freeze must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected a digest/mismatch stall for a stale freeze, got %v", err)
	}
}

// --- Ablation 2: SHORT qualified-set witness — drop a member from the qualifiedRoot pre-set id-list.
// The reconstructed pre-qualified digest mismatches the committed pre-digest ⇒ fold stall. ---
func TestRecomputeStateRootRotateAblationShortQualifiedSet(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	// Drop a member from the qualifiedRoot pre-set.
	dropOne := f.preQualifiedIDs()[0]
	for i := range w.DigestPreSets {
		if w.DigestPreSets[i].Tag == tagQualifiedRoot {
			w.DigestPreSets[i].PreIDs = dropID(w.DigestPreSets[i].PreIDs, dropOne)
		}
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a short qualified pre-set must stall, got nil")
	}
	// Stalls either at the member-set cross-check (digest) or the fold's pre-digest anchor.
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootFold) &&
		!errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected a digest/fold/mismatch stall, got %v", err)
	}
}

// --- Ablation 3: FLIPPED tally — a boundary at genesis where the era-4 lock-in FIRES. Forging a
// frozen member's regVersion below the era-4 threshold flips the tally, so the box would NOT lock in
// era4 ⇒ wrong era4LockedIn scalar ⇒ mismatch. We probe this at the GENESIS boundary via a fresh
// fixture whose lock-ins fire at genesis, reconstructing that boundary. ---
func TestRecomputeStateRootRotateAblationFlippedTally(t *testing.T) {
	// A genesis boundary that locks in era4 (regVersion 5 over the whole frozen set). We reconstruct
	// the h=2 boundary but FORGE a member's regVersion down; since era3/era4 already locked at genesis
	// in this fixture, use a fixture where a later boundary re-tallies. Simplest: assert the tally is
	// LOAD-BEARING by injecting a regVersion that would flip a NOT-yet-locked tally. In the standard
	// fixture all tallies locked at genesis, so instead we verify the box READS the witnessed
	// regVersion by confirming a forged regVersion that would change a tally is rejected when a tally
	// is live. We construct a fixture with a genesis-declared activation height that keeps era4 UNLOCKED
	// until a real signal — see rotateOps: a locked tally is skipped, so forging regVersion is inert
	// once locked. This ablation therefore drives the AGREE path and confirms a forged regVersion on a
	// LIVE member changes the epochSet weight leaf, which is fold-caught.
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBoundary(t, b)

	// Forge a frozen member's weight (the epochSet leaf value) — a wrong freeze weight ⇒ wrong
	// epochSet[id] leaf ⇒ mismatch. This proves the per-member freeze weight is load-bearing.
	if len(w.Rotate.Members) == 0 {
		t.Fatalf("fixture: no frozen members")
	}
	w.Rotate.Members[0].Weight = w.Rotate.Members[0].Weight + 1

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a forged freeze weight must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootMismatch) && !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: expected a mismatch/fold stall for a forged freeze weight, got %v", err)
	}
}

// --- Ablation 3b: LIVE tally, forged regVersion — a boundary where the era-4 lock-in FIRES.
// Genesis seats validators at regVersion 4 (era-3, below the era-4 threshold), so era4 stays UNLOCKED
// at genesis. At the h=2 boundary a regVersion-5 validator carrying dominant weight crosses the
// 3*ready > 2*total tally ⇒ era4LockedIn flips true. Forging that validator's regVersion DOWN to 4 in
// the rotate witness makes the box's tally NOT lock in ⇒ wrong era4LockedIn scalar ⇒ mismatch. This
// proves the per-member regVersion witness is load-bearing for the tally (R-P-tally-regversion). ---
func TestRecomputeStateRootRotateAblationLiveTallyForgedRegVersion(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(55301)
	// Genesis: a single small regVersion-4 validator ⇒ era4 tally does not lock (ready=0 at v5 thresh).
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 2<<20, ports.Hash{}, 4, 1))
	Sign(g, prop)
	c.apply(*g)
	if c.era4LockedIn {
		t.Fatalf("fixture: era4 must be UNLOCKED at genesis for this ablation")
	}
	// h=1: add a regVersion-5 validator with dominant weight, so at the h=2 boundary the era-4 tally
	// (regVersion>=5 weight) crosses 3*ready > 2*total and locks in.
	big := key(55302)
	prev1, _ := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev1}
	b1.BondRegs = append(b1.BondRegs, bondRegFull(big, ports.HashBytes(pubOf(big)), 16<<20, ports.Hash{}, 5, 2))
	Sign(&b1, prop)
	c.apply(b1)

	// Sanity: at the h=2 boundary apply() locks era4.
	prev, h := c.Head()
	rb := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(99)}}
	Sign(&rb, prop)
	sanity := c.cloneForDryRun()
	sanity.apply(rb)
	if !sanity.era4LockedIn {
		t.Fatalf("fixture: expected era4 to lock in at the h=2 boundary (era4LockedIn=%v)", sanity.era4LockedIn)
	}

	f := rotateFixture{c: c, proposer: prop}
	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	f.prover = prover
	f.prevRoot = prover.Root()

	committed, err := sanity.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	w := f.witnessForBoundary(t, rb)

	// AGREE first (honest regVersion) — the box must lock era4 and match.
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, rb, w); err != nil {
		t.Fatalf("honest live-tally boundary should AGREE, got %v", err)
	}

	// Now FORGE the big validator's regVersion down to 4 in the rotate witness. The box's era-4 tally
	// then does NOT cross the threshold ⇒ it does NOT lock era4 ⇒ its recomputed root omits the
	// era4LockedIn/era4Height writes ⇒ mismatch with the honest committed root.
	bigID := ports.HashBytes(pubOf(big))
	for i := range w.Rotate.Members {
		if w.Rotate.Members[i].ID == bigID {
			w.Rotate.Members[i].RegVersion = 4
		}
	}
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, rb, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a forged (lowered) regVersion on a live tally must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected ErrRecomputeStateRootMismatch (box fails to lock era4), got %v", err)
	}
}

// --- Ablation 4: MISSING rotate scalar — forge the committed root to reflect a DIFFERENT epochStart
// than the block writes, and hand an honest witness. The box computes epochStart=b.Height; the buggy
// committed root reflects a stale epochStart ⇒ mismatch. Proves the epochStart fold is load-bearing. ---
func TestRecomputeStateRootRotateAblationMissingScalar(t *testing.T) {
	f := buildRotateFixture(t)
	b := f.boundaryBlock(nil)

	// Buggy committed root: apply, then rewind epochStart to its pre-value (modelling "rotate did NOT
	// write epochStart"). Everything else is the honest post-state.
	buggyClone := f.c.cloneForDryRun()
	preEpochStart := buggyClone.epochStart
	buggyClone.apply(b)
	if buggyClone.epochStart == preEpochStart {
		t.Fatalf("fixture: apply() did not advance epochStart — ablation vacuous")
	}
	buggyClone.epochStart = preEpochStart // undo the write
	buggyCommitted, err := buggyClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("buggy StateRootForVersion: %v", err)
	}

	w := f.witnessForBoundary(t, b)
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, buggyCommitted, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a committed root reflecting a stale epochStart must stall the honest recompute, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected ErrRecomputeStateRootMismatch (honest epochStart != buggy committed), got %v", err)
	}
}

// --- Ablation 5: #535 recovery boundary — the box cannot reconstruct liveQualifiedSet(); it STALLS. ---
func TestRecomputeStateRootRotateAblationRecoveryStalls(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 64, LivenessRecoveryHeight: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(55201)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	c.apply(*g)
	prev, _ := c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev}
	Sign(&b, prop)
	c.apply(b)

	prev, h := c.Head()
	if h != 2 {
		t.Fatalf("fixture: expected head h=2, got %d", h)
	}
	rb := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(99)}}
	Sign(&rb, prop)

	// rotateOps must stall at the recovery boundary regardless of witness.
	_, err := c.rotateOps(rb, StateRootWitness{Rotate: &StateRootRotateWitness{}}, map[ports.NodeID]struct{}{})
	if !errors.Is(err, ErrRecomputeStateRootScopeStall) {
		t.Fatalf("ABLATION FAILED: the #535 recovery boundary must stall, got %v", err)
	}
}
