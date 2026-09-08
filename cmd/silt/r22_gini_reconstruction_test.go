package main

// R2.2 / blind PE B3 / red-team F1 — THE RECONSTRUCTION GATE.
//
// The property, in the red-team's own words: under `-privacy` ON, no open GET route may
// publish numbers from which a sample-stuffing caller recovers a node-wide counter the
// privacy clause withholds. It asserts the RECOVERY fails, not that some named field is
// absent, because the break is a reconstruction and not a direct field — a future edit that
// republishes the same information under a different name must redden here.
//
// recoverUnknown is LIFTED from the red-team's proof of concept
// (REDTEAM-c3-gossip-disclosure-f03ab50-2026-09-09; PoC at .../blind-c3/tree/core/node/
// zz_redteam_gini_test.go) rather than reimplemented, so the gate solves the attack the
// adversarial seat actually ran and not a weaker one this seat invented.
//
// TWO ARMS AND BOTH ARE LOAD-BEARING. The POSITIVE CONTROL comes first: with the token (and
// again with -privacy=off) the document still carries the Gini and the solve recovers the
// planted secret EXACTLY. Without it the negative arm would pass on a fixture where nothing
// was reconstructible in the first place, which is how the original examination convinced
// itself these routes were safe.
//
// WHERE THE FIXTURE COMES FROM. The sample is built here with the REAL credit.Gini over the
// same value set the node would produce; core/node's TestR22GossipSampleIsExactlyGiniOverThe
// SampledValues pins that EconomySample computes exactly that, and the red-team PoC drove it
// end to end through Node.handle. It is assembled here because memstore is not a
// CapacityReporter, so a cmd/silt fixture's node can never put self in its own sample.

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
)

// The planted secret and the adversary's chosen sybil value. The secret is a node-wide
// ServedBytes figure — the counter readerView nils and privacyWithheldEconomySelf drops.
const (
	r22Secret     = int64(987_654_321)
	r22SybilValue = int64(1_000_000)
	r22Sybils     = 4
)

// recoverUnknown solves for the single unknown sample value V given the published Gini and
// the adversary's n-1 known values (all the constant c). Binary search on whichever monotone
// branch V lives in. Integers, so recovery is exact. LIFTED from the red-team PoC.
func recoverUnknown(knownConst int64, nKnown int, gpub float64) (int64, bool) {
	gini := func(V int64) float64 {
		vals := make([]int64, 0, nKnown+1)
		for i := 0; i < nKnown; i++ {
			vals = append(vals, knownConst)
		}
		vals = append(vals, V)
		return credit.Gini(vals)
	}
	try := func(lo, hi int64) (int64, bool) {
		glo := gini(lo)
		for lo < hi {
			mid := lo + (hi-lo)/2
			gm := gini(mid)
			if math.Abs(gm-gpub) < 1e-15 {
				return mid, true
			}
			if (glo-gpub)*(gm-gpub) <= 0 {
				hi = mid
			} else {
				lo, glo = mid+1, gm
			}
		}
		if math.Abs(gini(lo)-gpub) < 1e-12 {
			return lo, true
		}
		return 0, false
	}
	if v, ok := try(0, knownConst); ok {
		return v, true
	}
	hi := knownConst
	for k := 0; k < 60; k++ {
		if gini(hi) >= gpub {
			break
		}
		hi *= 2
	}
	return try(knownConst, hi)
}

// r22StuffedSample is the adversary's view: r22Sybils planted values plus one secret.
func r22StuffedSample() node.EconomySample {
	vals := make([]int64, 0, r22Sybils+1)
	for i := 0; i < r22Sybils; i++ {
		vals = append(vals, r22SybilValue)
	}
	vals = append(vals, r22Secret)
	var total int64
	for _, v := range vals {
		total += v
	}
	return node.EconomySample{
		Size: len(vals), SelfIncluded: false, EstimatedNodes: 40,
		ServeGini: credit.Gini(vals), ServeSampleSize: len(vals), ServeWorkTotal: total,
		RepairGini: credit.Gini(vals), RepairSampleSize: len(vals), RepairWorkTotal: total,
		Mix: map[string]int{node.TierHorse: len(vals)},
	}
}

