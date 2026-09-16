package node

// AN OPERATOR TAKEDOWN THAT REPORTS SUCCESS MUST HAVE DELETED THE BYTES.
//
// The operator workflow for `-denylist` is: write the list, RESTART the daemon. On that
// path the node's resident chunk index (proofMeta) is rebuilt by StartProofReload, which
// is deliberately ASYNC — a synchronous scan of a large store held the relay and registry
// listeners down for minutes per restart. EnforceDenylist sweeps that same index. Run at
// config time it therefore swept a map the scan had not filled yet: it purged NOTHING,
// printed "denylist: honoring N denied root(s)", and left every denied byte on disk.
//
// WHY THE RACE WAS PERMANENT RATHER THAN SELF-CORRECTING. The async scan is safe for the
// re-announce path because an announce that races it self-corrects on the next reprovide
// sweep. The denylist purge is a ONE-SHOT sweep. It has no next sweep, so a miss is
// forever — and the miss is silent, because purging zero chunks is indistinguishable from
// having nothing to purge.
//
// The second casualty was quieter and worse. chunkDenied reads the same index and returns
// ok && denied, so an ABSENT entry reads as NOT DENIED — and AnnounceHeld skips
// advertising only what chunkDenied reports. With an empty index a restarted node
// re-announced the taken-down chunks to the DHT. Purging from the completion callback
// fixes both: the bytes leave the store before the sweep that would advertise them.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/diskproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/denylist"
	"github.com/nerolabs/silt/ports"
)

// restartedHolder rebuilds the state a daemon comes back up with: chunks already on disk
// from a previous run, their proofs in the backing store, and a COLD resident index.
func restartedHolder(t *testing.T, deniedRoot ports.Hash, denied, clean int) (*Node, *simclock.Scheduler, []ports.ChunkID) {
	t.Helper()
	ds, err := diskproofs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := memstore.New()
	var deniedIDs []ports.ChunkID

	put := func(i int, root ports.Hash, denied bool) {
		data := []byte{byte(i), byte(i >> 8), 0xAB}
		c := ports.Chunk{ID: ports.HashBytes(data), Data: data}
		if err := store.Put(bg(), c); err != nil {
			t.Fatal(err)
		}
		if err := ds.Put(c.ID, ports.StorageProof{Root: root, Index: i % 8, Total: 8, Column: i % 4}); err != nil {
			t.Fatal(err)
		}
		if denied {
			deniedIDs = append(deniedIDs, c.ID)
		}
	}
	for i := 0; i < denied; i++ {
		put(i, deniedRoot, true)
	}
	for i := 0; i < clean; i++ {
		var other ports.Hash
		other[0], other[1] = 0xEE, byte(i)
		put(1000+i, other, false)
	}

	var id ports.NodeID
	id[0] = 9
	sched := simclock.New()
	net := simnet.New(sched, 11, simnet.DefaultConfig())
	nd := New(id, DefaultConfig(), sched, net.Endpoint(id), store)
	nd.SetProofStore(ds)
	return nd, sched, deniedIDs
}

// TestDenylistPurgesOnRestartOnceTheIndexIsResident is the rule. The daemon applies the
// list while the index is still cold; the bytes must still be gone.
func TestDenylistPurgesOnRestartOnceTheIndexIsResident(t *testing.T) {
	var deniedRoot ports.Hash
	deniedRoot[0] = 0x77
	const deniedChunks, cleanChunks = 6, 5
	nd, sched, deniedIDs := restartedHolder(t, deniedRoot, deniedChunks, cleanChunks)

	dl := denylist.New()
	dl.Add(deniedRoot)
	nd.SetDenylist(dl)

	// The daemon's shape: the purge is hung off reload completion, and the list is set
	// during the same synchronous startup, before the loop turns.
	purged := 0
	nd.StartProofReload(func() { purged = nd.EnforceDenylist() })

	// A sweep at CONFIG TIME — the defect — sees a cold index and finds nothing.
	if got := nd.EnforceDenylist(); got != 0 {
		t.Fatalf("setup: the index should still be cold before the loop runs, but a sweep "+
			"already found %d chunks; this test would not be exercising the restart race", got)
	}

	for i := 0; i < 4000 && purged == 0; i++ {
		if !sched.Step() {
			break
		}
	}

	if purged != deniedChunks {
		t.Fatalf("THE TAKEDOWN PURGED %d OF %d DENIED CHUNKS.\n"+
			"  An operator who writes a denylist and restarts is told the list is being honored.\n"+
			"  If the purge swept the resident index before the async scan filled it, the denied\n"+
			"  bytes are still on disk and the operator has no way to see it: purging zero is\n"+
			"  indistinguishable from having nothing to purge.", purged, deniedChunks)
	}
	for _, id := range deniedIDs {
		if _, err := nd.store.Get(bg(), id); err == nil {
			t.Fatalf("denied chunk %s is STILL IN THE STORE after a reported purge", id)
		}
		if _, ok := nd.proofMeta[id]; ok {
			t.Fatalf("denied chunk %s still has resident metadata, so the node will keep "+
				"answering for it", id)
		}
	}
}

// TestDenylistPurgeSparesEverythingElse is the vacuity guard. A purge that deleted the
// whole store would satisfy the rule above and destroy the node.
func TestDenylistPurgeSparesEverythingElse(t *testing.T) {
	var deniedRoot ports.Hash
	deniedRoot[0] = 0x77
	const deniedChunks, cleanChunks = 3, 7
	nd, sched, _ := restartedHolder(t, deniedRoot, deniedChunks, cleanChunks)

	dl := denylist.New()
	dl.Add(deniedRoot)
	nd.SetDenylist(dl)
	done := false
	nd.StartProofReload(func() { nd.EnforceDenylist(); done = true })
	for i := 0; i < 4000 && !done; i++ {
		if !sched.Step() {
			break
		}
	}

	ids, err := nd.store.List(bg())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != cleanChunks {
		t.Fatalf("store holds %d chunks after the takedown, want the %d undenied ones — a "+
			"takedown that removes content it was not given is an outage, not a defence",
			len(ids), cleanChunks)
	}
}

// TestUndeniedRestartPurgesNothing is the positive control on the other axis: the
// completion callback must not become a general-purpose chunk shredder.
func TestUndeniedRestartPurgesNothing(t *testing.T) {
	var deniedRoot, unrelated ports.Hash
	deniedRoot[0] = 0x77
	unrelated[0] = 0x12 // a list naming a root this node never held
	nd, sched, _ := restartedHolder(t, deniedRoot, 4, 4)

	dl := denylist.New()
	dl.Add(unrelated)
	nd.SetDenylist(dl)
	purged, done := 0, false
	nd.StartProofReload(func() { purged = nd.EnforceDenylist(); done = true })
	for i := 0; i < 4000 && !done; i++ {
		if !sched.Step() {
			break
		}
	}
	if purged != 0 {
		t.Fatalf("purged %d chunks for a root this node never held", purged)
	}
	ids, _ := nd.store.List(bg())
	if len(ids) != 8 {
		t.Fatalf("store holds %d chunks, want all 8 untouched", len(ids))
	}
}
