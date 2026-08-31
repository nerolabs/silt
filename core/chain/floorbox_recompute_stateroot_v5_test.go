package chain

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the O(payload) HYBRID P1-a state-root recompute (floorbox_recompute_stateroot_v5.go).
//
// The recompute is CERTIFIED (2026-08-31) as the sound O(payload) spine for classes E + R: derive
// the write-set from the payload, witness each changed leaf against prevStateRoot, fold the
// changed paths, require the computed root == b.StateRoot. It NEVER Accepts (STOP boundary); it
// reproduces validateEra3Roots' StateRoot equality root-only and stalls otherwise.
//
// R3 (the execution-derived drift guard, MANDATORY): the box's derived write-set + fold is checked
// against the REAL apply() + StateRootForVersion(5) — the oracle a full node uses — and ablated RED
// on an omitted / injected / mis-valued write. A hand-written mirror shares the producer's blind
// spot (the session-7 scar), so the ground truth is real execution.

// stateRootFixture is a v5 chain at some committed pre-state, plus prevStateRoot and a Prover over
// its v5 leaf set (the any-of-N provider that holds the committed set). The box holds prevStateRoot.
type stateRootFixture struct {
	c        *Chain
	prevRoot ports.Hash
	prover   *statehash.Prover
}

// buildStateRootFixture builds an objective v5 chain and applies some entries + a revocation so the
// pre-state has byRoot / spent / revoked leaves for the E/R fold to change. TTL is enabled so the
// dueBucket scope-gate path is exercised. epochsEnabled requires an epoch config; we keep
// EpochBlocks large so no boundary fires on the test heights.
func buildStateRootFixture(t *testing.T) stateRootFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	prop := key(51001)
	// Genesis seats a bond so the chain is objective, and lands a couple of entries + a revocable
	// root, so the pre-state has real E/R leaves.
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(10), entry(11)}}
	g.BondRegs = append(g.BondRegs, bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	c.apply(*g)

	// A second block adds an entry with a token (spent leaf) and revokes entry(10)'s root.
	tok := &ports.PublishToken{Serial: []byte("serial-xyz")}
	e := entry(12)
	e.Token = tok
	prev, h := c.Head()
	b1 := &Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		Entries:     []ports.Entry{e},
		Revocations: []ports.Hash{entry(10).Root},
	}
	Sign(b1, prop)
	c.apply(*b1)

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
	return stateRootFixture{c: c, prevRoot: prevRoot, prover: prover}
}

// witnessForBlock builds the O(payload) witness bundle for block b against the fixture's pre-state,
// using the pre-state Prover as the any-of-N provider: one changed-leaf proof per derived write
// (plus delete siblings), and the dueBucket non-membership proof for the TTL scope gate.
func (f stateRootFixture) witnessForBlock(t *testing.T, b Block) StateRootWitness {
	t.Helper()
	writeSet := applyEntriesRevocationsWriteSet(b)
	var w StateRootWitness
	for _, wr := range writeSet {
		if wr.newValue == nil {
			// delete: membership proof + off-path sibling preimages
			wit, sibs, err := f.prover.ProveWithSiblings(wr.key)
			if err != nil {
				t.Fatalf("ProveWithSiblings(%x): %v", wr.key, err)
			}
			w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
				Key:            wr.key,
				OldValue:       statehash.Present, // a revoked leaf being un-revoked is present pre-apply
				Proof:          wit,
				DeleteSiblings: sibs,
			})
			continue
		}
		// add / overwrite: prove the key; the pre-state value is Present if already committed, else
		// absent (non-membership). We probe the pre-state via the prover to set OldValue correctly.
		wit, err := f.prover.Prove(wr.key)
		if err != nil {
			t.Fatalf("Prove(%x): %v", wr.key, err)
		}
		old := f.preValue(wr.key)
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
			Key:      wr.key,
			OldValue: old,
			Proof:    wit,
		})
	}
	// dueBucket TTL scope-gate proof (non-membership at b.Height).
	if f.c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		binary.BigEndian.PutUint64(hk[:], b.Height)
		dk := statehash.Key(tagDueBucket, hk[:])
		dp, err := f.prover.Prove(dk)
		if err != nil {
			t.Fatalf("Prove(dueBucket): %v", err)
		}
		w.DueBucketProof = dp
	}
	return w
}

