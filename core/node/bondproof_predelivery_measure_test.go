package node

import (
	"bytes"
	"testing"

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
