package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// A VIEW CHANGE RE-PROPOSES THE LOCKED BLOCK WITH ITS PROOFS IN IT.
//
// A validator locks on the block it precommits, and it stores the bytes the wire
// delivered. When that prepare-QC arrived by digest, those bytes are the shed form: a
// bond registration commits its proof by AnswerDigest and carries no Answer. If the
// height then needs a view change, the round's designee re-proposes the highest
// carried lock. Re-proposing the shed bytes as they are sends every peer without a
// receipt a block it cannot rebuild, and the designee's own replica refuses to store a
// block above the trust floor whose proof it cannot re-verify. The height stalls, round
// after round, on a value every validator is locked on.
//
// The re-proposal must therefore put back what the wire left out before it gathers,
// exactly as the prepare and precommit handlers do before they judge a block. A
// designee that cannot do that declines to lead the round rather than gather on a
// block it can never commit; the round then times out to the next designee.
func TestAForcedReProposalCarriesTheProofsItWasLockedOnWithout(t *testing.T) {
	type arm struct {
		name          string
		designeeHolds bool
	}
	for _, a := range []arm{{"designee holds the proof", true}, {"designee does not hold the proof", false}} {
		t.Run(a.name, func(t *testing.T) {
			nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
			designee, author, witness := nodes[0], nodes[1], nodes[2]

			// A third party's registration, the ordinary case on a live chain.
			author.EnableBond(ids[1].Signer(), 2<<20)
			head, _ := author.chain.Head()
			reg, ok := author.RegisterBondReg(head)
			if !ok {
				t.Fatal("VACUOUS: no registration was minted, so there is no proof to shed")
			}
			if a.designeeHolds {
				designee.handleChain(author.ID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(reg)})
			}

			b := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: head,
				Entries: []ports.Entry{mkEntry("forced")}, BondRegs: []chain.BondReg{reg}}
			d := chain.AnswerDigestOf(reg.Answer)
			b.BondRegs[0].AnswerDigest = &d
			chain.Sign(b, ids[1].Signer())
			shed := b.Prune()
			if !shed.HeavyProofsShed() {
				t.Fatal("the fixture's lock is not in the shed form, so this test proves nothing")
			}
			forced := &nodeLock{Round: 0, Hash: b.Hash(), Block: chain.Encode(&shed)}

			for _, n := range nodes[1:] {
				designee.chainSyncSeed = append(designee.chainSyncSeed, n.ID())
			}
			before := witness.Stats.ProposalsNeedingBodies
			designee.proposeAtNewView(designee.roundsFor(), 1, nil, forced)
			proposals := 0
			for _, m := range net.Pending() {
				if m.From == designee.ID() && m.Kind == ports.MsgProposeBlock {
					proposals++
				}
			}
			drainHeld(t, net, func([]simnet.HeldMsg) int { return 0 })

			// The witness holds no copy of the proof and reported none, so the designee
			// owes it the block carried. A shed block here is one it can never rebuild.
			if got := witness.Stats.ProposalsNeedingBodies - before; got != 0 {
				t.Fatalf("A VIEW CHANGE RE-PROPOSED A LOCKED BLOCK WITHOUT ITS PROOF: a peer with no receipt was "+
					"sent the shed form and asked for the bodies %d time(s). The designee re-proposed the bytes the "+
					"wire delivered to it, so the leg that should carry the proof carried only its digest, and the "+
					"designee's own commit of that block is refused at the trust floor", got)
			}
			if a.designeeHolds && proposals == 0 {
				t.Fatal("VACUOUS: a designee holding the proof sent no proposal at all, so nothing was re-proposed")
			}
			if !a.designeeHolds && proposals != 0 {
				t.Fatalf("a designee that cannot rebuild the locked block's proof gathered on it anyway (%d "+
					"proposals sent); it can never commit that block, so leading the round only burns it", proposals)
			}
			if designee.bondDrainInFlight {
				t.Fatal("the re-proposal left a proposal marked in flight, which blocks every later round on this node")
			}
		})
	}
}
