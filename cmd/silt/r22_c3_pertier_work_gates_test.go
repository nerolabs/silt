package main

// Lane C3 / R2.2 — THE PER-TIER WORK TOTALS, and the T-AR gate they re-found.
// (Economist ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09 §3a,
// Builder item 1; ratified context D-WORK-VISIBILITY, decisions.md, 2026-09-09.)
//
// WHY THIS FILE EXISTS. T-AR is a TIER-SHARE statement: "the edge tier that does the
// MAJORITY of the work must remain a net-positive place to do it" (docs/TENETS.md Part IX).
// Until the per-tier totals landed, nothing published carried a per-tier work quantity, so
// the tenet had no source and the two Ginis beside it answer a different question — they
// are per-NODE dispersion over a tier design that spans a 500:1 capacity ratio. The
// sibling file measures the two statistics DISAGREEING on one sample: G_adj 0.1726
// (CONCENTRATED) while the pony tier serves 0.8184 of the bytes. Only one of them is the
// tenet.
//
// AND THE NEAREST-LOOKING FIELD IS THE TRAP. mix[].share is a share of NODE COUNT and reads
// ~0.99 by construction under the ratified 10000:100:1 vision ratio.
// TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes measured 0.9891 against a true byte
// share of 0.1998. THE GATE BELOW DRIVES BOTH NUMBERS OFF THE PRODUCT and asserts they
// give OPPOSITE verdicts on that fixture — which is the whole reason the clause could not
// simply be pointed at the field that was already there.
//
// THE SAME BINDING CONSTRAINT AS THE SIBLING FILE. Every number asserted here is read out
// of the JSON body of a REAL GET on the REAL registered route, driven from a REAL
// *node.Node whose peerCaps were filled by REAL inbound gossip over a transport, through
// the shared c3Fixture. Nothing here recomputes a product figure beside the product's,
// except where an arm's whole point is an independent control and says so.
//
// WHAT THESE GATES MAY AND MAY NOT DO, stated before the constants so nobody over-reads
// them. Every term is self-reported and Sybil-settable in both directions, and on the
// shipped -privacy default NO node gossips its work counters at all, so in production every
// figure here is a NAMED ABSENCE. That is expected and ratified: D-WORK-VISIBILITY grades
// decentralization in the HARNESS, where the operator sets the counters, and claims no
// production concentration alarm. These gates are harness gates. They may abort a canary.
// They may never certify federation.

import (
	"math"
	"testing"

	"github.com/nerolabs/silt/core/node"
)

// ---- the constants, and exactly whose they are ---------------------------------------

const (
	// tarEdgeMajorityFloor is T-AR read LITERALLY: "majority" is more than half. It is the
	// tenet's own word turned into a number and nothing else, which is why it is the one
	// threshold in this lane the Economist RETAINED while withdrawing both Ginis'
	// (advisory "The numbers I am withdrawing, listed").
	tarEdgeMajorityFloor = 0.50
	// ptValidityMarginNum / ptValidityMarginDen is the advisory's 0.8 factor on the
	// mix-conditioned null, and it is [ASSUMPTION] in the advisory's own labelling — the
	// Economist's, not this seat's, and owed a re-derivation on the first field data.
	//
	// IT IS SPELLED AS A RATIO, AND AN EARLIER COMMENT HERE GAVE A FALSE REASON FOR THAT.
	// The struck claim was that with the literal 0.80 the fixed-point row would be "decided
	// by a rounding mode". MEASURED, and two blind seats measured it independently before
	// this seat re-ran it: fl(0.8) = 0x3fe999999999999a, the exact product 0.625*fl(0.8) is
	// 0.5 + 2^-55 -- a QUARTER ulp above 0.5, since ulp(0.5) upward is 2^-53 -- so it rounds to
	// exactly 0.5 under round-to-nearest, which Go mandates and exposes no mode for. Both
	// forms give bits 0x3fe0000000000000 at k = 10, substituting the literal leaves every
	// arm in this file GREEN, and the assertion that claimed to drive it drove nothing.
	//
	// THE TRUE REASON TO KEEP THE RATIO, and it is a preference and not a hazard: E*4 is
	// exact (a power-of-two scaling) and the single division that follows is correctly
	// rounded, so E*4/5 IS the correctly-rounded 4E/5 while E*0.8 carries two roundings.
	// They do differ, by one ulp, OFF the fixed point -- measured over 240,000 mixes of this
	// family, 83,512 of them (34.8 %), including the k = 4 row of the table in
	// TestGateC3_3c_TheValidityBoundaryIsMeasuredNotDerivedFromTheComparison, where min()
	// differs too (0.34042553191489361 against 0.34042553191489366). The table is written in
	// the ratio's values, so the code should be too. NOTHING DEPENDS ON IT: the min() clamp
	// absorbs an upward ulp at 0.50, and a downward ulp only loosens the floor, which is the
	// false-PASS direction and never a false RED.
	ptValidityMarginNum = 4.0
	ptValidityMarginDen = 5.0
)

// The disk weighting, in TB, from the Economist's published tier table
// (silt-reviews/economist/2026-09-01-tiered-edge-economy-sustainability-audit.md).
//
// WHY THE DISK WEIGHTING AND NOT THE CPU WEIGHTING, given that the same advisory WITHDREW
// the disk weighting as the model for a serve series. Both statements are right and they
// are about different uses. As a MODEL of serve work it is wrong: serve bytes are
// bandwidth- and demand-bound, not holdings-bound. As the null in a FLOOR's validity
// condition it is the correct choice precisely because it is the weighting that gives the
// edge tier its LOWEST honest share — it is the worst honest case, so a floor that does not
// false-fire under it does not false-fire under the CPU weighting either. Measured on the
// same mix (1000:10:1): disk-weighted honest pony share 0.6250, CPU-weighted 0.9141.
const (
	ptDiskWeightPony     = 0.1
	ptDiskWeightHorse    = 1.0
	ptDiskWeightArchival = 50.0
)

// ---- the gate predicate ---------------------------------------------------------------

type ptVerdict int

const (
	// ptPass: the sample's serve series covers enough of itself to be a measurement, and
	// the edge tier's measured byte share clears the floor its own sampled mix justifies.
	ptPass ptVerdict = iota
	// ptIndeterminate: withheld, absent, unknown, or the serve series covers too little of
	// the sample. NEVER reported as a tenet violation — under-reporting by the edge tier
	// depresses the edge tier's own share (see ptEdgeMajorityGate), so a dark panel and a
	// captured network are different facts with different operator actions.
	ptIndeterminate
	// ptEdgeMinority: measured, and the edge tier does a minority of the reported work.
	ptEdgeMinority
)

func (v ptVerdict) String() string {
	switch v {
	case ptPass:
		return "PASS"
	case ptIndeterminate:
		return "INDETERMINATE"
	default:
		return "EDGE-MINORITY"
	}
}

type ptReading struct {
	Verdict      ptVerdict
	Why          string
	Observed     float64 // the published ponyShareOfServedBytes
	TierCoverage float64 // reporting ponies / sampled ponies
	// WorstTierCoverage is phi: the LEAST-covered tier present in the sampled mix, and the
	// term that turns an observation into an interval. WorstTier names it.
	WorstTierCoverage float64
	WorstTier         string
	// Lower and Upper are [phi*observed, observed/phi], the interval the observation bounds
	// the truth to. The verdict is which side of the floor that interval lands on.
	Lower, Upper float64
	ExpectedNull float64 // the disk-weighted honest share for THIS sampled mix
	Floor        float64 // min(tarEdgeMajorityFloor, 4/5 * ExpectedNull)
}

// ptExpectedPonyShareDiskWeighted is the honest null for the SAMPLED mix. It is computed
// from the mix on every reading rather than carried as a constant, which is the correction
// the advisory's Finding 2 forced: the honest null is MIX-dependent, not sample-size
// dependent, and two 4,096-node samples of the SAME honest network read 0.1726 and 0.0800
// depending on whether the single archival node was drawn.
func ptExpectedPonyShareDiskWeighted(nw c3NetworkWire) float64 {
	w := map[string]float64{node.TierPony: ptDiskWeightPony, node.TierHorse: ptDiskWeightHorse,
		node.TierArchival: ptDiskWeightArchival}
	var pony, total float64
	for _, row := range nw.Mix {
		b := float64(row.Sampled) * w[row.Class]
		total += b
		if row.Class == node.TierPony {
			pony += b
		}
	}
	if total <= 0 {
		return 0
	}
	return pony / total
}

