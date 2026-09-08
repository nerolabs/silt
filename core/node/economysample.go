package node

// The gossip-estimated half of the economy-observability surface (Boulder 2, R2.2
// rows 6-11, Economist advisory ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-
// RC-scope-2026-09-07 §2). Every number here is an ESTIMATE over this node's local
// peer sample; none of it is committed, none of it is a consensus, standing or
// disbursement input, and none of it renders without its sample size.
//
// WHAT THE SAMPLE IS. Exactly the peers in peerCaps: the nodes that have gossiped a
// positive CapTotal (node.go, handle). That is not an accident of implementation, it
// is what makes the tier classification possible without a third gossip field — every
// sampled peer HAS a CapTotal by construction, so capacityTier can classify all of
// them. The sample is bounded by maxPeerInfo and evicted by evictPeerInfoIfFull, so a
// Sybil flood cannot grow it (peerinfo_bound_test.go).
//
// SELF IS IN THE SAMPLE when this node pledges capacity, exactly as EstimateNetwork
// includes it. Its three values are local-exact inside a gossip-estimated aggregate;
// SelfIncluded says so on the wire so nobody has to guess.

import (
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// The tier-class bands (R2.2 row 10). A peer's class is DERIVED from the CapTotal it
// already gossips — the advisory forbids a third gossip field, and rightly: all three
// numbers are self-reported, so a self-declared label adds attack surface and buys
// nothing a band over the pledged bytes does not already give.
//
// PROVENANCE OF THE TWO EDGES. They are read off the published tier table in the
// Economist's sustainability audit (silt-reviews/economist/2026-09-01-tiered-edge-
// economy-sustainability-audit.md, the "Pony / Horse / Archival" table): the horse is
// specified as "6-8 CPU / 4 GB / 16+ GB disk" and the archival as "24+ CPU / GPU /
// TBs". So the pony|horse edge is the horse's stated disk floor and the horse|archival
// edge is the archival's stated order of magnitude. The binary units are the code's:
// CapTotal is counted in bytes, so 16 GiB and 1 TiB are the nearest binary magnitudes
// to the table's "16 GB" and "TBs" and both sit at or above the prose figure.
//
// WHAT THIS IS NOT. Not a security parameter, not a standing input, not a role: a node
// declares its actual role with -serve-content/-validator/-archive and this never reads
// those. It is a presentation band over a self-reported number, used only to shape the
// mix histogram and to pick the repair-CAPABLE subset for the repair-work Gini. A node
// can lie its way into any band and gain nothing but a wrong dashboard.
const (
	// TierPonyMaxBytes is the exclusive upper edge of the pony band.
	TierPonyMaxBytes = int64(16) << 30 // 16 GiB — the horse's stated disk floor
	// TierHorseMaxBytes is the exclusive upper edge of the horse band.
	TierHorseMaxBytes = int64(1) << 40 // 1 TiB — the archival's stated "TBs"
)

// Tier class names, as they appear on the wire.
const (
	TierPony     = "pony"
	TierHorse    = "horse"
	TierArchival = "archival"
)

// capacityTier classifies a self-reported pledged-capacity total into one of the three
// published bands. A non-positive total is not classifiable and yields "" — such a peer
// is never in peerCaps anyway (handle gates on CapTotal > 0), so this is defence in
// depth, not a live case.
func capacityTier(capTotal int64) string {
	switch {
	case capTotal <= 0:
		return ""
	case capTotal < TierPonyMaxBytes:
		return TierPony
	case capTotal < TierHorseMaxBytes:
		return TierHorse
	default:
		return TierArchival
	}
}

// repairCapable reports whether a tier class is one the durability economy expects to
// do repair work. D-TIERING's coupling (b): durability is guaranteed by the PERSISTENT
// tiers, never by the transient edge. So the repair-work Gini is computed within the
// horse+archival subset only — see EconomySample.RepairGini for why the network-wide
// version is worthless.
func repairCapable(tier string) bool { return tier == TierHorse || tier == TierArchival }

// EconomySample is one node's gossip-estimated view of the reachable crowd's ECONOMY:
// the tier mix (rows 10-11) and the two work Ginis (rows 6-7), each with the sample
// size that produced it (row 13). Every consumer must check Size against its own floor
// before rendering: a Gini over two values inverts to the ratio of those two values.
type EconomySample struct {
	// Size is how many nodes the aggregates below are over — peers heard from, plus
	// self when SelfIncluded. THE REQUIRED SIBLING of every number in this struct.
	Size int
	// SelfIncluded is true when this node's own three values are in the sample.
	SelfIncluded bool
	// EstimatedNodes is the DHT crowd estimate (dht.EstimateNetworkSize) the mix is
	// extrapolated against. Same estimator NetEstimate already publishes.
	EstimatedNodes float64
	// Mix counts the sample by tier class, keyed by TierPony/TierHorse/TierArchival.
	// A class with no members is absent from the map, never a zero: "no archival node
	// in my sample" and "zero archival nodes exist" are different facts.
	Mix map[string]int
	// ServeGini is Gini over the sampled served-byte totals — the whole sample, because
	// every tier serves. RepairGini is Gini over the sampled repairs-done totals WITHIN
	// THE REPAIR-CAPABLE SUBSET ONLY, and RepairSampleSize is that subset's size.
	//
	// WHY THE SUBSET (advisory §2.1, and it is a correction to this track's own design
	// doc). Under D-TIERING transient ponies serve and relay but do not do durability
	// work, so at the vision ratio the NETWORK-WIDE repair Gini is 0.9902 with repair
	// spread perfectly evenly over every horse and archival node, and 0.9999 when one
	// archival node does all of it. A threshold below 0.99 fails on a healthy network
	// and one above it passes on total capture: the network-wide series carries no
	// signal at all. Within the capable subset it does.
	ServeGini        float64
	RepairGini       float64
	RepairSampleSize int
	// ServeWorkTotal and RepairWorkTotal are the SUMS the two Ginis were taken over, and
	// they are the only thing that tells a measured equality from no measurement at all:
	// credit.Gini returns 0 both when every value is identical and when the values sum to
	// zero ("universal poverty is technically equality", core/credit/credit.go). A sample
	// in which nobody has reported any work is the second case, and it is live on a fresh
	// network, on a node whose ledger does not implement workReporter, and on the mixed
	// case where "cannot see my counters" is summed with "served nothing". The consumer
	// renders that as a named absence, never as 0 (blind PE ruling B2).
	ServeWorkTotal  int64
	RepairWorkTotal int64
}

// EconomySample computes this node's gossip-estimated view of the crowd's economy.
// Loop-owned (it reads peerCaps and the ledger); call it on the event loop, as
// EstimateNetwork is called. Reading moves nothing.
func (n *Node) EconomySample() EconomySample {
	es := EconomySample{Mix: map[string]int{}}
	served := make([]int64, 0, len(n.peerCaps)+1)
	repairs := make([]int64, 0, len(n.peerCaps)+1)
	add := func(capTotal, srv, rep int64) {
		tier := capacityTier(capTotal)
		if tier == "" {
			return
		}
		es.Size++
		es.Mix[tier]++
		served = append(served, srv)
		if repairCapable(tier) {
			repairs = append(repairs, rep)
		}
	}
	for _, c := range n.peerCaps {
		add(c.total, c.served, c.repairs)
	}
	if n.capRep != nil {
		_, total := n.capRep.Capacity()
		srv, rep := n.selfWork()
		if capacityTier(total) != "" {
			es.SelfIncluded = true
			add(total, srv, rep)
		}
	}
	// The SAME estimator NetEstimate publishes, called through the same function, so
	// the two surfaces cannot drift onto different crowd estimates.
	es.EstimatedNodes = n.EstimateNetwork().EstimatedNodes
	es.ServeGini = credit.Gini(served)
	es.RepairGini = credit.Gini(repairs)
	es.RepairSampleSize = len(repairs)
	for _, v := range served {
		es.ServeWorkTotal += v
	}
	for _, v := range repairs {
		es.RepairWorkTotal += v
	}
	return es
}

// selfWork reads THIS node's two gossiped work counters off its own ledger. It uses
// credit.Ledger.WorkSample through an optional interface — the NON-registering read —
// because this runs on the outbound-message path and a read that registers an account
// would move faucet accounting as a side effect of sending a FindNode (see
// credit.Ledger.WorkSample). Zero when no ledger is wired or this node has no account
// yet, which is the honest reading: a node that has never been credited has served no
// bytes and done no repairs.
func (n *Node) selfWork() (served, repairs int64) {
	if n.workRep == nil {
		return 0, 0
	}
	s, r, _ := n.workRep.WorkSample(n.id)
	return s, r
}

// workReporter is the optional ledger interface the gossip stamp reads through.
type workReporter interface {
	WorkSample(ports.NodeID) (int64, int64, bool)
}
