package chain

import (
	"bytes"
	"errors"
	"sort"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// sortMembersByWeightDesc sorts ids by committed weight descending, ties broken by id bytes.
func sortMembersByWeightDesc(ids []ports.NodeID, weights map[ports.NodeID]int64) {
	sort.Slice(ids, func(i, j int) bool {
		if weights[ids[i]] != weights[ids[j]] {
			return weights[ids[i]] > weights[ids[j]]
		}
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})
}

// Tests for the trustless floor-box RECOMPUTE increment 1 (floorbox_recompute_v5.go): the
// root-only reproduction of requireEpochWeightQuorum (Σ epochSet weight super-quorum), proving
// the C-1 weight-composition pattern.
//
// The three HARD ABLATIONS (C-5, red-before-green), each injected and watched to flip the
// verdict, so a green here is not decoration:
//   - FORGED WEIGHT (C-1): a witness with the right members but a forged per-member weight ⇒
//     the recompute STALLS (the inclusion proof fails against the committed root). Proves C-1
//     closed the forgeable-tally hole.
//   - OMITTED MEMBER: a witness missing a frozen member ⇒ the reconstructed MTH ≠ the committed
//     epochSetRoot ⇒ STALL. Proves set-completeness.
//   - GENESIS-CONFIG-FROM-WITNESS (C-6): the recompute reads no threshold from the witness — a
//     shifted witness-carried threshold cannot move the verdict, because own config governs.
//
// The recompute NEVER flips WitnessValidateV5 to Accept (the STOP boundary); it reproduces ONE
// predicate.

// recomputeFixture is a mature v5 chain with a populated epochSet, plus the committed StateRoot
// and a Prover over its v5 leaf set. A floor box holds recomputeFixture.root; the Prover stands
// in for the any-of-N witness provider that holds the committed set.
type recomputeFixture struct {
	c        *Chain
	root     ports.Hash
	prover   *statehash.Prover
	members  []ports.NodeID // the frozen epochSet ids, in a stable order
	weights  map[ports.NodeID]int64
	proposer ports.NodeID
}

// buildRecomputeFixture matures an objective v5 chain so epochSet is frozen and populated, then
// snapshots its committed StateRoot and a Prover over its v5 leaves. It uses MatureValidators=0
// (matures at genesis) with several distinct bonds so the frozen epochSet has enough weight
// spread to make the >⅔ quorum math non-trivial.
func buildRecomputeFixture(t *testing.T) recomputeFixture {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 0, BondTTLBlocks: 64}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis (h0 is an epoch boundary) seats five distinct bonds with DISTINCT weights, so the
	// weight fold is not a degenerate all-equal set. Proposer is the first key.
	prop := key(93001)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	weights := []int64{8 << 20, 6 << 20, 5 << 20, 4 << 20, 4 << 20}
	keys := []ports.NodeID{}
	for i, w := range weights {
		k := key(int64(93001 + i))
		keys = append(keys, idOf(k))
		g.BondRegs = append(g.BondRegs, bondRegFull(k, ports.HashBytes(pubOf(k)), w, ports.Hash{}, 5, uint64(i+1)))
	}
	Sign(g, prop)
	c.apply(*g)

	if len(c.epochSet) == 0 {
		t.Fatal("fixture precondition: epochSet must be frozen (non-empty) after the genesis boundary")
	}

	leaves := c.stateRootLeavesV5()
	prover, err := statehash.NewProver(leaves)
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	root := prover.Root()

	// Confirm the Prover's root equals the chain's own v5 StateRoot (the box holds this root).
	sr, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if sr != root {
		t.Fatalf("fixture root mismatch: prover=%x chain=%x", root, sr)
	}

	members := make([]ports.NodeID, 0, len(c.epochSet))
	wmap := make(map[ports.NodeID]int64, len(c.epochSet))
	for id, w := range c.epochSet {
		members = append(members, id)
		wmap[id] = w
	}
	return recomputeFixture{c: c, root: root, prover: prover, members: members, weights: wmap, proposer: idOf(prop)}
}

