package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// R-BOX-ATTESTS — the LastCommit attestation carrier (owner call O1, ratified 2026-09-03)
// =============================================================================
//
// THE HAZARD (converged verdict §2.3). apply() writes validatorsSeen from b.Atts
// (chain.go:3293-3298) but Hash() excludes Atts (chain.go:656), and the era-3/era-4 root
// predicate re-runs the real apply over the ATTACHED certificate (era3validity.go:117-138,
// :148-160). A proposer populates its roots BEFORE it gathers (chainrole.go:870-884), so any
// certificate that would seat a NEW attester makes the recomputed root differ from the signed
// one and EVERY replica rejects that block. Consequence (a): validatorsSeen freezes at the
// pre-activation set, permanently ceilinging MatureCoefficient. Consequence (b): the height
// stalls for any round whose first-to-quorum prefix carries a never-seen qualified attester.
//
// THE FIX. Seat from a HASH-COVERED carrier: block h+1 republishes block h's precommits in
// LastCommit, which is folded into Hash(), so the proposer holds the seating bytes before it
// signs its roots. The seat lands one block late (monotone, disclosed).
//
// These are gates G1–G4, G6a, G6b and G9 of the verdict §11 table, at the tiers it names.
// G5 (compile-time rollout gate) is in this file too. G2 (node tier) is in
// core/node/lastcommit_carrier_node_test.go. G7 belongs to floor-box entry Part B. G8 (the box
// class-A re-point) is in floorbox_recompute_stateroot_atts_v5_test.go.

// carrierFor builds the LastCommit carrier for a block whose parent is c's head, the way an
// honest proposer does: every precommit it holds over the parent hash.
func carrierFor(t *testing.T, c *Chain) []Attestation {
	t.Helper()
	return c.HeadCarrier()
}

// mintNext4WithCarrier is mintNext4 plus the honest producer step O1 adds: a v5 block carries
// the parent's precommits in LastCommit, set BEFORE the roots are populated (the roots must
// cover the carrier's transition effect) and therefore before the block is signed.
func mintNext4Carrier(t *testing.T, c *Chain, keys []ed25519.PrivateKey, regs ...BondReg) *Block {
	t.Helper()
	prev, next := c.Head()
	b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}, BondRegs: regs}
	switch mv := c.MintVersion(next); {
	case mv >= BlockVersionWitnessable:
		b.LastCommit = carrierFor(t, c)
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

// TestG1_CarrierSeatsUnseenAttestersOneBlockLate is GATE G1 (cold chain, objective arm).
//
// RED at d7e4df0: era4AnchorChain(t, 1, 1) mints a v5 block at height 1 and signs it with all
// four anchor keys. Three of them have never been seen, so the attached certificate's apply
// writes three validatorsSeen leaves the pre-gather root does not contain, and Append fails
// with ErrEra3StateRootMismatch.
//
// GREEN on the carrier: height 1 commits (its own Atts write nothing), height 2 carries the
// three non-proposer precommits of height 1 in LastCommit and seats them — Regime().
// ValidatorsSeen == 3 after height 2, and 0 after height 1 (the disclosed one-block lag).
func TestG1_CarrierSeatsUnseenAttestersOneBlockLate(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)

	h1 := mintNext4Carrier(t, c, keys)
	if h1.Version != BlockVersionWitnessable {
		t.Fatalf("height 1 must mint v5, got v%d", h1.Version)
	}
	if len(h1.LastCommit) != 0 {
		t.Fatalf("height 1's carrier is EMPTY BY RULE (O1) — got %d entries", len(h1.LastCommit))
	}
	if err := c.Append(*h1); err != nil {
		t.Fatalf("G1 RED: height-1 v5 block with a four-key certificate was REJECTED: %v", err)
	}
	if got := c.Regime().ValidatorsSeen; got != 0 {
		t.Fatalf("after height 1 the seat has not landed yet (one-block lag): want 0 seen, got %d", got)
	}

	h2 := mintNext4Carrier(t, c, keys)
	if len(h2.LastCommit) != 4 {
		t.Fatalf("height 2 must carry every precommit the proposer holds for height 1 (4), got %d", len(h2.LastCommit))
	}
	if err := c.Append(*h2); err != nil {
		t.Fatalf("height-2 carrier block rejected: %v", err)
	}
	if got := c.Regime().ValidatorsSeen; got != 3 {
		t.Fatalf("the carrier must seat the THREE non-proposer attesters of height 1 (the parent's own "+
			"proposer is excluded by rule): want 3, got %d", got)
	}
}

