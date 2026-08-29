package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// era-4 build step 4d — height-gated activation + mint-flip to v5.
//
// The activation shape is the era-3 (2c) machinery reused one readiness level up: a
// frozen-epoch-weight supermajority signalling regVersion >= BlockVersionWitnessable
// (== 5) locks in era-4 at the NEXT finalized boundary (H_era4), from which v5 is REQUIRED
// (mint + validity). era-4 layers ON TOP of era-3: a v5 block commits a SUPERSET of the v4
// leaves, so H_era4 >= H_era3 (enforced in New for the pre-latch overrides; automatic
// post-latch because a v5 signaller is v4-ready). These oracles assert the four properties
// 4d must hold, EACH paired with a demonstrated RED (an injected defect that flips the
// assertion) recorded inline, per the session-7 rule: a green check is not shipped until
// its defect has been watched go red. Invariants: I1/I2/I4 untouched; I3 relied on (rule
// change integrates only at a finalized boundary, by weight); I5 preserved
// (era4Active/MintVersion/validateEra4Version are pure functions of committed state).
// Deliberation: docs/thinking/2026-08-29-era4-4d-activation-mintflip-approach.md.

// mintNext4 builds the next block the way an era-4 proposer WOULD: it asks the chain for
// the mint version, and for a v5 boundary block populates the committed roots over the
// block's own post-apply state (the same PopulateEra4Roots the propose path calls); for a
// v4 boundary block it populates the era-3 roots; below both it stamps v2. It signs a full
// two-phase certificate so ValidateCommit accepts it. This is the chain-tier analogue of
// core/node's proposeBlock mint-flip (chainrole.go).
func mintNext4(t *testing.T, c *Chain, keys []ed25519.PrivateKey, regs ...BondReg) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}, BondRegs: regs}
	switch mv := c.MintVersion(next); {
	case mv >= BlockVersionWitnessable:
		if err := c.PopulateEra4Roots(b); err != nil {
			t.Fatalf("populate era-4 roots at height %d: %v", next, err)
		}
	case mv >= BlockVersionStateRoot:
		if err := c.PopulateEra3Roots(b); err != nil {
			t.Fatalf("populate era-3 roots at height %d: %v", next, err)
		}
	default:
		b.Version = BlockVersionRounds
	}
	twoPhaseSign(b, keys)
	return b
}

// era4AnchorChain builds an anchor-launch chain (4 anchors, keys[0] the proposer) with
// genesis-declared era-3 AND era-4 activation boundaries. Anchors let a two-phase
// v2/v4/v5 block commit at the launch phase without the mature-epoch machinery, so the
// mint-flip and boundary-validity rules are exercised in isolation. era4 MUST be >= era3
// (the layering constraint New enforces).
func era4AnchorChain(t *testing.T, era3Activation, era4Activation uint64) (*Chain, []ed25519.PrivateKey) {
	t.Helper()
	keys := []ed25519.PrivateKey{key(53401), key(53402), key(53403), key(53404)}
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, BondTTLBlocks: 40,
		Era3ActivationHeight: era3Activation, Era4ActivationHeight: era4Activation}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for _, k := range keys {
		g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, keys
}

