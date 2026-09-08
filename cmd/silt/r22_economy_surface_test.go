package main

// R2.2 (Lane C3) — the four new economy routes, on the wire.
//
// The two properties the whole-surface scans (r29a_status_surface_test.go,
// r27_a4_wire_gate_test.go) CANNOT carry for these routes: those fixtures pin ONE
// instant, so the flow ring holds one sample and /api/economy/flows and
// /api/economy/g answer windowNotYetMeasured whatever the token — a clean walk over
// them there is vacuous. This file drives a REAL multi-sample window first, proves the
// figures are on the tokened wire, and only then walks the untokened surface for them.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// The fixture's figures, chosen so each is a value nothing else on the surface holds by
// coincidence and so the drain direction is unambiguous.
const (
	r22ServedUnits = int64(700_003) // -> funded (skim) = 700,003
	r22FirstBounty = int64(400_000) // cost-per-repair 400,000 at the window's start
	r22LaterBounty = int64(11_111)  // three of these -> cost declines -> g > 0
	r22ServedBytes = r22ServedUnits * econMintUnit
)

// economyRouteAt drives one GET on a route at a fixed wall instant.
func economyRouteAt(t *testing.T, s *uiServer, path string, at time.Time, tokened bool) string {
	t.Helper()
	s.now = func() time.Time { return at }
	h, ok := s.apiRoutes()["GET "+path]
	if !ok {
		t.Fatalf("no GET %s in apiRoutes — every route must be registered there or the whole-surface privacy gate never sees it", path)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil)
	if tokened {
		r.Header.Set("Authorization", "Bearer "+s.token)
	}
	w := httptest.NewRecorder()
	s.guard(h).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET %s: status %d (%s)", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

// TestR22FlowsAndGAreTokenGatedAcrossAMeasuredWindow is the non-vacuous half of the
// whole-surface examination that raised r29aWholeSurfaceGETRoutes from 7 to 11.
//
// ABLATIONS, each run RED before this shipped:
//
//	D1: make withheldEconomyFlows `return full` — the untokened walk reddens naming the
//	    pooled delta.
//	D2: keep the pooled row open and withhold only Objects — still RED, because on a
//	    one-object node the pooled delta IS the object's delta. That is the arm that
//	    matters: it is the exact mistake selfFunding shipped with.
//	D3: make withheldEconomyG `return full` — RED on the cost-per-repair figure.
func TestR22FlowsAndGAreTokenGatedAcrossAMeasuredWindow(t *testing.T) {
	s, led := statusServer(t)
	root := ports.Hash{0x22, 0x0F, 0x10}
	repairer := ports.NodeID{0xD1}
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root})
		led.RecordServeToObject(s.nd.ID(), ports.NodeID{0xC0}, root, ports.ChunkID{0x1}, r22ServedBytes)
		if got := led.PayBounty(root, repairer, r22FirstBounty); got != r22FirstBounty {
			t.Fatalf("fixture: first bounty paid %d, want %d", got, r22FirstBounty)
		}
	})

	// Four ring samples, one per flowSampleInterval, each with a bounty paid between —
	// so every sample-to-sample net delta is negative and the drain latch trips.
	at := s.started
	economyRouteAt(t, s, "/api/economy/flows", at, true) // sample 0
	for i := 0; i < flowDrainSamples; i++ {
		s.onLoop(func() {
			if got := led.PayBounty(root, repairer, r22LaterBounty); got != r22LaterBounty {
				t.Fatalf("fixture: bounty %d paid %d", i, got)
			}
		})
		at = at.Add(flowSampleInterval)
		economyRouteAt(t, s, "/api/economy/flows", at, true)
	}

	// --- the TOKENED document: the positive control, first, or the walk below proves
	// nothing (this is how every shipped gate has missed a field).
	var flows struct {
		Samples   int   `json:"samples"`
		WindowSec int64 `json:"windowSec"`
		NotYet    bool  `json:"windowNotYetMeasured"`
		Withheld  bool  `json:"detailWithheld"`
		Pooled    *struct {
			SkimIn    int64 `json:"skimIn"`
			BountyOut int64 `json:"bountyOut"`
			Net       int64 `json:"net"`
			Consec    int   `json:"consecutiveNegative"`
			Draining  bool  `json:"draining"`
		} `json:"pooled"`
		Objects []struct {
			Root      string `json:"root"`
			BountyOut int64  `json:"bountyOut"`
			Net       int64  `json:"net"`
			Draining  bool   `json:"draining"`
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/flows", at, true)), &flows); err != nil {
		t.Fatal(err)
	}
	wantOut := r22LaterBounty * int64(flowDrainSamples)
	if flows.NotYet || flows.Pooled == nil || flows.Samples != flowDrainSamples+1 {
		t.Fatalf("fixture is vacuous: tokened flows = %+v, want %d samples and a pooled row", flows, flowDrainSamples+1)
	}
	if flows.Pooled.BountyOut != wantOut || flows.Pooled.Net != -wantOut {
		t.Fatalf("pooled window = bountyOut %d net %d, want %d and %d. The WINDOW is the point: lifetime skimIn is hugely positive here and a node that was healthy for a year and is draining today must read as draining",
			flows.Pooled.BountyOut, flows.Pooled.Net, wantOut, -wantOut)
	}
	if flows.Pooled.Consec != flowDrainSamples || !flows.Pooled.Draining {
		t.Fatalf("pooled drain = %d consecutive / draining %v, want %d and true — %d consecutive negative samples is the advisory's drain signal",
			flows.Pooled.Consec, flows.Pooled.Draining, flowDrainSamples, flowDrainSamples)
	}
	if len(flows.Objects) != 1 || flows.Objects[0].Root != root.String() || flows.Objects[0].Net != -wantOut {
		t.Fatalf("tokened objects = %+v, want one row for %s with net %d", flows.Objects, root, -wantOut)
	}

	var g struct {
		Withheld  bool      `json:"detailWithheld"`
		NetAbsent string    `json:"networkNotKnowable"`
		Network   *struct{} `json:"network"`
		Objects   []struct {
			Root          string  `json:"root"`
			Known         bool    `json:"known"`
			G             float64 `json:"g"`
			Perpetual     bool    `json:"perpetualEarnable"`
			CostPerRepair int64   `json:"costPerRepair"`
			Reason        string  `json:"reason"`
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/g", at, true)), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Objects) != 1 || !g.Objects[0].Known {
		t.Fatalf("fixture is vacuous: tokened g = %+v, want one KNOWN row (a g rendered as unknown proves nothing about what is on the wire)", g.Objects)
	}
	// Cost fell from 400,000 (one repair) to (400,000+3*11,111)/4, so g must be POSITIVE
	// — the perpetual-earnable side of the D-S7 line.
	wantCost := (r22FirstBounty + r22LaterBounty*int64(flowDrainSamples)) / int64(flowDrainSamples+1)
	if g.Objects[0].CostPerRepair != wantCost || g.Objects[0].G <= 0 || !g.Objects[0].Perpetual {
		t.Fatalf("g row = %+v, want costPerRepair %d and g > 0 with perpetualEarnable", g.Objects[0], wantCost)
	}
	if g.Network != nil || g.NetAbsent == "" {
		t.Fatalf("network g = %v with reason %q. Row 5 is not computable from the two gossiped work fields (repairs-done is the DENOMINATOR and nothing gossips the numerator), so it must be ABSENT WITH ITS REASON, never estimated", g.Network, g.NetAbsent)
	}

	// --- the whole untokened GET surface must carry none of it, under any name.
	walked := 0
	for pattern, h := range s.apiRoutes() {
		method, path, _ := strings.Cut(pattern, " ")
		if method != http.MethodGet {
			continue
		}
		walked++
		s.now = func() time.Time { return at }
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		w := httptest.NewRecorder()
		s.guard(h).ServeHTTP(w, r) // NO Authorization header
		body := w.Body.String()
		for _, n := range jsonNumbers(t, body) {
			switch n {
			case wantOut, -wantOut, wantCost, r22FirstBounty:
				t.Fatalf("%s carries %d on the UNAUTHENTICATED wire. That is a bounty-out or realised-repair-cost figure of a root /api/roots names (red-team F2) — pooled or per-object makes no difference on a node caretaking one object:\n%s", pattern, n, body)
			}
		}
		if path != "/api/roots" && strings.Contains(body, root.String()) {
			t.Fatalf("%s names the cared root %s unauthenticated:\n%s", pattern, root.String(), body)
		}
	}
	if walked != r29aWholeSurfaceGETRoutes {
		t.Fatalf("walked %d GET routes, want %d", walked, r29aWholeSurfaceGETRoutes)
	}

	// And the untokened bodies must NAME the withhold. A flows document with no numbers
	// and no marker reads as "this node caretakes nothing" — a false absence (Don't #4).
	for _, path := range []string{"/api/economy/flows", "/api/economy/g"} {
		body := economyRouteAt(t, s, path, at, false)
		var doc struct {
			Withheld bool   `json:"detailWithheld"`
			Note     string `json:"note"`
		}
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		if !doc.Withheld {
			t.Fatalf("untokened %s carries no detailWithheld marker: an empty document reads as a node with no escrows:\n%s", path, body)
		}
	}
}

// TestR22WindowNotYetMeasuredIsNotAZero: before two samples exist there is no delta, and
// the endpoints must say so rather than publishing net 0. A zero here is the silent-loss
// shape — an operator reading "net 0" on a draining node has been told the wrong thing.
func TestR22WindowNotYetMeasuredIsNotAZero(t *testing.T) {
	s, led := statusServer(t)
	root := ports.Hash{0x22, 0x0E}
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root})
		led.RecordServeToObject(s.nd.ID(), ports.NodeID{0xC0}, root, ports.ChunkID{0x1}, 10*econMintUnit)
	})
	var doc struct {
		NotYet bool             `json:"windowNotYetMeasured"`
		Pooled *json.RawMessage `json:"pooled"`
		Note   string           `json:"note"`
	}
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/flows", s.started, true)), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.NotYet || doc.Pooled != nil {
		t.Fatalf("one-sample flows = %+v: the pooled row must be ABSENT with windowNotYetMeasured, never a zero net", doc)
	}
	if !strings.Contains(strings.ToLower(doc.Note), "not a zero") {
		t.Fatalf("note = %q — the absence must say what it is not", doc.Note)
	}
}