// TestG9_StubAttsDoNotMoveTheRecomputedRoot is GATE G9 (cold chain).
//
// RED at d7e4df0: ValidateProposal checks no attestation signature, and validateEra3Roots folds
// whatever b.Atts the proposal bytes carry (era3validity.go:117-138) — so a proposal carrying
// UNSIGNED stub Atts moves the recomputed root. GREEN on the carrier: a v5 block's Atts no
// longer feed apply at all, so stub Atts cannot move the root. (O1 pins the first of G9's two
// alternatives: "Atts in proposal bytes do not move the recomputed root".)
func TestG9_StubAttsDoNotMoveTheRecomputedRoot(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)
	mustAppend(t, c, mintNext4Carrier(t, c, keys))

	prev, next := c.Head()
	clean := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	clean.LastCommit = c.HeadCarrier()
	if err := c.PopulateEra4Roots(clean); err != nil {
		t.Fatalf("populate: %v", err)
	}

	// The same block with a fifth, never-seen identity's UNSIGNED stub attestation bolted on.
	stub := *clean
	stub.Atts = []Attestation{{PubKey: []byte(key(59901).Public().(ed25519.PublicKey)), Sig: make([]byte, ed25519.SignatureSize), Phase: PhasePrecommit}}
	sr, _, err := c.postApplyRoots(stub)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}
	if sr != *clean.StateRoot {
		t.Fatalf("G9 RED: stub Atts MOVED the recomputed state root (%x != %x) — a proposal's "+
			"uncovered, unverified attestation bytes still feed the committed transition", sr, *clean.StateRoot)
	}
}

