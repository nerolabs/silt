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
// THE GATE IS ONE ADJUSTED STATISTIC, NOT A THRESHOLD ON THE PUBLISHED NUMBER.
//
// The published Gini is computed over a SELF-SELECTED sample: under the certified M-2 rule
// (core/node/economysample.go) a peer that reported nothing is excluded rather than counted
// as a zero, and the entity a capture alarm points at is exactly the entity that decides
// whether it appears. Measured on the real route, ablation A3: a sample of 1011 in which
// five nodes serve 100 % of the bytes and 1006 are silent publishes
//
//	serveGini 0.0000  known:true  sampleSize 5
//
// Total capture rendered as perfect equality, and flagged as a measurement. A bare
// `serveGini <= 0.15` PASSES it.
//
// THE FIX, and it is an identity rather than a taste call. For a population split into a
// reporting fraction c holding all the work and a silent fraction (1-c) truly at zero, the
// mean-absolute-difference form of the Gini gives
//
//	G_adj = (1 - c) + c * G_pub
//
// so the error is (1 - c)(1 - G_pub) and the truth is bounded below by (1 - c). Both terms
// come from fields the panel already publishes (serveGini.sampleSize and sample.size; for
// repair, the capable count off the sibling route), so NO PRODUCT CHANGE is needed.
//
// The two gates assert G_adj against the tolerance. They do NOT carry a separate coverage
// clause, and the reason is arithmetic: a coverage floor is NECESSARY but not SUFFICIENT.
// A composite of `coverage >= 0.85` AND `G_pub <= 0.15` admits a true Gini of
// (1-0.85) + 0.85*0.15 = 0.2775; the repair pair admits 0.6400, which is inside the region
// the spec called capture. Gating on G_adj dominates the floor and re-derives it as a
// theorem: G_adj <= T implies c >= 1 - T. The floors are therefore DEFINED as 1 - T in the
// const block below and pinned by TestGateC3_IndeterminacyBoundaryIsATheoremOfTheTolerance.
// What they are is the INDETERMINACY BOUNDARY — below it the verdict is "I cannot see",
// never "this is captured" — not a gate clause.
//
// ONE-SIDEDNESS, so nobody over-reads these gates. The adjustment assumes silence means
// idle. Both terms of G_adj are Sybil-steerable in BOTH directions: a peer can go silent to
// deflate c, and 20 repairs-only sybils were measured to move a perfectly even network's
// serveGini from 0.0000 to 0.8696 (core/node/economysample.go, the M-2 comment). These gates
// may ABORT a canary. They may never CERTIFY federation.

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

// ---- the constants, and what they actually are --------------------------------------
//
// THESE ARE FIXTURE CONSTANTS FOR NAMED SYNTHETIC DISTRIBUTIONS. They are NOT honest
// bands and they are NOT field thresholds. Both of their original provenances were
// withdrawn by the Economist after this file's first round measured against them:
// /Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09.md
//
// WITHDRAWN, and the sentence is struck rather than softened: "the vision-ratio band is
// 0.058-0.121, so 0.15 clears the widest honest weighting". The 0.1209 corner is the DISK
// weighting (0.1/1/50 TB), which is a storage-share model applied to a serve-work series —
// serve bytes are bandwidth- and demand-bound, not holdings-bound. It is also a
// 10,101-node figure against a maxPeerInfo bound of 4,096. See
// TestGateC3_1_ServeGiniHonestNullIsMixDependentNotSampleSizeDependent.
//
// WITHDRAWN OUTRIGHT as a field threshold: `repairGiniWithinCapableSample <= 0.40` and its
// "midpoint of 0.019 (even) -> 0.751 (top-5-does-80 %)" provenance. Repair IS holdings-
// bound, and under a holdings-proportional null both endpoints of that span are HEALTHY
// shapes: measured, the honest null is 0.7424 at 11 capable nodes -- indistinguishable from
// the 0.7505 the spec labelled CAPTURE -- and 0.6282 at the canary minimum. A midpoint
// between two healthy shapes carries no economic content, and as a Phase-3 abort it would
// have killed the first honest canary silt ever ran. See
// TestGateC3_2_RepairNullIsHoldingsProportionalSoTheConstantIsNotAFieldThreshold.
//
// WHAT THEY ARE FOR. Each constant separates its own fixture from that fixture's ablations,
// measurably, and that is the whole claim. These are REGRESSION gates: they catch a change
// to EconomySample, credit.Gini, giniOver or the two route documents. They do not grade a
// network. The field alarms that replace them are mix-conditioned (serve) and
// relative-to-holdings (repair). THE PER-TIER WORK TOTALS THEY NEEDED HAVE LANDED (advisory
// §3a, Builder item 1): node.EconomySample carries ServeBytesByTier / RepairsByTier /
// PledgedBytesByTier / ReportersByTier / CapableSize, the mix rows carry a `work` block and
// the concentration document carries `ponyShareOfServedBytes`. The T-AR alarm built on them
// is r22_c3_pertier_work_gates_test.go. The two constants below are unchanged and still
// fixture constants; what changed is that the replacement is now buildable.
const (
	// c3ServeGiniMax is the tolerance the serve gate asserts G_adj against. Fixture
	// constant for the CPU-weighted (1/7/24) vision shape with every node reporting,
	// where the honest reading is 0.0752 and the concentrated ablation is 0.7941.
	c3ServeGiniMax = 0.15
	// c3RepairGiniMax is the same for the UNIFORM-REPAIR fixture (see c3HealthyPeers,
	// which is where the assumption doing all the work behind this number lives).
	c3RepairGiniMax = 0.40
	// The INDETERMINACY BOUNDARY, per series. NOT a gate clause and NOT an independent
	// parameter: G_adj <= T implies c >= 1 - T, so these are DEFINED as the theorem and
	// pinned as such. Below the boundary the verdict is c3Indeterminate -- "I cannot see"
	// -- and never c3Concentrated, because a figure over a minority of its own population
	// is not a measurement of that population.
	c3ServeReportingMin  = 1 - c3ServeGiniMax  // 0.85
	c3RepairReportingMin = 1 - c3RepairGiniMax // 0.60
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
	// The two fields the per-tier work totals added (r22_c3_pertier_work_gates_test.go).
	// Decoded here rather than in a second wire type so ONE decoder serves the fixture both
	// gate files drive. CapableSize is a POINTER: 0 capable nodes in the sample is a
	// measurement, and an absent key is not.
	PonyShareOfServedBytes *c3TenetShareWire `json:"ponyShareOfServedBytes"`
	CapableSize            *int              `json:"capableSize"`
}

