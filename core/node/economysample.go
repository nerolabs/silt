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

// RepairCapable reports whether a tier class is one the durability economy expects to
// do repair work. D-TIERING's coupling (b): durability is guaranteed by the PERSISTENT
// tiers, never by the transient edge. So the repair-work Gini is computed within the
// horse+archival subset only — see EconomySample.RepairGini for why the network-wide
// version is worthless.
//
// EXPORTED because cmd/silt must decide the same question about the same classes when it
// renders a per-tier repair share: the repair series' denominator is the capable subset,
// so a share published for a class OUTSIDE that subset would be a ratio over a population
// it is not a member of. The renderer asks this predicate rather than re-listing the two
// class names. TestR22PerTierTotalsFollowTheExclusionRuleAndNameTheirAbsences pins that
// RepairsByTier is keyed on exactly it, and
// TestGateC3_3e_CapableSizeClosesTheCrossDocumentRepairCoverageJoin drives the same rule
// through the rendered wire.
func RepairCapable(tier string) bool { return tier == TierHorse || tier == TierArchival }

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
	// ServeGini is Gini over the served-byte totals of the peers that REPORTED one, and
	// ServeSampleSize is how many that is. RepairGini is the same over the repairs-done
	// totals of the REPAIR-CAPABLE peers that reported one, and RepairSampleSize is that
	// subset's size. Each series carries its OWN size because each is over its own
	// population, and a size is only a useful sibling to the number it sized.
	//
	// WHY A NON-REPORTING PEER IS EXCLUDED RATHER THAN COUNTED AS ZERO (research
	// certification C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-2026-09-09, condition M-2).
	// Both wire fields are `omitempty`, so a peer WITHHOLDING its counters and a peer that
	// has done nothing are THE SAME BYTES, and under the certified gossip gate withholding
	// is the DEFAULT posture. Counting those absences as zeros would fill the series with
	// zeros and drive the serve Gini toward 1.0 — a FALSE READING OF TOTAL CAPTURE on the
	// shipped default, on a network where nothing is captured at all.
	//
	// THE DISCRIMINATOR IS THE PAIR, NOT THE FIELD, and that is what keeps the alarm alive.
	// A peer that serves but has never repaired emits key 29 and not key 30; a withholding
	// peer emits neither. So any peer with a positive term is a REPORTING peer and its zero
	// in the other field is a genuine measured zero that COUNTS — which is why "repair
	// concentrated on one of six capable nodes" still reddens the repair Gini instead of
	// collapsing to a one-element series. Only the all-zero peer is truly ambiguous, and it
	// is excluded. Membership is all-or-nothing: a peer cannot be in one series and out of
	// the other, because escaping either requires reporting zero in both.
	//
	// SHOULD IT BE PER-SERIES INSTEAD? No, and the blind PE re-ruling at 81d39c0 measured
	// the case for asking: a peer reporting ONLY repairs is admitted as reporting and lands
	// a zero in the SERVE series, so 20 free identities moved a perfectly even network's
	// serveGini from 0.0000 to 0.8696. Per-series membership — drop any peer with a zero in
	// THAT series — would close that particular lever, and it would cost the repair alarm
	// outright: repair concentrated on one of six capable nodes becomes a one-element
	// series, falls under the floor and renders "sample too small", so the capture case
	// reads as no-data. That is the worse failure. And it buys nothing real, because a
	// sybil does not need the repairs-only trick — it can declare any positive servedBytes
	// and steer the same number just as freely. The lever is not the discriminator, it is
	// that every term is self-reported. Which is why both figures are disclosed as
	// Sybil-settable IN EITHER DIRECTION on the wire, and may never become an input to
	// anything (research certification §3, standing constraint).
	//
	// WHY THE SUBSET (advisory §2.1, and it is a correction to this track's own design
	// doc). Under D-TIERING transient ponies serve and relay but do not do durability
	// work, so at the vision ratio the NETWORK-WIDE repair Gini is 0.9902 with repair
	// spread perfectly evenly over every horse and archival node, and 0.9999 when one
	// archival node does all of it. A threshold below 0.99 fails on a healthy network
	// and one above it passes on total capture: the network-wide series carries no
	// signal at all. Within the capable subset it does.
	ServeGini        float64
	ServeSampleSize  int
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

	// ---- the per-tier work totals (Economist ADVISORY-c3-concentration-gate-thresholds-
	// redderived-2026-09-09 §3a, Builder item 1) --------------------------------------
	//
	// WHY THEY EXIST. T-AR is a TIER-SHARE statement -- "the edge tier that does the
	// MAJORITY of the work must remain a net-positive place to do it" (docs/TENETS.md
	// Part IX). Nothing above carries a per-tier quantity, so the tenet had no source: the
	// two Ginis are per-NODE dispersion statistics, and on a network whose ratified tier
	// design spans a 500:1 capacity dispersion an elevated Gini is not a violation of
	// anything silt ratified. Measured on one fixture, the two statistics disagree in
	// opposite directions -- the serve Gini reads CONCENTRATED at G_adj 0.1726 while the
	// pony tier serves 0.8184 of the bytes, far above the tenet floor. Only one of them is
	// the tenet, and it is the tier share.
	//
	// THE SUBSTITUTION TRAP THESE CLOSE, and it is measured, not feared. economyNetwork's
	// mix[].share LOOKS like the missing number and is a share of NODE COUNT: under the
	// ratified vision ratio the pony share of nodes is ~0.99 BY CONSTRUCTION, so a gate
	// reading it for the T-AR floor passes on every distribution. Measured 0.9891 where the
	// true pony share of served bytes was 0.1998 (cmd/silt,
	// TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes).
	//
	// NO NEW GOSSIP FIELD. The tier class is DERIVED from the CapTotal every sampled peer
	// already gossips, through the same capacityTier that shapes Mix. R2.2 row 10 forbids a
	// third gossip field and these totals do not need one.
	//
	// THE SAME M-2 EXCLUSION RULE AS THE TWO SERIES, and it is what makes the shares
	// legible rather than a second copy of the hole. Only REPORTING peers -- a peer with a
	// positive term in either counter -- contribute to any of these maps. A peer that
	// reported nothing is out of both the numerator and the denominator, exactly as it is
	// out of the Ginis. That is why ReportersByTier ships beside them: it is the per-tier
	// coverage, the denominator's population, and the discriminator a consumer branches on.
	//
	// ABSENT IS NOT ZERO, and here the distinction is load-bearing twice over. A tier with
	// NO reporting peer is ABSENT from every map below -- never a 0 entry -- because "no
	// pony in my sample reported" and "the ponies in my sample served nothing" are
	// different facts and both render as 0.0. A tier WITH reporting peers that summed to
	// zero IS present with a 0 value, and that zero is a measurement. ReportersByTier is
	// the only field that tells the two apart, so a consumer that reads a byte total
	// without reading it beside ReportersByTier is reading an absence as a measurement.
	//
	// SHARES ON THE WIRE, NOT THESE TOTALS (advisory §3a). A per-tier absolute total plus
	// n-1 sybil-supplied terms recovers the n-th in one subtraction -- strictly easier than
	// the Gini inversion the reconstruction gate already closes. The cmd/silt renderer
	// publishes ratios only, and behind the SAME gossipWithheld marker as the Ginis,
	// because this is the same covered set one derivation removed.

	// ServeBytesByTier is the sum of reported servedBytes, keyed by tier class, over the
	// REPORTING peers of that tier. Absent for a tier with no reporting peer.
	ServeBytesByTier map[string]int64
	// RepairsByTier is the sum of reported repairsDone over the REPORTING peers of that
	// tier, keyed only for the repair-CAPABLE classes. A pony's repairs are outside the
	// repair series' population by construction (RepairCapable, D-TIERING coupling (b)),
	// so keying them here would publish a share whose denominator is not the population
	// the number claims to describe. Absent for a capable tier with no reporting peer, and
	// absent for the pony class always.
	RepairsByTier map[string]int64
	// PledgedBytesByTier is the sum of CapTotal over the REPORTING peers of that tier: the
	// NULL's denominator, which is what makes an observed share comparable to an expected
	// one without an assumed weighting table. It is over the reporting peers and not the
	// whole sample on purpose -- an expectation computed over a different population from
	// the observation is not a null, it is two numbers.
	PledgedBytesByTier map[string]int64
	// ReportersByTier is how many peers of that tier are in the work series at all. THE
	// REQUIRED SIBLING of the three totals above, for the same reason Size is the required
	// sibling of a Gini: a tier's share is a figure over that tier's reporters, and a share
	// whose coverage a reader has to guess is unreadable.
	ReportersByTier map[string]int
	// CapableSize is how many CLASSIFIABLE peers of this sample are repair-capable --
	// Mix[horse] + Mix[archival], reporting or not. It is the repair series' POPULATION,
	// and it ships here so a consumer can compute that series' reporting coverage from ONE
	// snapshot. Before it existed the only source was the tier mix on a sibling route, so
	// the coverage ratio was a join across two independent EconomySample() calls: in a
	// fixture they agree, on a live node peerCaps moves between them and the ratio can
	// exceed 1 (advisory §1, Builder item 3).
	CapableSize int
}

