package simnet

// R4.3b (2026-09-04) — the simnet class oracle: the deterministic mirror of the tcpnet
// classifier, so the node-tier liveness sims (core/node G-10) can run under the class
// rule. SetClass(id, class, group) is what an endpoint reports for id after a DIRECT
// delivery from id; a relay-spliced delivery reports (RELAYED, the relay's group); an
// endpoint that never heard from id reports known=false; DIRECT is never downgraded.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

func r43bID(b byte) ports.NodeID { return ports.HashBytes([]byte{b}) }

func TestR43b_SimnetClassOracleMirrorsTheTransport(t *testing.T) {
	s := simclock.New()
	n := New(s, 1, Config{LatencyMin: 1, LatencyMax: 1})
	pub, ponyA, ponyC, relay := r43bID(1), r43bID(2), r43bID(3), r43bID(4)
	for _, id := range []ports.NodeID{pub, ponyA, ponyC, relay} {
		n.Endpoint(id).SetHandler(func(ports.NodeID, ports.Message) {})
	}
	n.NAT(ponyA, 1, false)
	n.NAT(ponyC, 2, false)
	n.Relay(relay)
	n.SetClass(pub, ports.ClassDirect, 0x100)
	n.SetClass(ponyA, ports.ClassDirect, 0x201)
	n.SetClass(ponyC, ports.ClassDirect, 0x202)
	n.SetClass(relay, ports.ClassDirect, 0x1FF)

	if _, _, known := n.Endpoint(pub).ClassOf(ponyA); known {
		t.Fatal("oracle RED: an endpoint that never heard from a peer reports it as known")
	}
	n.Endpoint(ponyA).Send(pub, ports.Message{Kind: ports.MsgFindNode}) // the pony dials out
	s.Run()
	if c, g, known := n.Endpoint(pub).ClassOf(ponyA); !known || c != ports.ClassDirect || g != 0x201 {
		t.Fatalf("oracle RED: after a direct delivery pub reports pony A as (class %d, group %#x, known %v); want (DIRECT, 0x201, true) — a pony that dials out is DIRECT at its own NAT /24 (cert §2.1)", c, g, known)
	}
	n.Endpoint(ponyA).Send(ponyC, ports.Message{Kind: ports.MsgFindNode}) // cross-NAT: spliced through the relay
	s.Run()
	if n.Stats.Relayed == 0 {
		t.Fatal("fixture: the cross-NAT message was not relayed")
	}
	if c, g, known := n.Endpoint(ponyC).ClassOf(ponyA); !known || c != ports.ClassRelayed || g != 0x1FF {
		t.Fatalf("oracle RED: after a relay-spliced delivery pony C reports pony A as (class %d, group %#x, known %v); want (RELAYED, the relay's group 0x1ff, true)", c, g, known)
	}
	// A later relayed delivery never downgrades pub's DIRECT view (C-3): pub→ponyA is
	// direct (return mapping), so exercise the rule on ponyC after a punch upgrade instead:
	n.HolePunch(ponyA, ponyC)
	n.Endpoint(ponyA).Send(ponyC, ports.Message{Kind: ports.MsgFindNode})
	s.Run()
	if c, g, _ := n.Endpoint(ponyC).ClassOf(ponyA); c != ports.ClassDirect || g != 0x201 {
		t.Fatalf("oracle RED: after a hole-punch the direct delivery did not upgrade pony A to (DIRECT, 0x201); got (class %d, group %#x)", c, g)
	}
	if _, _, known := n.Endpoint(pub).ClassOf(r43bID(9)); known {
		t.Fatal("oracle RED: an id SetClass never named is known")
	}
}
