package main

// Lane C3 / R2.2 — THE THREE TESTABLE-TELEMETRY GATES the Economist advisory re-pointed.
// (ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07 §2.1.)
//
// WHY THESE GATES EXIST AT ALL. The observability design doc's own §6 gate asserted a
// NETWORK-WIDE repair Gini under a threshold. That gate is vacuous: under D-TIERING the
// transient pony tier serves and relays but does no durability work, so at the ratified
// vision ratio the network-wide repair Gini reads 0.9902 with repair spread PERFECTLY
// EVENLY over every capable node. A threshold below it fails on a healthy network; one
// above it passes on total capture. The advisory replaced it with three assertions. This
// file is two of them; the third is named at the bottom with the product surface it needs.
//
// THE BINDING CONSTRAINT, and how it is honoured. The test and the panel must share ONE
// code path, so the test proves the panel. Every number asserted here is read out of the
// JSON body of a REAL GET on the REAL registered route, driven from a REAL *node.Node
// whose peerCaps were filled by REAL inbound gossip over a transport. The path is:
//
//	simnet Endpoint.Send -> node.(*Node).handle -> n.peerCaps
//	  -> node.(*Node).EconomySample -> credit.Gini
//	  -> main.economyConcentrationDoc / main.economyNetworkDoc -> giniOver -> JSON
//
// Nothing here recomputes a concentration figure beside the product's. A gate that
// computes its own number proves nothing about what an operator sees.
//
// THE GATE IS A COMPOSITE, AND THE THIRD CLAUSE IS A TESTER ADDITION. The advisory
// specifies a bare threshold. A bare threshold does not hold, and the measurement that
// forced the extra clause is in TestGateC3_1_ServeWorkFederationOnTheOperatorPanel's
// ablation A3: under the certified M-2 exclusion rule a peer that reported NOTHING is
// dropped from the series rather than counted as a zero, so a network where five nodes do
// ALL the serving and every other node is silent publishes serveGini 0.0000 with
// known:true — total capture rendered as perfect equality. The reporting-coverage clause
// closes it, and its floor is DERIVED, not chosen: for a series in which a fraction c of
// the sample reports and the rest are truly at zero, the excluded truth is Gini = 1 - c,
// so the error the exclusion rule can introduce is bounded by (1 - c). Requiring that
// error to stay inside the gate's own tolerance gives 1 - c <= 0.15, i.e. c >= 0.85 for
// the serve series and c >= 0.60 for the repair series at its 0.40 tolerance. Both floors
// are built ONLY from fields the panel already publishes (serveGini.sampleSize,
// repairGini.sampleSize, sample.size, and the tier mix), so no product change is needed.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// ---- the thresholds, with their provenance ------------------------------------------
//
// EVOLVING-TIER PARAMETERS (docs/TENETS.md Part IX). They are recorded here beside the
// gate, as the advisory requires, so a future re-derivation reads the reason and not just
// the number.
const (
	// c3ServeGiniMax: the advisory's federated-serve baseline. Provenance: the honest
	// vision-ratio band is 0.0575 (CPU-weighted 1/7/24) to 0.1209 (disk-weighted
	// 0.1/1/50 TB), so 0.15 clears the widest honest weighting. READ
	// TestGateC3_1_ThresholdProvenanceIsUnreachableUnderDiskWeighting before trusting
	// that sentence: the 0.1209 corner is measured on a 10,101-node distribution and a
	// node's peerCaps sample is bounded at maxPeerInfo = 4096.
	c3ServeGiniMax = 0.15
	// c3RepairGiniMax: the midpoint of the measured span 0.0192 (repair even across 100
	// horses + 1 archival) to 0.7505 (top 5 of 101 do 80 %). Crossing it means a handful
	// of the persistent tier is doing most of the repair.
	c3RepairGiniMax = 0.40
	// c3ServeReportingMin / c3RepairReportingMin: DERIVED from the two above, see the
	// file header. The exclusion rule's worst-case error is (1 - coverage); holding that
	// error inside the series' own tolerance gives 1 - 0.15 and 1 - 0.40.
	c3ServeReportingMin  = 0.85
	c3RepairReportingMin = 0.60
)

// ---- the wire shapes the gate reads --------------------------------------------------
//
// Decoded off the served JSON, not off the Go struct, because the wire is what the panel
// renders and what an operator sees. Value is a POINTER: giniValue.Value is omitempty, so
// a Gini of exactly 0.0 arrives as an ABSENT key beside known:true, and a gate that
// decoded it into a plain float64 could not tell "measured 0.0" from "no such field".