// ptEdgeMajorityGate is advisory §3a assertion C-3: ponyShareOfServedBytes >= 0.50, with
// the validity condition attached.
//
// THE ORDER OF THE REFUSALS IS THE RULE. Absence first, then coverage, then the floor —
// because the failure this gate is most likely to see in the field is not capture, it is
// darkness, and a dark surface reported as a tenet violation is its own defect.
//
// THE COVERAGE CLAUSE IS PER-TIER, AND ITS FLOOR IS A THEOREM OF THE TENET FLOOR RATHER
// THAN A NEW PARAMETER. Its first form was the SAMPLE-WIDE reporting fraction against
// c3ServeReportingMin (0.85), and the Economist measured that structurally incapable of the
// job it was standing in for: see
// TestGateC3_3g_TheConcentratingTierMustBuyDecoysToBuyAPass, where five nodes of 1,011
// -- 0.49 % of the sample -- turn a measured EDGE-MINORITY of 0.1998 into a PASS of 0.9940
// while the sample-wide fraction stays at 0.9951. Under the ratified 10000:100:1 target the
// non-edge tiers ARE the sample's one percent, so a COUNT-WEIGHTED coverage measure is blind
// to exactly the tiers whose silence matters -- and the target ratio is what makes it so.
//
// WHAT THE REPLACEMENT BUYS, STATED HERE BECAUSE THIS IS WHERE THE CLAUSE LIVES: it PRICES
// that attack, it does not close it. The bound's purchase price is the assumption below, and
// a deliberate silencer violates that assumption by construction. MEASURED: the d = 0 arm
// refuses by 0.0030, and ONE decoy the adversary runs in its own band raises phi past the
// floor while also dragging the floor down. The price is one node per silenced band, and the
// same gate drives it.
//
// WHY A TIER SHARE NEEDS AN INTERVAL WHERE THE GINI GETS AN IDENTITY. For the Gini,
// silence-means-idle yields the two-sided identity G_adj = (1-c) + c*G_pub. For a tier SHARE
// it does not: the true share is (P + P_s) / (P + P_s + O + O_s), and O_s -- work done by
// silent NON-edge peers -- is unbounded above, so the observation bounds the truth in
// NEITHER direction on its own. Only the one-tier case is derivable, and
// TestGateC3_3b_EdgeSilenceDepressesTheEdgesOwnShare asserts it: if only ponies go silent
// the published share falls, because (P-d)/(P+O-d) < P/(P+O) for d,O > 0.
//
// SO THE BOUND IS BOUGHT WITH A NAMED ASSUMPTION, and it is the analogue of the Gini's
// "silence means idle": WITHIN A TIER, silence is uncorrelated with work rate, so that
// tier's true bytes are B_t = R_t / c_t. Under it, with phi = the least coverage among the
// tiers PRESENT in the sampled mix:
//
//	R_t <= R_t/c_t <= R_t/phi   for every tier
//	=>  phi * s_obs  <=  s_true  <=  s_obs / phi
//
// [DERIVED] That interval IS the gate. PASS iff its LOWER end clears the floor,
// EDGE-MINORITY iff its UPPER end is below the floor, INDETERMINATE iff it straddles --
// three cases, exhaustive, with no fourth.
//
// AND THE COVERAGE FLOOR FALLS OUT AS A THEOREM. s_obs <= 1, so a PASS requires
// phi >= phi*s_obs >= F: the minimum per-tier coverage a PASS needs is the tenet floor
// itself. No second parameter, exactly as the Gini's 1-T boundary is a theorem of T. Pinned
// with its endpoints run by
// TestGateC3_3f_ThePerTierCoverageFloorIsATheoremOfTheTenetFloor.
//
// THE SAMPLE-WIDE CLAUSE IS NOT KEPT AS A BELT TO THIS BRACES. It is dominated, and where
// the two disagree it is WRONG: every tier at coverage 0.6 with s_obs 0.95 gives a lower
// bound of 0.57, a sound PASS, which the 0.85 fraction refuses. A dominated clause that
// refuses sound readings is not a defence, so it is removed rather than carried.
//
// THE UNIFYING RULE, stated once because two seats rediscovered it separately (Economist
// addendum Part 2 §7.3): every concentration statistic carries a coverage refusal at its OWN
// granularity, and is never aggregated above the granularity at which capture can occur.
func ptEdgeMajorityGate(conc c3ConcentrationWire, nw c3NetworkWire) ptReading {
	if conc.CountersWithheld {
		return ptReading{Verdict: ptIndeterminate, Why: "the tenet figure is withheld: the reader cannot see it at all"}
	}
	if conc.Sample == nil {
		return ptReading{Verdict: ptIndeterminate, Why: "no sample block: the required sibling of every gossip-estimated number is absent"}
	}
	if conc.Sample.TooSmall {
		return ptReading{Verdict: ptIndeterminate, Why: "sample below minGossipSample, so no gossip-estimated figure is published"}
	}
	if conc.PonyShareOfServedBytes == nil {
		return ptReading{Verdict: ptIndeterminate,
			Why: "ponyShareOfServedBytes is ABSENT from the concentration document: the tenet has no source on this surface, which is the state this whole file exists to leave behind"}
	}
	ps := conc.PonyShareOfServedBytes
	r := ptReading{Observed: ps.val(), TierCoverage: ps.Coverage, ExpectedNull: ptExpectedPonyShareDiskWeighted(nw)}
	r.Floor = math.Min(tarEdgeMajorityFloor, r.ExpectedNull*ptValidityMarginNum/ptValidityMarginDen)
	if !ps.Known {
		r.Why = "the tenet figure is UNKNOWN (" + ps.Reason + "): an absent measurement is not a passing 0, and it is not a failing 0 either"
		r.Verdict = ptIndeterminate
		return r
	}
	// phi, and the interval it buys. It is read off the CONCENTRATION document, so the
	// observation and its validity input come from ONE snapshot -- the same reason
	// capableSize was moved onto this document.
	if conc.WorstTierCoverage == nil {
		r.Verdict, r.Why = ptIndeterminate, "the document publishes no per-tier coverage, so the share is over a population whose completeness is unknown"
		return r
	}
	r.WorstTierCoverage, r.WorstTier = conc.WorstTierCoverage.Coverage, conc.WorstTierCoverage.Class
	seen := itoa(conc.WorstTierCoverage.Reporting) + " of " + itoa(conc.WorstTierCoverage.Population)
	if r.WorstTierCoverage <= 0 {
		r.Verdict = ptIndeterminate
		r.Why = "the whole " + r.WorstTier + " tier is silent (" + seen + " reporting), so it is absent from the denominator entirely and the edge share is unbounded above"
		return r
	}
	r.Lower, r.Upper = r.WorstTierCoverage*r.Observed, r.Observed/r.WorstTierCoverage
	switch {
	case r.Lower >= r.Floor:
		r.Verdict = ptPass
	case r.Upper < r.Floor:
		r.Verdict = ptEdgeMinority
		r.Why = "the edge tier serves " + ftoa(r.Observed) + " of the reported bytes and even its UPPER bound " + ftoa(r.Upper) +
			" is below the floor " + ftoa(r.Floor) + " this sampled mix justifies (honest disk-weighted null " + ftoa(r.ExpectedNull) +
			"): work is concentrating away from the edge, measured"
	default:
		r.Verdict = ptIndeterminate
		r.Why = "the " + r.WorstTier + " tier reports only " + ftoa(r.WorstTierCoverage) + " of itself (" + seen +
			"), so the published " + ftoa(r.Observed) + " bounds the truth only to [" + ftoa(r.Lower) + ", " + ftoa(r.Upper) +
			"], which straddles the floor " + ftoa(r.Floor) + ". A silent tier leaves the DENOMINATOR, so its silence inflates every other tier's share"
	}
	return r
}

// ftoa keeps the messages above free of a fmt import in the predicate.
func ftoa(f float64) string { return itoaFloat(f) }

func itoaFloat(f float64) string {
	// four decimals, which is the precision every other message in this lane uses
	n := int64(math.Round(f * 10000))
	whole, frac := n/10000, n%10000
	if frac < 0 {
		frac = -frac
	}
	d := []byte{byte('0' + frac/1000%10), byte('0' + frac/100%10), byte('0' + frac/10%10), byte('0' + frac%10)}
	return itoa(int(whole)) + "." + string(d)
}

// ---- the fixtures ---------------------------------------------------------------------

// ptConcentratedPeers is the A2/1b shape: the top 5 horses serve 80 % of the bytes. Same
// MIX as the healthy vision shape (1000 : 10 : 1), so the two arms differ ONLY in where the
// bytes went — which is what makes the mix-conditioned floor identical across them and the
// comparison about the work rather than about the composition.
func ptConcentratedPeers() []c3Peer {
	const ponies = 1000
	return []c3Peer{
		{capTotal: c3HorseCap, served: 160_000 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3PonyCap, served: (200_000 / ponies) * c3Unit, repairs: 0, n: ponies},
		{capTotal: c3HorseCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: 1},
	}
}

// ---- GATE 3 ----------------------------------------------------------------------------

// TestGateC3_3_EdgeMajorityOfServeWorkIsTheTenetNotTheNodeShare is advisory assertion 1's
// second clause, which had no source until the per-tier totals landed.
//
// GATE: ptEdgeMajorityGate over GET /api/economy/concentration and /api/economy/network.
//
// FOUR ARMS. HEALTHY must PASS. CONCENTRATED must be EDGE-MINORITY. The SUBSTITUTION arm
// asserts the two adjacent product fields give OPPOSITE verdicts on the concentrated
// fixture — that is the trap, measured off the product on both sides rather than described.
// The WITHHELD arm asserts the whole block rides the existing privacy marker.
//
// CONTROLLED REVERT (G-PT-1): point ponyServeShare at the node count —
// `sample.Mix[node.TierPony]` over `sample.Size`. MEASURED at both reddening points, because
// "the concentrated arm passes" is a claim and only the run says so: the gate stops FIRST on
// the healthy arm's literal check (published 0.989119683, want 0.914076782), and with that
// one literal muted it reaches the concentrated arm and reports
// `CONCENTRATED ARM PASS: ... the edge tier serves 0.9891` — total serve capture read as a
// PASS, which is the substitution this whole file is about.
func TestGateC3_3_EdgeMajorityOfServeWorkIsTheTenetNotTheNodeShare(t *testing.T) {
	// --- HEALTHY: the ratified vision ratio, CPU-weighted, everybody reporting.
	concH, nwH := c3Fixture(t, 41, c3HealthyPeers())
	rH := ptEdgeMajorityGate(concH, nwH)
	if rH.Verdict != ptPass {
		t.Fatalf("HEALTHY ARM %s: %s\n  The ratified vision shape must clear its own tenet, or this gate fires on the network silt is trying to build.", rH.Verdict, rH.Why)
	}
	t.Logf("healthy: %s  edge serves %.4f of reported bytes (floor %.4f from a disk-weighted null of %.4f); tier coverage %.4f, series coverage %.4f",
		rH.Verdict, rH.Observed, rH.Floor, rH.ExpectedNull, rH.TierCoverage, rH.WorstTierCoverage)

	// The tenet figure must be a MEASUREMENT and not merely present.
	if !concH.PonyShareOfServedBytes.Known || concH.PonyShareOfServedBytes.Value == nil {
		t.Fatalf("HEALTHY ARM: ponyShareOfServedBytes is not a measurement (%+v)", concH.PonyShareOfServedBytes)
	}
	// 1000 ponies at 1 unit, 10 horses at 7, 1 archival at 24: 1000/1094.
	if want := 1000.0 / 1094.0; math.Abs(rH.Observed-want) > 1e-12 {
		t.Fatalf("HEALTHY ARM: the published edge share is %.9f, want %.9f (1000 pony-units of 1094 total). The figure is not the sum this fixture built.", rH.Observed, want)
	}
	// THE HARNESS DISCIPLINE (Economist addendum Part 1 §5), asserted rather than assumed: on
	// a GRADED arm the conditioned branch must be inert (the floor IS the flat tenet floor)
	// and coverage must be 1.0. Anything less is a fixture defect, not a finding.
	if rH.Floor != tarEdgeMajorityFloor || rH.WorstTierCoverage != 1 {
		t.Fatalf("HEALTHY ARM: floor %.6f (want the flat %.2f) at worst-tier coverage %.4f (want 1.0000). A graded arm that depends on the conditioned branch, or that runs short of full coverage, is a fixture defect.",
			rH.Floor, tarEdgeMajorityFloor, rH.WorstTierCoverage)
	}

	// --- CONCENTRATED: the same MIX, the bytes moved to five horses.
	concC, nwC := c3Fixture(t, 42, ptConcentratedPeers())
	rC := ptEdgeMajorityGate(concC, nwC)
	if rC.Verdict != ptEdgeMinority {
		t.Fatalf("CONCENTRATED ARM %s: %s\n  Five of 1011 nodes serve 80 %% of the bytes and the edge tier serves %.4f. If this is not EDGE-MINORITY the gate has no teeth.", rC.Verdict, rC.Why, rC.Observed)
	}
	if rC.Floor != rH.Floor {
		t.Fatalf("the two arms must share a floor (%.6f vs %.6f) for the comparison to isolate the WORK: they were built with the same mix on purpose", rC.Floor, rH.Floor)
	}
	if rC.WorstTierCoverage != 1 {
		t.Fatalf("CONCENTRATED ARM: worst-tier coverage %.4f, want 1.0000 — a graded arm short of full coverage is a fixture defect, and the silence case has its own gate (TestGateC3_3g_TheConcentratingTierMustBuyDecoysToBuyAPass)", rC.WorstTierCoverage)
	}
	// The fixture's OWN assigned share: 1000 ponies x 200 units of
	// 5*160000 + 1000*200 + 5*200 + 200 = 1,001,200.
	if want := 200_000.0 / 1_001_200.0; math.Abs(rC.Observed-want) > 1e-12 {
		t.Fatalf("CONCENTRATED ARM: the published edge share is %.9f, want %.9f from the fixture's own assignment", rC.Observed, want)
	}
	t.Logf("concentrated: %s  edge serves %.4f (floor %.4f) — same mix, same floor, the bytes moved",
		rC.Verdict, rC.Observed, rC.Floor)

	// --- THE SUBSTITUTION ARM. Both numbers off the PRODUCT, on the SAME document, giving
	// OPPOSITE verdicts. mix[].share is the field a reader reaches for; it is a share of
	// NODE COUNT and it passes total serve capture.
	var nodeShare float64
	var work *c3TierWorkWire
	for _, row := range nwC.Mix {
		if row.Class == node.TierPony {
			nodeShare, work = row.Share, row.Work
		}
	}
	if nodeShare < tarEdgeMajorityFloor {
		t.Fatalf("mix[pony].share is %.4f on the concentrated fixture, so the substitution does NOT invert here and this arm has no subject. Re-derive: the trap depends on the node share being high BY CONSTRUCTION under the vision ratio.", nodeShare)
	}
	if rC.Observed >= tarEdgeMajorityFloor {
		t.Fatalf("the concentrated fixture gives the edge tier %.4f of the bytes, which is not a minority. Rebuild it.", rC.Observed)
	}
	t.Logf("SUBSTITUTION, both off the product: mix[pony].share = %.4f (share of NODE COUNT, PASSES a 0.50 floor) while ponyShareOfServedBytes = %.4f (share of BYTES, VIOLATES it). Ratio %.2fx.",
		nodeShare, rC.Observed, nodeShare/rC.Observed)
	// And the per-tier block on the row carries the same byte share, so a reader who stays
	// on the network route is not left with only the misleading one.
	if work == nil || !work.ServeShare.Known {
		t.Fatalf("mix[pony].work.serveShare is absent or unknown on a fully-reporting fixture (%+v) — the byte share must sit BESIDE the node share, or the trap is unmitigated on that route", work)
	}
	if math.Abs(work.ServeShare.val()-rC.Observed) > 1e-12 {
		t.Fatalf("the two published edge byte shares disagree: mix[pony].work.serveShare %.9f vs concentration.ponyShareOfServedBytes %.9f. They are the same quantity and must come from the same totals.", work.ServeShare.val(), rC.Observed)
	}

	// --- THE WITHHELD ARM. No new marker, no new clause: the block rides gossipWithheld.
	// CONTROLLED REVERT (G-PT-9): set PonyShareOfServedBytes and CapableSize BEFORE the
	// gossipWithheld early return. Measured, the unauthenticated document then ships
	// `Known:true Value:1 Reporting:100 Population:100` at the shipped -privacy default.
	withheld := economyConcentrationDoc(node.EconomySample{Size: 100, ServeSampleSize: 100, ServeWorkTotal: 5,
		Mix: map[string]int{node.TierPony: 100}, ServeBytesByTier: map[string]int64{node.TierPony: 5},
		ReportersByTier: map[string]int{node.TierPony: 100}}, nil, readerAuth{privacy: privacyDefaultWithheld})
	if withheld.PonyShareOfServedBytes != nil || withheld.CapableSize != nil {
		t.Fatalf("the unauthenticated document at the shipped -privacy default still publishes the tenet share or the capable count (%+v / %+v). Both are derived from the covered set the Gini withhold exists to protect.",
			withheld.PonyShareOfServedBytes, withheld.CapableSize)
	}
	if !withheld.CountersWithheld {
		t.Fatalf("the withheld document carries no countersWithheld marker")
	}
	netWithheld := economyNetworkDoc(node.EconomySample{Size: 100, Mix: map[string]int{node.TierPony: 100}},
		readerAuth{privacy: privacyDefaultWithheld})
	if len(netWithheld.Mix) != 0 {
		t.Fatalf("the unauthenticated network document still publishes the mix, and the mix now carries the per-tier work block: %+v", netWithheld.Mix)
	}
}

