package dht

// R4.3b (2026-09-04) — the H5-B eclipse cap keyed on the OBSERVED contacted-at address
// (geth / Bitcoin Core form), at the routing-table tier. RED-first gates G-1..G-8 (table
// half) from the research certification §8:
// silt-reviews/research/research-outcome/R4.3b-relayed-class-and-observed-address-keying-
// RESEARCH-CERTIFICATION-2026-09-04.md. The four build conditions: C-1 one group namespace
// per /24 across classes; C-2 the reserve bounds ALL non-DIRECT entries at K − R; C-3 DIRECT
// is never downgraded; C-4 the class lives with the table entry. Node-tier halves live in
// core/node/r43b_address_keying_test.go; transport halves in adapters/tcpnet/r43b_class_test.go.
//
// The table never sees an IP: the oracle below stands in for the transport's
// ports.PeerClassifier and hands the table an opaque (class, group). Group values are
// arbitrary non-zero uint64s; group 0 is the exempt (loopback) group.

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/nerolabs/silt/ports"
)

const (
	r43bK        = 8 // the production bucket size (node.DefaultConfig().K)
	r43bCapDir   = 2 // cap_direct (daemon default -dht-domain-cap)
	r43bCapRelay = 2 // cap_relay, the shadow hypothesis (cert §4)
	r43bReserve  = 4 // R, the shadow hypothesis (cert §4): K − R = 4 free non-DIRECT slots
)

type classEntry struct {
	class ports.PeerClass
	group uint64
	known bool
}

// oracle is the test's stand-in for the transport classifier: per-id truth the table
// reads through ports.PeerClassifier. Mutating it models "the transport now knows".
type oracle map[ports.NodeID]classEntry

func (o oracle) ClassOf(id ports.NodeID) (ports.PeerClass, uint64, bool) {
	e := o[id]
	return e.class, e.group, e.known
}
func (o oracle) direct(id ports.NodeID, g uint64)  { o[id] = classEntry{ports.ClassDirect, g, true} }
func (o oracle) relayed(id ports.NodeID, g uint64) { o[id] = classEntry{ports.ClassRelayed, g, true} }
func (o oracle) unknown(id ports.NodeID)           { delete(o, id) }

// idInBucket builds an id that lands in bucket b of self's table: same bits above b,
// bit b flipped, bits below b random. (Random ids land in bucket b with probability
// 2^-(256-b), so grinding is hopeless for a close bucket; construction is exact.)
func idInBucket(rng *rand.Rand, self ports.NodeID, b int) ports.NodeID {
	id := self
	i := 31 - b/8
	hi := uint(b % 8)
	id[i] ^= 1 << hi
	if hi > 0 {
		id[i] ^= byte(rng.Intn(1 << hi)) // bits below hi within the byte
	}
	for j := i + 1; j < 32; j++ {
		id[j] = byte(rng.Intn(256))
	}
	if got := BucketIndex(self, id); got != b {
		panic(fmt.Sprintf("idInBucket: built bucket %d, want %d", got, b))
	}
	return id
}