type c3GiniWire struct {
	Known      bool     `json:"known"`
	Value      *float64 `json:"value"`
	SampleSize int      `json:"sampleSize"`
	Reason     string   `json:"reason"`
}

func (g *c3GiniWire) val() float64 {
	if g == nil || g.Value == nil {
		return 0
	}
	return *g.Value
}

type c3ConcentrationWire struct {
	Sample *struct {
		Size     int  `json:"size"`
		TooSmall bool `json:"tooSmall"`
	} `json:"sample"`
	ServeGini        *c3GiniWire `json:"serveGini"`
	RepairGini       *c3GiniWire `json:"repairGini"`
	CountersWithheld bool        `json:"countersWithheld"`
}

type c3NetworkWire struct {
	Mix []struct {
		Class   string  `json:"class"`
		Sampled int     `json:"sampled"`
		Share   float64 `json:"share"`
	} `json:"mix"`
}

// capable is the size of the repair-CAPABLE subset of the sample, read off the published
// tier mix. It is the denominator of the repair series' reporting coverage: the repair
// Gini is scoped to horse+archival, so its coverage must be measured against that subset
// and not against the whole sample.
func (n c3NetworkWire) capable() int {
	out := 0
	for _, row := range n.Mix {
		if row.Class == node.TierHorse || row.Class == node.TierArchival {
			out += row.Sampled
		}
	}
	return out
}

// ---- THE GATE PREDICATES -------------------------------------------------------------
//
// Written ONCE and exercised by every arm below, healthy and ablated alike. That is what
// makes each ablation a proof about THE GATE rather than about a hand-copied inequality:
// an ablation arm asserts that this exact function refuses, and names which clause.

// c3ServeGate is assertion 1 of the advisory, as a predicate over the served document.
func c3ServeGate(c c3ConcentrationWire) (ok bool, why string) {
	if c.CountersWithheld {
		return false, "serveGini withheld: the reader cannot see the figure at all"
	}
	if c.Sample == nil {
		return false, "no sample block: the required sibling of every gossip-estimated number is absent"
	}
	if c.ServeGini == nil {
		return false, fmt.Sprintf("no serveGini published at all over a sample of %d (the series is below minGossipSample=%d, or the route dropped it)", c.Sample.Size, minGossipSample)
	}
	// CLAUSE 1 — KNOWN-NESS, NOT THE VALUE. credit.Gini returns 0 both for a measured
	// equality and for a sample that summed to zero, and giniOver renders the second as
	// known:false with no value. A gate that read the number would score an absent
	// measurement as a perfect pass.
	if !c.ServeGini.Known {
		return false, "serveGini is UNKNOWN (" + c.ServeGini.Reason + "): an absent measurement is not a passing 0"
	}
	// CLAUSE 3 first, because it decides whether clause 2's number means anything.
	cov := float64(c.ServeGini.SampleSize) / float64(c.Sample.Size)
	if cov < c3ServeReportingMin {
		return false, fmt.Sprintf("serve reporting coverage %.4f (%d of %d sampled nodes reported serve work) is below %.2f: the M-2 exclusion rule drops the silent majority from the series, so the published %.4f understates the true concentration by as much as %.4f",
			cov, c.ServeGini.SampleSize, c.Sample.Size, c3ServeReportingMin, c.ServeGini.val(), 1-cov)
	}
	// CLAUSE 2 — the advisory's threshold.
	if v := c.ServeGini.val(); v > c3ServeGiniMax {
		return false, fmt.Sprintf("serveGini %.4f over %d reporting nodes exceeds %.2f: serve work is concentrating off the edge tier", v, c.ServeGini.SampleSize, c3ServeGiniMax)
	}
	return true, ""
}