// TestGateC3_3a_ATierWithNoReportingPeerIsANamedAbsenceNeverAZero closes the exclusion hole
// ONE LEVEL DOWN, and it is the reason tierShareOf checks reporters BEFORE the denominator.
//
// THE MECHANISM. Under the certified M-2 rule (core/node/economysample.go) a peer that
// reported neither counter is excluded from the work series rather than counted as a zero.
// So a tier ALL of whose peers are silent contributes 0 to every numerator while OTHER
// tiers keep the denominator positive — and 0/positive is a perfectly well-formed 0.0. That
// number says "this tier does none of the work". The truth is "no peer of this tier told me
// anything". On the shipped -privacy default that is EVERY tier, every time.
//
// CONTROLLED REVERT (G-PT-2): DELETE the reporters test from tierShareOf. The silent tier
// then renders known:true with value 0.0000 and this gate reddens on the Known assertion.
//
// AND THE REVERT THIS SEAT TRIED FIRST CAME BACK GREEN, which is why the last arm exists.
// Swapping the two refusals instead of deleting one changes nothing on this fixture: with the
// denominator positive the swapped code still reaches the reporters test and still refuses.
// The order is only observable when BOTH are unknown, so the wholly-silent arm at the end
// drives it (G-PT-2b). The first draft of tierShareOf's comment claimed the ORDER was the
// rule; running it refuted that in one command.
func TestGateC3_3a_ATierWithNoReportingPeerIsANamedAbsenceNeverAZero(t *testing.T) {
	// Ponies pledge capacity — so they are classifiable, counted in Size and in the mix —
	// and report NOTHING. The other two tiers report, so every denominator is positive.
	silentEdge := []c3Peer{
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 100},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
	conc, nw := c3Fixture(t, 43, silentEdge)

	var pony *c3TierWorkWire
	var sampled int
	for _, row := range nw.Mix {
		if row.Class == node.TierPony {
			pony, sampled = row.Work, row.Sampled
		}
	}
	if pony == nil {
		t.Fatalf("the pony row lost its work block entirely. A silent tier must be NAMED as absent, not omitted — an omitted block reads as an old server, not as a fact about this network.")
	}
	if sampled != 100 {
		t.Fatalf("mix[pony].sampled = %d, want 100: a silent peer still PLEDGED capacity and must stay in the composition answer", sampled)
	}
	if pony.Reporting != 0 || pony.Coverage != 0 {
		t.Fatalf("mix[pony] reports %d reporters at coverage %.4f, want 0 and 0 — the fixture did not land silent", pony.Reporting, pony.Coverage)
	}
	// THE PIN. Every one of the three shares must be a NAMED absence, and the denominator
	// each divides by must be positive so the arm is not vacuous.
	if conc.ServeGini == nil || !conc.ServeGini.Known {
		t.Fatalf("the serve series is dark on this fixture, so 0/0 explains the absences and this gate proves nothing about the ORDER of the refusals. The horses and the archival node must be reporting.")
	}
	for name, sh := range map[string]*c3TierShareWire{
		"serveShare": pony.ServeShare, "repairShare": pony.RepairShare, "pledgedShare": pony.PledgedShare,
	} {
		if sh == nil {
			t.Fatalf("mix[pony].work.%s is absent from the wire; it must be present and UNKNOWN, with a reason", name)
		}
		if sh.Known {
			t.Fatalf("mix[pony].work.%s renders known:true (value %.4f) on a tier where NO peer reported. A tier that told me nothing and a tier that did nothing are different facts and both compute to 0.0; the exclusion rule is what makes the first one look like the second.", name, sh.val())
		}
		if sh.Value != nil {
			t.Fatalf("mix[pony].work.%s ships a value beside known:false: %v", name, *sh.Value)
		}
		if sh.Reason == "" {
			t.Fatalf("mix[pony].work.%s is unknown with no reason. A named absence is named.", name)
		}
	}
	// The two absences are DIFFERENT absences and must not collapse: serve/pledged are "no
	// reporter", repair is "outside the population by construction" — a pony is not
	// repair-capable under D-TIERING coupling (b) even when every pony reports.
	if pony.RepairShare.Reason == pony.ServeShare.Reason {
		t.Fatalf("the pony repair share and the pony serve share give the SAME reason (%q). One is a missing measurement and the other is an undefined quantity; an operator's action differs.", pony.RepairShare.Reason)
	}
	// And the tenet figure itself.
	ps := conc.PonyShareOfServedBytes
	if ps == nil || ps.Known || ps.Value != nil {
		t.Fatalf("ponyShareOfServedBytes on a silent-edge network is %+v; it must be present, known:false and value-less", ps)
	}
	if ps.Population != 100 || ps.Reporting != 0 || ps.Coverage != 0 {
		t.Fatalf("ponyShareOfServedBytes reports %d of %d at coverage %.4f, want 0 of 100 at 0.0000 — the reader's only handle on the gap is these three numbers", ps.Reporting, ps.Population, ps.Coverage)
	}
	r := ptEdgeMajorityGate(conc, nw)
	if r.Verdict != ptIndeterminate {
		t.Fatalf("a network where the whole edge tier is silent reads %s (%s). It must be INDETERMINATE: reporting a dark surface as a tenet VIOLATION is the defect this verdict split exists to prevent, and on the shipped -privacy default darkness is the normal state.", r.Verdict, r.Why)
	}
	t.Logf("silent edge: %s — %s", r.Verdict, r.Why)

	// THE ORDERING ARM (G-PT-2b). Now nobody at all reports, so BOTH refusals in tierShareOf
	// are live: the tier has no reporter AND the denominator is zero. This is the ONLY input
	// on which the order of the two tests is observable, and it is the shipped -privacy
	// default's own shape (D-WORK-VISIBILITY: no node gossips its counters, so every peer is
	// excluded and both work series are empty). The reason must be the one an operator can
	// act on.
	allSilent := []c3Peer{
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 100},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 10},
		{capTotal: c3ArchivalCap, served: 0, repairs: 0, n: 1},
	}
	concQ, nwQ := c3Fixture(t, 51, allSilent)
	if concQ.ServeGini != nil {
		t.Fatalf("a wholly silent sample published a serve Gini (%+v); the denominator is not zero and this arm has no subject", concQ.ServeGini)
	}
	var ponyQ *c3TierWorkWire
	for _, row := range nwQ.Mix {
		if row.Class == node.TierPony {
			ponyQ = row.Work
		}
	}
	if ponyQ == nil || ponyQ.ServeShare == nil || ponyQ.ServeShare.Known {
		t.Fatalf("the wholly-silent pony row is %+v; it must be present and unknown", ponyQ)
	}
	if ponyQ.ServeShare.Reason != noTierReporters {
		t.Fatalf("on a wholly SILENT network the pony serve share gives the empty-denominator reason: %q. Both refusals are live here, so this is the input the ORDER decides, and the reporters reason is the one an operator can act on — the denominator being zero is a restatement of the same silence one level out.",
			ponyQ.ServeShare.Reason)
	}
	t.Logf("wholly silent (the shipped -privacy default's own shape): every tier unknown, and the reason is the no-reporters one, not the empty-denominator one")
}

