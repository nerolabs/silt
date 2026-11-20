package node

import (
	"github.com/nerolabs/silt/ports"
)

// A PROPOSAL ONE HEIGHT AHEAD WAITS FOR ITS PARENT INSTEAD OF BEING REFUSED.
//
// A height commits at its proposer and then reaches every other validator by the
// commit broadcast, so for a moment the validators hold different heads. The next
// height's designee may learn of the commit first and propose at once; an attester
// still one block behind then sees a block that "does not extend the local head" and
// refuses it. It has not judged the block, it has only not yet heard of its parent,
// and the parent is already on the wire to it. Refusing burns the designee's round:
// the designee has signed its slot, so the height waits out a full view change for
// a value every validator would have accepted a few hundred milliseconds later.
//
// So an attester exactly one height behind holds the request instead, and answers
// it when its head moves or when the hold expires, whichever comes first. The hold
// is half the request deadline, so the answer still reaches a proposer that is
// waiting for it. The answer is the ordinary verdict on the ordinary rules: nothing
// is accepted that would not have been accepted on arrival after the parent.
//
// Only an eligible proposer's request is held, and only a few at a time. A full
// park refuses as before, so a peer that floods it costs honest proposers at most
// the behaviour without the park.

// maxParkedProposals bounds what a peer can make this node hold.
const maxParkedProposals = 4

type parkedProposal struct {
	from ports.NodeID
	msg  ports.Message
}

// parkProposal holds a prepare request for height head+2, the block after the one
// this node has not yet received. It reports whether it held the request; when it
// did, the caller must not answer it.
func (n *Node) parkProposal(from ports.NodeID, msg ports.Message, height uint64) bool {
	if n.answeringParked || len(n.parkedProposals) >= maxParkedProposals {
		return false
	}
	if _, next := n.chain.Head(); height != next+1 || !n.chain.ProposerEligible(from) {
		return false
	}
	n.parkedProposals = append(n.parkedProposals, parkedProposal{from: from, msg: msg})
	n.logf(ports.LogDebug, "gather/prepare: held a proposal one height ahead until its parent arrives",
		"from", from, "height", height, "held", len(n.parkedProposals))
	rid := msg.RID
	n.clock.AfterFunc(n.cfg.RequestTimeout/2, func() {
		n.answerParkedProposals(func(p parkedProposal) bool { return p.from == from && p.msg.RID == rid })
	})
	return true
}

// answerParkedProposals answers the held requests that due selects, on the ordinary
// rules: all of them when the head moves, one of them when its hold expires. A
// request answered here is never held again.
func (n *Node) answerParkedProposals(due func(parkedProposal) bool) {
	if len(n.parkedProposals) == 0 {
		return
	}
	var answer []parkedProposal
	kept := n.parkedProposals[:0]
	for _, p := range n.parkedProposals {
		if due(p) {
			answer = append(answer, p)
		} else {
			kept = append(kept, p)
		}
	}
	n.parkedProposals = kept
	n.answeringParked = true
	for _, p := range answer {
		n.handleChain(p.from, p.msg)
	}
	n.answeringParked = false
}
