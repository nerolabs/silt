package tcpnet

import (
	"testing"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// A public node must not learn loopback/unspecified addresses from gossip
// (the phantom-peer poison that defeats provider resolution), while a
// NATed/on-host node keeps them so a loopback test swarm still discovers
// itself. Explicit -bootstrap (AddPeer) is always exempt.
func TestGossipLoopbackFilter(t *testing.T) {
	var id ports.NodeID
	id[0] = 0x99

	loop := eventloop.New()
	pub, err := New(loop, identity.FromSeed(1), "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	pub.SetAdvertise("46.225.149.61:4001") // public

	pub.learnGossip(id, "127.0.0.1:53042", false)
	if pub.PeerCount() != 0 {
		t.Fatalf("public node learned loopback gossip: got %d, want 0", pub.PeerCount())
	}
	pub.learnGossip(id, "203.0.113.7:4001", false)
	if pub.PeerCount() != 1 {
		t.Fatalf("public node dropped a routable gossip peer: got %d, want 1", pub.PeerCount())
	}

	var id2 ports.NodeID
	id2[0] = 0x42
	loop2 := eventloop.New()
	loc, err := New(loop2, identity.FromSeed(2), "127.0.0.1:0") // NATed/local
	if err != nil {
		t.Fatal(err)
	}
	loc.learnGossip(id2, "127.0.0.1:53042", false)
	if loc.PeerCount() != 1 {
		t.Fatalf("local node dropped loopback gossip (test swarm regression): got %d, want 1", loc.PeerCount())
	}

	// AddPeer (explicit bootstrap) is exempt even on a public node.
	var id3 ports.NodeID
	id3[0] = 0x07
	pub.AddPeer(id3, "127.0.0.1:9999")
	if pub.PeerCount() != 2 {
		t.Fatalf("AddPeer bootstrap was filtered: got %d, want 2", pub.PeerCount())
	}
}