// TestEra4PreLatchMintFlipAndBoundary4d: with genesis-declared boundaries (H_era3=2,
// H_era4=4), the four regimes are exercised:
//   - below H_era3: mint v2, accept v2;
//   - at/above H_era3 and below H_era4: mint v4, accept a correctly-rooted v4 block;
//   - at/above H_era4: mint v5, accept a correctly-rooted v5 block, REJECT a v4 block
//     (ErrEra4VersionRequired), REJECT a v5 block with a wrong StateRoot (the 4c predicate).
//
// GATE (PACE): "Before activation: v4, era-3 freeze holds" AND "At/after activation: v5,
// first v5 block accepted with the new keyspaces committed and RegCap-valid".
func TestEra4PreLatchMintFlipAndBoundary4d(t *testing.T) {
	c, keys := era4AnchorChain(t, 2, 4)

	// --- Below H_era3 (height 1): mint v2. ---
	if v := c.MintVersion(1); v != BlockVersionRounds {
		t.Fatalf("below H_era3: MintVersion must be v2 (%d), got %d", BlockVersionRounds, v)
	}
	mustAppend(t, c, mintNext4(t, c, keys)) // height 1, v2

	// --- At/above H_era3, below H_era4 (height 2,3): mint v4. ---
	if v := c.MintVersion(2); v != BlockVersionStateRoot {
		t.Fatalf("at H_era3 below H_era4: MintVersion must be v4 (%d), got %d", BlockVersionStateRoot, v)
	}
	if c.era4Active(2) || c.era4Active(3) {
		t.Fatal("below H_era4: era4Active must be false")
	}
	mustAppend(t, c, mintNext4(t, c, keys)) // height 2, v4
	mustAppend(t, c, mintNext4(t, c, keys)) // height 3, v4
	if c.Blocks(2)[0].Version != BlockVersionStateRoot {
		t.Fatalf("below H_era4: minted block must be v4, got v%d", c.Blocks(2)[0].Version)
	}
	// RED shown: if MintVersion returned v5 below H_era4 (era4Active fired early), height
	// 2's v4 block would be rejected by the era-4 boundary rule and mustAppend would fail.

	// --- At H_era4 (height 4): mint v5, accept correct v5. ---
	if v := c.MintVersion(4); v != BlockVersionWitnessable {
		t.Fatalf("at H_era4: MintVersion must be v5 (%d), got %d", BlockVersionWitnessable, v)
	}
	if !c.era4Active(4) {
		t.Fatal("at H_era4: era4Active must be true")
	}
	good := mintNext4(t, c, keys) // height 4, v5 with correct roots
	if good.Version != BlockVersionWitnessable {
		t.Fatalf("at H_era4: mintNext4 must produce v5, got v%d", good.Version)
	}

	// A v4 block at H_era4 is REJECTED (the era-4 version-boundary rule). Build one by hand
	// (bypassing mintNext4's flip) at the current head, with correct v4 roots so ONLY the
	// version rule can reject it.
	prev, next := c.Head()
	v4AtBoundary := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if err := c.PopulateEra3Roots(v4AtBoundary); err != nil { // stamps v4 + correct v4 roots
		t.Fatalf("populate v4 roots: %v", err)
	}
	twoPhaseSign(v4AtBoundary, keys)
	if err := c.Append(*v4AtBoundary); !errors.Is(err, ErrEra4VersionRequired) {
		t.Fatalf("at H_era4: a v4 block must be rejected with ErrEra4VersionRequired, got %v", err)
	}
	// RED shown: delete the validateEra4Version call in ValidateProposal and this v4 block
	// Appends cleanly — the rejection is what enforces the v5 mint-flip at the boundary.

	// A v5 block with a WRONG StateRoot is REJECTED (the 4c predicate, via validateEra3Roots
	// recomputing over StateRootForVersion(5)).
	prev, next = c.Head()
	wrong := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if err := c.PopulateEra4Roots(wrong); err != nil {
		t.Fatalf("populate v5 roots: %v", err)
	}
	bad := *wrong.StateRoot
	bad[0] ^= 0xFF // corrupt the committed v5 state root
	wrong.StateRoot = &bad
	twoPhaseSign(wrong, keys)
	if err := c.Append(*wrong); !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("at H_era4: a v5 block with a wrong StateRoot must be rejected (4c), got %v", err)
	}

	// The correct v5 block commits, and it carries the era-4 committed roots.
	mustAppend(t, c, good)
	committed := c.Blocks(4)[0]
	if committed.Version != BlockVersionWitnessable {
		t.Fatalf("at H_era4: committed block must be v5, got v%d", committed.Version)
	}
	if committed.StateRoot == nil || committed.LogRoot == nil {
		t.Fatal("at H_era4: the committed v5 block must carry StateRoot AND LogRoot")
	}
	// era-4 continues: the next block also mints v5 and commits.
	mustAppend(t, c, mintNext4(t, c, keys)) // height 5, v5
	if c.Blocks(5)[0].Version != BlockVersionWitnessable {
		t.Fatalf("past H_era4: minting must stay v5, got v%d", c.Blocks(5)[0].Version)
	}
}

