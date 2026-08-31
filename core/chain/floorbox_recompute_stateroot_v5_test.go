package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// Tests for the Path-1 state-root recompute sub-increment P1-a
// (floorbox_recompute_stateroot_v5.go): the root-only reproduction of validateEra3Roots' StateRoot
// equality check (era3validity.go:128) for the pure set-write classes E (entries) + R
// (revocations/un-revocations).
//
// The MECHANISM under test (reconstruct pre-state → apply payload → hash → compare):
//   - HONEST path: a complete pre-state witness + an E+R block whose committed StateRoot equals the
//     full node's post-apply root ⇒ the recompute returns nil (the full node's verdict).
//
// The HARD ABLATIONS (C-5, red-before-green — each injected and watched to flip nil→stall, so a
// green is not decoration):
//   - OMITTED / INJECTED / TAMPERED PRE-LEAF: the reconstructed pre-state root ≠ prevStateRoot ⇒
//     STALL. Proves the pre-state completeness anchor.
//   - TAMPERED COMMITTED StateRoot (or an omitted/injected payload write): the recomputed post-state
//     root ≠ the committed StateRoot ⇒ STALL. This is the forged-leaf / diverged-root ablation.
//   - OUT-OF-SCOPE class (bond reg / slash / seen-writing att / TTL expiry / boundary): STALL, the
//     box never-Accepts a class P1-a does not reproduce.
//
// The recompute NEVER flips WitnessValidateV5 to Accept (the STOP boundary): it reproduces the
// root-equality mechanism on E + R only.

// stateRootFixture is a v5 chain at a known committed pre-state, plus the committed StateRoot of
// that pre-state (prevStateRoot) and the complete pre-state leaf set the witness carries.
type stateRootFixture struct {
	c        *Chain
	prevRoot ports.Hash          // the pre-state's committed v5 StateRoot
	preWit   StateRootPreWitness // the complete pre-state leaf set
}

// buildStateRootFixture builds a v5 objective chain with a populated committed state (bonds,
// entries) and returns it at a NON-boundary, NON-expiry pre-state. EpochBlocks is large and
// BondTTLBlocks is large so the E+R test blocks that follow are neither boundaries nor expiries —
// the state effect under test is exactly the entries/revocations.
func buildStateRootFixture(t *testing.T) stateRootFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 1 << 20}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis (h0) seats three bonds and one entry. h0 is a boundary but we snapshot the pre-state
	// AFTER a later non-boundary block so the fixture pre-state is itself non-boundary.
	prop := key(94001)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(1)}}
	for i := 0; i < 3; i++ {
		k := key(int64(94001 + i))
		g.BondRegs = append(g.BondRegs, bondRegFull(k, ports.HashBytes(pubOf(k)), 8<<20, ports.Hash{}, 5, uint64(i+1)))
	}
	Sign(g, prop)
	c.apply(*g)

	// A second non-boundary block (h1) adds an entry + a revocation, so the pre-state carries a
	// revoked leaf (giving the un-revocation ablation something to remove) and a spent leaf.
	e1 := entry(1)
	rootToRevoke := e1.Root // the genesis entry, revocable (it is committed)
	b1 := Block{Version: BlockVersionWitnessable, Height: 1,
		Entries:     []ports.Entry{tokenEntry(2, &ports.PublishToken{Serial: []byte{2, 2, 2}})},
		Revocations: []ports.Hash{rootToRevoke}}
	c.apply(b1)

	prevRoot, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion(pre-state): %v", err)
	}
	return stateRootFixture{
		c:        c,
		prevRoot: prevRoot,
		preWit:   StateRootPreWitness{PreLeaves: c.stateRootLeavesV5()},
	}
}

// spentEntry builds an entry carrying a publish token (so apply() writes a spent leaf for it),
// with a deterministic per-byte serial.
func spentEntry(b byte) ports.Entry {
	return tokenEntry(b, &ports.PublishToken{Serial: []byte{b, b, b}})
}

// erBlock builds a v5 E+R block at height h that is neither a boundary nor an expiry under the
// fixture config, and returns it with the TRUE post-apply committed StateRoot filled in (the root a
// full node would commit), so the honest recompute has a correct committed root to match.
func (f stateRootFixture) erBlock(t *testing.T, h uint64, e ports.Entry, revs, unrevs []ports.Hash) Block {
	t.Helper()
	b := Block{Version: BlockVersionWitnessable, Height: h,
		Entries: []ports.Entry{e}, Revocations: revs, Unrevocations: unrevs}
	// The true post root: cloneForDryRun + apply + StateRootForVersion, exactly postApplyRoots.
	scratch := f.c.cloneForDryRun()
	scratch.apply(b)
	sr, err := scratch.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("post-apply StateRootForVersion: %v", err)
	}
	b.StateRoot = &sr
	return b
}

