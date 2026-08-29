package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// era-3 build step 2c — height-gated activation + mint-flip to v4.
//
// The certified activation shape (RESEARCH-CERTIFICATION-2026-08-28 Q5/Q7) is the
// #506 machinery reused one readiness level up: a frozen-epoch-weight supermajority
// signalling regVersion >= BlockVersionStateRoot (== 4) locks in era-3 at the NEXT
// finalized boundary (H_era3), from which v4 is REQUIRED (mint + validity). These
// model-check oracles assert the four properties 2c must hold, EACH paired with a
// demonstrated RED (an injected defect that flips the assertion) recorded inline, per
// the session-7 rule: a green check is not shipped until its defect has been watched
// go red. Invariants: I1/I2/I4 untouched; I3 relied on (rule change integrates only at
// a finalized boundary, by weight); I5 preserved (era3Active/MintVersion are pure
// functions of committed state). Deliberation:
// docs/thinking/2026-08-29-era3-step2c-activation-mint-flip.md.

// twoPhaseSign fills b's PrepareQC and Atts with a full round-0 two-phase certificate:
// the proposer (keys[0]) self-prepares and self-precommits (count-neutral by
// authorship), and every key signs both phases. This is the era-2/era-3 (v2/v4) commit
// certificate shape (mirrors archival_fixture_570_test.go's era2Block). b's roots and
// Version must already be set — the signatures cover them.
func twoPhaseSign(b *Block, keys []ed25519.PrivateKey) {
	Sign(b, keys[0])
	for _, k := range keys {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 0, PhasePrepare))
	}
	for _, k := range keys {
		b.Atts = append(b.Atts, AttestAt(b, k, 0, PhasePrecommit))
	}
}

// mintNext builds the next block the way a proposer WOULD (the chain-tier analogue of
// core/node's proposeBlock mint-flip): it asks the chain for the mint version, and for
// a v4 boundary block populates the committed roots over the block's own post-apply
// state (the same PopulateEra3Roots the propose path calls), then signs a full
// two-phase certificate so ValidateCommit accepts it. Below the boundary it stamps v2.
func mintNext(t *testing.T, c *Chain, keys []ed25519.PrivateKey, regs ...BondReg) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}, BondRegs: regs}
	if c.MintVersion(next) >= BlockVersionStateRoot {
		if err := c.PopulateEra3Roots(b); err != nil {
			t.Fatalf("populate era-3 roots at height %d: %v", next, err)
		}
	} else {
		b.Version = BlockVersionRounds
	}
	twoPhaseSign(b, keys)
	return b
}

// mustAppend appends b and fails the test on any validation error.
func mustAppend(t *testing.T, c *Chain, b *Block) {
	t.Helper()
	if err := c.Append(*b); err != nil {
		t.Fatalf("append height %d (v%d): %v", b.Height, b.Version, err)
	}
}

// era3AnchorChain builds an anchor-launch chain (4 anchors, keys[0] the proposer) with
// the genesis-declared era-3 activation boundary. Anchors let a two-phase v2/v4 block
// commit at the launch phase without the mature-epoch machinery, so the mint-flip and
// boundary-validity rules are exercised in isolation.
func era3AnchorChain(t *testing.T, activation uint64) (*Chain, []ed25519.PrivateKey) {
	t.Helper()
	keys := []ed25519.PrivateKey{key(52301), key(52302), key(52303), key(52304)}
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 3, BondTTLBlocks: 40, Era3ActivationHeight: activation}
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

