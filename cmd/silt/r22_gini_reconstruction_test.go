package main

// R2.2 / blind PE B3 / red-team F1 — THE RECONSTRUCTION GATE.
//
// The property, in the red-team's own words: under `-privacy` ON, no open GET route may
// publish numbers from which a sample-stuffing caller recovers a node-wide counter the
// privacy clause withholds. It asserts the RECOVERY fails, not that some named field is
// absent, because the break is a reconstruction and not a direct field.
//
// AND THE SOLVE READS NO KEY NAME — see solveFromDocument for the measured reason it had to
// stop. A rename, a re-nesting, or a move to another GET route are each encoded as their own
// arm, because "a rename must redden here" was a claim this gate made and did not hold.
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
// same value set the node would produce. core/node's
// TestR22GossipSampleIsExactlyGiniOverSampledValues pins that EconomySample computes exactly
// that — the joint between the two — and the red-team PoC drove it end to end through
// Node.handle. It is assembled here because memstore is not a
// CapacityReporter, so a cmd/silt fixture's node can never put self in its own sample.

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
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
		if hi > math.MaxInt64/2 {
			// The value-shaped walk feeds this every numeric leaf, including byte counts
			// and unix stamps that are not Ginis at all. Gini tends to (n-1)/n, so a leaf
			// above that limit is unreachable and the doubling would otherwise wrap
			// negative. Not a candidate: report no recovery.
			return 0, false
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
		Size: len(vals), SelfIncluded: true, EstimatedNodes: 40,
		ServeGini: credit.Gini(vals), ServeSampleSize: len(vals), ServeWorkTotal: total,
		RepairGini: credit.Gini(vals), RepairSampleSize: len(vals), RepairWorkTotal: total,
		Mix: map[string]int{node.TierHorse: len(vals)},
	}
}

// solveFromDocument runs the red-team's solve against the SERVED document — and it is
// VALUE-SHAPED, not name-shaped, which is the whole point of this rewrite.
//
// WHY IT HAD TO CHANGE (blind PE re-ruling at 81d39c0, measured). The first version decoded
// exactly two keys by name, `sample` and `serveGini`, while its own docstring claimed "a
// future edit that republishes the same information under a different name must redden
// here." False. The PE republished the identical Gini on the same unauthenticated document
// under the key `workConcentration`, changed nothing else, and solved the secret exactly:
//
//	{"tier":"…","countersWithheld":true,
//	 "workConcentration":{"known":true,"value":0.795966336337882,"sampleSize":5,…}, …}
//	SOLVE off the renamed field: recovered=987654321 ok=true secret=987654321
//
// The whole R2.2 suite in this package stayed **ok**, and the document still said
// `countersWithheld:true`. That is the third instance in this repo of a privacy gate keyed on
// a field NAME or a fixture VALUE rather than on the property.
//
// SO IT READS NO KEY AT ALL. It walks every numeric leaf of the body at any depth and tries
// the recovery against each. The sample size is not read from the document either: the
// ADVERSARY KNOWS IT, because the adversary planted n-1 of the n terms itself. A rename, a
// re-nesting, a move to another route, or an added sibling field all fail to evade it —
// the only thing that closes it is not publishing the value.
func solveFromDocument(t *testing.T, body string) (int64, bool) {
	t.Helper()
	_, got, ok := solveFromAnyLeaf(t, body)
	return got, ok
}

// solveFromAnyLeaf is the value-shaped solve. It returns the leaf that resolved, so a failure
// message can name the number that leaked rather than only the secret behind it.
func solveFromAnyLeaf(t *testing.T, body string) (float64, int64, bool) {
	t.Helper()
	for _, leaf := range jsonNumericLeaves(t, body) {
		// The adversary supplied r22Sybils of the r22Sybils+1 terms, so it knows the
		// count without being told. recoverUnknown matches the candidate Gini to 1e-12,
		// so a leaf that is not this sample's Gini does not resolve.
		if got, ok := recoverUnknown(r22SybilValue, r22Sybils, leaf); ok && got == r22Secret {
			return leaf, got, true
		}
	}
	return 0, 0, false
}

