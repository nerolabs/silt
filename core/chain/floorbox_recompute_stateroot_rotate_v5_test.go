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
	return f.rotateMemberPost(t, id, weight, f.c.regVersion)
}

// rotateMemberPost builds a frozen-member witness whose RegVersion is taken from the supplied
// regVersion map. Callers pass the POST-apply clone's regVersion so an in-block bonded member carries
// its just-written version (RegVersionKnown=true) — the DIRECTION B in-block cross-check input. For a
// steady-state member the post map equals pre-state, so the pre-state RegVersionProof still resolves.
func (f rotateFixture) rotateMemberPost(t *testing.T, id ports.NodeID, weight int64, regVersion map[ports.NodeID]uint8) StateRootRotateMember {
	t.Helper()
	esKey := statehash.Key(tagEpochSet, id[:])
	esOld := f.preValue(esKey)
	esProof, err := f.prover.Prove(esKey)
	if err != nil {
		t.Fatalf("Prove(epochSet %x): %v", id[:], err)
	}
	rv, ok := regVersion[id]
	m := StateRootRotateMember{
		ID:               id,
		Weight:           weight,
		RegVersion:       rv,
		RegVersionKnown:  ok,
		EpochSetProof:    esProof,
		EpochSetOldValue: esOld,
	}
	// R1.2 anchors: the qualified||id weight proof (steady-state Weight anchor) and the regVersion||id
	// proof (present when RegVersionKnown, else a non-membership proof). A fresh in-block bond has no
	// pre-state qualified||id leaf, so QualifiedProof is a non-membership proof there and the box
	// cross-checks Weight against the class-B write instead; likewise the box cross-checks RegVersion
	// against the class-B regVerWrites for an in-block member (DIRECTION B) and does not read the proof.
	m.QualifiedProof = mustProve(f.prover, statehash.Key(tagQualified, id[:]))
	m.RegVersionProof = mustProve(f.prover, statehash.Key(tagRegVersion, id[:]))
	return m
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

	// Rotate witness: frozen members + prior epochSet droppers + scalars. An in-block bonded member
	// carries its POST-write regVersion (from the applied clone) — the DIRECTION B cross-check input.
	var rw StateRootRotateWitness
	for id, wt := range clone.qualified {
		rw.Members = append(rw.Members, f.rotateMemberPost(t, id, wt, clone.regVersion))
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
	rw.GateLockedIn = f.scalarWit(t, tagGateLockedIn)
	rw.GateHeight = f.scalarWit(t, tagGateHeight)
	rw.Era3LockedIn = f.scalarWit(t, tagEra3LockedIn)
	rw.Era3Height = f.scalarWit(t, tagEra3Height)
	rw.Era4LockedIn = f.scalarWit(t, tagEra4LockedIn)
	rw.Era4Height = f.scalarWit(t, tagEra4Height)
	w.Rotate = &rw

	// Class M maturity witness. This fixture is mature-from-genesis (MatureValidators=0), so everMature
	// is already latched pre-state (pre=true) — class M emits nothing and reads no SeenSet, but the entry
	// still requires the witness so the latch is never silently skipped.
	w.Maturity = &StateRootMaturityWitness{EverMature: f.scalarWit(t, tagEverMature), MatureEpoch: f.scalarWit(t, tagMatureEpoch)}

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
			OwnerProof:  mustProve(f.prover, statehash.Key(tagBondRootOwner, r.Root[:])),
			ProvenProof: mustProve(f.prover, statehash.Key(tagBondRootProven, r.Root[:])),
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
	_, bWrites, err := f.c.bondRegOps(f.prevRoot, b, tmp)
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
	postQual, err := f.c.reconstructPostQualified(f.prevRoot, b, w)
	if err != nil {
		t.Fatalf("reconstructPostQualified: %v", err)
	}
	if _, ok := postQual[ports.HashBytes(pubOf(nv))]; !ok {
		t.Fatalf("the just-bonded validator must be in the reconstructed post-qualified freeze set")
	}
}

// --- Byte-exact: the reconstructed epochSetRoot equals nodeSetMTH over the real frozen post-set.
// Analogous to class A's TestRecomputeStateRootAttDigestByteExact: compares the rotate op's NewValue
// for tagEpochSetRoot directly to nodeSetMTHFromInt64(clone.epochSet). ---
func TestRecomputeStateRootRotateEpochSetRootByteExact(t *testing.T) {
	f := buildRotateFixture(t)
	nv := key(55077)
	reg := bondRegFull(nv, ports.HashBytes(pubOf(nv)), 6<<20, ports.Hash{}, 5, 3)
	b := f.boundaryBlock(&reg) // a membership change at the freeze, so the frozen set is non-trivial
	clone := f.c.cloneForDryRun()
	clone.apply(b)

	w := f.witnessForBoundary(t, b)
	postQual, qualWrites, regVerWrites, err := f.c.reconstructPostQualifiedWithWrites(f.prevRoot, b, w)
	if err != nil {
		t.Fatalf("reconstructPostQualifiedWithWrites: %v", err)
	}
	// Mature-from-genesis fixture ⇒ post-latch everMature is true (class M threads it in).
	ops, err := f.c.rotateOps(f.prevRoot, b, w, postQual, qualWrites, regVerWrites, true)
	if err != nil {
		t.Fatalf("rotateOps: %v", err)
	}

	want := nodeSetMTHFromInt64(clone.epochSet)
	found := false
	for _, op := range ops {
		if string(op.Key) == string(statehash.Key(tagEpochSetRoot, nil)) {
			found = true
			if string(op.NewValue) != string(want) {
				t.Fatalf("epochSetRoot not byte-exact: got %x want %x", op.NewValue, want)
			}
		}
	}
	if !found {
		t.Fatalf("no epochSetRoot op emitted at the boundary freeze")
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

// --- Ablation 3: FORGED FREEZE WEIGHT → wrong epochSet leaf. This ablation does NOT exercise the
// tally path: the standard fixture (MatureValidators=0) locks ALL three activation tallies at the
// GENESIS boundary, so at the h=2 boundary rotateTallyOps is a no-op (every tally is already locked
// and skipped). The stall here comes from forging a frozen member's WEIGHT: the per-member epochSet
// leaf value = the qualified weight, so a wrong weight diverges the epochSet[id] leaf ⇒ post-root !=
// StateRoot ⇒ fold-caught. It proves the per-member freeze weight is load-bearing. The LOAD-BEARING
// tally test is TestRecomputeStateRootRotateAblationLiveTallyForgedRegVersion (3b), which keeps era4
// UNLOCKED until a live h=2 signal and forges regVersion to flip that tally. ---
func TestRecomputeStateRootRotateAblationForgedFreezeWeight(t *testing.T) {
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
	// R1.2: the forged Weight fails the qualified||id present-anchor against prevStateRoot (the honest
	// QualifiedProof proves the true weight), so the class-P member anchor stalls
	// (ErrRecomputeStateRootDigest) — a stronger, earlier catch than the pre-R1.2 fold/mismatch.
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootMismatch) && !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: expected an anchor/mismatch/fold stall for a forged freeze weight, got %v", err)
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
	// R1.2: the forged RegVersion=4 fails the regVersion||id present-anchor against prevStateRoot (the
	// honest RegVersionProof proves the true value 5), so the class-P member anchor stalls
	// (ErrRecomputeStateRootDigest) BEFORE the tally runs — a stronger, earlier catch than the pre-R1.2
	// mismatch (which relied on the box computing a wrong lock-in). Either is a valid never-Accept stall.
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected an anchor or mismatch stall (box fails to lock era4), got %v", err)
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
	_, err := c.rotateOps(ports.Hash{}, rb, StateRootWitness{Rotate: &StateRootRotateWitness{}}, map[ports.NodeID]struct{}{}, nil, nil, true)
	if !errors.Is(err, ErrRecomputeStateRootScopeStall) {
		t.Fatalf("ABLATION FAILED: the #535 recovery boundary must stall, got %v", err)
	}
}

// ============================================================================================
// THE YOUNG→MATURE HANDOFF BOUNDARY (the Path-1 completeness gap this file closes).
//
// apply() latches everMature (class M) BEFORE rotateEpoch (chain.go:3303-3316), so on the ONE
// boundary that flips everMature false→true — the young→mature handoff — real apply() freezes the
// epochSet but the box (pre-fix) took the !everMature early-return and froze NOTHING → root mismatch
// → STALL. Every OTHER class-P test uses MatureValidators=0 (mature-from-genesis), so everMature is
// latched at h=0 and this path is unexercised. These tests seat the network YOUNG at genesis and
// mature it AT the boundary (the latch flips false→true at the boundary height).
// ============================================================================================

// handoffFixture builds a v5 chain that is YOUNG at genesis (validatorsSeen empty ⇒ coefficient 0 <
// MatureValidators) and matures AT the h=2 boundary: the boundary block carries non-proposer atts that
// seat two qualified validators into validatorsSeen, crossing MatureValidators=2. apply() seats them,
// then the latch (class M) flips everMature false→true, then rotateEpoch freezes — all in the boundary
// block. The box must reproduce the M-write + the freeze from the pre-state + witnesses.
type handoffFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
	proposer ed25519.PrivateKey
	att1     ed25519.PrivateKey
	att2     ed25519.PrivateKey
	att3     ed25519.PrivateKey
}

func buildHandoffFixture(t *testing.T) handoffFixture {
	t.Helper()
	// MatureValidators=2. The Nakamoto coefficient over N equal unset-domain bonds is the fewest whose
	// cumulative weight EXCEEDS total/3; for THREE equal bonds that is 2 (one bond = total/3, not >). So
	// the network first clears the bar only once THREE distinct validators are seated — which happens AT
	// the h=2 boundary block. EpochBlocks=2 ⇒ h=2 is the boundary. TTL enabled for the scope-gate path.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, MinBondBytes: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 2, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(56001)
	att1 := key(56002)
	att2 := key(56003)
	att3 := key(56004)
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

	// h=1: a plain E/R block, NO seating atts — validatorsSeen stays empty, network stays young.
	prev, _ := c.Head()
	b1 := Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev, Entries: []ports.Entry{entry(61)}}
	Sign(&b1, prop)
	c.apply(b1)
	if c.everMature {
		t.Fatalf("fixture: network must still be young at h=1, got everMature=true")
	}
	_, h := c.Head()
	if h != 2 {
		t.Fatalf("fixture: expected head h=2 (next block is the boundary), got %d", h)
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
	return handoffFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, att1: att1, att2: att2, att3: att3}
}

