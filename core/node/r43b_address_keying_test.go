package node

// R4.3b (2026-09-04) — observed-address keying of the DHT eclipse cap, node tier: the
// gates that need the walk (reply-introduced ids), the simnet NAT model (ponies behind a
// relay) and the announce/resolve path (discoverability). Table-tier halves live in
// core/dht/r43b_address_keying_test.go. Spec: silt-reviews/research/research-outcome/
// R4.3b-relayed-class-and-observed-address-keying-RESEARCH-CERTIFICATION-2026-09-04.md §8
// (G-1, G-2, G-7, G-10, G-12). The node never sees an IP: r43bOracle stands in for the
// transport's ports.PeerClassifier.

import (
	"sort"
	"strconv"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

type r43bClass struct {
	class ports.PeerClass
	group uint64
	known bool
}

// r43bOracle is a per-observer classifier: what THIS node's transport would report.
type r43bOracle struct {
	fixed map[ports.NodeID]r43bClass
	// fn, when set, answers ids absent from fixed (a topology-derived classifier).
	fn func(ports.NodeID) (ports.PeerClass, uint64, bool)
}

func newR43bOracle() *r43bOracle { return &r43bOracle{fixed: map[ports.NodeID]r43bClass{}} }

func (o *r43bOracle) ClassOf(id ports.NodeID) (ports.PeerClass, uint64, bool) {
	if e, ok := o.fixed[id]; ok {
		return e.class, e.group, e.known
	}
	if o.fn != nil {
		return o.fn(id)
	}
	return 0, 0, false
}
func (o *r43bOracle) direct(id ports.NodeID, g uint64) {
	o.fixed[id] = r43bClass{ports.ClassDirect, g, true}
}

// identitiesInBucket grinds n identities whose ids land in the same bucket of self.
func identitiesInBucket(t *testing.T, self ports.NodeID, n int, seedBase int64) []*identity.Identity {
	t.Helper()
	want := dht.BucketIndex(self, identity.FromSeed(seedBase).NodeID())
	var out []*identity.Identity
	for i := int64(0); len(out) < n && i < 20_000; i++ {
		id := identity.FromSeed(seedBase + i)
		if dht.BucketIndex(self, id.NodeID()) == want {
			out = append(out, id)
		}
	}
	if len(out) < n {
		t.Fatalf("fixture: could not find %d identities in one bucket", n)
	}
	return out
}

func r43bTabled(n *Node, id ports.NodeID) bool {
	for _, e := range n.table.Closest(id, n.table.Size()) {
		if e == id {
			return true
		}
	}
	return false
}

// TestR43b_G1_LabelledKeySurroundInOneObservedGroupStaysDiscoverable — G-1 at the
// announce/resolve tier (the H5-B shape, redteam_h5b_test.go): ten eclipsers own the
// NodeIDs closest to a key, declare TEN DISTINCT -domain labels (the free evasion the
// R4.3a ruling measured at 0/6) and all answered from ONE observed /24. Under
// -dht-address-cap=on the key stays discoverable through honest peers in other /24s.
// Control: under off the labelled surround suppresses discovery (the fixture is a real
// surround, and the label rule is inert against distinct labels).
func TestR43b_G1_LabelledKeySurroundInOneObservedGroupStaysDiscoverable(t *testing.T) {
	discoverable := func(t *testing.T, mode dht.AddressMode) bool {
		t.Helper()
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.DefaultConfig())
		key := ports.HashBytes([]byte("surrounded-key-r43b"))
		cands := make([]*identity.Identity, 3000)
		for i := range cands {
			cands[i] = identity.FromSeed(int64(i) + 1)
		}
		sort.Slice(cands, func(i, j int) bool {
			return dht.Closer(key, cands[i].NodeID(), cands[j].NodeID())
		})
		oracle := newR43bOracle()
		mk := func(ident *identity.Identity, domain string, group uint64) *Node {
			cfg := DefaultConfig()
			cfg.Domain = domain
			cfg.DHTDomainCap = 2 // the daemon default: the label rule stays wired, and is inert here
			cfg.RequireSignedProviders = true
			nd := New(ident.NodeID(), cfg, sched, net.Endpoint(ident.NodeID()), memstore.New())
			nd.SetSigner(ident.Signer())
			nd.SetPeerClassifier(oracle)
			nd.SetAddressDiversity(mode, 2, 2, 4)
			oracle.direct(ident.NodeID(), group)
			return nd
		}
		var all []*Node
		for i := 0; i < 10; i++ { // the surround: ten labels, ONE /24
			a := mk(cands[i], "adv-"+strconv.Itoa(i), 0xADD)
			a.SetEclipser(true)
			all = append(all, a)
		}
		var honest []*Node
		for i := 0; i < 8; i++ {
			h := mk(cands[800+i], "honest-"+strconv.Itoa(i), 0xB00+uint64(i))
			honest = append(honest, h)
			all = append(all, h)
		}
		provider := honest[0]
		fetcher := mk(cands[1600], "fetcher", 0xF00)
		all = append(all, fetcher)
		for _, a := range all { // deterministic wiring, as the H5-B gate does
			for _, b := range all {
				if a == b {
					continue
				}
				a.peerDomains[b.ID()] = b.domainID
				a.table.Observe(b.ID())
			}
		}
		provider.provs.Add(provider.providerRecord(key))
		announced := false
		provider.announceAll([]ports.ChunkID{key}, func() { announced = true })
		sched.Run()
		if !announced {
			t.Fatal("announce never completed")
		}
		var provs []ports.NodeID
		resolved := false
		fetcher.resolveProviders(key, func(p []ports.NodeID) { provs = p; resolved = true })
		sched.Run()
		if !resolved {
			t.Fatal("resolve never completed")
		}
		for _, p := range provs {
			if p == provider.ID() {
				return true
			}
		}
		return false
	}
	if discoverable(t, dht.AddressCapOff) {
		t.Fatal("G-1 control: under off a ten-label one-/24 surround should suppress discovery — the fixture is not a real surround (scar-fixture-green-on-wrong-arm)")
	}
	if !discoverable(t, dht.AddressCapOn) {
		t.Fatal("G-1 RED: a key surrounded by ten labelled eclipsers from ONE observed /24 is NOT discoverable under -dht-address-cap=on — the declared label still keys the cap, and N free labels are N domains")
	}
}

