package simnet

// R4.3b — the simnet class oracle, the deterministic mirror of the tcpnet
// classifier (class.go there). SetClass(id, class, group) is what an endpoint
// reports for id after a DIRECT delivery from id; a relay-spliced delivery reports
// (RELAYED, the relay's group); an endpoint that never heard from id reports
// known=false; DIRECT is never downgraded (C-3). A NATed sender with no SetClass
// defaults to one DIRECT group per NAT box (its home /24).

import "github.com/nerolabs/silt/ports"

type simClass struct {
	class ports.PeerClass
	group uint64
}

// SetClass declares the (class, group) an endpoint observes for a DIRECT delivery
// from id (and, for a relay, the group its spliced deliveries carry).
func (n *Network) SetClass(id ports.NodeID, class ports.PeerClass, group uint64) {
	if n.classes == nil {
		n.classes = make(map[ports.NodeID]simClass)
	}
	n.classes[id] = simClass{class: class, group: group}
}

// senderClass is what a direct delivery from id presents: SetClass if declared,
// else one group per NAT box, else unknown.
func (n *Network) senderClass(id ports.NodeID) (simClass, bool) {
	if c, ok := n.classes[id]; ok {
		return c, true
	}
	if nb, ok := n.nat[id]; ok {
		return simClass{class: ports.ClassDirect, group: 1<<40 | uint64(nb.lan)}, true
	}
	return simClass{}, false
}

// observe records a delivery from `from` at this endpoint.
func (e *Endpoint) observe(from ports.NodeID, relayed bool) {
	n := e.net
	var c simClass
	if relayed {
		rc, ok := n.classes[n.relay]
		if !ok {
			return
		}
		c = simClass{class: ports.ClassRelayed, group: rc.group}
	} else {
		sc, ok := n.senderClass(from)
		if !ok {
			return
		}
		c = sc
	}
	if e.seen == nil {
		e.seen = make(map[ports.NodeID]simClass)
	}
	if old, ok := e.seen[from]; ok && old.class == ports.ClassDirect && c.class != ports.ClassDirect {
		return // C-3
	}
	e.seen[from] = c
}

// ClassOf is the ports.PeerClassifier port for this endpoint.
func (e *Endpoint) ClassOf(id ports.NodeID) (ports.PeerClass, uint64, bool) {
	c, ok := e.seen[id]
	if !ok {
		return 0, 0, false
	}
	return c.class, c.group, true
}

var _ ports.PeerClassifier = (*Endpoint)(nil)
