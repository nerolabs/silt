package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Premise repro for the MATURING=1 SYBILS=8 field drill (10-maturing-handoff):
// the everMature latch is UNREACHABLE in that topology as parameterized, even
// with a perfect bond-reg drain — so the "latch never tripped in 420s" GAP
// (runs 9c3777d-73949, 7134711-18163) would recur on any re-run regardless of
// the #427 drain fix.
//
// The topology (integration/cloudtest/topology.py, MATURING=1): 4 validators
// which are ALL launch anchors (-anchors lists every validator), bar
// -mature-validators 2 at -operator-margin 1, plus 8 non-anchor Sybils bonding
// the MINIMUM (1 MiB) and all declaring ONE shared -domain sybilnet. The
// topology comment expects the latch to trip on "the coefficient the 4
// distinct-operator validators actually reach (2)". That model is wrong twice:
//
//  1. C2Metric EXCLUDES anchors (chain.go: `if c.cfg.Anchors[id] { continue }`)
//     — correctly, because the shed measures decentralization AWAY from the
//     scaffolding; counting the anchors' own bonds to shed the anchors would
//     be circular. So the 4 validators' 64 MiB bonds never enter the metric.
//  2. The only non-anchor cohort is the single-domain Sybil set, which the A
//     axis aggregates into ONE group → NakamotoDomains = 1 < 2, and
//     matureNow gates on min(NakamotoOperators, NakamotoDomains). That is the
//     C2 discount WORKING (TestC2SingleDomainSybilsDoNotMature pins it as the
//     defense) — the drill asks the metric to be tripped by exactly the cohort
//     the metric exists to refuse.
//
// (A third, independent blocker in the live topology: validatorsSeen fills
// only from ATTESTERS of committed blocks, and pre-shed the anchors gather
// only from -attesters = the other anchors — so the Sybils would not even be
// "seen". This repro GRANTS them seen status to show the arithmetic alone is
// decisive.)
//
// The intended maturation shape — honest NON-anchor validators with DISTINCT
// domains who ATTEST committed blocks — is the I3 oracle's setup
// (modelcheck_i3_test.go matureWeightedEpoch); the field topology has no such
// cohort. Fixing the drill is a harness/topology change (route via PE — the
// drill parameterization was a reviewed ruling), not a core change: the core
// behavior this test pins is CORRECT.
func TestMaturingFieldTopologyLatchUnreachable(t *testing.T) {
	const anchorBond = int64(64) << 20 // -bond 64M
	const sybilBond = int64(1) << 20   // MATURING: syb_bond = min_bond = 1M
	anchors := map[ports.NodeID]bool{}
	var anchorIDs []ports.NodeID
	for i := 0; i < 4; i++ {
		id := idOf(key(int64(9500 + i)))
		anchors[id] = true
		anchorIDs = append(anchorIDs, id)
	}
	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 2, OperatorMargin: 1, // the MATURING drill parameterization
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// The best case a perfect drain can ever reach: every bond banked, and every
	// participant — anchors AND Sybils — generously counted as seen attesters.
	const sybilDomain = uint64(0x5b17ce7) // the one shared -domain sybilnet
	for _, aid := range anchorIDs {
		c.bonded[aid] = anchorBond
		c.validatorsSeen[aid] = true
	}
	for i := 0; i < 8; i++ {
		sid := idOf(key(int64(9600 + i)))
		c.bonded[sid] = sybilBond
		c.bondDomain[sid] = sybilDomain
		c.validatorsSeen[sid] = true
	}

	m := c.C2Metric()
	t.Logf("C2 at full drain: NakamotoBonds=%d NakamotoOperators=%d NakamotoDomains=%d DistinctDomains=%d Participants=%d Margin=%d",
		m.NakamotoBonds, m.NakamotoOperators, m.NakamotoDomains, m.DistinctDomains, m.Participants, m.Margin)

	// The anchors' bonds must not be what matures the network (the shed would be
	// circular). If Participants counts more than the 8 Sybils, the exclusion moved.
	if m.Participants != 8 {
		t.Errorf("C2Metric must count only the 8 non-anchor Sybil bonds, got Participants=%d", m.Participants)
	}
	// The single-domain cohort aggregates to one address-diversity group.
	if m.NakamotoDomains != 1 {
		t.Errorf("8 same-domain Sybils must yield NakamotoDomains=1, got %d", m.NakamotoDomains)
	}
	// The drill's load-bearing premise: with EVERY bond banked and EVERY
	// participant seen, the bar-2 latch still cannot trip. If this ever flips to
	// Mature()=true, either the topology gained an honest non-anchor cohort (fix
	// this test's setup to match) or the C2 discount regressed (a real break —
	// see TestC2SingleDomainSybilsDoNotMature).
	if c.Mature() {
		t.Fatalf("MATURING topology premise: Mature()=true at full drain — the single-domain "+
			"discount or the anchor exclusion regressed; C2: %+v", m)
	}
	t.Logf("Mature()=false at FULL drain — the 10-maturing-handoff latch premise is unreachable in this topology; " +
		"the drill needs an honest non-anchor distinct-domain attesting cohort (the I3 oracle shape)")
}