// TestCarrierValidityRules pins the O1 validity rule on the cold chain: a sub-v5 block carrying
// the field is INVALID; height 1's carrier is empty BY RULE; entries must verify over b.Prev at
// PhasePrecommit; ids are distinct.
func TestCarrierValidityRules(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // height 1, v5, empty carrier

	build := func(mut func(*Block)) *Block {
		prev, next := c.Head()
		b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
		b.LastCommit = c.HeadCarrier()
		mut(b)
		if b.Version == 0 {
			if err := c.PopulateEra4Roots(b); err != nil {
				t.Fatalf("populate: %v", err)
			}
		}
		twoPhaseSign(b, keys)
		return b
	}

	t.Run("sub-v5 carrying the field is invalid", func(t *testing.T) {
		// A chain still in the era-3 window (H_era3=2, H_era4=9), so a v4 block is legal
		// there and the ONLY thing wrong with it is that it carries the v5-only field.
		c3, keys3 := era4AnchorChain(t, 2, 9)
		mustAppend(t, c3, mintNext4(t, c3, keys3)) // height 1, v2 — seats the three non-proposers
		prev, next := c3.Head()
		head := c3.Blocks(1)[0]
		b := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
		b.LastCommit = []Attestation{AttestAt(&head, keys3[1], 0, PhasePrecommit)}
		if err := c3.PopulateEra3Roots(b); err != nil {
			t.Fatalf("populate: %v", err)
		}
		twoPhaseSign(b, keys3)
		if b.Version != BlockVersionStateRoot {
			t.Fatalf("fixture must mint v4, got v%d", b.Version)
		}
		if err := c3.ValidateProposal(b); !errors.Is(err, ErrCarrierNotWitnessable) {
			t.Fatalf("want ErrCarrierNotWitnessable, got %v", err)
		}
	})
	t.Run("wrong-phase entry is invalid", func(t *testing.T) {
		b := build(func(b *Block) { b.LastCommit[0].Phase = PhasePrepare })
		if err := c.ValidateProposal(b); !errors.Is(err, ErrCarrierBadSignature) {
			t.Fatalf("want ErrCarrierBadSignature, got %v", err)
		}
	})
	t.Run("entry over the wrong block is invalid", func(t *testing.T) {
		b := build(func(b *Block) {
			// A genuine precommit at the same round over a DIFFERENT hash.
			other := &Block{Height: 99, Entries: []ports.Entry{entry(9)}}
			b.LastCommit[0] = AttestAt(other, keys[1], 0, PhasePrecommit)
		})
		if err := c.ValidateProposal(b); !errors.Is(err, ErrCarrierBadSignature) {
			t.Fatalf("want ErrCarrierBadSignature, got %v", err)
		}
	})
	t.Run("duplicate ids are invalid", func(t *testing.T) {
		b := build(func(b *Block) { b.LastCommit = append(b.LastCommit, b.LastCommit[0]) })
		if err := c.ValidateProposal(b); !errors.Is(err, ErrCarrierDuplicateID) {
			t.Fatalf("want ErrCarrierDuplicateID, got %v", err)
		}
	})
	t.Run("any round verifies", func(t *testing.T) {
		// O1: the rule binds to PhasePrecommit over b.Prev, NOT to CommitRound. A genuine
		// precommit at round 7 is a valid carrier entry.
		prev, _ := c.Head()
		head := c.Blocks(1)[0]
		if head.Hash() != prev {
			t.Fatal("head lookup")
		}
		b := build(func(b *Block) { b.LastCommit[0] = AttestAt(&head, keys[0], 7, PhasePrecommit) })
		if err := c.ValidateProposal(b); err != nil {
			t.Fatalf("a genuine round-7 precommit over the parent must be a valid carrier entry: %v", err)
		}
	})
	t.Run("height 1 carrier must be empty", func(t *testing.T) {
		c2, keys2 := era4AnchorChain(t, 1, 1)
		g := c2.Blocks(0)[0]
		b := &Block{Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
		b.LastCommit = []Attestation{AttestAt(&g, keys2[1], 0, PhasePrecommit)}
		if err := c2.PopulateEra4Roots(b); err != nil {
			t.Fatalf("populate: %v", err)
		}
		twoPhaseSign(b, keys2)
		if err := c2.ValidateProposal(b); !errors.Is(err, ErrCarrierAtHeightOne) {
			t.Fatalf("want ErrCarrierAtHeightOne, got %v", err)
		}
	})
}

// carrierEntry builds the LastCommit entry a proposer at c's head attaches for signer k: a genuine
// PhasePrecommit over the HEAD (= parent) block. It is the test-side twin of HeadCarrier for
// fixtures that mint a signer's carried precommit directly.
func carrierEntry(c *Chain, k ed25519.PrivateKey) Attestation {
	head, _ := c.headBlock()
	return AttestAt(&head, k, 0, PhasePrecommit)
}

// =============================================================================
// R-CARRIER-PARENTPROPOSER — the two driven FIX gates for the parent-proposer anchor
// =============================================================================
//
// The carrier transition excludes id == parent.ProposerID(). The box holds no parent block and
// the parent's proposer identity is not a committed leaf, so the witness carries it and the box
// ANCHORS it against the hash-covered b.Prev with the parent's own proposer signature
// (carrierParentProposerFromWitness). These are the driven gates the coverage table's two FIX
// rows name. Both are ADVERSARIAL-ROOT gates: the attacker also controls the committed root, so
// "the fold happens to mismatch" is not the defence — the STALL is.

// TestAdversarialRoot_ClassA_ForgedParentProposer: a ParentProposer naming a key that did NOT
// sign b.Prev must STALL. Without the anchor the box would skip whichever id the witness named
// (dropping a real seat) or, naming nobody, seat the parent's proposer — a one-seat forgery in
// either direction against an attacker-chosen root.
func TestAdversarialRoot_ClassA_ForgedParentProposer(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)

	// A key that never signed b.Prev, with a signature it did make over its own message.
	forged := key(54999)
	w.ParentProposer = pubOf(forged)
	w.ParentProposerSig = ed25519.Sign(forged, []byte("not the parent hash"))

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatal("GATE FAILED (ForgedParentProposer): the box accepted a parent-proposer witness whose " +
			"signature does not verify over b.Prev — the carrier's exclusion is then witness-chosen")
	}
	if !errors.Is(err, ErrRecomputeStateRootDigest) {
		t.Fatalf("want ErrRecomputeStateRootDigest, got %v", err)
	}
	t.Logf("GATE GREEN: forged ParentProposer STALLS: %v", err)
}

