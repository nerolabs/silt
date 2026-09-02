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
	// Class M: mature-from-genesis fixture ⇒ everMature already latched (pre=true), no crossing.
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
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

// --- 7f: class-T MULTI-BLOCK contiguity schedule. ---
//
// The dueBucket[b.Height] scope-gate (floorbox_recompute_stateroot_v5.go:383-410) is sound ONLY under
// chain height-CONTIGUITY: apply()'s sweep loop is `b.Height-regH > ttl` (chain.go:3273, a `>=` over
// ALL overdue ids), while the recompute reads the SINGLE bucket keyed at EXACTLY b.Height. These
// coincide because dueBucketMoveOnReg files each id at due = regH+ttl+1, and every intervening height
// runs a sweep — so when the chain reaches height h, every earlier bucket has already emptied and the
// only ids overdue at h are exactly dueBucket[h]. gate 7c ARGUES this; this test EXERCISES it: a
// multi-block schedule of consecutive sweep heights, each firing a DIFFERENT bucket, reproduced
// byte-exact; and an ablation where a height is SKIPPED so apply()'s `>=` vacuums a bucket the
// recompute's `==` gate never witnesses ⇒ stall. A future break of contiguity reddens the ablation.

// buildStaggeredTTLChain seats a proposer (renewed every block, never expires) plus expirers filed
// into DISTINCT due-buckets, and returns the chain positioned just before the first sweep. ttl=2 so
// an id registered at height r is due at r+3. Expirers are registered at genesis and h1 so they land
// in buckets 3 and 4; the proposer renews each block so it never co-occupies an expirer's bucket.
func buildStaggeredTTLChain(t *testing.T) (*Chain, ed25519.PrivateKey, ports.NodeID, ports.NodeID) {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(77001)
	e1 := key(77002) // registered at genesis ⇒ due 0+3 = 3
	e2 := key(77003) // registered at h1      ⇒ due 1+3 = 4
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(e1, ports.HashBytes(pubOf(e1)), 4<<20, ports.Hash{}, 5, 2))
	Sign(g, prop)
	c.apply(*g)
	// h1: renew prop, register e2.
	prev, _ := c.Head()
	c.apply(Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev,
		BondRegs: []BondReg{
			bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, prev, 5, 1),
			bondRegFull(e2, ports.HashBytes(pubOf(e2)), 4<<20, prev, 5, 3),
		}})
	// h2: renew prop only (keeps prop's bucket ahead of the sweep window).
	prev, _ = c.Head()
	c.apply(Block{Version: BlockVersionWitnessable, Height: 2, Prev: prev,
		BondRegs: []BondReg{bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, prev, 5, 1)}})
	return c, prop, ports.HashBytes(pubOf(e1)), ports.HashBytes(pubOf(e2))
}

// recomputeSweepAt captures a prover over the chain's CURRENT v5 leaf set, builds the class-T witness
// for the block b that fires the sweep at b.Height, and runs the REAL recompute against the real
// apply() committed root. It returns the recompute error (nil = agrees).
func recomputeSweepAt(t *testing.T, c *Chain, prop ed25519.PrivateKey, b Block) error {
	t.Helper()
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
	f := ttlFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, sweepH: b.Height}
	expired := []ports.NodeID{}
	for id := range c.dueBucket[b.Height] {
		expired = append(expired, id)
	}
	expired = sortIDs(expired)
	committed := f.applyAndCommittedRoot(t, b)
	w := f.ttlSweepWitness(t, b, expired)
	return c.RecomputeStateRootEntriesRevocations(prevRoot, committed, b, w)
}

