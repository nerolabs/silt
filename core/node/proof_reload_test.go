package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/diskproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// TestStartProofReloadIsNonBlockingAndChunked pins flixz finding F3: the startup
// proof reload must NOT block the daemon before it binds its relay/registry
// listeners. On a real 14 GB / 381K-file store the synchronous LoadProofs scanned
// the WHOLE store (~8m45s) BEFORE the relay (4002) and registry (4003) listeners
// bound → ~9 min of registry/relay downtime per restart, growing with store size.
//
// StartProofReload instead defers the scan onto the event loop in bounded batches
// (clock.AfterFunc), so daemon startup proceeds — the listeners bind — while the
// resident proofMeta index matures lazily in the background. The full proofs page
// on demand (#464), so serving does not wait for the scan. Every proofMeta write
// stays on the loop, preserving its single-threaded invariant.
//
// The test asserts the three properties that distinguish the async reload from the
// old blocking one: (1) it loads NOTHING synchronously (the OLD LoadProofs is the
// F3 downtime bug — it would leave proofMeta full right here); (2) it completes in
// multiple bounded loop events (so it never monopolizes the loop); (3) after the
// loop drains, every chunk's metadata is resident, same as the synchronous path.
func TestStartProofReloadIsNonBlockingAndChunked(t *testing.T) {
	const N = 500 // > several proofReloadBatch windows, so the reload must chunk

	dir := t.TempDir()
	ds, err := diskproofs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]ports.ChunkID, N)
	for i := 0; i < N; i++ {
		var id ports.ChunkID
		id[0], id[1], id[2], id[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
		ids[i] = id
		var root ports.Hash
		root[0], root[1] = byte(i), byte(i>>8)
		if err := ds.Put(id, ports.StorageProof{Root: root, Index: i % 8, Total: 8, Column: i % 4}); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	var idNode ports.NodeID
	idNode[0] = 3
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())
	nd := New(idNode, DefaultConfig(), sched, net.Endpoint(idNode), memstore.New())
	nd.SetProofStore(ds)

	// (1) Non-blocking: the reload is scheduled onto the loop, not run inline, so
	// nothing is resident until the loop runs. The OLD synchronous LoadProofs would
	// have all N loaded right here — which IS the F3 startup stall.
	nd.StartProofReload()
	if got := len(nd.proofMeta); got != 0 {
		t.Fatalf("StartProofReload loaded %d proofs synchronously — it must defer the scan to the loop so listeners bind first (F3)", got)
	}

	// (2)+(3) Drive the loop to quiescence, bounded so a bug can't hang the test.
	// Count the events it takes: a single monolithic task would finish in one.
	loaded := func() int {
		c := 0
		for _, id := range ids {
			if _, ok := nd.proofMeta[id]; ok {
				c++
			}
		}
		return c
	}
	events := 0
	for loaded() < N {
		if !sched.Step() {
			break
		}
		events++
		if events > N+1000 { // safety valve against a non-terminating reschedule
			break
		}
	}
	if got := loaded(); got != N {
		t.Fatalf("only %d/%d proofMeta entries resident after %d loop events — lazy reload lost data", got, N, events)
	}
	if events < 2 {
		t.Fatalf("reload finished in %d loop event(s) — it must run in bounded batches so it never monopolizes the loop (N=%d)", events, N)
	}
}
