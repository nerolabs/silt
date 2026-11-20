package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// AN ATTESTER ONE BLOCK BEHIND HOLDS THE NEXT PROPOSAL UNTIL ITS PARENT LANDS.
//
// Height 1 commits at its proposer, and the commit broadcast to one validator, the
// laggard, is still in flight. The next proposer has already heard of height 1 and
// proposes height 2 at once, gathering on the laggard among others. The laggard
// cannot yet judge a block whose parent it has never seen. Refusing it is not a
// verdict, it is a delivery gap, and it costs the proposer its round: the height
// then waits out a full view change for a block every validator accepts a moment
// later. The laggard must hold the request and answer it when height 1 arrives.
func TestAnAttesterOneBlockBehindHoldsTheNextProposalUntilItsParentLands(t *testing.T) {
	nodes, ids, net, _, cfg := tier2AnchorNet(t, 4)
	a, b, lag := nodes[0], nodes[1], nodes[3]
	all := make([]ports.NodeID, len(ids))
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	commitToLag := func(m simnet.HeldMsg) bool { return m.To == lag.ID() && m.Kind == ports.MsgCommitBlock }

	// Height 1 commits everywhere except at the laggard. The proposer's own callback
	// waits on every commit acknowledgement, the laggard's included, so the proposer's
	// head is the witness here.
	a.ProposeEntry(mkEntry("parent"), []ports.NodeID{all[1], all[2]}, all, cfg.Quorum, func(error) {})
	drainHeldExcept(t, net, commitToLag)
	if _, next := a.chain.Head(); next != 2 {
		t.Fatalf("height 1 did not commit at its proposer (next height %d)", next)
	}
	if _, next := b.chain.Head(); next != 2 {
		t.Fatalf("the next proposer has not heard of height 1 (next height %d), so this test proves nothing", next)
	}
	if _, next := lag.chain.Head(); next != 1 {
		t.Fatalf("the laggard already holds height 1 (next height %d), so this test proves nothing", next)
	}

	// Height 2 gathers on the laggard while height 1 is still on its way to it. The
	// gather needs the laggard's prepare: with a and the proposer alone it falls short.
	var err2 error
	done2 := false
	b.ProposeEntry(mkEntry("child"), []ports.NodeID{all[0], all[3]}, all, cfg.Quorum+1, func(e error) { err2, done2 = e, true })
	drainHeldExcept(t, net, commitToLag)
	if done2 && err2 != nil {
		t.Fatalf("THE LAGGARD REFUSED A PROPOSAL WHOSE PARENT WAS STILL ON THE WIRE TO IT: %v. "+
			"It answered a delivery gap as a verdict, and the proposer lost its round", err2)
	}

	// Height 1 lands at the laggard; the held proposal is answered on the ordinary rules.
	drainHeld(t, net, fifo)
	if !done2 {
		t.Fatal("the held proposal was never answered after its parent landed")
	}
	if err2 != nil {
		t.Fatalf("height 2 did not commit once the laggard caught up: %v", err2)
	}
	for i, nd := range nodes {
		if _, next := nd.chain.Head(); next != 3 {
			t.Fatalf("validator %d is at next height %d, want 3", i, next)
		}
	}
}

// A held request is answered when its hold expires even if the parent never comes,
// so a proposer waiting on it hears the ordinary refusal inside its deadline.
func TestAHeldProposalIsAnsweredWhenItsHoldExpires(t *testing.T) {
	nodes, ids, net, _, cfg := tier2AnchorNet(t, 4)
	a, b, lag := nodes[0], nodes[1], nodes[3]
	all := make([]ports.NodeID, len(ids))
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	commitToLag := func(m simnet.HeldMsg) bool { return m.To == lag.ID() && m.Kind == ports.MsgCommitBlock }
	a.ProposeEntry(mkEntry("parent"), []ports.NodeID{all[1], all[2]}, all, cfg.Quorum, func(error) {})
	drainHeldExcept(t, net, commitToLag)

	b.ProposeEntry(mkEntry("child"), []ports.NodeID{all[0], all[3]}, all, cfg.Quorum+1, func(error) {})
	drainHeldExcept(t, net, commitToLag)
	if len(lag.parkedProposals) != 1 {
		t.Fatalf("the laggard holds %d proposals, want 1", len(lag.parkedProposals))
	}
	lag.answerParkedProposals(func(p parkedProposal) bool { return p.from == b.ID() })
	if len(lag.parkedProposals) != 0 {
		t.Fatal("an expired hold left the request held")
	}
	refusals := 0
	for _, m := range net.Pending() {
		if m.From == lag.ID() && m.To == b.ID() && m.Kind == ports.MsgAttestReply {
			refusals++
		}
	}
	if refusals != 1 {
		t.Fatalf("the expired hold sent %d answers to the proposer, want exactly 1", refusals)
	}
	if len(lag.parkedProposals) != 0 {
		t.Fatal("answering an expired hold held the request again")
	}
}
