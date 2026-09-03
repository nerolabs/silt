package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

// R4.3a — the DHT domain-0 exemption (R4.2 direction certification §3, finding (ii),
// CERTIFIED): `domainSaturated` returned "never capped" for a peer whose declared domain is
// 0, and the domain is a free self-declaration off the wire, so a key-surround adversary
// defeated the H5-B eclipse cap by simply OMITTING -domain. Fix: an unknown domain counts
// against one shared "unknown" bucket under the same per-domain cap (unknown ⇒ capped, the
// conservative default a routing defence wants). Deliberation:
// docs/thinking/2026-09-03-r4.3a-dht-domain0-exemption-design.md.

// sameBucketIDs returns n NodeIDs that all land in the same k-bucket relative to self.
func sameBucketIDs(t *testing.T, self ports.NodeID, n int, seedBase int64) []ports.NodeID {
	t.Helper()
	want := dht.BucketIndex(self, identity.FromSeed(seedBase).NodeID())
	var out []ports.NodeID
	for i := int64(0); len(out) < n && i < 20_000; i++ {
		id := identity.FromSeed(seedBase + i).NodeID()
		if dht.BucketIndex(self, id) == want {
			out = append(out, id)
		}
	}
	if len(out) < n {
		t.Fatalf("fixture: could not find %d ids in one bucket", n)
	}
	return out
}

func countKept(tab *dht.Table, self ports.NodeID, ids []ports.NodeID) int {
	in := map[ports.NodeID]bool{}
	for _, id := range ids {
		in[id] = true
	}
	kept := 0
	for _, id := range tab.Closest(self, 1000) {
		if in[id] {
			kept++
		}
	}
	return kept
}

// TestR43a_UnknownDomainPeersAreCappedTogether is G-D0-1: a bucket already holding
// perDomainCap unknown-domain peers refuses a further unknown-domain peer. RED on the
// pre-fix tree: domain 0 was exempt, so all six were admitted.
func TestR43a_UnknownDomainPeersAreCappedTogether(t *testing.T) {
	self := identity.FromSeed(1).NodeID()
	tab := dht.NewTable(self, 20)
	domainOf := map[ports.NodeID]uint64{}
	tab.SetDiversity(func(id ports.NodeID) uint64 { return domainOf[id] }, 2)

	unknown := sameBucketIDs(t, self, 6, 20_000)
	for _, id := range unknown { // domainOf stays 0: undeclared
		tab.Observe(id)
	}
	if kept := countKept(tab, self, unknown); kept > 2 {
		t.Fatalf("R4.3a: routing table kept %d UNKNOWN-domain peers in one bucket, cap is 2 — omitting -domain is a free exemption from the H5-B eclipse cap (R4.2 cert §3)", kept)
	}
}

// TestR43a_KnownDistinctDomainStillAdmittedAtUnknownCap is G-D0-2: the diversity
// property survives — an at-cap "unknown" bucket still admits a peer from a KNOWN
// domain, and a known domain at cap still admits a peer from a DIFFERENT known domain.
func TestR43a_KnownDistinctDomainStillAdmittedAtUnknownCap(t *testing.T) {
	self := identity.FromSeed(1).NodeID()
	tab := dht.NewTable(self, 20)
	domainOf := map[ports.NodeID]uint64{}
	tab.SetDiversity(func(id ports.NodeID) uint64 { return domainOf[id] }, 2)

	ids := sameBucketIDs(t, self, 8, 30_000)
	unknown, a, b := ids[:4], ids[4:6], ids[6:8]
	for _, id := range unknown {
		tab.Observe(id) // fills (and, post-fix, saturates) the unknown bucket
	}
	for _, id := range a {
		domainOf[id] = 0xA
		tab.Observe(id)
	}
	for _, id := range b {
		domainOf[id] = 0xB
		tab.Observe(id)
	}
	if kept := countKept(tab, self, a); kept != 2 {
		t.Fatalf("a KNOWN domain must be admitted past a saturated unknown bucket, kept %d of 2", kept)
	}
	if kept := countKept(tab, self, b); kept != 2 {
		t.Fatalf("a second DISTINCT known domain must be admitted past domain A at cap, kept %d of 2", kept)
	}
}

// TestR43a_CapOffIsLegacy is G-D0-3: with the cap disabled nothing changes.
func TestR43a_CapOffIsLegacy(t *testing.T) {
	self := identity.FromSeed(1).NodeID()
	tab := dht.NewTable(self, 20)
	tab.SetDiversity(func(ports.NodeID) uint64 { return 0 }, 0)
	unknown := sameBucketIDs(t, self, 6, 40_000)
	for _, id := range unknown {
		tab.Observe(id)
	}
	if kept := countKept(tab, self, unknown); kept != 6 {
		t.Fatalf("cap off: all 6 must be admitted, kept %d", kept)
	}
}