// TestRecomputeStateRootTTLMultiBlockScheduleAgreesWithApply is the POSITIVE 7f test: two consecutive
// sweep heights (h=3 fires e1's bucket, h=4 fires e2's bucket), each reproduced byte-exact vs the real
// apply(). Between them the chain advances (h=3 applied for real) so h=4's pre-state reflects the h=3
// sweep — the contiguity the gate relies on.
func TestRecomputeStateRootTTLMultiBlockScheduleAgreesWithApply(t *testing.T) {
	c, prop, e1, e2 := buildStaggeredTTLChain(t)

	// h=3: e1's bucket fires. Confirm the fixture is real, run the recompute, then apply for real.
	if _, ok := c.dueBucket[3][e1]; !ok {
		t.Fatalf("fixture: e1 not due at bucket 3")
	}
	prev, _ := c.Head()
	b3 := Block{Version: BlockVersionWitnessable, Height: 3, Prev: prev, Entries: []ports.Entry{entry(41)}}
	if err := recomputeSweepAt(t, c, prop, b3); err != nil {
		t.Fatalf("h=3 sweep recompute should AGREE with real apply() but stalled: %v", err)
	}
	c.apply(b3) // advance the chain contiguously
	if _, still := c.bonded[e1]; still {
		t.Fatalf("fixture: h=3 sweep did not expire e1")
	}

	// h=4: e2's bucket fires, over the post-h=3 pre-state.
	if _, ok := c.dueBucket[4][e2]; !ok {
		t.Fatalf("fixture: e2 not due at bucket 4")
	}
	prev, _ = c.Head()
	b4 := Block{Version: BlockVersionWitnessable, Height: 4, Prev: prev, Entries: []ports.Entry{entry(42)}}
	if err := recomputeSweepAt(t, c, prop, b4); err != nil {
		t.Fatalf("h=4 sweep recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// TestRecomputeStateRootTTLAblationSkippedHeightContiguityBreak is the 7f ablation: SKIP a sweep
// height so apply()'s `>=` loop vacuums a bucket the recompute's `==` gate at a LATER height never
// witnesses. e1 is due at bucket 3; instead of applying h=3 then h=4 contiguously, we SKIP h=3 and
// apply h=4 directly. apply()'s sweep at h=4 finds e1 overdue (4-0 > 2) and expires it, but e1 sits in
// dueBucket[3], NOT dueBucket[4]. The recompute at h=4 witnesses dueBucket[4] (empty of e1), derives an
// expired set that OMITS e1, and folds a bondedRoot that still CONTAINS e1 — which MISMATCHES the
// committed root that reflects e1's expiry ⇒ stall. This reddens the moment height-contiguity breaks.
// Red-before-green: the CONTIGUOUS schedule (the positive test above) AGREES.
func TestRecomputeStateRootTTLAblationSkippedHeightContiguityBreak(t *testing.T) {
	c, prop, e1, _ := buildStaggeredTTLChain(t)
	if _, ok := c.dueBucket[3][e1]; !ok {
		t.Fatalf("fixture: e1 not due at bucket 3")
	}

	// SKIP h=3. Apply h=4 directly: apply()'s `>=` sweep expires e1 (in bucket 3), but the recompute
	// at h=4 only witnesses dueBucket[4].
	prev, _ := c.Head()
	b4 := Block{Version: BlockVersionWitnessable, Height: 4, Prev: prev, Entries: []ports.Entry{entry(43)}}

	// Confirm the break is real: apply(b4) sweeps e1, yet dueBucket[4] does NOT contain e1.
	clone := c.cloneForDryRun()
	clone.apply(b4)
	if _, still := clone.bonded[e1]; still {
		t.Fatalf("fixture: apply's >= sweep at h=4 did NOT expire e1 — ablation vacuous")
	}
	if _, inB4 := c.dueBucket[4][e1]; inB4 {
		t.Fatalf("fixture: e1 unexpectedly in dueBucket[4] — no contiguity break to test")
	}

	err := recomputeSweepAt(t, c, prop, b4)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a skipped-height sweep (apply vacuums an unwitnessed bucket) must stall, got nil")
	}
	// The recompute's short expired set folds a bondedRoot that still contains e1; the committed root
	// reflects e1's expiry. The stall surfaces as a fold or final-equality mismatch — both never-Accept.
	if !errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootMismatch) &&
		!errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootTTLWitness) {
		t.Fatalf("ABLATION FAILED: expected a fold/mismatch/digest/ttl stall for the skipped-height break, got %v", err)
	}
}

// --- Ablation 5: a sweep+non-proposer-att compound. Class A is now IN scope (P1-e), so the block
// DISPATCHES to the A reconstruction. The sweep witness carries no A witness (AttScreens /
// validatorsSeenRoot digest), so the A dispatch stalls (never-Accept preserved). ---
func TestRecomputeStateRootTTLAblationCompoundOutOfScope(t *testing.T) {
	f := buildTTLFixture(t)
	b := f.sweepBlock()
	expired := f.expiredMembers()
	// Add a non-proposer att (class A — now in scope, but unwitnessed here).
	other := key(71077)
	b.LastCommit = []Attestation{carrierEntry(f.c, other)}
	committed := f.applyAndCommittedRoot(t, b)
	w := f.ttlSweepWitness(t, b, expired)

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a sweep+unwitnessed-att compound must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: expected a digest/fold stall for the unwitnessed class-A part, got %v", err)
	}
}