// TestStateRootRecomputeEntriesRevocationsHonest is the green path: a complete pre-state witness +
// an E+R block whose committed StateRoot equals the full node's post-apply root ⇒ the recompute
// returns nil, the same verdict validateEra3Roots reaches.
func TestStateRootRecomputeEntriesRevocationsHonest(t *testing.T) {
	f := buildStateRootFixture(t)
	// An E+R block: one new token entry, one revocation of a fresh committed root, one un-revocation
	// of the pre-state's existing revoked root (entry(1).Root, revoked at h1 in the fixture).
	newRev := entry(9).Root
	unrev := entry(1).Root
	b := f.erBlock(t, 2, spentEntry(3), []ports.Hash{newRev}, []ports.Hash{unrev})

	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, *b.StateRoot, b, f.preWit); err != nil {
		t.Fatalf("honest recompute must accept the correct committed root, got stall: %v", err)
	}
}

// TestStateRootRecomputeRejectsOmittedPreLeaf ablates the pre-state completeness anchor: dropping a
// pre-state leaf makes the reconstructed pre-state root ≠ prevStateRoot ⇒ STALL.
func TestStateRootRecomputeRejectsOmittedPreLeaf(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.erBlock(t, 2, spentEntry(3), nil, nil)

	// Sanity: the complete witness accepts.
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, *b.StateRoot, b, f.preWit); err != nil {
		t.Fatalf("precondition: complete witness must accept, got: %v", err)
	}

	// Drop one leaf.
	short := StateRootPreWitness{PreLeaves: f.preWit.PreLeaves[1:]}
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, *b.StateRoot, b, short)
	if !errors.Is(err, ErrRecomputeStateRootPreStateIncomplete) {
		t.Fatalf("omitted pre-leaf must stall with ErrRecomputeStateRootPreStateIncomplete, got: %v", err)
	}
}

// TestStateRootRecomputeRejectsTamperedPreLeaf ablates the anchor from the value side: tampering a
// pre-state leaf's value (a forged committed weight) makes the reconstructed root ≠ prevStateRoot.
func TestStateRootRecomputeRejectsTamperedPreLeaf(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.erBlock(t, 2, spentEntry(3), nil, nil)

	// Copy the leaves and forge one value.
	tampered := make([]statehash.Leaf, len(f.preWit.PreLeaves))
	copy(tampered, f.preWit.PreLeaves)
	tampered[0] = statehash.Leaf{Key: tampered[0].Key, Value: append([]byte(nil), tampered[0].Value...)}
	tampered[0].Value[0] ^= 0xFF // flip a byte of the committed value

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, *b.StateRoot, b,
		StateRootPreWitness{PreLeaves: tampered})
	if !errors.Is(err, ErrRecomputeStateRootPreStateIncomplete) {
		t.Fatalf("tampered pre-leaf must stall with ErrRecomputeStateRootPreStateIncomplete, got: %v", err)
	}
}

// TestStateRootRecomputeRejectsInjectedPreLeaf ablates the anchor from the injection side: adding a
// leaf the pre-state does not contain makes the reconstructed root ≠ prevStateRoot.
func TestStateRootRecomputeRejectsInjectedPreLeaf(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.erBlock(t, 2, spentEntry(3), nil, nil)

	injected := append([]statehash.Leaf(nil), f.preWit.PreLeaves...)
	extra := entry(42).Root
	injected = append(injected, statehash.Leaf{
		Key:   statehash.Key(tagByRoot, extra[:]),
		Value: statehash.Present,
	})
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, *b.StateRoot, b,
		StateRootPreWitness{PreLeaves: injected})
	if !errors.Is(err, ErrRecomputeStateRootPreStateIncomplete) {
		t.Fatalf("injected pre-leaf must stall, got: %v", err)
	}
}

// TestStateRootRecomputeRejectsTamperedCommittedRoot ablates the terminal check: a committed
// StateRoot that does not equal the true post-apply root ⇒ STALL. This is the forged-committed-leaf
// / diverged-root ablation reproduced at the root level (a wrong committed root is exactly what a
// forged post-state leaf would produce).
func TestStateRootRecomputeRejectsTamperedCommittedRoot(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.erBlock(t, 2, spentEntry(3), []ports.Hash{entry(9).Root}, nil)

	forged := *b.StateRoot
	forged[0] ^= 0xFF
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forged, b, f.preWit)
	if !errors.Is(err, ErrRecomputeStateRootMismatch) {
		t.Fatalf("tampered committed root must stall with ErrRecomputeStateRootMismatch, got: %v", err)
	}
}

