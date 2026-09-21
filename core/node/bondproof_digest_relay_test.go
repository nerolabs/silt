package node

import (
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// RELAYING A BOND REGISTRATION'S PROOF BY DIGEST RATHER THAN BY VALUE.
//
// The wedge this serves: every consensus proposal carried the full ~1.5 MB
// space-time proof, and its per-attempt transport deadline is sized from a floor the
// impaired wire does not deliver — 24 s of wire against a 14 s deadline, measured.
// Build-immutable #5 names the structural close, which is to keep the large payload
// OFF the critical path rather than to make the deadline bigger.
//
// WHAT MAKES IT SOUND IS THAT THE BLOCK DOES NOT CHANGE. A v5 preimage folds
// BondReg.AnswerDigest in place of BondReg.Answer, so a block with the proofs shed
// hashes IDENTICALLY to the same block carrying them. The proposer signs one hash,
// every attester signs that hash, the committed bytes are the same bytes. Only what
// the proposal MESSAGE carries moves, which is transport and needs no era.
//
// The three properties below are what the relay rests on, in the order a reader
// should check them: the hash is unchanged, a receiver rebuilds the exact committed
// proof and refuses anything else, and a proposer sheds only where it has a receipt.

// THE LOAD-BEARING IDENTITY: shedding the proofs does not move the block hash.
//
// If this were ever false the relay would be a consensus change wearing a
// transport change's clothes — attesters would sign a different block from the one
// the proposer committed, and the two forms would fork. It is asserted here rather
// than inherited from Prune's own tests because THIS is the use that depends on it.
func TestSheddingTheProofsDoesNotMoveTheBlockHash(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	p := nodes[2]
	p.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := p.chain.Head()
	reg, ok := p.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration was minted, so there is no proof to shed")
	}

	b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
		Entries: []ports.Entry{mkEntry("relay")}, BondRegs: []chain.BondReg{reg}}
	// Commit the proof by digest exactly as the proposer path does
	// (chain.PopulateEra4Roots -> setBlockDigests), through the same exported
	// derivation the validity rule uses, so the two cannot disagree about which
	// bytes a digest commits.
	d := chain.AnswerDigestOf(reg.Answer)
	b.BondRegs[0].AnswerDigest = &d
	chain.Sign(b, ids[2].Signer())

	full := b.Hash()
	shed := b.Prune()
	if shed.BondRegs[0].Answer != nil {
		t.Fatal("Prune left the heavy proof in place, so nothing was shed and the comparison below is vacuous")
	}
	if shed.BondRegs[0].AnswerDigest == nil {
		t.Fatal("Prune dropped the DIGEST as well as the proof — then the shed body commits nothing and cannot be " +
			"reconstructed or validated")
	}

	t.Logf("MEASURED — one %d-byte registration, carried against shed:", len(bondRegEncode(reg)))
	t.Logf("  proposal payload carrying the proof: %d B", len(chain.Encode(b)))
	t.Logf("  proposal payload with it shed:       %d B", len(chain.Encode(&shed)))
	t.Logf("  block hash identical: %v", shed.Hash() == full)

	if shed.Hash() != full {
		t.Fatal("SHEDDING THE PROOF MOVED THE BLOCK HASH. Digest relay is then not a transport change at all: an " +
			"attester sent the shed form would sign a different block from the one the proposer committed, and the " +
			"two forms fork. Stop — the v5 preimage must fold AnswerDigest in place of Answer for this to be sound.")
	}
	if n := len(chain.Encode(&shed)); n >= len(chain.Encode(b))/2 {
		t.Fatalf("the shed proposal is %d B against the carried %d B — shedding is not removing the payload, so the "+
			"relay buys nothing on the critical path", n, len(chain.Encode(b)))
	}
}