// TestEra3PreLatchMintFlipAndBoundary2c: with the genesis-declared boundary, below
// H_era3 the chain mints v2 and a v2 block is accepted; at/above H_era3 the chain
// mints v4, a correctly-rooted v4 block is accepted, a v2 block is REJECTED
// (ErrEra3VersionRequired), and a v4 block with a wrong StateRoot is REJECTED (the 2b
// predicate). era-2 validation below the boundary is unchanged.
func TestEra3PreLatchMintFlipAndBoundary2c(t *testing.T) {
	// Boundary at height 3: heights 0..2 are era-2, height >= 3 is era-3 (the >=
	// convention — H_era3 is itself the first v4 height).
	c, keys := era3AnchorChain(t, 3)

	// --- Below H_era3: mint v2, accept v2, no v4 requirement. ---
	if v := c.MintVersion(1); v != BlockVersionRounds {
		t.Fatalf("below H_era3: MintVersion must be v2 (%d), got %d", BlockVersionRounds, v)
	}
	if c.era3Active(1) {
		t.Fatal("below H_era3: era3Active must be false")
	}
	mustAppend(t, c, mintNext(t, c, keys)) // height 1, v2
	mustAppend(t, c, mintNext(t, c, keys)) // height 2, v2
	if c.Blocks(1)[0].Version != BlockVersionRounds {
		t.Fatalf("below H_era3: minted block must be v2, got v%d", c.Blocks(1)[0].Version)
	}
	// RED shown: if the boundary rule fired early (era3Active used a height < H_era3,
	// or MintVersion returned v4 below the boundary), height 1's v2 block would be
	// rejected by ValidateProposal and mustAppend would fail here. It does not.

	// --- At H_era3 (height 3): mint v4, accept correct v4. ---
	if v := c.MintVersion(3); v != BlockVersionStateRoot {
		t.Fatalf("at H_era3: MintVersion must be v4 (%d), got %d", BlockVersionStateRoot, v)
	}
	if !c.era3Active(3) {
		t.Fatal("at H_era3: era3Active must be true")
	}
	good := mintNext(t, c, keys) // height 3, v4 with correct roots
	if good.Version != BlockVersionStateRoot {
		t.Fatalf("at H_era3: mintNext must produce v4, got v%d", good.Version)
	}

	// A v2 block at H_era3 is REJECTED (the version-boundary rule). Build one by hand
	// (bypassing mintNext's flip) at the current head.
	prev, next := c.Head()
	v2AtBoundary := &Block{Version: BlockVersionRounds, Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	twoPhaseSign(v2AtBoundary, keys)
	if err := c.Append(*v2AtBoundary); !errors.Is(err, ErrEra3VersionRequired) {
		t.Fatalf("at H_era3: a v2 block must be rejected with ErrEra3VersionRequired, got %v", err)
	}
	// RED shown: delete the era3Active version check in ValidateProposal and this v2
	// block Appends cleanly — the rejection is what enforces the mint-flip at the boundary.

	// A v4 block with a WRONG StateRoot is REJECTED (the 2b predicate).
	prev, next = c.Head()
	wrong := &Block{Version: BlockVersionStateRoot, Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if err := c.PopulateEra3Roots(wrong); err != nil {
		t.Fatalf("populate roots: %v", err)
	}
	bad := *wrong.StateRoot
	bad[0] ^= 0xFF // corrupt the committed state root
	wrong.StateRoot = &bad
	twoPhaseSign(wrong, keys)
	if err := c.Append(*wrong); !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("at H_era3: a v4 block with a wrong StateRoot must be rejected (2b), got %v", err)
	}

	// The correct v4 block commits.
	mustAppend(t, c, good)
	if c.Blocks(3)[0].Version != BlockVersionStateRoot {
		t.Fatalf("at H_era3: committed block must be v4, got v%d", c.Blocks(3)[0].Version)
	}
	// And era-3 continues: the next block also mints v4 and commits.
	mustAppend(t, c, mintNext(t, c, keys)) // height 4, v4
	if c.Blocks(4)[0].Version != BlockVersionStateRoot {
		t.Fatalf("past H_era3: minting must stay v4, got v%d", c.Blocks(4)[0].Version)
	}
}

// TestEra3PostLatchReadinessGatesActivation2c: era-3 activation via the epoch
// readiness tally is WEIGHT-counted (a cheap-bond heads majority does NOT lock in),
// locks in at the first boundary the ready weight clears >⅔, sets H_era3 to the NEXT
// boundary, and is MONOTONIC (a later ready-weight collapse never un-flips). This is
// the #506 tally at the regVersion >= 4 level, so it mirrors
// TestRegGateLockInIsWeightCountedAndBoundaryExact506.
func TestEra3PostLatchReadinessGatesActivation2c(t *testing.T) {
	whale := key(52311)
	minnows := []ed25519.PrivateKey{key(52312), key(52313), key(52314)}
	const whaleMiB = 8 << 20
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 4, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis: whale 8 MiB NOT era-3-signalling; minnows 2 MiB each signalling v4.
	// Ready weight 6/14 (43%) but ready heads ¾ (75%).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs, bondReg(whale, whaleMiB, ports.Hash{}))
	for _, m := range minnows {
		g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionStateRoot))
	}
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}

	// Two full epochs (boundaries 4, 8): the ¾ HEADS majority at 43% weight must NOT
	// lock era-3 in. Below the boundary the chain mints v2, so commit() (v1-tagged
	// era-2 blocks) still applies — nothing is era-3 active yet.
	for c.Len() < 9 {
		commit(t, c, whale, minnows)
	}
	if c.era3LockedIn {
		t.Fatal("2c: a ¾ HEADS majority at 43% weight locked era-3 in — readiness must be weight-counted")
	}
	// RED shown: change the tally's `ready += w` to `ready++` (count heads) and this
	// locks in at boundary 4 — the weight-counting is what refuses the cheap majority.

	// The whale's binary upgrades: its renewal signals v4. Ready weight 14/14 at the
	// next boundary (12) → lock-in there; H_era3 = 12 + 4 = 16.
	commit(t, c, whale, minnows, bondRegV(whale, whaleMiB, mustHead(c), BlockVersionStateRoot))
	for c.Len() < 13 {
		commit(t, c, whale, minnows)
	}
	if !c.era3LockedIn {
		t.Fatal("2c: full ready weight at an epoch boundary must lock era-3 in")
	}
	if c.era3Height != 16 {
		t.Fatalf("2c: H_era3 must be the NEXT boundary after lock-in (want 16), got %d", c.era3Height)
	}
	// RED shown: set era3Height = h (not h + EpochBlocks) and this reads 12 — the
	// one-epoch-of-notice is what the +EpochBlocks encodes.

	if c.MintVersion(16) != BlockVersionStateRoot || !c.era3Active(16) {
		t.Fatalf("2c: at/above H_era3=16 the chain must mint v4 and be era3Active")
	}
	if c.MintVersion(15) != BlockVersionRounds || c.era3Active(15) {
		t.Fatalf("2c: below H_era3=16 the chain must mint v2 and not be era3Active")
	}

	// Monotonic: the whale's next renewal reverts to a version-less binary (ready
	// weight collapses below ⅔), but era-3 activation never un-latches. Drive to just
	// before H_era3 so the chain still mints v2 for these era-2 commits.
	for c.Len() < 15 {
		commit(t, c, whale, minnows)
	}
	commit(t, c, whale, minnows, bondReg(whale, whaleMiB, mustHead(c))) // height 15, version-less renewal
	if !c.era3LockedIn || c.era3Height != 16 {
		t.Fatalf("2c: activation must be monotonic across a ready-weight collapse (lockedIn=%v H_era3=%d)",
			c.era3LockedIn, c.era3Height)
	}
	// RED shown: drop the `!c.era3LockedIn` guard on the tally and a later below-⅔
	// boundary would recompute/clear it — the guard is what makes it monotonic.
}