// TestStateRootRecomputeRejectsOmittedPayloadWrite ablates the post-state derivation: if the block's
// committed StateRoot was computed WITH an entry the recompute is not told about (the recompute
// applies fewer writes than the committed root reflects), the recomputed post-state root diverges ⇒
// STALL. Concretely: commit a root over TWO entries, hand the recompute a block with only ONE.
func TestStateRootRecomputeRejectsOmittedPayloadWrite(t *testing.T) {
	f := buildStateRootFixture(t)

	// The committed root reflects a block with two entries...
	full := Block{Version: BlockVersionWitnessable, Height: 2,
		Entries: []ports.Entry{spentEntry(3), entry(7)}}
	scratch := f.c.cloneForDryRun()
	scratch.apply(full)
	sr, err := scratch.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("post-apply root: %v", err)
	}

	// ...but the recompute is handed a block with only ONE entry.
	partial := Block{Version: BlockVersionWitnessable, Height: 2,
		Entries: []ports.Entry{spentEntry(3)}, StateRoot: &sr}
	e := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, sr, partial, f.preWit)
	if !errors.Is(e, ErrRecomputeStateRootMismatch) {
		t.Fatalf("a committed root reflecting an unwitnessed write must stall with mismatch, got: %v", e)
	}
}

// TestStateRootRecomputeScopeGateStalls ablates the never-Accept scope gate: each out-of-scope class
// must stall with ErrRecomputeStateRootOutOfScope, so a later sub-increment's absence can never be a
// silent wrong-Accept.
func TestStateRootRecomputeScopeGateStalls(t *testing.T) {
	f := buildStateRootFixture(t)

	reg := bondRegFull(key(95001), ports.HashBytes(pubOf(key(95001))), 8<<20, ports.Hash{}, 5, 9)
	cases := []struct {
		name string
		b    Block
	}{
		{"bond-reg", Block{Version: BlockVersionWitnessable, Height: 2, BondRegs: []BondReg{reg}}},
		{"slash", Block{Version: BlockVersionWitnessable, Height: 2, Slashes: []Equivocation{{}}}},
		{"non-proposer-att", Block{Version: BlockVersionWitnessable, Height: 2,
			Proposer: pubOf(key(94001)),
			Atts:     []Attestation{{PubKey: pubOf(key(94002))}}}},
		{"boundary", Block{Version: BlockVersionWitnessable, Height: 1024}}, // EpochBlocks=1024
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The committed root is irrelevant — the scope gate fires before the post-state compare.
			err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, ports.Hash{}, tc.b, f.preWit)
			if !errors.Is(err, ErrRecomputeStateRootOutOfScope) {
				t.Fatalf("%s must stall with ErrRecomputeStateRootOutOfScope, got: %v", tc.name, err)
			}
		})
	}
}

// TestStateRootRecomputeScopeGateStallsOnTTLExpiry ablates the TTL-expiry clause specifically: a
// block at a height where a bonded id's TTL fires is out of scope (Class T, P1-c) and must stall.
func TestStateRootRecomputeScopeGateStallsOnTTLExpiry(t *testing.T) {
	// A small TTL so an expiry is reachable at a modest, non-boundary height.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 1024, MatureValidators: 0, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(96001)
	g := &Block{Version: 1, Height: 0}
	k := key(96001)
	g.BondRegs = []BondReg{bondRegFull(k, ports.HashBytes(pubOf(k)), 8<<20, ports.Hash{}, 5, 1)}
	Sign(g, prop)
	c.apply(*g)

	prevRoot, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("pre-state root: %v", err)
	}
	pre := StateRootPreWitness{PreLeaves: c.stateRootLeavesV5()}

	// At height 6, height - regHeight(0) = 6 > ttl(4) ⇒ the genesis bond expires. Non-boundary
	// (6 % 1024 != 0). The scope gate must stall.
	b := Block{Version: BlockVersionWitnessable, Height: 6, Entries: []ports.Entry{entry(3)}}
	e := c.RecomputeStateRootEntriesRevocations(prevRoot, ports.Hash{}, b, pre)
	if !errors.Is(e, ErrRecomputeStateRootOutOfScope) {
		t.Fatalf("a TTL expiry at this height must stall out-of-scope, got: %v", e)
	}
}

// TestStateRootRecomputeRejectsDuplicatePreLeaf ablates the malformed-witness guard: a duplicate
// pre-state key is not valid state ⇒ STALL.
func TestStateRootRecomputeRejectsDuplicatePreLeaf(t *testing.T) {
	f := buildStateRootFixture(t)
	b := f.erBlock(t, 2, spentEntry(3), nil, nil)

	dup := append([]statehash.Leaf(nil), f.preWit.PreLeaves...)
	dup = append(dup, f.preWit.PreLeaves[0]) // repeat the first key
	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, *b.StateRoot, b,
		StateRootPreWitness{PreLeaves: dup})
	if !errors.Is(err, ErrRecomputeStateRootDuplicatePreLeaf) {
		t.Fatalf("duplicate pre-leaf must stall, got: %v", err)
	}
}