// TestGateC3_3b_EdgeSilenceDepressesTheEdgesOwnShare measures the ONE claim about this
// figure's error direction that is actually derivable, instead of asserting it in prose.
//
// THE CLAIM: if only ponies go silent, the published edge share FALLS, because a silent
// pony removes its bytes from the numerator AND the denominator, and (P-d)/(P+O-d) <
// P/(P+O) whenever d > 0 and O > 0. So under-reporting by the edge tier can produce a false
// ALARM that names itself in `coverage`; it cannot produce a false clean bill.
//
// AND THE LIMIT OF THE CLAIM, which is why the gate refuses rather than alarms on arm B:
// the same identity does NOT hold when non-pony peers are the silent ones, so the observed
// share is not a bound on the truth in general. The coverage clause is what stands in for
// the bound the Gini gets from its identity.
//
// CONTROLLED REVERT (G-PT-10, shared with TestGateC3_3f_ThePerTierCoverageFloorIsATheoremOfTheTenetFloor and TestGateC3_3g_TheConcentratingTierMustBuyDecoysToBuyAPass): gate the raw
// observation instead of the interval's lower bound. MEASURED: arm B goes to PASS at 0.8449.
//
// CONTROLLED REVERT (G-PT-3): scale the pony numerator by population/reporting — the obvious
// "correct for coverage" edit. MEASURED: arm B's share goes to 0.694444444 against the built
// 0.347222222, so the extrapolation makes edge silence RAISE the edge's share. That is the
// false-clean-bill direction, and it is why this figure is published raw with its coverage
// beside it rather than extrapolated.
func TestGateC3_3b_EdgeSilenceDepressesTheEdgesOwnShare(t *testing.T) {
	// THE MIX IS THE VISION FAMILY AT k = 11, DELIBERATELY (Economist addendum Part 1 §5):
	// at k >= 11 the min() clamp is strictly active, so the conditioned floor IS the flat
	// 0.50 tenet floor and no arm here depends on the conditioned branch. That branch belongs
	// to TestGateC3_3c_TheValidityBoundaryIsMeasuredNotDerivedFromTheComparison, which is where a boundary belongs. An earlier draft used a
	// 100 : 10 : 1 mix, where the honest disk-weighted null is 0.1429 and the conditioned
	// floor drops to 0.1143 -- and arm B then read PASS, correctly against THAT floor, which
	// told the reader nothing about edge silence.
	full := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1100},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 11},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
	// The SAME network with half the edge tier withholding. Nothing about the work changed;
	// only what was said about it.
	half := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 550},
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 550},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 11},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
	concA, nwA := c3Fixture(t, 44, full)
	concB, nwB := c3Fixture(t, 45, half)
	rA, rB := ptEdgeMajorityGate(concA, nwA), ptEdgeMajorityGate(concB, nwB)

	t.Logf("full reporting: %s share %.4f (tier coverage %.4f, worst-tier coverage %.4f) | half the edge silent: %s share %.4f (tier coverage %.4f, worst-tier coverage %.4f)",
		rA.Verdict, rA.Observed, rA.TierCoverage, rA.WorstTierCoverage, rB.Verdict, rB.Observed, rB.TierCoverage, rB.WorstTierCoverage)

	if rA.Verdict != ptPass {
		t.Fatalf("the fully-reporting arm reads %s (%s); it is the control and must be a measurement", rA.Verdict, rA.Why)
	}
	if concB.PonyShareOfServedBytes == nil || !concB.PonyShareOfServedBytes.Known {
		t.Fatalf("arm B publishes no measured edge share (%+v); half the edge tier IS reporting, so the figure exists", concB.PonyShareOfServedBytes)
	}
	// The fixture's OWN assigned shares (Economist addendum Part 1 §5: in a harness the
	// ground truth is known, so assert against it and not against a model). 1100 pony-units
	// of 1100 + 11*7 + 24 = 1201 with everyone reporting; 550 of 550 + 77 + 24 = 651 with
	// half the edge silent.
	if wantA, wantB := 1100.0/1201.0, 550.0/651.0; math.Abs(rA.Observed-wantA) > 1e-12 || math.Abs(concB.PonyShareOfServedBytes.val()-wantB) > 1e-12 {
		t.Fatalf("arms measured %.9f and %.9f, want %.9f and %.9f — the fixtures did not land as built", rA.Observed, concB.PonyShareOfServedBytes.val(), wantA, wantB)
	}
	// THE PIN, part 1 — strictly DOWN. Not "differs": the direction is the whole claim.
	if !(concB.PonyShareOfServedBytes.val() < rA.Observed) {
		t.Fatalf("edge silence did not DEPRESS the edge share: %.6f (half silent) is not below %.6f (all reporting). The one-sidedness this figure is trusted for has changed and the gate's refusal ordering must be re-argued.",
			concB.PonyShareOfServedBytes.val(), rA.Observed)
	}
	// THE PIN, part 2 — and the gate does NOT call it a violation. The share fell to
	// 0.3472, under the flat 0.50, and the honest reading is "I cannot see", not "the edge
	// lost the network".
	if rB.Verdict != ptIndeterminate {
		t.Fatalf("arm B reads %s (%s). The share fell BECAUSE of silence, and a false alarm that reports itself as measured capture is the failure this verdict split exists to prevent.", rB.Verdict, rB.Why)
	}
	if rB.Floor != tarEdgeMajorityFloor {
		t.Fatalf("arm B's floor is %.6f, not the flat %.2f. This fixture sits at k = 11 so the conditioned branch is INERT here; if the floor has moved, the arm is measuring the branch instead of the silence.", rB.Floor, tarEdgeMajorityFloor)
	}
	if !(rB.Lower < tarEdgeMajorityFloor && rB.Upper >= tarEdgeMajorityFloor) {
		t.Fatalf("arm B's interval [%.4f, %.4f] does not STRADDLE the floor %.2f, so the INDETERMINATE verdict above is not being reached through the case this gate is about", rB.Lower, rB.Upper, tarEdgeMajorityFloor)
	}
	if rA.WorstTierCoverage != 1 {
		t.Fatalf("the control arm's worst-tier coverage is %.4f, not 1.0000. On a graded harness arm anything less is a fixture defect (Economist addendum Part 1 §5).", rA.WorstTierCoverage)
	}
	if rB.TierCoverage != 0.5 {
		t.Fatalf("arm B tier coverage %.4f, want 0.5000 — the reader's handle on the gap must be exact", rB.TierCoverage)
	}
}

// TestGateC3_3c_TheValidityBoundaryIsMeasuredNotDerivedFromTheComparison publishes the two
// boundaries this floor has, WITH the runs behind them, and it corrects the advisory on one
// of them.
//
// THE ADVISORY SAYS: "under the disk weighting with one archival node forced into the
// sample, the honest pony byte share crosses 0.50 at n ~ 562 ... At every sample of >= ~560
// classifiable nodes this reduces to the flat 0.50 tenet floor."
//
// THE FIRST HALF IS RIGHT AND THE SECOND HALF IS WRONG, and the arithmetic is one step.
// min(0.50, 0.8*E) equals 0.50 when E >= 0.625, not when E >= 0.50. On the vision-ratio
// family (100k ponies : k horses : 1 archival, disk-weighted) E(k) = 10k/(11k+50), so:
//
//	E = 0.500  at k = 50/9 = 5.556  ->  n = 101k+1 = 562.1   <- where a BARE 0.50 floor stops false-firing
//	E = 0.625  at k = 10           ->  n = 1011             <- where the CONDITIONED floor becomes the flat 0.50
//
// Two different boundaries for two different claims. n ~ 562 is the point below which a
// bare 0.50 floor fires on an honest network. n = 1011 is the point at or above which the
// conditioned floor IS 0.50. Between them the conditioned floor is below 0.50 and the
// condition is doing work.
//
// MEASURED TABLE — the null function run at the endpoints and one step either side. Every
// row here was produced by ptExpectedPonyShareDiskWeighted on the mix named, not by reading
// the formula:
//
//	 k    n     mix (p:h:a)      E = null    (4/5)E     floor=min(0.50,(4/5)E)
//	 4   405    400:4:1          0.4255      0.3404     0.3404
//	 5   506    500:5:1          0.4762      0.3810     0.3810
//	 6   607    600:6:1          0.5172      0.4138     0.4138     <- E crosses 0.50 between k=5 and k=6 (562.1)
//	 9   910    900:9:1          0.6040      0.4832     0.4832
//	10  1011   1000:10:1         0.6250      0.5000     0.5000     <- FIXED POINT: the floor becomes flat here
//	11  1112   1100:11:1         0.6433      0.5146     0.5000     <- the min() bites
//
// AND THE FIXED POINT IS EXACT, not within a tolerance: 0.625*4/5 is 2.5/5 = 0.5 in binary
// arithmetic. That is why ptValidityMargin is a ratio and not the literal 0.80 — with 0.80
// the k=10 row is decided by a rounding.
//
// THE DRIVEN ARM: a real 556-node disk-weighted sample through the real routes, where the
// bare floor and the conditioned floor DISAGREE, so the correction is not only arithmetic.
//
// TWO CONTROLLED REVERTS, AND ONE THAT CANNOT REDDEN — recorded because a green ablation is
// itself the finding. G-PT-4: change ptDiskWeightArchival from 50 to 49. MEASURED, the table
// reddens on its FIRST row — `k=4 (n=405, 400:4:1): null 0.430108 floor 0.344086, want
// 0.425532 / 0.340426` — and the k=10 fixed point moves with it. Literal expected values are
// the only teeth a fixed point can have: there is no mutation to ablate at the boundary
// itself, so the gate must BE the table and never `want := theFunctionUnderTest(...)`.
// G-PT-11: change tarEdgeMajorityFloor from 0.50 to 0.51 — the k = 11 row and the fixed-point
// assertion both redden, so the flat floor is pinned in both directions.
// AND THE ONE THAT DOES NOT: substituting the literal 0.80 for ptValidityMarginNum/Den leaves
// every arm GREEN. Measured, recorded in the const block, and the reason the margin's FORM
// carries no claim in this file.
func TestGateC3_3c_TheValidityBoundaryIsMeasuredNotDerivedFromTheComparison(t *testing.T) {
	type row struct {
		k                   int
		wantNull, wantFloor float64
	}
	// The literal table. NEVER `want := theFunctionUnderTest(...)`: a fixed point has no
	// mutation to ablate, so the gate must BE the expected values.
	table := []row{
		{4, 0.425532, 0.340426},
		{5, 0.476190, 0.380952},
		{6, 0.517241, 0.413793},
		{9, 0.604027, 0.483221},
		{10, 0.625000, 0.500000},
		{11, 0.643275, 0.500000},
	}
	for _, r := range table {
		nw := ptVisionMix(r.k)
		null := ptExpectedPonyShareDiskWeighted(nw)
		floor := math.Min(tarEdgeMajorityFloor, null*ptValidityMarginNum/ptValidityMarginDen)
		if math.Abs(null-r.wantNull) > 5e-7 || math.Abs(floor-r.wantFloor) > 5e-7 {
			t.Fatalf("k=%d (n=%d, %d:%d:1): null %.6f floor %.6f, want %.6f / %.6f. This table IS the boundary claim; re-run it, do not re-word it.",
				r.k, 101*r.k+1, 100*r.k, r.k, null, floor, r.wantNull, r.wantFloor)
		}
		t.Logf("k=%2d  n=%4d  %5d:%2d:1   null %.6f   (4/5)null %.6f   floor %.6f", r.k, 101*r.k+1, 100*r.k, r.k, null, null*4/5, floor)
	}
	// THE FIXED POINT, exactly -- and what this pins is the BOUNDARY, not the spelling of
	// the margin. Substituting the literal 0.80 for the 4/5 ratio leaves this assertion and
	// every other arm in the file GREEN (measured, by two blind seats and then by this one),
	// because both forms give bits 0x3fe0000000000000 here. What it DOES pin is that k = 10
	// is a true fixed point rather than a near-miss inside the table's 5e-7 tolerance, and it
	// moves with the tenet floor: at tarEdgeMajorityFloor = 0.51 this line reddens, as does
	// the k = 11 row above.
	null10 := ptExpectedPonyShareDiskWeighted(ptVisionMix(10))
	if null10 != 0.625 {
		t.Fatalf("the k=10 null is %.17g, want exactly 0.625: the fixed point is the claim", null10)
	}
	if got := null10 * ptValidityMarginNum / ptValidityMarginDen; got != tarEdgeMajorityFloor {
		t.Fatalf("(4/5)*0.625 = %.17g, want exactly %.17g. k=10 is meant to be an EXACT fixed point, so if this needs a tolerance either the null or the tenet floor has moved.", got, tarEdgeMajorityFloor)
	}
	for _, k := range []int{9, 11} {
		null := ptExpectedPonyShareDiskWeighted(ptVisionMix(k))
		if k == 9 && !(null*4/5 < tarEdgeMajorityFloor) {
			t.Fatalf("k=9 conditioned floor %.6f is not BELOW the flat floor; the fixed point at k=10 has moved", null*4/5)
		}
		if k == 11 && !(null*4/5 > tarEdgeMajorityFloor) {
			t.Fatalf("k=11 conditioned floor %.6f is not ABOVE the flat floor; the fixed point at k=10 has moved", null*4/5)
		}
	}

	// THE DRIVEN ARM. 550 ponies : 5 horses : 1 archival, serve work weighted 1 : 10 : 500
	// — which IS the disk weighting (0.1 : 1 : 50) scaled by ten, so this fixture is the
	// honest null realised. n = 556, below the k=10 boundary, and the two floors disagree:
	// the conditioned floor is 0.4000 while the bare tenet floor is 0.5000.
	honestSmall := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 550},
		{capTotal: c3HorseCap, served: 10 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: 500 * c3Unit, repairs: 10, n: 1},
	}
	conc, nw := c3Fixture(t, 46, honestSmall)
	r := ptEdgeMajorityGate(conc, nw)
	// 550 / (550 + 50 + 500) = 0.5 exactly, and it is exact in binary: both terms are
	// integer multiples of 2^30 well under 2^53.
	if r.Observed != 0.5 {
		t.Fatalf("the n=556 honest disk-weighted sample published %.17g, want exactly 0.5 — the boundary arm is built ON that equality", r.Observed)
	}
	if r.ExpectedNull != 0.5 {
		t.Fatalf("the null for this mix is %.17g, want exactly 0.5: the fixture's serve weighting IS the disk weighting scaled by ten, so the two must coincide", r.ExpectedNull)
	}
	if r.Floor != 0.4 {
		t.Fatalf("the conditioned floor at n=556 is %.17g, want 0.4 ((4/5)*0.5)", r.Floor)
	}
	if r.Verdict != ptPass {
		t.Fatalf("the HONEST disk-weighted sample at n=556 reads %s (%s). This is the case the validity condition exists for: a bare 0.50 floor false-fires here on a network doing exactly what the vision ratio says it should.", r.Verdict, r.Why)
	}
	t.Logf("driven n=556 honest disk-weighted: observed %.4f, null %.4f, conditioned floor %.4f -> %s. A BARE 0.50 floor would read EDGE-MINORITY on this honest sample.",
		r.Observed, r.ExpectedNull, r.Floor, r.Verdict)
	// And one step below: 548 ponies, where the bare floor and the conditioned floor give
	// OPPOSITE verdicts on an honest network. That disagreement is the condition's teeth.
	honestSmaller := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 548},
		{capTotal: c3HorseCap, served: 10 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: 500 * c3Unit, repairs: 10, n: 1},
	}
	conc2, nw2 := c3Fixture(t, 47, honestSmaller)
	r2 := ptEdgeMajorityGate(conc2, nw2)
	if !(r2.Observed < tarEdgeMajorityFloor) {
		t.Fatalf("the 548-pony arm published %.6f, which still clears the bare floor; the two floors do not disagree here and the arm has no subject", r2.Observed)
	}
	if r2.Verdict != ptPass {
		t.Fatalf("the 548-pony HONEST sample reads %s (%s) — the conditioned floor was supposed to protect it", r2.Verdict, r2.Why)
	}
	t.Logf("driven n=554 honest disk-weighted: observed %.6f — BELOW the bare 0.50 floor, ABOVE the conditioned floor %.4f. Bare: EDGE-MINORITY (false). Conditioned: %s.",
		r2.Observed, r2.Floor, r2.Verdict)

	// AND THE CONDITIONED FLOOR MUST BITE IN THE RED DIRECTION TOO, or the branch is
	// exercised only where it forgives (blind PE ruling N-3). Every other violation arm in
	// this file sits at k = 10, where the conditioned floor IS the flat 0.50, so none of them
	// reaches EDGE-MINORITY through the min(). This one does: a 500 : 5 : 1 mix has a null of
	// 0.4762 and a conditioned floor of 0.3810, and the edge tier serves 0.3333 of the bytes
	// — under the CONDITIONED floor, not merely under the flat one.
	belowTheConditionedFloor := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 500},
		{capTotal: c3HorseCap, served: 100 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: 500 * c3Unit, repairs: 10, n: 1},
	}
	conc3, nw3 := c3Fixture(t, 56, belowTheConditionedFloor)
	r3 := ptEdgeMajorityGate(conc3, nw3)
	if r3.Floor >= tarEdgeMajorityFloor {
		t.Fatalf("this arm's floor is %.6f, at or above the flat %.2f, so the verdict below does not come through the conditioned branch and the arm has no subject", r3.Floor, tarEdgeMajorityFloor)
	}
	if r3.Verdict != ptEdgeMinority {
		t.Fatalf("an edge share of %.4f against a CONDITIONED floor of %.4f reads %s (%s); the min() branch must be able to refuse and not only to forgive", r3.Observed, r3.Floor, r3.Verdict, r3.Why)
	}
	t.Logf("driven n=506 EDGE-MINORITY through the CONDITIONED floor: observed %.4f, null %.4f, floor %.4f (below the flat %.2f) -> %s",
		r3.Observed, r3.ExpectedNull, r3.Floor, tarEdgeMajorityFloor, r3.Verdict)
}