func idsInBucket(rng *rand.Rand, self ports.NodeID, b, n int) []ports.NodeID {
	seen := map[ports.NodeID]bool{}
	out := make([]ports.NodeID, 0, n)
	for len(out) < n {
		id := idInBucket(rng, self, b)
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (t *Table) has(id ports.NodeID) bool {
	i := BucketIndex(t.self, id)
	if i < 0 {
		return false
	}
	for _, e := range t.buckets[i] {
		if e == id {
			return true
		}
	}
	return false
}

func (t *Table) kept(ids []ports.NodeID) int {
	n := 0
	for _, id := range ids {
		if t.has(id) {
			n++
		}
	}
	return n
}

// snapshot is id → bucket index for every tabled entry (bucket ORDER is not compared:
// the differential is about admission, and a move-to-back is not an admission).
func (t *Table) snapshot() map[ports.NodeID]int {
	out := map[ports.NodeID]int{}
	for i, b := range t.buckets {
		for _, id := range b {
			out[id] = i
		}
	}
	return out
}

func newOnTable(self ports.NodeID, o oracle, mode AddressMode) *Table {
	tab := NewTable(self, r43bK)
	tab.SetAddressDiversity(o, r43bCapDir, r43bCapRelay, r43bReserve, mode)
	return tab
}

func classOf(t *testing.T, tab *Table, id ports.NodeID, wantClass ports.PeerClass, wantGroup uint64, gate string) {
	t.Helper()
	c, g, known := tab.EntryClass(id)
	if !known || c != wantClass || g != wantGroup {
		t.Fatalf("%s: EntryClass = (class %d, group %#x, known %v); want (class %d, group %#x, known true) — the class must live WITH the table entry (C-4)", gate, c, g, known, wantClass, wantGroup)
	}
}

// TestR43b_G1_OneObservedGroupIsCappedUnderOn — G-1, table half. Eight Sybils that all
// answered from ONE observed /24 are ONE group whatever they declare: at most cap_direct
// of them enter a K=8 bucket under `on`, and the six honest peers from six distinct
// /24s that arrive AFTER them are all admitted. The inverse is pinned: under `off` the
// same surround fills the bucket (so the defence is the address rule, nothing else).
// The declared-label half of G-1 (ten distinct -domain labels) is
// core/node/r43a_dht_domain0_test.go TestR43b_OPENBREAK_LabelledSybilsDefeatTheDomainCap,
// rewritten to assert the defence.
func TestR43b_G1_OneObservedGroupIsCappedUnderOn(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var self ports.NodeID
	rng.Read(self[:])
	ids := idsInBucket(rng, self, 200, 14)
	sybils, honest := ids[:8], ids[8:14]

	run := func(mode AddressMode) (int, int) {
		o := oracle{}
		tab := newOnTable(self, o, mode)
		for _, id := range sybils { // the surround arrives first: incumbents win
			o.direct(id, 0xA24)
			tab.Observe(id)
		}
		for i, id := range honest {
			o.direct(id, 0xB00+uint64(i))
			tab.Observe(id)
		}
		return tab.kept(sybils), tab.kept(honest)
	}
	s, h := run(AddressCapOn)
	if s > r43bCapDir {
		t.Fatalf("G-1 RED: %d of 8 Sybils from ONE observed /24 admitted into a K=8 bucket under on (cap_direct=%d) — the eclipse still costs one /24", s, r43bCapDir)
	}
	if h != 6 {
		t.Fatalf("G-1 RED: %d of 6 honest peers (six distinct observed /24s) admitted after a one-/24 surround; want 6 (the bucket holds %d Sybils + 6 honest = 8)", h, s)
	}
	if s, h := run(AddressCapOff); s != 8 || h != 0 {
		t.Fatalf("G-1 inverse pin: under off the one-/24 surround should fill the bucket (8 Sybils, 0 honest); got %d/%d — something other than the address rule is capping", s, h)
	}
}

// TestR43b_G2_IntroducedIDsAreChargedToTheIntroducer — G-2, table half (Bitcoin Core's
// srcgroup rule, cert §5). Ids learned from a FindNodeReply and never contacted enter as
// UNVERIFIED in the INTRODUCER's group: one reply seeds at most cap uncontacted entries
// per bucket; the entry carries the introducer's group; on first contact it re-keys to
// its own observed group.
func TestR43b_G2_IntroducedIDsAreChargedToTheIntroducer(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var self ports.NodeID
	rng.Read(self[:])
	o := oracle{}
	tab := newOnTable(self, o, AddressCapOn)

	introducer := idInBucket(rng, self, 255) // a far peer that answered a query
	o.direct(introducer, 0x11)
	tab.Observe(introducer)

	xs := idsInBucket(rng, self, 200, 8) // the reply's ids: never contacted, unknown to the oracle
	for _, x := range xs {
		tab.ObserveIntroduced(x, introducer)
	}
	if k := tab.kept(xs); k > r43bCapDir {
		t.Fatalf("G-2 RED: %d of 8 never-contacted ids from ONE reply admitted into a bucket (want ≤ %d per introducer group) — a replier can seed a bucket with uncontacted ids for free", k, r43bCapDir)
	} else if k == 0 {
		t.Fatalf("G-2 fixture: 0 introduced ids admitted — the charge must admit up to the cap, not refuse")
	}
	var kept []ports.NodeID
	for _, x := range xs {
		if tab.has(x) {
			classOf(t, tab, x, ports.ClassUnverified, 0x11, "G-2 RED (introduced entry)")
			kept = append(kept, x)
		}
	}
	// A second introducer from another /24 gets its own cap in the same bucket.
	introducer2 := idInBucket(rng, self, 254)
	o.direct(introducer2, 0x22)
	tab.Observe(introducer2)
	ys := idsInBucket(rng, self, 200, 8)
	for _, y := range ys {
		tab.ObserveIntroduced(y, introducer2)
	}
	if k := tab.kept(ys); k > r43bCapDir || k == 0 {
		t.Fatalf("G-2 RED: second introducer group admitted %d uncontacted ids (want 1..%d)", k, r43bCapDir)
	}
	// First contact re-keys the entry to its OWN observed group.
	o.direct(kept[0], 0x33)
	tab.Observe(kept[0])
	classOf(t, tab, kept[0], ports.ClassDirect, 0x33, "G-2 RED (re-key on contact)")
	// An introduced-but-refused id that later answers at its own /24 is admitted on its own merit.
	var refused ports.NodeID
	for _, x := range xs {
		if !tab.has(x) {
			refused = x
			break
		}
	}
	o.direct(refused, 0x44)
	tab.Observe(refused)
	if !tab.has(refused) {
		t.Fatalf("G-2 RED: an id refused as uncontacted (introducer group full) was not admitted when it later answered from its own /24 (0x44, unsaturated)")
	}
	classOf(t, tab, refused, ports.ClassDirect, 0x44, "G-2 RED (late contact)")
}

// TestR43b_G3_RelayGroupIsOneNamespaceAcrossClasses — G-3 / C-1. RELAYED is its own
// class (cap_relay per relay group per bucket) but the GROUP namespace is shared: the
// relay's own DIRECT entry, a direct Sybil on the relay's /24 and the relay's clients all
// count in ONE group, so a /24 hosting a relay never yields cap_direct + cap_relay slots.
// Plus geth's table-wide per-group cap (10).
func TestR43b_G3_RelayGroupIsOneNamespaceAcrossClasses(t *testing.T) {
	t.Run("N Sybils behind one relay hold at most cap_relay", func(t *testing.T) {
		rng := rand.New(rand.NewSource(3))
		var self ports.NodeID
		rng.Read(self[:])
		o := oracle{}
		tab := newOnTable(self, o, AddressCapOn)
		xs := idsInBucket(rng, self, 200, 8)
		for _, x := range xs {
			o.relayed(x, 0x7E1A) // all reached through the same relay: the relay's group
			tab.Observe(x)
		}
		if k := tab.kept(xs); k > r43bCapRelay {
			t.Fatalf("G-3 RED: %d of 8 peers reached through ONE relay admitted into a bucket (cap_relay=%d) — a relay's address is N free groups", k, r43bCapRelay)
		}
		for _, x := range xs {
			if tab.has(x) {
				classOf(t, tab, x, ports.ClassRelayed, 0x7E1A, "G-3 RED (relayed entry)")
			}
		}
	})
	t.Run("a /24 hosting a relay plus a direct Sybil yields the composed bound, never cap_direct+cap_relay", func(t *testing.T) {
		rng := rand.New(rand.NewSource(4))
		var self ports.NodeID
		rng.Read(self[:])
		o := oracle{}
		tab := NewTable(self, r43bK)
		capDir, capRelay := 2, 4 // distinct on purpose: the composed bound must not be their sum
		tab.SetAddressDiversity(o, capDir, capRelay, r43bReserve, AddressCapOn)
		ids := idsInBucket(rng, self, 200, 8)
		relayNode, directSybil, clients := ids[0], ids[1], ids[2:8]
		o.direct(relayNode, 0x24)
		tab.Observe(relayNode)
		o.direct(directSybil, 0x24)
		tab.Observe(directSybil)
		for _, c := range clients {
			o.relayed(c, 0x24)
			tab.Observe(c)
		}
		inGroup := tab.kept(ids)
		if inGroup >= capDir+capRelay {
			t.Fatalf("G-3 RED (C-1): the /24 0x24 holds %d entries in one bucket (relay + direct Sybil + %d relayed clients) = cap_direct+cap_relay — the group namespace is split by class, so a relay /24 costs half", inGroup, tab.kept(clients))
		}
		if inGroup > max(capDir, capRelay) {
			t.Fatalf("G-3 RED (C-1): the /24 0x24 holds %d entries in one bucket; the composed bound is max(cap_direct, cap_relay)=%d", inGroup, max(capDir, capRelay))
		}
		if d := tab.kept(ids[:2]); d > capDir {
			t.Fatalf("G-3 RED (C-1): %d DIRECT entries of /24 0x24 in one bucket (cap_direct=%d)", d, capDir)
		}
	})
	t.Run("table-wide per-group cap", func(t *testing.T) {
		rng := rand.New(rand.NewSource(5))
		var self ports.NodeID
		rng.Read(self[:])
		o := oracle{}
		tab := newOnTable(self, o, AddressCapOn)
		tab.SetTableGroupCap(10)
		var spread []ports.NodeID
		for b := 255; b >= 241; b-- { // 15 buckets × 2 = 30 ids, all one /24
			spread = append(spread, idsInBucket(rng, self, b, 2)...)
		}
		for _, id := range spread {
			o.direct(id, 0x5A)
			tab.Observe(id)
		}
		if k := tab.kept(spread); k > 10 {
			t.Fatalf("G-3 RED: one /24 holds %d entries across the table (want ≤ 10, geth's bucketIPLimit-style table cap) — a single /24 can still own a large share of the whole table", k)
		}
		other := idInBucket(rng, self, 240)
		o.direct(other, 0x5B)
		tab.Observe(other)
		if !tab.has(other) {
			t.Fatalf("G-3 RED: a peer from a fresh /24 was refused after another /24 hit the table-wide cap — the table cap is per GROUP, not global")
		}
	})
}

// TestR43b_G4_ReserveBoundsAllNonDirectEntries — G-4 / C-2. Sybils spread behind M ≥ 8
// honest relays (free, cert §2.2) hold at most K − R slots per bucket; the remaining R
// slots admit DIRECT peers only; seeds / -persistent-peers are exempt from every cap;
// the reserve cannot be configured below K/2.
func TestR43b_G4_ReserveBoundsAllNonDirectEntries(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	var self ports.NodeID
	rng.Read(self[:])

	t.Run("relayed across 8 relays ≤ K−R; DIRECT still admitted", func(t *testing.T) {
		o := oracle{}
		tab := newOnTable(self, o, AddressCapOn)
		xs := idsInBucket(rng, self, 200, 16)
		for i, x := range xs {
			o.relayed(x, 0x900+uint64(i%8)) // 8 relay groups, 2 Sybils each: no per-relay cap fires
			tab.Observe(x)
		}
		if k := tab.kept(xs); k > r43bK-r43bReserve {
			t.Fatalf("G-4 RED (C-2): %d relayed entries in one bucket via 8 distinct relays (want ≤ K−R = %d) — honest relays' address diversity is borrowable for free", k, r43bK-r43bReserve)
		}
		ds := idsInBucket(rng, self, 200, 4)
		for i, d := range ds {
			o.direct(d, 0xD00+uint64(i))
			tab.Observe(d)
		}
		if k := tab.kept(ds); k != 4 {
			t.Fatalf("G-4 RED: %d of 4 DIRECT peers (distinct /24s) admitted into the reserve after the relayed pool filled its share; want 4", k)
		}
	})
	t.Run("the reserve counts relayed + unverified + unclassified together", func(t *testing.T) {
		o := oracle{}
		tab := newOnTable(self, o, AddressCapOn)
		introducer := idInBucket(rng, self, 255)
		o.direct(introducer, 0x11)
		tab.Observe(introducer)
		ids := idsInBucket(rng, self, 200, 8)
		for i, id := range ids {
			switch i % 3 {
			case 0:
				o.relayed(id, 0x900+uint64(i))
				tab.Observe(id)
			case 1:
				tab.ObserveIntroduced(id, introducer) // unverified
			default:
				tab.Observe(id) // unknown to the classifier: unclassified
			}
		}
		if k := tab.kept(ids); k > r43bK-r43bReserve {
			t.Fatalf("G-4 RED (C-2): %d non-DIRECT entries (relayed ∪ unverified ∪ unclassified) in one bucket; want ≤ K−R = %d — the reserve bounds only one of the three classes", k, r43bK-r43bReserve)
		}
	})
	t.Run("seeds and static peers are exempt from every cap", func(t *testing.T) {
		o := oracle{}
		tab := newOnTable(self, o, AddressCapOn)
		ids := idsInBucket(rng, self, 200, 6)
		for i, id := range ids[:4] { // the reserve is full
			o.relayed(id, 0x900+uint64(i))
			tab.Observe(id)
		}
		seed := ids[4]
		o.relayed(seed, 0x9FF) // a seed reached through a relay, into a full reserve
		tab.ObserveStatic(seed)
		if !tab.has(seed) {
			t.Fatalf("G-4 RED (C-2 exemption): a configured seed reached through a relay was refused because the reserve was full — seeds and -persistent-peers are operator-typed and exempt from every cap")
		}
		static := ids[5] // an unclassified static peer
		tab.ObserveStatic(static)
		if !tab.has(static) {
			t.Fatalf("G-4 RED (C-2 exemption): an unclassified -persistent-peers entry was refused")
		}
	})
	t.Run("R below K/2 is refused or clamped, never applied", func(t *testing.T) {
		o := oracle{}
		tab := NewTable(self, r43bK)
		applied := func() (panicked bool) {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
				}
			}()
			tab.SetAddressDiversity(o, r43bCapDir, r43bCapRelay, 2, AddressCapOn) // R=2 < K/2
			return false
		}()
		if applied {
			return // refused by panic: acceptable (a security parameter outside its certified range)
		}
		xs := idsInBucket(rng, self, 200, 8)
		for i, x := range xs {
			o.relayed(x, 0x900+uint64(i))
			tab.Observe(x)
		}
		if k := tab.kept(xs); k > r43bK/2 {
			t.Fatalf("G-4 RED: reserve=2 (< K/2) was APPLIED — %d non-DIRECT entries in one bucket (want ≤ K−K/2 = %d, the clamp) — the adversary owns a bucket MAJORITY for free via honest relays (cert §4)", k, r43bK/2)
		}
	})
}