// TestR43b_G2_ReplyLearnedIDsAreNeverDirectUntilTheyAnswer — G-2 at the walk. Under held
// delivery: A queries B; B's FindNodeReply carries eight ids in ONE bucket of A that A
// has never contacted. They enter A's table as UNVERIFIED in B's group, at most cap per
// bucket; when one of them answers A's next query it re-keys to its OWN observed /24.
func TestR43b_G2_ReplyLearnedIDsAreNeverDirectUntilTheyAnswer(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()
	identA, identB := identity.FromSeed(1), identity.FromSeed(2)
	xs := identitiesInBucket(t, identA.NodeID(), 8, 70_000)

	mk := func(ident *identity.Identity) *Node {
		return New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	}
	oracle := newR43bOracle()
	oracle.direct(identB.NodeID(), 0xB)
	nA := mk(identA)
	nA.SetPeerClassifier(oracle)
	nA.SetAddressDiversity(dht.AddressCapOn, 2, 2, 4)
	nB := mk(identB)
	xNodes := map[ports.NodeID]*Node{}
	for _, x := range xs {
		xNodes[x.NodeID()] = mk(x)
		nB.table.Observe(x.NodeID())
	}
	nA.table.Observe(identB.NodeID())

	target := xs[0].NodeID()
	nA.IterativeFindNode(target, func([]ports.NodeID) {})

	deliver := func(pick func(simnet.HeldMsg) bool) int {
		n := 0
		for _, m := range net.Pending() {
			if pick(m) {
				net.Deliver(m.ID)
				n++
			}
		}
		return n
	}
	if deliver(func(m simnet.HeldMsg) bool { return m.To == identB.NodeID() && m.Kind == ports.MsgFindNode }) != 1 {
		t.Fatal("fixture: A did not query B")
	}
	if deliver(func(m simnet.HeldMsg) bool { return m.From == identB.NodeID() && m.Kind == ports.MsgFindNodeReply }) != 1 {
		t.Fatal("fixture: B did not reply")
	}
	kept := 0
	for _, x := range xs {
		if r43bTabled(nA, x.NodeID()) {
			kept++
			c, g, known := nA.table.EntryClass(x.NodeID())
			if !known || c != ports.ClassUnverified || g != 0xB {
				t.Fatalf("G-2 RED: a reply-learned, never-contacted id is tabled as (class %d, group %#x, known %v); want (UNVERIFIED, introducer B's group 0xb, true) — the address book / a reply is being read as DIRECT", c, g, known)
			}
		}
	}
	if kept > 2 {
		t.Fatalf("G-2 RED: %d of 8 never-contacted ids from ONE FindNodeReply tabled in one bucket (want ≤ cap 2 per introducer group)", kept)
	}
	if kept == 0 {
		t.Fatal("G-2 fixture: no introduced id was tabled at all (the charge must admit up to the cap)")
	}
	// A now queries the α closest introduced ids. The first to answer re-keys to its own /24.
	var queried ports.NodeID
	for _, m := range net.Pending() {
		if m.From == identA.NodeID() && m.Kind == ports.MsgFindNode {
			queried = m.To
			break
		}
	}
	if queried == (ports.NodeID{}) {
		t.Fatal("fixture: A queried none of the introduced ids")
	}
	oracle.direct(queried, 0xC) // the conversation completes at its own /24
	deliver(func(m simnet.HeldMsg) bool { return m.To == queried && m.Kind == ports.MsgFindNode })
	if deliver(func(m simnet.HeldMsg) bool { return m.From == queried && m.Kind == ports.MsgFindNodeReply }) != 1 {
		t.Fatal("fixture: the queried id did not answer")
	}
	c, g, known := nA.table.EntryClass(queried)
	if !r43bTabled(nA, queried) || !known || c != ports.ClassDirect || g != 0xC {
		t.Fatalf("G-2 RED: an introduced id that ANSWERED A is (tabled %v, class %d, group %#x, known %v); want tabled as (DIRECT, its own /24 0xc, true)", r43bTabled(nA, queried), c, g, known)
	}
}

