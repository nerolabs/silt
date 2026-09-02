package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the P1-e class-A attestations → validatorsSeen state-root recompute
// (floorbox_recompute_stateroot_atts_v5.go).
//
// CERTIFIED-IN-DIRECTION (2026-08-31):
//   research: floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md
//
// R3 (execution-derived drift guard, MANDATORY): the box's derived A write-set + validatorsSeenRoot
// reconstruction is checked against the REAL apply() + StateRootForVersion(5), and ablated RED on a
// forged qualification screen, a legacy-mode block, and an omitted validatorsSeenRoot. Each ablation
// drives the REAL RecomputeStateRootEntriesRevocations (a hand-built mirror shares the producer's
// blind spot — the session-7 scar).

type attFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
	proposer ed25519.PrivateKey
	att      ed25519.PrivateKey
}

// buildAttFixture seats a proposer + an attester, both bonded above MinBond → both qualified and in
// the frozen epochSet (matureEpoch=true via MatureValidators=0). EpochBlocks large so no boundary
// fires at the test height. TTL enabled so the dueBucket scope-gate path is exercised.
func buildAttFixture(t *testing.T) attFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(54001)
	att := key(54002)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30)}}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1),
		bondRegFull(att, ports.HashBytes(pubOf(att)), 4<<20, ports.Hash{}, 5, 2),
	)
	Sign(g, prop)
	c.apply(*g)

	aid := ports.HashBytes(pubOf(att))
	if !c.attesterQualified(aid) {
		t.Fatalf("fixture: attester not qualified pre-block")
	}
	if c.validatorsSeen[aid] {
		t.Fatalf("fixture: attester already in validatorsSeen (ablation would be vacuous)")
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
	return attFixture{c: c, prevRoot: prevRoot, prover: prover, proposer: prop, att: att}
}

// attBlock builds a block with one E/R entry + a LastCommit carrier holding one carried signer
// (the fixture's attester, who is not the parent's proposer). R-BOX-ATTESTS O1: the v5 seating
// source is the hash-covered carrier over b.Prev, not the block's own Atts.
func (f attFixture) attBlock() Block {
	prev, h := f.c.Head()
	head, _ := f.c.headBlock()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(42)}}
	b.LastCommit = append(b.LastCommit, AttestAt(&head, f.att, 0, PhasePrecommit))
	return b
}

func (f attFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

func (f attFixture) leafWitness(t *testing.T, wr stateRootWrite) StateRootChangedLeafWitness {
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

func (f attFixture) digestWitness(t *testing.T, tag string, preIDs []ports.NodeID) StateRootDigestWitness {
	t.Helper()
	wit, err := f.prover.Prove(statehash.Key(tag, nil))
	if err != nil {
		t.Fatalf("Prove(%s): %v", tag, err)
	}
	return StateRootDigestWitness{Tag: tag, PreIDs: preIDs, Proof: wit}
}

func (f attFixture) preIDsValidatorsSeen() []ports.NodeID {
	out := make([]ports.NodeID, 0, len(f.c.validatorsSeen))
	for id := range f.c.validatorsSeen {
		out = append(out, id)
	}
	return sortIDs(out)
}

// attScreen builds the per-attester screen witness reading the fixture chain's committed pre-state,
// including the R1.2 per-field proofs against prevStateRoot (slashed / epochSet / bonded).
func (f attFixture) attScreen(id ports.NodeID) StateRootAttScreen {
	sz, bp := f.c.bonded[id]
	esVal, inES := f.c.epochSet[id]
	sc := StateRootAttScreen{
		Attester:      id,
		Slashed:       f.c.slashed[id],
		InEpochSet:    inES,
		BondedSize:    sz,
		BondedPresent: bp,
	}
	sc.SlashedProof = mustProve(f.prover, statehash.Key(tagSlashed, id[:]))
	sc.EpochSetProof = mustProve(f.prover, statehash.Key(tagEpochSet, id[:]))
	if inES {
		sc.EpochSetValue = statehash.EncodeInt64(esVal)
	}
	sc.BondedProof = mustProve(f.prover, statehash.Key(tagBonded, id[:]))
	return sc
}

// witnessForAtt builds the full witness for an att block: E/R + validatorsSeen changed leaves, the
// validatorsSeenRoot digest pre-set, the per-attester screens, and the dueBucket scope-gate proof.
func (f attFixture) witnessForAtt(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	// The A per-member write-set (validatorsSeen ADDs): build with the same pre-set the box derives.
	preSeen := idSet(f.preIDsValidatorsSeen())
	screens := map[ports.NodeID]StateRootAttScreen{}
	w.ParentProposer, w.ParentProposerSig = f.c.CarrierParentProposerWitness()
	parentProposer, _ := f.c.headProposerID()
	for i := range b.LastCommit {
		id := b.LastCommit[i].AttesterID()
		if id == parentProposer {
			continue
		}
		screens[id] = f.attScreen(id)
		w.AttScreens = append(w.AttScreens, f.attScreen(id))
	}
	aWrites, _, err := f.c.stateRootAttWriteSet(f.prevRoot, b, preSeen, screens, livePreForProbe(f.c), w.ParentProposer, w.ParentProposerSig)
	if err != nil {
		t.Fatalf("stateRootAttWriteSet: %v", err)
	}
	for _, wr := range aWrites {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	w.DigestPreSets = []StateRootDigestWitness{
		f.digestWitness(t, tagValidatorsSeenRoot, f.preIDsValidatorsSeen()),
	}
	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dp, err := f.prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		if err != nil {
			t.Fatalf("Prove(dueBucket): %v", err)
		}
		w.DueBucketProof = dp
	}
	// Class M: mature-from-genesis fixture ⇒ everMature already latched (pre=true), no crossing.
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
	return w
}

func (f attFixture) applyAndCommittedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// --- Ablation 0: the A recompute AGREES with real apply() over a qualified non-proposer att. ---
func TestRecomputeStateRootAttAgreesWithApply(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("A recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// --- Byte-exact: the reconstructed validatorsSeenRoot equals nodeSetMTH over the real post-set. ---
func TestRecomputeStateRootAttDigestByteExact(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	clone := f.c.cloneForDryRun()
	clone.apply(b)

	ops, _, err := f.c.attOps(f.prevRoot, b, f.witnessForAtt(t, b), livePreForProbe(f.c))
	if err != nil {
		t.Fatalf("attOps: %v", err)
	}
	want := nodeSetMTHFromBool(clone.validatorsSeen)
	found := false
	for _, op := range ops {
		if string(op.Key) == string(statehash.Key(tagValidatorsSeenRoot, nil)) {
			found = true
			if string(op.NewValue) != string(want) {
				t.Fatalf("validatorsSeenRoot not byte-exact: got %x want %x", op.NewValue, want)
			}
		}
	}
	if !found {
		t.Fatalf("no validatorsSeenRoot op emitted for a qualified att")
	}
}

// --- Ablation 1: forged qualification screen — claim the attester is NOT in the frozen epochSet.
// The box then does NOT write validatorsSeen ⇒ wrong validatorsSeenRoot ⇒ post-root != StateRoot ⇒
// stall. Drives the REAL recompute against the honest committed root. ---
func TestRecomputeStateRootAttAblationForgedScreen(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)

	// Forge the screen: claim the attester is NOT in the epochSet (so the box skips the write).
	aid := ports.HashBytes(pubOf(f.att))
	for i := range w.AttScreens {
		if w.AttScreens[i].Attester == aid {
			w.AttScreens[i].InEpochSet = false
		}
	}
	// R1.2: the forged InEpochSet=false requires a NON-MEMBERSHIP proof of epochSet||aid, but the honest
	// EpochSetProof proves PRESENT (the attester IS in the frozen set), so the class-A anchor stalls
	// (ErrRecomputeStateRootDigest) — a STRONGER, earlier catch than the pre-R1.2 fold mismatch.
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a forged (not-in-epochSet) screen must stall, got nil")
	}
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootMismatch) && !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: expected an anchor/mismatch/fold stall, got %v", err)
	}
}