// jsonNumericLeaves returns every number in a JSON document at any depth, as float64. The
// sibling jsonNumbers (r29a_status_surface_test.go) keeps only values that are exact int64s,
// which is right for the counter scans it serves and useless here: a Gini is a fraction, and
// the fraction is the leak.
func jsonNumericLeaves(t *testing.T, body string) []float64 {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("not a JSON body (%v): %s", err, body)
	}
	var out []float64
	var walk func(any)
	walk = func(x any) {
		switch x := x.(type) {
		case json.Number:
			if f, err := x.Float64(); err == nil {
				out = append(out, f)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return out
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

	// ---- THE RENAME ARM. This is the shape that slipped past the first version of this
	// gate, so it is encoded rather than trusted to a docstring. Republish the identical
	// Gini on the same withheld document under a key nothing else uses, change nothing
	// else, and the solve must still find it. If this arm ever passes, the gate has gone
	// back to reading key names.
	var renamed map[string]any
	if err := json.Unmarshal([]byte(shipped), &renamed); err != nil {
		t.Fatal(err)
	}
	renamed["workConcentration"] = map[string]any{
		"known": true, "value": sample.ServeGini, "sampleSize": sample.Size,
	}
	if leaf, got, ok := solveFromAnyLeaf(t, r22Serve(t, renamed)); !ok || got != r22Secret {
		t.Fatalf("the same Gini republished under the key \"workConcentration\" was NOT recovered (leaf %v got %d ok %v). The solve has gone back to reading key names, and a rename is exactly what the blind PE walked through this gate green while the unauthenticated document handed out the secret and still said countersWithheld:true", leaf, got, ok)
	}

	// ---- THE SELF ARM (the measurement behind "dropping self is not a fix"). A document
	// built from a sample that EXCLUDES self is still solvable, because the recovered term
	// need not be self: any node with an open route is an oracle for its PEERS' withheld
	// counters. This ran as a hand-driven revert in the first round and passed GREEN, which
	// is what refuted the narrowest of the three fixes the ruling offered; it is encoded
	// here so the refutation survives without anyone re-running it.
	noSelf := sample
	noSelf.SelfIncluded = false
	if got, ok := solveFromDocument(t, r22Serve(t, economyConcentrationDoc(noSelf, nil, r22Operator))); !ok || got != r22Secret {
		t.Fatalf("a self-EXCLUDED sample was not solvable (got %d ok %v). If that is now true, the peer-oracle case has changed and the whole shape of the fix should be re-argued — it was chosen because dropping self moves the target rather than closing it", got, ok)
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

// TestR22NoUnauthenticatedRouteReconstructsTheWorkCounterOnTheWholeSurface widens the arm
// above from ONE document to the whole GET table. A rename can move a value to another route
// as easily as to another key, and the two whole-surface scans that already walk apiRoutes
// cannot see this one: they match integer equality against fixture constants, and a Gini is a
// fraction (finding N2, value-scan blindness, its third instance in this repo).
//
// It drives the REAL routes on a REAL node at the shipped privacy default, so it also covers
// the handler wiring rather than the pure document builders.
//
// WHAT IT DOES AND DOES NOT COVER, stated because overclaiming a gate's coverage is the
// mistake that produced this rewrite. This fixture's node has an EMPTY peer sample —
// memstore is not a CapacityReporter and peerCaps is package-private to core/node, so a
// cmd/silt fixture cannot stuff one — which means the two gossip routes publish no Gini here
// whatever the privacy posture. So this walk does NOT redden under the gossipWithheld
// ablation; its sibling
// TestR22APublishedGiniNeverReconstructsAPrivacyWithheldCounter does, on a stuffed sample,
// and that is where the withhold itself is proved. The teeth of the CHECKER used below are
// demonstrated by the positive control, which recovers the secret from a real document. What
// this adds over the sibling is breadth: a value moved to any OTHER route is caught too, and
// the sibling only looks at two documents.
func TestR22NoUnauthenticatedRouteReconstructsTheWorkCounterOnTheWholeSurface(t *testing.T) {
	s, led := statusServer(t)
	s.privacy = privacyDefaultWithheld
	// Plant the secret on this node's own account, through the real ledger path.
	s.onLoop(func() { led.RecordServe(s.nd.ID(), ports.NodeID{0x01}, ports.ChunkID{}, r22Secret) })
	at := s.started

	// POSITIVE CONTROL: the solver must be able to find the value when it IS published, or
	// a clean walk proves nothing. Fed the document the sample would produce.
	if _, got, ok := solveFromAnyLeaf(t, r22Serve(t, economyConcentrationDoc(r22StuffedSample(), nil, r22Operator))); !ok || got != r22Secret {
		t.Fatalf("the solver cannot recover the secret even from a document that publishes the Gini (got %d ok %v); the walk below would be vacuous", got, ok)
	}

	walked := 0
	for pattern, h := range s.apiRoutes() {
		method, path, _ := strings.Cut(pattern, " ")
		if method != http.MethodGet {
			continue
		}
		walked++
		// Any status, not just 200: an error body is still a body the attacker reads, and
		// /api/fetch answers 400 on a registry-less daemon.
		s.now = func() time.Time { return at }
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		w := httptest.NewRecorder()
		s.guard(h).ServeHTTP(w, r) // NO Authorization header
		body := w.Body.String()
		if leaf, got, ok := solveFromAnyLeaf(t, body); ok {
			t.Fatalf("GET %s carries the number %v on the UNAUTHENTICATED wire at the shipped -privacy default, and a caller that planted %d of the sample's terms solves it for %d — the node-wide work counter readerView nils for this very reader. The key it is under does not matter:\n%s",
				path, leaf, r22Sybils, got, body)
		}
	}
	if walked != r29aWholeSurfaceGETRoutes {
		t.Fatalf("walked %d GET routes, want %d", walked, r29aWholeSurfaceGETRoutes)
	}
}

// TestR22TheOpenCrowdEstimateIsTheOneStatusAlreadyPublishes pins the exceptions above, so
// the reasons they stay open cannot rot into a leak. If /api/status ever withholds its
// network block, this reddens and the exception must be re-argued.
//
// IT PINS TWO QUANTITIES, and the second was added on the blind PE re-ruling at 81d39c0
// (register row R-C3-KNOWNPEERS-SIZE-OPEN). The withhold drops `sample.size` on the stated
// ground that "size is half of the equation the Gini is the other half of" — and that half
// is ALREADY OPEN two routes over: /api/status's network block is untouched by the privacy
// clause and carries KnownPeers = len(n.peerCaps) (core/node/capacity.go:28). Every peerCaps
// entry has CapTotal > 0 and is therefore classifiable, so
// EconomySample.Size == KnownPeers + (1 if self pledges).
//
// I PIN IT RATHER THAN WITHHOLDING IT, and the reason is the shape of the attack rather than
// the sensitivity of the number:
//   - Size alone is not an equation. The recovery needs the Gini, and no Gini is published on
//     the withholding posture — which is what the sibling gate asserts, value-shaped, over
//     every numeric leaf of every GET route.
//   - The party who could use n ALREADY KNOWS IT. n-1 of the terms are the adversary's own
//     planted sybils; it counts them itself. Withholding a number from the one reader who
//     supplied it is the "publish the one-term sum of a withheld array" mistake in reverse.
//   - /api/status already publishes a peer count openly and by design (`peers`, the transport
//     count). Withholding a second one beside it would be a withhold in name only, and the
//     disagreement is what invites a later edit to open the wrong one.
//
// So the honest invariant is the PAIR: the size may stay open exactly as long as no Gini
// does. Both halves are asserted here, and the sibling gate is what enforces the second.
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
	netBody := economyRouteAt(t, s, "/api/economy/network", at, false)
	if err := json.Unmarshal([]byte(netBody), &nw); err != nil {
		t.Fatal(err)
	}
	if !nw.CountersWithheld {
		t.Fatal("the unauthenticated network document is not marked withheld under the shipped privacy default")
	}
	if nw.EstimatedNodes != st.Network.EstimatedNodes {
		t.Fatalf("/api/economy/network publishes estimatedNodes %v while /api/status publishes %v. They must be the same number or the exception that keeps this one open is no longer true",
			nw.EstimatedNodes, st.Network.EstimatedNodes)
	}

	// R-C3-KNOWNPEERS-SIZE-OPEN. The withheld sample SIZE is open on /api/status as
	// KnownPeers. That is a decision, so it is pinned: if it is ever withheld, the "size is
	// half the equation" sentence in ui_economy.go must be re-read, and if the sentence is
	// ever taken to mean the size is secret, this names where it is not.
	if !strings.Contains(string(statusKey(t, statusAt(t, s, at, false), "network")), "KnownPeers") {
		t.Fatalf("/api/status's network block no longer carries KnownPeers on the unauthenticated wire. The concentration withhold drops sample.size calling it 'half of the equation'; that half was open here, and the pair — size open, Gini withheld — is the actual invariant. Re-argue the withhold before changing this")
	}
	// The OTHER half must be absent, which is what makes the openness above harmless. This
	// is the assertion the size pin exists to be read beside.
	for _, body := range []string{netBody, economyRouteAt(t, s, "/api/economy/concentration", at, false)} {
		for _, key := range []string{"\"serveGini\"", "\"repairGini\"", "\"sample\""} {
			if strings.Contains(body, key) {
				t.Fatalf("an unauthenticated document carries %s while the sample size stays open on /api/status. Either half alone is harmless; the pair is the equation:\n%s", key, body)
			}
		}
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
