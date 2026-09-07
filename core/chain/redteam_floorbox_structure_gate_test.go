package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// PERMANENT GATES for the FLOOR-BOX STRUCTURE round 1A (main-only; owner call 16).
//
// Governing documents:
//   - build brief: /Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md §7
//   - P-table delta: /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md
//   - composition:   /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md
//
// THE DEFECT SHAPE THESE GATES EXIST FOR. The box reproduced a node predicate's TAIL without the
// precondition a DIFFERENT validation stage had established (N1 the author screen, RT2-CARRIER-13
// the parent binding). A shared predicate SET cannot close that class; only a shared PATH can.
// These gates drive the ONE v5 accept path (validate_v5.go) through the node's own liveView.
//
// THE FIXTURE IS v5-DRIVEN, AND THAT IS ASSERTED FIRST (arm D / G-D1). BlockVersionRounds (2) is
// what the proposer mints today and Era4ActivationHeight has no cmd/silt flag, so a fixture that
// does not BUILD its v5 chain never enters the composition and every equivalence gate over it is
// vacuously green (the silt-gates-hold-the-seam scar). TestGD1_FixtureCommitsAWitnessableBlock is
// therefore the first gate in the file and every other gate here builds on the same fixture.

// structFixture is a node-accepted v5 world: four launch anchors, all bonded at genesis, objective
// mode, mature-from-genesis (MatureValidators: 0 so the maturity latch is true pre-state).
type structFixture struct {
	c    *Chain
	keys []ed25519.PrivateKey
}

func buildStructFixture(t *testing.T) structFixture {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(77000 + i))
		anchors[idOf(keys[i])] = true
	}
	c := New(Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 0,
	}, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	// One committed v5 block, so the block under test sits at height 2 (a carrier is illegal at
	// height <= 1) and the fixture's pre-state is a real post-apply state.
	f := structFixture{c: c, keys: keys}
	b1 := f.mkBlock(t, func(b *Block) { b.Entries = []ports.Entry{entry(1)} })
	if err := c.Append(b1); err != nil {
		t.Fatalf("fixture h1 must COMMIT (it is the node oracle's own path): %v", err)
	}
	return f
}

// mkBlock builds a fully certified v5 block on the fixture's head: proposer signature, committed
// roots, prepare QC and precommit certificate — a block the NODE accepts. mutate shapes the
// payload before the roots are computed.
func (f structFixture) mkBlock(t *testing.T, mutate func(*Block)) Block {
	t.Helper()
	prev, h := f.c.Head()
	b := &Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h + 40))}}
	if mutate != nil {
		mutate(b)
	}
	state, log, err := f.c.postApplyRoots(*b)
	if err != nil {
		t.Fatalf("postApplyRoots: %v", err)
	}
	b.StateRoot, b.LogRoot = &state, &log
	Sign(b, f.keys[0])
	for _, k := range f.keys {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare))
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit))
	}
	return *b
}

// =============================================================================
// G-D1 / arm D — the fixture reaches the v5 path (NON-VACUITY, runs first)
// =============================================================================

// TestGD1_FixtureCommitsAWitnessableBlock. Ablation: force b.Version = BlockVersionRounds in
// mkBlock; this gate must FAIL. Without it, a v2 fixture makes every gate in this file and in
// composition_stage_cover_v5_test.go vacuously green.
func TestGD1_FixtureCommitsAWitnessableBlock(t *testing.T) {
	f := buildStructFixture(t)
	head := f.c.Blocks(1)
	if len(head) == 0 {
		t.Fatal("fixture committed no block above genesis")
	}
	if got := head[len(head)-1].Version; got != BlockVersionWitnessable {
		t.Fatalf("FIXTURE VACUOUS: the committed block is v%d, want v%d (BlockVersionWitnessable). "+
			"A sub-v5 fixture never enters ValidateProposalV5/ValidateCommitV5, so every equivalence "+
			"gate over it passes for the wrong reason.", got, BlockVersionWitnessable)
	}
	b := f.mkBlock(t, nil)
	if b.Version != BlockVersionWitnessable {
		t.Fatalf("FIXTURE VACUOUS: mkBlock mints v%d, want v%d", b.Version, BlockVersionWitnessable)
	}
	if err := f.c.ValidateCommit(&b); err != nil {
		t.Fatalf("ORACLE BROKEN: the node must accept its own certified v5 block; got %v", err)
	}
}