// TestR43b_G7_ADistinctSenderFloodEvictsNoTabledEntry — G-7 / C-4 at the node: the
// red-team's Finding-C shape (4096+ distinct senders overflowing the peer-keyed soft
// caches) must remove NO tabled entry and change no stored class. The class lives with
// the table entry, not in an evictable cache.
func TestR43b_G7_ADistinctSenderFloodEvictsNoTabledEntry(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	self := identity.FromSeed(1)
	n := New(self.NodeID(), DefaultConfig(), sched, net.Endpoint(self.NodeID()), memstore.New())
	oracle := newR43bOracle()
	n.SetPeerClassifier(oracle)
	n.SetAddressDiversity(dht.AddressCapOn, 2, 2, 4)
	tabled := identitiesInBucket(t, self.NodeID(), 6, 80_000)
	for i, p := range tabled {
		oracle.direct(p.NodeID(), 0x600+uint64(i))
		n.handle(p.NodeID(), ports.Message{Kind: ports.MsgFindNode})
		if !r43bTabled(n, p.NodeID()) {
			t.Fatalf("fixture: peer %d not tabled", i)
		}
	}
	for i := 0; i < maxPeerInfo+1000; i++ { // the flood: distinct never-seen senders
		n.handle(identity.FromSeed(int64(200_000+i)).NodeID(), ports.Message{Kind: ports.MsgFindNode, Domain: 0xF10D})
	}
	for i, p := range tabled {
		if !r43bTabled(n, p.NodeID()) {
			t.Fatalf("G-7 RED (C-4): tabled peer %d was EVICTED by a %d-distinct-sender flood — the Finding-C cache overflow became an eviction weapon", i, maxPeerInfo+1000)
		}
		c, g, known := n.table.EntryClass(p.NodeID())
		if !known || c != ports.ClassDirect || g != 0x600+uint64(i) {
			t.Fatalf("G-7 RED (C-4): tabled peer %d's stored class is (class %d, group %#x, known %v) after the flood; want its admission class (DIRECT, %#x, true) — the class does not live with the entry", i, c, g, known, 0x600+uint64(i))
		}
	}
}

// r43bTopology is the G-10 swarm: public nodes, one relay, NATed ponies each on its own
// home /24. classFor(observer) mirrors what a real transport at observer would see under
// the simnet NAT model: a public sender is DIRECT at its own /24; a pony is DIRECT at
// its NAT /24 at every public node (it dialled out — cert §2.1) and RELAYED at the
// relay's /24 at another pony (relay-spliced both ways until a punch).
type r43bTopology struct {
	public, ponies []*identity.Identity
	relay          *identity.Identity
	group          map[ports.NodeID]uint64
	isPony         map[ports.NodeID]bool
}