// preValue returns the committed pre-state value of a leaf key, by consulting the fixture chain's
// v5 leaf set. Present-marker for a committed set key, nil if absent.
func (f stateRootFixture) preValue(key []byte) []byte {
	for _, lf := range f.c.stateRootLeavesV5() {
		if string(lf.Key) == string(key) {
			return lf.Value
		}
	}
	return nil
}

// applyAndCommittedRoot applies b to a CLONE of the fixture chain (real apply()) and returns the
// committed v5 StateRoot the full node computes — the R3 oracle.
func (f stateRootFixture) applyAndCommittedRoot(t *testing.T, b Block) ports.Hash {
	t.Helper()
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	sr, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("clone StateRootForVersion: %v", err)
	}
	return sr
}

// nextERBlock builds an E/R-only block at the fixture head: an add, a token entry (spent), a
// revocation of a currently-unrevoked root, and an un-revocation of the fixture's revoked root.
func (f stateRootFixture) nextERBlock() Block {
	prev, h := f.c.Head()
	tok := &ports.PublishToken{Serial: []byte("serial-new")}
	e := entry(20)
	e.Token = tok
	return Block{
		Version:       BlockVersionWitnessable,
		Height:        h,
		Prev:          prev,
		Entries:       []ports.Entry{entry(21), e},
		Revocations:   []ports.Hash{entry(11).Root}, // revoke a currently-unrevoked committed root
		Unrevocations: []ports.Hash{entry(10).Root}, // un-revoke the fixture's revoked root (a delete)
	}
}

// TestRecomputeStateRootAgreesWithApply is the R3 ground-truth check: the O(payload) recompute
// AGREES (returns nil) with the committed StateRoot the REAL apply() + StateRootForVersion(5)
// produces, over an E/R block that exercises add / token-spent / revoke / un-revoke (a delete).
func TestRecomputeStateRootAgreesWithApply(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBlock(t, b)

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("recompute should AGREE with real apply() but stalled: %v", err)
	}
}

// TestRecomputeStateRootAblationTamperedRoot: a tampered committed StateRoot ⇒ mismatch stall.
func TestRecomputeStateRootAblationTamperedRoot(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()
	committed := f.applyAndCommittedRoot(t, b)
	committed[0] ^= 0xff // TAMPER
	w := f.witnessForBlock(t, b)

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: a tampered committed StateRoot must stall with mismatch, got %v", err)
	}
}

// TestRecomputeStateRootAblationOmittedWrite: the committed root reflects a write the box's derived
// set does NOT produce (an extra byRoot leaf) ⇒ the fold's recomputed root diverges ⇒ mismatch.
// This proves the payload-derivation + fold catches a write the block does not justify.
func TestRecomputeStateRootAblationOmittedWrite(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()

	// The HONEST committed root includes only b's E/R writes. Build a FORGED committed root that
	// ALSO adds an unrelated byRoot leaf the block does not carry — the classic "un-named X" the
	// cert names. The box's fold (over the payload-derived set) computes the honest root, which
	// differs from this forged one ⇒ mismatch.
	clone := f.c.cloneForDryRun()
	clone.apply(b)
	clone.byRoot[entry(99).Root] = entry(99) // inject an extra committed leaf NOT in b's payload
	forged, err := clone.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatal(err)
	}
	w := f.witnessForBlock(t, b)

	rerr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forged, b, w)
	if !errors.Is(rerr, ErrRecomputeStateRootMismatch) {
		t.Fatalf("ABLATION FAILED: an un-named extra committed write must stall with mismatch, got %v", rerr)
	}
}