// c3RepairGate is assertion 2 of the advisory. Its coverage denominator is the CAPABLE
// subset, read off the tier mix on the sibling route — the same node.EconomySample, one
// derivation apart.
func c3RepairGate(c c3ConcentrationWire, nw c3NetworkWire) (ok bool, why string) {
	if c.CountersWithheld {
		return false, "repairGini withheld: the reader cannot see the figure at all"
	}
	if c.Sample == nil {
		return false, "no sample block"
	}
	if c.RepairGini == nil {
		return false, fmt.Sprintf("no repairGini published at all: the repair-capable reporting subset is below minGossipSample=%d, so the whole durability-concentration alarm is dark on a sample of %d", minGossipSample, c.Sample.Size)
	}
	if !c.RepairGini.Known {
		return false, "repairGini is UNKNOWN (" + c.RepairGini.Reason + "): an absent measurement is not a passing 0"
	}
	capable := nw.capable()
	if capable == 0 {
		return false, "the tier mix reports NO repair-capable node in the sample, yet a repair Gini was published over it"
	}
	cov := float64(c.RepairGini.SampleSize) / float64(capable)
	if cov < c3RepairReportingMin {
		return false, fmt.Sprintf("repair reporting coverage %.4f (%d of %d repair-capable nodes reported) is below %.2f: the published %.4f is a statement about a minority of the persistent tier",
			cov, c.RepairGini.SampleSize, capable, c3RepairReportingMin, c.RepairGini.val())
	}
	if v := c.RepairGini.val(); v > c3RepairGiniMax {
		return false, fmt.Sprintf("repairGini within the capable subset %.4f over %d reporting nodes exceeds %.2f: a handful of the persistent tier is doing most of the repair", v, c.RepairGini.SampleSize, c3RepairGiniMax)
	}
	return true, ""
}

// ---- the fixture: a real node, real inbound gossip, the real routes -------------------

// c3Peer is one gossiped peer: what it PLEDGES (which decides its tier band) and what it
// REPORTS (which decides whether it is in the work series at all).
type c3Peer struct {
	capTotal int64
	served   int64
	repairs  int64
	n        int // how many peers of this shape
}

// The three bands, by pledged capacity, straddling node.TierPonyMaxBytes (16 GiB) and
// node.TierHorseMaxBytes (1 TiB). Read from the product constants so a band move reddens
// here rather than silently re-tiering the fixture.
const (
	c3PonyCap     = int64(4) << 30  // 4 GiB   -> pony
	c3HorseCap    = int64(64) << 30 // 64 GiB  -> horse
	c3ArchivalCap = int64(4) << 40  // 4 TiB   -> archival
	c3Unit        = int64(1) << 30  // 1 GiB of served bytes
)