func newR43bTopology(nPublic, nPonies int) *r43bTopology {
	tp := &r43bTopology{group: map[ports.NodeID]uint64{}, isPony: map[ports.NodeID]bool{}}
	tp.relay = identity.FromSeed(9000)
	tp.group[tp.relay.NodeID()] = 0x1FF
	for i := 0; i < nPublic; i++ {
		p := identity.FromSeed(int64(9100 + i))
		tp.public = append(tp.public, p)
		tp.group[p.NodeID()] = 0x100 + uint64(i)
	}
	for i := 0; i < nPonies; i++ {
		q := identity.FromSeed(int64(9500 + i))
		tp.ponies = append(tp.ponies, q)
		tp.group[q.NodeID()] = 0x200 + uint64(i)
		tp.isPony[q.NodeID()] = true
	}
	return tp
}

func (tp *r43bTopology) classFor(observer ports.NodeID) func(ports.NodeID) (ports.PeerClass, uint64, bool) {
	return func(id ports.NodeID) (ports.PeerClass, uint64, bool) {
		g, ok := tp.group[id]
		if !ok {
			return 0, 0, false
		}
		if tp.isPony[id] && tp.isPony[observer] {
			return ports.ClassRelayed, tp.group[tp.relay.NodeID()], true
		}
		return ports.ClassDirect, g, true
	}
}

// r43bRunSwarm bootstraps the topology under mode and returns the nodes by id.
func r43bRunSwarm(t *testing.T, tp *r43bTopology, mode dht.AddressMode) (*simclock.Scheduler, *simnet.Network, map[ports.NodeID]*Node) {
	t.Helper()
	return r43bRunSwarmSeed(t, tp, mode, 7)
}

// r43bRunSwarmSeed is r43bRunSwarm under a chosen simnet seed (latency draws).
func r43bRunSwarmSeed(t *testing.T, tp *r43bTopology, mode dht.AddressMode, seed int64) (*simclock.Scheduler, *simnet.Network, map[ports.NodeID]*Node) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, seed, simnet.DefaultConfig())
	net.Relay(tp.relay.NodeID())
	nodes := map[ports.NodeID]*Node{}
	var order []*Node // construction order: the swarm must bootstrap deterministically (a map walk is not)
	mk := func(ident *identity.Identity) *Node {
		cfg := DefaultConfig()
		cfg.RequestRetries = 1
		nd := New(ident.NodeID(), cfg, sched, net.Endpoint(ident.NodeID()), memstore.New())
		o := newR43bOracle()
		o.fn = tp.classFor(ident.NodeID())
		nd.SetPeerClassifier(o)
		nd.SetAddressDiversity(mode, 2, 2, 4)
		nodes[ident.NodeID()] = nd
		order = append(order, nd)
		return nd
	}
	mk(tp.relay)
	for _, p := range tp.public {
		mk(p)
	}
	for i, q := range tp.ponies {
		net.NAT(q.NodeID(), 100+i, false) // each pony behind its own cone NAT
		mk(q)
	}
	boot := tp.public[0].NodeID()
	for _, nd := range order {
		if nd.ID() == boot {
			nd.Bootstrap([]ports.NodeID{tp.public[1].NodeID()}, func() {})
			continue
		}
		nd.Bootstrap([]ports.NodeID{boot}, func() {})
	}
	sched.Run()
	// A second self-lookup round, as the daemon's bootstrap refresh would do.
	for _, nd := range order {
		nd.IterativeFindNode(nd.ID(), func([]ports.NodeID) {})
	}
	sched.Run()
	return sched, net, nodes
}

func r43bPonyPresence(tp *r43bTopology, nodes map[ports.NodeID]*Node) int {
	n := 0
	for _, p := range tp.public {
		for _, e := range nodes[p.NodeID()].table.Closest(p.NodeID(), 1<<20) {
			if tp.isPony[e] {
				n++
			}
		}
	}
	return n
}

