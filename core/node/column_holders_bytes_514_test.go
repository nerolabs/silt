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

// TestConfirmColumnHoldersCorpseGatesDeadRecord514 is the liveness RED for the
// #514 fix (PE ruling RULING-PR607-514-holders-byte-confirm-2026-08-27.md, Q2):
// PR #607's confirmColumnHolders dialed a stale/dead provider record ONCE PER
// SHARD of the column, at a full HolderDialTimeout each — the #226/#277/#501
// dead-holder dial-storm re-entering through the holders read. On a multi-stripe
// object (one shard per stripe) that is stripes × HolderDialTimeout serial per
// dead record. The fix corpse-gates confirmColumnHolders like probeShard: a
// provider proven dead this walk is dialed AT MOST ONCE, then skipped.
//
// The setup: a column with 5 shards (a 5-stripe object). DEAD holds a provider
// record but is a black hole (never replies); LIVE holds a shard and answers.
// With corpse-gating DEAD is dialed once and the remaining 4 shards are skipped
// (HolderDialsSkipped == 4). ABLATION: drop the corpse-gate check from
// confirmColumnHolders and DEAD is dialed all 5 times (HolderDialsSkipped == 0),
// the dial-storm the ruling flagged.
func TestConfirmColumnHoldersCorpseGatesDeadRecord514(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DHTDomainCap = 0
	sched := simclock.New()
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}

	var meID, liveID, deadID ports.NodeID
	meID[0], liveID[0], deadID[0] = 1, 2, 3
	meEnd := &linkEnd{net: ln, id: meID}
	liveEnd := &linkEnd{net: ln, id: liveID}
	// DEAD's end is registered (a route exists) but carries a COUNTING black-hole
	// handler that never replies — so every dial to it TIMES OUT and marks a
	// corpse, and we can count exactly how many dials it received.
	deadDials := 0
	deadEnd := &linkEnd{net: ln, id: deadID, h: func(ports.NodeID, ports.Message) { deadDials++ }}
	ln.ends[meID], ln.ends[liveID], ln.ends[deadID] = meEnd, liveEnd, deadEnd

	me := New(meID, cfg, sched, meEnd, memstore.New())

	// Five shards of one column (a 5-stripe object). LIVE holds the LAST one, so a
	// per-shard walk of DEAD reaches every shard before LIVE is even tried — the
	// worst case the dial-storm lives in.
	var shards []ports.ChunkID
	liveStore := memstore.New()
	for i := 0; i < 5; i++ {
		c := ports.NewChunk([]byte{byte('s'), byte(i)})
		shards = append(shards, c.ID)
		if i == 4 {
			liveStore.Put(bg(), c)
		}
	}
	New(liveID, cfg, sched, liveEnd, liveStore)

	// me's provider records for the column (unsigned self-vouch, accepted under
	// the default non-strict config). DEAD first so it is walked first.
	colKey := ports.Hash{0xC0}
	me.provs.Add(ports.ProviderRecord{Key: colKey, ID: deadID})
	me.provs.Add(ports.ProviderRecord{Key: colKey, ID: liveID})

	provs := []ports.NodeID{deadID, liveID}

	var confirmed []ports.NodeID
	done := false
	me.confirmColumnHolders(provs, shards, func(ids []ports.NodeID) { confirmed, done = ids, true })
	sched.Run()

	if !done {
		t.Fatal("confirmColumnHolders never completed")
	}
	// LIVE is byte-confirmed and kept; DEAD (no bytes, unreachable) is dropped.
	if !contains(confirmed, liveID) {
		t.Fatalf("byte-confirmed holders dropped the LIVE holder: got %v", confirmed)
	}
	if contains(confirmed, deadID) {
		t.Fatalf("byte-confirmed holders kept the DEAD record-holder: got %v", confirmed)
	}
	// The liveness assertion: DEAD is dialed EXACTLY ONCE, then its remaining
	// shards are abandoned by the corpse-gate (one whole-provider skip) instead of
	// paying a full HolderDialTimeout per shard. Ablating the in-shard gate makes
	// DEAD dialed 5 times (once per shard = the dial-storm) with zero skips.
	if deadDials != 1 {
		t.Fatalf("#514 liveness: DEAD record-holder was dialed %d times (want exactly 1); "+
			">1 is the once-per-shard dial-storm the ruling flagged", deadDials)
	}
	if me.Stats.HolderDialsSkipped < 1 {
		t.Fatalf("#514 liveness: the corpse-gate never skipped DEAD's remaining shards (HolderDialsSkipped=%d) — "+
			"the gate did not engage, so DEAD's shards would each pay a full HolderDialTimeout", me.Stats.HolderDialsSkipped)
	}
}

func contains(ids []ports.NodeID, want ports.NodeID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
