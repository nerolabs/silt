package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// THE LAST CAUSE OF THE WEDGE IS AN EVIDENCE GAP, NOT A DELIVERY GAP.
//
// Every validator already HOLDS every peer's registration: a renewal is broadcast
// to the whole set and each receiver verifies and queues it. What no proposer could
// see was that fact. peerCanReconstruct had exactly two evidences — the peer
// AUTHORED the registration, or it ACKNOWLEDGED this node's OWN — and on a live
// chain renewals are staggered, so a block carries one OTHER validator's
// registration and only its author qualified. Measured in the field on run
// 7eaf3bd-75421: 9 of 24 registration-bearing prepare legs shed, 15 carried
// ~1,574,000 B each, and the chain wedged at 814 s against a 220 s bound.
//
// The third evidence closes it: the HOLDER reports its own answer-digests on the
// head probe that already crosses between every pair of nodes on every sweep.

// THE PROPERTY: a peer that reports holding a registration is shed to, even though
// this node neither authored it nor was acknowledged for it.
func TestAProposerShedsToAPeerThatReportedHoldingAThirdPartysRegistration(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	proposer, author, other := nodes[0], nodes[2], ids[1].NodeID()
	author.EnableBond(ids[2].Signer(), 2<<20)

	head, _ := author.chain.Head()
	reg, ok := author.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: the author minted no registration")
	}
	b := blockCarrying(t, proposer, reg)

	// BEFORE any report: only the author qualifies. This is the field's 1-of-n-1.
	if !proposer.peerCanReconstruct(ids[2].NodeID(), b) {
		t.Fatal("PREMISE BROKEN: the registration's own AUTHOR must always qualify")
	}
	if proposer.peerCanReconstruct(other, b) {
		t.Fatal("PREMISE BROKEN: a peer that has reported nothing must NOT qualify, or the assertion below " +
			"passes without the report doing anything")
	}

	// The peer reports its queue on its next head reply.
	proposer.noteHeldRegs(other, []ports.Hash{*b.BondRegs[0].AnswerDigest})

	if !proposer.peerCanReconstruct(other, b) {
		t.Fatal("a peer that REPORTED holding this registration is still sent the proof carried. That is the " +
			"1-of-n-1 coverage the field measured: every attester already holds the bytes, and the proposer " +
			"cannot see it")
	}
}

// THE CONTROL ON THE DIGEST: a report of some OTHER registration buys nothing.
// Without this the rule would be "a peer that said anything gets shed to", which
// would send a block to a peer that cannot rebuild it on every renewal boundary.
func TestReportingADifferentDigestDoesNotQualifyAPeer(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	proposer, author, other := nodes[0], nodes[2], ids[1].NodeID()
	author.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := author.chain.Head()
	reg, _ := author.RegisterBondReg(head)
	b := blockCarrying(t, proposer, reg)

	var wrong ports.Hash
	wrong[0] = 0xAB
	proposer.noteHeldRegs(other, []ports.Hash{wrong})

	if proposer.peerCanReconstruct(other, b) {
		t.Fatal("a peer that reported an UNRELATED digest was treated as holding this registration — the " +
			"report is being read as a flag rather than as a statement about specific bytes")
	}
}

// THE CONTROL ON STALENESS: a report REPLACES the last one rather than adding to
// it, so a peer whose queue drained stops being shed to.
//
// Evidence that outlives the fact it is about is the defect the acknowledgement
// path already had to fix once; accumulating reports would rebuild it here.
func TestAnEmptyReportClearsWhatAPeerWasHolding(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	proposer, author, other := nodes[0], nodes[2], ids[1].NodeID()
	author.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := author.chain.Head()
	reg, _ := author.RegisterBondReg(head)
	b := blockCarrying(t, proposer, reg)
	d := *b.BondRegs[0].AnswerDigest

	proposer.noteHeldRegs(other, []ports.Hash{d})
	if !proposer.peerCanReconstruct(other, b) {
		t.Fatal("PREMISE BROKEN: the peer did not qualify after reporting the digest")
	}
	// Its queue drained — the next reply carries nothing.
	proposer.noteHeldRegs(other, nil)
	if proposer.peerCanReconstruct(other, b) {
		t.Fatal("a peer that reported an EMPTY queue is still treated as holding the registration. A report " +
			"must replace the last one, never accumulate: a union of past reports asserts bytes the peer " +
			"dropped heights ago")
	}
	// And a report naming a different registration also displaces the old one.
	var another ports.Hash
	another[0] = 0x7F
	proposer.noteHeldRegs(other, []ports.Hash{d})
	proposer.noteHeldRegs(other, []ports.Hash{another})
	if proposer.peerCanReconstruct(other, b) {
		t.Fatal("a later report did not displace the earlier one — the evidence accumulates")
	}
}