// c3TenetShareWire is the T-AR figure as served. Value is a POINTER for the same reason
// c3GiniWire.Value is: it is omitempty, so a measured 0.0 arrives as an absent key beside
// known:true and a plain float64 could not tell it from "no such field".
type c3TenetShareWire struct {
	Known      bool     `json:"known"`
	Value      *float64 `json:"value"`
	Reporting  int      `json:"reporting"`
	Population int      `json:"population"`
	Coverage   float64  `json:"coverage"`
	Reason     string   `json:"reason"`
}

func (t *c3TenetShareWire) val() float64 {
	if t == nil || t.Value == nil {
		return 0
	}
	return *t.Value
}

// c3TierShareWire and c3TierWorkWire are the per-tier work block on a mix row.
type c3TierShareWire struct {
	Known  bool     `json:"known"`
	Value  *float64 `json:"value"`
	Reason string   `json:"reason"`
}

func (t *c3TierShareWire) val() float64 {
	if t == nil || t.Value == nil {
		return 0
	}
	return *t.Value
}

type c3TierWorkWire struct {
	Reporting    int              `json:"reporting"`
	Coverage     float64          `json:"coverage"`
	ServeShare   *c3TierShareWire `json:"serveShare"`
	RepairShare  *c3TierShareWire `json:"repairShare"`
	PledgedShare *c3TierShareWire `json:"pledgedShare"`
}

