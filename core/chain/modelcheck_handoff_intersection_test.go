package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — the launch→mature seam.
//
// Intersection is a claim about ONE height: at most one block finalizes there. The launch
// oracle enumerates disjoint anchor coalitions while the training wheels are engaged, and
// the mature oracle pins the weight threshold once they have shed. Neither asserts what
// must hold BETWEEN them, because each runs wholly inside one phase.
//
// The seam is load-bearing because the eligible-attester set changes DISCONTINUOUSLY
// there: launchAnchorGiven grants a declared anchor bond-free eligibility while
// !handedOff and takes it away forever afterward. A network whose anchors have not
// registered their own bonds therefore swaps one signer population for another at a
// single height. If two honest replicas could hold different phase rules at one height,
// each would finalize a block the other rejects and they would partition permanently
// with no Byzantine actor present.
//
// They cannot, and these oracles pin the two structural reasons why: the maturity latch
// is a pure function of applied history (so delivery order cannot move it), and fork
// adoption replays that history rather than carrying the live scalar across.

// seamEras are the block versions the chain actually mints. v3 is a registration-gate
// threshold, never a mint target, so the mintable set is v2, v4 and v5 — and v5 is the
// open era, the one a shipped untrusted validator runs. A seam property that holds only
// on the oldest format asserts nothing about what ships, so every oracle below runs the
// whole set.
var seamEras = []struct {
	name         string
	era3, era4   uint64
	mintsVersion uint64
}{
	{"v2 rounds", 0, 0, BlockVersionRounds},
	{"v4 state root", 1, 0, BlockVersionStateRoot},
	{"v5 witnessable (open era)", 1, 1, BlockVersionWitnessable},
}

// seamEra is one era's chain plus the keys that sign for it.
type seamEra struct {
	c      *Chain
	ak, vk []ed25519.PrivateKey
	era4   bool
}

// newSeamEra builds the seam fixture at a given era: unbonded launch anchors whose
// eligibility vanishes at the handoff, plus bonded validators in distinct domains.
func newSeamEra(t *testing.T, nAnchors, nBonded int, era3, era4 uint64, epochBlocks uint64) *seamEra {
	t.Helper()
	const bond = int64(20) << 20

	ak := make([]ed25519.PrivateKey, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ak {
		ak[i] = key(int64(9200 + i))
		anchors[idOf(ak[i])] = true
	}
	vk := make([]ed25519.PrivateKey, nBonded)
	for i := range vk {
		vk[i] = key(int64(9300 + i))
	}

	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, MatureValidators: 2, OperatorMargin: 1,
		EpochBlocks: epochBlocks, Era3ActivationHeight: era3, Era4ActivationHeight: era4,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range vk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, bond, ports.Hash{}, uint64(i+1)))
	}
	Sign(g, ak[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return &seamEra{c: c, ak: ak, vk: vk, era4: era4 > 0}
}

// mint builds the next block at whatever version this chain mints, signed by `signers`
// and SEATING `seat` as validators the chain has seen.
//
// Which field does the seating is exactly what changes across the eras, and it is the
// reason these oracles must run on all of them: below v5 the seating is read from the
// block's own attestations, which a block hash cannot cover because they are signatures
// over that hash. At v5 it moves to the LastCommit carrier, which IS folded into the
// hash. Any consensus state derived from an un-hash-covered field can be rewritten by a
// fork that matches our hashes block for block.
func (w *seamEra) mint(t *testing.T, signers, seat []ed25519.PrivateKey, seed byte) *Block {
	t.Helper()
	prev, next := w.c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(seed)}}

	if w.era4 {
		// The carrier seats: genuine precommits over the PARENT, folded into this
		// block's hash. Height 1 carries no carrier by rule — there is no committed
		// parent to precommit over — so the first block seats nobody in this era.
		if next > 1 {
			parent := w.c.blocks[len(w.c.blocks)-1]
			for _, k := range seat {
				b.LastCommit = append(b.LastCommit, AttestAt(&parent, k, 0, PhasePrecommit, w.c.ChainID()))
			}
		}
		if err := w.c.PopulateEra4Roots(b); err != nil {
			t.Fatalf("era-4 roots at height %d: %v", next, err)
		}
	} else if w.c.MintVersion(next) >= BlockVersionStateRoot {
		if err := w.c.PopulateEra3Roots(b); err != nil {
			t.Fatalf("era-3 roots at height %d: %v", next, err)
		}
	} else {
		b.Version = BlockVersionRounds
	}

	// Below v5 the attestations are the seating, so the seat set signs.
	sign := signers
	if !w.era4 {
		sign = append(append([]ed25519.PrivateKey{}, signers...), seat...)
	}
	twoPhaseSign(b, sign, w.c.ChainID())
	return b
}