// TestAdversarialRoot_ClassA_MissingParentProposerSig: an omitted signature must STALL, never
// fall through to "no exclusion" (the C-7 §104 banned move — a missing proof is never read as a
// false/absent value).
func TestAdversarialRoot_ClassA_MissingParentProposerSig(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)
	w.ParentProposerSig = nil

	err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w)
	if err == nil {
		t.Fatal("GATE FAILED (MissingParentProposerSig): the box folded class A with an UNANCHORED " +
			"parent-proposer claim")
	}
	if !errors.Is(err, ErrRecomputeStateRootDigest) {
		t.Fatalf("want ErrRecomputeStateRootDigest, got %v", err)
	}
	t.Logf("GATE GREEN: missing ParentProposerSig STALLS: %v", err)
}

// TestG8_BoxIsBlindToTheBlocksOwnAtts is GATE G8's second arm (cold box): a served copy of the
// same committed block carrying a DIFFERENT but valid certificate (the S5 same-round superset)
// no longer moves the box verdict, because the class-A derivation reads the hash-covered carrier.
// RED before the re-point: the derivation read b.Atts, so the superset variant made the box
// Reject a canonical block.
func TestG8_BoxIsBlindToTheBlocksOwnAtts(t *testing.T) {
	f := buildAttFixture(t)
	b := f.attBlock()
	committed := f.applyAndCommittedRoot(t, b)
	w := f.witnessForAtt(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, b, w); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	// The SAME block as a peer served it: a superset certificate in the uncovered Atts slot.
	variant := b
	variant.Atts = []Attestation{carrierEntry(f.c, f.proposer), carrierEntry(f.c, f.att), carrierEntry(f.c, key(54777))}
	if variant.Hash() != b.Hash() {
		t.Fatal("the Atts slot must not be hash-covered — the S5 variant is the same block")
	}
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, committed, variant, w); err != nil {
		t.Fatalf("G8 RED: a same-hash served variant with a different certificate moved the box verdict: %v", err)
	}
}