// TestR43b_G5_DirectIsNeverDowngraded — G-5 / C-3. A peer classified DIRECT, later reached
// only through a relay into a SATURATED relayed class, keeps its DIRECT entry (the
// cert §2.1 oscillation: a public node re-dials a pony through the relay after the pony's
// outbound conn dies); a later DIRECT conversation at a new /24 re-keys it.
func TestR43b_G5_DirectIsNeverDowngraded(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var self ports.NodeID
	rng.Read(self[:])
	o := oracle{}
	tab := newOnTable(self, o, AddressCapOn)
	ids := idsInBucket(rng, self, 200, 5)
	for i, id := range ids[:4] { // reserve full: 2 relays × cap_relay
		o.relayed(id, 0x900+uint64(i%2))
		tab.Observe(id)
	}
	pony := ids[4]
	o.direct(pony, 0xA1)
	tab.Observe(pony)
	classOf(t, tab, pony, ports.ClassDirect, 0xA1, "G-5 fixture")

	o.relayed(pony, 0x900) // the transport now reaches it through relay 0x900 (saturated)
	tab.Observe(pony)
	if !tab.has(pony) {
		t.Fatalf("G-5 RED (C-3): a DIRECT peer later reached through a relay into a saturated relayed class was EVICTED — the re-check downgraded it and fired")
	}
	classOf(t, tab, pony, ports.ClassDirect, 0xA1, "G-5 RED (C-3: DIRECT downgraded to the relay's class)")

	o.direct(pony, 0xA2) // a later DIRECT conversation at a different /24
	tab.Observe(pony)
	classOf(t, tab, pony, ports.ClassDirect, 0xA2, "G-5 RED (a later DIRECT conn at a new /24 must re-key)")
}

