package tcpnet

import (
	"net"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// TestReachabilityDialsBackOnlyTheObservedHost: the dial-back that answers a reachability
// check goes only to the host the asker connected from. The advertised address is the
// asker's own claim; dialing it would let anyone aim this node at any host and port.
func TestReachabilityDialsBackOnlyTheObservedHost(t *testing.T) {
	tr := &Transport{classes: map[ports.NodeID]peerClass{}}
	id := identity.FromSeed(720).NodeID()
	tr.observeConn(id, &net.TCPAddr{IP: net.ParseIP("198.51.100.7"), Port: 50123}, false)
	for addr, want := range map[string]bool{
		"198.51.100.7:4001": true,  // its own host, any port
		"203.0.113.9:22":    false, // someone else's host
		"localhost:4001":    false, // a name, not the observed address
		"garbage":           false,
	} {
		if got := tr.dialBackMatchesObserved(id, addr); got != want {
			t.Errorf("%s: dial-back allowed=%v, want %v", addr, got, want)
		}
	}
	other := identity.FromSeed(721).NodeID()
	tr.observeConn(other, &net.TCPAddr{IP: net.ParseIP("198.51.100.8"), Port: 1}, true) // relayed
	if tr.dialBackMatchesObserved(other, "198.51.100.8:4001") {
		t.Error("a relayed connection's address was trusted for a dial-back")
	}
}