// ptVisionMix builds the mix of the vision-ratio family at scale k: 100k ponies, k horses,
// one archival node. Counts only — this feeds the NULL, which is arithmetic over the
// composition and needs no work counters.
func ptVisionMix(k int) c3NetworkWire {
	var nw c3NetworkWire
	nw.Mix = append(nw.Mix, struct {
		Class   string          `json:"class"`
		Sampled int             `json:"sampled"`
		Share   float64         `json:"share"`
		Work    *c3TierWorkWire `json:"work"`
	}{Class: node.TierPony, Sampled: 100 * k})
	nw.Mix = append(nw.Mix, struct {
		Class   string          `json:"class"`
		Sampled int             `json:"sampled"`
		Share   float64         `json:"share"`
		Work    *c3TierWorkWire `json:"work"`
	}{Class: node.TierHorse, Sampled: k})
	nw.Mix = append(nw.Mix, struct {
		Class   string          `json:"class"`
		Sampled int             `json:"sampled"`
		Share   float64         `json:"share"`
		Work    *c3TierWorkWire `json:"work"`
	}{Class: node.TierArchival, Sampled: 1})
	return nw
}

// TestGateC3_3d_RepairShareIsRelativeToHoldingsWhichIsWhatTheWithdrawnConstantCouldNotBe
// builds the replacement the Economist named when it withdrew repairGini <= 0.40 outright.
//
// WHY THE CONSTANT HAD TO GO, measured HERE rather than cited: repair is holdings-bound (a
// node repairs shards for the roots it holds), so under a holdings-proportional null the
// honest repair Gini is FAR above 0.40 — this fixture's honest arm publishes it, and the
// gate logs the number. A threshold that fires on the honest shape is not a threshold.
//
// THE REPLACEMENT, from advisory Finding 2b, and every term is measured from the sample:
//
//	expectedRepairShare(tier) = that tier's pledged share WITHIN the capable, reporting set
//	observedRepairShare(tier) = repairs(tier) / repairs(capable)
//	ALARM when observed - expected > margin, for any tier
//
// The margin 0.20 is the advisory's [ASSUMPTION], labelled as such there and owed a
// re-derivation on the first field data. It is not this seat's number and it is not
// asserted as a field alarm — this is a harness gate under D-WORK-VISIBILITY.
//
// CONTROLLED REVERT (G-PT-5): accumulate PledgedBytesByTier over every CLASSIFIABLE peer
// instead of every REPORTING peer (move the line above the M-2 return in
// core/node/economysample.go). MEASURED at both tiers: the five silent horses' pledge enters
// the expectation, the honest arm's excess moves from 0.0000 to +0.0547 on "archival", and
// core/node's TestR22PerTierTotalsFollowTheExclusionRuleAndNameTheirAbsences reddens on
// `PledgedBytesByTier[pony] = 12884901888, want 8589934592`.
func TestGateC3_3d_RepairShareIsRelativeToHoldingsWhichIsWhatTheWithdrawnConstantCouldNotBe(t *testing.T) {
	// HONEST, holdings-proportional: a 4 TiB archival node holds 64x a 64 GiB horse, so it
	// repairs 64x as much. Ponies serve but do no durability work (D-TIERING coupling (b)).
	//
	// FULL CAPABLE COVERAGE, and it is now a REQUIREMENT rather than an accident: the
	// coverage refusal in ptWorstRepairExcess declines to grade this arm otherwise. An earlier
	// draft carried five SILENT horses here so the G-PT-5 revert would be visible at this
	// tier; that made the arm undecidable under the refusal, and the honest reading is that a
	// partly silent capable tier cannot be graded at all. G-PT-5 is a core/node-tier defect
	// and core/node's own gate reddens on it -- an ablation belongs at the tier the defect
	// lives at, not at whichever tier can be contorted to see it.
	//
	// THE ARCHIVAL TIER HAS THREE NODES, not the vision ratio's one, and that is forced
	// rather than chosen: a tier share is published only over minGossipSample reporters
	// (TestGateC3_3h_ATierShareWithOneReporterIsThatPeersCounter), so a one-node archival tier
	// publishes NO share and drops out of the renormalisation entirely. Measured on the
	// one-node version, the honest excess reads -0.8649 instead of 0. That is the honest
	// consequence and not a fixture convenience: at the ratified 10000 : 100 : 1 target three
	// archival nodes need 30,000 ponies and maxPeerInfo is 4,096, so THE REPAIR ALARM IS
	// STRUCTURALLY DARK ON A VISION-RATIO SAMPLE. It grades a harness topology, which is what
	// D-WORK-VISIBILITY asks of it.
	honest := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 1, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 64, n: 3},
	}
	conc, nw := c3Fixture(t, 48, honest)
	if conc.RepairGini == nil || !conc.RepairGini.Known {
		t.Fatalf("the honest arm published no repair Gini (%+v); the comparison below needs it", conc.RepairGini)
	}
	t.Logf("HONEST holdings-proportional repair: the published repairGini is %.4f — the WITHDRAWN constant was 0.40, so an absolute threshold reads this healthy network as captured.",
		conc.RepairGini.val())
	if conc.RepairGini.val() <= c3RepairGiniMax {
		t.Fatalf("the holdings-proportional honest arm publishes repairGini %.4f, at or under the withdrawn %.2f. The withdrawal's premise was that this shape reads ABOVE it; if that has changed, re-open the withdrawal before relying on this gate.",
			conc.RepairGini.val(), c3RepairGiniMax)
	}
	// THE REPLACEMENT READING. Renormalise the published pledged shares onto the capable
	// set — they are published over ALL reporting peers, and the repair shares are
	// published over the capable reporters, so the two must be brought onto one population
	// before they are compared. That renormalisation is derivable from the wire alone,
	// which is the reason pledgedShare ships as a share rather than as a byte total.
	worst, worstTier, ok := ptWorstRepairExcess(t, nw)
	if !ok {
		t.Fatalf("the HONEST arm was refused for coverage on the %s tier; a graded arm must have full capable coverage or it measures nothing", worstTier)
	}
	t.Logf("HONEST: max(observed repair share - expected from holdings) = %+.4f on %q", worst, worstTier)
	if math.Abs(worst) > 1e-9 {
		t.Fatalf("on a repair distribution built EXACTLY proportional to holdings the excess is %+.6f on %q, not 0. The expectation and the observation are not over the same population.", worst, worstTier)
	}

	// CAPTURED: the same holdings, all the repair on one horse.
	captured := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 202, n: 1},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 0, n: 9},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 0, n: 3},
	}
	_, nwCap := c3Fixture(t, 49, captured)
	worstCap, worstCapTier, okCap := ptWorstRepairExcess(t, nwCap)
	if !okCap {
		t.Fatalf("the CAPTURED arm was refused for coverage on the %s tier; it is built at full coverage", worstCapTier)
	}
	t.Logf("CAPTURED: max(observed - expected) = %+.4f on %q, against the advisory's [ASSUMPTION] margin of 0.20", worstCap, worstCapTier)
	// WITHDRAWN as a margin (Economist addendum Part 2 §7.2(a)) and kept only as the constant
	// the two-arm straddle is asserted against. The NAME says so, because the promotion this
	// file most has to survive is a later seat reading it as a threshold.
	const ptWithdrawnMargin = 0.20
	if !(worstCap > ptWithdrawnMargin) {
		t.Fatalf("all the repair on one horse gives an excess of %+.4f on %q, which does not clear the %.2f margin. The relative reading has no teeth on the shape the absolute one was supposed to catch.", worstCap, worstCapTier, ptWithdrawnMargin)
	}
	if !(math.Abs(worst) < ptWithdrawnMargin) {
		t.Fatalf("the honest arm's excess %+.4f already clears the constant, so the two arms do not straddle it", worst)
	}

	// ADVERSARIAL SILENCE (Economist addendum Part 1 §4, second half). The SAME capture, with
	// the capturing horse withholding. It leaves BOTH terms and the remaining tiers
	// renormalise, so the alarm reads clean unless the coverage refusal declines to grade it.
	capturedMute := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 1}, // does 202 repairs, says nothing
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 0, n: 9},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 3},
	}
	_, nwMute := c3Fixture(t, 55, capturedMute)
	_, muteTier, okMute := ptWorstRepairExcess(t, nwMute)
	raw, rawTier := ptRawRepairExcess(t, nwMute)
	if okMute {
		t.Fatalf("a capable tier at partial coverage was GRADED: excess %+.4f on %q against a %.2f constant, while the horse that did 202 of 232 repairs said nothing. Its silence bought it the clean bill.", raw, rawTier, ptWithdrawnMargin)
	}
	t.Logf("ADVERSARIAL SILENCE: refused on the %q tier. Ungated, the same sample reads %+.4f on %q — under the WITHDRAWN %.2f, so NO ALARM — while the withholding horse did 202 of 232 repairs (0.8707 observed against a holdings expectation of 0.0448, a true excess of +0.8259).",
		muteTier, raw, rawTier, ptWithdrawnMargin)

	// THE INTRA-TIER ARM. Item 3 of ptWorstRepairExcess's doc block, DRIVEN rather than
	// asserted: route the whole horse tier's repairs onto ONE horse, everybody still
	// reporting. The tier statistic cannot move, because the tier's total did not. Nothing is
	// withheld, so no coverage refusal can see it.
	intraTier := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 1}, // one horse does the tier's ten
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 0, n: 9},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 64, n: 3},
	}
	_, nwIntra := c3Fixture(t, 57, intraTier)
	intraExcess, intraTierName, okIntra := ptWorstRepairExcess(t, nwIntra)
	if !okIntra {
		t.Fatalf("the intra-tier arm was refused on %q; it withholds NOTHING and must be graded, or it does not demonstrate the blind spot", intraTierName)
	}
	if intraExcess != worst {
		t.Fatalf("routing the whole horse tier's repairs onto one horse moved the tier excess to %+.6f, from %+.6f on the evenly-spread arm. If the tier statistic CAN see intra-tier concentration, item 3 of ptWorstRepairExcess's doc block is wrong and must be re-derived.", intraExcess, worst)
	}
	t.Logf("INTRA-TIER: ten horse repairs on ONE horse reads %+.4f on %q — IDENTICAL to the evenly-spread honest arm, at coverage 1.0000. A tier statistic cannot see within-tier capture, and no coverage refusal can, because nothing is withheld.",
		intraExcess, intraTierName)

	// THE CEILING, in the same breath as the separation (blind PE ruling N-1, Economist
	// addendum Part 2 §7.1). Item 1 of the doc block, measured on this fixture.
	var archExpected, capablePledged float64
	for _, r := range nw.Mix {
		if node.RepairCapable(r.Class) && r.Work != nil && r.Work.PledgedShare != nil && r.Work.PledgedShare.Known {
			capablePledged += r.Work.PledgedShare.val()
			if r.Class == node.TierArchival {
				archExpected = r.Work.PledgedShare.val()
			}
		}
	}
	archExpected /= capablePledged
	if ceiling := 1 - archExpected; ceiling >= ptWithdrawnMargin {
		t.Fatalf("the archival tier's expected share is %.4f, so its total-capture excess ceiling is %.4f, which CLEARS %.2f. Item 1 of the doc block has changed and must be re-derived.", archExpected, ceiling, ptWithdrawnMargin)
	} else {
		t.Logf("THE CEILING: the archival tier's holdings already predict %.4f of the repairs, so TOTAL capture by it reads at most %+.4f — under the WITHDRAWN %.2f and undetectable for ANY repair distribution. The normalisation N = (observed-expected)/(1-expected) is the specified cure and it is OWED.",
			archExpected, ceiling, ptWithdrawnMargin)
	}
}

