package node

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// A RENEWAL IS BROADCAST ONCE AND REMEMBERED UNTIL IT COMMITS.
//
// "Due" means no block has COMMITTED this registration yet. It does not mean none
// was sent. Those two statements were the same for as long as the head kept moving,
// and they came apart the moment it stopped: a chain that cannot commit holds
// BondRenewalDue true indefinitely, so the sweep re-minted and re-broadcast the full
// ~1.5 MB space-time proof to every peer every ChainSyncInterval, for as long as the
// stall lasted.
//
// Measured in the field: 29 re-broadcasts from one validator inside one ten-minute
// window, one per 30 s sweep, until the per-peer outbound budget was full and the
// transport began DROPPING the consensus frames that would have ended the stall —
// `outbound budget full: 67066043 bytes already in flight against a 67108864-byte
// share; dropped a frame of 1575203 bytes`. The stall fed the traffic that sustained
// the stall.
//
// The same line also cleared ownRegAcks on every sweep, on the reasoning that a
// fresh registration is about different bytes. On a stalled chain the head does not
// move and a registration is a deterministic function of its prev, so the bytes were
// IDENTICAL and the receipts were discarded anyway — faster than a stalled round
// could ever use them. That is why the digest relay shed to every peer at h1-h4 and
// to no peer afterwards.
//
// The rule these assert: mint once, keep the receipts while the bytes are unchanged,
// and re-send only where there is no receipt.

// renewalPeers is the peer set a node sweeps, minus itself.
func renewalPeers(ids []*identity.Identity, self int) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(ids))
	for i, id := range ids {
		if i != self {
			out = append(out, id.NodeID())
		}
	}
	return out
}

// sweepRenewal runs one sweep's renewal and lets every message AND ITS REPLY land,
// which is what makes the acknowledgements real rather than assumed. It reports how
// many submits left for the wire.
func sweepRenewal(t *testing.T, net *simnet.Network, n *Node, peers []ports.NodeID) int {
	t.Helper()
	before := net.Stats.Kinds[ports.MsgSubmitBondReg]
	n.SubmitBondRenewal(peers)
	drainHeld(t, net, func([]simnet.HeldMsg) int { return 0 })
	return net.Stats.Kinds[ports.MsgSubmitBondReg] - before
}

// bondedRenewer returns a 4-anchor net with node 2 bonded, plus its peer set.
func bondedRenewer(t *testing.T) ([]*Node, []*identity.Identity, *simnet.Network, *Node, []ports.NodeID) {
	t.Helper()
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	v := nodes[2]
	v.EnableBond(ids[2].Signer(), 2<<20)
	return nodes, ids, net, v, renewalPeers(ids, 2)
}

// THE STORM: a renewal that cannot commit is sent ONCE, not once per sweep.
//
// The control that makes this mean something is the first sweep: it must broadcast
// to every peer. A fix that simply stopped submitting would pass a "no traffic on
// later sweeps" assertion perfectly and silently decay the validator out of the
// bonded set, which is the failure this test is most at risk of certifying.
func TestAStalledRenewalIsBroadcastOnceRatherThanEverySweep(t *testing.T) {
	_, _, net, v, peers := bondedRenewer(t)

	if !v.chain.BondRenewalDue(v.ID()) {
		t.Fatal("VACUOUS: the renewal was not due at the start, so every count below would be measuring the " +
			"due-gate rather than the re-send rule")
	}

	const sweeps = 6
	counts := make([]int, sweeps)
	for i := range counts {
		counts[i] = sweepRenewal(t, net, v, peers)
	}

	total := 0
	for _, c := range counts {
		total += c
	}
	t.Logf("MEASURED — MsgSubmitBondReg sends per sweep, with the chain committing nothing:")
	t.Logf("  per sweep: %v", counts)
	t.Logf("  total over %d sweeps to %d peers: %d", sweeps, len(peers), total)

	if !v.chain.BondRenewalDue(v.ID()) {
		t.Fatal("VACUOUS: the registration COMMITTED during the sweeps, so the renewal stopped being due for a " +
			"reason that is not the rule under test — the zero counts would be the commit's, not the rule's")
	}
	if counts[0] != len(peers) {
		t.Fatalf("the FIRST sweep broadcast to %d of %d peers. A renewal nobody holds must reach everybody, or the "+
			"validator decays out of the bonded set while this test reports success", counts[0], len(peers))
	}
	for i := 1; i < sweeps; i++ {
		if counts[i] != 0 {
			t.Fatalf("sweep %d re-broadcast the registration to %d peer(s) that had already acknowledged it. "+
				"Every peer holds these exact bytes and the chain has not committed them — re-sending is the storm "+
				"that filled the outbound budget in the field and dropped the consensus frames that would have "+
				"cleared the stall (per-sweep counts %v)", i, counts[i], counts)
		}
	}
}