// TestG3_ServedVariantDeterminism is GATE G3 (cold chain): a peer may legitimately serve a
// same-hash copy of a committed block carrying a DIFFERENT valid certificate. Both variants must
// ACCEPT with identical StateRoot, and the child must seat the same signers from its hash-covered
// carrier regardless of which copy the replica held.
//
// RED at d7e4df0: apply seated from b.Atts, so the superset variant recomputed a different root
// and a fresh replica REJECTED a block whose certificate was valid (shape S5).
func TestG3_ServedVariantDeterminism(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)
	h1 := mintNext4Carrier(t, c, keys)

	// Variant A: the certificate as minted. Variant B: the SAME block with a same-round genuine
	// precommit from a fifth qualified identity appended, plus a rewritten CommitRound.
	// Variant A: the certificate as minted (round 0).
	// Variant B: a genuine SAME-ROUND precommit from a fifth qualified identity appended (the S5
	// same-round superset).
	// Variant C: an entirely DIFFERENT-round but valid (PrepareQC_r', Atts_r') pair with
	// CommitRound rewritten to r' — legal because CommitRound is uncovered by Hash().
	fifth := key(58001)
	varA := *h1
	varB := *h1
	varB.Atts = append(append([]Attestation(nil), h1.Atts...), AttestAt(h1, fifth, 0, PhasePrecommit))
	varC := *h1
	varC.CommitRound = 3
	varC.PrepareQC = nil
	varC.Atts = nil
	for _, k := range keys {
		varC.PrepareQC = append(varC.PrepareQC, AttestAt(h1, k, 3, PhasePrepare))
		varC.Atts = append(varC.Atts, AttestAt(h1, k, 3, PhasePrecommit))
	}
	if varA.Hash() != varB.Hash() || varA.Hash() != varC.Hash() {
		t.Fatal("G3 premise broken: Atts / PrepareQC / CommitRound must be OUTSIDE Hash() — the variants are the same block")
	}

	cA, _ := era4AnchorChain(t, 1, 1)
	cB, _ := era4AnchorChain(t, 1, 1)
	cC, _ := era4AnchorChain(t, 1, 1)
	if err := cA.Append(varA); err != nil {
		t.Fatalf("variant A rejected: %v", err)
	}
	if err := cB.Append(varB); err != nil {
		t.Fatalf("G3 RED (same-round superset): a replica holding a DIFFERENT but valid certificate "+
			"for the same block rejected a canonical block: %v", err)
	}
	if err := cC.Append(varC); err != nil {
		t.Fatalf("G3 RED (different-round pair): a replica holding a valid round-3 certificate pair "+
			"with a rewritten CommitRound rejected a canonical block: %v", err)
	}
	rA, _ := cA.Head()
	rB, _ := cB.Head()
	rC, _ := cC.Head()
	if rA != rB || rA != rC {
		t.Fatal("the replicas committed different heads for the same block")
	}
	if *cA.Blocks(1)[0].StateRoot != *cB.Blocks(1)[0].StateRoot || *cA.Blocks(1)[0].StateRoot != *cC.Blocks(1)[0].StateRoot {
		t.Fatal("G3 RED: the replicas computed different committed StateRoots")
	}
	if cA.Regime().ValidatorsSeen != cB.Regime().ValidatorsSeen || cA.Regime().ValidatorsSeen != cC.Regime().ValidatorsSeen {
		t.Fatal("G3 RED: the replicas seated different attester sets")
	}

	// THE CHILD. One child block, built by replica A, appended to ALL THREE replicas: each
	// seats exactly the signers its HASH-COVERED carrier names, regardless of which copy of the
	// parent's certificate that replica happens to hold. (The children each replica would MINT
	// legitimately differ — the carrier source is the replica's own stored certificate — but
	// once a child is minted its carrier is signed content and every replica agrees on it.)
	child := mintNext4Carrier(t, cA, keys)
	for name, ch := range map[string]*Chain{"A": cA, "B": cB, "C": cC} {
		if err := ch.Append(*child); err != nil {
			t.Fatalf("G3 RED: replica %s rejected the child carrier block: %v", name, err)
		}
		if got := ch.Regime().ValidatorsSeen; got != 3 {
			t.Fatalf("G3 RED: replica %s seated %d, want 3 — the seat depends on which certificate "+
				"copy the replica held", name, got)
		}
	}
}

// TestG4_NewOperatorsRaiseTheCoefficient is GATE G4 (cold chain, objective arm).
//
// The verdict is explicit that this asserts a CEILING, not monotonicity: a seated member may
// lapse (TTL) and re-bond and be re-counted, so a monotone assertion is wrong and would pass
// vacuously. What was broken is that NO operator joining after activation could EVER be counted.
//
// RED at d7e4df0: a validator bonded after activation attests, its block is rejected, and if the
// proposer trims it the chain commits but the operator is never seated — C2Metric().Participants
// never rises. GREEN: the new operator is counted within the stated 2-height bound.
func TestG4_NewOperatorsRaiseTheCoefficient(t *testing.T) {
	c, keys := era4AnchorChain(t, 1, 1)
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // h1
	mustAppend(t, c, mintNext4Carrier(t, c, keys)) // h2 — seats the three founding attesters
	before := c.Regime().ValidatorsSeen
	if before != 3 {
		t.Fatalf("setup: want 3 seated, got %d", before)
	}

	// A NEW operator bonds after activation.
	joiner := key(58101)
	prev, _ := c.Head()
	reg := NewBondReg(joiner, ports.HashBytes(pubOf(joiner)), twoMiB, []byte("valid"), prev, 9)
	mustAppend(t, c, mintNext4Carrier(t, c, keys, reg)) // h3: the reg commits

	// h4: the joiner attests; h5 carries its precommit and seats it. Two heights, as stated.
	h4 := mintNext4Carrier(t, c, append(append([]ed25519.PrivateKey(nil), keys...), joiner))
	mustAppend(t, c, h4)
	h5 := mintNext4Carrier(t, c, keys)
	mustAppend(t, c, h5)

	jid := ports.HashBytes(pubOf(joiner))
	if !c.validatorsSeen[jid] {
		t.Fatal("G4 RED: an operator that joined AFTER activation was never seated — the measurement " +
			"is frozen and MatureCoefficient is permanently ceilinged by the pre-activation set")
	}
	if got := c.Regime().ValidatorsSeen; got <= before {
		t.Fatalf("G4 RED: the seated set did not grow: %d -> %d", before, got)
	}
	if got := c.C2Metric().Participants; got == 0 {
		t.Fatal("G4 RED: C2Metric counts no participants — the CT-1 arrival-rate alarm reads zero forever")
	}
}