// witnessFor builds a well-formed EpochSetWitness proving the complete epochSet against the
// committed root: the epochSetRoot digest leaf + one per-member weight leaf per id.
func (f recomputeFixture) witnessFor(t *testing.T) EpochSetWitness {
	t.Helper()
	digestKey := statehash.Key(tagEpochSetRoot, nil)
	digestVal := nodeSetMTHFromInt64(f.c.epochSet)
	dProof, err := f.prover.Prove(digestKey)
	if err != nil {
		t.Fatalf("Prove(epochSetRoot): %v", err)
	}
	mw := make(map[ports.NodeID]MemberWeightWitness, len(f.members))
	for _, id := range f.members {
		mk := statehash.Key(tagEpochSet, id[:])
		p, err := f.prover.Prove(mk)
		if err != nil {
			t.Fatalf("Prove(epochSet[%x]): %v", id[:], err)
		}
		mw[id] = MemberWeightWitness{Weight: f.weights[id], Proof: p}
	}
	return EpochSetWitness{
		IDs:               append([]ports.NodeID(nil), f.members...),
		DigestRootWitness: dProof,
		DigestRootValue:   digestVal,
		MemberWeights:     mw,
	}
}

// seenAll returns a seen-set of every member except the proposer (a full coalition).
func (f recomputeFixture) seenAll() map[ports.NodeID]bool {
	seen := make(map[ports.NodeID]bool)
	for _, id := range f.members {
		if id != f.proposer {
			seen[id] = true
		}
	}
	return seen
}

// TestRecomputeEpochWeightQuorum_MatchesFullNode is the equivalence anchor: over the SAME
// proposer + attester coalition, the trustless recompute's verdict equals the full node's
// requireEpochWeightQuorum verdict — for BOTH a passing coalition (proposer + all attesters)
// and a failing one (proposer alone, <⅔ of the weight). This proves the recompute reproduces
// the predicate, not merely that it does not crash.
func TestRecomputeEpochWeightQuorum_MatchesFullNode(t *testing.T) {
	f := buildRecomputeFixture(t)

	t.Run("full coalition -> quorum met (matches full node)", func(t *testing.T) {
		seen := f.seenAll()
		w := f.witnessFor(t)
		met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
		if reason != nil {
			t.Fatalf("recompute stalled unexpectedly: %v", reason)
		}
		nodeErr := f.c.requireEpochWeightQuorum(f.proposer, seen, 0)
		if met != (nodeErr == nil) {
			t.Fatalf("recompute verdict %v != full node verdict %v (nodeErr=%v)", met, nodeErr == nil, nodeErr)
		}
		if !met {
			t.Fatal("full coalition should meet the quorum")
		}
	})

	t.Run("proposer alone -> quorum NOT met (matches full node)", func(t *testing.T) {
		seen := map[ports.NodeID]bool{} // proposer alone: its weight is < ⅔ of the total
		w := f.witnessFor(t)
		met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
		if reason != nil {
			t.Fatalf("recompute stalled unexpectedly: %v", reason)
		}
		nodeErr := f.c.requireEpochWeightQuorum(f.proposer, seen, 0)
		if met != (nodeErr == nil) {
			t.Fatalf("recompute verdict %v != full node verdict %v (nodeErr=%v)", met, nodeErr == nil, nodeErr)
		}
		if met {
			t.Fatal("proposer alone (8<<20 of 27<<20 weight) must NOT meet the >⅔ quorum")
		}
	})
}