// TestEra4FirstV5BlockCommitsTheSpineKeyspaces4d: the FIRST v5 block's committed StateRoot
// is the v5 root (era-3 leaves PLUS the maintenance-spine keyspaces plus the era-4 activation
// scalars), NOT the era-3 root, and a validator recompute over the v5 leaf set ACCEPTS it.
// Below the boundary the committed root is byte-identical to era-3 (the freeze holds).
//
// GATE (PACE): "Before activation the v4 committed root stays byte-identical to era-3;
// at/after activation the first v5 block correctly commits the new keyspaces."
func TestEra4FirstV5BlockCommitsTheSpineKeyspaces4d(t *testing.T) {
	c, keys := era4AnchorChain(t, 2, 4)

	// Drive to the last v4 height (3) so the chain carries real bonded/qualified/dueBucket
	// state at the boundary. The v4 block's committed root must equal the era-3 root
	// (StateRoot(), the 18-leaf set) — the freeze.
	mustAppend(t, c, mintNext4(t, c, keys)) // 1, v2
	mustAppend(t, c, mintNext4(t, c, keys)) // 2, v4
	v4 := c.Blocks(2)[0]
	era3Root, err := c.chainRootAtBlock(t, v4)
	if err != nil {
		t.Fatalf("recompute v4 root: %v", err)
	}
	if v4.StateRoot == nil || *v4.StateRoot != era3Root {
		t.Fatalf("the v4 block's committed root must be the era-3 (v4) root — the freeze; got %v want %x",
			v4.StateRoot, era3Root)
	}

	mustAppend(t, c, mintNext4(t, c, keys)) // 3, v4

	// The FIRST v5 block (height 4). Its committed root is the v5 root, which DIFFERS from
	// the era-3 root over the same state (the spine keyspaces + era-4 scalars are now
	// committed) — that difference is exactly what era-4 exists to commit.
	first := mintNext4(t, c, keys) // height 4, v5
	if first.Version != BlockVersionWitnessable {
		t.Fatalf("the first post-H_era4 block must be v5, got v%d", first.Version)
	}
	// Recompute both roots over the pre-apply state to compare shapes deterministically.
	v4Root, err := c.StateRootForVersion(BlockVersionStateRoot)
	if err != nil {
		t.Fatalf("v4 root: %v", err)
	}
	v5Root, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("v5 root: %v", err)
	}
	if v4Root == v5Root {
		t.Fatal("the v5 root must DIFFER from the v4 root over the same state — the " +
			"maintenance-spine keyspaces (qualified/dueBucket/epochStart) + the era-4 activation " +
			"scalars are committed only in the v5 root. If they are equal, the v5 leaves are not " +
			"being emitted.")
	}

	// The validator ACCEPTS the first v5 block: its recompute over the v5 leaf set matches.
	mustAppend(t, c, first)
	// RED shown: route the v5 recompute through the era-3 marshaller (StateRootForVersion
	// returning the v4 leaves for a v5 block) and validateEra3Roots rejects the first v5
	// block — the spine leaves are then missing from the recomputed root.
}

// TestEra4BoundaryIsExactNoOffByOne4d: the v5 flip is EXACTLY at H_era4 — era4Active(H-1)
// is false and era4Active(H) is true, with no off-by-one. An injected strict-greater
// comparison (h > era4Height, the WRONG boundary for a >= mint era) is caught: at H_era4
// the chain would mint v4 and the boundary rule would reject it.
//
// GATE (PACE): "Boundary determinism: the flip is exactly at the height, no off-by-one."
func TestEra4BoundaryIsExactNoOffByOne4d(t *testing.T) {
	c, _ := era4AnchorChain(t, 2, 5)

	// H_era4 = 5. Exactly at 5 era-4 is active; at 4 it is not.
	if c.era4Active(4) {
		t.Fatal("off-by-one: era4Active must be FALSE at H_era4-1 (height 4)")
	}
	if !c.era4Active(5) {
		t.Fatal("off-by-one: era4Active must be TRUE exactly at H_era4 (height 5)")
	}
	if c.MintVersion(4) != BlockVersionStateRoot {
		t.Fatalf("at H_era4-1 the chain must mint v4 (era-3 active, era-4 not), got v%d", c.MintVersion(4))
	}
	if c.MintVersion(5) != BlockVersionWitnessable {
		t.Fatalf("exactly at H_era4 the chain must mint v5, got v%d", c.MintVersion(5))
	}
	// RED shown: change era4Active's genesis-override branch to `h > cfg.Era4ActivationHeight`
	// (strict, the WRONG boundary for a >= mint era) and era4Active(5) reads false — the
	// at-boundary v5 assertion above reddens. The >= is what makes H_era4 the first v5 height.
}

