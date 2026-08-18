package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// TestPeerInfoMapsBoundedUnderSybilFlood is the boundedness-audit regression for
// the peer-keyed gossip caches: peerCaps/peerBonds/peerDomains are written keyed
// by the sender of any non-ephemeral message, so a flood of distinct (sybil)
// NodeIDs would grow them without bound (a resident-memory DoS). They must stay
// ≤ maxPeerInfo no matter how many distinct senders gossip — RAM = f(cap), not
// f(distinct peers ever seen).
func TestPeerInfoMapsBoundedUnderSybilFlood(t *testing.T) {
	var id ports.NodeID
	id[0] = 1
	sched := simclock.New()
	net := simnet.New(sched, 9, simnet.DefaultConfig())
	n := New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())

	// Far more distinct sybil identities than the cap, each gossiping capacity,
	// a bond, and a domain — the exact fields that populate the three maps.
	const sybils = maxPeerInfo * 3
	var root ports.Hash
	root[0] = 0xB0
	for i := 0; i < sybils; i++ {
		var from ports.NodeID
		from[0], from[1], from[2], from[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
		n.handle(from, ports.Message{
			Kind:    ports.MsgFindNode,
			Domain:  uint64(i) | 1,  // non-zero → peerDomains
			CapUsed: 1, CapTotal: 2, // >0 → peerCaps
			BondRoot: root, BondSize: 1, // non-zero → peerBonds
		})
	}

	for name, got := range map[string]int{
		"peerCaps":    len(n.peerCaps),
		"peerBonds":   len(n.peerBonds),
		"peerDomains": len(n.peerDomains),
	} {
		if got > maxPeerInfo {
			t.Fatalf("%s grew to %d under a %d-sybil flood — not bounded by maxPeerInfo=%d", name, got, sybils, maxPeerInfo)
		}
		if got == 0 {
			t.Fatalf("%s is empty — the gossip was not recorded at all", name)
		}
	}
}
