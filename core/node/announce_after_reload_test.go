package node

// A RESTARTED HOLDER MUST ADVERTISE ITS CODED CONTENT WHERE READERS LOOK.
//
// A coded shard is announced under its COLUMN key, not its own chunk id: a whole column
// shares one key, and that key is what a reader asks the DHT for. Deriving it needs the
// shard's Root and Column, which live in the resident proof index — so AnnounceHeld reads
// that index, and the daemon persists every hosted proof precisely so a restart can
// rebuild it. The startup comment states the dependency and the consequence of losing it:
// "a restart re-announces coded shards under the right column key (AnnounceHeld, below,
// reads the reloaded proofs) — otherwise a disk full of content is invisible until
// re-hosted."
//
// THE LAZY RELOAD BROKE THAT CONTRACT WITHOUT MOVING THE COMMENT. Rebuilding the index
// was made asynchronous because scanning a 14 GB store synchronously held the relay and
// registry listeners down for ~9 minutes per restart. AnnounceHeld still runs in
// synchronous startup, which now puts it BEFORE the scan it is documented to read. With a
// cold index placementKey falls back to the bare chunk id, so every coded shard is
// advertised under a key no reader queries. The node looks healthy and serves nothing.
//
// IT SELF-HEALS, EVENTUALLY, WHICH IS WHY IT HID. StartReprovide re-runs AnnounceHeld on
// a timer, so the records correct themselves within about half a provider-record TTL. A
// fetch inside that window finds nothing, and whether a given test sees it is luck: the
// takedown suite's control-file fetch passed standalone and failed in the sweep on the
// same build, same fixture.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/diskproofs"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// codedHolderAfterRestart stages a node that already holds coded shards on disk with
// their proofs, and a COLD resident index — the state a daemon boots into.
func codedHolderAfterRestart(t *testing.T, shards int) (*Node, *simclock.Scheduler, ports.Hash, []ports.ChunkID) {
	t.Helper()
	ds, err := diskproofs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := memstore.New()
	var root ports.Hash
	root[0], root[1] = 0xC0, 0xDE
	ids := make([]ports.ChunkID, 0, shards)
	for i := 0; i < shards; i++ {
		data := []byte{byte(i), 0x5A, byte(i >> 8)}
		c := ports.Chunk{ID: ports.HashBytes(data), Data: data}
		if err := store.Put(bg(), c); err != nil {
			t.Fatal(err)
		}
		// Column 2 of 8 for every shard: one shared column key, which is the whole
		// point — readers ask for the column, not the shard.
		if err := ds.Put(c.ID, ports.StorageProof{Root: root, Index: i, Total: 8, Column: 2}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	var id ports.NodeID
	id[0] = 0x2B
	sched := simclock.New()
	net := simnet.New(sched, 23, simnet.DefaultConfig())
	nd := New(id, DefaultConfig(), sched, net.Endpoint(id), store)
	nd.SetProofStore(ds)
	return nd, sched, root, ids
}

// TestAnnounceAfterReloadUsesPlacementKeys is the rule: once the index is resident, the
// node's records must sit under the column key a reader queries.
func TestAnnounceAfterReloadUsesPlacementKeys(t *testing.T) {
	const shards = 5
	nd, sched, root, ids := codedHolderAfterRestart(t, shards)
	want := colKey(root, 2)

	// The daemon's shape: the startup announce runs against a COLD index, then the
	// reload completes and announces again.
	nd.AnnounceHeld(func(int) {})
	for i := 0; i < 200; i++ {
		if !sched.Step() {
			break
		}
	}
	coldHasColumn := len(nd.provs.IDs(want)) > 0

	announced := false
	nd.StartProofReload(func() { nd.AnnounceHeld(func(int) { announced = true }) })
	for i := 0; i < 4000 && !announced; i++ {
		if !sched.Step() {
			break
		}
	}
	if !announced {
		t.Fatal("the post-reload announce never ran")
	}

	if got := nd.provs.IDs(want); len(got) == 0 {
		t.Fatalf("NO PROVIDER RECORD UNDER THE COLUMN KEY after the index went resident.\n"+
			"  A reader fetching this content asks for the column key %s and finds nobody, so a\n"+
			"  node with the bytes on disk serves nothing — 'invisible until re-hosted'.", want)
	}
	// And the cold announce is what the fix exists to repair: it could not have known the
	// column, so it published under bare ids instead.
	if coldHasColumn {
		t.Skip("the cold announce already resolved the column key; this fixture no longer " +
			"reproduces the startup race it was written for")
	}
	for _, id := range ids {
		if len(nd.provs.IDs(ports.Hash(id))) > 0 && len(nd.provs.IDs(want)) == 0 {
			t.Fatalf("shard %s is advertised under its BARE id and not its column key", id)
		}
	}
}

// TestColdAnnounceMissesTheColumnKey is the ablation, held as a gate: it pins WHY the
// second announce is needed, so deleting it fails loudly rather than silently restoring a
// window that only shows up as a flaky integration fetch.
func TestColdAnnounceMissesTheColumnKey(t *testing.T) {
	nd, sched, root, _ := codedHolderAfterRestart(t, 4)
	want := colKey(root, 2)

	nd.AnnounceHeld(func(int) {}) // startup announce only: the index is cold
	for i := 0; i < 200; i++ {
		if !sched.Step() {
			break
		}
	}
	if len(nd.provs.IDs(want)) > 0 {
		t.Fatal("a COLD announce resolved the column key: this fixture is not staging the " +
			"defect, so the paired rule above proves nothing")
	}
}