// ptWorstRepairExcess is the Finding-2b statistic: the largest amount by which any capable
// tier's observed repair share exceeds the share its holdings predict. Both terms come off
// the SAME mix rows of the SAME document, so there is no join.
// WHAT THIS STATISTIC DOES NOT CATCH — the doc block Economist addendum Part 2 §7.5 requires
// before this merges, because the promotion path is short and inviting: a later seat reads a
// passing test and adds an "archival capture" arm to it. Every item is a MEASURED limit of
// the statistic, not of the fixtures.
//
//  1. THE EXCESS IS CAPPED AT 1 - expected(tier), so a tier whose holdings already predict
//     most of the repairs cannot travel far enough to trip an additive margin however
//     completely it captures the work. On the 1000 : 10 : 1 mix the ceilings are 0.8649
//     (horse) and 0.1351 (archival) against a 0.20 margin, a 6.40x spread in ONE sample --
//     so TOTAL ARCHIVAL CAPTURE READS +0.1351 AND PASSES, for ANY repair distribution
//     whatsoever. On THIS file's fixture, which carries three archival nodes so the tier
//     clears the reporting floor, it is worse: the ceiling is 0.0495, measured and logged by
//     the arm below. NO ARM ASSERTING ARCHIVAL CAPTURE MAY BE ADDED AGAINST THIS MARGIN. It
//     would not be a stronger gate; it would be a gate that cannot fire.
//
//  2. ON A SAMPLE WITH EXACTLY ONE CAPABLE TIER THE EXCESS IS IDENTICALLY 0.0000, for every
//     possible repair distribution: expected = observed = 1.0 by construction. That is not a
//     weak reading, it is no reading -- and the Economist puts it at ~59.5 % of honest
//     4,096-node draws, since a uniform 4,096-of-10,101 draw contains the single archival
//     node only 40.5 % of the time. The same single-archival integer floor that distorted the
//     Gini's null deletes this one.
//
//  3. IT IS A TIER STATISTIC WHERE THE CONSTANT IT REPLACED WAS A NODE STATISTIC, so
//     intra-tier capture is invisible to it AT FULL COVERAGE -- routing the whole horse
//     tier's repairs onto ONE horse reads +0.0000 at the tier. The coverage refusal below
//     closes the WITHHOLDING door and cannot close this one, because this attack withholds
//     nothing. DRIVEN by the intra-tier arm of
//     TestGateC3_3d_RepairShareIsRelativeToHoldingsWhichIsWhatTheWithdrawnConstantCouldNotBe
//     rather than asserted here.
//
//  4. THE 0.20 IS WITHDRAWN by Economist addendum Part 2 §7.2(a) -- withdrawn OUTRIGHT as an
//     additive margin, not re-priced downward, because an additive excess is
//     tier-incomparable and no value of it works. It survives in this file only as the
//     constant a TWO-ARM FIXTURE STRADDLE is asserted against (0.0000 honest, +0.9505
//     captured, both live), which is sound as a regression gate over named synthetic
//     distributions and unsound the moment it is read as a band. The replacement is the
//     NORMALISATION N = (observed - expected)/(1 - expected), which reads 1.0 for total
//     capture in EVERY tier and 0.0 for exactly-holdings, so any margin below 1.0 is
//     non-vacuous -- and it needs no data. OWED, not built here; see
//     docs/thinking/2026-09-09-per-tier-work-totals.md.
//
// AND THE LARGEST LIMIT, which is about the QUESTION and not the number: this statistic
// CONDITIONS ON HOLDINGS, so recentralization that lives IN THE HOLDINGS is invisible to it
// by construction. On this file's own honest fixture the archival tier holds 0.9505 of the
// repair-capable bytes before a single repair is routed, and nothing in this lane gates that.
// The Economist withdraws its own substitution on that ground: the raw repair Gini asked "is
// repair piling onto the few?" and this asks "is repair allocated other than by holdings?",
// and the second was adopted for having a convenient null of 0. Publishing the capable-set
// pledged concentration beside the excess is the specified follow-on; it is OWED, with no
// threshold, the Economist having no honest null for a pledged distribution.
//
// THE COVERAGE REFUSAL (Economist addendum Part 1 §4, second half). The same hole that
// defeats the edge share defeats this alarm and by the same mechanism: a wholly or partly
// silent capable tier leaves BOTH the observed numerator and the expected denominator, the
// remaining tiers renormalise, and the offending tier's own silence buys it a clean bill.
// Measured in the adversarial arm below: the horse that does 202 of 232 repairs withholds and
// the excess reads +0.0448, no alarm, where the truth is +0.8259.
//
// THE FLOOR HERE IS 1.0, and that is a deliberate difference from the edge share's derived
// floor. The edge share's floor falls out of the tenet floor as a theorem; this alarm's only
// tolerance is a WITHDRAWN assumption, and a coverage floor derived from an unfounded
// tolerance would inherit the assumption rather than answer it. Under D-WORK-VISIBILITY this
// is harness apparatus and the harness answer to coverage is 1.0 -- anything less is a
// fixture defect, the same discipline arms A1a and B3 already apply to the Gini. It is
// strictly stronger than any floor and costs no parameter.
func ptWorstRepairExcess(t *testing.T, nw c3NetworkWire) (float64, string, bool) {
	t.Helper()
	for _, r := range nw.Mix {
		if !node.RepairCapable(r.Class) || r.Work == nil {
			continue
		}
		if r.Work.Coverage != 1 {
			t.Logf("REFUSED: the %s tier reports %d of %d (coverage %.4f). A silent capable peer leaves BOTH terms of the excess and the rest renormalise, so the offending tier's silence would buy it a clean bill.",
				r.Class, r.Work.Reporting, r.Sampled, r.Work.Coverage)
			return 0, r.Class, false
		}
		// A capable tier that publishes no share is not a tier that contributed nothing --
		// it is a tier the renormalisation cannot see. Dropping it silently is the same
		// defect one level in.
		if r.Work.RepairShare == nil || !r.Work.RepairShare.Known || r.Work.PledgedShare == nil || !r.Work.PledgedShare.Known {
			t.Logf("REFUSED: the %s tier publishes no share (repair %+v, pledged %+v) over %d reporter(s); a capable tier the renormalisation cannot see is not a capable tier that did nothing.",
				r.Class, r.Work.RepairShare, r.Work.PledgedShare, r.Work.Reporting)
			return 0, r.Class, false
		}
	}
	var capablePledged float64
	for _, r := range nw.Mix {
		if node.RepairCapable(r.Class) && r.Work != nil && r.Work.PledgedShare != nil && r.Work.PledgedShare.Known {
			capablePledged += r.Work.PledgedShare.val()
		}
	}
	if capablePledged <= 0 {
		t.Fatalf("no capable tier published a known pledged share, so there is no expectation to compare against: %+v", nw.Mix)
	}
	worst, worstTier := math.Inf(-1), ""
	for _, r := range nw.Mix {
		if !node.RepairCapable(r.Class) || r.Work == nil || r.Work.RepairShare == nil || !r.Work.RepairShare.Known {
			continue
		}
		expected := r.Work.PledgedShare.val() / capablePledged
		if excess := r.Work.RepairShare.val() - expected; excess > worst {
			worst, worstTier = excess, r.Class
		}
	}
	if worstTier == "" {
		t.Fatalf("no capable tier published a known repair share: %+v", nw.Mix)
	}
	return worst, worstTier, true
}

// ptRawRepairExcess is ptWorstRepairExcess WITHOUT the refusals: what the alarm would read if
// it graded a sample it should decline. It exists so the adversarial arm can report the number
// the refusal suppressed, rather than asserting that suppressing it was worthwhile.
func ptRawRepairExcess(t *testing.T, nw c3NetworkWire) (float64, string) {
	t.Helper()
	var capablePledged float64
	for _, r := range nw.Mix {
		if node.RepairCapable(r.Class) && r.Work != nil && r.Work.PledgedShare != nil && r.Work.PledgedShare.Known {
			capablePledged += r.Work.PledgedShare.val()
		}
	}
	if capablePledged <= 0 {
		return 0, ""
	}
	worst, worstTier := math.Inf(-1), ""
	for _, r := range nw.Mix {
		if !node.RepairCapable(r.Class) || r.Work == nil || r.Work.RepairShare == nil || !r.Work.RepairShare.Known {
			continue
		}
		if excess := r.Work.RepairShare.val() - r.Work.PledgedShare.val()/capablePledged; excess > worst {
			worst, worstTier = excess, r.Class
		}
	}
	return worst, worstTier
}