// TestRecomputeEpochWeightQuorum_ForgedWeightRejects is HARD ABLATION 1 (C-1): a witness with
// the RIGHT members but a FORGED per-member weight makes the recompute STALL — the forged
// weight's inclusion proof does not verify against the committed root. Proves C-1 closed the
// forgeable-tally hole: membership completeness alone (the digest) would have accepted it.
//
// RED-BEFORE-GREEN: the un-forged witness (TestRecomputeEpochWeightQuorum_MatchesFullNode) meets
// the quorum; forging one member's weight flips it to a stall.
func TestRecomputeEpochWeightQuorum_ForgedWeightRejects(t *testing.T) {
	f := buildRecomputeFixture(t)
	seen := f.seenAll()

	// Baseline: the honest witness meets the quorum (the green this ablation reddens).
	w := f.witnessFor(t)
	if met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w); !met || reason != nil {
		t.Fatalf("baseline should meet quorum with no stall; met=%v reason=%v", met, reason)
	}

	// THE INJECTED DEFECT: forge one member's claimed weight (inflate it) while KEEPING its
	// original inclusion proof. The proof was built for the TRUE weight, so Resolve against the
	// forged EncodeInt64(weight) fails ⇒ the member's weight is unproven ⇒ stall.
	victim := f.members[0]
	forged := w // shallow copy; MemberWeights map is shared, so clone the entry we mutate
	forged.MemberWeights = make(map[ports.NodeID]MemberWeightWitness, len(w.MemberWeights))
	for id, mw := range w.MemberWeights {
		forged.MemberWeights[id] = mw
	}
	orig := forged.MemberWeights[victim]
	forged.MemberWeights[victim] = MemberWeightWitness{Weight: orig.Weight + (100 << 20), Proof: orig.Proof}

	met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, forged)
	if met {
		t.Fatal("C-1 VIOLATION: a forged per-member weight was accepted — the tally is forgeable (the digest bound membership but not the weights)")
	}
	if !errors.Is(reason, ErrRecomputeMemberWeightUnproven) {
		t.Fatalf("forged weight should stall on ErrRecomputeMemberWeightUnproven; got %v", reason)
	}
}

// TestRecomputeEpochWeightQuorum_OmittedMemberRejects is HARD ABLATION 2: a witness MISSING a
// frozen member reconstructs a DIFFERENT MTH than the committed epochSetRoot ⇒ STALL. Proves
// set-completeness — a withholding prover cannot shrink the tally's denominator to force a
// quorum, because the digest binds the complete id-set.
//
// RED-BEFORE-GREEN: the complete witness meets the quorum; dropping a member flips it to a stall.
func TestRecomputeEpochWeightQuorum_OmittedMemberRejects(t *testing.T) {
	f := buildRecomputeFixture(t)
	seen := f.seenAll()

	if met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, f.witnessFor(t)); !met || reason != nil {
		t.Fatalf("baseline should meet quorum with no stall; met=%v reason=%v", met, reason)
	}

	// THE INJECTED DEFECT: drop one member from the witnessed id-list (and its weight proof). The
	// reconstructed nodeSetMTH over the short list differs from the committed epochSetRoot ⇒ the
	// set-completeness check fails ⇒ stall.
	w := f.witnessFor(t)
	dropped := f.members[len(f.members)-1]
	shortIDs := make([]ports.NodeID, 0, len(f.members)-1)
	for _, id := range f.members {
		if id != dropped {
			shortIDs = append(shortIDs, id)
		}
	}
	w.IDs = shortIDs
	delete(w.MemberWeights, dropped)

	met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
	if met {
		t.Fatal("SET-COMPLETENESS VIOLATION: a witness missing a frozen member was accepted — the tally denominator was shrunk undetected")
	}
	if !errors.Is(reason, ErrRecomputeSetIncomplete) {
		t.Fatalf("omitted member should stall on ErrRecomputeSetIncomplete; got %v", reason)
	}
}