// TestR43b_G10_ThirtyPoniesBehindOneRelayStayDiscoverable — G-10 (cert §8, the honest-pony
// liveness sim): 30 NATed ponies behind ONE relay + 10 public nodes under on (R=4,
// cap_relay=2). Every pony tabled at a public node is DIRECT there (it dialled out);
// pony presence in public tables is not below the off baseline; at pony observers the
// relayed entries never exceed K − R per bucket; and every pony stays discoverable from
// a fresh public fetcher.
//
// The presence arm is summed over ten simnet seeds (Builder, 2026-09-04): the proxy is
// chaotic in the seed — the address rule refuses NOTHING at a public node in this swarm
// (0 would-refuse cells, 0 UNVERIFIED entries at the ten public tables under shadow), so
// a per-seed on−off delta is the ponies' walk dynamics (their reserve bites, so they
// query different peers), measured at −2..+6 per seed (8 of 10 seeds on ≥ off, totals
// 1402 vs 1374). One seed is noise; the sum is the measurement.
func TestR43b_G10_ThirtyPoniesBehindOneRelayStayDiscoverable(t *testing.T) {
	tp := newR43bTopology(10, 30)
	baseline, presence := 0, 0
	for seed := int64(1); seed <= 10; seed++ {
		_, _, off := r43bRunSwarmSeed(t, tp, dht.AddressCapOff, seed)
		_, _, onS := r43bRunSwarmSeed(t, tp, dht.AddressCapOn, seed)
		baseline += r43bPonyPresence(tp, off)
		presence += r43bPonyPresence(tp, onS)
	}
	sched, net, on := r43bRunSwarm(t, tp, dht.AddressCapOn)

	for _, p := range tp.public {
		nd := on[p.NodeID()]
		for _, e := range nd.table.Closest(p.NodeID(), 1<<20) {
			if !tp.isPony[e] {
				continue
			}
			c, g, known := nd.table.EntryClass(e)
			if !known || c != ports.ClassDirect || g != tp.group[e] {
				t.Fatalf("G-10 RED: pony %s at public node %s is (class %d, group %#x, known %v); want (DIRECT, its own NAT /24 %#x, true) — a pony that dialled out is being keyed on the relay", e.String()[:8], p.NodeID().String()[:8], c, g, known, tp.group[e])
			}
		}
	}
	if presence < baseline {
		t.Errorf("G-10 RED: pony entries across the 10 public tables, summed over 10 seeds = %d under on, %d under off (the R4.3a-stripped baseline) — honest NATed presence dropped", presence, baseline)
	} else {
		t.Logf("G-10: pony presence in public tables (10 seeds) on=%d off=%d", presence, baseline)
	}
	for _, q := range tp.ponies {
		nd := on[q.NodeID()]
		relayedPerBucket := map[int]int{}
		for _, e := range nd.table.Closest(q.NodeID(), 1<<20) {
			if c, _, known := nd.table.EntryClass(e); known && c == ports.ClassRelayed {
				relayedPerBucket[dht.BucketIndex(q.NodeID(), e)]++
			}
		}
		for b, k := range relayedPerBucket {
			if k > 8-4 {
				t.Fatalf("G-10 RED (C-2): pony %s holds %d RELAYED entries in bucket %d; want ≤ K−R = 4", q.NodeID().String()[:8], k, b)
			}
		}
	}
	// Discoverability from a fresh public fetcher bootstrapped through public[0].
	fetcher := identity.FromSeed(9999)
	tp.group[fetcher.NodeID()] = 0x1F0
	cfg := DefaultConfig()
	f := New(fetcher.NodeID(), cfg, sched, net.Endpoint(fetcher.NodeID()), memstore.New())
	o := newR43bOracle()
	o.fn = tp.classFor(fetcher.NodeID())
	f.SetPeerClassifier(o)
	f.SetAddressDiversity(dht.AddressCapOn, 2, 2, 4)
	f.Bootstrap([]ports.NodeID{tp.public[0].NodeID()}, func() {})
	sched.Run()
	found := 0
	for _, q := range tp.ponies {
		hit := false
		f.IterativeFindNode(q.NodeID(), func(res []ports.NodeID) {
			for _, r := range res {
				if r == q.NodeID() {
					hit = true
				}
			}
		})
		sched.Run()
		if hit {
			found++
		}
	}
	if found != len(tp.ponies) {
		t.Fatalf("G-10 RED: %d of %d ponies behind ONE relay discoverable from a fresh public fetcher under on (R=4, cap_relay=2); want all %d", found, len(tp.ponies), len(tp.ponies))
	}
}