// A RECEIVER REBUILDS THE COMMITTED PROOF, AND REFUSES ANYTHING ELSE.
//
// Reconstruction is the half that could quietly weaken validity, so the digest is
// the authority and the candidate is checked against it. AnswerDigest is folded into
// the v5 preimage in place of the proof, so it is covered by the proposer's own
// signature: a candidate whose sha256 equals it IS the committed proof. Nothing
// trusts the queue — the queue only offers candidates.
//
// The negative arm is the one that matters. A peer that poisoned the queue with a
// registration of its own devising offers a candidate that fails the digest check,
// and the block is refused rather than validated against bytes nobody committed.
func TestReconstructionRestoresTheCommittedProofAndRefusesAnyOther(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	attester, proposer := nodes[0], nodes[2]
	proposer.EnableBond(ids[2].Signer(), 2<<20)

	head, _ := proposer.chain.Head()
	reg, ok := proposer.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration was minted")
	}
	// The attester is handed the proof the ordinary way, so it has a candidate.
	attester.handleChain(proposer.ID(), ports.Message{
		Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(reg),
	})

	mkShed := func() *chain.Block {
		b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
			Entries: []ports.Entry{mkEntry("relay")}, BondRegs: []chain.BondReg{reg}}
		d := chain.AnswerDigestOf(reg.Answer)
		b.BondRegs[0].AnswerDigest = &d
		chain.Sign(b, ids[2].Signer())
		shed := b.Prune()
		return &shed
	}

	// POSITIVE: the shed block is refilled with the exact bytes the proposer committed.
	got := mkShed()
	if !attester.reconstructShedProofs(got) {
		t.Fatal("an attester holding the very registration the block commits could not reconstruct it — the relay " +
			"cannot work at all if the queue it reads is not the queue the submission lands in")
	}
	if string(got.BondRegs[0].Answer) != string(reg.Answer) {
		t.Fatal("reconstruction produced bytes that are not the committed proof")
	}
	if got.Hash() != mkShed().Hash() {
		t.Fatal("reconstruction moved the block hash — it must put back what was shed and change nothing else")
	}
	t.Logf("MEASURED — reconstruction restored %d B of proof from the attester's own queue, hash unchanged",
		len(got.BondRegs[0].Answer))

	// NEGATIVE: the queue holds a registration for this validator whose proof is NOT
	// the committed one. It must be refused, not substituted.
	poisoned := reg
	poisoned.Answer = append([]byte(nil), reg.Answer...)
	poisoned.Answer[0] ^= 0xff
	attester.pendingBondRegs = nil
	attester.pendingBondRegs = append(attester.pendingBondRegs, pendingBondReg{R: poisoned})

	bad := mkShed()
	if attester.reconstructShedProofs(bad) {
		t.Fatal("A CANDIDATE THAT IS NOT THE COMMITTED PROOF WAS SUBSTITUTED. The digest is covered by the proposer's " +
			"signature, so only bytes whose sha256 equals it are the committed proof; accepting any other means a peer " +
			"who poisons the pending queue decides what a block says, which is a consensus break and not a relay bug.")
	}
	if attester.Stats.ProposalsNeedingBodies == 0 {
		t.Fatal("the refusal was not counted — a miss must be visible, or the evidence and the reality drift " +
			"silently until a round stalls")
	}
	t.Log("negative control: a non-matching candidate is refused and counted, never substituted")
}