// c3Fixture builds a uiServer over a real node, gossips every peer in over simnet, waits
// for the sample to converge, and returns the two served documents.
func c3Fixture(t *testing.T, seed int64, peers []c3Peer) (c3ConcentrationWire, c3NetworkWire) {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	clk := walltime.New(loop)
	net := simnet.New(clk, seed, simnet.Config{}) // zero latency: deterministic, and the
	// gate is about a computed figure, not about timing
	id := identity.FromSeed(2200 + seed)
	self := id.NodeID()
	nd := node.New(self, node.DefaultConfig(), clk, net.Endpoint(self), memstore.New())
	// privacy ON (the compiled default) with the token presented: the OPERATOR's own
	// dashboard read, which is the reader these two panels are for. An untokened reader
	// gets countersWithheld, and c3ServeGate refuses that explicitly rather than reading
	// an absent field as a pass.
	s := &uiServer{loop: loop, nd: nd, token: "tok", started: time.Now(),
		peerCount: func() int { return 0 }, privacy: privacyDefaultWithheld}

	total := 0
	for _, p := range peers {
		total += p.n
	}
	if total < minGossipSample {
		t.Fatalf("fixture builds %d peers, below minGossipSample=%d — the routes would publish nothing and every arm would be vacuous", total, minGossipSample)
	}
	// Sends run ON THE LOOP: simnet.Network is single-threaded state (Stats, the RNG, the
	// endpoint map) and the node replies to every MsgFindNode from that same loop.
	s.onLoop(func() {
		i := 0
		for _, p := range peers {
			for k := 0; k < p.n; k++ {
				i++
				var from ports.NodeID
				from[0], from[1], from[2] = byte(i), byte(i>>8), byte(i>>16)
				from[31] = 0xC3
				if err := net.Endpoint(from).Send(self, ports.Message{Kind: ports.MsgFindNode,
					CapUsed: 1, CapTotal: p.capTotal, ServedBytes: p.served, RepairsDone: p.repairs}); err != nil {
					t.Errorf("fixture send %d: %v", i, err)
				}
			}
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	got := 0
	for time.Now().Before(deadline) {
		s.onLoop(func() { got = nd.EconomySample().Size })
		if got == total {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got != total {
		t.Fatalf("EconomySample.Size converged to %d, want %d — the fixture never landed, so any verdict below is on the wrong sample", got, total)
	}

	at := time.Now()
	var conc c3ConcentrationWire
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/concentration", at, true)), &conc); err != nil {
		t.Fatalf("decode /api/economy/concentration: %v", err)
	}
	var nw c3NetworkWire
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/network", at, true)), &nw); err != nil {
		t.Fatalf("decode /api/economy/network: %v", err)
	}
	return conc, nw
}

// ---- the shapes ----------------------------------------------------------------------
//
// THE HEALTHY SHAPE is the ratified vision ratio (D-TIERING, "ponies : horses : archival
// ~ 10000 : 100 : 1") at the largest instance that fits maxPeerInfo, weighted CPU-wise
// 1 : 7 : 24 — one of the two honest weightings the advisory names. Every peer reports,
// ponies do no durability work, and repair is spread evenly across the capable tier.
func c3HealthyPeers() []c3Peer {
	return []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
}

// ---- GATE 1 ---------------------------------------------------------------------------

// TestGateC3_1_ServeWorkFederationOnTheOperatorPanel is advisory assertion 1.
//
// GATE: c3ServeGate over GET /api/economy/concentration — serveGini KNOWN, reporting
// coverage >= 0.85, value <= 0.15.
//
// FOUR ARMS. The healthy arm must PASS; the three ablations must each make the SAME
// predicate refuse, and each must refuse on a DIFFERENT clause. An ablation that reddens
// the clause a different ablation already covered proves nothing new.
func TestGateC3_1_ServeWorkFederationOnTheOperatorPanel(t *testing.T) {
	// --- HEALTHY: the vision ratio, CPU-weighted, everybody reporting.
	conc, _ := c3Fixture(t, 1, c3HealthyPeers())
	ok, why := c3ServeGate(conc)
	if !ok {
		t.Fatalf("HEALTHY ARM RED: %s\n  The federated baseline must PASS on the ratified vision shape or the gate is unusable: it would fire on a network where nothing is wrong.", why)
	}
	t.Logf("healthy vision shape: serveGini %.4f known=%v over %d of %d sampled nodes (coverage %.4f)",
		conc.ServeGini.val(), conc.ServeGini.Known, conc.ServeGini.SampleSize, conc.Sample.Size,
		float64(conc.ServeGini.SampleSize)/float64(conc.Sample.Size))

	// --- ABLATION A1a — NOBODY REPORTS ANYTHING. Every peer pledges capacity and emits
	// neither work key, so the M-2 rule excludes all of them and the serve series is
	// EMPTY. economyConcentrationDoc's own floor then omits serveGini entirely. The gate
	// must refuse the ABSENCE: a consumer that reads a missing field as "nothing to alarm
	// about" is blind on a network that has reported no work at all.
	silent := []c3Peer{
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 60},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 6},
	}
	concA1a, _ := c3Fixture(t, 2, silent)
	if concA1a.ServeGini != nil {
		t.Fatalf("ABLATION A1a: serveGini was published over a wholly silent sample (%+v). Re-derive this arm — it exists to prove the gate refuses an ABSENT figure.", concA1a.ServeGini)
	}
	okA1a, whyA1a := c3ServeGate(concA1a)
	if okA1a {
		t.Fatalf("ABLATION A1a STAYED GREEN: no serveGini is published at all and the gate passed. Absence is being read as health.")
	}
	t.Logf("ABLATION A1a RED (as required): %s", whyA1a)

	// --- ABLATION A1b — THE KNOWN-NESS CLAUSE, and reaching it takes a specific shape.
	// giniOver renders known:false when the series SUMMED TO ZERO, but the route only
	// publishes the series at all once it holds minGossipSample entries — so the wholly
	// silent sample above never reaches that branch. The reachable shape is a sample whose
	// peers DO report (so they are in the series) but report zero SERVE work: the M-2
	// discriminator is the PAIR, so a peer emitting repairs and no serve is a reporting
	// peer whose serve zero counts. credit.Gini over [0,0,...] returns 0 ("universal
	// poverty is technically equality") and a gate that read the VALUE would score it as
	// perfect federation.
	repairOnly := []c3Peer{
		{capTotal: c3HorseCap, served: 0, repairs: 10, n: 8},
		{capTotal: c3ArchivalCap, served: 0, repairs: 10, n: 1},
	}
	concA1b, _ := c3Fixture(t, 5, repairOnly)
	if concA1b.ServeGini == nil {
		t.Fatal("ABLATION A1b: no serveGini published at all — this arm needs the series PRESENT and unknown, so it must clear minGossipSample. Rebuild the fixture.")
	}
	if concA1b.ServeGini.Known {
		t.Fatalf("ABLATION A1b: serveGini came back known:true over a series that summed to zero (value %.4f) — giniOver's total<=0 branch is not firing, and an absent measurement is being published as a measured equality", concA1b.ServeGini.val())
	}
	if concA1b.ServeGini.val() > c3ServeGiniMax {
		t.Fatalf("ABLATION A1b: the bare value is %.4f, so a value-only gate would already redden here and this arm would not prove the known-ness clause. Rebuild the fixture.", concA1b.ServeGini.val())
	}
	okA1b, whyA1b := c3ServeGate(concA1b)
	if okA1b {
		t.Fatalf("ABLATION A1b STAYED GREEN: serveGini is UNKNOWN (%q) with a rendered value of %.4f, and the gate passed. An absent measurement is being scored as a perfect pass — this is the exact defect giniOver.Known exists to expose.",
			concA1b.ServeGini.Reason, concA1b.ServeGini.val())
	}
	t.Logf("ABLATION A1b RED (as required): %s", whyA1b)

	// --- ABLATION A2 — the advisory's own ablation: a CONCENTRATED distribution where the
	// top 5 nodes serve 80 % of the bytes. Every node still reports, so coverage is 1.0
	// and this arm isolates the threshold clause.
	const a2Ponies = 1000
	concentrated := []c3Peer{
		// the top 5: 80 % of 1,000,000 units, split evenly
		{capTotal: c3HorseCap, served: 160_000 * c3Unit, repairs: 10, n: 5},
		// everyone else shares the remaining 20 %
		{capTotal: c3PonyCap, served: (200_000 / a2Ponies) * c3Unit, repairs: 0, n: a2Ponies},
		{capTotal: c3HorseCap, served: (200_000 / a2Ponies) * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: (200_000 / a2Ponies) * c3Unit, repairs: 10, n: 1},
	}
	concA2, _ := c3Fixture(t, 3, concentrated)
	okA2, whyA2 := c3ServeGate(concA2)
	if okA2 {
		t.Fatalf("ABLATION A2 STAYED GREEN: the top 5 of %d nodes serve 80 %% of the bytes and the serve gate passed. serveGini=%.4f known=%v coverage=%.4f — the advisory's assertion 1 is decoration.",
			concA2.Sample.Size, concA2.ServeGini.val(), concA2.ServeGini.Known,
			float64(concA2.ServeGini.SampleSize)/float64(concA2.Sample.Size))
	}
	if cov := float64(concA2.ServeGini.SampleSize) / float64(concA2.Sample.Size); cov < c3ServeReportingMin {
		t.Fatalf("ABLATION A2 reddened on the COVERAGE clause (%.4f), not on the threshold clause. That makes it a duplicate of A3 and leaves the advisory's own ablation unproven — the fixture must have every node reporting.", cov)
	}
	if concA2.ServeGini.val() <= c3ServeGiniMax {
		t.Fatalf("ABLATION A2: serveGini %.4f is at or under the %.2f threshold on a top-5-serve-80%% distribution — the threshold does not separate concentration from health", concA2.ServeGini.val(), c3ServeGiniMax)
	}
	t.Logf("ABLATION A2 RED (as required): %s", whyA2)

	// --- ABLATION A3 — THE ONE THE ADVISORY DID NOT SPECIFY, and the reason the gate has
	// a third clause. Same total capture as A2, but the losing majority reports NOTHING
	// rather than a little. Under the certified M-2 rule those peers are EXCLUDED from the
	// series rather than counted as zeros, so the published figure is a Gini over the five
	// capturing nodes alone — and because they capture EQUALLY it is 0.0000 with
	// known:true. A bare `serveGini <= 0.15` gate passes total capture.
	capturedAndSilent := []c3Peer{
		{capTotal: c3HorseCap, served: 200_000 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 5},
		{capTotal: c3ArchivalCap, served: 0, repairs: 0, n: 1},
	}
	concA3, _ := c3Fixture(t, 4, capturedAndSilent)
	// First RECORD the defeat, so the finding is a measurement and not a claim.
	if concA3.ServeGini == nil || !concA3.ServeGini.Known {
		t.Fatalf("ABLATION A3: expected a KNOWN serveGini over the five capturing nodes; got %+v. The exclusion rule changed — re-derive c3ServeReportingMin.", concA3.ServeGini)
	}
	if concA3.ServeGini.val() > c3ServeGiniMax {
		t.Fatalf("ABLATION A3: the BARE threshold now reddens on total-capture-plus-silence (serveGini %.4f > %.2f). That is better than when this gate was written, and it means the M-2 exclusion rule changed — re-derive the coverage clause rather than deleting it.", concA3.ServeGini.val(), c3ServeGiniMax)
	}
	okA3, whyA3 := c3ServeGate(concA3)
	if okA3 {
		t.Fatalf("ABLATION A3 STAYED GREEN: %d of %d sampled nodes are silent while 5 nodes serve 100 %% of the bytes, and the gate passed with serveGini %.4f known:true. Total capture is rendering as perfect equality.",
			concA3.Sample.Size-concA3.ServeGini.SampleSize, concA3.Sample.Size, concA3.ServeGini.val())
	}
	t.Logf("ABLATION A3 RED (as required): %s", whyA3)
	t.Logf("ABLATION A3 measurement: the panel publishes serveGini %.4f known:true over %d of %d sampled nodes while five nodes serve 100 %% of the bytes. The advisory's bare `serveGini <= %.2f` PASSES this. The coverage clause is what refuses it.",
		concA3.ServeGini.val(), concA3.ServeGini.SampleSize, concA3.Sample.Size, c3ServeGiniMax)
}

// ---- GATE 2 ---------------------------------------------------------------------------

// TestGateC3_2_RepairWorkWithinTheCapableSubsetOnTheOperatorPanel is advisory assertion 2.
//
// GATE: c3RepairGate over GET /api/economy/concentration and GET /api/economy/network —
// repairGini PRESENT and KNOWN, reporting coverage over the CAPABLE subset >= 0.60,
// value <= 0.40.
func TestGateC3_2_RepairWorkWithinTheCapableSubsetOnTheOperatorPanel(t *testing.T) {
	// --- HEALTHY: repair spread evenly over 10 horses + 1 archival, 1000 ponies serving
	// and doing none of it. This is the shape whose NETWORK-WIDE repair Gini is ~0.99.
	conc, nw := c3Fixture(t, 11, c3HealthyPeers())
	ok, why := c3RepairGate(conc, nw)
	if !ok {
		t.Fatalf("HEALTHY ARM RED: %s\n  Repair spread PERFECTLY EVENLY across the whole persistent tier must pass, or the gate fires on the vision shape itself.", why)
	}
	if got, want := conc.RepairGini.SampleSize, 11; got != want {
		t.Fatalf("the repair series covered %d nodes, want %d (the capable subset only). If the 1000 ponies are in it the figure is ~0.99 by construction and carries no signal — this is the advisory's §2.1 correction.", got, want)
	}
	t.Logf("healthy vision shape: repairGini(capable subset) %.4f known=%v over %d of %d capable nodes",
		conc.RepairGini.val(), conc.RepairGini.Known, conc.RepairGini.SampleSize, nw.capable())

	// --- ABLATION B1 — the advisory's ablation: route EVERY repair to one node. The other
	// capable nodes still serve, so they still report and their zero is a genuine measured
	// zero that counts. This isolates the threshold clause.
	oneRepairer := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 0, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 500, n: 1},
	}
	concB1, nwB1 := c3Fixture(t, 12, oneRepairer)
	okB1, whyB1 := c3RepairGate(concB1, nwB1)
	if okB1 {
		t.Fatalf("ABLATION B1 STAYED GREEN: every repair on ONE of %d capable nodes and the gate passed. repairGini=%.4f — the scoped series does not redden on total durability capture, so it is decoration.", nwB1.capable(), concB1.RepairGini.val())
	}
	if concB1.RepairGini == nil || !concB1.RepairGini.Known {
		t.Fatalf("ABLATION B1 reddened on the KNOWN clause, not the threshold clause: %+v. The fixture must keep every capable node REPORTING so its zero counts.", concB1.RepairGini)
	}
	if cov := float64(concB1.RepairGini.SampleSize) / float64(nwB1.capable()); cov < c3RepairReportingMin {
		t.Fatalf("ABLATION B1 reddened on the COVERAGE clause (%.4f), duplicating B2 and leaving the advisory's own ablation unproven", cov)
	}
	if concB1.RepairGini.val() <= c3RepairGiniMax {
		t.Fatalf("ABLATION B1: repairGini %.4f is at or under %.2f with every repair on one node — the threshold does not separate capture from health", concB1.RepairGini.val(), c3RepairGiniMax)
	}
	t.Logf("ABLATION B1 RED (as required): %s", whyB1)

	// --- ABLATION B2 — the repair analogue of A3. Four capable nodes repair equally; the
	// other seven report NOTHING AT ALL and are excluded. The published repairGini is
	// 0.0000 known:true over a minority of the persistent tier. Coverage refuses it.
	minorityReports := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 4},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 6},
		{capTotal: c3ArchivalCap, served: 0, repairs: 0, n: 1},
	}
	concB2, nwB2 := c3Fixture(t, 13, minorityReports)
	okB2, whyB2 := c3RepairGate(concB2, nwB2)
	if okB2 {
		t.Fatalf("ABLATION B2 STAYED GREEN: %d of %d repair-capable nodes are silent and the gate passed on a repairGini of %.4f over the remaining %d.",
			nwB2.capable()-concB2.RepairGini.SampleSize, nwB2.capable(), concB2.RepairGini.val(), concB2.RepairGini.SampleSize)
	}
	if concB2.RepairGini == nil || !concB2.RepairGini.Known || concB2.RepairGini.val() > c3RepairGiniMax {
		t.Fatalf("ABLATION B2 reddened on a clause other than coverage (%+v); it then duplicates B1/B3 and the coverage clause is unproven", concB2.RepairGini)
	}
	t.Logf("ABLATION B2 RED (as required): %s", whyB2)

	// --- ABLATION B3 — the DARK-PANEL ablation. Only two capable nodes report, which is
	// below minGossipSample, so economyConcentrationDoc omits repairGini ENTIRELY. A
	// consumer that treats an absent field as "nothing to alarm about" reads a captured
	// durability tier as healthy. The gate must refuse ABSENCE, not just a bad value.
	belowFloor := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 2},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 8},
		{capTotal: c3ArchivalCap, served: 0, repairs: 0, n: 1},
	}
	concB3, nwB3 := c3Fixture(t, 14, belowFloor)
	if concB3.RepairGini != nil {
		t.Fatalf("ABLATION B3: repairGini was published over %d reporting capable nodes, below minGossipSample=%d. Re-derive this arm: it exists to prove the gate refuses an ABSENT figure.", concB3.RepairGini.SampleSize, minGossipSample)
	}
	okB3, whyB3 := c3RepairGate(concB3, nwB3)
	if okB3 {
		t.Fatalf("ABLATION B3 STAYED GREEN: the repair panel is DARK (repairGini absent) and the gate passed. Absence is being read as health.")
	}
	t.Logf("ABLATION B3 RED (as required): %s", whyB3)
}