// r43bEvent is one step of the G-6 random trace.
type r43bEvent struct {
	kind       string // observe | introduce | remove | static
	id, introd ports.NodeID
}

func (e r43bEvent) String() string { return fmt.Sprintf("%s %s", e.kind, e.id.String()[:8]) }

func r43bShadowCount(tab *Table, reserve, capRelay int) int {
	n := 0
	for k, v := range tab.ShadowRefusals() {
		if k.Reserve == reserve && k.CapRelay == capRelay {
			n += v
		}
	}
	return n
}

// TestR43b_G6_ShadowChangesNoAdmissionAndCountsEveryRefusal — G-6 (cert §6.2). Over random
// swarms the same event trace is fed to three tables (off, shadow, on) kept in lock-step:
// after every event shadow's buckets equal off's; whenever `on` diverges from off it does
// so by exactly the event's id and shadow recorded exactly one new would-refuse cell under
// the configured (R, cap_relay); when `on` does not diverge, shadow recorded nothing (or a
// bucket-full short-circuit). The refused id is then removed from off/shadow so the three
// stay synchronised and every later decision is compared on identical state. The grid
// subtest pins that shadow evaluates R ∈ {4,6,8} × cap_relay ∈ {2,4} (series A).
func TestR43b_G6_ShadowChangesNoAdmissionAndCountsEveryRefusal(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		rng := rand.New(rand.NewSource(100 + seed))
		var self ports.NodeID
		rng.Read(self[:])
		o := oracle{}
		mk := func(mode AddressMode) *Table {
			tab := newOnTable(self, o, mode)
			tab.SetTableGroupCap(10)
			return tab
		}
		off, shadow, on := mk(AddressCapOff), mk(AddressCapShadow), mk(AddressCapOn)

		var ids []ports.NodeID
		ids = append(ids, idsInBucket(rng, self, 200, 40)...)
		ids = append(ids, idsInBucket(rng, self, 150, 30)...)
		for b := 255; b >= 245; b-- {
			ids = append(ids, idsInBucket(rng, self, b, 3)...)
		}
		groups := []uint64{0x10, 0x11, 0x12}
		relays := []uint64{0x20, 0x21}

		for step := 0; step < 600; step++ {
			id := ids[rng.Intn(len(ids))]
			ev := r43bEvent{id: id}
			switch r := rng.Intn(100); {
			case r < 50:
				ev.kind = "observe"
				if _, known := o[id]; !known && rng.Intn(2) == 0 { // first contact classifies it
					if rng.Intn(3) == 0 {
						o.relayed(id, relays[rng.Intn(len(relays))])
					} else {
						o.direct(id, groups[rng.Intn(len(groups))])
					}
				}
			case r < 75:
				ev.kind = "introduce"
				var cands []ports.NodeID
				for cid, b := range off.snapshot() {
					if _, known := o[cid]; known && b != BucketIndex(self, id) {
						cands = append(cands, cid)
					}
				}
				if len(cands) == 0 {
					continue
				}
				sort.Slice(cands, func(i, j int) bool { return cands[i].String() < cands[j].String() })
				ev.introd = cands[rng.Intn(len(cands))]
			case r < 82:
				ev.kind = "remove"
			case r < 85:
				ev.kind = "static"
			default:
				ev.kind = "observe" // a repeat contact: move-to-back, or a re-key if the oracle changed
			}
			before := r43bShadowCount(shadow, r43bReserve, r43bCapRelay)
			for _, tab := range []*Table{off, shadow, on} {
				switch ev.kind {
				case "observe":
					tab.Observe(ev.id)
				case "introduce":
					tab.ObserveIntroduced(ev.id, ev.introd)
				case "remove":
					tab.Remove(ev.id)
				case "static":
					tab.ObserveStatic(ev.id)
				}
			}
			sOff, sSh, sOn := off.snapshot(), shadow.snapshot(), on.snapshot()
			if len(sOff) != len(sSh) {
				t.Fatalf("G-6 RED (seed %d, step %d, %s): shadow holds %d entries, off holds %d — shadow changed an admission", seed, step, ev, len(sSh), len(sOff))
			}
			for id, b := range sOff {
				if sSh[id] != b {
					t.Fatalf("G-6 RED (seed %d, step %d, %s): %s is in off's bucket %d but not shadow's — shadow changed an admission", seed, step, ev, id.String()[:8], b)
				}
			}
			var offOnly, onOnly []ports.NodeID
			for id := range sOff {
				if _, ok := sOn[id]; !ok {
					offOnly = append(offOnly, id)
				}
			}
			for id := range sOn {
				if _, ok := sOff[id]; !ok {
					onOnly = append(onOnly, id)
				}
			}
			delta := r43bShadowCount(shadow, r43bReserve, r43bCapRelay) - before
			if len(onOnly) != 0 {
				t.Fatalf("G-6 RED (seed %d, step %d, %s): on holds %d entries off does not — on admitted something off refused", seed, step, ev, len(onOnly))
			}
			switch len(offOnly) {
			case 0:
				if delta != 0 {
					// The only benign extra count: the bucket was already full, so `on` refused
					// on K and the address rule was still evaluated (recorded) — no divergence.
					full := 0
					for _, b := range sOff {
						if b == BucketIndex(self, ev.id) {
							full++
						}
					}
					if full < r43bK || delta != 1 {
						t.Fatalf("G-6 RED (seed %d, step %d, %s): shadow recorded %d would-refuse cell(s) but on made no refusal (bucket holds %d) — the counter over-reports", seed, step, ev, delta, full)
					}
				}
			case 1:
				if offOnly[0] != ev.id {
					t.Fatalf("G-6 RED (seed %d, step %d, %s): on diverged by %s, not by the event's id — a refusal touched a different entry", seed, step, ev, offOnly[0].String()[:8])
				}
				if delta != 1 {
					t.Fatalf("G-6 RED (seed %d, step %d, %s): on refused the event's id but shadow recorded %d new would-refuse cell(s) under (R=%d, cap_relay=%d); want exactly 1", seed, step, ev, delta, r43bReserve, r43bCapRelay)
				}
				off.Remove(ev.id) // resynchronise so the next decision is compared on identical state
				shadow.Remove(ev.id)
			default:
				t.Fatalf("G-6 RED (seed %d, step %d, %s): on diverged from off by %d entries in one step", seed, step, ev, len(offOnly))
			}
		}
		t.Logf("G-6 seed %d: %d would-refuse cells recorded under (R=%d, cap_relay=%d), %d entries tabled", seed, r43bShadowCount(shadow, r43bReserve, r43bCapRelay), r43bReserve, r43bCapRelay, off.Size())
		if r43bShadowCount(shadow, r43bReserve, r43bCapRelay) == 0 {
			t.Fatalf("G-6 fixture (seed %d): the trace produced no would-be refusal at all — the differential proved nothing; widen the trace", seed)
		}
	}

	t.Run("shadow evaluates the (R, cap_relay) grid", func(t *testing.T) {
		rng := rand.New(rand.NewSource(99))
		var self ports.NodeID
		rng.Read(self[:])
		o := oracle{}
		tab := newOnTable(self, o, AddressCapShadow)
		// bucket 200: 8 relayed behind 4 relays (2 each) — only the reserve bites.
		for i, id := range idsInBucket(rng, self, 200, 8) {
			o.relayed(id, 0x900+uint64(i%4))
			tab.Observe(id)
		}
		// bucket 150: 6 relayed behind ONE relay — per-relay cap and reserve both bite.
		for _, id := range idsInBucket(rng, self, 150, 6) {
			o.relayed(id, 0x9AA)
			tab.Observe(id)
		}
		want := map[ShadowKey]int{
			{200, ports.ClassRelayed, 4, 2}: 4, {200, ports.ClassRelayed, 4, 4}: 4,
			{200, ports.ClassRelayed, 6, 2}: 6, {200, ports.ClassRelayed, 6, 4}: 6,
			{200, ports.ClassRelayed, 8, 2}: 8, {200, ports.ClassRelayed, 8, 4}: 8,
			{150, ports.ClassRelayed, 4, 2}: 4, {150, ports.ClassRelayed, 4, 4}: 2,
			{150, ports.ClassRelayed, 6, 2}: 4, {150, ports.ClassRelayed, 6, 4}: 4,
			{150, ports.ClassRelayed, 8, 2}: 6, {150, ports.ClassRelayed, 8, 4}: 6,
		}
		got := tab.ShadowRefusals()
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("G-6 RED (series A grid): would-refuse[bucket %d, relayed, R=%d, cap_relay=%d] = %d, want %d — shadow does not evaluate the (R, cap_relay) grid the reserve must be set from (cert §6.3)", k.Bucket, k.Reserve, k.CapRelay, got[k], v)
			}
		}
		if tab.Size() != 14 {
			t.Fatalf("G-6 RED: shadow admitted %d of 14 (want all 14: shadow never refuses)", tab.Size())
		}
	})
}