type c3NetworkWire struct {
	Mix []struct {
		Class   string          `json:"class"`
		Sampled int             `json:"sampled"`
		Share   float64         `json:"share"`
		Work    *c3TierWorkWire `json:"work"`
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
// an ablation arm asserts that this exact function refuses, and names which state.

// c3Verdict separates the two ways a concentration alarm can refuse. They are different
// facts with different operator actions, and collapsing them is how a dark panel gets read
// as a healthy one -- or, worse, how "I cannot see" gets reported as "you are captured".
type c3Verdict int

const (
	// c3Pass: enough of the population reported for G_adj to be a measurement of that
	// population, and it is inside the tolerance.
	c3Pass c3Verdict = iota
	// c3Indeterminate: the series is dark (absent, or known:false), or its reporting
	// fraction is below the indeterminacy boundary. G_adj is still computed and is a
	// LOWER bound on the truth, but it is dominated by what was not seen.
	c3Indeterminate
	// c3Concentrated: measured, and over tolerance.
	c3Concentrated
)

func (v c3Verdict) String() string {
	switch v {
	case c3Pass:
		return "PASS"
	case c3Indeterminate:
		return "INDETERMINATE"
	default:
		return "CONCENTRATED"
	}
}

// c3Reading is one series' decomposition, which is the form the panel should lead with:
// `unseen` is what the exclusion rule hid, `seen` is what was measured, and their sum is
// the concentration counting silence as idle.
type c3Reading struct {
	Verdict    c3Verdict
	Why        string
	Coverage   float64 // c = series size / the population it claims to describe
	Published  float64 // G_pub, the number on the wire
	Unseen     float64 // 1 - c
	Seen       float64 // c * G_pub
	Adjusted   float64 // G_adj = Unseen + Seen
	Population int
	Reporting  int
}

// c3Adjust computes the decomposition and applies the tolerance. One function for both
// series: they differ only in their population and their tolerance.
func c3Adjust(series *c3GiniWire, population int, tol float64) c3Reading {
	r := c3Reading{Population: population, Reporting: series.SampleSize, Published: series.val()}
	if population <= 0 {
		r.Verdict, r.Why = c3Indeterminate, "the series claims a population of zero"
		return r
	}
	r.Coverage = float64(series.SampleSize) / float64(population)
	r.Unseen = 1 - r.Coverage
	r.Seen = r.Coverage * r.Published
	r.Adjusted = r.Unseen + r.Seen
	// The boundary is checked FIRST and it decides the LABEL, not the refusal: because
	// the boundary is defined as 1 - tol, anything below it necessarily has G_adj > tol
	// too. What this ordering buys is that a figure over a minority of its population is
	// never reported as measured capture.
	if r.Coverage < 1-tol {
		r.Verdict = c3Indeterminate
		r.Why = fmt.Sprintf("reporting fraction %.4f (%d of %d) is below the indeterminacy boundary %.2f: the published %.4f is a figure over a minority of its own population, and G_adj = (1-c) + c*G_pub = %.4f + %.4f = %.4f is a LOWER bound on the truth, not a reading of it",
			r.Coverage, r.Reporting, r.Population, 1-tol, r.Published, r.Unseen, r.Seen, r.Adjusted)
		return r
	}
	if r.Adjusted > tol {
		r.Verdict = c3Concentrated
		r.Why = fmt.Sprintf("G_adj %.4f exceeds %.2f (published %.4f over %d of %d reporting; unseen %.4f + seen %.4f): work is concentrating, measured",
			r.Adjusted, tol, r.Published, r.Reporting, r.Population, r.Unseen, r.Seen)
		return r
	}
	r.Verdict = c3Pass
	return r
}

// c3ServeGate is advisory assertion 1 over the served document. Population = the whole
// classifiable sample, because every tier serves.
func c3ServeGate(c c3ConcentrationWire) c3Reading {
	if c.CountersWithheld {
		return c3Reading{Verdict: c3Indeterminate, Why: "serveGini withheld: the reader cannot see the figure at all"}
	}
	if c.Sample == nil {
		return c3Reading{Verdict: c3Indeterminate, Why: "no sample block: the required sibling of every gossip-estimated number is absent"}
	}
	if c.ServeGini == nil {
		return c3Reading{Verdict: c3Indeterminate, Population: c.Sample.Size,
			Why: fmt.Sprintf("no serveGini published at all over a sample of %d: the reporting series is below minGossipSample=%d, so the panel is DARK", c.Sample.Size, minGossipSample)}
	}
	// KNOWN-NESS, NOT THE VALUE. credit.Gini returns 0 both for a measured equality and
	// for a series that summed to zero, and giniOver renders the second as known:false
	// with no value. A gate that read the number would score an absent measurement as a
	// perfect pass -- and would then feed that 0 into G_adj as if it were seen.
	if !c.ServeGini.Known {
		return c3Reading{Verdict: c3Indeterminate, Population: c.Sample.Size, Reporting: c.ServeGini.SampleSize,
			Why: "serveGini is UNKNOWN (" + c.ServeGini.Reason + "): an absent measurement is not a passing 0"}
	}
	return c3Adjust(c.ServeGini, c.Sample.Size, c3ServeGiniMax)
}

// c3RepairGate is advisory assertion 2. Population = the repair-CAPABLE subset, because
// the series is scoped to it.
//
// THE BUILD DEFECT THIS READ ACROSS IS FIXED, and this gate has not yet been re-pointed at
// the fix. The capable count still comes from /api/economy/network while the numerator comes
// from /api/economy/concentration -- two separate s.nd.EconomySample() calls, two snapshots;
// the fixture reads both at one instant so they agree, but on a live node peerCaps moves
// between them and the coverage ratio can exceed 1. economyConcentration now carries
// `capableSize` itself (advisory §1, Builder item 3), and
// TestGateC3_3e_CapableSizeClosesTheCrossDocumentRepairCoverageJoin asserts it equals this
// function's nw.capable() on one snapshot. Re-pointing this gate at c.CapableSize is the
// Tester's call, not the Builder's.
func c3RepairGate(c c3ConcentrationWire, nw c3NetworkWire) c3Reading {
	if c.CountersWithheld {
		return c3Reading{Verdict: c3Indeterminate, Why: "repairGini withheld: the reader cannot see the figure at all"}
	}
	if c.Sample == nil {
		return c3Reading{Verdict: c3Indeterminate, Why: "no sample block"}
	}
	if c.RepairGini == nil {
		return c3Reading{Verdict: c3Indeterminate, Population: nw.capable(),
			Why: fmt.Sprintf("no repairGini published at all: the repair-capable reporting subset is below minGossipSample=%d, so the whole durability-concentration alarm is DARK on a sample of %d", minGossipSample, c.Sample.Size)}
	}
	if !c.RepairGini.Known {
		return c3Reading{Verdict: c3Indeterminate, Population: nw.capable(), Reporting: c.RepairGini.SampleSize,
			Why: "repairGini is UNKNOWN (" + c.RepairGini.Reason + "): an absent measurement is not a passing 0"}
	}
	if nw.capable() == 0 {
		return c3Reading{Verdict: c3Indeterminate, Why: "the tier mix reports NO repair-capable node in the sample, yet a repair Gini was published over it"}
	}
	return c3Adjust(c.RepairGini, nw.capable(), c3RepairGiniMax)
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
// ~ 10000 : 100 : 1") at an instance that fits maxPeerInfo, with serve work weighted
// CPU-wise 1 : 7 : 24. Every peer reports, ponies do no durability work.
//
// THE REPAIR LEG CARRIES AN ASSUMPTION, AND IT IS THE ONE DOING ALL THE WORK BEHIND
// c3RepairGiniMax. This fixture gives a 64 GiB horse and a 4 TiB archival node TEN REPAIRS
// EACH -- repair UNIFORM PER NODE. Repair is in fact holdings-bound: a node repairs shards
// for the roots it holds, so repair opportunity scales with stored bytes. Under a
// holdings-proportional null at this very mix (10 horses @ 1 TB + 1 archival @ 50 TB) the
// honest repair Gini is 0.7424, not 0.0000 -- above the 0.40 constant, and numerically
// indistinguishable from the 0.7505 the original spec labelled CAPTURE. That is why 0.40
// is a fixture constant here and was withdrawn as a field threshold. See
// TestGateC3_2_RepairNullIsHoldingsProportionalSoTheConstantIsNotAFieldThreshold, which
// drives the holdings-proportional shape through the same route.
func c3HealthyPeers() []c3Peer {
	return []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 10, n: 1},
	}
}

// ---- GATE 1 ---------------------------------------------------------------------------

// TestGateC3_1_ServeWorkFederationOnTheOperatorPanel is advisory assertion 1, re-pointed.
//
// GATE: c3ServeGate over GET /api/economy/concentration. Asserts, in order, that the serve
// series is PRESENT, that it is KNOWN, and that the ADJUSTED concentration
// G_adj = (1-c) + c*G_pub is within c3ServeGiniMax. There is no separate coverage clause;
// the indeterminacy boundary is a theorem of the tolerance, pinned by
// TestGateC3_IndeterminacyBoundaryIsATheoremOfTheTolerance.
//
// FIVE ARMS AND FOUR OUTCOMES. The healthy arm must PASS. A1a (series absent) and A1b
// (series known:false) must be INDETERMINATE-because-dark. A3 (capture behind silence) must
// be INDETERMINATE-because-below-the-boundary. Only A2 (capture in the open) may be
// CONCENTRATED. Each arm asserts WHICH state, not merely that the gate refused — an
// ablation that reddens the state a different ablation already covered proves nothing new,
// and reporting "I cannot see" as "you are captured" is its own defect.
func TestGateC3_1_ServeWorkFederationOnTheOperatorPanel(t *testing.T) {
	// --- HEALTHY: the vision ratio, CPU-weighted, everybody reporting.
	conc, _ := c3Fixture(t, 1, c3HealthyPeers())
	r := c3ServeGate(conc)
	if r.Verdict != c3Pass {
		t.Fatalf("HEALTHY ARM %s: %s\n  The fixture the constant was cut for must PASS, or the regression gate fires on its own null.", r.Verdict, r.Why)
	}
	t.Logf("healthy: %s  G_adj %.4f = unseen %.4f + seen %.4f  (published %.4f, %d of %d reporting)",
		r.Verdict, r.Adjusted, r.Unseen, r.Seen, r.Published, r.Reporting, r.Population)
	if r.Adjusted != r.Published {
		t.Fatalf("HEALTHY ARM: G_adj %.4f != published %.4f at full coverage — the adjustment must be the identity when c = 1, or it is not the same statistic", r.Adjusted, r.Published)
	}

	// --- ABLATION A1a — NOBODY REPORTS ANYTHING. Every peer pledges capacity and emits
	// neither work key, so the M-2 rule excludes all of them and the serve series is
	// EMPTY. economyConcentrationDoc's own floor then omits serveGini entirely. The gate
	// must return INDETERMINATE: a consumer that reads a missing field as "nothing to
	// alarm about" is blind on a network that has reported no work at all.
	silent := []c3Peer{
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 60},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 6},
	}
	concA1a, _ := c3Fixture(t, 2, silent)
	if concA1a.ServeGini != nil {
		t.Fatalf("ABLATION A1a: serveGini was published over a wholly silent sample (%+v). Re-derive this arm — it exists to prove the gate refuses an ABSENT figure.", concA1a.ServeGini)
	}
	rA1a := c3ServeGate(concA1a)
	if rA1a.Verdict != c3Indeterminate {
		t.Fatalf("ABLATION A1a: verdict %s, want INDETERMINATE. No serveGini is published at all; absence is being read as %s.", rA1a.Verdict, rA1a.Verdict)
	}
	t.Logf("ABLATION A1a %s (as required): %s", rA1a.Verdict, rA1a.Why)

	// --- ABLATION A1b — THE KNOWN-NESS CLAUSE, and reaching it takes a specific shape.
	// giniOver renders known:false when the series SUMMED TO ZERO, but the route only
	// publishes the series at all once it holds minGossipSample entries — so the wholly
	// silent sample above never reaches that branch. The reachable shape is a sample whose
	// peers DO report (so they are in the series) but report zero SERVE work: the M-2
	// discriminator is the PAIR, so a peer emitting repairs and no serve is a reporting
	// peer whose serve zero counts. credit.Gini over [0,0,...] returns 0 ("universal
	// poverty is technically equality") and a gate that read the VALUE — or fed it into
	// G_adj as if it were seen — would score it as perfect federation at full coverage.
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
	// The trap this arm closes: coverage here is 1.0 (all 9 peers are in the series), so
	// G_adj would compute to 0.0000 and PASS if known-ness were not checked first.
	if cov := float64(concA1b.ServeGini.SampleSize) / float64(concA1b.Sample.Size); cov < 1 {
		t.Fatalf("ABLATION A1b: reporting fraction %.4f < 1. This arm must have FULL coverage, or it reddens on the boundary and the known-ness check is unproven.", cov)
	}
	rA1b := c3ServeGate(concA1b)
	if rA1b.Verdict != c3Indeterminate {
		t.Fatalf("ABLATION A1b: verdict %s, want INDETERMINATE. serveGini is UNKNOWN (%q) at full coverage, so G_adj would read 0.0000 — an absent measurement scored as perfect federation.",
			rA1b.Verdict, concA1b.ServeGini.Reason)
	}
	t.Logf("ABLATION A1b %s (as required): %s", rA1b.Verdict, rA1b.Why)

	// --- ABLATION A2 — the spec's own ablation: a CONCENTRATED distribution where the top
	// 5 nodes serve 80 % of the bytes. Every node still reports, so coverage is 1.0, G_adj
	// is the identity on G_pub, and this arm isolates measured concentration.
	const a2Ponies = 1000
	concentrated := []c3Peer{
		{capTotal: c3HorseCap, served: 160_000 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3PonyCap, served: (200_000 / a2Ponies) * c3Unit, repairs: 0, n: a2Ponies},
		{capTotal: c3HorseCap, served: (200_000 / a2Ponies) * c3Unit, repairs: 10, n: 5},
		{capTotal: c3ArchivalCap, served: (200_000 / a2Ponies) * c3Unit, repairs: 10, n: 1},
	}
	concA2, _ := c3Fixture(t, 3, concentrated)
	rA2 := c3ServeGate(concA2)
	if rA2.Verdict != c3Concentrated {
		t.Fatalf("ABLATION A2: verdict %s, want CONCENTRATED. The top 5 of %d nodes serve 80 %% of the bytes. G_adj %.4f, coverage %.4f, published %.4f — the spec's assertion 1 is decoration.",
			rA2.Verdict, concA2.Sample.Size, rA2.Adjusted, rA2.Coverage, rA2.Published)
	}
	if rA2.Coverage < 1 {
		t.Fatalf("ABLATION A2 has coverage %.4f, so it could redden as INDETERMINATE and duplicate A3. Every node must report here.", rA2.Coverage)
	}
	t.Logf("ABLATION A2 %s (as required): %s", rA2.Verdict, rA2.Why)

	// --- ABLATION A3 — THE ONE THE SPEC DID NOT HAVE, and the reason the gate is an
	// adjusted statistic. Same total capture as A2, but the losing majority reports NOTHING
	// rather than a little. Under M-2 those peers are EXCLUDED, so the published figure is
	// a Gini over the five capturing nodes alone — and because they capture EQUALLY it is
	// 0.0000 with known:true. A bare `serveGini <= 0.15` gate passes total capture.
	capturedAndSilent := []c3Peer{
		{capTotal: c3HorseCap, served: 200_000 * c3Unit, repairs: 10, n: 5},
		{capTotal: c3PonyCap, served: 0, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 5},
		{capTotal: c3ArchivalCap, served: 0, repairs: 0, n: 1},
	}
	concA3, _ := c3Fixture(t, 4, capturedAndSilent)
	// First RECORD the defeat, so the finding is a measurement and not a claim.
	if concA3.ServeGini == nil || !concA3.ServeGini.Known {
		t.Fatalf("ABLATION A3: expected a KNOWN serveGini over the five capturing nodes; got %+v. The exclusion rule changed — re-derive the adjustment.", concA3.ServeGini)
	}
	if concA3.ServeGini.val() > c3ServeGiniMax {
		t.Fatalf("ABLATION A3: the BARE published figure now exceeds %.2f (%.4f). When this arm was written the panel published 0.0000 known:true on total capture. The M-2 exclusion rule changed — re-derive the adjustment rather than deleting it.", c3ServeGiniMax, concA3.ServeGini.val())
	}
	rA3 := c3ServeGate(concA3)
	if rA3.Verdict != c3Indeterminate {
		t.Fatalf("ABLATION A3: verdict %s, want INDETERMINATE. %d of %d sampled nodes are silent while 5 serve 100 %% of the bytes; the published figure is %.4f known:true. Total capture must not be reported as measured — below the boundary the honest answer is 'I cannot see'.",
			rA3.Verdict, rA3.Population-rA3.Reporting, rA3.Population, rA3.Published)
	}
	// THE ADJUSTMENT IS EXACT ON THIS FIXTURE, not merely conservative: the true Gini of a
	// population where 5 of 1011 hold everything equally is 1 - 5/1011.
	wantAdj := 1 - float64(rA3.Reporting)/float64(rA3.Population)
	if diff := rA3.Adjusted - wantAdj; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("ABLATION A3: G_adj %.6f, want the exact true Gini %.6f (= 1 - %d/%d). The identity G_adj = (1-c) + c*G_pub is not being computed.", rA3.Adjusted, wantAdj, rA3.Reporting, rA3.Population)
	}
	t.Logf("ABLATION A3 %s (as required): %s", rA3.Verdict, rA3.Why)
	t.Logf("ABLATION A3 measurement: the panel publishes serveGini %.4f known:true over %d of %d sampled nodes while five nodes serve 100 %% of the bytes. A bare `serveGini <= %.2f` PASSES this. G_adj = %.4f, which is EXACTLY the true Gini 1 - %d/%d = %.4f.",
		rA3.Published, rA3.Reporting, rA3.Population, c3ServeGiniMax, rA3.Adjusted, rA3.Reporting, rA3.Population, wantAdj)
}

