package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the P1-c class-T TTL-sweep state-root recompute (floorbox_recompute_stateroot_ttl_v5.go).
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md
//     (T CERTIFIED-in-direction, inherits the CRUX dueBucket reconstruction).
//
// R3 (execution-derived drift guard, MANDATORY): the box's derived T write-set + digest+bucket
// reconstruction is checked against the REAL apply() + StateRootForVersion(5) — the oracle a full
// node uses — and ablated RED on a forged expired set (wrong dueBucket members), a mis-derived
// bonded/qualified delta, an omitted digest reconstruction, and the circular anchor. Ground truth is
// real execution (the session-7 scar: a hand-built mirror shares the producer's blind spot).

// ttlFixture is a v5 chain with a bonded+qualified member whose TTL is about to fire, advanced to
// the block BEFORE the sweep. prevStateRoot + a Prover over its v5 leaf set are captured there.
type ttlFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
	proposer ed25519.PrivateKey
	expirer  ed25519.PrivateKey
	sweepH   uint64
}

// buildTTLFixture seats a proposer and an expirer at genesis (ttl=4 ⇒ due height 5), advances empty
// blocks to h=4 renewing the proposer each block (so it never expires), and captures the pre-state
// at h=4. The sweep fires at h=5.
func buildTTLFixture(t *testing.T) ttlFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(71001)
	expirer := key(71002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30), entry(31)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(expirer, ports.HashBytes(pubOf(expirer)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)

	// Advance to h=4, renewing the proposer each block so it never expires; the expirer never renews.
	for h := uint64(1); h <= 4; h++ {
		prev, _ := c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, prev, 5, 1)}}
		c.apply(b)
	}
	eid := ports.HashBytes(pubOf(expirer))
	if _, ok := c.bonded[eid]; !ok {
		t.Fatalf("fixture: expirer not bonded at h=4")
	}
	if _, ok := c.qualified[eid]; !ok {
		t.Fatalf("fixture: expirer not qualified at h=4")
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
		t.Fatalf("fixture pre-root mismatch: prover=%x chain=%x", prevRoot, sr)
	}
	return ttlFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, expirer: expirer, sweepH: 5}
}

// sweepBlock builds the block at h=5 that fires the expirer's TTL sweep. It renews NOTHING (a pure
// sweep) so the ONLY state change is the expirer's expiry, plus one E/R entry.
func (f ttlFixture) sweepBlock() Block {
	prev, _ := f.c.Head()
	return Block{
		Version: BlockVersionWitnessable,
		Height:  f.sweepH,
		Prev:    prev,
		Entries: []ports.Entry{entry(40)},
	}
}

func (f ttlFixture) preIDsBonded() []ports.NodeID    { return sortIDs(mapIDs(f.c.bonded)) }
func (f ttlFixture) preIDsQualified() []ports.NodeID { return sortIDs(mapIDs(f.c.qualified)) }
func (f ttlFixture) preIDsSlashed() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.slashed))
	for id := range f.c.slashed {
		out = append(out, id)
	}
	return sortIDs(out)
}

func mapIDs(m map[ports.NodeID]int64) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

func (f ttlFixture) digestWitness(t *testing.T, tag string, preIDs []ports.NodeID) StateRootDigestWitness {
	t.Helper()
	wit, err := f.prover.Prove(statehash.Key(tag, nil))
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootDigestWitness{Tag: tag, PreIDs: preIDs, Proof: wit}
}

func (f ttlFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

func (f ttlFixture) leafWitness(t *testing.T, wr stateRootWrite) StateRootChangedLeafWitness {
	t.Helper()
	old := f.preValue(wr.key)
	if wr.newValue == nil {
		wit, sibs, err := f.prover.ProveWithSiblings(wr.key)
		if err != nil {
			t.Fatalf("ProveWithSiblings(%x): %v", wr.key, err)
		}
		return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit, DeleteSiblings: sibs}
	}
	wit, err := f.prover.Prove(wr.key)
	if err != nil {
		t.Fatalf("Prove(%x): %v", wr.key, err)
	}
	return StateRootChangedLeafWitness{Key: wr.key, OldValue: old, Proof: wit}
}

func (f ttlFixture) applyAndCommittedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// expiredMembers returns the members of dueBucket[sweepH] read from the fixture chain.
func (f ttlFixture) expiredMembers() []ports.NodeID {
	out := []ports.NodeID{}
	for id := range f.c.dueBucket[f.sweepH] {
		out = append(out, id)
	}
	return sortIDs(out)
}

// ttlSweepWitness builds the class-T witness: the dueBucket[h] member proof + delete siblings, the
// bonded/qualified/slashed digest pre-sets, the per-member DELETE proofs, and the E/R proofs.
func (f ttlFixture) ttlSweepWitness(t *testing.T, b Block, expired []ports.NodeID) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	// E/R changed leaves.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	// The dueBucket[h] bucket proof + delete siblings (the bucket empties).
	var hk [8]byte
	putUint64BE(hk[:], b.Height)
	bucketKey := statehash.Key(tagDueBucket, hk[:])
	bwit, bsibs, err := f.prover.ProveWithSiblings(bucketKey)
	if err != nil {
		t.Fatalf("ProveWithSiblings(dueBucket[%d]): %v", b.Height, err)
	}
	w.TTLSweep = &StateRootTTLWitness{
		Height:               b.Height,
		Members:              expired,
		BucketProof:          bwit,
		BucketDeleteSiblings: bsibs,
	}
	w.DueBucketProof = bwit // scope gate reads this against prevStateRoot

	// The digest pre-sets.
	w.DigestPreSets = []StateRootDigestWitness{
		f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
		f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
		f.digestWitness(t, tagSlashedRoot, f.preIDsSlashed()),
	}

	// The per-member DELETE proofs, built with the SAME anchored pre-qualified set the box derives.
	preQualified := idSet(f.preIDsQualified())
	for _, wr := range stateRootTTLWriteSet(expired, b.Height, preQualified) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	return w
}