// TestEra4ActivationIsReorgStableAndReplayDerived4d: H_era4 is derived committed state. A
// fresh replica replaying the identical committed history computes the IDENTICAL activation
// (so the boundary is reorg-stable); and a fork WITHOUT the era-4 ready signal carries NO
// era-4 activation. This is the post-latch (readiness-tally) path — the same guarantee
// era-3's TestEra3ActivationIsReorgStableAndReplayDerived2c gives, one era up.
//
// GATE (PACE): "Boundary determinism: all nodes agree" AND "Cross-activation continuity".
func TestEra4ActivationIsReorgStableAndReplayDerived4d(t *testing.T) {
	whale := key(53421)
	minnows := []ed25519.PrivateKey{key(53422), key(53423), key(53424)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 4, BondTTLBlocks: 64}
	build := func() *Chain {
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		// All four signal v5 (which is also v4-ready), so BOTH era-3 and era-4 lock at the
		// first boundary. The mature v1 commit() cannot build a v4/v5 block, so commit only
		// heights 1..3 (below H_era4 = 4).
		g.BondRegs = append(g.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, BlockVersionWitnessable))
		for _, m := range minnows {
			g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionWitnessable))
		}
		Sign(g, whale)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		return c
	}
	c := build()
	for c.Len() < 4 {
		commit(t, c, whale, minnows)
	}
	if !c.era4LockedIn {
		t.Fatal("rig: an all-ready-v5 genesis set must lock era-4 in at the first boundary")
	}
	if c.era4Height != 4 {
		t.Fatalf("rig: H_era4 want 4, got %d", c.era4Height)
	}
	// Layering: era-3 must have locked at the SAME boundary (a v5 signaller is v4-ready).
	if !c.era3LockedIn || c.era3Height != c.era4Height {
		t.Fatalf("layering: era-3 must lock alongside era-4 (H_era3=%d H_era4=%d)", c.era3Height, c.era4Height)
	}

	// Replay into a fresh replica: identical committed history ⇒ identical era-4 activation.
	replica := New(cfg, func(ports.NodeID) int64 { return 0 })
	replica.SetBondVerifier(objectiveVerify)
	blocks := c.Blocks(0)
	if err := replica.AppendGenesis(blocks[0]); err != nil {
		t.Fatalf("replica genesis: %v", err)
	}
	for _, b := range blocks[1:] {
		if err := replica.Append(b); err != nil {
			t.Fatalf("replica replay height %d: %v", b.Height, err)
		}
	}
	if replica.era4LockedIn != c.era4LockedIn || replica.era4Height != c.era4Height {
		t.Fatalf("4d: replayed era-4 activation diverged (live H_era4=%d, replica H_era4=%d) — "+
			"must be a pure function of committed history", c.era4Height, replica.era4Height)
	}

	// A fork WITHOUT the era-4 ready signal earns NO era-4 activation: same shape, v4-only
	// regs. This fork DOES lock era-3 at boundary 0 (all signal v4), so H_era3 = 4 and the
	// v1 commit() helper can only build heights 1..3 (below H_era3) — exactly the era-3
	// reorg test's constraint. Boundary 0 is where era-4 must decline to lock.
	// RED shown: make the era-4 rotateEpoch tally lock in whenever total>0 (drop the
	// `3*ready > 2*total` readiness threshold) and this unready fork locks era-4 in at
	// boundary 0 — the readiness gate is what makes the boundary EARNED.
	unready := New(cfg, func(ports.NodeID) int64 { return 0 })
	unready.SetBondVerifier(objectiveVerify)
	gu := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	gu.BondRegs = append(gu.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, BlockVersionStateRoot))
	for _, m := range minnows {
		gu.BondRegs = append(gu.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionStateRoot))
	}
	Sign(gu, whale)
	if err := unready.AppendGenesis(*gu); err != nil {
		t.Fatalf("unready genesis: %v", err)
	}
	for unready.Len() < 4 {
		commit(t, unready, whale, minnows)
	}
	if unready.era4LockedIn {
		t.Fatal("4d: a chain whose set never signals regVersion>=5 must never lock era-4 in — " +
			"the boundary is earned, not free")
	}
	// era-3 DID lock on the unready fork (the minnows+whale all signal v4), confirming era-4
	// is a strictly-higher, independent readiness level: the SAME frozen set that clears the
	// >=4 bar does not clear the >=5 bar.
	if !unready.era3LockedIn {
		t.Fatal("4d: the v4-signalling fork must still lock era-3 in — era-4's higher bar is independent")
	}
}

// TestEra4LayeringConstraintPanicsOnMisconfig4d: New PANICS if a genesis-declared era-4
// boundary is set BELOW the era-3 boundary — a v5 block minted below H_era3 commits a
// superset of leaves on a chain that is not yet era-3-active, an ill-formed block. The
// misconfiguration fails loudly at construction rather than at the boundary.
//
// GATE (PACE): the era-3 → era-4 layering invariant (H_era4 >= H_era3).
func TestEra4LayeringConstraintPanicsOnMisconfig4d(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("New must PANIC when Era4ActivationHeight < Era3ActivationHeight — a v5 block " +
				"below H_era3 is ill-formed (v5 ⊇ v4)")
		}
	}()
	cfg := Config{Quorum: 1, Era3ActivationHeight: 10, Era4ActivationHeight: 5}
	_ = New(cfg, func(ports.NodeID) int64 { return 0 })
	// RED shown: drop the New assertion and this constructs cleanly — the panic is what
	// forecloses minting a v5 block below the era-3 boundary. Equal heights are allowed
	// (H_era4 == H_era3 is the tightest legal layering; both eras activate together).
}

// chainRootAtBlock recomputes the committed StateRoot a validator would check for block b,
// by the block's own version — the same StateRootForVersion(b.Version) postApplyRoots uses.
// It is a read-only helper: it clones, applies b on the clone, and reads the root, leaving
// the live chain untouched. Used to assert the v4 freeze without minting.
func (c *Chain) chainRootAtBlock(t *testing.T, b Block) (ports.Hash, error) {
	t.Helper()
	sr, _, err := c.postApplyRoots(b)
	return sr, err
}