// handoffBoundaryBlock builds the h=2 boundary block carrying non-proposer atts from att1+att2 (the
// seating that crosses the maturity bar) plus an E/R entry.
func (f handoffFixture) handoffBoundaryBlock() Block {
	prev, h := f.c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(99)}}
	b.LastCommit = append(b.LastCommit, carrierEntry(f.c, f.att1), carrierEntry(f.c, f.att2), carrierEntry(f.c, f.att3))
	Sign(&b, f.proposer)
	return b
}

func (f handoffFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

func (f handoffFixture) prove(t *testing.T, key []byte) statehash.Witness {
	t.Helper()
	wit, err := f.prover.Prove(key)
	if err != nil {
		t.Fatalf("Prove(%x): %v", key, err)
	}
	return wit
}

func (f handoffFixture) proveWithSiblings(t *testing.T, key []byte) (statehash.Witness, []statehash.FoldSibling) {
	t.Helper()
	wit, sibs, err := f.prover.ProveWithSiblings(key)
	if err != nil {
		t.Fatalf("ProveWithSiblings(%x): %v", key, err)
	}
	return wit, sibs
}

func (f handoffFixture) sortedIDs(m map[ports.NodeID]int64) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return sortIDs(out)
}

// seenWitnessPost builds the SeenSetWitness the box feeds RecomputeMatureNow for the handoff decision.
// It witnesses the POST-apply validatorsSeen set against the POST-apply committed root (the maturity
// latch reads the post-block seen/bonded set), so it is built from a prover over the applied clone.
func (f handoffFixture) seenWitnessPost(t *testing.T, applied *Chain) SeenSetWitness {
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

// witnessForHandoff builds the full compound (E/R + A + P) witness for the handoff boundary block,
// including the everMature pre-scalar proof and the SeenSet maturity witness the fix consumes.
func (f handoffFixture) witnessForHandoff(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	// E/R changed leaves.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
			Key: wr.key, OldValue: f.preValue(wr.key), Proof: f.prove(t, wr.key),
		})
	}

	// The applied clone (post-apply state) — the freeze source + the maturity witness target.
	applied := f.c.cloneForDryRun()
	applied.apply(b)

	// Class A: validatorsSeen ADDs + the validatorsSeenRoot digest. Screen each carried signer
	// from the fixture's committed pre-state.
	preSeen := idSet(f.sortedSeenIDs())
	screens := map[ports.NodeID]StateRootAttScreen{}
	w.ParentProposer, w.ParentProposerSig = f.c.CarrierParentProposerWitness()
	parentProposer, _ := f.c.headProposerID()
	for i := range b.LastCommit {
		id := b.LastCommit[i].AttesterID()
		if id == parentProposer {
			continue
		}
		sz, bp := f.c.bonded[id]
		esVal, inES := f.c.epochSet[id]
		sc := StateRootAttScreen{Attester: id, Slashed: f.c.slashed[id], InEpochSet: inES, BondedSize: sz, BondedPresent: bp,
			SlashedProof:  mustProve(f.prover, statehash.Key(tagSlashed, id[:])),
			EpochSetProof: mustProve(f.prover, statehash.Key(tagEpochSet, id[:])),
			BondedProof:   mustProve(f.prover, statehash.Key(tagBonded, id[:]))}
		if inES {
			sc.EpochSetValue = statehash.EncodeInt64(esVal)
		}
		screens[id] = sc
		w.AttScreens = append(w.AttScreens, sc)
	}
	aWrites, _, err := f.c.stateRootAttWriteSet(f.prevRoot, b, preSeen, screens, livePreForProbe(f.c), w.ParentProposer, w.ParentProposerSig)
	if err != nil {
		t.Fatalf("stateRootAttWriteSet: %v", err)
	}
	for _, wr := range aWrites {
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
			Key: wr.key, OldValue: f.preValue(wr.key), Proof: f.prove(t, wr.key),
		})
	}

	// DigestPreSets: validatorsSeenRoot (class A), qualifiedRoot (freeze source anchor), epochSetRoot
	// (freeze digest).
	w.DigestPreSets = append(w.DigestPreSets,
		StateRootDigestWitness{Tag: tagValidatorsSeenRoot, PreIDs: f.sortedSeenIDs(), Proof: f.prove(t, statehash.Key(tagValidatorsSeenRoot, nil))},
		StateRootDigestWitness{Tag: tagQualifiedRoot, PreIDs: f.sortedIDs(f.c.qualified), Proof: f.prove(t, statehash.Key(tagQualifiedRoot, nil))},
		StateRootDigestWitness{Tag: tagEpochSetRoot, PreIDs: f.sortedIDs(f.c.epochSet), Proof: f.prove(t, statehash.Key(tagEpochSetRoot, nil))},
	)

	// Rotate witness: frozen members (the POST-qualified freeze set) + prior epochSet droppers +
	// scalars + the SeenSet maturity witness.
	var rw StateRootRotateWitness
	for id, wt := range applied.qualified {
		esKey := statehash.Key(tagEpochSet, id[:])
		// POST-apply regVersion (DIRECTION B): equals pre-state here (no in-block bond at the handoff).
		rv, rvKnown := applied.regVersion[id]
		rw.Members = append(rw.Members, StateRootRotateMember{
			ID: id, Weight: wt, RegVersion: rv, RegVersionKnown: rvKnown,
			EpochSetProof: f.prove(t, esKey), EpochSetOldValue: f.preValue(esKey),
			QualifiedProof:  mustProve(f.prover, statehash.Key(tagQualified, id[:])),
			RegVersionProof: mustProve(f.prover, statehash.Key(tagRegVersion, id[:])),
		})
	}
	for id := range f.c.epochSet {
		if _, stillFrozen := applied.qualified[id]; stillFrozen {
			continue
		}
		esKey := statehash.Key(tagEpochSet, id[:])
		wit, sibs := f.proveWithSiblings(t, esKey)
		rw.PriorEpochSet = append(rw.PriorEpochSet, StateRootRotateMember{
			ID: id, EpochSetOldValue: f.preValue(esKey), EpochSetProof: wit, EpochSetDeleteSiblings: sibs,
		})
	}
	scalarWit := func(tag string) StateRootRotateScalar {
		key := statehash.Key(tag, nil)
		return StateRootRotateScalar{OldValue: f.preValue(key), Proof: f.prove(t, key)}
	}
	rw.EpochStart = scalarWit(tagEpochStart)
	rw.MatureEpoch = scalarWit(tagMatureEpoch)
	rw.GateLockedIn = scalarWit(tagGateLockedIn)
	rw.GateHeight = scalarWit(tagGateHeight)
	rw.Era3LockedIn = scalarWit(tagEra3LockedIn)
	rw.Era3Height = scalarWit(tagEra3Height)
	rw.Era4LockedIn = scalarWit(tagEra4LockedIn)
	rw.Era4Height = scalarWit(tagEra4Height)
	w.Rotate = &rw

	// Class M maturity witness (the SINGLE owner of the tagEverMature write): the pre-latch everMature
	// scalar proof + the POST-apply SeenSet. On this fixture the boundary block IS the crossing, so
	// class M reconstructs matureNow over the applied state and emits the everMature false→true op.
	w.Maturity = &StateRootMaturityWitness{
		EverMature:  scalarWit(tagEverMature),
		MatureEpoch: scalarWit(tagMatureEpoch),
		SeenSet:     f.seenWitnessPost(t, applied),
	}

	// dueBucket scope-gate proof (non-membership at b.Height).
	var hk [8]byte
	putUint64BE(hk[:], b.Height)
	w.DueBucketProof = f.prove(t, statehash.Key(tagDueBucket, hk[:]))
	return w
}