// TestR22GossipEstimatedFieldsNeverRenderWithoutTheirSample drives the PURE renderings
// (economyConcentrationDoc / economyNetworkDoc) so the honesty rules are testable without
// a node, a chain and a peer sample behind them.
//
// ABLATION E1: drop the `if !out.Sample.TooSmall` guard in economyConcentrationDoc and
// the first arm reddens — a two-node Gini would ship, and a two-node Gini IS the ratio of
// those two nodes' counters.
func TestR22GossipEstimatedFieldsNeverRenderWithoutTheirSample(t *testing.T) {
	for _, size := range []int{0, 1, minGossipSample - 1} {
		sample := node.EconomySample{Size: size, ServeGini: 0.42, RepairGini: 0.7, RepairSampleSize: size, Mix: map[string]int{node.TierPony: size}}
		conc := economyConcentrationDoc(sample, nil)
		if !conc.Sample.TooSmall || conc.ServeGini != nil || conc.RepairGini != nil {
			t.Fatalf("size %d: concentration published an estimate below the floor: %+v", size, conc)
		}
		if !strings.Contains(conc.Sample.Note, "too small") {
			t.Fatalf("size %d: the absence does not say why: %q", size, conc.Sample.Note)
		}
		net := economyNetworkDoc(sample)
		if !net.Sample.TooSmall || len(net.Mix) != 0 {
			t.Fatalf("size %d: network published a mix below the floor: %+v", size, net)
		}
		if len(net.Bands) != 3 || len(net.TargetRatio) != 3 {
			t.Fatalf("size %d: the published bands and the target ratio must ship even with no sample — they are constants an operator checks the classification against", size)
		}
	}

	// At the floor the estimate renders, WITH its sample size beside it.
	sample := node.EconomySample{Size: 8, SelfIncluded: true, ServeGini: 0.11, RepairGini: 0.33, RepairSampleSize: 4,
		ServeWorkTotal: 9_000, RepairWorkTotal: 12, EstimatedNodes: 40,
		Mix: map[string]int{node.TierPony: 5, node.TierHorse: 2, node.TierArchival: 1}}
	conc := economyConcentrationDoc(sample, nil)
	if conc.ServeGini == nil || !conc.ServeGini.Known || conc.ServeGini.SampleSize != 8 || conc.ServeGini.Value != 0.11 {
		t.Fatalf("serveGini = %+v, want a KNOWN value 0.11 over 8", conc.ServeGini)
	}
	if conc.RepairGini == nil || conc.RepairGini.SampleSize != 4 {
		t.Fatalf("repairGini = %+v: the repair series must carry the CAPABLE SUBSET's size (4), not the sample's (8) — the wrong sibling on the wrong number", conc.RepairGini)
	}
	if !strings.Contains(conc.RepairGini.Scope, "REPAIR-CAPABLE") {
		t.Fatalf("repairGini scope = %q: a repair Gini without its scope is unreadable — the network-wide one is ~0.99 on a healthy network", conc.RepairGini.Scope)
	}
	if conc.C2 != nil || conc.C2Absent == "" {
		t.Fatalf("with no chain, C2 must be ABSENT with a reason, never a zeroed block a reader takes for a decentralised network: %+v / %q", conc.C2, conc.C2Absent)
	}
	// The repair subset carries its own floor: a big sample with a tiny capable subset
	// must still withhold the repair series.
	thin := economyConcentrationDoc(node.EconomySample{Size: 20, ServeWorkTotal: 5, RepairGini: 0.9,
		RepairSampleSize: minGossipSample - 1, RepairWorkTotal: 9}, nil)
	if thin.ServeGini == nil || thin.RepairGini != nil {
		t.Fatalf("a 20-node sample with a %d-node capable subset published the repair Gini: %+v", minGossipSample-1, thin.RepairGini)
	}

	net := economyNetworkDoc(sample)
	if len(net.Mix) != 3 {
		t.Fatalf("mix = %+v, want three rows", net.Mix)
	}
	if net.ObservedRatioAbsent != "" || len(net.ObservedRatio) != 3 {
		t.Fatalf("observed ratio = %+v / %q", net.ObservedRatio, net.ObservedRatioAbsent)
	}
	// No archival node in the sample: the ratio has no denominator, and that is UNKNOWN.
	noArch := economyNetworkDoc(node.EconomySample{Size: 9, Mix: map[string]int{node.TierPony: 9}})
	if len(noArch.ObservedRatio) != 0 || noArch.ObservedRatioAbsent == "" {
		t.Fatalf("with no archival node the ratio must be absent with its reason, not a division by zero: %+v / %q", noArch.ObservedRatio, noArch.ObservedRatioAbsent)
	}
	if len(noArch.Mix) != 1 {
		t.Fatalf("mix = %+v: a class with no members is ABSENT, never a zero row — 'none in my sample' is not 'none exist'", noArch.Mix)
	}
}