// ---- the provenance pin ----------------------------------------------------------------

// TestGateC3_1_ThresholdProvenanceIsUnreachableUnderDiskWeighting records a MEASURED
// limit on assertion 1's threshold, so the number is not carried forward on the strength
// of its own footnote.
//
// THE ADVISORY'S SENTENCE: "the vision-ratio band is 0.058-0.121, so 0.15 clears the
// widest honest weighting". The 0.1209 corner is the DISK weighting (0.1 / 1 / 50 TB) at
// the vision ratio 10000 : 100 : 1 — a distribution of 10,101 nodes. A node's peerCaps
// sample is bounded at node.maxPeerInfo, and the disk-weighted honest Gini does not fall
// to 0.15 until the sample passes ~5,600 nodes. So on EVERY sample a silt node can
// actually hold, the disk-weighted honest shape reads ABOVE the threshold.
//
// Measured (core/credit.Gini, vision proportions np : np/100 : 1, weights 0.1/1/50 TB):
//
//	n =  1,011  gini 0.3671      n =  4,096  gini 0.1726   <- the observable ceiling
//	n =  2,021  gini 0.2507      n =  5,051  gini 0.1574
//	n =  3,031  gini 0.2016      n =  6,061  gini 0.1455   <- first pass, unobservable
//	n =  4,041  gini 0.1745      n = 10,101  gini 0.1209   <- the advisory's corner
//
// WHAT THIS DOES NOT SAY. It does not say the gate is wrong. Gate 1's healthy arm uses the
// CPU weighting (1 / 7 / 24), which clears 0.15 at every observable size, and the
// concentrated ablation reads 0.72-0.80 at every size, so the gate separates. It says the
// threshold's stated provenance covers one of the two honest weightings on any sample this
// code can observe, and that a disk-heavy real network may sit above 0.15 with nothing
// wrong. That is an owner/Economist re-derivation, not a number a tester may tune.
//
// This test drives the REAL route so the claim is about the shipped surface. It reddens if
// the disk-weighted honest shape ever passes at the observable ceiling — which is the
// event that would discharge the finding.
func TestGateC3_1_ThresholdProvenanceIsUnreachableUnderDiskWeighting(t *testing.T) {
	if testing.Short() {
		t.Skip("drives a 4,096-peer sample; -short runs the two gates only")
	}
	// The observable ceiling: node.maxPeerInfo peers at the vision proportions, weighted
	// by the disk table. 4055 : 40 : 1 = 4,096 = the bound.
	diskWeighted := []c3Peer{
		{capTotal: c3PonyCap, served: 100 * c3Unit, repairs: 0, n: 4055},     // 0.1 TB
		{capTotal: c3HorseCap, served: 1000 * c3Unit, repairs: 10, n: 40},    // 1 TB
		{capTotal: c3ArchivalCap, served: 50000 * c3Unit, repairs: 10, n: 1}, // 50 TB
	}
	conc, _ := c3Fixture(t, 21, diskWeighted)
	if conc.ServeGini == nil || !conc.ServeGini.Known {
		t.Fatalf("provenance pin: no known serveGini over the ceiling sample: %+v", conc.ServeGini)
	}
	v := conc.ServeGini.val()
	t.Logf("disk-weighted honest vision shape at the observable ceiling (%d peers = maxPeerInfo): serveGini %.4f, threshold %.2f",
		conc.Sample.Size, v, c3ServeGiniMax)
	if v <= c3ServeGiniMax {
		t.Fatalf("PROVENANCE PIN DISCHARGED (or the shape moved): the disk-weighted honest vision shape now reads %.4f <= %.2f at the largest observable sample. When this test was written it read 0.1726 and the threshold's stated provenance (0.1209) was only reachable at 10,101 nodes, above maxPeerInfo. Re-read the finding before deleting this test.", v, c3ServeGiniMax)
	}
}