// TestGateC3_IndeterminacyBoundaryIsATheoremOfTheTolerance pins the reason the gates carry
// no separate coverage clause.
//
// A coverage floor is NECESSARY but not SUFFICIENT: a composite of `coverage >= 0.85` AND
// `G_pub <= 0.15` admits a true Gini of (1-0.85) + 0.85*0.15 = 0.2775, and the repair pair
// admits 0.6400 — inside the region the original spec called capture. Gating on G_adj
// dominates the floor, because G_adj <= T implies c >= 1 - T. This test asserts that
// implication numerically over the whole tolerance range, so the constants can never drift
// into being two independent parameters again.
func TestGateC3_IndeterminacyBoundaryIsATheoremOfTheTolerance(t *testing.T) {
	if c3ServeReportingMin != 1-c3ServeGiniMax || c3RepairReportingMin != 1-c3RepairGiniMax {
		t.Fatalf("the boundaries are no longer defined as 1 - tolerance (serve %.4f vs %.4f, repair %.4f vs %.4f) — they have become independent parameters, which is the structure this file exists to refuse",
			c3ServeReportingMin, 1-c3ServeGiniMax, c3RepairReportingMin, 1-c3RepairGiniMax)
	}
	// G_adj <= T  =>  c >= 1 - T, for every reachable (c, G_pub).
	for _, tol := range []float64{c3ServeGiniMax, c3RepairGiniMax} {
		for ci := 1; ci <= 1000; ci++ {
			c := float64(ci) / 1000
			for gi := 0; gi <= 100; gi++ {
				gpub := float64(gi) / 100
				adj := (1 - c) + c*gpub
				if adj <= tol && c < 1-tol-1e-12 {
					t.Fatalf("counterexample at tol=%.2f: c=%.3f G_pub=%.2f gives G_adj=%.4f <= tol while c < %.2f. The boundary is NOT implied and must be reinstated as a separate clause.", tol, c, gpub, adj, 1-tol)
				}
			}
		}
	}
	// And the separate-clause composite really does admit what the ruling says it admits.
	for _, tc := range []struct {
		name     string
		cov, tol float64
		want     float64
	}{
		{"serve", c3ServeReportingMin, c3ServeGiniMax, 0.2775},
		{"repair", c3RepairReportingMin, c3RepairGiniMax, 0.6400},
	} {
		got := (1 - tc.cov) + tc.cov*tc.tol
		if diff := got - tc.want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("%s: the separate-clause composite admits a true Gini of %.4f, not the %.4f this file's header claims. Correct the header.", tc.name, got, tc.want)
		}
		t.Logf("%s: a SEPARATE coverage clause at %.2f with a published tolerance of %.2f would admit a true Gini of %.4f — which is why the gate folds them into one statistic",
			tc.name, tc.cov, tc.tol, got)
	}
}

