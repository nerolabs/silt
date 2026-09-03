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

// TestR43b_OPENBREAK_LabelledSybilsDefeatTheDomainCap records the LIVE residual R4.3b
// closes: N Sybils declaring N distinct free labels are N domains, so the per-domain cap
// never fires and they fill the bucket. This gate asserts the attack SUCCEEDS today (the
// break is open and owned, ROADMAP R4.3b). When R4.3b lands it goes RED and must be
// flipped to assert the defence — an open break may not close silently.
func TestR43b_OPENBREAK_LabelledSybilsDefeatTheDomainCap(t *testing.T) {
	self := identity.FromSeed(1).NodeID()
	tab := dht.NewTable(self, 8)
	domainOf := map[ports.NodeID]uint64{}
	tab.SetDiversity(func(id ports.NodeID) uint64 { return domainOf[id] }, 2)

	ids := sameBucketIDs(t, self, 10, 60_000)
	sybils, honest := ids[:8], ids[8:10]
	for i, id := range sybils {
		domainOf[id] = 0x1000 + uint64(i) // a distinct free label each
		tab.Observe(id)
	}
	for _, id := range honest {
		domainOf[id] = 0x2000
		tab.Observe(id)
	}
	if kept := countKept(tab, self, sybils); kept != 8 {
		t.Fatalf("OPEN BREAK changed: %d of 8 labelled Sybils admitted (expected all 8 — a declared label costs $0). If R4.3b landed, flip this gate to assert the defence and close the ROADMAP residual", kept)
	}
	if kept := countKept(tab, self, honest); kept != 0 {
		t.Fatalf("OPEN BREAK changed: %d honest peers admitted past a labelled surround (expected 0 under the declared-label cap)", kept)
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