// TestRecomputeStateRootAblationForgedProof: a changed-leaf proof for the WRONG pre-value ⇒ the
// fold's VerifyProof fails ⇒ stall (never folds an unverified change).
func TestRecomputeStateRootAblationForgedProof(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBlock(t, b)

	// Corrupt one changed-leaf's claimed OldValue to a wrong non-empty value: VerifyProof will fail.
	found := false
	for i := range w.ChangedLeaves {
		if len(w.ChangedLeaves[i].OldValue) == 0 {
			w.ChangedLeaves[i].OldValue = []byte{0x42} // claim present where it is absent
			found = true
			break
		}
	}
	if !found {
		// fall back: corrupt any proof
		w.ChangedLeaves[0].Proof = statehash.NewWitness(nil)
	}

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: a forged changed-leaf proof must stall in the fold, got %v", err)
	}
}

// TestRecomputeStateRootAblationOmittedProof: a missing witness for a derived changed key ⇒ stall.
func TestRecomputeStateRootAblationOmittedProof(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBlock(t, b)

	// Drop the last changed-leaf witness.
	w.ChangedLeaves = w.ChangedLeaves[:len(w.ChangedLeaves)-1]

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: a missing witness for a derived changed key must stall, got %v", err)
	}
}

// TestRecomputeStateRootAttIncompleteWitnessStalls: class A (non-proposer att) is now IN scope
// (P1-e), so a class-A block DISPATCHES. With an E/R-only witness (no AttScreens, no validatorsSeenRoot
// digest) the box cannot reconstruct the A delta ⇒ it stalls (never-Accept preserved). The stall moves
// from the scope gate to the A dispatch, but it is still a stall — never a wrong-Accept.
func TestRecomputeStateRootAttIncompleteWitnessStalls(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()
	// Inject a non-proposer attestation (class A) but supply only the E/R witness (no A witness).
	bb := b
	b.Atts = append(b.Atts, Attest(&bb, key(52002)))
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBlock(t, b)

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatalf("ABLATION FAILED: a class-A block with an incomplete (E/R-only) witness must stall, got nil")
	}
	// The A dispatch stalls on the missing validatorsSeenRoot digest / attester screen (never-Accept).
	if !errors.Is(err, ErrRecomputeStateRootDigest) && !errors.Is(err, ErrRecomputeStateRootFold) {
		t.Fatalf("ABLATION FAILED: expected a digest/fold stall for an unwitnessed class-A block, got %v", err)
	}
}

// TestRecomputeStateRootTTLScopeGateStalls: when a TTL expiry FIRES at b.Height (dueBucket[h]
// occupied), the dueBucket witness cannot prove non-membership ⇒ the scope gate stalls. This is the
// O(payload) re-anchor — no whole-map scan, an O(1) dueBucket proof.
func TestRecomputeStateRootTTLScopeGateStalls(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.nextERBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForBlock(t, b)

	// Ablate: hand the box a MEMBERSHIP dueBucket proof (a proof for an OCCUPIED bucket). We prove a
	// dueBucket key that IS occupied in the pre-state. The fixture's genesis bond registered at
	// height 0 with TTL 64 → dueBucket[0+64+1] = dueBucket[65] is occupied. Prove THAT key (a
	// membership proof) and hand it as the b.Height witness — the box requires ProvenAbsent, so a
	// membership proof stalls.
	var hk [8]byte
	binary.BigEndian.PutUint64(hk[:], 65)
	occupiedKey := statehash.Key(tagDueBucket, hk[:])
	mp, err := f.prover.Prove(occupiedKey)
	if err != nil {
		t.Fatalf("Prove(occupied dueBucket): %v", err)
	}
	w.DueBucketProof = mp

	rerr := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if !errors.Is(rerr, ErrRecomputeStateRootTTLWitness) {
		t.Fatalf("ABLATION FAILED: a non-absent dueBucket witness must stall the TTL scope gate, got %v", rerr)
	}
}