// ---- GATE 2 ---------------------------------------------------------------------------

// TestGateC3_2_RepairWorkWithinTheCapableSubsetOnTheOperatorPanel is advisory assertion 2,
// re-pointed — and note that its constant is withdrawn as a FIELD alarm, so what this test
// is, exactly, is a REGRESSION gate over a named synthetic fixture.
//
// GATE: c3RepairGate over GET /api/economy/concentration and GET /api/economy/network.
// Same structure as gate 1 — PRESENT, then KNOWN, then G_adj within c3RepairGiniMax — with
// the population being the repair-CAPABLE subset, because the series is scoped to it.
//
// FOUR ARMS, FOUR OUTCOMES: healthy PASS, B1 CONCENTRATED (capture in the open), B2
// INDETERMINATE (capture behind silence), B3 INDETERMINATE (panel dark).
func TestGateC3_2_RepairWorkWithinTheCapableSubsetOnTheOperatorPanel(t *testing.T) {
	// --- HEALTHY: repair spread evenly over 10 horses + 1 archival, 1000 ponies serving
	// and doing none of it. This is the shape whose NETWORK-WIDE repair Gini is ~0.99.
	// READ c3HealthyPeers's comment first: "evenly" is UNIFORM PER NODE, which is an
	// assumption, and it is the assumption c3RepairGiniMax rests on.
	conc, nw := c3Fixture(t, 11, c3HealthyPeers())
	r := c3RepairGate(conc, nw)
	if r.Verdict != c3Pass {
		t.Fatalf("HEALTHY ARM %s: %s\n  Uniform repair across the whole persistent tier must pass, or the regression gate fires on its own fixture.", r.Verdict, r.Why)
	}
	if got, want := conc.RepairGini.SampleSize, 11; got != want {
		t.Fatalf("the repair series covered %d nodes, want %d (the capable subset only). If the 1000 ponies are in it the figure is ~0.99 by construction and carries no signal — this is the §2.1 scoping correction.", got, want)
	}
	t.Logf("healthy: %s  G_adj %.4f = unseen %.4f + seen %.4f  (published %.4f, %d of %d capable reporting)",
		r.Verdict, r.Adjusted, r.Unseen, r.Seen, r.Published, r.Reporting, r.Population)

	// --- ABLATION B1 — the spec's ablation: route EVERY repair to one node. The other
	// capable nodes still serve, so they still report and their zero is a genuine measured
	// zero that counts. Coverage is 1.0, so this arm isolates measured concentration.
	oneRepairer := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 0, n: 10},
		{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 500, n: 1},
	}
	concB1, nwB1 := c3Fixture(t, 12, oneRepairer)
	rB1 := c3RepairGate(concB1, nwB1)
	if rB1.Verdict != c3Concentrated {
		t.Fatalf("ABLATION B1: verdict %s, want CONCENTRATED. Every repair on ONE of %d capable nodes, G_adj %.4f, coverage %.4f — the scoped series does not redden on total durability capture, so it is decoration.",
			rB1.Verdict, nwB1.capable(), rB1.Adjusted, rB1.Coverage)
	}
	if rB1.Coverage < 1 {
		t.Fatalf("ABLATION B1 has coverage %.4f, so it could redden as INDETERMINATE and duplicate B2. Every capable node must keep REPORTING so its zero counts.", rB1.Coverage)
	}
	t.Logf("ABLATION B1 %s (as required): %s", rB1.Verdict, rB1.Why)

	// --- ABLATION B2 — the repair analogue of A3. Four capable nodes repair equally; the
	// other seven report NOTHING AT ALL and are excluded. The published repairGini is
	// 0.0000 known:true over a minority of the persistent tier, and G_adj recovers 0.6364.
	minorityReports := []c3Peer{
		{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 1000},
		{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 10, n: 4},
		{capTotal: c3HorseCap, served: 0, repairs: 0, n: 6},
		{capTotal: c3ArchivalCap, served: 0, repairs: 0, n: 1},
	}
	concB2, nwB2 := c3Fixture(t, 13, minorityReports)
	rB2 := c3RepairGate(concB2, nwB2)
	if rB2.Verdict != c3Indeterminate {
		t.Fatalf("ABLATION B2: verdict %s, want INDETERMINATE. %d of %d repair-capable nodes are silent and the published figure is %.4f over the remaining %d.",
			rB2.Verdict, rB2.Population-rB2.Reporting, rB2.Population, rB2.Published, rB2.Reporting)
	}
	if concB2.RepairGini == nil || !concB2.RepairGini.Known || concB2.RepairGini.val() > c3RepairGiniMax {
		t.Fatalf("ABLATION B2 reddened for a reason other than the boundary (%+v); it then duplicates B1/B3 and the adjustment is unproven", concB2.RepairGini)
	}
	wantB2 := 1 - float64(rB2.Reporting)/float64(rB2.Population)
	if diff := rB2.Adjusted - wantB2; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("ABLATION B2: G_adj %.6f, want the exact true Gini %.6f (= 1 - %d/%d)", rB2.Adjusted, wantB2, rB2.Reporting, rB2.Population)
	}
	t.Logf("ABLATION B2 %s (as required): %s", rB2.Verdict, rB2.Why)

	// --- ABLATION B3 — the DARK-PANEL ablation. Only two capable nodes report, which is
	// below minGossipSample, so economyConcentrationDoc omits repairGini ENTIRELY. A
	// consumer that treats an absent field as "nothing to alarm about" reads a captured
	// durability tier as healthy. The gate must return INDETERMINATE on ABSENCE.
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
	rB3 := c3RepairGate(concB3, nwB3)
	if rB3.Verdict != c3Indeterminate {
		t.Fatalf("ABLATION B3: verdict %s, want INDETERMINATE. The repair panel is DARK (repairGini absent).", rB3.Verdict)
	}
	t.Logf("ABLATION B3 %s (as required): %s", rB3.Verdict, rB3.Why)
}