// seamNetwork builds an objective network whose eligible-attester set CHANGES at the
// handoff. The A launch anchors carry NO committed bond — pure training wheels, eligible
// only while the network is young — and the V validators carry real bonds in distinct
// domains. That is the sharpest form of the seam, and the form a real launch takes:
// anchors are expected to shed, not to have been quietly bonded all along.
func seamNetwork(t *testing.T, nAnchors, nBonded int) (*Chain, []ed25519.PrivateKey, []ed25519.PrivateKey) {
	return seamNetworkEpochs(t, nAnchors, nBonded, 0)
}

// seamNetworkEpochs is seamNetwork with the mature-phase epoch cadence supplied.
// epochBlocks 0 disables epochs — the live-recompute path a trusted/demo swarm takes.
// Any positive cadence is the untrusted-validator posture, where the finality quorum and
// the weight quorum are read from a snapshot frozen at the last finalized boundary.
func seamNetworkEpochs(t *testing.T, nAnchors, nBonded int, epochBlocks uint64) (*Chain, []ed25519.PrivateKey, []ed25519.PrivateKey) {
	t.Helper()
	const bond = int64(20) << 20

	ak := make([]ed25519.PrivateKey, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ak {
		ak[i] = key(int64(9200 + i))
		anchors[idOf(ak[i])] = true
	}
	vk := make([]ed25519.PrivateKey, nBonded)
	for i := range vk {
		vk[i] = key(int64(9300 + i))
	}

	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, MatureValidators: 2, OperatorMargin: 1,
		EpochBlocks: epochBlocks,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis banks the real bonds, one distinct domain each. The anchors bank nothing:
	// their eligibility IS the launch crutch and vanishes with it.
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, k := range vk {
		g.BondRegs = append(g.BondRegs, bondRegDom(k, bond, ports.Hash{}, uint64(i+1)))
	}
	Sign(g, ak[0]) // an anchor proposes genesis — the cold-start crutch doing its job
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, ak, vk
}

// maturingBlock is a valid launch-phase commit at height 1 whose bonded attesters, once
// applied, carry the maturity coefficient over the bar and trip the one-way latch. The
// proposer is an anchor because a young network admits no other proposer; the bonded
// validators attest, and attesters (never the proposer) are what enter validatorsSeen.
func maturingBlock(prev ports.Hash, ak, vk []ed25519.PrivateKey, seed byte) *Block {
	return maturingBlockAt(prev, 1, ak, vk, seed)
}

func maturingBlockAt(prev ports.Hash, height uint64, ak, vk []ed25519.PrivateKey, seed byte) *Block {
	b := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(seed)}}
	Sign(b, ak[0])
	b.Atts = append(b.Atts, Attest(b, ak[1]))
	for _, k := range vk {
		b.Atts = append(b.Atts, Attest(b, k))
	}
	return b
}

// anchorBlock is a valid launch-phase commit carried by the unbonded anchors alone — the
// block shape that is eligible before the handoff and ineligible after it.
func anchorBlock(prev ports.Hash, height uint64, ak []ed25519.PrivateKey, seed byte) *Block {
	b := &Block{Version: 1, Height: height, Prev: prev, Entries: []ports.Entry{entry(seed)}}
	Sign(b, ak[0])
	for _, k := range ak[1:] {
		b.Atts = append(b.Atts, Attest(b, k))
	}
	return b
}

