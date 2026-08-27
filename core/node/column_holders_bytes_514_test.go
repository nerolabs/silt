package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// TestColumnHoldersByteConfirmsDropsPhantomRecord514 is the deterministic
// reproducer for the #514 e2e flake, at the tier where the selector logic lives.
//
// The flake: TestRepairBountyPaysOnTheWire kills a column's RECORD holders
// (swarm holders → ColumnHolders) but a live byte copy survives on a node the
// record-view omitted, so the caretaker's byte-confirmed sweep (probeShard) sees
// missing ≤ slack and never arms repair — premise defeated. Root cause: the
// holders view reported raw provider records while the repair judgment
// byte-confirms with MsgHasChunk, so the two views diverged.
//
// This models the divergence directly: node PHANTOM holds a column provider
// record but NOT the bytes (a #497 lost-ack / #517 stale record); node REAL
// holds the bytes. A record-view holders read lists PHANTOM; the byte-confirmed
// read must list only REAL. Killing PHANTOM would leave the bytes on REAL — the
// exact under-kill that defeats the premise.
//
// ABLATION: swap confirmColumnHolders for the raw record set (return provs
// unfiltered) and this test goes RED — it lists the phantom. That is the check
// injected-and-watched-red rule: a green here is the byte-confirm working, not
// decoration.
func TestColumnHoldersByteConfirmsDropsPhantomRecord514(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DHTDomainCap = 0
	sched := simclock.New()
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}

	var meID, realID, phantomID ports.NodeID
	meID[0], realID[0], phantomID[0] = 1, 2, 3
	meEnd := &linkEnd{net: ln, id: meID}
	realEnd := &linkEnd{net: ln, id: realID}
	phantomEnd := &linkEnd{net: ln, id: phantomID}
	ln.ends[meID], ln.ends[realID], ln.ends[phantomID] = meEnd, realEnd, phantomEnd

	me := New(meID, cfg, sched, meEnd, memstore.New())

	// One column shard. REAL holds it; PHANTOM does not — but both carry a
	// provider record for the column (the lost-ack / stale-record shape).
	shard := ports.NewChunk([]byte("column-0 shard bytes"))
	realStore := memstore.New()
	realStore.Put(bg(), shard)
	real := New(realID, cfg, sched, realEnd, realStore)
	phantom := New(phantomID, cfg, sched, phantomEnd, memstore.New()) // empty store

	// me knows the provider records for both, in table order.
	colKey := ports.Hash{0xC0} // stand-in column key; the record set is what matters
	me.provs.Add(real.providerRecord(colKey))
	me.provs.Add(phantom.providerRecord(colKey))
	me.table.Observe(realID)
	me.table.Observe(phantomID)

	// Precondition: the RAW record view lists BOTH — the divergence is real.
	var raw []ports.NodeID
	me.resolveProviders(colKey, func(ids []ports.NodeID) { raw = ids })
	sched.Run()
	if !contains(raw, phantomID) || !contains(raw, realID) {
		t.Fatalf("precondition: raw record view must list both holders, got %v", raw)
	}

	// The byte-confirmed view must drop PHANTOM (no bytes) and keep REAL.
	var confirmed []ports.NodeID
	done := false
	me.confirmColumnHolders(raw, []ports.ChunkID{shard.ID}, func(ids []ports.NodeID) {
		confirmed, done = ids, true
	})
	sched.Run()

	if !done {
		t.Fatal("confirmColumnHolders never completed")
	}
	if contains(confirmed, phantomID) {
		t.Fatalf("#514: byte-confirmed holders still lists the phantom record-holder %v — "+
			"the selector would under-kill (kill the record, leave the bytes)", confirmed)
	}
	if !contains(confirmed, realID) {
		t.Fatalf("#514: byte-confirmed holders dropped the REAL byte-holder: got %v", confirmed)
	}
	_ = real
	_ = phantom
}

func contains(ids []ports.NodeID, want ports.NodeID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