// --- Ablation 2: legacy-mode block — the A screen falls to rep(id), not a committed leaf. The box
// asserts objective-mode and STALLS. We build a legacy (non-objective) chain and confirm the A
// dispatch stalls. ---
func TestRecomputeStateRootAttAblationLegacyMode(t *testing.T) {
	// A legacy chain: MinBond=0 (or no verifier) ⇒ objective() false. attOps must stall.
	cfg := Config{Quorum: 1, MinBond: 0, ByzantineQuorum: true, EpochBlocks: 1024, MatureValidators: 0}
	c := New(cfg, func(ports.NodeID) int64 { return 100 }) // rep-based
	// No bond verifier ⇒ objective() false.

	prop := key(54101)
	att := key(54102)
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(30)}}
	Sign(g, prop)
	c.apply(*g)
	if c.objective() {
		t.Fatalf("fixture: chain should be legacy (non-objective)")
	}

	prev, h := c.Head()
	head, _ := c.headBlock()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev}
	b.LastCommit = append(b.LastCommit, AttestAt(&head, att, 0, PhasePrecommit))
	pub, sig := c.CarrierParentProposerWitness()

	// The legacy assertion lives in stateRootAttWriteSet (reached by the A dispatch). It must stall
	// with a scope stall — the box refuses to reproduce rep(id) qualification from committed state.
	_, _, wsErr := c.stateRootAttWriteSet(ports.Hash{}, b, map[ports.NodeID]struct{}{}, map[ports.NodeID]StateRootAttScreen{}, livePreForProbe(c), pub, sig)
	if !errors.Is(wsErr, ErrRecomputeStateRootScopeStall) {
		t.Fatalf("ABLATION FAILED: a legacy-mode A screen must stall with a scope stall, got %v", wsErr)
	}
}

// --- Ablation 3: omitted validatorsSeenRoot digest witness — the box cannot reconstruct the touched
// digest ⇒ stall (never folds an unwitnessed digest change). ---
func TestRecomputeStateRootAttAblationOmittedDigest(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)

	// Drop the validatorsSeenRoot digest witness.
	w.DigestPreSets = nil

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootDigest) {
		t.Fatalf("ABLATION FAILED: an omitted validatorsSeenRoot witness must stall with ErrRecomputeStateRootDigest, got %v", err)
	}
}

// --- Ablation 4: a proposer-only att set writes NOTHING to validatorsSeen — the box must NOT emit a
// digest op (the set is unchanged), and the recompute must AGREE (E/R-only effect). ---
func TestRecomputeStateRootAttProposerOnlyNoWrite(t *testing.T) {
	f := buildAttFixture(t)
	prev, h := f.c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(42)}}
	b.Atts = append(b.Atts, Attest(&b, f.proposer)) // proposer's own att — skipped by apply()
	Sign(&b, f.proposer)
	committed := f.applyAndCommittedRoot(t, b)

	// No AttScreens needed (no non-proposer att). E/R-only witness.
	var w StateRootWitness
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		w.ChangedLeaves = append(w.ChangedLeaves, f.leafWitness(t, wr))
	}
	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		putUint64BE(hk[:], b.Height)
		dp, _ := f.prover.Prove(statehash.Key(tagDueBucket, hk[:]))
		w.DueBucketProof = dp
	}
	w.Maturity = latchedMaturityWitness(t, f.prover, f.preValue)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("a proposer-only att block is E/R-only and should AGREE, got %v", err)
	}
}