// TestEra3ActivationIsReorgStableAndReplayDerived2c: H_era3 is derived committed
// state. A fresh replica replaying the identical committed history computes the
// IDENTICAL activation (so the boundary is reorg-stable — a reorg replays every
// rotation and re-derives the same H_era3, epoch-final per #357 Condition A); and a
// fork WITHOUT the ready signal carries NO activation (the boundary cannot be moved to
// un-enforce it). Cert Q5.
func TestEra3ActivationIsReorgStableAndReplayDerived2c(t *testing.T) {
	whale := key(52321)
	minnows := []ed25519.PrivateKey{key(52322), key(52323), key(52324)}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 4, BondTTLBlocks: 64}
	build := func() *Chain {
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = append(g.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, BlockVersionStateRoot))
		for _, m := range minnows {
			g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, BlockVersionStateRoot))
		}
		Sign(g, whale)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		return c
	}
	c := build()
	// This rig matures at genesis (all bonds mature immediately), so lock-in fires at
	// the boundary-0 rotation and H_era3 = 0 + EpochBlocks = 4. The mature v1 commit()
	// cannot build a v4 block, so commit only heights 1..3 (below H_era3).
	for c.Len() < 4 {
		commit(t, c, whale, minnows)
	}
	if !c.era3LockedIn {
		t.Fatal("rig: an all-ready genesis set must lock era-3 in at the first boundary")
	}
	if c.era3Height != 4 {
		t.Fatalf("rig: H_era3 want 4, got %d", c.era3Height)
	}

	// Replay into a fresh replica: identical committed history ⇒ identical activation.
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
	if replica.era3LockedIn != c.era3LockedIn || replica.era3Height != c.era3Height {
		t.Fatalf("2c: replayed activation diverged (live H_era3=%d, replica H_era3=%d) — must be a pure function of committed history",
			c.era3Height, replica.era3Height)
	}

	// A fork WITHOUT the ready signal earns NO activation: same shape, version-less regs.
	// RED shown: make the rotateEpoch tally lock in whenever total>0 (drop the
	// `3*ready > 2*total` readiness threshold) and this unready fork locks era-3 in at
	// boundary 0 — the readiness gate is what makes the boundary EARNED, so it cannot be
	// moved onto a history that never signalled it.
	unready := New(cfg, func(ports.NodeID) int64 { return 0 })
	unready.SetBondVerifier(objectiveVerify)
	gu := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	gu.BondRegs = append(gu.BondRegs, bondReg(whale, twoMiB, ports.Hash{}))
	for _, m := range minnows {
		gu.BondRegs = append(gu.BondRegs, bondReg(m, twoMiB, ports.Hash{}))
	}
	Sign(gu, whale)
	if err := unready.AppendGenesis(*gu); err != nil {
		t.Fatalf("unready genesis: %v", err)
	}
	for unready.Len() < 10 {
		commit(t, unready, whale, minnows)
	}
	if unready.era3LockedIn {
		t.Fatal("2c: a chain whose set never signals regVersion>=4 must never lock era-3 in — the boundary is earned, not free")
	}
}
