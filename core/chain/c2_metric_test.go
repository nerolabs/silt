package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// C2 metric wiring (#185 / D-C2). The concentration measurement is computed from
// the COMMITTED on-chain bond ledger (never gossip — that kills the skew half),
// and discounted by the operator margin M to bound the split half. These pin the
// arithmetic and the split-resistant shed.

type c2Bond struct {
	priv ed25519.PrivateKey
	size int64
}

// buildC2 builds an objective chain (two anchors bootstrap it) with
// MatureValidators=k and OperatorMargin=m, seeds `bonds` at genesis, and commits
// one block per bond so each enters the participating (attested) set.
func buildC2(t *testing.T, k, m int, bonds []c2Bond) *Chain {
	t.Helper()
	const minBond = int64(1) << 20
	a1, a2 := key(1), key(2)
	cfg := Config{
		Quorum: 2, MinBond: minBond, MinProposerRep: 0, MinAttesterRep: 0,
		Anchors:          map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true},
		AnchorQuorum:     1,
		MatureValidators: k,
		OperatorMargin:   m,
	}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	regs := []BondReg{bondReg(a1, minBond, ports.Hash{}), bondReg(a2, minBond, ports.Hash{})}
	for _, b := range bonds {
		regs = append(regs, bondReg(b.priv, b.size, ports.Hash{}))
	}
	g := &Block{Version: BlockVersion, Height: 0, Entries: []ports.Entry{entry(0)}, BondRegs: regs}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	prev := g
	for _, b := range bonds {
		prev = appendCommit(t, c, a1, prev, a2, b.priv) // anchor a2 + the bond's own key
	}
	return c
}

// TestC2Metric_SkewReportsCostToCorrupt: over a set dominated by one whale, the
// coefficient is 1 (weight-aware, not a head-count), and the metric reports the
// participating weight and the cost-to-corrupt (⌊total/3⌋+1) taken straight from
// the committed bonds.
func TestC2Metric_SkewReportsCostToCorrupt(t *testing.T) {
	const minBond = int64(1) << 20
	dom, s1, s2, s3 := key(10), key(11), key(12), key(13)
	c := buildC2(t, 2, 1, []c2Bond{{dom, 10 << 20}, {s1, minBond}, {s2, minBond}, {s3, minBond}})

	m := c.C2Metric()
	if m.Participants != 4 {
		t.Fatalf("participants = %d, want 4", m.Participants)
	}
	if want := int64(13) << 20; m.TotalBondedBytes != want {
		t.Fatalf("total bonded = %d, want %d", m.TotalBondedBytes, want)
	}
	if m.NakamotoBonds != 1 {
		t.Fatalf("nakamoto bonds = %d, want 1 (one whale exceeds ⌊total/3⌋; satellite keys don't dilute weight)", m.NakamotoBonds)
	}
	if m.NakamotoOperators != 1 || m.Margin != 1 {
		t.Fatalf("operators/margin = %d/%d, want 1/1 at M=1", m.NakamotoOperators, m.Margin)
	}
	if want := (int64(13)<<20)/3 + 1; m.CostToCorruptBytes != want {
		t.Fatalf("cost-to-corrupt = %d, want %d (⌊total/3⌋+1)", m.CostToCorruptBytes, want)
	}
	if c.Mature() {
		t.Fatal("a whale-dominated set (coefficient 1 < MatureValidators 2) must NOT shed the wheels")
	}
}