// solveFromDocument runs the red-team's solve against whatever the SERVED document carries.
// It reads the JSON, not the Go struct, because the wire is what an attacker sees.
func solveFromDocument(t *testing.T, body string) (int64, bool) {
	t.Helper()
	var doc struct {
		Sample *struct {
			Size int `json:"size"`
		} `json:"sample"`
		ServeGini *struct {
			Known      bool    `json:"known"`
			Value      float64 `json:"value"`
			SampleSize int     `json:"sampleSize"`
		} `json:"serveGini"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if doc.ServeGini == nil || !doc.ServeGini.Known {
		return 0, false
	}
	n := doc.ServeGini.SampleSize
	if doc.Sample != nil && doc.Sample.Size > n {
		n = doc.Sample.Size
	}
	if n < 2 {
		return 0, false
	}
	return recoverUnknown(r22SybilValue, n-1, doc.ServeGini.Value)
}

func r22Serve(t *testing.T, doc any) string {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestR22APublishedGiniNeverReconstructsAPrivacyWithheldCounter(t *testing.T) {
	sample := r22StuffedSample()

	// ---- POSITIVE CONTROL 1: the OPERATOR's own tokened read still works, and the solve
	// still succeeds against it. This proves the attack is real and the fixture is not
	// vacuous; it is also the honest statement of what the token buys.
	tokened := r22Serve(t, economyConcentrationDoc(sample, nil, readerAuth{token: true, tokenHeader: true, privacy: true}))
	got, ok := solveFromDocument(t, tokened)
	if !ok || got != r22Secret {
		t.Fatalf("the TOKENED document did not reconstruct the secret (got %d ok %v). Without a working solve here the negative arm below is vacuous — it would pass on a document nothing could be recovered from, which is exactly how the first examination convinced itself these routes were safe", got, ok)
	}

	// ---- POSITIVE CONTROL 2: -privacy=off. A node that publishes its raw node-wide
	// counters to any reader loses nothing by publishing a Gini over them, so the withhold
	// must NOT fire there — otherwise this is theatre rather than a containment.
	open := r22Serve(t, economyConcentrationDoc(sample, nil, readerAuth{privacy: false}))
	if got, ok := solveFromDocument(t, open); !ok || got != r22Secret {
		t.Fatalf("-privacy=off withheld the Gini (got %d ok %v). That node publishes stats.BytesServed raw to the same reader; withholding a derivation of it closes nothing and makes the two surfaces disagree", got, ok)
	}

	// ---- THE GATE: the shipped default, unauthenticated. The recovery must FAIL.
	shipped := r22Serve(t, economyConcentrationDoc(sample, nil, readerAuth{privacy: privacyDefaultWithheld}))
	if got, ok := solveFromDocument(t, shipped); ok {
		t.Fatalf("recovered %d from the UNAUTHENTICATED /api/economy/concentration document on the shipped -privacy default. A published Gini plus its sample size is one equation; a reader that supplies the other %d terms with free identities solves it for a node-wide work counter readerView nils for this very reader (blind PE B3, red-team F1):\n%s", got, r22Sybils, shipped)
	}
	// ...and the absence is NAMED. A document with no numbers and no marker reads as a node
	// that knows no peers, which is a different fact.
	var doc struct {
		CountersWithheld bool   `json:"countersWithheld"`
		Note             string `json:"note"`
		C2Absent         string `json:"c2Absent"`
	}
	if err := json.Unmarshal([]byte(shipped), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.CountersWithheld {
		t.Fatalf("the withheld concentration document carries no countersWithheld marker:\n%s", shipped)
	}
	if doc.C2Absent == "" {
		t.Fatalf("the committed-global C2 leg lost its own absence reason to the privacy clause:\n%s", shipped)
	}

	// ---- The same for the network route: the mix publishes the sample SIZE, which is half
	// the equation, so it moves with the Ginis.
	netShipped := r22Serve(t, economyNetworkDoc(sample, readerAuth{privacy: privacyDefaultWithheld}))
	var nd struct {
		Sample           *json.RawMessage  `json:"sample"`
		Mix              []json.RawMessage `json:"mix"`
		ObservedRatio    []json.RawMessage `json:"observedRatio"`
		CountersWithheld bool              `json:"countersWithheld"`
		EstimatedNodes   float64           `json:"estimatedNodes"`
		Bands            []json.RawMessage `json:"bands"`
	}
	if err := json.Unmarshal([]byte(netShipped), &nd); err != nil {
		t.Fatal(err)
	}
	if nd.Sample != nil || len(nd.Mix) != 0 || len(nd.ObservedRatio) != 0 || !nd.CountersWithheld {
		t.Fatalf("the unauthenticated /api/economy/network still publishes the sample or the mix. The mix sums to the sample SIZE, and size is half of the equation the Gini is the other half of:\n%s", netShipped)
	}
	if len(nd.Bands) != 3 {
		t.Fatalf("the published bands went with the withhold. They are constants an operator checks the classification against and carry no measurement:\n%s", netShipped)
	}
	if nd.EstimatedNodes != sample.EstimatedNodes {
		t.Fatalf("estimatedNodes was withheld (%v). It is the SAME number /api/status publishes in `network`, which the privacy clause does not touch — withholding it here is a withhold in name only, and the disagreement invites a later edit to open the wrong one", nd.EstimatedNodes)
	}
}

// TestR22TheOpenCrowdEstimateIsTheOneStatusAlreadyPublishes pins the one exception above, so
// the reason estimatedNodes stays open cannot rot into a leak. If /api/status ever withholds
// its network block, this reddens and the exception must be re-argued.
func TestR22TheOpenCrowdEstimateIsTheOneStatusAlreadyPublishes(t *testing.T) {
	s, _ := statusServer(t)
	s.privacy = privacyDefaultWithheld
	at := s.started

	var st struct {
		Network struct {
			EstimatedNodes float64
		} `json:"network"`
	}
	if err := json.Unmarshal([]byte(statusAt(t, s, at, false)), &st); err != nil {
		t.Fatal(err)
	}
	var nw struct {
		EstimatedNodes   float64 `json:"estimatedNodes"`
		CountersWithheld bool    `json:"countersWithheld"`
	}
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/network", at, false)), &nw); err != nil {
		t.Fatal(err)
	}
	if !nw.CountersWithheld {
		t.Fatal("the unauthenticated network document is not marked withheld under the shipped privacy default")
	}
	if nw.EstimatedNodes != st.Network.EstimatedNodes {
		t.Fatalf("/api/economy/network publishes estimatedNodes %v while /api/status publishes %v. They must be the same number or the exception that keeps this one open is no longer true",
			nw.EstimatedNodes, st.Network.EstimatedNodes)
	}
}

// TestR22TheTwoGossipRoutesHonourThePrivacyClauseOverHTTP is the wiring half: the withhold
// above is in a pure function, and a handler that forgets to pass the reader's auth through
// would leave it dead. Drives the REAL routes through the REAL guard.
func TestR22TheTwoGossipRoutesHonourThePrivacyClauseOverHTTP(t *testing.T) {
	s, _ := statusServer(t)
	s.privacy = privacyDefaultWithheld
	at := s.started
	for _, path := range []string{"/api/economy/concentration", "/api/economy/network"} {
		var untokened, tokened struct {
			Sample           *json.RawMessage `json:"sample"`
			CountersWithheld bool             `json:"countersWithheld"`
			Note             string           `json:"note"`
		}
		if err := json.Unmarshal([]byte(economyRouteAt(t, s, path, at, false)), &untokened); err != nil {
			t.Fatal(err)
		}
		if !untokened.CountersWithheld || untokened.Sample != nil {
			t.Fatalf("GET %s unauthenticated on the shipped privacy default: countersWithheld=%v sample=%v — the handler is not passing the reader's auth into the document", path, untokened.CountersWithheld, untokened.Sample)
		}
		if !strings.Contains(untokened.Note, "privacy") || !strings.Contains(untokened.Note, "token") {
			t.Fatalf("GET %s withholds silently: note = %q. The absence must name itself and its recovery", path, untokened.Note)
		}
		if err := json.Unmarshal([]byte(economyRouteAt(t, s, path, at, true)), &tokened); err != nil {
			t.Fatal(err)
		}
		if tokened.CountersWithheld || tokened.Sample == nil {
			t.Fatalf("GET %s WITH the token is still withheld (%+v). The operator's own dashboard reads these panels; the withhold is a containment on the anonymous reader, not on the operator", path, tokened)
		}
	}
}