// THE LIVENESS HALF: a peer WITHOUT a receipt is still retried.
//
// This is the positive control on the gate's own axis. A renewal that is never
// re-sent is a validator that lapses the first time a packet is lost, which is a
// liveness regression wearing a traffic fix's clothes — and it would pass the storm
// test above cleanly.
func TestAPeerWhoseSubmitWasLostIsRetriedOnTheNextSweep(t *testing.T) {
	_, _, net, v, peers := bondedRenewer(t)

	// Sweep one, but DROP the submit bound for the first peer — it never arrives,
	// so it never acknowledges, exactly as a lost packet leaves it.
	lost := peers[0]
	v.SubmitBondRenewal(peers)
	dropped := 0
	for _, m := range net.Pending() {
		if m.Kind == ports.MsgSubmitBondReg && m.To == lost {
			if net.DropPending(m.ID) {
				dropped++
			}
		}
	}
	drainHeld(t, net, func([]simnet.HeldMsg) int { return 0 })
	if dropped != 1 {
		t.Fatalf("PREMISE BROKEN: meant to drop exactly one submit to the lost peer, dropped %d — the retry below "+
			"would then be measuring a peer that was never missing", dropped)
	}

	// The receipt state that DECIDES sweep 2 is the one standing before it runs: a
	// message that never arrived must have earned nothing.
	ackedBefore, heldBefore := v.ownRegAcks[lost], len(v.ownRegAcks)

	// Sweep two: the peers that acknowledged are left alone, the lost one is retried.
	got := sweepRenewal(t, net, v, peers)

	t.Logf("MEASURED — one peer's submit was lost on sweep 1:")
	t.Logf("  after sweep 1: receipts %d of %d, lost peer acknowledged=%v", heldBefore, len(peers), ackedBefore)
	t.Logf("  sweep 2 re-sent to: %d peer(s)", got)
	t.Logf("  after sweep 2: receipts %d of %d, lost peer acknowledged=%v", len(v.ownRegAcks), len(peers), v.ownRegAcks[lost])

	if ackedBefore {
		t.Fatal("the peer whose submit was DROPPED was recorded as having acknowledged it. A receipt that survives a " +
			"message that never arrived is not evidence of anything, and the digest relay would shed to it")
	}
	if heldBefore != len(peers)-1 {
		t.Fatalf("after sweep 1 the node holds %d receipts of %d peers, want %d — one peer never received the "+
			"submit, so exactly one receipt must be missing", heldBefore, len(peers), len(peers)-1)
	}
	if got != 1 {
		t.Fatalf("sweep 2 re-sent the registration to %d peer(s), want exactly 1 — the one whose submit was lost. "+
			"Zero means a lost renewal is never retried and the validator lapses; more than one means peers that "+
			"already hold these bytes are being re-sent them, which is the storm", got)
	}
	// And the retry has to actually land, or "retried" is a send into nothing.
	if !v.ownRegAcks[lost] || len(v.ownRegAcks) != len(peers) {
		t.Fatalf("after the retry the lost peer acknowledged=%v with %d of %d receipts held — the re-send must "+
			"recover the missing receipt, otherwise the peer is retried on every sweep forever and the storm is "+
			"back by another route", v.ownRegAcks[lost], len(v.ownRegAcks), len(peers))
	}
}

