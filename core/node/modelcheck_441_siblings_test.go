package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// The #441 certification's §6 sibling oracles — the merge gate around the
// certified entries-are-mempool fix, alongside the (formerly born-RED)
// starvation oracle in modelcheck_441_publish_starvation_test.go:
//
//   §6.1 launch-face: the escape fires with ENTRY-ONLY pending work
//   §6.2 adversarial designee dropping entries → commits within O(f+1) heights
//   §6.3 entry-vs-entry contention under a full budget → FIFO, no starvation
//   §6.4 entry flood does not crowd out consensus-critical renewals
//   §6.5 S1-with-entries: a pending entry never displaces a locked value
//        (I1-across-rounds regression with entries in play)

// findEntryHeight reports the height whose committed block carries root, or 0.
func findEntryHeight(nd *Node, root ports.Hash) uint64 {
	blocks := nd.Chain().Blocks(0)
	for i := range blocks {
		for _, e := range blocks[i].Entries {
			if e.Root == root {
				return blocks[i].Height
			}
		}
	}
	return 0
}

// TestModelCheck_441_LaunchFace_EntryOnlyWorkArmsTheEscape is §6.1 — the soak
// run 9453325-7258 face: a height whose r0 prepare slots are consumed, on a
// network with EMPTY renewal queues, must still escape and commit, driven by
// nothing but the pending entry (pre-fix, maybeAdvanceRound quiesced and the
// height stalled ~361s until unrelated renewal traffic arrived).
func TestModelCheck_441_LaunchFace_EntryOnlyWorkArmsTheEscape(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, 3)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
		nd.pendingBondRegs = nil // ENTRY-ONLY: the launch-face premise — no drain work anywhere
	}

	// The entry is submitted everywhere, then the (h1, r0) designee proposes
	// its entry-carrying block and its precommit replies are HELD — r0 slots
	// consumed, the height stuck, and the ONLY pending work network-wide is
	// the entry (the r0 designee's own queue emptied into the held proposal;
	// the other three still hold their mempool copies).
	entry := mkEntry("launch-face-entry")
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	nodes[0].SubmitEntry(entry, all)
	drainHeld(t, net, fifo)
	desig := byID[nodes[0].designatedProposer(1, 0)]
	holdDesig := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.To == desig.id
	}
	desig.maybeProposeBondDrain()
	drainHeldExcept(t, net, holdDesig)
	if h := findEntryHeight(nodes[0], entry.Root); h != 0 {
		t.Fatal("setup: the held r0 proposal must not have committed yet")
	}

	// The escape must arm from the entry alone: sweeps → round-change → the
	// r1 new-view folds a mempool copy → commit. Pre-fix this loop did
	// nothing (quiesce) and the assert below failed.
	sweepRounds(t, net, holdDesig, nodes)
	sweepRounds(t, net, holdDesig, nodes)
	drainHeld(t, net, fifo)
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	h := findEntryHeight(nodes[0], entry.Root)
	if h == 0 {
		t.Fatal("§6.1 LAUNCH-FACE: entry-only pending work did not arm the escape — the height stayed stuck with no drain traffic to ride (the 361s soak stall)")
	}
	blk := nodes[0].Chain().Blocks(h)
	if len(blk) == 0 || blk[0].CommitRound == 0 {
		t.Fatalf("VACUOUS: the entry committed at round %d — the escape (round ≥1) did not do the committing, so this run does not exercise entry-armed round advance", blk[0].CommitRound)
	}
}