// TestRecomputeEpochWeightQuorum_InjectedMemberRejects is the dual of the omission: INJECTING an
// extra id into the witnessed list also breaks set-completeness (a different MTH), so a prover
// cannot pad the set either. Same closure, opposite direction.
func TestRecomputeEpochWeightQuorum_InjectedMemberRejects(t *testing.T) {
	f := buildRecomputeFixture(t)
	seen := f.seenAll()

	w := f.witnessFor(t)
	extra := idOf(key(99999)) // not a frozen member
	w.IDs = append(append([]ports.NodeID(nil), f.members...), extra)
	// Give the extra a (bogus) weight witness so the code reaches the completeness check, not the
	// missing-weight-witness stall. Reuse any real proof — it will not matter; the MTH mismatch
	// fires first.
	w.MemberWeights[extra] = MemberWeightWitness{Weight: 1 << 20, Proof: w.MemberWeights[f.members[0]].Proof}

	met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
	if met {
		t.Fatal("SET-COMPLETENESS VIOLATION: a witness with an injected extra member was accepted")
	}
	if !errors.Is(reason, ErrRecomputeSetIncomplete) {
		t.Fatalf("injected member should stall on ErrRecomputeSetIncomplete; got %v", reason)
	}
}

// TestRecomputeEpochWeightQuorum_GenesisConfigFromOwnConfig is HARD ABLATION 3 (C-6): the
// recompute's verdict depends ONLY on the committed weights (own StateRoot) and the fixed ⅔
// consensus constant — never on a per-deployment config knob an attacker could shift. The test
// proves this by running the SAME witness against boxes with WIDELY DIFFERENT own MinBond
// configs (the C-6 genesis-config surface) and asserting the verdict is INVARIANT.
//
// TEETH (why the coalition is chosen to sit near the ⅔ knee): the coalition is the proposer +
// enough attesters that it JUST clears (or JUST misses) ⅔ of the FULL frozen weight. A
// config-sensitive fold that screened members by own MinBond would, at a high own MinBond, DROP
// members and SHRINK the denominator — flipping a JUST-missing coalition to JUST-clearing (or
// vice-versa). So a recompute that (wrongly) read MinBond into the fold would make these boxes
// DIVERGE. They do not: own config does not enter the fold, proving C-6. (This is the exact
// ablation that must go red if a config-sensitive screen is injected — verified in the builder
// report by injecting an own-MinBond screen and watching the boxes diverge.)
func TestRecomputeEpochWeightQuorum_GenesisConfigFromOwnConfig(t *testing.T) {
	f := buildRecomputeFixture(t)
	w := f.witnessFor(t)

	// The coalition is chosen so a config-sensitive fold would FLIP the verdict, giving the test
	// real teeth (verified by injection in the builder report). Weights (in 1<<20 units) are
	// 8,6,5,4,4; total = 27. Coalition = proposer(8) + the 5-member = 13. Over the FULL set:
	// 3*13 = 39 < 2*27 = 54, so it MISSES ⅔. A high-MinBond screen (5<<20) that WRONGLY dropped
	// the two 4-members from the tally would shrink total to 19: 3*13 = 39 > 2*19 = 38, so it
	// would CLEAR. So a config-sensitive fold makes low-box (miss) and high-box (clear) DIVERGE.
	// A correct C-6 fold ignores own MinBond, so both boxes MISS — invariant.
	heavy := membersByWeightDesc(f)
	if heavy[0] != f.proposer {
		t.Fatalf("fixture invariant: proposer must be the heaviest member; got heaviest=%x proposer=%x", heavy[0][:], f.proposer[:])
	}
	seen := map[ports.NodeID]bool{heavy[2]: true} // the 5<<20 member; coalition = 8 + 5 = 13<<20

	// Boxes with WIDELY different own MinBond: low (era4MinBond=1<<20, admits all) vs high (5<<20,
	// would drop the two 4<<20 members if the fold were config-sensitive). A correct C-6 fold
	// ignores both, so the verdict is invariant.
	lowBox := f.c
	highBox := New(Config{Quorum: 1, MinBond: 5 << 20, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 0, BondTTLBlocks: 64},
		func(ports.NodeID) int64 { return 0 })
	highBox.SetBondVerifier(objectiveVerify)

	metLow, rLow := lowBox.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
	metHigh, rHigh := highBox.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
	if rLow != nil || rHigh != nil {
		t.Fatalf("neither box should stall; rLow=%v rHigh=%v", rLow, rHigh)
	}
	if metLow != metHigh {
		t.Fatalf("C-6 VIOLATION: boxes with different own MinBond (1<<20 vs 5<<20) disagreed on the "+
			"SAME witness (low=%v high=%v) — the fold read own config into the tally; the verdict must "+
			"depend only on the committed weights + the fixed ⅔ constant", metLow, metHigh)
	}
	// Anchor (non-vacuity): the coalition genuinely MISSES ⅔ over the full set, so the invariance
	// is over a live near-knee verdict, not a trivial constant.
	if metLow {
		t.Fatal("fixture invariant: the chosen coalition (13<<20 of 27<<20) should MISS the ⅔ quorum")
	}
}