func (f handoffFixture) sortedSeenIDs() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.validatorsSeen))
	for id := range f.c.validatorsSeen {
		out = append(out, id)
	}
	return sortIDs(out)
}

func (f handoffFixture) committedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// --- Handoff POSITIVE: the box AGREES with real apply() over the young→mature handoff boundary. The
// boundary block seats the attesters, the latch flips everMature false→true, and the freeze fires —
// the box must reproduce the M-write + the freeze. This MUST fail against the pre-fix code (verified
// by the ablation below, which forces the pre-everMature read). ---
func TestRecomputeStateRootRotateHandoffAgreesWithApply(t *testing.T) {
	f := buildHandoffFixture(t)
	b := f.handoffBoundaryBlock()

	// Precondition: the boundary block IS the handoff — apply() flips everMature false→true here.
	applied := f.c.cloneForDryRun()
	applied.apply(b)
	if !applied.everMature {
		t.Fatalf("fixture: the boundary block must latch everMature true (the handoff); got false")
	}
	if len(applied.epochSet) == 0 {
		t.Fatalf("fixture: the handoff freeze must produce a non-empty epochSet")
	}

	committed := f.committedRoot(t, b)
	w := f.witnessForHandoff(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("handoff recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- DIRECTION A MatureEpoch SUPPRESS gate (classP-anchoring cert 2026-09-02, P-s2). The handoff
// boundary is the first mature rotation: apply() flips matureEpoch false→true. A forged
// MatureEpoch.OldValue=true makes scalarFoldOp suppress the emit (post==pre), so the matureEpoch
// write is omitted and never fold-checked. The attacker commits a root that OMITS the matureEpoch
// write. Direction A (rotateOps → anchorRotateScalar(tagMatureEpoch)) Resolves the committed
// pre-value (false) present against prevStateRoot BEFORE the emit decision; a forged =true fails
// IsProvenPresent ⇒ STALL. This gate forges the suppression and asserts the box STALLS. ---
func TestRecomputeStateRootRotateMatureEpochOldValueSuppressionStalls(t *testing.T) {
	f := buildHandoffFixture(t)
	b := f.handoffBoundaryBlock()

	// Baseline: the honest witness (MatureEpoch.OldValue=false, the real flip) AGREES with apply().
	committed := f.committedRoot(t, b)
	w := f.witnessForHandoff(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline must agree: %v", err)
	}

	// FORGE: claim matureEpoch was ALREADY latched (OldValue=true) ⇒ scalarFoldOp suppresses the emit.
	fw := f.witnessForHandoff(t, b)
	fw.Rotate.MatureEpoch.OldValue = statehash.EncodeBool(true)

	// The attacker commits the SUPPRESSED root: the post-apply state with matureEpoch forced back to
	// false (the false→true write omitted).
	sup := f.c.cloneForDryRun()
	sup.apply(b)
	sup.matureEpoch = false
	forgedRoot, err := sup.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("forgedRoot: %v", err)
	}
	if forgedRoot == committed {
		t.Fatalf("GATE VACUOUS: forged suppressed root == honest committed root")
	}

	if rerr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forgedRoot, b, fw); rerr == nil {
		t.Fatalf("ANCHOR REGRESSED: box WRONG-ACCEPTS a forged MatureEpoch.OldValue=true suppression.\n"+
			"  Direction A (anchorRotateScalar(tagMatureEpoch)) must STALL a forged pre-latch value.\n"+
			"  forgedRoot=%x honest=%x", forgedRoot, committed)
	} else {
		t.Logf("ANCHOR HOLDS (Direction A): a forged MatureEpoch.OldValue=true STALLS (%v) — the matureEpoch "+
			"pre-state anchor catches the latch suppression.", rerr)
	}
}

// --- Handoff ablation: force P to consume the PRE everMature (the bug). The box then takes the
// early-return, freezes NOTHING, and omits the everMature/matureEpoch/epochSet writes ⇒ recomputed
// root != committed StateRoot ⇒ STALL (RED). Restoring the post-latch read makes it GREEN (the
// positive test above). This proves the fix is load-bearing at the exact boundary the gap lives on. ---
func TestRecomputeStateRootRotateHandoffAblationPreEverMature(t *testing.T) {
	f := buildHandoffFixture(t)
	b := f.handoffBoundaryBlock()
	committed := f.committedRoot(t, b)
	w := f.witnessForHandoff(t, b)

	// Reproduce the BUG: pre-fix, rotateOps read the PRE everMature (false on the handoff) and took the
	// !everMature early-return — writing ONLY epochStart, NO freeze, NO everMature/matureEpoch/epochSet
	// writes. Model that exact buggy class-P op set (epochStart-only) and fold it with the honest E/R+A
	// ops; the recomputed root must DIVERGE from the honest committed root (the box would STALL).
	buggyPOps := []statehash.FoldOp{{
		Key:      statehash.Key(tagEpochStart, nil),
		OldValue: w.Rotate.EpochStart.OldValue,
		NewValue: statehash.EncodeUint64(b.Height),
		Proof:    w.Rotate.EpochStart.Proof,
	}}
	entryOps := f.nonRotateOps(t, b, w)
	buggyRoot, err := statehash.FoldChangedPaths(f.prevRoot, append(entryOps, buggyPOps...))
	if err != nil {
		t.Fatalf("fold (buggy): %v", err)
	}
	if buggyRoot == committed {
		t.Fatalf("ABLATION FAILED: the pre-everMature freeze must diverge the root, but it matched")
	}

	// And the FIXED path AGREES (post-latch) — the RED→GREEN pair on the same fixture.
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("the fixed (post-latch) recompute should AGREE, got %v", err)
	}
}

// nonRotateOps rebuilds the E/R + class-A FoldOps the entry constructs for the handoff block, so the
// ablation can fold them together with a BUGGY class-P op set. It mirrors the entry's write-set →
// FoldOp assembly (floorbox_recompute_stateroot_v5.go) for E/R + A only.
func (f handoffFixture) nonRotateOps(t *testing.T, b Block, w StateRootWitness) []statehash.FoldOp {
	t.Helper()
	witByKey := map[string]*StateRootChangedLeafWitness{}
	for i := range w.ChangedLeaves {
		witByKey[string(w.ChangedLeaves[i].Key)] = &w.ChangedLeaves[i]
	}
	var ops []statehash.FoldOp
	// Class A digest ops + per-member writes.
	aOps, aWrites, err := f.c.attOps(f.prevRoot, b, w, livePreForProbe(f.c))
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