// TestModelCheck_441_AdversarialDesigneeDropsEntry is §6.2 — the certified
// fairness bound: a designee that never receives (models: silently drops) the
// entry delays it at most its own height; the next honest designee's block
// carries it. Commits within O(f+1)=2 heights for f=1.
func TestModelCheck_441_AdversarialDesigneeDropsEntry(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	_, h0 := nodes[4].chain.Head()
	dropper := byID[nodes[4].designatedProposer(h0, 0)]

	// Submit to everyone EXCEPT the (h0, r0) designee — the drop, modeled at
	// the wire (a Byzantine designee that discards the submit is
	// indistinguishable from one that never received it).
	entry := mkEntry("dropped-by-designee")
	rest := make([]ports.NodeID, 0, len(all)-1)
	for _, id := range all {
		if id != dropper.id {
			rest = append(rest, id)
		}
	}
	// Deliver the submit from a non-eligible probe so no node self-queues
	// outside `rest`: every eligible proposer except the dropper now holds it.
	raw := entryEncode(entry)
	probe := net.Endpoint(identity.FromSeed(8995).NodeID())
	for _, id := range rest {
		probe.Send(id, ports.Message{Kind: ports.MsgSubmitEntry, Data: raw})
	}
	drainHeld(t, net, fifo)

	// Drive: the dropper's height commits (its drain block, no entry), then
	// the next designee's height must carry the entry. refill() keeps the
	// renewal traffic flowing — the adversarial case is drop-under-load.
	for i := 0; i < 2; i++ {
		refill()
		_, hh := nodes[4].chain.Head()
		d := byID[nodes[4].designatedProposer(hh, 0)]
		d.maybeProposeBondDrain()
		drainHeld(t, net, fifo)
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)
		if findEntryHeight(nodes[4], entry.Root) != 0 {
			break
		}
	}
	h := findEntryHeight(nodes[4], entry.Root)
	if h == 0 {
		t.Fatal("§6.2 FAIRNESS BOUND BROKEN: the entry did not commit within O(f+1)=2 heights of a single dropping designee — permanent minority censorship")
	}
	if h > h0+1 {
		t.Fatalf("§6.2: entry committed at h%d — outside the O(f+1) bound from h%d (one adversarial designee must cost at most one height)", h, h0)
	}
}

// TestModelCheck_441_EntryFIFONoInternalStarvation is §6.3 — under a budget
// that admits ONE entry per block, three queued entries commit across
// successive blocks in FIFO submit order; none is deferred past the batch.
func TestModelCheck_441_EntryFIFONoInternalStarvation(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
		nd.cfg.MaxEntryBytesPerBlock = 1 // 1 byte: exactly one entry folds per block (≥1 always folds)
	}
	e1, e2, e3 := mkEntry("fifo-1"), mkEntry("fifo-2"), mkEntry("fifo-3")
	for _, e := range []ports.Entry{e1, e2, e3} {
		nodes[4].SubmitEntry(e, all)
		drainHeld(t, net, fifo)
	}
	for i := 0; i < 3; i++ {
		refill()
		_, hh := nodes[4].chain.Head()
		d := byID[nodes[4].designatedProposer(hh, 0)]
		d.maybeProposeBondDrain()
		drainHeld(t, net, fifo)
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)
	}
	h1, h2, h3 := findEntryHeight(nodes[4], e1.Root), findEntryHeight(nodes[4], e2.Root), findEntryHeight(nodes[4], e3.Root)
	if h1 == 0 || h2 == 0 || h3 == 0 {
		t.Fatalf("§6.3 INTERNAL STARVATION: not all queued entries committed under contention (h1=%d h2=%d h3=%d)", h1, h2, h3)
	}
	if !(h1 <= h2 && h2 <= h3) {
		t.Fatalf("§6.3 FIFO VIOLATED: commit order %d,%d,%d does not follow submit order — a later entry queue-jumped", h1, h2, h3)
	}
}

// TestModelCheck_441_EntryFloodDoesNotCrowdRenewals is §6.4 — Addition 1's
// dual: with the entry budget saturated by a flood, the designee's block STILL
// carries the pending bond renewal (the budgets are independent; consensus
// standing cannot be starved by publish traffic).
func TestModelCheck_441_EntryFloodDoesNotCrowdRenewals(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
		nd.cfg.MaxEntryBytesPerBlock = 200 // a couple of entries per block — the flood must queue
	}
	for i := 0; i < 24; i++ {
		nodes[4].SubmitEntry(mkEntry("flood-"+string(rune('a'+i))), all)
	}
	drainHeld(t, net, fifo)
	refill() // the renewal under threat (reg9's registration, pending everywhere)
	_, hh := nodes[4].chain.Head()
	d := byID[nodes[4].designatedProposer(hh, 0)]
	d.maybeProposeBondDrain()
	drainHeld(t, net, fifo)
	blk := nodes[4].Chain().Blocks(hh)
	if len(blk) == 0 {
		t.Fatal("setup: the designee's block did not commit")
	}
	if len(blk[0].BondRegs) == 0 {
		t.Fatal("§6.4 CROWD-OUT: the entry flood displaced the pending bond renewal from the designee's block — the budgets are not independent and consensus standing can be starved by publish traffic")
	}
	if len(blk[0].Entries) == 0 {
		t.Fatal("§6.4: the block carried no entries at all under a saturated queue — at least one must always fold")
	}
	if n := len(byID[d.id].pendingEntries); n == 0 {
		t.Fatal("§6.4 premise: the flood was expected to exceed one block's entry budget (nothing left queued — raise the flood or lower the budget)")
	}
}