// TestC2Metric_OperatorMarginRaisesShedBar: the SAME set that matures at M=1
// stays immature at M=2 — the split half of the skew+split attack. A splitter must
// clear MatureValidators × M distinct bonds to shed.
func TestC2Metric_OperatorMarginRaisesShedBar(t *testing.T) {
	four := func(margin int) *Chain {
		return buildC2(t, 2, margin, []c2Bond{
			{key(20), 3 << 20}, {key(21), 3 << 20}, {key(22), 3 << 20}, {key(23), 3 << 20},
		})
	}

	// 4 equal bonds → coefficient 2. At M=1 (operators 2 ≥ 2) the wheels shed.
	c1 := four(1)
	if m := c1.C2Metric(); m.NakamotoBonds != 2 || m.NakamotoOperators != 2 {
		t.Fatalf("M=1: bonds/operators = %d/%d, want 2/2", m.NakamotoBonds, m.NakamotoOperators)
	}
	if !c1.Mature() {
		t.Fatal("4 equal bonds at M=1 (operators 2 ≥ MatureValidators 2) should mature")
	}

	// Same 4 bonds at M=2 → operators ⌊2/2⌋=1 < 2 → still immature: a possible
	// single operator behind ≤2 keys is not enough decentralization.
	c2 := four(2)
	if m := c2.C2Metric(); m.NakamotoOperators != 1 || m.Margin != 2 {
		t.Fatalf("M=2: operators/margin = %d/%d, want 1/2", m.NakamotoOperators, m.Margin)
	}
	if c2.Mature() {
		t.Fatal("the split-half bar: 4 bonds at M=2 (operators 1 < 2) must NOT shed")
	}

	// A genuinely larger set clears the M=2 bar: 10 equal bonds → coefficient 4 →
	// operators 2 ≥ 2 → mature.
	var big []c2Bond
	for i := 0; i < 10; i++ {
		big = append(big, c2Bond{key(int64(30 + i)), 3 << 20})
	}
	c3 := buildC2(t, 2, 2, big)
	if m := c3.C2Metric(); m.NakamotoBonds != 4 || m.NakamotoOperators != 2 {
		t.Fatalf("10 equal bonds: bonds/operators = %d/%d, want 4/2", m.NakamotoBonds, m.NakamotoOperators)
	}
	if !c3.Mature() {
		t.Fatal("10 equal bonds at M=2 (operators 2 ≥ 2) should mature — real decentralization clears the raised bar")
	}
}

// TestC2Metric_ConcentrationSignals pins the F-1-follow-up observability fields:
// HHI, Gini, and the top bond's share — the out-of-band veto that makes an
// honest-whale concentration event LOUD (measurement, not enforcement). An even
// set reads low on all three (no alarm); a whale-dominated set reads high and its
// top share clears the ⅓ capture fraction (the daemon's alarm threshold).
func TestC2Metric_ConcentrationSignals(t *testing.T) {
	const minBond = int64(1) << 20
	approx := func(got, want, eps float64) bool { d := got - want; return d < eps && d > -eps }

	// Even: four equal bonds → HHI 1/4, Gini 0, top 1/4 (< ⅓, no alarm).
	e := buildC2(t, 2, 1, []c2Bond{{key(20), minBond}, {key(21), minBond}, {key(22), minBond}, {key(23), minBond}})
	em := e.C2Metric()
	if !approx(em.HHI, 0.25, 0.001) || !approx(em.Gini, 0, 0.001) || !approx(em.TopShare, 0.25, 0.001) {
		t.Fatalf("even set: HHI=%.4f Gini=%.4f top=%.4f, want ~0.25/0/0.25", em.HHI, em.Gini, em.TopShare)
	}
	if em.TopShare >= 1.0/3 {
		t.Fatal("an even set must not trip the ⅓ concentration alarm")
	}

	// Whale: one 10 MiB bond + three 1 MiB → total 13 MiB. top 10/13≈0.77 (≥ ⅓,
	// alarm), HHI≈0.61, Gini≈0.52.
	w := buildC2(t, 2, 1, []c2Bond{{key(30), 10 << 20}, {key(31), minBond}, {key(32), minBond}, {key(33), minBond}})
	wm := w.C2Metric()
	if !approx(wm.TopShare, 10.0/13, 0.001) {
		t.Fatalf("whale top share = %.4f, want ~%.4f", wm.TopShare, 10.0/13)
	}
	if wm.TopShare < 1.0/3 {
		t.Fatal("a whale-dominated set must trip the ⅓ concentration alarm")
	}
	if !approx(wm.HHI, 0.6095, 0.002) || !approx(wm.Gini, 0.519, 0.002) {
		t.Fatalf("whale HHI=%.4f Gini=%.4f, want ~0.61/0.52", wm.HHI, wm.Gini)
	}
	// Concentration must read strictly higher than the even set on every signal.
	if !(wm.HHI > em.HHI && wm.Gini > em.Gini && wm.TopShare > em.TopShare) {
		t.Fatal("the whale set must read more concentrated than the even set on HHI, Gini, and top share")
	}
}

