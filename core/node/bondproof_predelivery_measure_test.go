package node

import (
	"bytes"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// MEASURING WHETHER PRE-DELIVERY IS ALREADY HAPPENING, AND WHETHER IT LINES UP.
//
// The critical-path measurement (bondproof_criticalpath_measure_test.go) answered
// three questions and pointed at one route: pre-deliver EVERY registration
// including the proposer's own, so the attester's queue covers 100% of sources,
// then relay the committed bytes by digest. Its second question measured
// RegisterBondReg in ISOLATION — minted over b.Prev at propose time, never
// submitted — and concluded the proposer's own reg reaches no attester.
//
// THAT IS TRUE OF THE FUNCTION AND IT IS NOT THE WHOLE PATH, which is why this
// file exists rather than the build going straight in. chainSyncTick calls
// SubmitBondRenewal on every sweep with no proposer exemption, so a validator that
// is about to propose has usually ALREADY broadcast a registration of its own. The
// route's premise is therefore not "nothing is pre-delivered" but something
// narrower and sharper: does the registration an attester was handed MATCH the one
// the proposal commits?
//
// It matches only if both are minted over the SAME prev. SubmitBondRenewal signs
// over the head at sweep time; the proposal is built after the reconcile settles,
// on whatever head that leaves. So the two coincide exactly when the head does not
// move in between — and a healthy chain moves its head constantly.
//
// These are assertions about what is true NOW. Neither is a gate on a fix; there is
// no fix yet. They exist so the numbers cannot rot while the shape is decided, and
// so the premise is re-checked rather than recalled.

// PREMISE 1 — THE REGISTRATION IS A DETERMINISTIC FUNCTION OF ITS PREV.
//
// Everything about digest relay rests on this. If minting twice over one prev
// produced two different byte strings, a digest committed by the proposer could
// never match the bytes an attester holds, and the whole route would be dead
// however the delivery was arranged.
//
// It is not obvious from the types. The answer runs a VDF and an ed25519 signature
// over a plot read, and a randomized proof or a randomized signature at any layer
// would break it silently — nothing else in the tree would go red.
func TestBondRegIsADeterministicFunctionOfItsPrev(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	p := nodes[2]
	p.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := p.chain.Head()

	first, ok1 := p.RegisterBondReg(head)
	second, ok2 := p.RegisterBondReg(head)
	if !ok1 || !ok2 {
		t.Fatalf("VACUOUS: RegisterBondReg returned ok=%v,%v — the node holds no bond, so nothing was compared. "+
			"Give it one before reading anything here.", ok1, ok2)
	}
	a, b := bondRegEncode(first), bondRegEncode(second)

	other := head
	other[0] ^= 0xff
	third, _ := p.RegisterBondReg(other)
	c := bondRegEncode(third)

	t.Logf("MEASURED — one %d-byte registration, minted twice over the same prev and once over another:", len(a))
	t.Logf("  same prev, two mints:  identical=%v", bytes.Equal(a, b))
	t.Logf("  different prev:        identical=%v", bytes.Equal(a, c))

	if !bytes.Equal(a, b) {
		t.Fatal("A REGISTRATION IS NOT A DETERMINISTIC FUNCTION OF ITS PREV. Two mints over one prev produced different " +
			"bytes, so a digest the proposer commits can never be matched against bytes an attester was handed, and " +
			"digest relay is not available at all. Find which layer became randomized — the VDF proof, the plot read, " +
			"or the signature — before pricing any delivery scheme.")
	}
	if bytes.Equal(a, c) {
		t.Fatal("two DIFFERENT prevs produced the SAME registration — the per-position nonce is not binding, which is a " +
			"replay hole and not a relay question. That is the finding; stop here.")
	}
}

// PREMISE 2 — SO THE QUESTION IS HEAD SKEW, AND THIS MEASURES IT.
//
// A registration submitted over head H and a registration proposed over head H'
// are the same bytes iff H == H'. This drives the two cases against the product's
// own mint and reports which bytes an attester would be able to reconstruct from.
//
// The result is the shape of the fix. If a proposal's prev equals the head its
// sweep submitted over, the existing submit IS the pre-delivery and what remains is
// only choosing the digest form per peer. If it does not, the proposer must mint
// over the prev it will actually build on and deliver THAT, which is a change to
// the order of the propose path rather than to its contents.
func TestASubmittedRegMatchesTheProposedOneOnlyWhenTheHeadHeldStill(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	attester, proposer := nodes[0], nodes[2]
	proposer.EnableBond(ids[2].Signer(), 2<<20)

	// What the sweep broadcasts: a registration over the head at submit time.
	sweepHead, _ := proposer.chain.Head()
	submitted, ok := proposer.RegisterBondReg(sweepHead)
	if !ok {
		t.Fatal("VACUOUS: the proposer minted no registration, so neither case below means anything")
	}
	attester.handleChain(proposer.ID(), ports.Message{
		Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(submitted),
	})
	queued := 0
	var held chain.BondReg
	for _, pr := range attester.pendingBondRegs {
		if pr.R.ValidatorID() == proposer.ID() {
			queued++
			held = pr.R
		}
	}
	if queued != 1 {
		t.Fatalf("PREMISE BROKEN: the proposer's submitted registration reached the attester's queue %d times, want 1. "+
			"Everything below reads that queue, so find out why before drawing a conclusion.", queued)
	}

	// CASE A — the head held still: the proposal's prev IS the head the sweep
	// signed over.
	sameHead, _ := proposer.RegisterBondReg(sweepHead)
	matchSame := bytes.Equal(bondRegEncode(sameHead), bondRegEncode(held))

	// CASE B — the head moved by one block between the submit and the propose,
	// which is the ordinary case on a live chain: chainSyncTick submits, then
	// proposes only after the reconcile settles.
	movedHead := sweepHead
	movedHead[0] ^= 0x01
	moved, _ := proposer.RegisterBondReg(movedHead)
	matchMoved := bytes.Equal(bondRegEncode(moved), bondRegEncode(held))

	t.Logf("MEASURED — can an attester reconstruct the proposal's registration from what it was handed?")
	t.Logf("  proposal built on the head the sweep submitted over:  reconstructable=%v", matchSame)
	t.Logf("  proposal built one head later (the ordinary case):    reconstructable=%v", matchMoved)
	t.Logf("  => pre-delivery is ALREADY happening; what it is not is ALIGNED")

	if !matchSame {
		t.Fatal("a registration proposed over the SAME prev the sweep submitted over does not match the queued bytes. " +
			"That contradicts the determinism premise above, so one of the two measurements is wrong; re-derive both " +
			"before building anything on either.")
	}
	if matchMoved {
		t.Fatal("a registration minted over a DIFFERENT prev matched the queued bytes. Then the prev does not reach the " +
			"registration at all, which is a replay hole: the same proof would satisfy every position. Stop and " +
			"re-read RegisterBondReg's nonce.")
	}
}

// PREMISE 3 — AND THE PROPOSER REALLY DOES SUBMIT, DRIVEN RATHER THAN READ.
//
// The two arms above rest on a claim about the SWEEP: chainSyncTick calls
// SubmitBondRenewal with no proposer exemption, so a validator about to propose has
// already broadcast a registration of its own. That is a claim about control flow,
// and reading control flow is how the earlier measurement came to say the
// proposer's own registration reaches nobody — true of RegisterBondReg in
// isolation, and not true of the path.
//
// So this drives it: give a validator a bond, run the renewal it would run, and
// count what leaves for the wire.
func TestAProposerBroadcastsItsOwnRegistrationBeforeItProposes(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	proposer := nodes[2]
	proposer.EnableBond(ids[2].Signer(), 2<<20)

	peers := make([]ports.NodeID, 0, len(ids))
	for i, id := range ids {
		if i != 2 {
			peers = append(peers, id.NodeID())
		}
	}

	due := proposer.chain.BondRenewalDue(proposer.ID())
	before := net.Stats.Kinds[ports.MsgSubmitBondReg]
	proposer.SubmitBondRenewal(peers)
	sent := net.Stats.Kinds[ports.MsgSubmitBondReg] - before

	t.Logf("MEASURED — what a bonded validator's own renewal puts on the wire:")
	t.Logf("  BondRenewalDue for itself: %v", due)
	t.Logf("  MsgSubmitBondReg sends to its %d peers: %d", len(peers), sent)
	t.Logf("  => the earlier reading — that a proposer's own registration reaches nobody — is TRUE of")
	t.Logf("     RegisterBondReg alone and FALSE of the sweep. The gap is alignment, not delivery.")

	if !due {
		t.Fatal("VACUOUS: the validator's renewal was not due, so the zero below would measure the gate rather than " +
			"the broadcast. Arrange a due renewal before reading this.")
	}
	if sent != len(peers) {
		t.Fatalf("a bonded validator with a due renewal broadcast its own registration to %d of %d peers. If it is ZERO, "+
			"the sweep really does exempt a proposer and the route's original reading stands — pre-delivery must then be "+
			"BUILT rather than aligned, and the two arms above are measuring a path that does not run.", sent, len(peers))
	}
}

// THE ALIGNMENT FIX, ASSERTED: THE BLOCK COMMITS THE BYTES THE PEERS WERE HANDED.
//
// The three measurements above locate the defect exactly. Pre-delivery already
// happens, a registration is deterministic in its prev, and the chain accepts one
// over the last K committed heads — so the only reason an attester could not
// reconstruct the proposal's registration was that the proposer threw away the copy
// it had already broadcast and minted a new one over a later head.
//
// This asserts the rule that replaces that: a proposer embeds the registration it
// SUBMITTED, so every attester holding the submitted copy holds the committed bytes.
// That is the precondition for relaying the registration by digest rather than by
// value; the relay itself is not built here, and this property is what it will rest
// on.
//
// The control is the half that makes it a fix rather than a cache: when the kept
// copy is no longer one the chain would accept, the proposer mints fresh rather than
// proposing a registration its own block check would reject.
func TestAProposerCommitsTheRegistrationItBroadcast(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	attester, proposer := nodes[0], nodes[2]
	proposer.EnableBond(ids[2].Signer(), 2<<20)

	peers := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[3].NodeID()}
	proposer.SubmitBondRenewal(peers)
	if proposer.ownBondReg == nil {
		t.Fatal("VACUOUS: the renewal broadcast kept no registration, so there is nothing for a proposal to reuse " +
			"and the assertion below would pass on a fresh mint by accident")
	}
	broadcast := bondRegEncode(*proposer.ownBondReg)

	// The attester receives the broadcast the way the wire delivers it.
	attester.handleChain(proposer.ID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: broadcast})
	var held []byte
	for _, pr := range attester.pendingBondRegs {
		if pr.R.ValidatorID() == proposer.ID() {
			held = bondRegEncode(pr.R)
		}
	}
	if held == nil {
		t.Fatal("PREMISE BROKEN: the broadcast registration never reached the attester's queue, so reconstructability " +
			"cannot be read from it")
	}

	// THE RULE: the block built on a LATER prev commits the bytes already delivered.
	// The prev is deliberately not the head the submit signed over — that is the
	// ordinary case, and the case that used to produce unreconstructable bytes.
	laterPrev, _ := proposer.chain.Head()
	laterPrev[31] ^= 0x01
	proposed, ok := proposer.ownRegForBlock(laterPrev)
	if !ok {
		t.Fatal("the proposer produced no registration for its block at all")
	}
	committed := bondRegEncode(proposed)

	t.Logf("MEASURED — can an attester reconstruct the registration the proposal commits?")
	t.Logf("  broadcast on the sweep:      %d B", len(broadcast))
	t.Logf("  committed by the proposal:   %d B", len(committed))
	t.Logf("  reconstructable from the attester's own queue: %v", bytes.Equal(committed, held))

	if !bytes.Equal(committed, held) {
		t.Fatal("the proposal commits a registration NO attester holds. The proposer minted fresh instead of reusing " +
			"the copy it broadcast, so every attester's queued copy is the wrong bytes for this block and the " +
			"registration can never be relayed by digest. Check ownRegForBlock still prefers ownBondReg, and that " +
			"SubmitBondRenewal still keeps what it sent.")
	}

	// CONTROL — a kept copy the chain would NOT accept must not be proposed. Without
	// this, "reuse whatever was kept" would eventually propose a stale registration
	// and burn the proposer's turn on a block its own validity check rejects.
	stale := *proposer.ownBondReg
	stale.Size = 0 // a registration no ValidateBondReg accepts
	proposer.ownBondReg = &stale
	fresh, ok := proposer.ownRegForBlock(laterPrev)
	if !ok {
		t.Fatal("CONTROL BROKEN: with an unacceptable kept copy the proposer produced nothing at all — it must fall " +
			"back to a fresh mint, not give up its turn")
	}
	if bytes.Equal(bondRegEncode(fresh), bondRegEncode(stale)) {
		t.Fatal("CONTROL BROKEN: the proposer reused a registration the chain would refuse. Reuse must be gated on " +
			"ValidateBondReg — the same gate the block itself faces — or a kept copy outlives the head window and " +
			"the proposal is rejected by every replica.")
	}
	t.Log("control: a kept copy the chain would refuse is replaced by a fresh mint, so reuse never outlives the window")
}

