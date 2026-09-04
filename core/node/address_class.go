package node

// R4.3b — the node's half of observed-address keying. The DHT's eclipse cap reads
// the transport's ports.PeerClassifier (an opaque salted (class, group) per peer
// it conversed with); the declared -domain label stays what it was for the C2
// concentration metric and preferFreshDomain, and keeps its (inert-against-an-
// adversary) legacy admission rule. Three admission paths feed the table:
//   - a message from `from` on a live conversation → Observe (classified by the transport);
//   - every id in a FindNodeReply → ObserveIntroduced(id, replier): UNVERIFIED, charged
//     to the replier's group until it answers;
//   - -bootstrap seeds and -persistent-peers (MarkSeed / AddStaticPeer) → ObserveStatic,
//     exempt from every cap (operator-typed, count-bounded).
// Cert: silt-reviews/research/research-outcome/R4.3b-relayed-class-and-observed-
// address-keying-RESEARCH-CERTIFICATION-2026-09-04.md §5, §7.

import (
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

// addressCapConfig is what SetAddressDiversity was called with, so the classifier
// and the cap can be wired in either order.
type addressCapConfig struct {
	set                          bool
	mode                         dht.AddressMode
	capDirect, capRelay, reserve int
}

// SetPeerClassifier wires the optional observed-address classifier (the transport,
// or a test oracle). nil = no classifier: every entry is unclassified, bounded by
// the reserve under `on`.
func (n *Node) SetPeerClassifier(cl ports.PeerClassifier) {
	n.classifier = cl
	if n.addrCap.set {
		n.table.SetAddressDiversity(cl, n.addrCap.capDirect, n.addrCap.capRelay, n.addrCap.reserve, n.addrCap.mode)
	}
}

// SetAddressDiversity configures the observed-address cap on this node's table
// (-dht-address-cap=off|shadow|on, default shadow). The reserve is a security
// parameter: the table clamps it to ≥ K/2 (cert §4).
func (n *Node) SetAddressDiversity(mode dht.AddressMode, capDirect, capRelay, reserve int) {
	n.addrCap = addressCapConfig{set: true, mode: mode, capDirect: capDirect, capRelay: capRelay, reserve: reserve}
	n.table.SetAddressDiversity(n.classifier, capDirect, capRelay, reserve, mode)
}

// MarkSeed marks id as an operator-configured -bootstrap seed: exempt from the
// address cap (ObserveStatic) but, unlike AddStaticPeer, still evicted on a
// reachability timeout. peers.json / DNS / mDNS-discovered peers are NOT seeds in
// this sense: they are discovered data, and a warm restart must not exempt a
// cached table from the cap.
func (n *Node) MarkSeed(id ports.NodeID) {
	if n.seedPeers == nil {
		n.seedPeers = make(map[ports.NodeID]bool)
	}
	n.seedPeers[id] = true
}

// observeSeed tables a bootstrap seed: static (cap-exempt) if the operator typed
// it, an ordinary observation otherwise.
func (n *Node) observeSeed(id ports.NodeID) {
	if n.staticPeers[id] || n.seedPeers[id] {
		n.table.ObserveStatic(id)
		return
	}
	n.table.Observe(id)
}
