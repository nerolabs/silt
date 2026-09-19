package chain

// THE COMMITTED MEMBERSHIP A FLOOR BOX HAS TO RECONSTRUCT IS BOUNDED, AND THE BOUND IS DERIVED
// FROM TWO RULES THAT ALREADY SHIP.
//
// WHY IT MATTERS. A block touching any id-keyed keyspace makes the box rebuild that keyspace's
// whole-set digest, and a whole-set digest is an MTH over the COMPLETE post-state member list. So
// the box's cost for such a block is O(registry), not O(payload) — the one cost in the witness
// composition that does not shrink with the block. On a 1-core / 2 GiB box that is the difference
// between a validator and an OOM: the filed measurement
// (floorbox_wholeset_witness_size_measure_test.go) puts the member list at 0.31 MB for 10,000
// members and 30.5 MB for 1,000,000, and the SMT build behind it at 1,222 MB of live heap at a
// million. Unbounded membership is therefore not a performance question, it is build-immutable 8:
// an unbounded system on a small box is unsafe, not slow.
//
// THE BOUND, AND IT NEEDS NO NEW MECHANISM. Two shipped validity rules compose into one:
//
//	RegCap          a v5 block is INVALID if it carries more than RegCap registrations,
//	                counted AFTER the same-id fold, fresh and renewal together.
//	the TTL sweep   apply evicts every id whose latest registration is older than
//	                BondTTLBlocks (chain.go: `b.Height-regH > ttl`).
//
// An id is bonded only if it registered within the last ttl blocks, and each of those blocks
// admitted at most RegCap distinct ids. So
//
//	|bonded| <= RegCap * (BondTTLBlocks + 1)
//
// and `qualified` is a subset of `bonded`, so it inherits the same ceiling. At the shipped
// objective defaults — RegCap 256, TTL 32 — that is 8,448 members, against the ~100 bonded
// validators the finished system describes. At 8,448 the member list is ~0.27 MB per digest: the
// O(registry) term is real, and it is bounded two orders of magnitude below where it hurts.
//
// TWO RESIDUALS, NAMED RATHER THAN ROUNDED AWAY.
//
//  1. THE BOUND IS CONDITIONAL ON THE TTL. With BondTTLBlocks == 0 nothing is ever evicted and
//     `bonded` grows without limit. The TTL defaults ON for an untrusted objective swarm, which is
//     the posture this bound is claimed for; a trusted or demo swarm that disables it has no bound
//     and is outside the claim. TestBondedSetIsUnboundedWithoutTheTTL holds that distinction so it
//     cannot be forgotten.
//  2. `slashed` IS MONOTONIC AND IS NOT COVERED. apply never removes a slashed id — that is the
//     point of slashing — so the slashed keyspace grows with every attributable equivocation for
//     the life of the chain. It is bounded only by how many distinct identities ever held standing
//     and then equivocated, each of which cost a real sealed bond. That is a genuine open residual
//     of the whole-set digest cost, and it is recorded here rather than in a comment that claims
//     the whole keyspace is bounded.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// membershipBoundChain builds an objective chain with the given TTL and drives `blocks` heights,
// each carrying exactly RegCap fresh registrations — the largest registration load a valid v5 block
// can carry. It returns the chain so a caller can read the live membership.
//
// Each block uses fresh identities, which is the WORST case for the bound: renewals fold into the
// same ids and cannot grow the set, so only fresh registrations can push membership up.
func membershipBoundChain(t *testing.T, ttl uint64, blocks int, regsPerBlock int) *Chain {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		EpochBlocks: 1 << 20, MatureValidators: 0, BondTTLBlocks: ttl}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(1)}}
	Sign(g, key(1))
	c.apply(*g)

	seed := int64(500000)
	for h := 1; h <= blocks; h++ {
		prev, _ := c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: uint64(h), Prev: prev}
		for i := 0; i < regsPerBlock; i++ {
			seed++
			k := key(seed)
			b.BondRegs = append(b.BondRegs, bondRegFull(k, ports.HashBytes(pubOf(k)), 4<<20, prev, 5, 1))
		}
		c.apply(b)
	}
	return c
}

