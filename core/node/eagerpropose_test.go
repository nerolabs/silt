package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// AN EARLY OFFER LETS ONLY THE DESIGNEE PROPOSE, AND IS NEVER AN IDLE SWEEP.
//
// Between sweeps, a validator holding entries is offered the sweep's proposal
// decision. The round's designee proposes on it. Every other validator declines
// without counting the offer toward the takeover walk: that walk measures a silent
// designee in sweeps, and an offer that counted would hand the height to the next
// rank after a few publishes instead of a few sweeps.
func TestAnEarlyOfferLetsOnlyTheDesigneeProposeAndNeverCountsAsASweep(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	for _, nd := range nodes {
		for _, id := range all {
			if id != nd.ID() {
				nd.chainSyncSeed = append(nd.chainSyncSeed, id)
			}
		}
		nd.queuePendingEntry(mkEntry("waiting"))
	}
	props := nodes[0].chain.EligibleProposers()
	if len(props) != len(nodes) {
		t.Fatalf("%d eligible proposers, want %d", len(props), len(nodes))
	}
	_, height := nodes[0].chain.Head()
	designee := props[height%uint64(len(props))]

	proposalsFrom := func(id ports.NodeID) int {
		c := 0
		for _, m := range net.Pending() {
			if m.From == id && m.Kind == ports.MsgProposeBlock {
				c++
			}
		}
		return c
	}
	for _, nd := range nodes {
		if nd.ID() == designee {
			continue
		}
		nd.proposeDrain(true)
		if got := proposalsFrom(nd.ID()); got != 0 {
			t.Fatalf("a validator that is not the designee proposed %d time(s) on an early offer", got)
		}
		if nd.drainWaitSweeps != 0 {
			t.Fatalf("an early offer counted as an idle sweep (wait %d): it hurries the takeover walk", nd.drainWaitSweeps)
		}
	}
	for _, nd := range nodes {
		if nd.ID() == designee {
			nd.proposeDrain(true)
			if proposalsFrom(nd.ID()) == 0 {
				t.Fatal("the designee did not propose on an early offer while holding entries")
			}
		}
	}
}

// With ProposeDelay off, an entry arriving arms nothing: the sweep alone proposes.
func TestAnEntryArmsNoEarlyOfferWhenTheDelayIsOff(t *testing.T) {
	nodes, _, _, _, _ := tier2AnchorNet(t, 4)
	nd := nodes[0]
	nd.queuePendingEntry(mkEntry("off"))
	if nd.proposeArmed {
		t.Fatal("an entry armed an early offer with ProposeDelay off")
	}
	nd.cfg.ProposeDelay = 1000 * ports.Millisecond
	nd.queuePendingEntry(mkEntry("on"))
	if !nd.proposeArmed {
		t.Fatal("an entry armed no early offer with ProposeDelay set")
	}
}