// TestModelCheck_HandoffRulesFollowHistoryNotDeliveryOrder pins the property that makes
// the seam safe: which phase rules govern a height is decided by the history a replica
// has applied, never by the order blocks arrived in. Two replicas are fed the same two
// blocks in opposite orders; both must end on the same head holding the same phase.
//
// Were this false, one replica would finalize an anchor-carried block that the other
// refuses as ineligible, at one height, with no Byzantine actor — a permanent partition
// arrived at by network jitter alone.
func TestModelCheck_HandoffRulesFollowHistoryNotDeliveryOrder(t *testing.T) {
	first, ak, vk := seamNetwork(t, 3, 4)
	second, _, _ := seamNetwork(t, 3, 4)
	genesis, _ := first.Head()

	matures := maturingBlock(genesis, ak, vk, 0xA1)
	plain := anchorBlock(genesis, 1, ak, 0xA2)

	// Setup check: BOTH siblings are valid launch commits, so the orders being compared
	// are genuinely available to an honest replica.
	if err := first.ValidateCommit(matures); err != nil {
		t.Fatalf("GATE VACUOUS: the maturity-tripping sibling must be a valid launch commit: %v", err)
	}
	if err := first.ValidateCommit(plain); err != nil {
		t.Fatalf("GATE VACUOUS: the anchor-carried sibling must be a valid launch commit: %v", err)
	}

	// Order A: the maturing block lands. Order B: the plain block lands first and the
	// maturing block arrives as a competing sibling at the same height.
	if err := first.Append(*matures); err != nil {
		t.Fatalf("order A: %v", err)
	}
	if err := second.Append(*plain); err != nil {
		t.Fatalf("order B: %v", err)
	}
	_ = second.Append(*matures) // a same-height sibling: adopted or refused, both are fine

	// Whatever each replica ended up holding, the phase must be a function of THAT head,
	// not of the order it saw. Replicas on the same head hold the same rules.
	headFirst, _ := first.Head()
	headSecond, _ := second.Head()
	if headFirst == headSecond && first.handedOff() != second.handedOff() {
		t.Fatalf("PHASE DEPENDS ON DELIVERY ORDER — two replicas on the SAME head %x disagree about the "+
			"handoff (%v vs %v). The eligible-attester set changes at the seam, so they would finalize "+
			"different blocks at one height and partition permanently on jitter alone.",
			headFirst[:8], first.handedOff(), second.handedOff())
	}

	// And the latch must reflect the applied history: the replica that applied the
	// maturity-tripping block has handed off.
	if !first.handedOff() {
		t.Fatalf("GATE VACUOUS: applying the maturity-tripping block left the latch down "+
			"(coefficient %d, bar 2) — this fixture never reaches the seam and asserts nothing",
			first.MatureCoefficient())
	}
	t.Logf("phase follows history: head-matched replicas agree on the handoff; coefficient %d cleared the bar",
		first.MatureCoefficient())
}

// TestModelCheck_HandoffLatchNeverReArms is the one-way-shed oracle at the seam. The
// maturity latch is monotonic by promise: once the network has handed off, the launch
// anchors lose bond-free eligibility FOREVER, so no later loss of decentralization can
// restore a standing dependency on them.
//
// Fork adoption is where that promise is most exposed, because adopt() assigns the latch
// from the replayed trial chain rather than preserving the live scalar — a fork that
// never matured carries a FALSE latch. What protects the shed is not the assignment but
// the finality gate above it: every committed block is super-quorum-final, so a fork that
// omits the block which tripped maturity is refused before the replay ever runs. This
// oracle drives the real adoption path and pins that refusal, with a positive control so
// a Reconcile that refuses EVERYTHING cannot pass it.
func TestModelCheck_HandoffLatchNeverReArms(t *testing.T) {
	for _, era := range seamEras {
		for _, ep := range []struct {
			name   string
			blocks uint64
		}{{"epochs on", 4}, {"epochs off", 0}} {
			t.Run(era.name+", "+ep.name, func(t *testing.T) {
				assertShedHolds(t, era.era3, era.era4, ep.blocks)
			})
		}
	}
}

