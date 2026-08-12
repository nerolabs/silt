package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// walkDeadRig wires a searcher, a LIVE peer that answers provider lookups, and a
// DEAD peer that is routable (so a dial to it is not a fast synchronous refusal)
// but never replies — a dial to it can only end in a RequestTimeout. Both peers
// are seeded into the searcher's routing table, so a provider walk considers
// both. deadGetProviders counts walk dials (MsgGetProviders) aimed at the corpse.
func walkDeadRig(t *testing.T, cfg Config) (searcher *Node, deadID ports.NodeID, deadGetProviders *int, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
	var meID, liveID ports.NodeID
	meID[0], liveID[0], deadID[0] = 1, 2, 3

	dials := 0
	meEnd := &linkEnd{net: ln, id: meID}
	liveEnd := &linkEnd{net: ln, id: liveID}
	deadEnd := &linkEnd{net: ln, id: deadID}
	deadEnd.h = func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgGetProviders {
			dials++ // a walk dial actually left for the corpse
		}
		// silent: never replies → the searcher can only time out on it
	}
	ln.ends[meID], ln.ends[liveID], ln.ends[deadID] = meEnd, liveEnd, deadEnd

	searcher = New(meID, cfg, sched, meEnd, memstore.New())
	New(liveID, cfg, sched, liveEnd, memstore.New()) // answers via the real handler

	// Both peers are known to the routing table, so a provider walk for any key
	// will consider dialing each of them.
	searcher.table.Observe(liveID)
	searcher.table.Observe(deadID)
	return searcher, deadID, &dials, sched
}

// TestProviderWalkSkipsCooledPeer is the churn dial-storm regression on the DHT
// side: a peer that timed out once is negative-cached, so a later provider walk
// (the discovery step a repair sweep runs for every key) FAILS it in the lookup
// without a dial instead of eating another full RequestTimeout on the same
// corpse. Before this, a caretaker sweeping a churny swarm re-dialed dead
// routing entries every walk and never finished a sweep — so it never saw the
// loss and never repaired.
func TestProviderWalkSkipsCooledPeer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DHTDomainCap = 0 // isolate the base walk (no diversity sweep second leg)
	searcher, deadID, deadDials, sched := walkDeadRig(t, cfg)

	// The corpse is already negative-cached — a prior sweep timed out on it.
	searcher.deadUntil[deadID] = searcher.clock.Now().Add(searcher.cfg.HolderCooldown)

	var done bool
	searcher.resolveProviders(ports.Hash{0xAB}, func([]ports.NodeID) { done = true })
	sched.Run()

	if !done {
		t.Fatal("resolveProviders never completed")
	}
	if *deadDials != 0 {
		t.Fatalf("a cooled-down peer must be skipped by the provider walk: got %d dials, want 0", *deadDials)
	}
}

// TestProviderDiversitySweepSkipsCooledPeer is the #277 regression the 2026-08-12
// blind field test surfaced: TestProviderWalkSkipsCooledPeer above sets
// DHTDomainCap = 0, which isolates AWAY the diversity-sweep second leg of
// resolveProviders — the exact leg the daemon and client always run
// (DHTDomainCap = 2, cmd/silt/daemon.go + client.go). So the green walk test gave
// false confidence: it proved the base walk is gated and never exercised the
// sweep. Here we turn the sweep ON and prove the negative cache is honored there
// too — a cooled corpse must not eat a full RequestTimeout dial on every resolve
// (the churn dial-storm, #277).
func TestProviderDiversitySweepSkipsCooledPeer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DHTDomainCap = 2 // ENGAGE the diversity sweep — exactly what daemon.go/client.go do
	searcher, deadID, deadDials, sched := walkDeadRig(t, cfg)

	// The corpse is already negative-cached: a prior resolve timed out on it.
	searcher.deadUntil[deadID] = searcher.clock.Now().Add(searcher.cfg.HolderCooldown)

	var done bool
	searcher.resolveProviders(ports.Hash{0xAB}, func([]ports.NodeID) { done = true })
	sched.Run()

	if !done {
		t.Fatal("resolveProviders never completed")
	}
	if *deadDials != 0 {
		t.Fatalf("#277: a cooled peer must be skipped by the diversity sweep too, "+
			"but sweepProviders re-dialed the corpse: got %d dials, want 0", *deadDials)
	}
}

// TestProviderWalkWithoutCooldownDialsDeadPeer is the inverted control: with the
// negative cache disabled the SAME dead peer IS dialed by the walk — proving the
// cache, not some other effect, is what suppresses the re-dial.
func TestProviderWalkWithoutCooldownDialsDeadPeer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DHTDomainCap = 0
	cfg.HolderCooldown = 0 // negative cache OFF
	searcher, deadID, deadDials, sched := walkDeadRig(t, cfg)
	_ = deadID

	var done bool
	searcher.resolveProviders(ports.Hash{0xAB}, func([]ports.NodeID) { done = true })
	sched.Run()

	if !done {
		t.Fatal("resolveProviders never completed")
	}
	if *deadDials != 1 {
		t.Fatalf("with the negative cache off the corpse is dialed by the walk: got %d dials, want 1", *deadDials)
	}
}

// TestProbeShardSkipsCooledHolder covers the repair-probe half of the same fix:
// the dispersion audit's HasChunk probe skips a negative-cached holder when a
// live holder for the shard still exists (the anyLive guard, mirroring the fetch
// path #226/#69), so a repair sweep isn't stalled re-dialing dead provider
// records shard-by-shard.
func TestProbeShardSkipsCooledHolder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DHTDomainCap = 0
	sched := simclock.New()
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
	var meID, liveID, deadID ports.NodeID
	meID[0], liveID[0], deadID[0] = 1, 2, 3

	meEnd := &linkEnd{net: ln, id: meID}
	liveEnd := &linkEnd{net: ln, id: liveID}
	deadEnd := &linkEnd{net: ln, id: deadID}
	ln.ends[meID], ln.ends[liveID], ln.ends[deadID] = meEnd, liveEnd, deadEnd

	me := New(meID, cfg, sched, meEnd, memstore.New())

	chunk := ports.NewChunk([]byte("probe target shard"))
	key := chunk.ID
	liveStore := memstore.New()
	liveStore.Put(bg(), chunk)
	live := New(liveID, cfg, sched, liveEnd, liveStore)
	dead := New(deadID, cfg, sched, deadEnd, memstore.New())

	// Count MsgHasChunk probes to the corpse, then make it silent (timeout only).
	deadHasChunk := 0
	deadEnd.h = func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgHasChunk {
			deadHasChunk++
		}
	}

	// me holds a provider record for BOTH holders; only live is in the table so
	// the discovery walk never dials the corpse — this isolates the HasChunk skip.
	me.provs.Add(live.providerRecord(key))
	me.provs.Add(dead.providerRecord(key))
	me.table.Observe(liveID)
	me.deadUntil[deadID] = me.clock.Now().Add(me.cfg.HolderCooldown)

	var ok, done bool
	me.probeShard(key, key, false, func(found bool, _ map[uint64]bool) { ok, done = found, true })
	sched.Run()

	if !done {
		t.Fatal("probeShard never completed")
	}
	if !ok {
		t.Fatal("probe should find the shard on the LIVE holder")
	}
	if deadHasChunk != 0 {
		t.Fatalf("a cooled holder must be skipped by the probe when a live one exists: got %d HasChunk dials, want 0", deadHasChunk)
	}
}
