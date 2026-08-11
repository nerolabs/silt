package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// #286 Layer 2 Q4 (docs/network-durability.md §2/§8): a CONFIGURED static consensus peer
// (-persistent-peers → AddStaticPeer) must NEVER be evicted from the routing table on a
// reachability timeout — its address is configured, not discovered, so a transient WAN
// miss is a retry situation, not an eviction. A normal DISCOVERED peer is still evicted
// once retries exhaust. Evicting a validator mid-genesis is what let a proposer lose an
// attester's address and stall quorum on the cross-region cold start.
func TestStaticPeerSurvivesReachabilityEviction286(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	cfg := DefaultConfig()
	cfg.RequestRetries = 1 // exercise the retry path, then exhaust

	nID := identity.FromSeed(4000).NodeID()
	n := New(nID, cfg, sched, net.Endpoint(nID), memstore.New())

	staticID := identity.FromSeed(4001).NodeID()
	normalID := identity.FromSeed(4002).NodeID()

	// Both known to the table, both DOWN — their endpoints exist but are killed, so every
	// request to them times out (the reachability-eviction path).
	_ = net.Endpoint(staticID)
	_ = net.Endpoint(normalID)
	net.Kill(staticID)
	net.Kill(normalID)
	n.table.Observe(staticID)
	n.table.Observe(normalID)
	n.AddStaticPeer(staticID) // the only difference between the two
	if n.Table().Size() != 2 {
		t.Fatalf("precondition: table=%d, want 2", n.Table().Size())
	}

	inTable := func(id ports.NodeID) bool {
		for _, p := range n.table.Closest(id, 20) {
			if p == id {
				return true
			}
		}
		return false
	}

	// A DHT-class RPC to each (not bond-challenge, not holder-fetch) — the eviction path.
	// Nobody answers, so both exhaust retries and reach the eviction branch.
	n.request(staticID, ports.Message{Kind: ports.MsgFindNode, Target: staticID}, func(ports.Message, error) {})
	n.request(normalID, ports.Message{Kind: ports.MsgFindNode, Target: normalID}, func(ports.Message, error) {})

	// Advance well past all retries + backoff so the eviction branch has fired for both.
	sched.RunUntil(sched.Now().Add(cfg.RequestTimeout*8 + cfg.RequestBackoff*8 + ports.Second))

	if !inTable(staticID) {
		t.Fatal("#286 L2 Q4: a configured static peer was EVICTED on a reachability timeout — it must be retained")
	}
	if inTable(normalID) {
		t.Fatal("a normal discovered peer should be evicted from the table after retries exhaust (control)")
	}
}