// TestModelCheck_441_S1WithEntries_LockNeverDisplaced is §6.5 — the
// I1-across-rounds regression with entries in play: the S1 schedule (a real
// >2/3-weight quorum for X delayed at round 0) runs while an entry sits in
// every mempool. The lock rule must carry X — the entry must NOT displace the
// locked value at the contested height (the forced re-proposal path bypasses
// the mempool fold by construction), and the entry commits at a LATER height
// (liveness preserved, no honest slash).
func TestModelCheck_441_S1WithEntries_LockNeverDisplaced(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	maturers := nodes[4:]

	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { honestSlashed = true })
	}

	// An entry pending EVERYWHERE while the height is contested.
	entry := mkEntry("entry-during-s1")
	nodes[4].SubmitEntry(entry, all)
	drainHeld(t, net, fifo)

	holdM2Reply := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.From == id(6) && m.To == id(4)
	}
	prev, ch := nodes[4].chain.Head()
	blkX := &chain.Block{Version: chain.BlockVersion, Height: ch, Prev: prev, Entries: []ports.Entry{mkEntry("X-s1-entries")}}
	var xDone bool
	nodes[4].proposeBlock(blkX, []ports.NodeID{id(5), id(6)}, all, 0, func(err error) { xDone = true })
	drainHeldExcept(t, net, holdM2Reply)
	if xDone {
		t.Fatal("setup: the round-0 gather must be suspended on the held reply")
	}
	for _, i := range []int{4, 5, 6} {
		if rs := nodes[i].roundsFor(); rs.Lock == nil {
			t.Fatalf("setup: maturer %d must be locked at round 0 (S1 premise)", i)
		}
	}

	refill()
	sweepRounds(t, net, holdM2Reply, maturers)
	refill()
	sweepRounds(t, net, holdM2Reply, maturers)
	drainHeld(t, net, fifo)
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	// The contested height must hold the LOCKED X exactly — with an entry in
	// every mempool, a lock-displacing fold would surface right here.
	blk := nodes[4].Chain().Blocks(ch)
	if len(blk) == 0 {
		t.Fatalf("LIVENESS: the contested height %d never committed", ch)
	}
	lockHash := blkX.Hash()
	if blk[0].Hash() != lockHash {
		t.Fatalf("§6.5 I1/LOCK VIOLATION: the contested height committed a different block than the locked X — a mempool entry displaced a carried lock")
	}
	// And the entry still commits — at a later height (operation liveness).
	refill()
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	for i := 0; i < 2 && findEntryHeight(nodes[4], entry.Root) == 0; i++ {
		_, hh := nodes[4].chain.Head()
		d := byID[nodes[4].designatedProposer(hh, 0)]
		d.maybeProposeBondDrain()
		drainHeld(t, net, fifo)
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)
		refill()
	}
	// The entry may legitimately have RIDDEN the contested proposal itself —
	// proposeBlock folds the mempool before signing, so X-with-entry was the
	// locked value (the strongest liveness outcome); or it lands on a later
	// designee block. Either way it must commit, at or after the contest.
	if h := findEntryHeight(nodes[4], entry.Root); h == 0 || h < ch {
		t.Fatalf("§6.5: the pending entry must commit at or after the contested height (got h%d, contested h%d)", h, ch)
	}
	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed")
	}
}
