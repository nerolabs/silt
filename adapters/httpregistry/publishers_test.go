package httpregistry

import (
	"net"
	"testing"
)

func TestPublisherAllowedReadsOnlyTheTCPPeer(t *testing.T) {
	_, n, _ := net.ParseCIDR("203.0.113.0/24")
	nets := []*net.IPNet{n}
	for addr, want := range map[string]bool{
		"203.0.113.7:51234": true,
		"203.0.114.7:51234": false,
		"[::1]:443":         false,
		"garbage":           false,
	} {
		if got := publisherAllowed(nets, addr); got != want {
			t.Errorf("%s: allowed=%v, want %v", addr, got, want)
		}
	}
	if !publisherAllowed(nil, "198.51.100.1:1") {
		t.Error("with no list, everyone may publish")
	}
}