// membersByWeightDesc returns the fixture's frozen members sorted by committed weight descending
// (ties broken by id bytes for determinism), so a coalition can be chosen deterministically.
func membersByWeightDesc(f recomputeFixture) []ports.NodeID {
	out := append([]ports.NodeID(nil), f.members...)
	sortMembersByWeightDesc(out, f.weights)
	return out
}

// TestRecomputeEpochWeightQuorum_UnprovenDigestStalls proves the box stalls (never folds) when
// the committed epochSetRoot leaf itself cannot be proven present — e.g. a witness verified
// against the WRONG root. The digest is the anchor of set-completeness; without it the box has
// nothing to compare the reconstructed MTH to.
func TestRecomputeEpochWeightQuorum_UnprovenDigestStalls(t *testing.T) {
	f := buildRecomputeFixture(t)
	seen := f.seenAll()
	w := f.witnessFor(t)

	// Verify the honest witness against a WRONG root: every proof fails, starting with the digest.
	var wrongRoot ports.Hash
	wrongRoot[0] = 0xff
	met, reason := f.c.RecomputeEpochWeightQuorum(wrongRoot, f.proposer, seen, w)
	if met {
		t.Fatal("a witness verified against the wrong root must not meet the quorum")
	}
	if !errors.Is(reason, ErrRecomputeDigestRootUnproven) {
		t.Fatalf("wrong-root digest should stall on ErrRecomputeDigestRootUnproven; got %v", reason)
	}
}

// TestRecomputeEpochWeightQuorum_MissingMemberWeightWitnessStalls proves the box stalls when a
// completeness-verified member has no weight witness at all (the map entry is absent). The set
// reconstructs (digest matches) but a member's weight cannot be verified ⇒ stall, never fold a
// partial set as if the missing member weighed zero.
func TestRecomputeEpochWeightQuorum_MissingMemberWeightWitnessStalls(t *testing.T) {
	f := buildRecomputeFixture(t)
	seen := f.seenAll()
	w := f.witnessFor(t)
	delete(w.MemberWeights, f.members[0]) // keep IDs complete (digest matches) but drop a weight

	met, reason := f.c.RecomputeEpochWeightQuorum(f.root, f.proposer, seen, w)
	if met {
		t.Fatal("a member with no weight witness must stall the fold, not be folded as zero")
	}
	if !errors.Is(reason, ErrRecomputeMemberWeightUnproven) {
		t.Fatalf("missing weight witness should stall on ErrRecomputeMemberWeightUnproven; got %v", reason)
	}
}

// TestRecomputeEpochWeightQuorum_NeverFlipsWitnessValidateAccept pins the STOP boundary: this
// increment reproduces ONE predicate; it must NOT have flipped WitnessValidateV5 to Accept. A
// v5 block with a directive still returns IndeterminateTrustlessly (the gated recompute seam),
// so the box STILL never-Accepts.
func TestRecomputeEpochWeightQuorum_NeverFlipsWitnessValidateAccept(t *testing.T) {
	f := buildRecomputeFixture(t)
	got, _ := f.c.WitnessValidateV5(v5Block(3), f.root, RecoveryDirective{})
	if got == Accept {
		t.Fatal("STOP boundary violated: WitnessValidateV5 returned ACCEPT — the accept flip (#657) must wait until ALL predicates are reproduced")
	}
}