// TestR22AnAllZeroWorkSampleIsNotAMeasuredEquality is blocker B2 (blind PE, measured).
//
// credit.Gini returns 0 when the values sum to zero — "universal poverty is technically
// equality" in its own comment. Nothing downstream told that from a measured equality, so
// eight peers that had all reported nothing published serveGini {value:0, sampleSize:8},
// which the dashboard rendered as "0.0000 — gossip-estimated over 8 nodes". A reader saw
// perfect equality of work across eight nodes. The truth was that no node reported any.
//
// It is live in three states, not hypothetical: a fresh network; a node whose ledger does
// not implement workReporter (workRep stays nil and selfWork gossips 0); and the mixed case
// where "cannot see my counters" and "served nothing" are summed into one distribution.
//
// ABLATION E2: delete giniOver's `total <= 0` branch and both arms redden.
func TestR22AnAllZeroWorkSampleIsNotAMeasuredEquality(t *testing.T) {
	// The PE's fixture: eight peers, all reporting zero work.
	zero := economyConcentrationDoc(node.EconomySample{Size: 8, ServeGini: 0, RepairGini: 0,
		RepairSampleSize: 8, ServeWorkTotal: 0, RepairWorkTotal: 0}, nil)
	for name, gv := range map[string]*giniValue{"serveGini": zero.ServeGini, "repairGini": zero.RepairGini} {
		if gv == nil {
			t.Fatalf("%s is absent; B2 is about what it SAYS, so an absent block means the fixture changed", name)
		}
		if gv.Known {
			t.Fatalf("%s = %+v over a sample that reported no work at all. A 0 here reads as perfect equality across %d nodes; the truth is that %d nodes said nothing (blind PE B2)", name, gv, gv.SampleSize, gv.SampleSize)
		}
		if gv.Value != 0 || gv.Reason == "" {
			t.Fatalf("%s = %+v: an unknown Gini must carry no value and a reason", name, gv)
		}
		if gv.SampleSize != 8 {
			t.Fatalf("%s dropped its sample size (%d): the absence still needs its sibling", name, gv.SampleSize)
		}
	}

	// A MEASURED equality is a real result and must still publish, or the fix above is a
	// blanket refusal to publish zeros rather than a distinction between two facts.
	equal := economyConcentrationDoc(node.EconomySample{Size: 8, ServeGini: 0, ServeWorkTotal: 8_000,
		RepairGini: 0, RepairSampleSize: 8, RepairWorkTotal: 40}, nil)
	if equal.ServeGini == nil || !equal.ServeGini.Known || equal.ServeGini.Value != 0 {
		t.Fatalf("eight nodes that each served the same real number of bytes publish %+v; that IS a measured equality and 0 is its answer", equal.ServeGini)
	}
	if equal.RepairGini == nil || !equal.RepairGini.Known {
		t.Fatalf("a measured repair equality was suppressed: %+v", equal.RepairGini)
	}

	// And the two series are independent: serving reported, repair not.
	mixed := economyConcentrationDoc(node.EconomySample{Size: 8, ServeGini: 0.2, ServeWorkTotal: 9_000,
		RepairGini: 0, RepairSampleSize: 8, RepairWorkTotal: 0}, nil)
	if mixed.ServeGini == nil || !mixed.ServeGini.Known || mixed.RepairGini == nil || mixed.RepairGini.Known {
		t.Fatalf("a sample that served but never repaired = serve %+v repair %+v; the two sums are separate questions", mixed.ServeGini, mixed.RepairGini)
	}
}