// TestG5_StampFiveImpliesTheCarrierIsHashCovered is GATE G5 (unit). It is VACUOUSLY GREEN today,
// by design: the readiness stamp is 3 and this round does NOT raise it (owner call O2 is not
// ratified here). The gate is the compile-time rollout condition the stamp-raising release must
// satisfy — a binary that stamps 5 MUST cover the carrier in Hash(), and NO code path stamps 4.
func TestG5_StampFiveImpliesTheCarrierIsHashCovered(t *testing.T) {
	k := key(58201)
	r := NewBondReg(k, ports.HashBytes(pubOf(k)), twoMiB, []byte("valid"), ports.Hash{}, 1)

	if r.Version == BlockVersionStateRoot {
		t.Fatalf("ROLLOUT RULE VIOLATED (O2): no release ever stamps %d. The stamp goes 3 → 5 "+
			"directly, so era-3 and era-4 lock in at the SAME rotation and no v4 window opens.",
			BlockVersionStateRoot)
	}
	if r.Version < BlockVersionWitnessable {
		t.Logf("G5 vacuous by design: the stamp is %d, not %d — this round does not raise it.",
			r.Version, BlockVersionWitnessable)
		return
	}
	// The stamp HAS been raised. The carrier must be hash-covered, or a v5 network mints blocks
	// whose seating transition is not signed.
	a := Block{Version: BlockVersionWitnessable, Height: 2, Entries: []ports.Entry{entry(1)}}
	b := a
	b.LastCommit = []Attestation{{PubKey: pubOf(k), Sig: make([]byte, 64), Phase: PhasePrecommit}}
	if a.Hash() == b.Hash() {
		t.Fatal("ROLLOUT GATE (G5) RED: this binary stamps 5 but Hash() does NOT cover LastCommit — " +
			"the v5 seating transition would ride on unsigned bytes (R-BOX-ATTESTS)")
	}
}

// TestG6a_NoV4WindowOnTheTallyPath is GATE G6a (cold chain). With every reg stamped 5, BOTH
// activation tallies clear at the SAME rotation, so era3Height == era4Height and the first
// root-checked block is v5 — no block is ever minted at v4.
//
// The ablation is the forbidden case, and the test DOCUMENTS it as forbidden rather than
// accepting it: stamping an epoch of regs at 4 first opens a v4 window.
func TestG6a_NoV4WindowOnTheTallyPath(t *testing.T) {
	whale := key(58301)
	minnows := []ed25519.PrivateKey{key(58302), key(58303), key(58304)}
	build := func(stamp uint8) *Chain {
		cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 4, BondTTLBlocks: 64}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = append(g.BondRegs, bondRegV(whale, twoMiB, ports.Hash{}, stamp))
		for _, m := range minnows {
			g.BondRegs = append(g.BondRegs, bondRegV(m, twoMiB, ports.Hash{}, stamp))
		}
		Sign(g, whale)
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		return c
	}

	// THE ROLLOUT RULE (O2): every reg stamped 5. Both tallies clear at the SAME rotation.
	c := build(BlockVersionWitnessable)
	for c.Len() < 4 {
		commit(t, c, whale, minnows)
	}
	if !c.era3LockedIn || !c.era4LockedIn {
		t.Fatalf("both tallies must lock in at the first rotation (era3 %v, era4 %v)", c.era3LockedIn, c.era4LockedIn)
	}
	if c.era3Height != c.era4Height {
		t.Fatalf("G6a RED: a v4 WINDOW opened — H_era3 %d != H_era4 %d. With every reg stamped 5 "+
			"both tallies clear at the SAME rotation; a differing pair means some reg stamped 4, "+
			"which the rollout rule (O2) FORBIDS.", c.era3Height, c.era4Height)
	}
	// The first root-checked block mints v5, never v4 — and every block committed past the
	// boundary is v5.
	if v := c.MintVersion(c.era4Height); v != BlockVersionWitnessable {
		t.Fatalf("G6a RED: the first root-checked block (height %d) mints v%d, not v5 — a v4 window", c.era4Height, v)
	}
	for c.Len() <= int(c.era4Height)+2 {
		mustAppend(t, c, mintNext4Carrier(t, c, append([]ed25519.PrivateKey{whale}, minnows...)))
	}
	for _, b := range c.Blocks(0) {
		if b.Version == BlockVersionStateRoot {
			t.Fatalf("G6a RED: height %d minted a v4 block. Under the rollout rule NO release ever "+
				"stamps 4, so no v4 block is ever minted — era-3 is frozen-and-retired-UNRUN.", b.Height)
		}
	}

	// THE FORBIDDEN CASE, documented as forbidden rather than accepted: stamping the regs at 4
	// clears the era-3 tally alone and opens a v4 window. This is the ablation that proves the
	// gate above has teeth; it is NOT an accepted configuration.
	bad := build(BlockVersionStateRoot)
	for bad.Len() < 4 {
		commit(t, bad, whale, minnows)
	}
	if !bad.era3LockedIn || bad.era4LockedIn {
		t.Fatalf("ABLATION SETUP: a stamp-4 fleet must lock era-3 ONLY (era3 %v, era4 %v)", bad.era3LockedIn, bad.era4LockedIn)
	}
	if v := bad.MintVersion(bad.era3Height); v != BlockVersionStateRoot {
		t.Fatalf("ABLATION: a stamp-4 fleet must mint v4 at H_era3, got v%d", v)
	}
	t.Logf("ABLATION (FORBIDDEN, not accepted): stamping regs at %d opens a v4 window at height %d "+
		"with era-4 unlocked. The O2 rollout rule is that no release ever stamps 4.",
		BlockVersionStateRoot, bad.era3Height)
}

