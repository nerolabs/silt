package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Root-cause repro for the SYBILS=8 field FAIL: "5-sybil-no-capture CAPTURE —
// the Sybil cohort advanced the chain 26→37 with ALL anchors down." The cloud
// topology: 4 anchor-validators (in cfg.Anchors) + 8 non-anchor Sybils, every
// Sybil bond declaring the SAME domain ("sybilnet"), mature-validators=4.
//
// The intended C2 defense: C2Metric counts only NON-anchor committed bonds
// (the Sybils), and same-domain bonds AGGREGATE into one address-diversity
// group, so NakamotoDomains stays at 1 ≪ 4 → the network never matures →
// the launch-anchor training wheels stay engaged → a no-anchor commit is
// refused (ErrAnchorRequired). If that holds, the Sybils CANNOT capture.
//
// This test builds that exact committed bonded state directly and asks the two
// load-bearing questions: (1) does the network report Mature()? (2) can a
// no-anchor Sybil quorum satisfy the commit gate? A capture requires maturity
// (anchors shed) — so if Mature() is FALSE, the anchor gate must hold and the
// field FAIL is NOT a C2 break in the metric; if Mature() is TRUE, the
// single-domain discount failed and it IS.
func TestC2SingleDomainSybilsDoNotMature(t *testing.T) {
	const bond = int64(64) << 20
	anchors := map[ports.NodeID]bool{}
	var anchorIDs []ports.NodeID
	for i := 0; i < 4; i++ {
		id := idOf(key(int64(9000 + i)))
		anchors[id] = true
		anchorIDs = append(anchorIDs, id)
	}
	cfg := Config{
		Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 4,
		OperatorMargin: 2, // the untrusted-objective default (DerivedOperatorMargin)
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// The committed state after the drain banked everything: the 4 anchors hold
	// bonds (excluded from C2Metric as anchors), and 8 non-anchor Sybils each hold
	// a 64 MiB bond ALL in the single declared domain "sybilnet". Every one has
	// attested a committed block (validatorsSeen), which is what C2Metric counts.
	const sybilDomain = uint64(0x5ceb11) // one shared domain hash for all Sybils
	for _, aid := range anchorIDs {
		c.bonded[aid] = bond
		c.validatorsSeen[aid] = true
	}
	var sybilIDs []ports.NodeID
	for i := 0; i < 8; i++ {
		sid := idOf(key(int64(9100 + i)))
		sybilIDs = append(sybilIDs, sid)
		c.bonded[sid] = bond
		c.bondDomain[sid] = sybilDomain
		c.validatorsSeen[sid] = true
	}

	m := c.C2Metric()
	t.Logf("C2: NakamotoBonds=%d NakamotoOperators=%d NakamotoDomains=%d DistinctDomains=%d Participants=%d Margin=%d",
		m.NakamotoBonds, m.NakamotoOperators, m.NakamotoDomains, m.DistinctDomains, m.Participants, m.Margin)

	if m.DistinctDomains != 1 {
		t.Errorf("8 same-domain Sybils must aggregate to ONE address-diversity group, got DistinctDomains=%d "+
			"(if >1, the domain is not being committed/counted — the discount is bypassed)", m.DistinctDomains)
	}
	if m.NakamotoDomains >= cfg.MatureValidators {
		t.Errorf("NakamotoDomains=%d ≥ MatureValidators=%d — the single-domain discount FAILED to bound maturity",
			m.NakamotoDomains, cfg.MatureValidators)
	}
	if c.Mature() {
		t.Fatalf("C2 BREAK: an 8-Sybil single-domain cohort (+4 anchors) reports Mature()=true — the network "+
			"would shed its launch anchors and the Sybils could capture (the field FAIL). C2 metric: %+v", m)
	}
	t.Logf("Mature()=%v EverMature()=%v — training wheels %s",
		c.Mature(), c.EverMature(), map[bool]string{true: "SHED", false: "ENGAGED"}[c.Mature()])

	// The other load-bearing leg: on this young (never-matured) network, a block
	// attested ONLY by Sybils (no anchor sign-off) must be REFUSED by the training
	// wheels — so even if the Sybils gather their own quorum with the anchors down,
	// they cannot commit. This is what makes a real capture impossible here; the
	// field FAIL must therefore be a harness artifact (a lagging Sybil catching up
	// to anchor-committed blocks read as an "advance"), not a genuine capture.
	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, key(9000))
	c2 := New(cfg, func(ports.NodeID) int64 { return 0 })
	c2.SetBondVerifier(objectiveVerify)
	if err := c2.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	// Seed the same young bonded state into a real replica by committing an
	// anchor-attested block that banks the Sybil bonds, then attempt a no-anchor
	// Sybil-quorum block on top.
	// Bank THREE Sybil bonds (all same domain) so the Sybils can more than meet
	// RequiredQuorum(2) among themselves — exactly the cloud case (8 Sybils).
	s1, s2, s3 := key(9100), key(9101), key(9102)
	b1 := &Block{Version: BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{
			bondRegDom(s1, bond, g.Hash(), sybilDomain),
			bondRegDom(s2, bond, g.Hash(), sybilDomain),
			bondRegDom(s3, bond, g.Hash(), sybilDomain),
		}}
	Sign(b1, key(9000))
	b1.Atts = []Attestation{Attest(b1, key(9001)), Attest(b1, key(9002))} // anchors co-sign (young-network gate satisfied)
	if err := c2.Append(*b1); err != nil {
		t.Fatalf("setup: anchor-attested drain block should commit: %v", err)
	}
	// The capture attempt: a Sybil proposes, and TWO other Sybils attest — enough
	// to satisfy RequiredQuorum(2), so the ONLY thing that can refuse it is the
	// anchor training-wheels gate. With the anchors down there is no anchor
	// sign-off, so it must be refused with ErrAnchorRequired.
	b2 := &Block{Version: BlockVersion, Height: 2, Prev: b1.Hash(), Entries: []ports.Entry{entry(2)}}
	Sign(b2, s1)
	b2.Atts = []Attestation{Attest(b2, s2), Attest(b2, s3)}
	err := c2.Append(*b2)
	if err == nil {
		t.Fatal("C2 BREAK: a no-anchor Sybil-quorum block committed on a YOUNG network — the anchor gate did not hold")
	}
	t.Logf("young-network no-anchor Sybil-quorum commit correctly refused: %v", err)
}
