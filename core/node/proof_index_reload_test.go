package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/diskproofs"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// countingProofs counts per-proof reads on the way to a disk store with a metadata index.
type countingProofs struct {
	*diskproofs.Store
	gets int
}

func (c *countingProofs) Get(id ports.ChunkID) (ports.StorageProof, bool, error) {
	c.gets++
	return c.Store.Get(id)
}

// TestARestartReadsTheProofIndexNotEveryProof: the first reload of a store with no index reads
// proof by proof and then writes the index; a restart on the same store loads every proof's
// metadata from the index with no per-proof reads — the 75-minute restart on the test network
// was 140k of them.
func TestARestartReadsTheProofIndexNotEveryProof(t *testing.T) {
	dir := t.TempDir()
	disk, err := diskproofs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const n = 300
	for i := 0; i < n; i++ {
		id := ports.ChunkID(ports.HashBytes([]byte{byte(i), byte(i >> 8), 3}))
		disk.Put(id, ports.StorageProof{Root: ports.HashBytes([]byte{byte(i)}), Index: i, Total: n, Column: i % 16})
	}
	boot := func() (*Node, *countingProofs) {
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.DefaultConfig())
		id := identity.FromSeed(4300).NodeID()
		nd := New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())
		d, err := diskproofs.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		cp := &countingProofs{Store: d}
		nd.SetProofStore(cp)
		reloaded := false
		nd.StartProofReload(func() { reloaded = true })
		sched.Run()
		if !reloaded || len(nd.proofMeta) != n {
			t.Fatalf("reload: done=%v, %d proofs resident, want %d", reloaded, len(nd.proofMeta), n)
		}
		return nd, cp
	}
	_, first := boot()
	if first.gets < n {
		t.Fatalf("CONTROL: the first reload read %d proofs; with no index it must read all %d", first.gets, n)
	}
	nd, second := boot()
	if second.gets != 0 {
		t.Fatalf("the restart read %d proofs one by one; the index should serve all %d", second.gets, n)
	}
	id := ports.ChunkID(ports.HashBytes([]byte{byte(42), 0, 3}))
	if m := nd.proofMeta[id]; m.Index != 42 || m.Column != 42%16 {
		t.Fatalf("proof 42's metadata after the indexed restart: %+v", m)
	}
}