// TestG6b_OverrideActivationIgnoresTheStamp is GATE G6b (cold chain / config), NEW in the
// converged verdict. The pre-latch genesis override does NOT consult regVersion at all, so the
// 3 → 5 stamp rule protects the TALLY path only. This gate makes the exposure VISIBLE and
// RED-on-regression: the rollout rule (O2) must read "no mainnet era activation, by tally OR by
// pre-latch genesis override, on a binary without the carrier."
//
// It also pins that setting Era3ActivationHeight WITHOUT Era4ActivationHeight opens a v4 window,
// and that New's layering assertion does NOT prevent it (that assertion fires only when BOTH are
// set and era4 < era3).
func TestG6b_OverrideActivationIgnoresTheStamp(t *testing.T) {
	// Era-3 overridden, era-4 left at 0 ⇒ a v4 WINDOW. New does not panic.
	c, keys := era4AnchorChain(t, 1, 0)
	if v := c.MintVersion(1); v != BlockVersionStateRoot {
		t.Fatalf("G6b: era3-only override must mint v4 at height 1, got v%d", v)
	}
	if c.era4Active(1) {
		t.Fatal("G6b: era-4 must NOT be active with Era4ActivationHeight unset and no tally")
	}
	// Every reg in this fixture is stamped BELOW 4 (bondReg leaves the default), and the override
	// activates anyway — that is the whole point: the override bypasses regVersion.
	for _, r := range c.Blocks(0)[0].BondRegs {
		if r.Version >= BlockVersionStateRoot {
			t.Fatalf("G6b VACUOUS: the fixture's regs must be stamped below 4 for the bypass to be visible (got %d)", r.Version)
		}
	}
	if !c.era3Active(1) {
		t.Fatal("G6b RED: the pre-latch override did not activate era-3 — if this ever starts " +
			"consulting regVersion, the O2 rollout rule's second clause can be narrowed")
	}
	_ = keys

	// And with BOTH overrides equal there is no window: era-4 is active at the same height.
	c2, _ := era4AnchorChain(t, 1, 1)
	if !c2.era3Active(1) || !c2.era4Active(1) {
		t.Fatal("G6b: with both overrides at 1 both eras must be active at height 1")
	}
	if v := c2.MintVersion(1); v != BlockVersionWitnessable {
		t.Fatalf("G6b: with both overrides at 1 the first block must mint v5, got v%d", v)
	}
}
