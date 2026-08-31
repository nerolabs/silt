package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the P1-b class-S changed-whole-set-digest state-root recompute
// (floorbox_recompute_stateroot_slash_v5.go).
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md
//   PE ruling: RULING-floorbox-recompute-P1b-SA-digest-scope-2026-08-31.md
//
// R3 (execution-derived drift guard, MANDATORY): the box's derived S write-set + digest
// reconstruction is checked against the REAL apply() + StateRootForVersion(5) — the oracle a full
// node uses — and ablated RED on a forged per-member screen value, a mis-derived delta, a wrong
// culprit, an omitted digest reconstruction, and the circular prevStateRoot/StateRoot anchor swap.
// A hand-written mirror shares the producer's blind spot (the session-7 scar), so the ground truth
// is real execution.

// slashFixture is a v5 chain whose pre-state has a bonded+qualified culprit ready to slash, plus
// prevStateRoot and a Prover over its v5 leaf set. The box holds prevStateRoot.
type slashFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
	proposer ed25519.PrivateKey
	culprit  ed25519.PrivateKey
}

// buildSlashFixture seats a proposer (objective) and a culprit, both bonded above MinBond so both
// are qualified. It applies a couple of entries so the pre-state also has E/R leaves. TTL is
// enabled so the dueBucket scope-gate path is exercised; EpochBlocks is large so no boundary fires.
func buildSlashFixture(t *testing.T) slashFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(53001)
	culprit := key(53002)
	// Genesis seats the proposer's bond (objective) + the culprit's bond, both > MinBond → both
	// bonded AND qualified. A couple of entries give E/R leaves.
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30), entry(31)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(culprit, ports.HashBytes(pubOf(culprit)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)

	// Confirm the culprit is bonded AND qualified pre-slash (else the ablations are vacuous).
	cid := ports.HashBytes(pubOf(culprit))
	if _, ok := c.bonded[cid]; !ok {
		t.Fatalf("fixture: culprit not bonded pre-slash")
	}
	if _, ok := c.qualified[cid]; !ok {
		t.Fatalf("fixture: culprit not qualified pre-slash")
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
	return slashFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, culprit: culprit}
}

// slashBlock builds a slash-only block (plus one E/R entry) at the fixture head slashing the
// fixture's culprit.
func (f slashFixture) slashBlock() Block {
	prev, h := f.c.Head()
	b := Block{
		Version: BlockVersionWitnessable,
		Height:  h,
		Prev:    prev,
		Entries: []ports.Entry{entry(40)},
		Slashes: []Equivocation{slashProof(f.culprit, prev, 0x41, 0x42)},
	}
	return b
}

// preIDs returns the pre-state member id-list of the named keyspace, read from the fixture chain.
func (f slashFixture) preIDsBonded() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.bonded))
	for id := range f.c.bonded {
		out = append(out, id)
	}
	return sortIDs(out)
}
func (f slashFixture) preIDsQualified() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.qualified))
	for id := range f.c.qualified {
		out = append(out, id)
	}
	return sortIDs(out)
}
func (f slashFixture) preIDsSlashed() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.slashed))
	for id := range f.c.slashed {
		out = append(out, id)
	}
	return sortIDs(out)
}

// digestWitness proves one digest leaf against prevStateRoot and packs the pre-set id-list.
func (f slashFixture) digestWitness(t *testing.T, tag string, preIDs []ports.NodeID) StateRootDigestWitness {
	t.Helper()
	key := statehash.Key(tag, nil)
	wit, err := f.prover.Prove(key)
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootDigestWitness{Tag: tag, PreIDs: preIDs, Proof: wit}
}