// A PROPOSER SHEDS ONLY WHERE IT HAS A RECEIPT.
//
// The relay is only safe from stalls because the proposer does not guess. Two kinds
// of evidence let it shed — the peer acknowledged this node's registration, or the
// peer authored it — and nothing else does, however likely.
func TestAProposerShedsOnlyForPeersItHasEvidenceFor(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	proposer := nodes[2]
	proposer.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := proposer.chain.Head()
	reg, ok := proposer.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration was minted")
	}
	b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
		BondRegs: []chain.BondReg{reg}}

	acked, silent := ids[0].NodeID(), ids[1].NodeID()
	proposer.ownRegAcks = map[ports.NodeID]bool{acked: true}

	t.Logf("MEASURED — which peers may be sent the proposal with its proof shed:")
	t.Logf("  a peer that ACKNOWLEDGED the registration: %v", canRebuild(proposer, acked, b))
	t.Logf("  a peer that did not:                       %v", canRebuild(proposer, silent, b))
	t.Logf("  the validator that AUTHORED it:            %v", canRebuild(proposer, proposer.ID(), b))

	if !canRebuild(proposer, acked, b) {
		t.Fatal("a peer that acknowledged this exact registration was not shed for — then the relay never engages " +
			"and the proposal carries the proof to everyone, which is the state this change exists to leave")
	}
	if canRebuild(proposer, silent, b) {
		t.Fatal("A PEER WITH NO RECEIPT WAS SHED FOR. That is a guess about the network, and a wrong guess costs the " +
			"round it is wrong in — the fallback is a round trip PLUS the same payload, measured as worse than " +
			"carrying. Evidence only.")
	}
	// And the receipt dies with the bytes it is about.
	proposer.SubmitBondRenewal([]ports.NodeID{acked})
	if proposer.ownRegAcks[acked] {
		t.Fatal("a receipt for the PREVIOUS registration survived a fresh broadcast — an ack is about specific bytes, " +
			"and carrying it forward claims a peer holds a proof it was never sent")
	}
	t.Log("control: a fresh broadcast clears every prior receipt, so evidence never outlives the bytes it is about")
}

// AND THE PROOF CROSSED TWICE PER ROUND, WHICH THE FIRST READING MISSED.
//
// The proposal is not the only message that carries the block. The prepare-QC that
// opens the precommit leg carries the WHOLE BLOCK a second time — prepareQCEnv.Raw
// is the same chain.Encode(b) the proposal sent — so every attester receives the
// heavy proof TWICE per round, and shedding only the proposal halves a cost that is
// paid twice.
//
// The precommit leg is also where the evidence is strongest. A peer named in the
// prepare-QC PREPARED on this block: it received it, reconstructed it, validated it
// and signed its hash. It cannot have done that without holding the proof. That is a
// stronger claim than an acknowledgement, and it is available for free at exactly
// the moment the second copy would otherwise be sent.
func TestTheProofCrossesTwicePerRoundAndBothLegsCanShedIt(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	p := nodes[2]
	p.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := p.chain.Head()
	reg, ok := p.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration was minted")
	}
	b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
		Entries: []ports.Entry{mkEntry("relay")}, BondRegs: []chain.BondReg{reg}}
	d := chain.AnswerDigestOf(reg.Answer)
	b.BondRegs[0].AnswerDigest = &d
	chain.Sign(b, ids[2].Signer())

	raw := chain.Encode(b)
	shedBlk := b.Prune()
	shedRaw := chain.Encode(&shedBlk)
	qc := []chain.Attestation{chain.AttestAt(b, ids[0].Signer(), 0, chain.PhasePrepare, p.chainID())}

	propose, _ := cborMarshalForTest(proposeEnv{Raw: raw, Round: 0})
	proposeShed, _ := cborMarshalForTest(proposeEnv{Raw: shedRaw, Round: 0})
	prepQC, _ := cborMarshalForTest(prepareQCEnv{Raw: raw, Round: 0, QC: qc})
	prepQCShed, _ := cborMarshalForTest(prepareQCEnv{Raw: shedRaw, Round: 0, QC: qc})

	carried := len(propose) + len(prepQC)
	shed := len(proposeShed) + len(prepQCShed)

	t.Logf("MEASURED — what ONE attester receives for ONE round carrying ONE %d-byte registration:", len(reg.Answer))
	t.Logf("  proposal      carried %8d B   shed %6d B", len(propose), len(proposeShed))
	t.Logf("  prepare-QC    carried %8d B   shed %6d B", len(prepQC), len(prepQCShed))
	t.Logf("  ROUND TOTAL   carried %8d B   shed %6d B   -> %.0fx", carried, shed, float64(carried)/float64(shed))

	// THE ASSERTION THAT CATCHES THE MISSED LEG: the prepare-QC must carry the block,
	// or this measurement is describing a shape the product does not have.
	if len(prepQC) < len(raw) {
		t.Fatalf("the prepare-QC is %d B against a %d-byte block — it no longer carries the block, so the second "+
			"crossing this measures does not happen and the precommit-leg shed is dead code. Re-read "+
			"prepareQCEnv before citing any figure here.", len(prepQC), len(raw))
	}
	// And shedding must remove the payload from BOTH legs, not one.
	if len(proposeShed) >= len(propose)/2 || len(prepQCShed) >= len(prepQC)/2 {
		t.Fatalf("shedding left %d B of %d on the proposal and %d B of %d on the prepare-QC — a leg that still "+
			"carries the proof pays the whole cost, whatever the other one saves",
			len(proposeShed), len(propose), len(prepQCShed), len(prepQC))
	}
}