// AND WHAT MINTING ONCE SAVES ON THE LOOP, measured rather than implied.
//
// A fresh registration is not a lookup. It reads the seed block out of the plot,
// proves that leaf, evaluates the VDF the space-time proof is built on, and signs
// the result — on the node's single serialized loop (B2), inside the propose path.
// Re-minting when a valid registration is already in hand spends that twice for one
// claim.
//
// The figure below is this rig's VDF delay, not the shipped one, so read it as the
// SHAPE rather than as the field cost: the saving is one whole space-time answer per
// proposal, whatever the delay is set to.
func TestReusingTheBroadcastRegistrationSavesAWholeMint(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	p := nodes[2]
	p.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := p.chain.Head()

	const runs = 5
	start := time.Now()
	for i := 0; i < runs; i++ {
		if _, ok := p.RegisterBondReg(head); !ok {
			t.Fatal("VACUOUS: the node minted nothing, so the timing below is of a failed call")
		}
	}
	mint := time.Since(start) / runs

	p.SubmitBondRenewal([]ports.NodeID{ids[0].NodeID()})
	if p.ownBondReg == nil {
		t.Fatal("VACUOUS: nothing was kept to reuse")
	}
	start = time.Now()
	for i := 0; i < runs; i++ {
		if _, ok := p.ownRegForBlock(head); !ok {
			t.Fatal("the reuse path produced nothing")
		}
	}
	reuse := time.Since(start) / runs

	t.Logf("PROPOSE-PATH COST of this node's own registration (VDF delay %d, %d label samples):",
		p.cfg.BondVDFDelay, p.cfg.BondLabelSamples)
	t.Logf("  fresh mint (what the propose path used to do): %v", mint)
	t.Logf("  reuse the broadcast copy:                      %v", reuse)
	t.Logf("  => one whole space-time answer, off the single serialized loop, per proposal")

	// The assertion is about the SHAPE, not the clock: reuse must not be doing the
	// work again. A threshold rather than a ratio, because a rig this fast makes any
	// ratio noise-dominated.
	if reuse >= mint {
		t.Fatalf("reusing the broadcast registration cost %v against a fresh mint's %v — reuse is not saving the "+
			"space-time answer, so ownRegForBlock is minting anyway and the propose path still pays twice", reuse, mint)
	}
}