// The re-split's reachability half (PE concurrence 2026-08-15): with the 8 cohort
// slots split into 4 honest maturers (full 64M bond, UNSET domain — each an
// independent address-diversity group) + 4 single-domain MinBond Sybils, the
// bar-2 latch IS reachable at full drain — min(NakamotoOperators,
// NakamotoDomains) = 2 — while the Sybil cohort alone still cannot mature it
// (their 4 MiB single group is nowhere near the ⅓ threshold). This is the
// deterministic RED/GREEN home for the 10-maturing-handoff drill premise: the
// field run confirms on the wire what this asserts on a laptop.
func TestMaturingResplitTopologyLatchReachable(t *testing.T) {
	const maturerBond = int64(64) << 20 // -bond 64M, no -domain
	const sybilBond = int64(1) << 20    // MinBond, all -domain sybilnet
	anchors := map[ports.NodeID]bool{}
	var anchorIDs []ports.NodeID
	for i := 0; i < 4; i++ {
		id := idOf(key(int64(9700 + i)))
		anchors[id] = true
		anchorIDs = append(anchorIDs, id)
	}
	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 2, OperatorMargin: 1,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	const sybilDomain = uint64(0x5b17ce7)
	for _, aid := range anchorIDs {
		c.bonded[aid] = maturerBond
		c.validatorsSeen[aid] = true
	}
	for i := 0; i < 4; i++ { // maturers: full bond, domain UNSET (independent)
		mid := idOf(key(int64(9800 + i)))
		c.bonded[mid] = maturerBond
		c.validatorsSeen[mid] = true
	}
	for i := 0; i < 4; i++ { // sybils: MinBond, one shared domain
		sid := idOf(key(int64(9900 + i)))
		c.bonded[sid] = sybilBond
		c.bondDomain[sid] = sybilDomain
		c.validatorsSeen[sid] = true
	}

	m := c.C2Metric()
	t.Logf("C2 at full drain (re-split): NakamotoBonds=%d NakamotoOperators=%d NakamotoDomains=%d DistinctDomains=%d Participants=%d",
		m.NakamotoBonds, m.NakamotoOperators, m.NakamotoDomains, m.DistinctDomains, m.Participants)

	if m.NakamotoOperators < cfg.MatureValidators || m.NakamotoDomains < cfg.MatureValidators {
		t.Fatalf("re-split premise: min(operators=%d, domains=%d) must reach the bar %d at full drain",
			m.NakamotoOperators, m.NakamotoDomains, cfg.MatureValidators)
	}
	if !c.Mature() {
		t.Fatalf("re-split premise: Mature() must be true at full drain (the field drill's latch depends on it); C2: %+v", m)
	}

	// The Sybil half of the drill still holds: the cheap single-domain cohort
	// ALONE (maturers not yet drained) must NOT mature the network.
	c2 := New(cfg, func(ports.NodeID) int64 { return 0 })
	c2.SetBondVerifier(objectiveVerify)
	for _, aid := range anchorIDs {
		c2.bonded[aid] = maturerBond
		c2.validatorsSeen[aid] = true
	}
	for i := 0; i < 4; i++ {
		sid := idOf(key(int64(9900 + i)))
		c2.bonded[sid] = sybilBond
		c2.bondDomain[sid] = sybilDomain
		c2.validatorsSeen[sid] = true
	}
	if c2.Mature() {
		t.Fatalf("re-split premise: the 4-Sybil single-domain cohort alone must NOT mature the network; C2: %+v", c2.C2Metric())
	}
	t.Logf("Mature()=true with the maturer cohort drained; false with the Sybil cohort alone — the drill premise is reachable AND still refuses the cheap cohort")
}
