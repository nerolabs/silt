package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// TestSurvivorNakamoto_CountsDistinctFailureDomains pins the raw non-globality
// metric (R-2 / immutable #5 / #180): the survivor Nakamoto-coefficient over a key's
// live provider set is the number of DISTINCT failure domains those providers sit
// in — how many independent domains a censor must eclipse to make the content
// undiscoverable. A set spread across N domains reads N; the same providers
// collapsed into ONE domain read 1 (one key-surround from dark), which is exactly
// the collapse the weight of the provider count hides.
func TestSurvivorNakamoto_CountsDistinctFailureDomains(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	self := identity.FromSeed(801)
	n := New(self.NodeID(), DefaultConfig(), sched, net.Endpoint(self.NodeID()), memstore.New())

	key := ports.ChunkID(ports.HashBytes([]byte("a-root-key")))
	prov := []ports.NodeID{identity.FromSeed(810).NodeID(), identity.FromSeed(811).NodeID(), identity.FromSeed(812).NodeID()}

	// Three providers in three distinct declared domains → survivor-nakamoto 3.
	for i, pid := range prov {
		n.provs.Add(ports.ProviderRecord{Key: key, ID: pid})
		n.peerDomains[pid] = uint64(100 + i) // 100, 101, 102
	}
	if sn := n.SurvivorNakamoto(key); sn != 3 {
		t.Fatalf("three providers across three domains → survivor-nakamoto 3, got %d", sn)
	}

	// Collapse all three into ONE domain → survivor-nakamoto 1, even though the
	// provider COUNT is unchanged. This is the censor's fingerprint the raw count
	// hides and the metric exposes.
	for _, pid := range prov {
		n.peerDomains[pid] = 500
	}
	if sn := n.SurvivorNakamoto(key); sn != 1 {
		t.Fatalf("three providers collapsed into one domain → survivor-nakamoto 1, got %d", sn)
	}

	// A provider whose domain is unknown counts as its own independent position
	// (conservative), so a domainless set is never read as collapsed.
	delete(n.peerDomains, prov[0])
	delete(n.peerDomains, prov[1])
	delete(n.peerDomains, prov[2])
	if sn := n.SurvivorNakamoto(key); sn != 3 {
		t.Fatalf("three providers with unknown domains → three independent positions, got %d", sn)
	}
}