// EconomySample computes this node's gossip-estimated view of the crowd's economy.
// Loop-owned (it reads peerCaps and the ledger); call it on the event loop, as
// EstimateNetwork is called. Reading moves nothing.
func (n *Node) EconomySample() EconomySample {
	es := EconomySample{Mix: map[string]int{}, ServeBytesByTier: map[string]int64{},
		RepairsByTier: map[string]int64{}, PledgedBytesByTier: map[string]int64{}, ReportersByTier: map[string]int{}}
	served := make([]int64, 0, len(n.peerCaps)+1)
	repairs := make([]int64, 0, len(n.peerCaps)+1)
	add := func(capTotal, srv, rep int64) {
		tier := capacityTier(capTotal)
		if tier == "" {
			return
		}
		// Size and Mix count every CLASSIFIABLE peer: they are answers about the crowd's
		// SHAPE, derived from the capacity pledge, and a peer that withholds its work
		// counters still pledged capacity. The two work series below are narrower.
		es.Size++
		es.Mix[tier]++
		// M-2: a peer that reported NOTHING AT ALL is excluded from both work series.
		// The discriminator is the PAIR, not the field: both wire fields are omitempty,
		// so a withholding peer emits NEITHER key while a peer that serves but has never
		// repaired emits key 29 and not key 30. So a peer with any positive term is a
		// REPORTING peer, and its zero in the other field is a genuine measured zero that
		// counts. Only the all-zero peer is ambiguous, and there "withholding" and "has
		// done nothing yet" really are the same bytes — excluding it is the honest move
		// and it is the safe direction, because counting it as a zero inflates the Gini
		// toward the capture reading.
		if srv <= 0 && rep <= 0 {
			return
		}
		served = append(served, srv)
		// THE PER-TIER TOTALS accumulate HERE and nowhere else, which is what keeps them
		// on the SAME population as the two series: past the M-2 return, so a peer that
		// reported nothing contributes to no numerator and no denominator, and the map key
		// is written only for a REPORTING peer, so a tier with no reporter stays ABSENT
		// rather than acquiring a 0 that reads as a measured zero. A tier whose reporters
		// summed to nothing DOES get a 0 here, and that zero is a measurement -- the pair
		// (total, ReportersByTier) is what tells the two apart.
		es.ReportersByTier[tier]++
		es.ServeBytesByTier[tier] += srv
		es.PledgedBytesByTier[tier] += capTotal
		if RepairCapable(tier) {
			repairs = append(repairs, rep)
			// Keyed for the CAPABLE classes only, matching the repairs series above
			// through the same predicate: a pony's repairs are outside the population the
			// repair share claims to describe.
			es.RepairsByTier[tier] += rep
		}
	}
	for _, c := range n.peerCaps {
		add(c.total, c.served, c.repairs)
	}
	if n.capRep != nil {
		_, total := n.capRep.Capacity()
		// SELF FOLLOWS THE SAME RULE AS EVERY PEER (M-3, second leg). A node that does
		// not publish its work counters on the wire does not put them into its own
		// published aggregate either: the route clause in cmd/silt is what closes M-3,
		// and this is the belt to its braces — under the withholding default self's terms
		// never enter the series at all, so there is nothing for a future open route to
		// republish. Self stays in Size and Mix, which are capacity answers.
		var srv, rep int64
		if n.cfg.PublishWorkCounters {
			srv, rep = n.selfWork()
		}
		if capacityTier(total) != "" {
			es.SelfIncluded = true
			add(total, srv, rep)
		}
	}
	// The SAME estimator NetEstimate publishes, called through the same function, so
	// the two surfaces cannot drift onto different crowd estimates.
	es.EstimatedNodes = n.EstimateNetwork().EstimatedNodes
	es.ServeGini = credit.Gini(served)
	es.ServeSampleSize = len(served)
	es.RepairGini = credit.Gini(repairs)
	es.RepairSampleSize = len(repairs)
	for _, v := range served {
		es.ServeWorkTotal += v
	}
	for _, v := range repairs {
		es.RepairWorkTotal += v
	}
	// CapableSize walks Mix through the SAME RepairCapable predicate the repairs series
	// uses, rather than naming horse and archival again: the two cannot drift onto
	// different populations, and a future change to the capable set moves both at once.
	for tier, n := range es.Mix {
		if RepairCapable(tier) {
			es.CapableSize += n
		}
	}
	return es
}

// selfWork reads THIS node's two gossiped work counters off its own ledger. SINCE THE
// PROCESS STARTED, not lifetime: the ledger is ephemeral through the RC (D-FP2-SCOPE), so
// both reset at every restart. See ports.Message.ServedBytes for what that costs. It uses
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