// TestR43b_G10_SpreadSwarmDiscoverabilityUnderOn — the PE's R4.3a density/discoverability
// harness re-run with the class oracle (cert §8 G-10, second clause): 30 honest spread
// nodes in distinct /24s; 10 eclipsers per key from ONE /24 surrounding six keys, joining
// BEFORE the honest swarm (incumbent) or AFTER (late); six provider/fetcher pairs. Pass:
// incumbent ≥ 4/6, late 6/6. Control: the same fixture under off must do worse than on,
// or the fixture does not discriminate.
func TestR43b_G10_SpreadSwarmDiscoverabilityUnderOn(t *testing.T) {
	const pairs = 6
	run := func(t *testing.T, mode dht.AddressMode, incumbent bool) int {
		t.Helper()
		sched := simclock.New()
		net := simnet.New(sched, 3, simnet.DefaultConfig())
		oracle := newR43bOracle()
		mk := func(ident *identity.Identity, group uint64) *Node {
			cfg := DefaultConfig()
			cfg.RequireSignedProviders = true
			cfg.DHTDomainCap = 2
			nd := New(ident.NodeID(), cfg, sched, net.Endpoint(ident.NodeID()), memstore.New())
			nd.SetSigner(ident.Signer())
			nd.SetPeerClassifier(oracle)
			nd.SetAddressDiversity(mode, 2, 2, 4)
			oracle.direct(ident.NodeID(), group)
			return nd
		}
		keys := make([]ports.Hash, pairs)
		for i := range keys {
			keys[i] = ports.HashBytes([]byte("spread-key-" + strconv.Itoa(i)))
		}
		pool := make([]*identity.Identity, 4000)
		for i := range pool {
			pool[i] = identity.FromSeed(int64(30_000 + i))
		}
		used := map[ports.NodeID]bool{}
		var eclipsers []*Node
		for _, key := range keys { // the 10 closest unused ids to each key, one /24
			sort.Slice(pool, func(i, j int) bool { return dht.Closer(key, pool[i].NodeID(), pool[j].NodeID()) })
			for i, n := 0, 0; n < 10; i++ {
				if used[pool[i].NodeID()] {
					continue
				}
				used[pool[i].NodeID()] = true
				a := mk(pool[i], 0xADD)
				a.SetEclipser(true)
				eclipsers = append(eclipsers, a)
				n++
			}
		}
		var honest []*Node
		for i := 0; i < 30; i++ {
			honest = append(honest, mk(identity.FromSeed(int64(40_000+i)), 0xB00+uint64(i)))
		}
		seed := honest[0]
		boot := func(ns []*Node) {
			for _, nd := range ns {
				if nd == seed {
					continue
				}
				nd.Bootstrap([]ports.NodeID{seed.ID()}, func() {})
			}
			sched.Run()
		}
		if incumbent {
			boot(eclipsers)
			boot(honest)
		} else {
			boot(honest)
			boot(eclipsers)
		}
		for _, nd := range append(append([]*Node{}, honest...), eclipsers...) {
			nd.IterativeFindNode(nd.ID(), func([]ports.NodeID) {})
		}
		sched.Run()
		found := 0
		for i := 0; i < pairs; i++ {
			provider, fetcher := honest[1+i], honest[10+i]
			provider.provs.Add(provider.providerRecord(keys[i]))
			provider.announceAll([]ports.ChunkID{keys[i]}, func() {})
			sched.Run()
			var provs []ports.NodeID
			fetcher.resolveProviders(keys[i], func(p []ports.NodeID) { provs = p })
			sched.Run()
			for _, p := range provs {
				if p == provider.ID() {
					found++
					break
				}
			}
		}
		return found
	}
	incOn, lateOn := run(t, dht.AddressCapOn, true), run(t, dht.AddressCapOn, false)
	incOff, lateOff := run(t, dht.AddressCapOff, true), run(t, dht.AddressCapOff, false)
	t.Logf("G-10 spread harness: incumbent on=%d/6 off=%d/6; late on=%d/6 off=%d/6", incOn, incOff, lateOn, lateOff)
	if incOn <= incOff && lateOn <= lateOff {
		t.Fatalf("G-10 control: on (incumbent %d, late %d) is no better than off (incumbent %d, late %d) — the fixture does not discriminate (scar-fixture-green-on-wrong-arm)", incOn, lateOn, incOff, lateOff)
	}
	if incOn < 4 {
		t.Errorf("G-10 RED: incumbent eclipsers (one /24, ten per key) — %d/6 keys discoverable under on; the cert's pass bar is ≥ 4/6", incOn)
	}
	if lateOn != pairs {
		t.Errorf("G-10 RED: late eclipsers — %d/6 keys discoverable under on; the cert's pass bar is 6/6", lateOn)
	}
}