// witnessForSlash builds the full O(payload)+O(registry-per-digest) witness for a slash block: the
// E/R changed-leaf proofs, the S per-member changed-leaf proofs (slashed add, bonded delete,
// qualified delete), the three digest pre-set witnesses, and the dueBucket scope-gate proof.
func (f slashFixture) witnessForSlash(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	var w StateRootWitness

	// E/R changed leaves.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	// The S per-member write-set: build it with the SAME pre-membership the box will derive, so the
	// test witness set matches the box's derived key set.
	preBonded := idSet(f.preIDsBonded())
	preQualified := idSet(f.preIDsQualified())
	for _, wr := range stateRootSlashWriteSet(b, preBonded, preQualified) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}

	// The three touched digest pre-sets.
	w.DigestPreSets = []StateRootDigestWitness{
		f.digestWitness(t, tagSlashedRoot, f.preIDsSlashed()),
		f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
		f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
	}

	// dueBucket TTL scope-gate proof (non-membership at b.Height).
	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dk := statehash.Key(tagDueBucket, hk[:])
		dp, err := f.prover.Prove(dk)
		if err != nil {
			t.Fatalf("Prove(dueBucket): %v", err)
		}
		w.DueBucketProof = dp
	}
	return w
}

// leafWitness proves one changed per-member/E-R leaf against prevStateRoot, reading the true
// committed pre-state value for OldValue (nil if absent). For a delete it also collects the
// off-path sibling preimages.
func (f slashFixture) leafWitness(t *testing.T, wr stateRootWrite) StateRootChangedLeafWitness {
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

// preValue returns the committed pre-state value of a leaf key from the fixture chain's v5 leaf set.
func (f slashFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

// applyAndCommittedRoot applies b to a CLONE (real apply()) and returns the committed v5 StateRoot
// — the R3 oracle.
func (f slashFixture) applyAndCommittedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

func idSet(ids []ports.NodeID) map[ports.NodeID]struct{} {
	m := make(map[ports.NodeID]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func putUint64BE(b []byte, v uint64) {
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
}

// --- Ablation 1: the S recompute AGREES with real apply() over a slash-of-a-qualified-culprit. ---
func TestRecomputeStateRootSlashAgreesWithApply(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForSlash(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("S recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Byte-exact: the reconstructed post-digests equal statehash's nodeSetMTH over the real post-set.
func TestRecomputeStateRootSlashDigestsAreByteExact(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()

	// Real post-state via apply().
	clone := f.c.cloneForDryRun()
	clone.apply(b)

	ops, _, _, err := stateRootSlashDigestOps(b, []StateRootDigestWitness{
		f.digestWitness(t, tagSlashedRoot, f.preIDsSlashed()),
		f.digestWitness(t, tagBondedRoot, f.preIDsBonded()),
		f.digestWitness(t, tagQualifiedRoot, f.preIDsQualified()),
	})
	if err != nil {
		t.Fatalf("stateRootSlashDigestOps: %v", err)
	}
	want := map[string][]byte{
		string(statehash.Key(tagSlashedRoot, nil)):   nodeSetMTHFromBool(clone.slashed),
		string(statehash.Key(tagBondedRoot, nil)):    nodeSetMTHFromInt64(clone.bonded),
		string(statehash.Key(tagQualifiedRoot, nil)): nodeSetMTHFromInt64(clone.qualified),
	}
	for _, op := range ops {
		w := want[string(op.Key)]
		if string(op.NewValue) != string(w) {
			t.Fatalf("digest %x NewValue not byte-exact: got %x want %x", op.Key, op.NewValue, w)
		}
	}
}

// --- Ablation 2: forged per-member screen value — claim the culprit was NOT qualified pre-state
// (drop it from the qualified pre-set). The box then does NOT delete it from post-qualified ⇒ wrong
// qualifiedRoot ⇒ post-root != StateRoot ⇒ stall. (C-1: the delta is derived from the anchored
// pre-set, and a forged pre-set fails the fold's pre-digest anchor OR diverges the post-root.)
func TestRecomputeStateRootSlashAblationForgedQualifiedScreen(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForSlash(t, b)

	// Drop the culprit from the qualified pre-set id-list handed to the box.
	cid := ports.HashBytes(pubOf(f.culprit))
	for i := range w.DigestPreSets {
		if w.DigestPreSets[i].Tag == tagQualifiedRoot {
			w.DigestPreSets[i].PreIDs = dropID(w.DigestPreSets[i].PreIDs, cid)
		}
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a forged qualified pre-set (culprit dropped) must stall, got nil")
	}
	// It stalls either at the fold's pre-digest anchor (VerifyProof) or at the terminal mismatch —
	// both are correct never-Accept outcomes.
	if !errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected a fold/mismatch stall, got %v", err)
	}
}

// --- Ablation 3: mis-derived delta — the slash does NOT delete bonded. This ablation drives the
// REAL RecomputeStateRootEntriesRevocations path (session-7 scar: a hand-built root comparison that
// never touches the production fold is DECORATION — it stays green even if the box's bonded delete
// regresses). We forge a committed StateRoot that reflects the BUGGY post-state (culprit STILL
// bonded post-slash) and hand the box an HONEST witness. The box derives the CORRECT S delta (it
// DELETES the culprit from post-bonded, stateroot_slash_v5.go:176), folds the honest bondedRoot, and
// that must MISMATCH the buggy committed root ⇒ ErrRecomputeStateRootMismatch. This proves the box's
// bonded delete is load-bearing: were the box to skip the delete (the regression this guards), its
// recompute would MATCH the buggy committed root and wrongly return nil.
func TestRecomputeStateRootSlashAblationBondedNotDeleted(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	cid := ports.HashBytes(pubOf(f.culprit))

	// The honest committed root: apply() deletes the culprit from bonded.
	honestClone := f.c.cloneForDryRun()
	honestClone.apply(b)
	honestBonded := nodeSetMTHFromInt64(honestClone.bonded)

	// The BUGGY committed root: apply the slash, then re-insert the culprit into bonded at its
	// pre-slash bond value — modelling "the slash did NOT delete bonded". Everything else (slashed,
	// qualified, E/R) is the honest post-state, so ONLY the bonded set membership differs.
	buggyClone := f.c.cloneForDryRun()
	preBond, ok := buggyClone.bonded[cid]
	if !ok {
		t.Fatalf("fixture: culprit not bonded pre-slash")
	}
	buggyClone.apply(b)
	if _, stillBonded := buggyClone.bonded[cid]; stillBonded {
		t.Fatalf("fixture: apply() did NOT delete the culprit from bonded — ablation is vacuous")
	}
	buggyClone.bonded[cid] = preBond // undo the delete: the mis-derived (bonded-not-deleted) post-state
	buggyBonded := nodeSetMTHFromInt64(buggyClone.bonded)
	if string(buggyBonded) == string(honestBonded) {
		t.Fatalf("ABLATION VACUOUS: re-adding the culprit produced the SAME bondedRoot as the honest " +
			"delete — the culprit must be bonded pre-slash for this ablation to bite")
	}
	buggyCommitted, err := buggyClone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("buggy StateRootForVersion: %v", err)
	}

	// Drive the REAL path with an HONEST witness against the BUGGY committed root. The box's honest
	// bonded delete makes its recomputed root diverge from the buggy committed root ⇒ terminal stall.
	w := f.witnessForSlash(t, b)
	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, buggyCommitted, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a committed root reflecting bonded-not-deleted must stall the honest " +
			"recompute, got nil (the box's bonded delete is not load-bearing)")
	}
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected ErrRecomputeStateRootMismatch (honest bondedRoot != buggy "+
			"committed root), got %v", err)
	}
}

// --- Ablation 4: wrong culprit — slash a DIFFERENT (unbonded/unqualified) id than the block names.
// The box derives the delta from b.Slashes (the real culprit), so a witness that pre-sets a wrong
// id changes the wrong digest ⇒ post-root != StateRoot ⇒ stall. Here we corrupt the block's slash
// to name a culprit not in the pre-sets and confirm the recompute stalls against the honest root.
func TestRecomputeStateRootSlashAblationWrongCulprit(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	honestCommitted := f.applyAndCommittedRoot(t, b)

	// Build the witness for the HONEST block, but hand the box a block that slashes a DIFFERENT id.
	w := f.witnessForSlash(t, b)
	other := key(53099)
	bWrong := b
	prev, _ := f.c.Head()
	bWrong.Slashes = []Equivocation{slashProof(other, prev, 0x51, 0x52)}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, honestCommitted, bWrong, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a block naming a different culprit than the committed root reflects must stall, got nil")
	}
	// The wrong culprit's slashed||other ADD has no supplied witness (the witness was built for the
	// honest culprit) ⇒ the fold stalls; even if witnessed, the recomputed root would mismatch the
	// honest committed root. Both are never-Accept stalls.
	if !errors.Is(err, ErrRecomputeStateRootFold) && !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: expected a fold/mismatch stall for a wrong culprit, got %v", err)
	}
}