// THE RECEIPTS SURVIVE A SWEEP WHEN THE BYTES DID NOT CHANGE.
//
// This is the property the digest relay rests on for a proposer's OWN registration.
// In the field it held at h1-h4, where every proposer shed to all three peers, and
// never again for the rest of the run — because each sweep threw the receipts away
// and the relay fell back to its one remaining evidence, the peer that AUTHORED the
// registration. That is the difference between shedding to n-1 attesters and to 1.
func TestReceiptsSurviveASweepWhenTheRegistrationDidNot(t *testing.T) {
	_, _, net, v, peers := bondedRenewer(t)

	sweepRenewal(t, net, v, peers)
	first := len(v.ownRegAcks)
	if first != len(peers) {
		t.Fatalf("PREMISE BROKEN: %d of %d peers acknowledged the first broadcast, so the survival assertion below "+
			"has nothing to survive", first, len(peers))
	}
	if v.ownBondReg == nil {
		t.Fatal("PREMISE BROKEN: the sweep kept no registration")
	}
	kept := bondRegEncode(*v.ownBondReg)

	resent := 0
	for i := 0; i < 4; i++ {
		resent += sweepRenewal(t, net, v, peers)
	}

	after := len(v.ownRegAcks)
	same := v.ownBondReg != nil && bytes.Equal(bondRegEncode(*v.ownBondReg), kept)

	t.Logf("MEASURED — across four further sweeps with the head held still:")
	t.Logf("  registration bytes unchanged: %v", same)
	t.Logf("  receipts held: %d of %d (was %d)", after, len(peers), first)
	t.Logf("  bytes re-sent to refill them: %d submits", resent)

	// THE RECEIPTS MUST SURVIVE WITHOUT BEING RE-BOUGHT, which is the whole property.
	// Clearing them and immediately refilling them by re-broadcasting to everybody
	// leaves the count identical and costs a full round of proofs per sweep — so a
	// test that read only the count would pass on exactly the behaviour being fixed.
	if resent != 0 {
		t.Fatalf("the receipts were refilled by re-sending %d submit(s) across four sweeps that changed nothing. "+
			"Surviving is not the same as being re-bought: each of those carries the full space-time proof, and "+
			"under impairment the replacement acknowledgements do not return before the next sweep clears them again", resent)
	}

	if !same {
		t.Fatal("the kept registration was re-minted although the head did not move. A registration is a " +
			"deterministic function of its prev, so a re-mint over the same prev is the same claim over the same " +
			"nonce — paid for twice, and it is what discards the receipts below")
	}
	if after != first {
		t.Fatalf("receipts fell from %d to %d across sweeps that changed nothing. The relay's ONLY evidence for a "+
			"proposer's own registration is these receipts, so discarding them collapses its coverage to peers that "+
			"authored their own — 1 of n-1 attesters, which is what the field measured", first, after)
	}
}

// THE CONTROL ON THE KEEP: a registration the chain would no longer accept is
// re-minted, and its receipts go with it.
//
// Without this, "keep what was broadcast" becomes "keep it forever", and a validator
// sits on a registration no proposer can commit while its standing decays. The
// receipts must go too — they are about bytes that are no longer the claim.
func TestAStaleKeptRegistrationIsRemintedAndItsReceiptsCleared(t *testing.T) {
	_, _, net, v, peers := bondedRenewer(t)

	sweepRenewal(t, net, v, peers)
	if v.ownBondReg == nil || len(v.ownRegAcks) != len(peers) {
		t.Fatalf("PREMISE BROKEN: after the first sweep the node holds reg=%v with %d receipts, want a registration "+
			"and %d receipts", v.ownBondReg != nil, len(v.ownRegAcks), len(peers))
	}

	// Make the kept copy one the chain refuses: mint it over a prev no committed
	// window holds. The signature is the node's own, so this is a STALENESS refusal
	// and not a forgery — the shape the head window exists to bound, forced rather
	// than waited for.
	head, _ := v.chain.Head()
	bogus := head
	bogus[0] ^= 0xFF
	staleReg, ok := v.RegisterBondReg(bogus)
	if !ok {
		t.Fatal("PREMISE BROKEN: could not mint the stale registration this arm needs")
	}
	if v.chain.ValidateBondReg(staleReg) {
		t.Fatal("PREMISE BROKEN: the chain ACCEPTS the registration meant to be stale, so the re-mint below would " +
			"not be exercised")
	}
	v.ownBondReg = &staleReg

	got := sweepRenewal(t, net, v, peers)
	reminted := v.ownBondReg != nil && !bytes.Equal(bondRegEncode(*v.ownBondReg), bondRegEncode(staleReg))

	t.Logf("MEASURED — the kept registration was made unacceptable to the chain:")
	t.Logf("  re-minted: %v", reminted)
	t.Logf("  re-broadcast to: %d of %d peers", got, len(peers))

	if !reminted {
		t.Fatal("the node kept a registration its own chain refuses. A kept copy that outlives the window is a " +
			"validator that never renews: ValidateBondReg is the gate, and it must decide whether the copy stands")
	}
	if got != len(peers) {
		t.Fatalf("a freshly minted registration reached %d of %d peers. New bytes mean every prior receipt is about "+
			"something else, so all of them must be re-sent", got, len(peers))
	}
}

