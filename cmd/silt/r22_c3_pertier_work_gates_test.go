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
	// IT IS A RATIO AND NOT THE LITERAL 0.80 FOR A MEASURED REASON. The validity boundary
	// is a FIXED POINT: at a disk-weighted null of exactly 0.625 the conditioned floor
	// equals the flat floor exactly. 0.8 is not representable in binary, so 0.8*0.625
	// lands within half an ulp of 0.5 and whether it rounds to 0.5 is a property of the
	// rounding mode rather than of the economics. 0.625*4/5 is exact (2.5/5), so the
	// boundary row of the table below is decided by the arithmetic it claims to be
	// decided by. TestGateC3_3c_TheValidityBoundaryIsMeasuredNotDerivedFromTheComparison
	// drives that row.
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
	Verdict        ptVerdict
	Why            string
	Observed       float64 // the published ponyShareOfServedBytes
	TierCoverage   float64 // reporting ponies / sampled ponies
	SeriesCoverage float64 // the serve series' reporting fraction of the whole sample
	ExpectedNull   float64 // the disk-weighted honest share for THIS sampled mix
	Floor          float64 // min(tarEdgeMajorityFloor, 4/5 * ExpectedNull)
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
// THE COVERAGE CLAUSE REUSES c3ServeReportingMin AND INTRODUCES NO NEW PARAMETER. It is
// the SAME serve series over the SAME population, so it inherits that series' own
// indeterminacy boundary (1 - c3ServeGiniMax = 0.85, a theorem of the tolerance, pinned by
// TestGateC3_IndeterminacyBoundaryIsATheoremOfTheTolerance).
//
// AND HERE IS WHY THE COVERAGE CLAUSE CANNOT BE DERIVED THE WAY THE GINI'S WAS, which is a
// correction to the advisory's framing and this seat measured it rather than assuming it.
// For the Gini, silence-means-idle yields a two-sided identity, G_adj = (1-c) + c*G_pub.
// For a tier SHARE it does not: the true share is (P + P_s) / (P + P_s + O + O_s), and O_s
// -- work done by silent NON-pony peers -- is unbounded above, so the observed share is
// neither an upper nor a lower bound on the truth in general. What IS derivable, and is
// asserted by TestGateC3_3b_EdgeSilenceDepressesTheEdgesOwnShare, is the ONE-TIER case: if only ponies go silent, the published
// share falls, because (P-d)/(P+O-d) < P/(P+O) for d,O > 0. So the honest statement is
// "this is a share over the REPORTING population", and coverage is published so the reader
// can see how much of the population that is.
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
	// The serve series' own coverage, off the two fields the panel already publishes.
	if conc.ServeGini == nil || !conc.ServeGini.Known || conc.Sample.Size <= 0 {
		r.Verdict, r.Why = ptIndeterminate, "the serve series is dark, so the share is over a population of unknown size"
		return r
	}
	r.SeriesCoverage = float64(conc.ServeGini.SampleSize) / float64(conc.Sample.Size)
	if r.SeriesCoverage < c3ServeReportingMin {
		r.Verdict = ptIndeterminate
		r.Why = "the serve series covers " + ftoa(r.SeriesCoverage) + " of the sample, below the boundary " +
			ftoa(c3ServeReportingMin) + ": a tier share over a minority of the population is not a reading of that population"
		return r
	}
	if r.Observed < r.Floor {
		r.Verdict = ptEdgeMinority
		r.Why = "the edge tier serves " + ftoa(r.Observed) + " of the reported bytes, below the floor " + ftoa(r.Floor) +
			" that this sampled mix justifies (honest disk-weighted null " + ftoa(r.ExpectedNull) + ")"
		return r
	}
	r.Verdict = ptPass
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
		rH.Verdict, rH.Observed, rH.Floor, rH.ExpectedNull, rH.TierCoverage, rH.SeriesCoverage)

	// The tenet figure must be a MEASUREMENT and not merely present.
	if !concH.PonyShareOfServedBytes.Known || concH.PonyShareOfServedBytes.Value == nil {
		t.Fatalf("HEALTHY ARM: ponyShareOfServedBytes is not a measurement (%+v)", concH.PonyShareOfServedBytes)
	}
	// 1000 ponies at 1 unit, 10 horses at 7, 1 archival at 24: 1000/1094.
	if want := 1000.0 / 1094.0; math.Abs(rH.Observed-want) > 1e-12 {
		t.Fatalf("HEALTHY ARM: the published edge share is %.9f, want %.9f (1000 pony-units of 1094 total). The figure is not the sum this fixture built.", rH.Observed, want)
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
// CONTROLLED REVERT (G-PT-3): scale the pony numerator by population/reporting — the obvious
// "correct for coverage" edit. MEASURED: arm B's share goes to 0.694444444 against the built
// 0.347222222, so the extrapolation makes edge silence RAISE the edge's share. That is the
// false-clean-bill direction, and it is why this figure is published raw with its coverage
// beside it rather than extrapolated.
func TestGateC3_3b_EdgeSilenceDepressesTheEdgesOwnShare(t *testing.T) {
	full := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 100},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
	// The SAME network with half the edge tier withholding. Nothing about the work changed;
	// only what was said about it.
	half := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 50},
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 50},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
	concA, nwA := c3Fixture(t, 44, full)
	concB, nwB := c3Fixture(t, 45, half)
	rA, rB := ptEdgeMajorityGate(concA, nwA), ptEdgeMajorityGate(concB, nwB)

	t.Logf("full reporting: %s share %.4f (tier coverage %.4f, series coverage %.4f) | half the edge silent: %s share %.4f (tier coverage %.4f, series coverage %.4f)",
		rA.Verdict, rA.Observed, rA.TierCoverage, rA.SeriesCoverage, rB.Verdict, rB.Observed, rB.TierCoverage, rB.SeriesCoverage)

	if rA.Verdict != ptPass {
		t.Fatalf("the fully-reporting arm reads %s (%s); it is the control and must be a measurement", rA.Verdict, rA.Why)
	}
	if concB.PonyShareOfServedBytes == nil || !concB.PonyShareOfServedBytes.Known {
		t.Fatalf("arm B publishes no measured edge share (%+v); half the edge tier IS reporting, so the figure exists", concB.PonyShareOfServedBytes)
	}
	// 100/194 = 0.515464 with everyone reporting; 50/144 = 0.347222 with half silent.
	if wantA, wantB := 100.0/194.0, 50.0/144.0; math.Abs(rA.Observed-wantA) > 1e-12 || math.Abs(concB.PonyShareOfServedBytes.val()-wantB) > 1e-12 {
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
	if rB.Observed >= tarEdgeMajorityFloor {
		t.Fatalf("arm B's share %.4f still clears the flat floor, so the INDETERMINATE verdict above is not being reached through the case this gate is about. Silence more of the edge tier.", rB.Observed)
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
// CONTROLLED REVERT (G-PT-4): change ptDiskWeightArchival from 50 to 49. MEASURED, the table
// reddens on its FIRST row — `k=4 (n=405, 400:4:1): null 0.430108 floor 0.344086, want
// 0.425532 / 0.340426` — and the k=10 fixed point moves with it. Literal expected values are
// the only teeth a fixed point can have: there is no mutation to ablate at the boundary
// itself, so the gate must BE the table and never `want := theFunctionUnderTest(...)`.
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
	// THE FIXED POINT, exactly. k=10 is where the conditioned floor becomes the flat tenet
	// floor, and one step either side is on the other side of that statement.
	null10 := ptExpectedPonyShareDiskWeighted(ptVisionMix(10))
	if null10 != 0.625 {
		t.Fatalf("the k=10 null is %.17g, want exactly 0.625: the fixed point is the claim", null10)
	}
	if got := null10 * ptValidityMarginNum / ptValidityMarginDen; got != tarEdgeMajorityFloor {
		t.Fatalf("(4/5)*0.625 = %.17g, want exactly %.17g. If this needs a tolerance the margin has stopped being a ratio and the boundary row is decided by a rounding.", got, tarEdgeMajorityFloor)
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
	// AND THE FIXTURE CARRIES FIVE SILENT HORSES, which is not decoration. Without them
	// every classifiable peer is also a reporting peer, the two populations coincide, and
	// the gate cannot see whether the expectation is computed over the reporting set or over
	// the whole sample. Measured: with full coverage the G-PT-5 revert leaves this gate
	// GREEN; with the silent horses in it the honest arm's excess moves off 0 to +0.0547.
	// A fully-covered fixture is an excused row.
	honest := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 1, n: 10},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 5}, // classifiable, capable, SILENT
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 64, n: 1},
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
	worst, worstTier := ptWorstRepairExcess(t, nw)
	t.Logf("HONEST: max(observed repair share - expected from holdings) = %+.4f on %q", worst, worstTier)
	if math.Abs(worst) > 1e-9 {
		t.Fatalf("on a repair distribution built EXACTLY proportional to holdings the excess is %+.6f on %q, not 0. The expectation and the observation are not over the same population.", worst, worstTier)
	}

	// CAPTURED: the same holdings, all the repair on one horse.
	captured := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 74, n: 1},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 0, n: 9},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 0, n: 1},
	}
	_, nwCap := c3Fixture(t, 49, captured)
	worstCap, worstCapTier := ptWorstRepairExcess(t, nwCap)
	t.Logf("CAPTURED: max(observed - expected) = %+.4f on %q, against the advisory's [ASSUMPTION] margin of 0.20", worstCap, worstCapTier)
	const advisoryMargin = 0.20
	if !(worstCap > advisoryMargin) {
		t.Fatalf("all the repair on one horse gives an excess of %+.4f on %q, which does not clear the %.2f margin. The relative reading has no teeth on the shape the absolute one was supposed to catch.", worstCap, worstCapTier, advisoryMargin)
	}
	if !(math.Abs(worst) < advisoryMargin) {
		t.Fatalf("the honest arm's excess %+.4f already clears the margin, so the two arms do not straddle it", worst)
	}
}

// ptWorstRepairExcess is the Finding-2b statistic: the largest amount by which any capable
// tier's observed repair share exceeds the share its holdings predict. Both terms come off
// the SAME mix rows of the SAME document, so there is no join.
func ptWorstRepairExcess(t *testing.T, nw c3NetworkWire) (float64, string) {
	t.Helper()
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