// TestR22PublishedBandsMatchTheClassifierIsUsing: the bands on the wire are what an
// operator checks the classification against, so they must be the SAME numbers
// core/node cuts on. A drifting band table is a lie with a citation attached.
func TestR22PublishedBandsMatchTheClassifierIsUsing(t *testing.T) {
	bands := publishedTierBands()
	if bands[0].MaxBytes+1 != node.TierPonyMaxBytes || bands[1].MinBytes != node.TierPonyMaxBytes {
		t.Fatalf("the pony|horse edge on the wire (%d/%d) is not core/node's %d", bands[0].MaxBytes, bands[1].MinBytes, node.TierPonyMaxBytes)
	}
	if bands[1].MaxBytes+1 != node.TierHorseMaxBytes || bands[2].MinBytes != node.TierHorseMaxBytes {
		t.Fatalf("the horse|archival edge on the wire (%d/%d) is not core/node's %d", bands[1].MaxBytes, bands[2].MinBytes, node.TierHorseMaxBytes)
	}
	if bands[2].MaxBytes != 0 {
		t.Fatal("the top band has an upper edge; it must be open-ended (omitempty) or an archival node above it falls out of every class")
	}
	for _, b := range bands {
		if !strings.Contains(b.Source, "sustainability-audit") {
			t.Fatalf("band %q carries no provenance: a published number without its source is a magic constant", b.Class)
		}
	}
	if r := targetTierRatio(); r[0].Ratio != 10000 || r[1].Ratio != 100 || r[2].Ratio != 1 {
		t.Fatalf("target ratio = %+v, want the ratified 10000:100:1 (decisions.md, D-TIERING 2026-08-31)", r)
	}
}

