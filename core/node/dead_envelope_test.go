package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

func nodeHasProvider(n *Node, key ports.Hash, id ports.NodeID) bool {
	for _, r := range n.provs.Get(key) {
		if r.ID == id {
			return true
		}
	}
	return false
}

// TestConfirmedDeadHolderPrunedFromReplicatedKeptForSole is the P0-1 headline
// regression (#277 dead-peer envelope): when a holder is CONFIRMED dead (a real
// dial to it exhausts its retries and times out), it must leave the provider-record
// candidate set for keys that have a live alternative — so the fetch/repair loop
// stops re-dialing the corpse once per deadUntil cooldown forever — WHILE its record
// for a key it SOLELY provides is kept, so that content stays discoverable (#69).
//
// Fails before the node.go RemoveIfNotSole wiring (the corpse lingered in every
// key); passes after.
func TestConfirmedDeadHolderPrunedFromReplicatedKeptForSole(t *testing.T) {
	cfg := DefaultConfig()
	searcher, deadID, _, sched := walkDeadRig(t, cfg) // deadID is routable but silent → a dial can only time out
	var liveID ports.NodeID
	liveID[0] = 2 // the live peer walkDeadRig wires alongside the corpse

	replicated := ports.Hash{0x01} // corpse + a live sibling both provide this
	soleKey := ports.Hash{0x02}    // corpse is the ONLY provider of this
	searcher.provs.Add(ports.ProviderRecord{Key: replicated, ID: deadID})
	searcher.provs.Add(ports.ProviderRecord{Key: replicated, ID: liveID})
	searcher.provs.Add(ports.ProviderRecord{Key: soleKey, ID: deadID})

	// Dial the corpse directly so it times out → confirmed dead (evicted +
	// negative-cached) → the prune fires. A HasChunk is a non-consensus holder dial,
	// exactly the class the pruning targets.
	var done bool
	searcher.request(deadID, ports.Message{Kind: ports.MsgHasChunk, ChunkID: ports.ChunkID(replicated)},
		func(ports.Message, error) { done = true })
	sched.Run()

	if !done {
		t.Fatal("the dial to the corpse never completed (should have timed out)")
	}
	if nodeHasProvider(searcher, replicated, deadID) {
		t.Fatal("#277: a confirmed-dead holder must be pruned from a REPLICATED key (a live sibling exists)")
	}
	if !nodeHasProvider(searcher, replicated, liveID) {
		t.Fatal("the live sibling must remain a provider of the replicated key")
	}
	if !nodeHasProvider(searcher, soleKey, deadID) {
		t.Fatal("#69: a SOLE dead holder must be KEPT — orphaning it makes its content undiscoverable")
	}
	if searcher.Stats.DeadProviderRecordsPruned != 1 {
		t.Fatalf("expected exactly 1 pruned record (the replicated corpse), got %d", searcher.Stats.DeadProviderRecordsPruned)
	}
}