// --- Ablation 1: the T recompute AGREES with real apply() over a firing sweep. ---
func TestRecomputeStateRootTTLAgreesWithApply(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()
	if len(expired) == 0 {
		t.Fatalf("fixture: no expired members in dueBucket[%d]", f.sweepH)
	}
	committed := f.applyAndCommittedRoot(t, b)
	w := f.ttlSweepWitness(t, b, expired)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("T recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Byte-exact: the reconstructed post-digests equal statehash's nodeSetMTH over the real post-set.
func TestRecomputeStateRootTTLDigestsAreByteExact(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()

	clone := f.c.cloneForDryRun()
	clone.apply(b)

	ops, _, _, _, err := stateRootTTLDigestOps(
		StateRootTTLWitness{Height: b.Height, Members: expired,
			BucketProof: f.ttlSweepWitness(t, b, expired).TTLSweep.BucketProof},
		[]StateRootDigestWitness{
			f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
			f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
		})
	if err != nil {
		t.Fatalf("stateRootTTLDigestOps: %v", err)
	}
	want := map[string][]byte{
		string(statehash.Key(tagBondedRoot, nil)):    nodeSetMTHFromInt64(clone.bonded),
		string(statehash.Key(tagQualifiedRoot, nil)): nodeSetMTHFromInt64(clone.qualified),
	}
	for _, op := range ops {
		w, ok := want[string(op.Key)]
		if !ok {
			continue // the bucket op
		}
		if string(op.NewValue) != string(w) {
			t.Fatalf("digest %x NewValue not byte-exact: got %x want %x", op.Key, op.NewValue, w)
		}
	}
}

// --- Ablation 2: forged expired set — drop the real expirer, add a bogus id. The reconstructed
// dueBucketMTH(Members) no longer matches the committed bucket value ⇒ the scope gate's present-proof
// fails ⇒ stall (the CRUX completeness anchor). ---
func TestRecomputeStateRootTTLAblationForgedExpiredSet(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.ttlSweepWitness(t, b, expired)

	// Corrupt the expired member list: swap the real expirer for a bogus id.
	bogus := ports.HashBytes(pubOf(key(71099)))
	w.TTLSweep.Members = []ports.NodeID{bogus}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a forged expired set must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootTTLWitness) {
		t.Fatalf("ABLATION FAILED: expected a TTL-witness stall (bucket MTH mismatch), got %v", err)
	}
}

// --- Ablation 3: mis-derived delta — a committed root reflecting bonded-NOT-deleted. We forge a
// StateRoot where the expirer is STILL bonded post-sweep and hand the box an HONEST witness. The box
// derives the CORRECT delta (DELETE the expirer from bonded), folds the honest bondedRoot, and that
// must MISMATCH the buggy committed root ⇒ ErrRecomputeStateRootMismatch. This drives the REAL
// recompute (session-7 scar: a hand-built comparison is decoration). ---
func TestRecomputeStateRootTTLAblationBondedNotDeleted(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()
	eid := expired[0]

	buggyClone := f.c.cloneForDryRun()
	preBond, ok := buggyClone.bonded[eid]
	if !ok {
		t.Fatalf("fixture: expirer not bonded pre-sweep")
	}
	buggyClone.apply(b)
	if _, still := buggyClone.bonded[eid]; still {
		t.Fatalf("fixture: apply() did NOT sweep the expirer — ablation vacuous")
	}
	buggyClone.bonded[eid] = preBond // undo the delete: the mis-derived post-state
	buggyCommitted, err := buggyClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("buggy StateRootForVersion: %v", err)
	}

	w := f.ttlSweepWitness(t, b, expired)
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, buggyCommitted, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a committed root reflecting bonded-not-deleted must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected ErrRecomputeStateRootMismatch, got %v", err)
	}
}

// --- Ablation 4: OMITTED touched-digest reconstruction — drop the qualifiedRoot pre-set witness.
// The box cannot reconstruct that digest change ⇒ stall. ---
func TestRecomputeStateRootTTLAblationOmittedDigest(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.ttlSweepWitness(t, b, expired)

	var kept []StateRootDigestWitness
	for _, d := range w.DigestPreSets {
		if d.Tag != tagQualifiedRoot {
			kept = append(kept, d)
		}
	}
	w.DigestPreSets = kept

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootDigest) {
		t.Fatalf("ABLATION FAILED: an omitted touched-digest witness must stall with ErrRecomputeStateRootDigest, got %v", err)
	}
}

// --- Ablation 5: an out-of-scope compound — a sweep block that ALSO carries a non-proposer att
// stalls at the scope gate (A is out of scope), never Accepts. ---
func TestRecomputeStateRootTTLAblationCompoundOutOfScope(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()
	// Add a non-proposer att (class A, out of scope).
	other := key(71077)
	bb := b
	b.Atts = []Attestation{Attest(&bb, other)}
	committed := f.applyAndCommittedRoot(t, b)
	w := f.ttlSweepWitness(t, b, expired)

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootScopeStall) {
		t.Fatalf("ABLATION FAILED: a sweep+non-proposer-att compound must stall at the scope gate, got %v", err)
	}
}