// ALL, NOT ANY: one unreportable registration makes the whole block carry.
func TestOneUnreportedRegistrationMakesTheWholeBlockCarry(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	proposer, a1, a2 := nodes[0], nodes[2], nodes[3]
	other := ids[1].NodeID()
	a1.EnableBond(ids[2].Signer(), 2<<20)
	a2.EnableBond(ids[3].Signer(), 2<<20)
	head, _ := a1.chain.Head()
	r1, ok1 := a1.RegisterBondReg(head)
	r2, ok2 := a2.RegisterBondReg(head)
	if !ok1 || !ok2 {
		t.Fatal("VACUOUS: needed two registrations from two validators")
	}
	b := blockCarrying(t, proposer, r1, r2)
	if len(b.BondRegs) != 2 {
		t.Fatalf("PREMISE BROKEN: the block carries %d registrations, want 2", len(b.BondRegs))
	}

	// The peer reports only the FIRST.
	proposer.noteHeldRegs(other, []ports.Hash{*b.BondRegs[0].AnswerDigest})
	if proposer.peerCanReconstruct(other, b) {
		t.Fatal("a block with a registration the peer did NOT report was still shed to it. One missing proof " +
			"makes the whole block unvalidatable there, so the rule is ALL, never ANY")
	}
	// With both reported it qualifies — the arm that keeps the check above from
	// passing for the wrong reason.
	proposer.noteHeldRegs(other, []ports.Hash{*b.BondRegs[0].AnswerDigest, *b.BondRegs[1].AnswerDigest})
	if !proposer.peerCanReconstruct(other, b) {
		t.Fatal("a peer that reported BOTH registrations was still refused")
	}
}

// THE INVENTORY A NODE PUBLISHES IS WHAT IT CAN ACTUALLY REBUILD, and it costs no
// hashing to produce — the digests were computed when the registrations arrived.
func TestAHoldersReportNamesWhatItCanRebuild(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	holder, author := nodes[0], nodes[2]
	author.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := author.chain.Head()
	reg, ok := author.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration minted")
	}

	if got := holder.heldRegDigests(); len(got) != 0 {
		t.Fatalf("a node holding nothing reported %d digest(s)", len(got))
	}
	holder.handleChain(author.ID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(reg)})

	got := holder.heldRegDigests()
	if len(got) != 1 {
		t.Fatalf("after queueing one registration the holder reported %d digest(s), want 1", len(got))
	}
	// The digest it reports must be the one a proposer will sign in the block, or
	// the report is about bytes nobody will ask for.
	answer, rebuilt := holder.heldAnswerFor(author.ID(), got[0])
	if !rebuilt {
		t.Fatal("the holder reported a digest it cannot itself rebuild — the report and the reconstruction " +
			"path disagree about which bytes are held")
	}
	t.Logf("MEASURED — the holder reports 1 digest naming a %d-byte answer it can rebuild", len(answer))
}

// blockCarrying builds a witnessable block carrying regs, with each registration's
// answer committed by digest exactly as the proposer path does — the digest is the
// thing peerCanReconstruct reads, so a fixture that skipped it would test nothing.
func blockCarrying(t *testing.T, proposer *Node, regs ...chain.BondReg) *chain.Block {
	t.Helper()
	head, _ := proposer.chain.Head()
	b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
		Entries: []ports.Entry{mkEntry("held-inventory")}, BondRegs: append([]chain.BondReg(nil), regs...)}
	for i := range b.BondRegs {
		d := chain.AnswerDigestOf(b.BondRegs[i].Answer)
		b.BondRegs[i].AnswerDigest = &d
	}
	return b
}

// END TO END OVER THE SWEEP: the report actually travels, on the message that
// already crosses, and the prober records it.
//
// The three arms above test the RULE against a report placed by hand. This one
// tests that a report is produced, encoded, carried and consumed by the real
// chain-sync path — because a rule that nothing delivers to is a rule that never
// fires, which is exactly how this coverage gap survived two field runs.
func TestAHeadProbeCarriesTheHoldersInventoryToTheProber(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	prober, holder, author := nodes[0], nodes[1], nodes[2]
	author.EnableBond(ids[2].Signer(), 2<<20)

	head, _ := author.chain.Head()
	reg, ok := author.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration minted")
	}
	// The holder receives the author's renewal the way the sweep delivers it.
	holder.handleChain(author.ID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(reg)})
	if len(holder.pendingBondRegs) != 1 {
		t.Fatalf("PREMISE BROKEN: the holder queued %d registrations, want 1", len(holder.pendingBondRegs))
	}

	if got := len(prober.peerHeldRegs[holder.ID()]); got != 0 {
		t.Fatalf("PREMISE BROKEN: the prober already holds %d digest(s) for that peer before any sweep", got)
	}

	prober.SyncChain([]ports.NodeID{holder.ID()}, func(int, error) {})
	drainHeld(t, net, func([]simnet.HeldMsg) int { return 0 })

	held := prober.peerHeldRegs[holder.ID()]
	d := chain.AnswerDigestOf(reg.Answer)
	t.Logf("MEASURED — after one chain-sync sweep the prober holds %d reported digest(s) for that peer", len(held))

	if !held[d] {
		t.Fatalf("the holder's inventory did not reach the prober over a real sweep (%d digest(s) recorded). "+
			"The rule reads peerHeldRegs, so a report that never arrives leaves coverage exactly where the "+
			"field found it: 1 of n-1 attesters", len(held))
	}

	// AND THE BLOCK THE PROBER WOULD PROPOSE IS NOW SHEDDABLE TO THAT PEER — the
	// end the whole path exists for, asserted rather than inferred from the map.
	b := blockCarrying(t, prober, reg)
	if !prober.peerCanReconstruct(holder.ID(), b) {
		t.Fatal("the digest arrived and the proposer still will not shed to that peer")
	}
}
