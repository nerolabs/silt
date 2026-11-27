package main

import (
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

// TestAddressCapShadowTallyCountsOnlyTheShippedSettings. The shadow records would-be refusals under
// several candidate settings at once; the tally a field run collects must count only the settings
// the node ships with, or a decision to turn the cap on would rest on someone else's numbers.
func TestAddressCapShadowTallyCountsOnlyTheShippedSettings(t *testing.T) {
	refusals := map[dht.ShadowKey]int{
		{Bucket: 3, Class: ports.ClassDirect, Reserve: 4, CapRelay: 2}:     5,
		{Bucket: 7, Class: ports.ClassDirect, Reserve: 4, CapRelay: 2}:     2,
		{Bucket: 3, Class: ports.ClassRelayed, Reserve: 4, CapRelay: 2}:    3,
		{Bucket: 3, Class: ports.ClassUnverified, Reserve: 4, CapRelay: 2}: 1,
		{Bucket: 3, Class: ports.ClassDirect, Reserve: 6, CapRelay: 2}:     100, // another candidate
		{Bucket: 3, Class: ports.ClassRelayed, Reserve: 4, CapRelay: 1}:    100, // another candidate
	}
	got := addressCapShadowTally(refusals, 4, 2, 40)
	want := "would refuse 11 (direct 7, relayed 3, unverified 1) at reserve=4 cap_relay=2; routing table holds 40"
	if !strings.Contains(got, want) {
		t.Fatalf("tally:\n  got  %s\n  want …%s", got, want)
	}
	if empty := addressCapShadowTally(nil, 4, 2, 0); !strings.Contains(empty, "would refuse 0 (direct 0, relayed 0, unverified 0)") {
		t.Fatalf("an empty shadow must still report its zero: %s", empty)
	}
}