// --- Ablation 5: OMITTED touched-digest reconstruction — drop the qualifiedRoot pre-set witness.
// The box cannot reconstruct that digest change ⇒ stall (never folds an unwitnessed digest change).
func TestRecomputeStateRootSlashAblationOmittedDigest(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForSlash(t, b)

	// Drop the qualifiedRoot digest witness.
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

// --- Ablation 6: the pre-set anchored against StateRoot instead of prevStateRoot (the circular
// bug). We prove the digest leaves against the POST-state root (committed StateRoot) and hand those
// proofs to the box, which verifies OldValue against prevStateRoot ⇒ the proof fails ⇒ stall. This
// proves the anchor is prevStateRoot, not StateRoot.
func TestRecomputeStateRootSlashAblationCircularAnchor(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	committed := f.applyAndCommittedRoot(t, b)

	// A prover over the POST-state (committed StateRoot). Proofs issued here are against StateRoot.
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	postLeaves := clone.stateRootLeavesV5()
	postProver, err := statehash.NewProver(postLeaves)
	if err != nil {
		t.Fatalf("post NewProver: %v", err)
	}
	if postProver.Root() != committed {
		t.Fatalf("post-prover root != committed StateRoot")
	}

	w := f.witnessForSlash(t, b)
	// Overwrite the digest proofs with POST-state (StateRoot-anchored) proofs — the circular anchor.
	// Keep the pre-set id-lists honest; only the proof anchor is wrong. The box's fold verifies each
	// digest op's OldValue (the PRE-digest) against prevStateRoot, so a StateRoot-anchored proof of a
	// value that differs from the post-committed value fails.
	replace := func(tag string, preIDs []ports.NodeID) statehash.Witness {
		wit, perr := postProver.Prove(statehash.Key(tag, nil))
		if perr != nil {
			t.Fatalf("post Prove(%s): %v", tag, perr)
		}
		return wit
	}
	for i := range w.DigestPreSets {
		d := &w.DigestPreSets[i]
		d.Proof = replace(d.Tag, d.PreIDs)
	}

	err = f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: a StateRoot-anchored (circular) digest proof must fail the fold's "+
			"prevStateRoot verify, got %v", err)
	}
}

// --- Ablation 7: an out-of-scope compound — a slash block that ALSO carries a non-proposer att
// (class A) stalls at the scope gate, never Accepts. (Class B bond regs are now IN scope — P1-d — so
// the out-of-scope compound uses class A, which stays deferred on the R-A-frozenset residual.)
func TestRecomputeStateRootSlashAblationCompoundOutOfScope(t *testing.T) {
	f := buildSlashFixture(t)
	b := f.slashBlock()
	bb := b
	b.Atts = append(b.Atts, Attest(&bb, key(53077)))
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForSlash(t, b)

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootScopeStall) {
		t.Fatalf("ABLATION FAILED: a slash+non-proposer-att compound must stall at the scope gate, got %v", err)
	}
}

func dropID(ids []ports.NodeID, drop ports.NodeID) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}
