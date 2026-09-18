package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// A publish/fetch client must survive ONE lost packet with its route into the
// network intact.
//
// The adverse internet is the everyday case, and the rule it imposes is
// retry-don't-evict: a live peer must not be torn out of the routing table
// because a single packet was slow or dropped. The rule bites hardest on the
// swarm client, whose whole routing table at start-up is the bootstrap peers
// named on its command line — evict the one it was given and it has no route
// left, so a WAN that merely hiccuped fails the publish or the fetch outright.
//
// Three arms, because the rule alone could pass for the wrong reason:
//
//	rule     — the SHIPPED client posture keeps its only route across one drop.
//	vacuity  — with NOTHING dropped the route is present, so the rule is not
//	           asserting over a peer that was never in the table.
//	ablation — a posture with the retry REMOVED loses the route on the same one
//	           drop, so the rule passes because of the retry and not because
//	           eviction has quietly stopped working.
func TestOneLostPacketDoesNotEvictTheSwarmClientsOnlyRoute(t *testing.T) {
	// routeSurvives builds a two-node world — the client and the single peer it
	// was bootstrapped with — sends one discovery RPC, optionally loses the
	// first packet of it, delivers everything after that, and reports whether
	// the peer is still routable when the dust settles.
	routeSurvives := func(t *testing.T, cfg Config, dropFirst bool) bool {
		t.Helper()
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.DefaultConfig())
		net.EnableHeldDelivery()

		clientID := identity.FromSeed(1)
		peerID := identity.FromSeed(2)

		client := New(clientID.NodeID(), cfg, sched, net.Endpoint(clientID.NodeID()), memstore.New())
		client.SetSigner(clientID.Signer())
		client.SetEphemeral(true) // what a swarm add/get client is

		peerCfg := DefaultConfig()
		peer := New(peerID.NodeID(), peerCfg, sched, net.Endpoint(peerID.NodeID()), memstore.New())
		peer.SetSigner(peerID.Signer())

		// The one route the client was given, exactly as -peers supplies it.
		client.observeSeed(peerID.NodeID())

		target := ports.HashBytes([]byte("some-content-key"))
		client.IterativeFindNode(target, func([]ports.NodeID) {})

		// Drive to quiescence: lose at most the FIRST packet, deliver every
		// packet after it, and let the clock fire the timeouts and retries in
		// between. A bounded loop so a wedge fails the test rather than hanging.
		lost := !dropFirst
		for step := 0; step < 500; step++ {
			if p := net.Pending(); len(p) > 0 {
				if !lost {
					net.DropPending(p[0].ID)
					lost = true
				} else {
					net.Deliver(p[0].ID)
				}
				continue
			}
			if !sched.Step() {
				break
			}
		}

		for _, id := range client.table.Closest(ports.Hash(peerID.NodeID()), 8) {
			if id == peerID.NodeID() {
				return true
			}
		}
		return false
	}

	t.Run("rule: the shipped client keeps its only route across one lost packet", func(t *testing.T) {
		if !routeSurvives(t, SwarmClientConfig(), true) {
			t.Fatal("one dropped packet evicted the swarm client's only route into the network: " +
				"a publish or fetch on a WAN that hiccuped once now has no peer left to ask")
		}
	})

	t.Run("vacuity: the route is there when nothing is dropped", func(t *testing.T) {
		if !routeSurvives(t, SwarmClientConfig(), false) {
			t.Fatal("the peer was not routable even with NO packet dropped — the rule arm above " +
				"would pass or fail for a reason that has nothing to do with the retry")
		}
	})

	t.Run("ablation: without the retry the same drop evicts the route", func(t *testing.T) {
		noRetry := SwarmClientConfig()
		noRetry.RequestRetries = 0
		if routeSurvives(t, noRetry, true) {
			t.Fatal("the route survived one drop with the retry REMOVED — eviction is not happening " +
				"at all, so the rule arm proves nothing about retry-don't-evict")
		}
	})
}