// THE ACKNOWLEDGEMENT MUST MEAN WHAT THE RELAY READS IT AS.
//
// ownRegAcks is the digest relay's evidence that a peer HOLDS the proof, and it is
// written from this reply's OK bit. The reply was sent unconditionally — after a
// rate refusal, a decode failure, a sender-binding refusal, a validity refusal, and
// off the objective path entirely. So a receipt recorded "the message arrived",
// while the relay read it as "the bytes are held", and the gap between those two is
// a proposal shed to a peer that cannot rebuild it.
//
// The gap is recoverable — the peer answers NeedBody and the proposer re-sends
// carried — but a recovery on the critical path is precisely what this route exists
// to avoid, and the field counted it happening on every seat.
func TestAnAcknowledgementIsNotSentForARegistrationThatWasRefused(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	v, peer := nodes[2], nodes[0]
	v.EnableBond(ids[2].Signer(), 2<<20)

	head, _ := v.chain.Head()
	good, ok := v.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration minted, so neither arm below sends anything")
	}
	// The refused arm: the same claim anchored to a prev no committed window holds.
	bogus := head
	bogus[0] ^= 0xFF
	bad, ok := v.RegisterBondReg(bogus)
	if !ok {
		t.Fatal("VACUOUS: could not mint the registration the refused arm needs")
	}

	send := func(raw []byte) (okReply, replied bool) {
		v.request(peer.ID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: raw},
			func(resp ports.Message, err error) {
				replied = err == nil
				okReply = err == nil && resp.OK
			})
		drainHeld(t, net, func([]simnet.HeldMsg) int { return 0 })
		return
	}

	goodOK, goodReplied := send(bondRegEncode(good))
	queuedGood := peerHoldsRegFrom(peer, v.ID())

	peer.pendingBondRegs = nil
	badOK, badReplied := send(bondRegEncode(bad))
	queuedBad := peerHoldsRegFrom(peer, v.ID())

	t.Logf("MEASURED — what the submit acknowledgement says against what the receiver did:")
	t.Logf("  ACCEPTED registration: queued=%v  replied=%v  OK=%v", queuedGood, goodReplied, goodOK)
	t.Logf("  REFUSED  registration: queued=%v  replied=%v  OK=%v", queuedBad, badReplied, badOK)

	if !queuedGood || !goodOK {
		t.Fatalf("an ACCEPTED registration must be acknowledged OK (queued=%v OK=%v). Without this the relay has no "+
			"evidence for anybody and every proposal carries its proofs, which is the pre-relay wire", queuedGood, goodOK)
	}
	if queuedBad {
		t.Fatal("PREMISE BROKEN: the receiver QUEUED the registration meant to be refused, so the arm below is not " +
			"measuring a refusal")
	}
	if !badReplied {
		t.Fatal("a refused submit drew no reply at all. The submitter must hear something — silence is retried " +
			"forever and is indistinguishable from a lost packet (B5)")
	}
	if badOK {
		t.Fatal("a REFUSED registration was acknowledged OK. ownRegAcks is written from this bit and the digest " +
			"relay reads it as 'this peer holds the proof', so an OK here sheds a proposal to a peer that never " +
			"queued the bytes — the receipt says the message arrived, while the relay needs it to say the bytes " +
			"are held")
	}
}

