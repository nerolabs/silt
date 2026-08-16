package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// The #441 oracle — BORN RED on this branch (the wedge-oracle pattern): the
// deterministic repro of the mature-regime PUBLISH starvation observed on run
// a56ac10-42834 (zero entry-blocks committed post-latch across 33+ heights;
// the live-captured client error 'propose height 45 round 1: insufficient
// valid attestations: 2 prepares of 2 gathered').
//
// THE MECHANISM UNDER TEST (code-cited): the #432 view-change machinery is
// DRAIN-ONLY — recordRoundChange fires maybeProposeBondDrain's block at the
// new round; ProposeEntry has no round seat and no round-aware retry. So under
// the WAN-observed schedule — the drain designee's proposal reaches the
// attesters' (h, r0, prepare) slots first — the entry proposal is refused at
// r0 (never-sign-twice) and CANNOT win any later round either: every new-view
// re-proposes the drain. The publish starves for as long as renewals keep the
// drain armed, which at steady state is forever.
//
// THE ORACLE PROPERTY (what a FIXED system must satisfy): with the chain live
// and committing, a client that re-submits its entry each height commits it
// within PUBLISH_HEIGHT_BUDGET heights. RED today: the entry never commits.
// The fix is research/PE-gated (#441 — proposal scheduling touches the
// certified #432 round machinery); this oracle is the consult's evidence and
// the fix's merge gate, parked on its branch until then.
const publishHeightBudget = 3

func TestModelCheck_441_PublishStarvedAcrossRounds(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	maturers := nodes[4:]
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}

	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { honestSlashed = true })
	}

	entry := mkEntry("the-starved-publish")
	entryCommitted := func() bool {
		blocks := nodes[4].Chain().Blocks(0)
		for i := range blocks {
			for _, e := range blocks[i].Entries {
				if e.Root == entry.Root {
					return true
				}
			}
		}
		return false
	}

	// The WAN-observed schedule, repeated per height: the drain designee's
	// sweep reaches the attesters first (its prepares take the (h, r0) slots),
	// the client's entry proposal arrives second and is refused everywhere,
	// the escape round's new-view seat belongs to the drain (recordRoundChange),
	// the drain commits at r1, the client re-submits at the new head. At WAN
	// cadence this is the common case: the sweep is a local 30s timer while the
	// client's proposal rides a cross-region round trip.
	for h := 0; h < publishHeightBudget; h++ {
		refill() // steady state: renewals keep every drain queue armed
		_, height := nodes[4].chain.Head()

		// The (height, r0) drain designee sweeps first — its proposal PREPARES
		// at every attester (slots consumed) but its final precommit replies
		// are held, so r0 cannot complete (the crossed-race face; without the
		// hold the drain just commits at r0 and the entry loses identically).
		desig := byID[nodes[4].designatedProposer(height, 0)]
		if desig == nil {
			t.Fatalf("h%d: designated (h, r0) proposer is not a fixture node", height)
		}
		holdDesigPrecommits := func(m simnet.HeldMsg) bool {
			return m.Kind == ports.MsgPrecommitReply && m.To == desig.id
		}
		desig.maybeProposeBondDrain()
		drainHeldExcept(t, net, holdDesigPrecommits)

		// The client's entry proposal (from a bonded, proposer-ELIGIBLE maturer
		// that is not the designee — eligibility is not the problem, the
		// consumed slots are). It must fail: every attester already signed the
		// drain's (h, r0, prepare).
		var pub *Node
		for _, m := range maturers {
			if m.id != desig.id {
				pub = m
				break
			}
		}
		var pubErr error
		pubDone := false
		pub.ProposeEntry(entry, attListExcept(all[4:], pub.id), all, 0, func(err error) { pubDone, pubErr = true, err })
		drainHeldExcept(t, net, holdDesigPrecommits)
		if pubDone && pubErr == nil {
			// The entry landed — the oracle property holds at this height.
			break
		}

		// The escape: sweeps → round-change → the (h, r1) new-view — which
		// recordRoundChange gives to the DRAIN. Release the held replies so the
		// height resolves (r0's suspended drain quorum or r1's new-view — either
		// way a DRAIN block commits and the head moves past the entry's height).
		refill()
		sweepRounds(t, net, holdDesigPrecommits, maturers)
		refill()
		sweepRounds(t, net, holdDesigPrecommits, maturers)
		drainHeld(t, net, fifo)
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)

		_, after := nodes[4].chain.Head()
		if after == height {
			t.Fatalf("h%d: the height never resolved — this schedule wants starvation-with-progress, not a wedge (the #432 escape must still commit the DRAIN)", height)
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed during the publish/drain contention")
	}
	if !entryCommitted() {
		_, head := nodes[4].chain.Head()
		t.Fatalf("#441 PUBLISH STARVATION: the chain is LIVE (head next=%d, drain blocks committing every height) but the client's entry NEVER committed within %d heights of continuous re-submission — the round machinery's new-view seat is drain-only, so the publish can win no round once its r0 slot is consumed", head, publishHeightBudget)
	}
}

func attListExcept(ids []ports.NodeID, except ports.NodeID) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(ids))
	for _, id := range ids {
		if id != except {
			out = append(out, id)
		}
	}
	return out
}