// cborMarshalForTest encodes an envelope the way the propose path encodes it, so the
// sizes above are wire sizes and not struct estimates.
func cborMarshalForTest(v any) ([]byte, error) { return cbor.Marshal(v) }

// THE COMMIT PATH NEVER SHEDS, AND THAT IS A BOUNDARY WORTH A TEST.
//
// Digest relay is sound for the two GATHER legs because their receiver is only
// being asked to ATTEST, and it is a peer that demonstrably holds the proof — so the
// bytes are put back before anything judges the block, and what it signs is the full
// block's hash. Commit is different in kind: a replica that accepts a block STORES
// it, and a stored Answer-less registration is one whose space-time proof can never
// be re-verified. validateBondRegs refuses exactly that above the trust floor,
// because trusting it would let a peer skip the proof and forge standing.
//
// So the relay stops at the gather. This asserts the boundary rather than trusting
// the next reader to notice it: extending the shed to MsgCommitBlock would look like
// a natural generalization of a change that saves 3 MB a round, and it is a
// no-discount break.
func TestTheCommitPathAlwaysCarriesTheProof(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	p := nodes[2]
	p.EnableBond(ids[2].Signer(), 2<<20)
	head, _ := p.chain.Head()
	reg, ok := p.RegisterBondReg(head)
	if !ok {
		t.Fatal("VACUOUS: no registration was minted")
	}
	b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
		Entries: []ports.Entry{mkEntry("commit")}, BondRegs: []chain.BondReg{reg}}
	d := chain.AnswerDigestOf(reg.Answer)
	b.BondRegs[0].AnswerDigest = &d
	chain.Sign(b, ids[2].Signer())

	// A replica handed the SHED form at commit must not be able to store it: the
	// proof is unverifiable, which is the property the trust floor protects.
	shed := b.Prune()
	if !shed.HeavyProofsShed() {
		t.Fatal("the fixture's shed block does not report its proofs shed, so the arm below proves nothing")
	}
	attester := nodes[0]
	before := attester.chain.Len()
	attester.handleChain(p.ID(), ports.Message{Kind: ports.MsgCommitBlock, Data: chain.Encode(&shed)})
	if attester.chain.Len() != before {
		t.Fatal("A REPLICA COMMITTED A BLOCK WHOSE SPACE-TIME PROOF IT NEVER SAW. A stored Answer-less registration " +
			"can never be re-verified, so accepting one lets a peer skip the proof and forge standing — the " +
			"no-discount break the trust floor exists to refuse. The digest relay must stop at the gather legs.")
	}
	_ = net
	t.Log("a shed block offered at COMMIT is not stored — the relay's boundary holds")
}

// canRebuild is peerCanReconstruct's verdict alone, for the assertions that are about
// whether a peer qualifies rather than about why it did not. The reason is asserted
// where it is the subject (TestThePrepareQCLegShedsOnEveryEvidenceAndNotOnQuorumMembershipAlone).
func canRebuild(n *Node, v ports.NodeID, b *chain.Block) bool {
	ok, _ := n.peerCanReconstruct(v, b)
	return ok
}
