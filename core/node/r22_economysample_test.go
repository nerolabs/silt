package node

// R2.2 (Lane C3) — the two gossip fields and the two work Ginis.
//
// The design doc named two honesty gaps this closes (§3): per-node repairs-done had no
// gossip carrier, and peer gossip carried capacity bytes only. Rows 8-9 add EXACTLY TWO
// fields; row 10 forbids a third and derives the tier class from the capacity total that
// was already on the wire.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

func r22Node(t *testing.T) *Node {
	t.Helper()
	var id ports.NodeID
	id[0] = 0x22
	sched := simclock.New()
	net := simnet.New(sched, 3, simnet.DefaultConfig())
	return New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())
}

// gossip delivers one message carrying the four self-reported figures a peer advertises.
func gossip(n *Node, i int, capTotal, served, repairs int64) {
	var from ports.NodeID
	from[0], from[1], from[2], from[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
	n.handle(from, ports.Message{Kind: ports.MsgFindNode, CapUsed: 1, CapTotal: capTotal,
		ServedBytes: served, RepairsDone: repairs})
}

// TestR22WorkGossipLandsAndRidesTheExistingBound: rows 8-9 must actually populate the
// sample, and they must add NO new peer-keyed state — they ride peerCaps, under
// evictPeerInfoIfFull, so the resident cost stays f(maxPeerInfo).
//
// ABLATION: drop `served: msg.ServedBytes, repairs: msg.RepairsDone` from handle's
// capInfo literal and the first arm reddens with zeros; the bound arm is the existing
// peerinfo_bound_test property restated on the new fields.
func TestR22WorkGossipLandsAndRidesTheExistingBound(t *testing.T) {
	n := r22Node(t)
	gossip(n, 7, 32<<30, 4096, 9)
	var from ports.NodeID
	from[0] = 7
	c, ok := n.peerCaps[from]
	if !ok {
		t.Fatal("the gossiping peer is not in peerCaps at all")
	}
	if c.served != 4096 || c.repairs != 9 {
		t.Fatalf("peerCaps entry = served %d, repairs %d; want 4096 and 9. Without these two the repair-work Gini has no input at all and the panel is empty by construction (design doc §3 gap 1)", c.served, c.repairs)
	}

	// The bound: a flood of distinct senders, every one carrying work figures.
	flood := r22Node(t)
	for i := 0; i < maxPeerInfo*2; i++ {
		gossip(flood, i, 1<<20, int64(i), int64(i))
	}
	if got := len(flood.peerCaps); got > maxPeerInfo {
		t.Fatalf("peerCaps grew to %d under a %d-sender flood — rows 8-9 must ride the EXISTING bound, not open a new unbounded map (build-immutable #8)", got, maxPeerInfo*2)
	}
}

// TestR22GossipStampNeedsTheCapacityPledge pins the coupling row 10 depends on: the work
// figures are filed only alongside a positive CapTotal, because the tier band that
// classifies the sample is DERIVED from CapTotal. A work figure with no pledge could not
// be classified, so it is not sampled at all — and the sample is therefore exactly the
// capacity sample, which is what makes every member classifiable.
func TestR22GossipStampNeedsTheCapacityPledge(t *testing.T) {
	n := r22Node(t)
	var from ports.NodeID
	from[0] = 0x51
	n.handle(from, ports.Message{Kind: ports.MsgFindNode, ServedBytes: 999, RepairsDone: 9}) // no CapTotal
	if _, ok := n.peerCaps[from]; ok {
		t.Fatal("a peer with no capacity pledge entered the sample; capacityTier cannot classify it, so it would sit in the Gini and outside the mix")
	}
	if s := n.EconomySample(); s.Size != 0 {
		t.Fatalf("EconomySample.Size = %d over an unclassifiable peer, want 0", s.Size)
	}
}

// TestR22TierBandsCutWhereThePublishedTableCuts. The bands are a PUBLISHED number, so
// the edges are pinned: 16 GiB (the horse's stated disk floor) and 1 TiB (the archival's
// stated "TBs"), both exclusive upper edges of the band below.
func TestR22TierBandsCutWhereThePublishedTableCuts(t *testing.T) {
	for _, tc := range []struct {
		bytes int64
		want  string
	}{
		{0, ""}, {-1, ""},
		{1, TierPony},
		{TierPonyMaxBytes - 1, TierPony},
		{TierPonyMaxBytes, TierHorse},
		{TierHorseMaxBytes - 1, TierHorse},
		{TierHorseMaxBytes, TierArchival},
		{50 << 40, TierArchival},
	} {
		if got := capacityTier(tc.bytes); got != tc.want {
			t.Fatalf("capacityTier(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
	if RepairCapable(TierPony) {
		t.Fatal("the pony is repair-CAPABLE in this build. D-TIERING coupling (b) is that durability is guaranteed by the persistent tiers, never the transient edge — and if the pony is in the repair sample the repair Gini goes back to ~0.99 by construction")
	}
	if !RepairCapable(TierHorse) || !RepairCapable(TierArchival) {
		t.Fatal("the persistent tiers are not repair-capable — the repair sample would be empty")
	}
}

// TestR22RepairGiniIsScopedToTheCapableSubset is the CORRECTION the Economist advisory
// §2.1 forced on this track's own design doc, encoded.
//
// THE MEASUREMENT. Build the vision shape: many ponies that serve and do no durability
// work, a handful of persistent nodes that do all of it, spread EVENLY among themselves.
// That is a HEALTHY network. Over the whole sample its repair Gini is near 1 — because
// most nodes are structurally at zero, not because anything concentrated. Over the
// capable subset it is near 0, which is the truth. A threshold on the network-wide
// series therefore cannot separate health from capture, and this test measures both
// numbers so the claim is not taken on faith.
func TestR22RepairGiniIsScopedToTheCapableSubset(t *testing.T) {
	n := r22Node(t)
	const ponies, horses = 60, 6
	var wholeSample []int64
	for i := 0; i < ponies; i++ {
		gossip(n, i, 4<<30, 1_000_000, 0) // pony: serves, never repairs
		wholeSample = append(wholeSample, 0)
	}
	for i := 0; i < horses; i++ {
		gossip(n, 1000+i, 64<<30, 1_000_000, 10) // horse: serves the same, repairs evenly
		wholeSample = append(wholeSample, 10)
	}

	s := n.EconomySample()
	if s.Size != ponies+horses {
		t.Fatalf("sample size %d, want %d", s.Size, ponies+horses)
	}
	if s.RepairSampleSize != horses {
		t.Fatalf("repair sample = %d, want %d (the capable subset only)", s.RepairSampleSize, horses)
	}
	networkWide := credit.Gini(wholeSample)

	// The advisory's threshold for the healthy baseline is 0.40 within the capable
	// subset. Perfectly even repair inside the subset must sit far below it...
	if s.RepairGini > 0.40 {
		t.Fatalf("repairGini within the capable subset = %.4f on a PERFECTLY EVEN repair distribution — the healthy baseline must clear the advisory's 0.40 threshold or the gate is unusable", s.RepairGini)
	}
	// ...and the network-wide series must sit ABOVE it on that same healthy network,
	// which is the whole reason the design doc's gate was vacuous.
	if networkWide <= 0.40 {
		t.Fatalf("the network-wide repair Gini is %.4f on the vision shape; this test claims it is high BY CONSTRUCTION and the claim just failed — re-derive before trusting the scope decision", networkWide)
	}
	t.Logf("healthy vision shape: repairGini(capable subset, n=%d) = %.4f · repairGini(whole sample, n=%d) = %.4f. The network-wide number reads as near-total capture on a network where repair is spread perfectly evenly",
		s.RepairSampleSize, s.RepairGini, len(wholeSample), networkWide)

	// And the series still has teeth where it is scoped: route every repair to one
	// capable node and the SUBSET Gini must cross the threshold.
	c := r22Node(t)
	for i := 0; i < ponies; i++ {
		gossip(c, i, 4<<30, 1_000_000, 0)
	}
	gossip(c, 2000, 64<<30, 1_000_000, 60)
	for i := 1; i < horses; i++ {
		gossip(c, 2000+i, 64<<30, 1_000_000, 0)
	}
	if cs := c.EconomySample(); cs.RepairGini <= 0.40 {
		t.Fatalf("all repair on ONE of %d capable nodes gives repairGini %.4f — the scoped series does not redden on total capture, so it is decoration", horses, cs.RepairGini)
	}
}

// TestR22ServeGiniCoversTheWholeSampleAndSeparatesFromRepair. Serve-work is NOT scoped:
// every tier serves, and the vision is that the edge does the majority of it. The two
// series must be able to disagree — that disagreement (serving federated, repair
// concentrating) is the drift a single balance Gini cannot see.
func TestR22ServeGiniCoversTheWholeSampleAndSeparatesFromRepair(t *testing.T) {
	n := r22Node(t)
	for i := 0; i < 60; i++ {
		gossip(n, i, 4<<30, 1_000_000, 0) // ponies serve evenly
	}
	for i := 0; i < 6; i++ {
		gossip(n, 1000+i, 64<<30, 1_000_000, 0)
	}
	gossip(n, 2000, 64<<30, 1_000_000, 500) // one capable node does every repair
	s := n.EconomySample()
	if s.ServeGini > 0.15 {
		t.Fatalf("serveGini = %.4f over a perfectly even serve distribution, want <= 0.15 (the advisory's federated baseline)", s.ServeGini)
	}
	if s.RepairGini <= 0.40 {
		t.Fatalf("repairGini = %.4f while ONE capable node does every repair — the two series are not separating, which is exactly what a balance Gini already fails to do", s.RepairGini)
	}
	t.Logf("serveGini %.4f (federated) alongside repairGini %.4f (captured), sample %d / repair-capable %d", s.ServeGini, s.RepairGini, s.Size, s.RepairSampleSize)
}

// TestR22SelfWorkIsZeroWithoutAWorkReportingLedger: a node whose ledger does not expose
// the non-registering reader gossips zeros rather than panicking or guessing. Also pins
// that SetLedger is where the reader is resolved — the ablation is deleting that line.
func TestR22SelfWorkIsZeroWithoutAWorkReportingLedger(t *testing.T) {
	n := r22Node(t)
	if s, r := n.selfWork(); s != 0 || r != 0 {
		t.Fatalf("selfWork with no ledger = (%d, %d), want zeros", s, r)
	}
	led := credit.New(0, 5_000)
	n.SetLedger(led)
	if n.workRep == nil {
		t.Fatal("SetLedger did not resolve the work reader: every outbound message would gossip zero served bytes and the whole serve-Gini sample would read as a network that has served nothing")
	}
	led.RecordServe(n.id, ports.NodeID{0xEE}, ports.ChunkID{0x1}, 8192)
	if s, _ := n.selfWork(); s != 8192 {
		t.Fatalf("selfWork served = %d, want 8192", s)
	}
}