// peerHoldsRegFrom reports whether n's pending queue holds a registration authored
// by vid — the receiver-side fact an acknowledgement is supposed to be about.
func peerHoldsRegFrom(n *Node, vid ports.NodeID) bool {
	for _, pr := range n.pendingBondRegs {
		if pr.R.ValidatorID() == vid {
			return true
		}
	}
	return false
}

// THE GATE IS THE HEAD, NOT THE VALIDITY WINDOW, AND THE DIFFERENCE IS LOAD-BEARING.
//
// A registration stays acceptable to ValidateBondReg over the last
// chain.BondRegHeadWindow committed heads — a bound on the NONCE's freshness, and
// not on whether committing it would still renew standing. Where the TTL is tighter
// than that window, a kept copy stays window-valid long after the standing it
// defends has decayed: a node holding one broadcasts nothing while it drops out of
// the bonded set, and the quorum loses its weight.
//
// That is not hypothetical. Keeping on ValidateBondReg alone was written first, and
// sim.TestObjectiveBondRenewalSustainsAttestOnlyValidator — TTL 3 against the
// default window of 8 — stalled at round 5 with `0 prepares of 2 gathered`. This
// pins the rule that replaces it at the tier the logic lives on: a moved head is a
// chain that is COMMITTING, and a renewal must track it or lapse.
func TestAMovedHeadRemintsTheRenewalRatherThanKeepingIt(t *testing.T) {
	_, ids, net, v, peers := bondedRenewer(t)

	sweepRenewal(t, net, v, peers)
	if v.ownBondReg == nil || len(v.ownRegAcks) != len(peers) {
		t.Fatalf("PREMISE BROKEN: after the first sweep the node holds reg=%v with %d receipts, want a "+
			"registration and %d receipts", v.ownBondReg != nil, len(v.ownRegAcks), len(peers))
	}
	kept := bondRegEncode(*v.ownBondReg)

	// Held still, it stands and costs nothing — the control that makes the arm below
	// a statement about the HEAD rather than about sweeping twice.
	if got := sweepRenewal(t, net, v, peers); got != 0 {
		t.Fatalf("a sweep with the head unmoved re-sent to %d peer(s), want 0", got)
	}

	// Now commit a block, so the head this node builds on has moved.
	prev, next := v.chain.Head()
	b := &chain.Block{Version: 1, Height: next, Prev: prev, Entries: []ports.Entry{mkEntry("renewal-head-advance")}}
	chain.Sign(b, ids[0].Signer())
	b.Atts = []chain.Attestation{chain.Attest(b, ids[1].Signer()), chain.Attest(b, ids[3].Signer())}
	if err := v.chain.Append(*b); err != nil {
		t.Fatalf("PREMISE BROKEN: could not advance the head: %v", err)
	}
	moved, _ := v.chain.Head()
	if moved == prev {
		t.Fatal("PREMISE BROKEN: the head did not move, so the arm below is the control again")
	}
	stillWindowValid := v.chain.ValidateBondReg(*v.ownBondReg)

	got := sweepRenewal(t, net, v, peers)
	reminted := v.ownBondReg != nil && !bytes.Equal(bondRegEncode(*v.ownBondReg), kept)

	t.Logf("MEASURED — one block committed under a kept registration:")
	t.Logf("  the kept copy is still inside the validity window: %v", stillWindowValid)
	t.Logf("  re-minted anyway: %v", reminted)
	t.Logf("  re-broadcast to: %d of %d peers", got, len(peers))

	if !stillWindowValid {
		t.Skip("the kept copy left the validity window on one block, so this run cannot separate the head gate " +
			"from the window gate — the assertion needs a copy that the window still accepts")
	}
	if !reminted {
		t.Fatal("the head moved and the node kept its old registration anyway. ValidateBondReg still accepting it " +
			"is exactly the trap: the window bounds the nonce's freshness, not whether committing the copy would " +
			"renew standing, so a TTL tighter than the window lapses the validator while it broadcasts nothing")
	}
	if got != len(peers) {
		t.Fatalf("the re-minted registration reached %d of %d peers. New bytes mean every prior receipt is about "+
			"something else, so all of them must be re-sent", got, len(peers))
	}
}