func assertShedHolds(t *testing.T, era3, era4, epochBlocks uint64) {
	t.Helper()
	w := newSeamEra(t, 3, 4, era3, era4, epochBlocks)

	// Drive to the handoff, seating the bonded validators so their distinct domains
	// carry the maturity coefficient over the bar.
	var driveErr error
	for h := 0; h < 16 && !w.c.handedOff(); h++ {
		b := w.mint(t, w.ak, w.vk, byte(0xB0+h))
		if driveErr = w.c.Append(*b); driveErr != nil {
			break
		}
	}
	if !w.c.handedOff() {
		if era4 == 0 && era3 > 0 {
			// The state-root format cannot seat a first-time attester, so it can never
			// reach maturity and has no shed to exercise. That is the reason the daemon
			// refuses to run any height on it; it is pinned by its own gate.
			t.Logf("the state-root format never hands off (coefficient %d, stopped at height %d: %v) — a format "+
				"whose validator set cannot grow can never mature, which is why the daemon refuses it",
				w.c.MatureCoefficient(), len(w.c.blocks), driveErr)
			return
		}
		t.Fatalf("GATE VACUOUS: the network never handed off (coefficient %d, bar 2), so there is no latch to "+
			"re-arm and this asserts nothing. The chain stopped at height %d with: %v.",
			w.c.MatureCoefficient(), len(w.c.blocks), driveErr)
	}
	matured, _ := w.c.Head()
	mintedAt := w.c.blocks[len(w.c.blocks)-1].Version

	// The re-arming history: the launch anchors alone, seating nobody, so it never
	// matures. It is built on its OWN replica from the same genesis, which is how an
	// adversary builds it — every root and digest is honest for the state it commits.
	rival := newSeamEra(t, 3, 4, era3, era4, epochBlocks)
	for len(rival.c.blocks) < len(w.c.blocks)+2 {
		b := rival.mint(t, rival.ak, rival.ak, byte(0xF0+len(rival.c.blocks)))
		if err := rival.c.Append(*b); err != nil {
			t.Fatalf("building the rival history at height %d (v%d): %v", b.Height, b.Version, err)
		}
	}
	fork := append([]Block(nil), rival.c.blocks...)
	if fork[0].Hash() != w.c.blocks[0].Hash() {
		t.Fatalf("GATE VACUOUS: the rival history branches from a different genesis — Reconcile refuses it "+
			"as foreign before the shed is ever tested")
	}
	if len(fork) <= len(w.c.blocks) {
		t.Fatalf("GATE VACUOUS: the rival history (%d blocks) is not longer than ours (%d) — fork-choice "+
			"refuses it on height alone and the shed is never tested", len(fork), len(w.c.blocks))
	}

	adopted, err := w.c.Reconcile(fork)
	if adopted {
		t.Fatalf("THE SHED RE-ARMED (minted v%d) — a longer anchor-only history was ADOPTED, reverting the blocks "+
			"that tripped maturity. The latch is now %v and the launch anchors have their bond-free eligibility "+
			"back, so a reorg restores exactly the standing dependency the one-way latch exists to make permanent.",
			mintedAt, w.c.handedOff())
	}
	if !w.c.handedOff() {
		t.Fatalf("THE SHED RE-ARMED WITHOUT AN ADOPTION (minted v%d) — Reconcile refused the history (%v) but the "+
			"latch is DOWN, so the refusal path mutated the very scalar it exists to protect.", mintedAt, err)
	}
	if head, _ := w.c.Head(); head != matured {
		t.Fatalf("the refused history moved the head from %x to %x — a refusal must leave the chain untouched",
			matured[:8], head[:8])
	}

	// Positive control: a history that CONTAINS our finalized head and extends it must
	// still adopt, so the refusal above is the finality gate doing its job rather than
	// Reconcile saying no to everything.
	ext := append([]Block(nil), w.c.blocks...)
	for i := 0; i < 2; i++ {
		b := w.mint(t, w.vk, w.vk, byte(0xE0+i))
		if err := w.c.Append(*b); err != nil {
			t.Fatalf("extending our own history at height %d (v%d): %v", b.Height, b.Version, err)
		}
		ext = append(ext, *b)
	}
	if ok, rerr := w.c.Reconcile(ext); !ok && rerr != nil {
		t.Fatalf("GATE VACUOUS: a history that CONTAINS the finalized head was also refused (%v) — this oracle "+
			"cannot tell the finality gate from a Reconcile that refuses everything", rerr)
	}
	if !w.c.handedOff() {
		t.Fatalf("THE SHED RE-ARMED (minted v%d) — extending our own matured history left the latch down", mintedAt)
	}
	t.Logf("minted v%d: the shed held — an anchor-only history omitting the finalized maturity blocks is refused, "+
		"and one containing them adopts with the latch up", mintedAt)
}