// TestGateC3_3e_CapableSizeClosesTheCrossDocumentRepairCoverageJoin is advisory §1 Builder
// item 3.
//
// THE DEFECT: the repair series' reporting coverage was a ratio whose numerator came from
// /api/economy/concentration and whose denominator came from /api/economy/network — two
// separate s.nd.EconomySample() calls, two snapshots. In a fixture they agree. On a live
// node peerCaps moves between them, and a coverage ratio can then exceed 1, which is not a
// number the consumer has any way to interpret.
//
// THE FIX: economyConcentration carries capableSize itself, so the ratio is computable from
// one snapshot. This gate asserts the two agree, so a future edit that drifts them apart
// reddens rather than silently re-opening the join.
//
// IT ALSO PINS THE POPULATION RULE that decides which tiers get a repair share at all:
// RepairsByTier is keyed on node.RepairCapable and on nothing else, so a pony that reports
// repairs is still OUTSIDE the repair series' population — and its repair share is an
// undefined quantity rather than a small number.
//
// THREE CONTROLLED REVERTS, each ISOLATED so each names one condition:
//
//	G-PT-6  delete `out.CapableSize = &capable` from economyConcentrationDoc -> the join
//	        assertion reddens on `CapableSize:<nil>`.
//	G-PT-7a delete the node.RepairCapable test from tierWorkFor ONLY -> the reporting pony
//	        tier renders `&{Known:true Value:<nil>}` (a measured 0.0 repair share over a
//	        denominator it is not in). TestGateC3_3a_ATierWithNoReportingPeerIsANamedAbsenceNeverAZero
//	        reddens too, because the pony's repair refusal collapses into the
//	        no-reporters reason.
//	G-PT-7b key RepairsByTier for EVERY tier in core/node ONLY -> this gate stays GREEN,
//	        correctly: the renderer's own predicate still defends. core/node's
//	        TestR22PerTierTotalsFollowTheExclusionRuleAndNameTheirAbsences is what reddens.
//	        Two guards, and the isolated runs show each covers a different tier.
func TestGateC3_3e_CapableSizeClosesTheCrossDocumentRepairCoverageJoin(t *testing.T) {
	// A pony that reports repairs. It must never acquire a repair share: under D-TIERING
	// coupling (b) durability is the persistent tiers' work, so the repair series'
	// denominator excludes it and a share of a denominator it is not in is not a ratio.
	peers := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 7, n: 40},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 6},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
	conc, nw := c3Fixture(t, 50, peers)

	if conc.CapableSize == nil {
		t.Fatalf("the concentration document carries no capableSize. The repair series' coverage denominator then has to come off a SECOND EconomySample() call on a sibling route, and on a live node the two snapshots disagree:\n%+v", conc)
	}
	if *conc.CapableSize != nw.capable() {
		t.Fatalf("capableSize on /api/economy/concentration is %d while the tier mix on /api/economy/network sums to %d capable nodes. They are the same population read from the same sample; if they can disagree, the join is back.", *conc.CapableSize, nw.capable())
	}
	if *conc.CapableSize != 7 {
		t.Fatalf("capableSize is %d, want 7 (6 horses + 1 archival). It must count every CLASSIFIABLE capable peer, reporting or not — it is the repair series' POPULATION, not its sample.", *conc.CapableSize)
	}
	// The coverage the fix makes computable from ONE document.
	if conc.RepairGini == nil {
		t.Fatalf("no repair Gini on a fixture with 7 capable reporters; the coverage ratio has no numerator")
	}
	cov := float64(conc.RepairGini.SampleSize) / float64(*conc.CapableSize)
	if cov != 1 {
		t.Fatalf("repair coverage from ONE snapshot is %.4f, want 1.0000: every capable peer in this fixture reports", cov)
	}
	t.Logf("repair coverage %d/%d = %.4f, both terms off ONE document", conc.RepairGini.SampleSize, *conc.CapableSize, cov)

	// THE POPULATION RULE. The pony tier reported 7 repairs per node and must still have NO
	// repair share, with the scope reason rather than the no-reporters one.
	var pony, horse *c3TierWorkWire
	for _, row := range nw.Mix {
		switch row.Class {
		case node.TierPony:
			pony = row.Work
		case node.TierHorse:
			horse = row.Work
		}
	}
	if pony == nil || horse == nil {
		t.Fatalf("the fixture did not land: pony=%v horse=%v", pony, horse)
	}
	if pony.Reporting != 40 {
		t.Fatalf("the pony tier shows %d reporters, want 40 — this arm needs the ponies REPORTING for the scope refusal to be the thing under test", pony.Reporting)
	}
	if pony.RepairShare == nil || pony.RepairShare.Known || pony.RepairShare.Value != nil {
		t.Fatalf("a REPORTING pony tier acquired a repair share: %+v. Its 280 repairs are outside the repair series' denominator, so any ratio published here is over a population the tier is not a member of.", pony.RepairShare)
	}
	if horse.RepairShare == nil || !horse.RepairShare.Known {
		t.Fatalf("the horse tier has no repair share (%+v); the pony refusal above would then be indistinguishable from the feature being off", horse.RepairShare)
	}
	// 6 horses x 10 + 1 archival x 10 = 70 repairs in the capable series; horses did 60.
	if want := 60.0 / 70.0; math.Abs(horse.RepairShare.val()-want) > 1e-12 {
		t.Fatalf("horse repair share %.9f, want %.9f (60 of 70 capable repairs — the pony repairs are NOT in the denominator)", horse.RepairShare.val(), want)
	}
	t.Logf("population rule: 40 ponies reported 280 repairs and hold NO repair share (%q); the horse tier holds %.4f of the CAPABLE series' 70 repairs",
		pony.RepairShare.Reason, horse.RepairShare.val())
}

// ---- the two silence holes, and the two floors that close them -------------------------

// ptConcentratedPeersTop5Silent is ptConcentratedPeers with the FIVE HORSES DOING THE
// CONCENTRATING withholding their counters, plus `decoys` extra reporting horses the same
// operator runs IN ITS OWN BAND. Nothing else changes: same pledges from the concentrators,
// same true edge share, same everything the composition answer sees.
//
// THE DECOYS ARE THE PRICE OF DEFEATING THE COVERAGE FLOOR, and they are why this fixture is
// parameterised rather than fixed. phi is a COUNT ratio inside a band the adversary already
// populates, so adding a reporting node in that band raises it -- and the decoy also enlarges
// the tier, which drags the mix-conditioned floor DOWN. The attack is helped twice.
func ptConcentratedPeersTop5Silent(decoys int) []c3Peer {
	const ponies = 1000
	peers := []c3Peer{
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 5}, // the concentrating horses WITHHOLD
		{capTotal: c3PonyCap, served: (200_000 / ponies) * c3Unit, repairs: 0, n: ponies},
		{capTotal: c3HorseCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: 1},
	}
	if decoys > 0 {
		peers = append(peers, c3Peer{capTotal: c3HorseCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: decoys})
	}
	return peers
}

// ptTrueEdgeShare is the fixture's OWN assignment: what the edge tier really serves, decoys
// and silence included. Computed from the fixture's definition, never from a product figure —
// it is the ground truth the published number is compared against.
func ptTrueEdgeShare(decoys int) float64 {
	return 200_000.0 / float64(5*160_000+200_000+5*200+200+decoys*200)
}