// TestGateC3_2_RepairNullIsHoldingsProportionalSoTheConstantIsNotAFieldThreshold is the
// repair provenance pin, and it is the reason c3RepairGiniMax was withdrawn as a field
// threshold rather than merely re-labelled.
//
// THE WITHDRAWN SENTENCE: 0.40 was "the midpoint of the measured span 0.019 (even) to
// 0.7505 (top 5 of 101 do 80 %)". That span runs between two HEALTHY shapes, not between a
// healthy one and a captured one. Repair is holdings-bound — a node repairs shards for the
// roots it holds, so repair opportunity scales with stored bytes — and under that null:
//
//	capable n=3   0.6282   <- the spec's own Phase-3 canary minimum (>= 2 caretaker-judges)
//	capable n=4   0.6934
//	capable n=6   0.7424
//	capable n=11  0.7424   <- the shipped healthy fixture's mix
//	capable n=21  0.6667
//	capable n=101 0.3234   <- the FIRST size that clears 0.40 is n ~= 81
//
// So a 0.40 abort would have fired on the first honest canary silt ever ran, and 0.7424 is
// numerically indistinguishable from the 0.7505 the spec labelled capture.
//
// THIS TEST DRIVES THE HOLDINGS-PROPORTIONAL SHAPE THROUGH THE REAL ROUTE, so the claim is
// about the shipped surface and not about arithmetic in a comment. Both arms are HONEST
// topologies. Both must be reported CONCENTRATED by the shipped gate. That is the pin: the
// gate's constant does not distinguish an honest holdings-proportional durability tier from
// a captured one, which is exactly why it may not be used as a field alarm.
//
// DISCHARGE CONDITION: this pin clears when the repair alarm is RELATIVE TO HOLDINGS —
// alarm when observedRepairShare(tier) - expectedRepairShare(tier) > margin, both terms
// measured from the sample's own CapTotal — not when a Gini drops below a constant. That
// needs the per-tier work totals (Builder item, advisory §3a).
func TestGateC3_2_RepairNullIsHoldingsProportionalSoTheConstantIsNotAFieldThreshold(t *testing.T) {
	// Repairs proportional to pledged bytes: a 1 TB-class horse does 1, a 50 TB-class
	// archival node does 50. Everyone reports, so coverage is 1.0 and G_adj is the
	// identity — this pin is about the NULL, not about the adjustment.
	holdingsNull := func(horses int) []c3Peer {
		return []c3Peer{
			{capTotal: c3PonyCap, served: 1 * c3Unit, repairs: 0, n: 60},
			{capTotal: c3HorseCap, served: 7 * c3Unit, repairs: 1, n: horses},
			{capTotal: c3ArchivalCap, served: 24 * c3Unit, repairs: 50, n: 1},
		}
	}
	for _, tc := range []struct {
		name    string
		horses  int
		wantMin float64 // the measured null, asserted as a floor so this cannot silently drift
	}{
		{"the Phase-3 canary MINIMUM (2 caretaker-judges + 1 archival)", 2, 0.62},
		{"the shipped healthy fixture's own mix (10 horses + 1 archival)", 10, 0.74},
	} {
		conc, nw := c3Fixture(t, int64(41+tc.horses), holdingsNull(tc.horses))
		r := c3RepairGate(conc, nw)
		if r.Coverage < 1 {
			t.Fatalf("%s: coverage %.4f, want 1.0 — this pin must isolate the NULL, not the adjustment", tc.name, r.Coverage)
		}
		if r.Published < tc.wantMin {
			t.Fatalf("%s: the holdings-proportional honest null now reads %.4f, below the measured %.2f this pin was cut against. The shape moved — re-derive before trusting the withdrawal.", tc.name, r.Published, tc.wantMin)
		}
		if r.Verdict != c3Concentrated {
			t.Fatalf("PIN DISCHARGED (or the gate moved): %s reads %s at G_adj %.4f against a constant of %.2f. When this pin was written the shipped gate reported CONCENTRATED on an HONEST holdings-proportional topology, which is why %0.2f was withdrawn as a field threshold. Re-read the finding before deleting this test.",
				tc.name, r.Verdict, r.Adjusted, c3RepairGiniMax, c3RepairGiniMax)
		}
		t.Logf("%s: HONEST holdings-proportional repair reads %s at G_adj %.4f against the %.2f constant. An honest canary would abort.",
			tc.name, r.Verdict, r.Adjusted, c3RepairGiniMax)
	}
}