// TestModelCheck_HandoffBondedAnchorsIntersect bounds the seam from the other side and
// proves these oracles DISCRIMINATE rather than passing by construction. When the anchors
// registered their own real bonds before the handoff — the posture the launch crutch
// documents as expected — they stay eligible on real weight after the wheels shed, the
// eligible set does not change discontinuously, and coalitions drawn under either phase
// rule must intersect.
func TestModelCheck_HandoffBondedAnchorsIntersect(t *testing.T) {
	build := func() (*Chain, []ed25519.PrivateKey) {
		const bond = int64(20) << 20
		ak := make([]ed25519.PrivateKey, 3)
		anchors := map[ports.NodeID]bool{}
		for i := range ak {
			ak[i] = key(int64(9400 + i))
			anchors[idOf(ak[i])] = true
		}
		vk := make([]ed25519.PrivateKey, 4)
		for i := range vk {
			vk[i] = key(int64(9500 + i))
		}
		cfg := Config{
			Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
			Anchors: anchors, MatureValidators: 2, OperatorMargin: 1,
		}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		// EVERY signer banks a real bond, anchors included — the shed is then a change
		// of privilege, not a change of membership.
		all := append(append([]ed25519.PrivateKey{}, ak...), vk...)
		for i, k := range all {
			g.BondRegs = append(g.BondRegs, bondRegDom(k, bond, ports.Hash{}, uint64(i+1)))
		}
		Sign(g, ak[0])
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		return c, all
	}

	young, pool := build()
	old, _ := build()
	old.everMature = true // the counterfactual: the same parent under the post-shed rules

	prev, _ := young.Head()
	launch := finalCoalitions(t, young, pool, prev, 1)
	mature := finalCoalitions(t, old, pool, prev, 2)

	if len(launch) == 0 || len(mature) == 0 {
		t.Fatalf("GATE VACUOUS: launch=%d mature=%d finalizing coalitions — a phase that finalizes nothing asserts nothing",
			len(launch), len(mature))
	}
	for i := range launch {
		for j := range mature {
			if disjoint(launch[i], mature[j]) {
				t.Fatalf("bonded anchors still admit disjoint coalitions across the phases: launch %v and mature %v "+
					"share no signer — the shed is not privilege-only and the seam needs a rule, not a posture",
					launch[i], mature[j])
			}
		}
	}
	t.Logf("bonded anchors: %d launch and %d mature coalitions, all intersecting — a bonded anchor set makes the shed privilege-only",
		len(launch), len(mature))
}

// finalCoalitions enumerates EVERY (proposer, attester-subset) over the supplied signer
// pool at the contested height and returns the signer-index set of each coalition whose
// block passes ValidateCommit — every coalition that would finalize under the rules this
// replica holds.
func finalCoalitions(t *testing.T, c *Chain, pool []ed25519.PrivateKey, prev ports.Hash, contentSeed byte) [][]int {
	t.Helper()
	var final [][]int
	for p := range pool {
		others := make([]int, 0, len(pool)-1)
		for i := range pool {
			if i != p {
				others = append(others, i)
			}
		}
		for _, attMask := range anchorSubsets(len(others)) {
			b := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(contentSeed)}}
			Sign(b, pool[p])
			signers := []int{p}
			for _, oi := range attMask {
				ai := others[oi]
				b.Atts = append(b.Atts, Attest(b, pool[ai]))
				signers = append(signers, ai)
			}
			if c.ValidateCommit(b) == nil {
				final = append(final, signers)
			}
		}
	}
	return final
}