// TestBondedMembershipIsBoundedByRegCapAndTTL drives the worst case the validity rules permit —
// every block carrying the maximum number of FRESH registrations — and requires the live bonded and
// qualified sets to stay under the derived ceiling.
//
// It uses a short TTL so the sweep fires inside a test, and the SAME derivation the shipped
// defaults use. The bound is a formula, not a number: reading RegCap and the configured TTL here
// means a change to either re-derives the expectation instead of silently invalidating it.
func TestBondedMembershipIsBoundedByRegCapAndTTL(t *testing.T) {
	const ttl = 2
	const blocks = 12
	// RegCap registrations per block is the most a valid block can carry; the test uses a smaller
	// per-block count so it runs in seconds, and asserts against the bound that count implies. The
	// ceiling below is therefore the REAL rule (RegCap) when regsPerBlock == RegCap, and a tighter
	// one here — which is the stronger assertion, not a weaker one.
	const regsPerBlock = 40

	c := membershipBoundChain(t, ttl, blocks, regsPerBlock)
	bounded := regsPerBlock * (ttl + 1)

	if got := len(c.bonded); got > bounded {
		t.Fatalf("THE MEMBERSHIP BOUND DOES NOT HOLD: %d bonded after %d blocks of %d fresh "+
			"registrations at TTL %d, ceiling %d.\n"+
			"  A floor box rebuilds the bonded whole-set digest over the COMPLETE member list, so an\n"+
			"  unbounded set is an unbounded per-block cost on a 2 GiB box. The bound is supposed to\n"+
			"  follow from the per-block registration cap and the TTL sweep together; if it no longer\n"+
			"  does, one of those two rules has moved and the floor-box cost claim moves with it.",
			got, blocks, regsPerBlock, ttl, bounded)
	}
	if got := len(c.qualified); got > len(c.bonded) {
		t.Fatalf("qualified (%d) exceeds bonded (%d) — qualified is filtered FROM bonded, so it can "+
			"never be larger, and the bound it inherits is void if it is", got, len(c.bonded))
	}

	// NON-VACUITY. A run where the sweep never fired, or where registrations never landed, would
	// satisfy the ceiling by doing nothing.
	if len(c.bonded) == 0 {
		t.Fatal("GATE VACUOUS: no id is bonded, so the ceiling is met by an empty set. The fixture " +
			"registered nothing, or apply screened every registration out.")
	}
	if blocks*regsPerBlock <= bounded {
		t.Fatalf("GATE VACUOUS: the run registered %d ids against a ceiling of %d, so the ceiling "+
			"could be met without the sweep ever evicting anything. Drive more blocks than the TTL "+
			"window.", blocks*regsPerBlock, bounded)
	}
	t.Logf("%d fresh registrations across %d blocks at TTL %d leave %d bonded / %d qualified, "+
		"ceiling %d. At the shipped objective defaults (RegCap %d, TTL 32) the same derivation gives %d.",
		blocks*regsPerBlock, blocks, ttl, len(c.bonded), len(c.qualified), bounded, RegCap, RegCap*33)
}

// TestBondedSetIsUnboundedWithoutTheTTL is the other half of the claim, and the reason the bound is
// stated as conditional. With the sweep disabled nothing is ever evicted, so membership grows with
// the chain and the floor box's whole-set cost grows with it.
//
// This is not a defect: a trusted or demo swarm may legitimately run without a re-challenge cadence.
// It is a SCOPE line. The bounded-cost claim belongs to the untrusted objective posture, where the
// TTL defaults on, and this test exists so nobody reads the bound as unconditional.
func TestBondedSetIsUnboundedWithoutTheTTL(t *testing.T) {
	const blocks = 12
	const regsPerBlock = 40

	withTTL := membershipBoundChain(t, 2, blocks, regsPerBlock)
	without := membershipBoundChain(t, 0, blocks, regsPerBlock)

	if len(without.bonded) != blocks*regsPerBlock {
		t.Fatalf("with the TTL disabled every registration should still be bonded: got %d, want %d",
			len(without.bonded), blocks*regsPerBlock)
	}
	if len(without.bonded) <= len(withTTL.bonded) {
		t.Fatalf("the TTL made no difference (%d bonded with it, %d without) — either the sweep is "+
			"not firing, in which case the bound above is vacuous, or the fixture is not driving "+
			"enough blocks to cross the window",
			len(withTTL.bonded), len(without.bonded))
	}
	t.Logf("TTL off: %d bonded and still growing. TTL on: %d. The bound is a property of the "+
		"re-challenge cadence, not of the chain.", len(without.bonded), len(withTTL.bonded))
}

// TestSlashedMembershipIsMonotonic records the residual the bound does not cover. A slashed id is
// never evicted — that is what slashing means — so this keyspace grows for the life of the chain
// and its whole-set digest cost grows with it.
//
// The test asserts the monotonicity rather than a ceiling, because there is no ceiling to assert.
// Its job is to make the residual visible in the suite instead of leaving it as a claim in a
// comment: if slashing ever became reversible, this goes red and the cost argument is re-opened.
func TestSlashedMembershipIsMonotonic(t *testing.T) {
	const ttl = 2
	c := membershipBoundChain(t, ttl, 3, 4)

	culprit := key(900001)
	cid := ports.HashBytes(pubOf(culprit))
	prev, h := c.Head()
	c.apply(Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		Slashes: []Equivocation{slashProof(culprit, prev, 0x41, 0x42)}})
	if !c.slashed[cid] {
		t.Fatal("FIXTURE: the culprit was not slashed, so nothing below is under test")
	}

	// Drive well past the TTL window. A bonded id would have been evicted by now.
	for i := 0; i < int(ttl)+4; i++ {
		p, hh := c.Head()
		c.apply(Block{Version: BlockVersionWitnessable, Height: hh, Prev: p})
	}
	if !c.slashed[cid] {
		t.Fatal("SLASHING BECAME REVERSIBLE: a slashed id left the set after the TTL window.\n" +
			"  That changes two things at once. A slashed equivocator could re-earn standing, which is\n" +
			"  the defence the slash exists to be; and the slashed keyspace would gain a bound it does\n" +
			"  not have today, which is an input to the floor box's whole-set cost. Re-derive both.")
	}
	t.Logf("slashed holds %d id(s) and is monotonic: it has no TTL, so the whole-set cost of this "+
		"keyspace grows with the number of identities that ever equivocated. Unbounded, and named.",
		len(c.slashed))
}