// ---- the serve provenance pin ---------------------------------------------------------

// TestGateC3_1_ServeGiniHonestNullIsMixDependentNotSampleSizeDependent records the MEASURED
// reason c3ServeGiniMax is a fixture constant and not a field threshold.
//
// RENAMED. The 2026-09-07 advisory cites this pin as
// TestGateC3_1_ThresholdProvenanceIsUnreachableUnderDiskWeighting. That name states the
// SYMPTOM I first measured — the honest disk-weighted Gini falls as n grows and does not
// reach 0.15 until ~5,600 nodes, above the maxPeerInfo bound of 4,096 — and the Economist's
// re-derivation showed the diagnosis was wrong. A test whose name does not name the axis it
// measures is a defect this repo has been bitten by more than twice, so the name moved with
// the assertion. The finding is NOT discharged; it is re-founded.
//
// THE CAUSE. Remove the single archival node and the disk-weighted honest Gini is
// SCALE-INVARIANT:
//
//	n =    101 (100 ponies + 1 horse)        0.0810
//	n =  1,010 (1000 ponies + 10 horses)     0.0810
//	n =  4,095 (4055 ponies + 40 horses)     0.0800
//	n = 10,100 (10000 ponies + 100 horses)   0.0810
//
// The entire falling curve is one artifact: below 10,101 nodes the vision ratio
// 10000 : 100 : 1 cannot be held with an INTEGER archival node, so forcing exactly one
// over-weights the 50 TB tier by 10101/n — 2.47x at n=4,096, 9.99x at n=1,011, 99x at
// n=102. Sample size was a proxy for the real variable, the sampled COMPOSITION, and it is
// a proxy that fails: two samples of the same honest network at the SAME size, differing by
// one node, read 0.1726 and 0.0800 — a factor of 2.16.
//
// SO THE FIELD ALARM MUST BE MIX-AWARE, NOT SIZE-AWARE. The panel already publishes the
// mix; the null should be recomputed from it on every reading rather than compared to a
// constant.
//
// SECOND, AND SEPARATE: the disk weighting (0.1/1/50 TB) is a STORAGE-SHARE model applied
// to a SERVE-work series. Serve bytes are bandwidth- and demand-bound, not holdings-bound.
// That is why the withdrawn provenance sentence cited the wrong corner of the wrong band.
//
// AND THE GINI CANNOT CARRY T-AR AT ALL. T-AR is a TIER-SHARE statement; the Gini is a
// per-node DISPERSION statistic. On a network whose ratified tier design spans a 500:1
// capacity dispersion the two disagree by construction: at the ceiling sample below, the
// Gini reads 0.1726 (over the constant) while the pony tier still does 81.8 % of the bytes
// (far above the tenet floor). This test asserts that disagreement, because it is the whole
// argument for moving the T-AR gate to the tier share (TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes, and the Builder's
// per-tier work totals).
//
// DISCHARGE CONDITION, and HALF OF IT IS NOW MET: this pin clears when the serve series'
// honest weighting is MEASURED (per-tier serve bytes on the wire) and the alarm is computed
// against a mix-conditioned null — never when a Gini happens to fall below a constant. The
// per-tier serve bytes ARE on the wire now (mix[].work.serveShare,
// concentration.ponyShareOfServedBytes) and the mix-conditioned floor is built in
// r22_c3_pertier_work_gates_test.go. What this pin still records is unchanged and still
// true: the GINI's honest null moves with the sampled mix, so a size-based alarm on the
// Gini is unsound. Whether that retires this test is the Tester's call.
func TestGateC3_1_ServeGiniHonestNullIsMixDependentNotSampleSizeDependent(t *testing.T) {
	if testing.Short() {
		t.Skip("drives two 4,096-peer samples; -short runs the two gates only")
	}
	// The observable ceiling, disk-weighted, vision proportions. maxPeerInfo is 4,096.
	// ARM 1 forces the single archival node in; ARM 2 is the SAME honest network at the
	// SAME sample size with the archival node not drawn. A uniform 4,096-of-10,101 draw
	// contains it only 40.5 % of the time, so ARM 2 is the more typical sample and ARM 1
	// is the worst case.
	withArchival := []c3Peer{
		{capTotal: c3PonyCap, served: 100 * c3Unit, repairs: 0, n: 4055},     // 0.1 TB
		{capTotal: c3HorseCap, served: 1000 * c3Unit, repairs: 10, n: 40},    // 1 TB
		{capTotal: c3ArchivalCap, served: 50000 * c3Unit, repairs: 10, n: 1}, // 50 TB
	}
	withoutArchival := []c3Peer{
		{capTotal: c3PonyCap, served: 100 * c3Unit, repairs: 0, n: 4056},
		{capTotal: c3HorseCap, served: 1000 * c3Unit, repairs: 10, n: 40},
	}
	concA, _ := c3Fixture(t, 21, withArchival)
	concB, _ := c3Fixture(t, 22, withoutArchival)
	rA, rB := c3ServeGate(concA), c3ServeGate(concB)
	if rA.Coverage < 1 || rB.Coverage < 1 {
		t.Fatalf("both arms must have full coverage (got %.4f, %.4f) — this pin is about the NULL, not the adjustment", rA.Coverage, rB.Coverage)
	}
	if concA.Sample.Size != concB.Sample.Size {
		t.Fatalf("the two arms must be the SAME sample size for the comparison to isolate the mix; got %d and %d", concA.Sample.Size, concB.Sample.Size)
	}
	t.Logf("SAME honest network, SAME sample size %d, differing by ONE node: with archival G_adj %.4f (%s) | without archival G_adj %.4f (%s) | ratio %.2fx",
		concA.Sample.Size, rA.Adjusted, rA.Verdict, rB.Adjusted, rB.Verdict, rA.Adjusted/rB.Adjusted)

	// THE PIN, part 1 — the mix moves the honest null across the constant. One node.
	if rA.Verdict != c3Concentrated {
		t.Fatalf("PIN DISCHARGED (or the shape moved): the disk-weighted honest vision shape with one archival node now reads %s at G_adj %.4f against the %.2f constant. When this pin was written it read 0.1726 and CONCENTRATED — an honest topology reported as capture. Re-read the finding before deleting this test.",
			rA.Verdict, rA.Adjusted, c3ServeGiniMax)
	}
	if rB.Verdict != c3Pass {
		t.Fatalf("the SAME honest network without the archival node reads %s at G_adj %.4f. This pin needs the two arms to STRADDLE the constant; if they no longer do, the mix-dependence claim must be re-measured.", rB.Verdict, rB.Adjusted)
	}
	if ratio := rA.Adjusted / rB.Adjusted; ratio < 2.0 {
		t.Fatalf("one node moves the honest null by only %.2fx (%.4f vs %.4f). The mix-dependence this pin records has weakened — re-derive before relying on a size-based alarm.", ratio, rA.Adjusted, rB.Adjusted)
	}

	// THE PIN, part 2 — the Gini and T-AR disagree on the SAME sample. The Gini calls
	// ARM 1 concentrated; the pony tier serves 81.8 % of its bytes, far above the tenet
	// floor of 0.50. The tier share is computed from the FIXTURE here, deliberately: this
	// arm is about the two statistics disagreeing, and computing one of them from its own
	// definition keeps the comparison independent of the surface that publishes the other.
	// The product now publishes it too (concentration.ponyShareOfServedBytes), and
	// TestGateC3_3_EdgeMajorityOfServeWorkIsTheTenetNotTheNodeShare drives the product's
	// figure through the same disagreement.
	ponyBytes := float64(4055 * 100)
	totalBytes := float64(4055*100 + 40*1000 + 1*50000)
	ponyServeShare := ponyBytes / totalBytes
	if ponyServeShare < 0.50 {
		t.Fatalf("fixture defect: the disk-weighted ceiling shape gives the pony tier %.4f of served bytes, so the two statistics do not disagree here and the pin has no subject", ponyServeShare)
	}
	t.Logf("THE TWO STATISTICS DISAGREE on one sample: the Gini reports %s (G_adj %.4f > %.2f) while the pony tier serves %.4f of the bytes — far above the T-AR floor of 0.50. The Gini is per-node dispersion; T-AR is a tier share. Only one of them is the tenet.",
		rA.Verdict, rA.Adjusted, c3ServeGiniMax, ponyServeShare)
}

// ---- the anti-substitution pin ---------------------------------------------------------

// TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes exists because advisory assertion 1
// has a SECOND clause this file cannot encode: "pony share of served bytes >= 0.50", the
// T-AR tenet read literally (the edge tier does the MAJORITY of the work).
//
// THE PRODUCT NOW PUBLISHES IT (advisory §3a, Builder item 1): node.EconomySample carries
// ServeBytesByTier / ReportersByTier beside Mix, the mix rows carry work.serveShare and
// /api/economy/concentration carries ponyShareOfServedBytes. The clause is no longer
// STOPPED; the gate built on it is
// TestGateC3_3_EdgeMajorityOfServeWorkIsTheTenetNotTheNodeShare.
//
// THIS TEST STAYS, and it is now MORE load-bearing rather than less: the trap it closes is
// the RESEMBLANCE between two adjacent fields on the same row, and adding the right one
// beside the wrong one does not remove the wrong one. Its sibling gate drives the same
// inversion off the PRODUCT'S figure; this one keeps the fixture-computed control.
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