// ---- the anti-substitution pin ---------------------------------------------------------

// TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes exists because advisory assertion 1
// has a SECOND clause this file cannot encode: "pony share of served bytes >= 0.50", the
// T-AR tenet read literally (the edge tier does the MAJORITY of the work).
//
// NO PRODUCT SURFACE PUBLISHES IT. node.EconomySample carries Mix (a count per tier),
// ServeGini, ServeSampleSize and ServeWorkTotal (the sample-wide SUM). It does not carry a
// per-tier serve-byte total, and neither of the two gossip-estimated routes derives one.
// So the clause is STOPPED pending a product change, and this test is the guard that stops
// the nearest-looking field from being substituted for it.
//
// THE TRAP THIS CLOSES. economyNetwork's mix[].share LOOKS like the missing number and is
// not: it is a share of NODE COUNT. Under the vision ratio the pony share of nodes is
// ~0.99 by construction, so a gate reading mix[].share for the T-AR floor would pass on
// EVERY distribution — including one where the edge tier serves 20 % of the bytes. This
// test measures exactly that inversion on the concentrated fixture.
func TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes(t *testing.T) {
	// The A2 shape: the top 5 horses serve 80 % of the bytes. TRUE pony byte share ~0.20.
	const ponies = 1000
	concentrated := []c3Peer{
		{capTotal: c3HorseCap, served: 160_000 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3PonyCap, served: (200_000 / ponies) * c3Unit, repairs: 0, n: ponies},
		{capTotal: c3HorseCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: (200_000 / ponies) * c3Unit, repairs: 10, n: 1},
	}
	conc, nw := c3Fixture(t, 31, concentrated)

	var ponyNodeShare float64
	for _, row := range nw.Mix {
		if row.Class == node.TierPony {
			ponyNodeShare = row.Share
		}
	}
	// The true pony share of served BYTES on this fixture, computed from the fixture's own
	// definition (NOT from any product figure — there is none to read).
	truePonyByteShare := float64(ponies*(200_000/ponies)) / float64(5*160_000+ponies*(200_000/ponies)+5*(200_000/ponies)+(200_000/ponies))

	t.Logf("concentrated fixture: mix[pony].share = %.4f (share of NODE COUNT) while the true pony share of served BYTES is %.4f. serveGini = %.4f.",
		ponyNodeShare, truePonyByteShare, conc.ServeGini.val())

	if ponyNodeShare < 0.50 {
		t.Fatalf("mix[pony].share is %.4f on the vision ratio — this pin assumes it is high BY CONSTRUCTION (it is a node-count share) and that assumption just failed. Re-derive before trusting the substitution warning.", ponyNodeShare)
	}
	if truePonyByteShare >= 0.50 {
		t.Fatalf("fixture defect: the concentrated shape gives the pony tier %.4f of served bytes, which is not concentrated. Rebuild it.", truePonyByteShare)
	}
	// THE PIN. On this fixture the T-AR floor is VIOLATED (0.20 of served bytes) while the
	// nearest published field reads 0.99. Anything that substitutes mix[].share for the
	// missing byte-share clause passes total serve capture.
	for _, row := range nw.Mix {
		if row.Class == node.TierPony && row.Sampled != ponies {
			t.Fatalf("mix[pony].sampled = %d, want %d — the fixture did not land as built", row.Sampled, ponies)
		}
	}
}