// TestR22ConcentrationCarriesC2WhenAChainExists closes the read-through half of row 12
// and the split-detection companion: publishing three of C2's four concentration signals
// would show an equal-bond SPLITTER as maximally decentralised.
func TestR22ConcentrationCarriesC2WhenAChainExists(t *testing.T) {
	m := chain.C2{NakamotoBonds: 4, NakamotoOperators: 4, NakamotoDomains: 3, Participants: 9,
		DistinctDomains: 3, HHI: 0.2, Gini: 0.3, TopShare: 0.4, WeightUniformity: 0.99}
	doc := economyConcentrationDoc(node.EconomySample{Size: 5, ServeWorkTotal: 1}, &m)
	if doc.C2 == nil || doc.C2Absent != "" {
		t.Fatalf("C2 missing with a chain present: %+v", doc)
	}
	if doc.C2.Tier != "committed-global" {
		t.Fatalf("C2 tier = %q: it is exact as of my head, a different tier from the gossip-estimated Ginis beside it", doc.C2.Tier)
	}
	if doc.C2.WeightUniformity != 0.99 {
		t.Fatal("weightUniformity is not published. HHI, Gini and TopShare are all BLIND to an equal-bond split — a splitter posting N identical min-bonds reads as maximally decentralised on all three, and this is the only one that carries the tell")
	}
}

var _ = credit.Gini
