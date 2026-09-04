package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

// R4.3a (2026-09-03) — the DHT domain-0 question, as RULED: the "unknown ⇒ capped" table
// change was built (PR #715) and then STRIPPED after the red-team showed it is a regression
// in the default (domainless) swarm: with everyone undeclared, two early Sybils lock a K=8
// bucket. No declared-label design prices an eclipse (N free labels ⇒ N domains). The close
// is R4.3b, observed-address keying. These gates encode the red-team's findings so the
// regression cannot return and the open residual cannot close silently.
// Source: silt-reviews/red-team/RED-TEAM-R4.3b-dht-eclipse-keying-2026-09-03.md.

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

// TestR43a_TwoUndeclaredSybilsDoNotLockADomainlessBucket is the red-team's Attack A,
// inverted into a regression gate: in the default swarm (nobody sets -domain), two
// undeclared Sybils observed first must NOT exclude the honest undeclared peers that
// follow. Under the stripped "unknown ⇒ capped" rule honest_kept was 0 (RED); with the
// exemption kept it is 6 (the bucket fills to K). Exclusion cost stays 8 identities.
func TestR43a_TwoUndeclaredSybilsDoNotLockADomainlessBucket(t *testing.T) {
	self := identity.FromSeed(1).NodeID()
	tab := dht.NewTable(self, 8) // K=8, the production bucket size
	domainOf := map[ports.NodeID]uint64{}
	tab.SetDiversity(func(id ports.NodeID) uint64 { return domainOf[id] }, 2)

	ids := sameBucketIDs(t, self, 10, 50_000)
	sybils, honest := ids[:2], ids[2:8]
	for _, id := range sybils { // undeclared, observed FIRST (incumbents win)
		tab.Observe(id)
	}
	for _, id := range honest { // undeclared honest peers arriving after
		tab.Observe(id)
	}
	if kept := countKept(tab, self, honest); kept != 6 {
		t.Fatalf("red-team Attack A: %d of 6 honest undeclared peers admitted after 2 undeclared Sybils in a K=8 bucket — a cap on the unknown pool lets 2 identities lock a bucket in the default swarm (exclusion cost 8 → 2)", kept)
	}
}

// TestR43b_OPENBREAK_LabelledSybilsDefeatTheDomainCap — FLIPPED 2026-09-04 (R4.3b G-1).
// Until R4.3b this gate asserted the LIVE break: N Sybils declaring N distinct free labels
// were N domains, the per-domain cap never fired, and they filled the bucket. R4.3b keys
// the cap on the OBSERVED contacted-at address instead: the same eight Sybils, eight
// labels, ONE observed /24, hold at most cap_direct of a K=8 bucket under
// -dht-address-cap=on, and the two honest peers from other /24s are admitted. The
// inverse is pinned so the break cannot reopen silently: under off the labelled surround
// still fills the bucket (the label rule is inert against distinct labels), and it is the
// address rule alone that defends. Name kept: CHANGELOG cites it.
func TestR43b_OPENBREAK_LabelledSybilsDefeatTheDomainCap(t *testing.T) {
	self := identity.FromSeed(1).NodeID()
	ids := sameBucketIDs(t, self, 10, 60_000)
	sybils, honest := ids[:8], ids[8:10]

	run := func(mode dht.AddressMode) (int, int) {
		tab := dht.NewTable(self, 8)
		labels := map[ports.NodeID]uint64{}
		tab.SetDiversity(func(id ports.NodeID) uint64 { return labels[id] }, 2) // the legacy label rule, still wired
		oracle := newR43bOracle()
		tab.SetAddressDiversity(oracle, 2, 2, 4, mode)
		for i, id := range sybils {
			labels[id] = 0x1000 + uint64(i) // a distinct free label each …
			oracle.direct(id, 0x5B24)       // … all answered from ONE observed /24
			tab.Observe(id)
		}
		for i, id := range honest {
			labels[id] = 0x2000
			oracle.direct(id, 0xB00+uint64(i))
			tab.Observe(id)
		}
		return countKept(tab, self, sybils), countKept(tab, self, honest)
	}
	if s, h := run(dht.AddressCapOff); s != 8 || h != 0 {
		t.Fatalf("inverse pin: under off eight labelled Sybils from one /24 should still fill the bucket (8 kept, 0 honest); got %d/%d — the declared label is capping, or the fixture is no longer a surround", s, h)
	}
	s, h := run(dht.AddressCapOn)
	if s > 2 {
		t.Fatalf("G-1 RED: %d of 8 labelled Sybils from ONE observed /24 admitted into a K=8 bucket under -dht-address-cap=on (cap_direct=2) — a declared label still keys the cap; N free labels are N domains", s)
	}
	if h != 2 {
		t.Fatalf("G-1 RED: %d of 2 honest peers (distinct observed /24s) admitted past the labelled surround under on; want 2", h)
	}
}

// TestR43a_HelloWritesOnlyTheSendersOwnDomain is the red-team's boundary that HELD:
// the only write to peerDomains is keyed by the authenticated sender, so a peer can set
// its own domain and nobody else's. A third-party poisoning path would be a new
// exemption/eviction lever; this pins its absence.
func TestR43a_HelloWritesOnlyTheSendersOwnDomain(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	self := identity.FromSeed(1)
	n := New(self.NodeID(), DefaultConfig(), sched, net.Endpoint(self.NodeID()), memstore.New())
	sender, victim := identity.FromSeed(2).NodeID(), identity.FromSeed(3).NodeID()
	n.peerDomains[victim] = 0xB0B
	n.handle(sender, ports.Message{Domain: 0xA11})
	if n.peerDomains[sender] != 0xA11 {
		t.Fatalf("sender's own domain not recorded: %x", n.peerDomains[sender])
	}
	if n.peerDomains[victim] != 0xB0B {
		t.Fatalf("a hello from one peer changed ANOTHER peer's domain: %x", n.peerDomains[victim])
	}
	if len(n.peerDomains) != 2 {
		t.Fatalf("a hello wrote %d domain entries; only the sender's may change", len(n.peerDomains))
	}
}
