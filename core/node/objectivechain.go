// Objective fork-choice wiring (M0 consensus D2 /). These turn a validator's
// replica from the subjective reputation view onto the objective, on-chain
// PoST-bond view: the chain verifies bond registrations with the same space-time
// primitive the audit loop uses (EnableObjectiveChain), and a node mints its own
// registration from its held bond (RegisterBondReg). With MinBond set, quorum
// and eligibility then become a function of the chain — identical on every
// replica — so honest replicas can no longer diverge.
package node

import (
	"github.com/nerolabs/silt/core/bond"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/vdf"
	"github.com/nerolabs/silt/ports"
)

// SpaceTimeBondVerifier returns the bond.VerifySpaceTime-backed verifier
// closure the objective chain runs over committed BondRegs. Exported so the
// daemon can wire it onto a replica BEFORE chainstore.Replay: objective is
// MinBond>0 AND verifyBond!=nil, so a replay executed before the verifier is
// wired silently falls to the LEGACY rep-gated qualification — with an empty
// boot ledger, validatorsSeen rebuilds EMPTY and the everMature latch is
// lost.
func SpaceTimeBondVerifier(delay uint64, k int) func(pk []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool {
	return func(pk []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool {
		ans, err := bond.DecodeAnswer(answer)
		if err != nil {
			return false
		}
		// pk is the authoritative validator key from BondReg.Validator (the chain
		// verifies the ed25519 signature over it). Labels are recomputed from
		// H(pk, n), so a plot sealed for another identity or size fails (G2).
		return bond.VerifySpaceTime(pk, root, size, nonce, ans, vdf.Default(), delay, k)
	}
}

// EnableObjectiveChain injects the space-time bond verifier into this node's
// replica so on-chain BondRegs are re-checked against the real bond primitive
// (bond.VerifySpaceTime, the same check the audit loop runs). Call after
// EnableChain. It only changes behavior when the chain's Config.MinBond > 0 —
// otherwise the replica stays on the legacy reputation path. NOTE: a
// replica that REPLAYS history (chainstore.Replay) must have the verifier
// wired BEFORE the replay — the daemon does this directly via
// SpaceTimeBondVerifier; Reload refuses an objective-config replay without it.
func (n *Node) EnableObjectiveChain() {
	if n.chain == nil {
		return
	}
	n.chain.SetBondVerifier(SpaceTimeBondVerifier(n.cfg.BondVDFDelay, n.cfg.BondLabelSamples))
}

// RegisterBondReg builds this node's signed on-chain bond registration for the
// position following prev: its space-time proof answered for BondRegNonce(prev)
// and signed by its key. Returns false if the node holds no bond. A bonded
// proposer includes the result in a block so the validator enters (or renews in)
// the objective set; the fresh per-position nonce stops the proof being replayed.
func (n *Node) RegisterBondReg(prev ports.Hash) (chain.BondReg, bool) {
	if n.bond == nil || n.signer == nil {
		return chain.BondReg{}, false
	}
	ans, ok := n.bond.AnswerSpaceTime(chain.BondRegNonce(prev), vdf.Default(), n.cfg.BondVDFDelay, n.cfg.BondLabelSamples)
	if !ok {
		return chain.BondReg{}, false
	}
	answer, err := bond.EncodeAnswer(ans)
	if err != nil {
		return chain.BondReg{}, false
	}
	return chain.NewBondReg(n.signer, n.bond.Root, n.bond.Size, answer, prev, n.domainID), true
}

// SubmitBondRenewal broadcasts a fresh self-signed bond registration to peers so
// whichever one proposes next folds it into a block — the NON-PROPOSER renewal
// path (H2 /). It is the liveness half of a bond TTL: an attest-only validator
// that still holds its plot can answer the fresh challenge and so keeps its
// objective standing without ever proposing, while a validator that RELEASED its
// plot (the release-and-coast attack) cannot produce the proof and decays out.
// No-op off the objective path or with no bond/signer. Fire-and-forget: a dropped
// submission is retried on the next sweep, and one inclusion resets the TTL clock.
func (n *Node) SubmitBondRenewal(peers []ports.NodeID) {
	if n.chain == nil || !n.chain.Objective() || n.bond == nil || n.signer == nil {
		return
	}
	// Q1(c): an F2 eviction is permanent (the chain never clears slashed),
	// but it also deletes bonded[id] — which makes BondRenewalDue read true
	// FOREVER for this node. Without this gate an evicted daemon re-broadcast
	// its full ~1.5 MB space-time proof every sweep, unbounded, and honest
	// proposers committed each one as a fresh block: the island OOM's dominant
	// driver (the bond-renewal storm). Back off permanently and say why, once —
	// silence here would read as a discovery failure (B5).
	if n.chain.IsSlashed(n.id) {
		if !n.evictionLogged {
			n.evictionLogged = true
			n.logf(ports.LogWarn, "bond renewal suppressed: this identity is permanently evicted (F2 equivocation slash) — it can never re-earn standing; run a new identity to rejoin", "id", n.id)
		}
		return
	}
	// Only submit when a (re)registration is actually due — not on every sweep. An
	// already-bonded validator broadcasting its full space-time proof each sweep just
	// hands proposers more block bloat to carry (the peer half of the wedge).
	if !n.chain.BondRenewalDue(n.id) {
		return
	}
	head, next := n.chain.Head()

	// MINT ONCE, AND RE-SEND ONLY WHERE THERE IS NO RECEIPT.
	//
	// "Due" means no block has COMMITTED this registration yet. It does not mean
	// none was SENT. Those were the same statement for as long as the head kept
	// moving, and they came apart the moment it stopped: a chain that cannot commit
	// holds BondRenewalDue true indefinitely, so this path re-minted and
	// re-broadcast the full ~1.5 MB space-time proof to every peer on every sweep,
	// for as long as the stall lasted.
	//
	// Measured in the field: 29 re-broadcasts from one validator inside one
	// ten-minute window, one per 30 s sweep, until the per-peer outbound budget was
	// full and the transport began DROPPING the consensus frames that would have
	// ended the stall. The stall fed the traffic that sustained the stall, so the
	// chain could not recover on its own.
	//
	// Clearing the receipts every sweep had the same root and its own cost. A
	// registration is a deterministic function of its prev, so while the head holds
	// still the bytes are IDENTICAL — the receipts were discarded and re-bought,
	// each time at the price of a full proof per peer, and under impairment the
	// replacement acknowledgements did not return before the next sweep cleared
	// them again. The digest relay's only evidence for a proposer's OWN
	// registration is these receipts, so that churn is what collapsed its coverage
	// to the peers that authored their own.
	minted := false
	if n.ownBondReg == nil || n.ownBondRegHead != head || !n.chain.ValidateBondReg(*n.ownBondReg) {
		// KEEP WHAT WAS BROADCAST. The block this node proposes next should commit
		// these bytes rather than a freshly minted equivalent over a later head: the
		// peers being handed this copy are the same peers that will attest that
		// block, and only bytes they already hold can ever be relayed by digest. The
		// chain decides when the kept copy stops standing — ValidateBondReg is the
		// same gate the block itself will face, so a kept copy can never outlive its
		// window.
		reg, ok := n.RegisterBondReg(head)
		if !ok {
			return
		}
		kept := reg
		n.ownBondReg = &kept
		n.ownBondRegHead = head
		// A fresh registration means every prior ack is about different bytes.
		n.ownRegAcks = make(map[ports.NodeID]bool, len(peers))
		n.ownRegDelivered = false
		minted = true
	}
	if n.ownRegAcks == nil {
		// A registration minted by the PROPOSE path deliberately carries no
		// receipts, and a nil map cannot record the ones this broadcast earns.
		n.ownRegAcks = make(map[ports.NodeID]bool, len(peers))
	}

	// Only peers WITHOUT a receipt are sent anything. A peer that acknowledged
	// these exact bytes holds them; re-sending is the storm. A peer that did not —
	// a lost packet, a refusal, a restart, or a receipt the relay dropped when it
	// answered NeedBody — is retried here, which is what keeps a lost renewal from
	// silently decaying the validator out of the bonded set.
	//
	// WHAT BOUNDS HOLDING A RECEIPT FOREVER, since a peer's pending queue does not
	// survive its restart and nothing tells us that it restarted. The HEAD does, and
	// it is the gate above. The copy is kept only while the head it was minted over
	// is still the head, so the two cases close themselves:
	//
	//   - the chain is COMMITTING: the head moves, the branch above re-mints over
	//     the new head and re-broadcasts to everybody, and a peer that restarted is
	//     handed the registration again on the next block. This is the behaviour
	//     that shipped before, unchanged, because a live chain was never the
	//     problem.
	//   - the chain is WEDGED: the head does not move, so the copy stands and no
	//     traffic is sent. Nothing is lost by that silence — the TTL is denominated
	//     in BLOCKS, so a chain that commits nothing cannot expire the standing this
	//     renewal is defending either, and a peer that restarts into a wedged chain
	//     is not going to propose its way out regardless.
	//
	// The gate is the HEAD rather than ValidateBondReg, and the difference is not
	// cosmetic. ValidateBondReg accepts a reg over the last BondRegHeadWindow heads,
	// which bounds the NONCE's freshness — not whether committing it would still
	// renew standing. Where the TTL is tighter than that window, a registration
	// stays window-valid after the standing it defends has already decayed, and a
	// node holding one sends nothing while it drops out of the bonded set. Both
	// conditions are checked, so the copy stands only while it is the current head's
	// AND the chain would still take it.
	//
	// WHAT THIS DOES NOT BOUND, named rather than left to be discovered: a peer that
	// refuses PERMANENTLY — a mixed swarm where it is not on the objective path, or
	// a skew that never heals — never acknowledges, so it is retried every sweep for
	// as long as that lasts. That is not a regression (every peer was retried every
	// sweep before), and it cannot be fixed by giving up, because giving up on an
	// unacknowledged peer is the lapse this retry exists to prevent. It is made
	// VISIBLE instead: the re-send below names how many peers it is still chasing,
	// so a permanent refusal reads as a stuck count rather than as silence (S5).
	pending := make([]ports.NodeID, 0, len(peers))
	for _, p := range peers {
		if p == n.id || n.ownRegAcks[p] {
			continue
		}
		pending = append(pending, p)
	}
	if len(pending) == 0 {
		// Every peer holds it and no block has committed it. That is a chain that is
		// not proposing, not a delivery that failed, and the two want different
		// remedies — say so ONCE per registration rather than every sweep, which is
		// the log-shaped version of the same storm (S5).
		if !n.ownRegDelivered {
			n.ownRegDelivered = true
			n.logf(ports.LogInfo, "bond renewal delivered to every peer — waiting for a proposer to COMMIT it",
				"signed_next_height", next, "size", n.ownBondReg.Size, "peers", len(peers))
		}
		return
	}
	n.ownRegDelivered = false
	// The signed-over head is the other half of the refusal correlation: a
	// receiver whose committed window does not yet include this head refuses
	// the reg with a bare "signature" error (chainrole MsgSubmitBondReg), so
	// without this line a field read cannot tell WAN head-skew from forgery.
	if minted {
		n.logf(ports.LogInfo, "bond renewal submitted", "signed_next_height", next, "size", n.ownBondReg.Size, "peers", len(pending))
	} else {
		n.logf(ports.LogInfo, "bond renewal re-sent to peers without a receipt",
			"signed_next_height", next, "size", n.ownBondReg.Size, "peers", len(pending), "of", len(peers))
	}
	raw := bondRegEncode(*n.ownBondReg)
	for _, p := range pending {
		p := p
		n.request(p, ports.Message{Kind: ports.MsgSubmitBondReg, Data: raw}, func(resp ports.Message, err error) {
			// RECORD THE ACK, which is the whole reason this callback is no longer
			// empty. A peer that acknowledged this registration HOLDS these bytes —
			// the reply is OK only when the receiver queued them — and that is the
			// only evidence that lets the block carrying them be relayed to that peer
			// by digest. An unacknowledged peer is not assumed to hold anything: it
			// gets the proof carried, exactly as before.
			if err == nil && resp.OK && n.ownRegAcks != nil {
				n.ownRegAcks[p] = true
				return
			}
			// AND COUNT THE ONES THAT DID NOT COME BACK, because silence here is not
			// the same as a proof that did not cross. The deadline this request runs
			// under is sized off RequestSizeFloorBytesPerSec, so on a link below that
			// floor it expires while the bytes are still on the wire — the peer queues
			// them and replies, and the reply lands after nobody is listening. The
			// registration is then held by the peer, unknown to its author, re-sent in
			// full on the next sweep, and carried again in the proposal (S5).
			n.Stats.BondRegSubmitAcksLost++
		})
	}
}

// ownRegForBlock is the registration this node embeds in a block it proposes on
// prev: the one it already BROADCAST, when the chain still accepts it, and a fresh
// mint otherwise.
//
// MINT ONCE, AND PREFER THE COPY THE ATTESTERS WERE HANDED. Both are valid and both
// commit; the difference is who else holds the bytes. A registration is a
// deterministic function of its prev, so re-minting over a LATER head is not a
// different registration in any meaningful sense — it is the same claim over a
// different nonce, and it is bytes no attester has ever seen. Meanwhile the copy
// they were handed on the sweep sits in their pending queues, verified, unused, and
// valid for the block being built: the head window (chain.BondRegHeadWindow) exists
// precisely so a registration survives the head advancing under it.
//
// The chain decides, never this function. ValidateBondReg is the same gate the
// block will face — the head window and the re-registration interval both — so a
// registration this returns is one the proposer's own block check will accept, and
// a stale one falls through to a fresh mint rather than burning the proposer's turn
// on a block its own validity rule would reject.
//
// A FRESH MINT IS NOT FREE, which is the second reason to prefer the kept copy: it
// runs the VDF the space-time proof is built on, on the single serialized loop, for
// a proof the node already produced.
func (n *Node) ownRegForBlock(prev ports.Hash) (chain.BondReg, bool) {
	if n.ownBondReg != nil && n.chain != nil && n.chain.ValidateBondReg(*n.ownBondReg) {
		return *n.ownBondReg, true
	}
	reg, ok := n.RegisterBondReg(prev)
	if ok {
		kept := reg
		n.ownBondReg = &kept
		n.ownBondRegHead = prev
		// Minted here rather than broadcast, so nobody has acknowledged these bytes
		// and nobody may be sent them by digest — nor has anything been delivered.
		n.ownRegAcks = nil
		n.ownRegDelivered = false
	}
	return reg, ok
}

// carryReason names why a gather leg was sent with its heavy proofs CARRIED rather
// than relayed by digest. The relay's coverage — the share of legs that shed — is the
// quantity the adverse-network liveness bound rests on, and a bare ratio cannot say
// what to do about it. These separate the two cases, which were measured to be
// different failures wanting different work (core/node/bondreg_relay_coverage_measure_test.go).
type carryReason uint8

const (
	carryNotShed carryReason = iota // the leg shed; there is no carry to give a reason for
	// carryNoDigestForm: the block predates the witnessable era or carries no
	// registration, so there is no heavy proof to shed and no saving to be had. Counted
	// in neither half of the coverage ratio — it is not a coverage failure, and putting
	// it in the denominator would report a working relay as a broken one.
	carryNoDigestForm
	// carryOwnRegUnacked: the block carries THIS node's own registration and nothing
	// evidences that the peer holds it. On a wire that delivers this is zero, because
	// the proposer commits the copy it broadcast and a broadcast is acknowledged only
	// once the peer has queued the bytes. On a wire that does not, it is EVERY carried
	// leg — and the peer has no evidence because it has no bytes: the ~1.5 MB submit
	// that would have delivered them has not completed on the same congested link.
	// There is no more evidence to add for that case. The relay cannot shed bytes a
	// peer has never received, so this is the first crossing and not an evidence gap.
	carryOwnRegUnacked
	// carryPeerUnreported: the block carries a THIRD party's registration the peer has
	// not reported holding. Every validator is sent every renewal, so this is usually
	// the report arriving after the proposal rather than the bytes being absent — the
	// inventory rides a 30-second head probe while a round is shorter than that. This
	// is the half more evidence would close.
	carryPeerUnreported
)

func (c carryReason) String() string {
	switch c {
	case carryNotShed:
		return "none"
	case carryNoDigestForm:
		return "no-digest-form"
	case carryOwnRegUnacked:
		return "own-reg-unacked"
	case carryPeerUnreported:
		return "peer-unreported"
	}
	return "unknown"
}

// peerCanReconstruct reports whether v demonstrably holds the heavy space-time
// proof of EVERY bond registration in b, so the block may be relayed to v with
// those proofs replaced by the digests that already commit them — and, when it does
// not, which registration went unevidenced.
//
// EVIDENCE, NEVER INFERENCE. There are exactly three ways to know, and all three are
// observations rather than expectations:
//
//   - v ACKNOWLEDGED this node's own registration (ownRegAcks). The ack is a
//     receipt for specific bytes, and it is discarded the moment those bytes change.
//   - v AUTHORED the registration. A validator holds its own proof by construction;
//     it is the party that produced it.
//   - v REPORTED holding it, on the answer-digest list its last chain-sync head
//     reply carried (peerHeldRegs). This is the widest of the three and the one the
//     field made necessary: the first two only ever cover a registration this node
//     or v authored, so on a live chain — where renewals are staggered and a block
//     carries one OTHER validator's registration — exactly one attester out of n-1
//     qualified, and the rest were sent ~1.5 MB they already held.
//
// Anything else — "every validator is sent every submission, so v probably has it"
// — is an assumption about the network, and a wrong one costs the round it is wrong
// in. The third evidence is not that assumption: it is v's own statement about v's
// own queue, which is a fact reported by the only party that can observe it and the
// only party that pays when it is wrong (it answers NeedBody and takes the round
// trip). A peer this returns false for is sent the proofs carried, which is what the
// wire did for every peer before this existed.
//
// ALL, not any: one unreconstructable registration makes the whole block
// unvalidatable to v, so a mixed block falls back for that peer.
//
// THE REASON IS REPORTED, not left for a reader to derive. The evidences are
// disjunctive, so a bare false says only that none of them held, and which
// registration went unevidenced is what separates a gap worth closing from a ceiling.
func (n *Node) peerCanReconstruct(v ports.NodeID, b *chain.Block) (bool, carryReason) {
	if b.Version < chain.BlockVersionWitnessable || len(b.BondRegs) == 0 {
		return false, carryNoDigestForm // only the witnessable era commits a proof by digest
	}
	for i := range b.BondRegs {
		author := b.BondRegs[i].ValidatorID()
		switch {
		case author == v:
			continue // it produced this proof
		case author == n.id && n.ownRegAcks[v]:
			continue // it acknowledged receiving this proof from us
		case n.peerReportedHolding(v, b.BondRegs[i].AnswerDigest):
			continue // it told us, on its last head reply, that it holds these bytes
		case author == n.id:
			return false, carryOwnRegUnacked
		default:
			return false, carryPeerUnreported
		}
	}
	return true, carryNotShed
}

// shedQCLegFor decides whether the prepare-QC leg to v may carry the digest form,
// and names the reason when it may not.
//
// A PEER NAMED IN THE PREPARE-QC PREPARED ON THIS BLOCK, which is a stronger claim
// than any receipt: it received the block, reconstructed the proofs, validated it and
// signed its hash. It cannot have done that without holding them. That evidence is
// free exactly at the moment the second copy would otherwise be sent.
//
// AND IT IS NOT THE ONLY EVIDENCE, which is what this seam exists to say. The QC names
// the quorum that replied FIRST, not the set of peers that hold the proof — every
// attester outside it still has the three ordinary evidences, and a peer that
// demonstrably holds the bytes does not stop holding them for having been slow. Before
// this leg consulted them, an attester left out of a quorum it was one reply late for
// was sent the full proof a SECOND time in the same round, having already been shed to
// on the first. Measured over eleven heights on a fast wire: the precommit leg carried
// 19,726,159 B on QC membership alone and 4,578,693 B with the ordinary evidences
// behind it, across the identical number of sends.
func (n *Node) shedQCLegFor(v ports.NodeID, b *chain.Block, prepared map[ports.NodeID]bool) (bool, carryReason) {
	if prepared[v] {
		return true, carryNotShed
	}
	return n.peerCanReconstruct(v, b)
}

// noteGatherLeg records one gather leg's outcome against the relay's coverage, which
// is the share of registration-bearing legs that crossed by digest rather than
// carrying ~1.5 MB of space-time proof.
//
// A leg with nothing to shed is counted in NEITHER. Coverage is a property of the
// legs where shedding was possible at all, and folding the era-less and
// registration-less ones into the denominator would report a relay that is working
// perfectly as one that covers a few percent.
func (n *Node) noteGatherLeg(shed bool, why carryReason) {
	switch {
	case shed:
		n.Stats.GatherLegsShed++
	case why == carryOwnRegUnacked:
		n.Stats.GatherLegsCarried++
		n.Stats.GatherLegsCarriedOwnUnacked++
	case why == carryPeerUnreported:
		n.Stats.GatherLegsCarried++
		n.Stats.GatherLegsCarriedPeerUnreported++
	}
}

// reconstructShedProofs refills the heavy proofs of a block whose registrations
// arrived by digest, from registrations this node already holds. It reports whether
// EVERY shed proof was restored.
//
// THE DIGEST IS THE AUTHORITY AND THE CANDIDATE IS CHECKED AGAINST IT. AnswerDigest
// is folded into the v5 preimage in place of the proof, so it is covered by the
// proposer's own signature over the block: a candidate whose sha256 equals it IS the
// committed proof, and one whose sha256 does not is refused. Nothing here trusts the
// queue — the queue only supplies candidates, and a peer that poisoned it with a
// registration of its own devising supplies one that fails this check.
//
// It restores the block to the exact bytes the proposer committed, so the ordinary
// validity path runs afterwards UNCHANGED — the space-time proof is re-verified, the
// head window is enforced, and an Answer-less block is still refused at the trust
// floor. This adds no acceptance; it only puts back what the wire left out.
func (n *Node) reconstructShedProofs(b *chain.Block) bool {
	if !b.HeavyProofsShed() {
		return true // nothing was shed
	}
	for i := range b.BondRegs {
		if b.BondRegs[i].Answer != nil || b.BondRegs[i].AnswerDigest == nil {
			continue
		}
		want := *b.BondRegs[i].AnswerDigest
		answer, ok := n.heldAnswerFor(b.BondRegs[i].ValidatorID(), want)
		if !ok {
			n.Stats.ProposalsNeedingBodies++
			return false
		}
		b.BondRegs[i].Answer = answer
	}
	return true
}

// heldAnswerFor finds a space-time proof this node already holds whose digest is
// `want`. It searches the registrations a proposer would have been handed: the
// pending submissions queue, and this node's own last broadcast registration.
//
// The identity is checked as well as the digest. A digest collision is not the
// threat — sha256 makes that negligible — but reading a proof out from under the
// wrong validator would silently attribute one validator's possession to another,
// and the cheapest moment to refuse that is before it is substituted.
func (n *Node) heldAnswerFor(validator ports.NodeID, want ports.Hash) ([]byte, bool) {
	for _, pr := range n.pendingBondRegs {
		if pr.R.ValidatorID() != validator || pr.R.Answer == nil {
			continue
		}
		if chain.AnswerDigestOf(pr.R.Answer) == want {
			return pr.R.Answer, true
		}
	}
	if n.ownBondReg != nil && n.ownBondReg.ValidatorID() == validator && n.ownBondReg.Answer != nil {
		if chain.AnswerDigestOf(n.ownBondReg.Answer) == want {
			return n.ownBondReg.Answer, true
		}
	}
	return nil, false
}

// heldRegDigests is the set of bond-registration answer-digests this node can
// rebuild from what it already holds — its pending queue plus its own broadcast
// registration. It is reported to peers on every chain-sync head reply, and it is
// what lets a PROPOSER shed those registrations' heavy proofs to this node.
//
// WHY THE HOLDER REPORTS, rather than the submitter vouching for who acknowledged
// it. A holder that misreports is sent a form it cannot rebuild, answers NeedBody
// and pays the round trip itself; a third party asserting the same thing would put
// that cost on the proposer. The claim and the cost of it being wrong belong with
// the same party, and only this arrangement puts them there.
//
// It is ADVISORY AND NEVER AUTHORITY. Nothing here grants acceptance: a receiver
// still rebuilds only bytes whose sha256 equals the digest the proposer SIGNED
// (reconstructShedProofs), so a false entry buys a wasted round trip and can never
// substitute a proof. That is the same bound the acknowledgement path already has.
//
// No hashing happens here — every digest was computed once when its registration
// arrived, because this runs on the single serialized loop on a 30-second timer and
// the answers are megabytes each (see pendingBondReg.D).
func (n *Node) heldRegDigests() []ports.Hash {
	if n.chain == nil || !n.chain.Objective() {
		return nil
	}
	out := make([]ports.Hash, 0, len(n.pendingBondRegs)+1)
	for i := range n.pendingBondRegs {
		if n.pendingBondRegs[i].R.Answer != nil {
			out = append(out, n.pendingBondRegs[i].D)
		}
	}
	if n.ownBondReg != nil && n.ownBondReg.Answer != nil {
		out = append(out, chain.AnswerDigestOf(n.ownBondReg.Answer))
	}
	return out
}

// noteHeldRegs records what a peer reported holding on its last head reply,
// REPLACING any earlier report rather than accumulating one.
//
// Replacement is the whole discipline: the list is a snapshot of a queue that
// drains as registrations commit, so a union of past reports would keep asserting
// that a peer holds bytes it dropped heights ago — evidence that outlives the fact
// it is about, which is precisely the defect the acknowledgement path already had
// to fix once.
func (n *Node) noteHeldRegs(peer ports.NodeID, digests []ports.Hash) {
	if len(digests) == 0 {
		delete(n.peerHeldRegs, peer)
		return
	}
	if n.peerHeldRegs == nil {
		n.peerHeldRegs = make(map[ports.NodeID]map[ports.Hash]bool)
	}
	held := make(map[ports.Hash]bool, len(digests))
	for _, d := range digests {
		held[d] = true
	}
	n.peerHeldRegs[peer] = held
}

// peerReportedHolding reports whether v's last head reply listed this registration's
// answer-digest. A nil digest is never a match: the digest is what the proposer
// SIGNS in place of the proof, so a registration without one cannot be relayed by
// one either.
func (n *Node) peerReportedHolding(v ports.NodeID, d *ports.Hash) bool {
	if d == nil {
		return false
	}
	return n.peerHeldRegs[v][*d]
}