// TestR43b_G7_RecheckIsANarrowingAndClassLossIsInert — G-7 / C-4. An UNVERIFIED entry
// whose first classification lands in a saturated group is removed; into a non-saturated
// group it stays, re-keyed; a re-check never adds; and CLASS LOSS (the classifier no
// longer knows a tabled id) changes nothing — never a re-check, never an eviction, even
// with the reserve full (the red-team Finding-C flood must not become an eviction weapon).
func TestR43b_G7_RecheckIsANarrowingAndClassLossIsInert(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	var self ports.NodeID
	rng.Read(self[:])
	o := oracle{}
	tab := newOnTable(self, o, AddressCapOn)
	ids := idsInBucket(rng, self, 200, 8)
	for _, id := range ids[:2] { // group 0xS is saturated (cap_direct)
		o.direct(id, 0x5)
		tab.Observe(id)
	}
	introducer := idInBucket(rng, self, 255)
	o.direct(introducer, 0x11)
	tab.Observe(introducer)
	x1, x2 := ids[2], ids[3]
	tab.ObserveIntroduced(x1, introducer)
	tab.ObserveIntroduced(x2, introducer)
	if !tab.has(x1) || !tab.has(x2) {
		t.Fatalf("G-7 fixture: the two introduced ids were not admitted (introducer group 0x11 has room)")
	}

	o.direct(x1, 0x5) // first classification: its true group is saturated
	tab.Observe(x1)
	if tab.has(x1) {
		t.Fatalf("G-7 RED: an UNVERIFIED entry whose first classification lands in a SATURATED group (0x5 holds cap_direct) stayed tabled — the re-check is not applied (the admission rule applied late)")
	}
	size := tab.Size()
	o.direct(x2, 0x6) // non-saturated
	tab.Observe(x2)
	if !tab.has(x2) {
		t.Fatalf("G-7 RED: an UNVERIFIED entry re-checked into a NON-saturated group was removed — the re-check is wider than a narrowing")
	}
	classOf(t, tab, x2, ports.ClassDirect, 0x6, "G-7 RED (re-key)")
	if tab.Size() != size {
		t.Fatalf("G-7 RED: table size %d → %d across a re-check; a re-check never adds", size, tab.Size())
	}

	// Class loss with the reserve full: an unclassified x2 would overflow K−R; it must stay DIRECT.
	for i, id := range ids[4:8] {
		o.relayed(id, 0x900+uint64(i%2))
		tab.Observe(id)
	}
	o.unknown(x2)
	tab.Observe(x2)
	if !tab.has(x2) {
		t.Fatalf("G-7 RED (C-4): a tabled DIRECT entry was EVICTED when the classifier lost its class — class loss fired the re-check (Finding-C flood ⇒ eviction weapon)")
	}
	classOf(t, tab, x2, ports.ClassDirect, 0x6, "G-7 RED (C-4: class loss must be inert; the class lives with the entry)")
}