// TestGateC3_3f_ThePerTierCoverageFloorIsATheoremOfTheTenetFloor pins the coverage floor the
// edge-share gate uses, and pins that it is DERIVED rather than chosen.
//
// THE DERIVATION. Under the named assumption (within a tier, silence is uncorrelated with
// work rate) the observation bounds the truth to [phi*s_obs, s_obs/phi], where phi is the
// least coverage among the tiers present. A PASS is the lower end clearing the floor F, and
// s_obs <= 1, so
//
//	PASS  =>  phi >= phi * s_obs  =>  phi >= F
//
// The minimum per-tier coverage a PASS requires IS the tenet floor. No second parameter, the
// same shape as the Gini's 1-T indeterminacy boundary being a theorem of T.
//
// THE ENDPOINTS ARE RUN, not read off the inequality. The fixture puts s_obs at exactly 1.0 —
// every served byte in the sample is a pony's — so phi alone decides and the boundary sits at
// phi = F = 0.50 exactly. The horse tier has ten members of which m report (serving zero and
// repairing, so they are REPORTING peers under M-2 and contribute a measured zero to the
// serve series). m = 4 / 5 / 6 straddles it.
//
// MEASURED, and this table is the claim:
//
//	m   phi    lower = phi * 1.0   F      verdict
//	4   0.4    0.4                 0.50   INDETERMINATE
//	5   0.5    0.5                 0.50   PASS          <- the boundary; 5/10 is exact in binary
//	6   0.6    0.6                 0.50   PASS
//
// CONTROLLED REVERT (G-PT-10, shared with TestGateC3_3g_TheConcentratingTierMustBuyDecoysToBuyAPass and TestGateC3_3b_EdgeSilenceDepressesTheEdgesOwnShare): change the PASS
// test in ptEdgeMajorityGate from `r.Lower >= r.Floor` to `r.Observed >= r.Floor`, dropping
// the interval and gating the raw observation. MEASURED, the m = 4 row then PASSES.
func TestGateC3_3f_ThePerTierCoverageFloorIsATheoremOfTheTenetFloor(t *testing.T) {
	type row struct {
		reporting int
		wantPhi   float64
		want      ptVerdict
	}
	for _, r := range []row{{4, 0.4, ptIndeterminate}, {5, 0.5, ptPass}, {6, 0.6, ptPass}} {
		peers := []c3Peer{
			// The edge tier holds every served byte, so s_obs is exactly 1.0 and phi is the
			// only term left. 1000 reporters clears the per-tier reporting floor.
			{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
			// Reporting horses serve NOTHING and repair, so they are in the series with a
			// measured zero. Silent horses report neither and are excluded.
			{capTotal: c3HorseCap, served: 0, repairs: 10, n: r.reporting},
			{capTotal: c3HorseCap, served: 0, repairs: 0, n: 10 - r.reporting},
			// ONE archival node, so the mix is the vision family at k = 10 and the
			// conditioned floor is exactly the flat 0.50. Three would drag the disk-weighted
			// null to 0.3846 and the floor to 0.3077 (measured), moving the boundary off
			// phi = F and making this table about the wrong thing.
			{capTotal: c3ArchivalCap, served: 0, repairs: 10, n: 1},
		}
		conc, nw := c3Fixture(t, int64(60+r.reporting), peers)
		got := ptEdgeMajorityGate(conc, nw)
		if got.Observed != 1 {
			t.Fatalf("m=%d: the edge share is %.9f, want exactly 1.0 — this fixture exists to take s_obs out of the comparison so phi alone decides", r.reporting, got.Observed)
		}
		if got.Floor != tarEdgeMajorityFloor {
			t.Fatalf("m=%d: the floor is %.6f, not the flat %.2f; the conditioned branch must be inert here or the boundary is not at phi = F", r.reporting, got.Floor, tarEdgeMajorityFloor)
		}
		if got.WorstTierCoverage != r.wantPhi || got.WorstTier != node.TierHorse {
			t.Fatalf("m=%d: phi = %.6f on %q, want %.6f on horse", r.reporting, got.WorstTierCoverage, got.WorstTier, r.wantPhi)
		}
		if got.Verdict != r.want {
			t.Fatalf("m=%d of 10 horses reporting: phi %.4f, lower bound %.4f against floor %.4f reads %s, want %s (%s)",
				r.reporting, got.WorstTierCoverage, got.Lower, got.Floor, got.Verdict, r.want, got.Why)
		}
		t.Logf("m=%2d  phi %.4f  lower %.4f  F %.4f  -> %s", r.reporting, got.WorstTierCoverage, got.Lower, got.Floor, got.Verdict)
	}
	// THE THEOREM ITSELF, as an equality rather than as the table: at the boundary row phi and
	// F coincide, and 5/10 is exact in binary so this is not a tolerance question.
	if 5.0/10.0 != tarEdgeMajorityFloor {
		t.Fatalf("the boundary fixture's phi is %.17g and the floor is %.17g; they must coincide EXACTLY for the m=5 row to be the boundary", 5.0/10.0, tarEdgeMajorityFloor)
	}
}

// TestGateC3_3g_TheConcentratingTierMustBuyDecoysToBuyAPass encodes the Economist's addendum
// Part 1 §4 attack permanently, AND the price of defeating the fix — because the fix does not
// close the attack, it prices it, and an earlier name for this gate ("CannotBuyAPass")
// asserted a universal that one node defeats.
//
// THE ATTACK COSTS NOTHING AT FIRST. The party doing the concentrating simply stops
// reporting, which is the compiled default (D-WORK-VISIBILITY). No Sybils, no forged
// counters, five nodes. MEASURED, before the per-tier coverage clause existed:
//
//	all report:        EDGE-MINORITY  edge share 0.1998  sample-wide coverage 1.0000
//	top-5 horses mute: PASS           edge share 0.9940  sample-wide coverage 0.9951
//
// The sample-wide floor is 0.85 and the silence is 0.49 % of the sample, so no count-weighted
// coverage clause can ever reach it. Under the ratified 10000:100:1 target the non-edge tiers
// ARE the sample's one percent — 1.088 % at 1000:10:1, 0.9999 % at 10000:100:1 — so the tiers
// whose silence matters are structurally invisible to a count-weighted measure, and the
// target ratio is what makes it so.
//
// AND HERE IS WHAT THE INTERVAL BUYS, AND WHAT IT COSTS THE ADVERSARY TO BUY BACK. The bound
// phi*s_obs <= s_true <= s_obs/phi is sound and its floor is a theorem, but its PURCHASE
// PRICE is the named assumption that within a tier silence is uncorrelated with work rate —
// and this attack is DEFINED by silencing the highest-work members of a tier, so it violates
// the assumption by construction. phi is a COUNT ratio inside a band the adversary already
// populates. MEASURED on this fixture family, and the arm below drives it:
//
//	decoys=0  horses=10 ( 5 reporting)  phi=0.5000  interval [0.4970, 1.9881]  floor=0.5000 -> INDETERMINATE  (TRUE 0.1998)
//	decoys=1  horses=11 ( 6 reporting)  phi=0.5455  interval [0.5417, 1.8206]  floor=0.4969 -> PASS           (TRUE 0.1997)
//	decoys=2  horses=12 ( 7 reporting)  phi=0.5833  interval [0.5787, 1.7007]  floor=0.4938 -> PASS           (TRUE 0.1997)
//	decoys=5  horses=15 (10 reporting)  phi=0.6667  interval [0.6594, 1.4837]  floor=0.4848 -> PASS           (TRUE 0.1996)
//
// ONE NODE. The d = 0 arm refuses by 0.0030 (lower bound 0.4970 against a floor of 0.5000),
// and a single decoy in the adversary's own band raises phi past the floor — while ALSO
// enlarging the tier, which drags the mix-conditioned floor down from 0.5000 to 0.4969. The
// attack is helped twice, and the true edge share never moves off 0.1997.
//
// SO THE HONEST STATEMENT IS "PRICED, NOT CLOSED", the same shape as the reporters floor's
// "parity, not closure" and the four limits on ptWorstRepairExcess. The interval defends
// against INNOCENT under-reporting, which is the common case, and against a deliberate
// silencer it costs one node per silenced band. That is a real price and it is not zero — the
// adversary must now run and maintain nodes in the band it is hiding in, and every one of them
// is a node it must keep reporting plausibly. It is not a closure and this gate does not
// claim one.
//
// CONTROLLED REVERT (G-PT-10): see
// TestGateC3_3f_ThePerTierCoverageFloorIsATheoremOfTheTenetFloor.
func TestGateC3_3g_TheConcentratingTierMustBuyDecoysToBuyAPass(t *testing.T) {
	concC, nwC := c3Fixture(t, 52, ptConcentratedPeers())
	rC := ptEdgeMajorityGate(concC, nwC)
	if rC.Verdict != ptEdgeMinority {
		t.Fatalf("the control arm reads %s; it must be EDGE-MINORITY or this gate has no subject", rC.Verdict)
	}

	// THE ATTACK AND ITS PRICE, driven together. d = 0 is what the coverage clause catches;
	// d = 1 is what it does not, and recording the second is the whole reason this gate was
	// renamed. Both are asserted, so if either verdict ever moves the limit must be re-priced
	// rather than silently re-described.
	type arm struct {
		decoys int
		want   ptVerdict
	}
	readings := map[int]ptReading{}
	for _, a := range []arm{{0, ptIndeterminate}, {1, ptPass}, {2, ptPass}} {
		conc, nw := c3Fixture(t, int64(52+10*a.decoys+1), ptConcentratedPeersTop5Silent(a.decoys))
		r := ptEdgeMajorityGate(conc, nw)
		readings[a.decoys] = r
		trueShare := ptTrueEdgeShare(a.decoys)
		t.Logf("decoys=%d  phi=%.4f  interval [%.4f, %.4f]  floor=%.4f -> %-13s (published %.4f, TRUE %.4f)",
			a.decoys, r.WorstTierCoverage, r.Lower, r.Upper, r.Floor, r.Verdict, r.Observed, trueShare)
		if trueShare >= tarEdgeMajorityFloor {
			t.Fatalf("decoys=%d: the TRUE edge share is %.4f, which is not a minority — the fixture stopped being an attack", a.decoys, trueShare)
		}
		if r.Verdict != a.want {
			t.Fatalf("decoys=%d reads %s at a published %.4f against a TRUE %.4f (phi %.4f, interval [%.4f, %.4f], floor %.4f), want %s.\n  d=0 is what the per-tier coverage clause CATCHES; d=1 is the price of defeating it. If either has moved, the limit recorded in this gate's doc block must be RE-PRICED, not re-worded.",
				a.decoys, r.Verdict, r.Observed, trueShare, r.WorstTierCoverage, r.Lower, r.Upper, r.Floor, a.want)
		}
	}

	// The margin the attack has to cross, stated as a number so nobody has to re-derive how
	// close d = 0 was. 0.4970 against 0.5000.
	r0, r1 := readings[0], readings[1]
	if margin := r0.Floor - r0.Lower; margin <= 0 || margin > 0.01 {
		t.Fatalf("the d=0 arm refuses by %.4f. This gate's doc block records 0.0030, and the whole reason one decoy suffices is that the margin is thin; if it has moved, re-measure the price before trusting the recorded table.", margin)
	} else {
		t.Logf("PRICED, NOT CLOSED: d=0 refuses by only %.4f (lower %.4f against floor %.4f). ONE decoy in the adversary's own band raises phi %.4f -> %.4f AND drags the floor %.4f -> %.4f, and the gate passes a network whose edge tier truly serves %.4f.",
			margin, r0.Lower, r0.Floor, r0.WorstTierCoverage, r1.WorstTierCoverage, r0.Floor, r1.Floor, ptTrueEdgeShare(1))
	}
	if !(r1.Floor < r0.Floor) {
		t.Fatalf("the decoy did NOT drag the conditioned floor down (%.6f then %.6f). The doc block says the attack is helped twice; if only one of the two effects is live, say so.", r0.Floor, r1.Floor)
	}

	// The sample-wide fraction the first version of this clause used. Asserted so the reason
	// the clause was replaced cannot rot into a claim nobody re-runs.
	concS, _ := c3Fixture(t, 53, ptConcentratedPeersTop5Silent(0))
	if concS.ServeGini == nil || concS.Sample == nil {
		t.Fatalf("the silent arm publishes no serve series; the sample-wide comparison below has no terms")
	}
	if wide := float64(concS.ServeGini.SampleSize) / float64(concS.Sample.Size); wide < c3ServeReportingMin {
		t.Fatalf("the sample-wide reporting fraction on the silent arm is %.4f, already BELOW the old %.2f clause — this arm exists because that clause CANNOT reach this attack, so if it now can, the finding must be re-derived", wide, c3ServeReportingMin)
	} else {
		t.Logf("the sample-wide fraction the old clause read is %.4f, clear of its %.2f floor: 5 silent nodes of %d is %.2f %% of the sample, and a count-weighted measure cannot see it",
			wide, c3ServeReportingMin, concS.Sample.Size, 100*5.0/float64(concS.Sample.Size))
	}
}

// TestGateC3_3h_ATierShareWithOneReporterIsThatPeersCounter is the second floor, and it is a
// DIFFERENT quantity from the coverage refusal above (blind PE ruling B-1).
//
// COVERAGE asks "how much of this tier is in the series" and defends the MEASUREMENT: a
// silent tier leaves the denominator and inflates everyone else. REPORTERS asks "how many
// peers is this figure over" and defends the PEERS: a tier share over one reporter IS that
// peer's counter. Neither implies the other — a tier of 1 node fully reporting has coverage
// 1.0 and one reporter, and a tier of 1,000 with 999 silent has one reporter too. Both floors
// are needed and this file drives both.
//
// THE INVERSION IS ONE DIVISION. The published share is R_t / SIGMA R; a reader that supplied
// every other term knows SIGMA R − R_t, so R_t = share · SIGMA R recovers the tier's TOTAL —
// and at one reporter that total is that peer's exact counter. MEASURED before the fix, on
// this exact fixture:
//
//	serveGini published?    false      <- suppressed by ITS OWN floor, on the same document
//	ponyShareOfServedBytes: known=true value=0.230769231 reporting=1
//	INVERSION: 1000/(1-share) = 1300   -> the single honest pony served exactly 300
//
// Strictly easier than the two-term Gini inversion that floor exists to stop, reaching the
// same reader it still defends.
//
// THE FLOOR IS minGossipSample UNCHANGED, AND THE REASON IS PARITY RATHER THAN INFORMATION.
// That constant's own derivation is exact for a GINI: at n = 1 the aggregate is the value, at
// n = 2 it inverts to two named peers' ratio, and 3 is the smallest sample where neither
// holds. It does NOT carry over exactly to a share -- R_t / SIGMA R resolves to a named peer
// at ANY n once the reader supplies the other n-1 terms, which is the whole threat model. So
// the share does not get a NEW answer from that derivation; it gets the same constant because
// the requirement is that this figure not be MORE exposed than the aggregate published two
// inches from it. Parity is the argument, and the next paragraph is what parity costs.
//
// AND IT BUYS PARITY, NOT CLOSURE: at three reporters an adversary holding two sybils IN THAT
// BAND still recovers the third, exactly as four sybils recover the Gini's secret
// (r22_gini_reconstruction_test.go). Reconstruction is closed by gossipWithheld. What this
// closes is the ASYMMETRY of one document suppressing serveGini at two reporters while
// publishing a figure that inverts to one.
//
// CONTROLLED REVERT (G-PT-13): delete the `reporters < minGossipSample` branch from
// tierShareOf and this gate reddens on the inversion above, with the recovered 300 in the
// failure message.
func TestGateC3_3h_ATierShareWithOneReporterIsThatPeersCounter(t *testing.T) {
	const secret, sybil = 300, 1000
	peers := []c3Peer{
		{capTotal: c3PonyCap, served: secret * c3Unit, repairs: 0, n: 1},
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 89},
		{capTotal: c3HorseCap, served: sybil * c3Unit, repairs: 0, n: 1},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 9},
	}
	conc, nw := c3Fixture(t, 54, peers)

	// THE CONTROL: the sibling aggregate on the same document IS dark here, by its own floor.
	// Without it this arm could pass on a document where nothing was suppressed, and the
	// asymmetry it is about would not exist.
	if conc.ServeGini != nil {
		t.Fatalf("serveGini is published over a 2-member series (%+v). minGossipSample=%d is what suppresses it, and this arm is about a NEW figure escaping the same floor on the same document.", conc.ServeGini, minGossipSample)
	}
	ps := conc.PonyShareOfServedBytes
	if ps == nil {
		t.Fatalf("the tenet figure vanished entirely; it must be PRESENT and named as below the floor")
	}
	if ps.Known {
		got := float64(sybil) / (1 - ps.val())
		t.Fatalf("ponyShareOfServedBytes is published as %.9f over %d reporter(s). A reader that supplied the other term inverts it in ONE DIVISION: %d/(1-share) = %.4f, so the single honest pony served exactly %.0f — the counter -privacy withholds, recovered from a document whose serveGini is dark for being over TWO peers.",
			ps.val(), ps.Reporting, sybil, got, got-float64(sybil))
	}
	if ps.Reason != belowTierReportingFloor {
		t.Fatalf("the tenet figure is unknown for the reason %q; at %d reporter(s) with work reported elsewhere the fact is the FLOOR, which is a different fact from having no reporter at all", ps.Reason, ps.Reporting)
	}
	if ps.Reporting != 1 {
		t.Fatalf("the fixture did not land: %d pony reporters, want 1", ps.Reporting)
	}
	for _, row := range nw.Mix {
		if row.Work == nil {
			t.Fatalf("mix row %s lost its work block", row.Class)
		}
		if row.Work.ServeShare == nil || row.Work.ServeShare.Known {
			t.Fatalf("mix[%s].work.serveShare is published as %.9f over %d reporter(s): the same inversion, on the other route",
				row.Class, row.Work.ServeShare.val(), row.Work.Reporting)
		}
	}
	r := ptEdgeMajorityGate(conc, nw)
	if r.Verdict != ptIndeterminate {
		t.Fatalf("a sample with one reporter per tier reads %s; it must be INDETERMINATE", r.Verdict)
	}
	t.Logf("one reporter per tier: serveGini dark, every tier share dark, gate %s", r.Verdict)
}