// TestC2Metric_WeightUniformityCatchesEqualBondSplit pins the seam-5 count/entropy
// companion signal. An equal-bond SPLIT — one operator posting N identical min-bonds
// across N keys — is the strategy the WEIGHT signals (HHI, Gini, TopShare) are blind
// to: it drives them all to their most-decentralized values, so the ⅓ whale alarm
// never fires. WeightUniformity exposes the "many atoms, implausibly uniform"
// fingerprint (→1 for identical bonds) that the weight signals miss, while a whale
// reads LOW uniformity. It is necessary-not-sufficient (a size-varying splitter
// evades it, healthy decentralization is also uniform — #182), but it is strictly
// more signal than the weight-only alarms had.
func TestC2Metric_WeightUniformityCatchesEqualBondSplit(t *testing.T) {
	const minBond = int64(1) << 20
	approx := func(got, want, eps float64) bool { d := got - want; return d < eps && d > -eps }

	// Equal-bond split: 12 identical min-bonds (one operator across 12 keys). The
	// weight signals read maximally decentralized — HHI=1/12, Gini=0, top=1/12, well
	// under the ⅓ alarm — so a monitor watching only those sees "nothing wrong".
	var split []c2Bond
	for i := int64(0); i < 12; i++ {
		split = append(split, c2Bond{key(40 + i), minBond})
	}
	sm := buildC2(t, 2, 1, split)
	m := sm.C2Metric()
	if m.Participants != 12 {
		t.Fatalf("split: Participants=%d, want 12", m.Participants)
	}
	if m.TopShare >= 1.0/3 || !approx(m.HHI, 1.0/12, 0.001) || !approx(m.Gini, 0, 0.001) {
		t.Fatalf("split must read decentralized on the WEIGHT signals: HHI=%.4f Gini=%.4f top=%.4f (want ~%.4f/0/<⅓)", m.HHI, m.Gini, m.TopShare, 1.0/12)
	}
	// The new signal: perfectly uniform → WeightUniformity ≈ 1.0.
	if !approx(m.WeightUniformity, 1.0, 0.001) {
		t.Fatalf("equal-bond split must read WeightUniformity ≈ 1.0 (the atomization fingerprint), got %.4f", m.WeightUniformity)
	}
	// The daemon's atomization note fires exactly here (whale-clean + atomized + uniform).
	if !(m.TopShare < 1.0/3 && m.Participants >= 8 && m.WeightUniformity >= 0.9) {
		t.Fatal("the equal-bond split must satisfy the atomization-note condition the whale alarm misses")
	}

	// A whale-dominated set reads LOW uniformity — the signal is orthogonal to the
	// weight ones and points the other way (concentration, not atomization).
	w := buildC2(t, 2, 1, []c2Bond{{key(60), 10 << 20}, {key(61), minBond}, {key(62), minBond}, {key(63), minBond}})
	wm := w.C2Metric()
	if wm.WeightUniformity >= m.WeightUniformity {
		t.Fatalf("a whale set must read lower WeightUniformity than an equal split: whale=%.4f split=%.4f", wm.WeightUniformity, m.WeightUniformity)
	}
	if wm.WeightUniformity >= 0.9 {
		t.Fatalf("a whale set must not read as uniform: WeightUniformity=%.4f", wm.WeightUniformity)
	}
}