// TestR43b_G8_GroupZeroIsExemptFromTheGroupCap — G-8, table half. Loopback / link-local
// conversations classify to group 0 (exempt): an on-host swarm's table fills to K exactly
// as before. (The RFC1918-is-CLASSIFIED half is adapters/tcpnet/r43b_class_test.go.)
func TestR43b_G8_GroupZeroIsExemptFromTheGroupCap(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	var self ports.NodeID
	rng.Read(self[:])
	o := oracle{}
	tab := newOnTable(self, o, AddressCapOn)
	ids := idsInBucket(rng, self, 200, 8)
	for _, id := range ids {
		o.direct(id, 0) // loopback: exempt group
		tab.Observe(id)
	}
	if k := tab.kept(ids); k != r43bK {
		t.Fatalf("G-8 RED: %d of 8 loopback (group 0) peers admitted under on; want 8 — an on-host e2e swarm's tables shrink", k)
	}
}

// TestR43b_TelemetrySeriesBAndEFromTheTable — the per-node halves of series B (relay
// fan-in) and E (per-bucket group-density census), read straight from the entries'
// stored classes (C-4). Series A is ShadowRefusals (G-6).
func TestR43b_TelemetrySeriesBAndEFromTheTable(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	var self ports.NodeID
	rng.Read(self[:])
	o := oracle{}
	tab := newOnTable(self, o, AddressCapOn)
	b200 := idsInBucket(rng, self, 200, 6)
	o.direct(b200[0], 0xA)
	o.direct(b200[1], 0xA)
	o.direct(b200[2], 0xB)
	o.relayed(b200[3], 0x91)
	o.relayed(b200[4], 0x91)
	o.relayed(b200[5], 0x92)
	for _, id := range b200 {
		tab.Observe(id)
	}
	b150 := idsInBucket(rng, self, 150, 2)
	for _, id := range b150 {
		o.relayed(id, 0x91)
		tab.Observe(id)
	}
	if tab.Size() != 8 {
		t.Fatalf("fixture: %d tabled, want 8", tab.Size())
	}
	clients, top := tab.RelayFanIn()
	if clients[0x91] != 4 || clients[0x92] != 1 || top < 0.79 || top > 0.81 {
		t.Fatalf("series B RED: RelayFanIn = (%v, top %.2f); want relay 0x91→4 clients, 0x92→1, top share 0.80", clients, top)
	}
	rows := map[int]BucketCensus{}
	for _, r := range tab.GroupCensus() {
		rows[r.Bucket] = r
	}
	if r := rows[200]; r.Entries != 6 || r.Groups != 4 || r.GroupsAtCap != 2 || r.NonDirect != 3 {
		t.Fatalf("series E RED: census[200] = %+v; want Entries 6, Groups 4 (A,B,91,92), GroupsAtCap 2 (A at cap_direct, 91 at cap_relay), NonDirect 3", r)
	}
	if r := rows[150]; r.Entries != 2 || r.Groups != 1 || r.GroupsAtCap != 1 || r.NonDirect != 2 {
		t.Fatalf("series E RED: census[150] = %+v; want Entries 2, Groups 1, GroupsAtCap 1, NonDirect 2", r)
	}
}

// TestR43b_G5_DirectIsNeverDowngradedIntoAnUnsaturatedRelayClass — G-5 / C-3, the case
// the saturated fixture above does not reach (Builder, 2026-09-04): a re-key of a
// contacted entry is applied only when the new key is admissible, so with the relay
// class SATURATED that rule alone keeps the pony DIRECT even without C-3. C-3 is "never
// downgraded", full stop: reached later through an UNSATURATED relay, the DIRECT entry
// keeps its class too — otherwise an honest pony slides into the reserve, which fills,
// and the re-check evicts it (cert §2.1 oscillation).
func TestR43b_G5_DirectIsNeverDowngradedIntoAnUnsaturatedRelayClass(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	var self ports.NodeID
	rng.Read(self[:])
	o := oracle{}
	tab := newOnTable(self, o, AddressCapOn)
	pony := idInBucket(rng, self, 200)
	o.direct(pony, 0xA1)
	tab.Observe(pony)
	o.relayed(pony, 0x900) // an empty relay class: the re-key would be admissible
	tab.Observe(pony)
	classOf(t, tab, pony, ports.ClassDirect, 0xA1, "G-5 RED (C-3: DIRECT downgraded into an unsaturated relay class)")
}
