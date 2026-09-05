package main

// R2.9a — the /api/status surface: the cached fixed-interval snapshot (G-BB-26 /
// Tester gate BB-21) and the object-level attribution leak (red-team F2, owner-ratified
// 2026-09-05). Both are properties of the WIRE, so both are asserted on the raw JSON
// rather than on a struct that would silently drop a key it does not declare.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

// statusServer is the /api/status fixture. economyServer now wires peerCount itself
// (both documents are served off one snapshot, so /api/economy/self reaches
// computeStatus too); this keeps the belt-and-braces set so a fixture change there
// cannot turn every status gate into a segfault.
//
// IT IS THE UNTAGGED FIXTURE, and its tagged counterpart builds on it. Every gate in
// this file is about the status endpoint itself — the cache, its provenance stamps, the
// invalidation hook and the F2 token gate — and every one of them must hold in a build
// that has no B_bootstrap instrument at all.
func statusServer(t *testing.T) (*uiServer, *credit.Ledger) {
	t.Helper()
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 }
	return s, led
}

// statusAt drives one GET /api/status at a fixed wall instant, with or without the
// bearer token, and returns the raw body.
func statusAt(t *testing.T, s *uiServer, at time.Time, tokened bool) string {
	t.Helper()
	s.now = func() time.Time { return at }
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/status", nil)
	if tokened {
		r.Header.Set("Authorization", "Bearer "+s.token)
	}
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// statusKeyPresent reports whether the top-level document carries key at all, and its
// raw value when it does. statusKey fails on an absent key; this one is for the gates
// that assert absence-versus-presence (the R2.9a three wire states).
func statusKeyPresent(t *testing.T, body, key string) (json.RawMessage, bool) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatalf("decode status: %v (%s)", err, body)
	}
	raw, ok := top[key]
	return raw, ok
}

func statusKey(t *testing.T, body, key string) json.RawMessage {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatalf("decode status: %v (body %s)", err, body)
	}
	return top[key]
}

// TestR29aBB21StaleIsIdentifiableAsStale. A cache that quietly serves an old number is
// a silent-loss failure shape (Don't #4). The document carries its own provenance: a
// FIXED taken-at (which is what makes two reads inside one interval byte-identical), an
// age computed at SERVE time (the one field that moves, deliberately), and T itself so
// an analyst can price R-BB-DELTA-TRAJECTORY without reading the source.
func TestR29aBB21StaleIsIdentifiableAsStale(t *testing.T) {
	s, led := statusServer(t)
	// One served byte so the document is not degenerate. Untagged this records
	// (identity, cumulative bytes) and no `when` at all — the point of D-BB-BUILD-TAG.
	led.RecordServe(ports.HashBytes([]byte("srv")), ports.HashBytes([]byte{0x00, 0x00, 0x29}), ports.Hash{}, 4096)

	base := s.started
	var first, second struct {
		TakenAt  int64 `json:"snapshotTakenAtUnix"`
		AgeSec   int64 `json:"snapshotAgeSec"`
		Interval int64 `json:"snapshotIntervalSec"`
	}
	if err := json.Unmarshal([]byte(statusAt(t, s, base, false)), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(statusAt(t, s, base.Add(4*time.Second), false)), &second); err != nil {
		t.Fatal(err)
	}
	if first.Interval != int64(statusSnapshotInterval/time.Second) || second.Interval != first.Interval {
		t.Fatalf("snapshotIntervalSec = %d / %d, want %d published on every response — T is a security parameter and it is published so the residual can be priced", first.Interval, second.Interval, int64(statusSnapshotInterval/time.Second))
	}
	if first.TakenAt != second.TakenAt {
		t.Fatalf("snapshotTakenAtUnix moved between two cached reads (%d -> %d) — it stamps the SNAPSHOT, not the request, and a moving stamp would make two reads inside one interval distinguishable", first.TakenAt, second.TakenAt)
	}
	if first.AgeSec != 0 {
		t.Fatalf("snapshotAgeSec = %d on a freshly computed document, want 0", first.AgeSec)
	}
	if second.AgeSec != 4 {
		t.Fatalf("snapshotAgeSec = %d four seconds into the interval, want 4 — without a moving age a reader cannot tell a cached value from a live one, which is exactly the silent-loss shape Don't #4 forbids", second.AgeSec)
	}
}

// TestR29aBB21OperatorsOwnWriteIsNotHiddenByTheCache. The cache must not hide the
// operator's own mutation: POST /api/fund debits the balance, and a dashboard that
// shows the OLD balance for up to a whole interval reads as "the action failed" — a
// silent-loss shape (Don't #4) and a worse one than ordinary polling staleness, because
// the client knows it just wrote. Found by TestEconomyEndToEndOnLiveDaemon, which went
// RED on the first build of this change with "funding did not debit the balance:
// 500000 -> 500000".
//
// The invalidation sits AFTER the token gate, so an unauthenticated reader cannot use
// it to drive the recompute rate the cache exists to cap.
func TestR29aBB21OperatorsOwnWriteIsNotHiddenByTheCache(t *testing.T) {
	s, led := statusServer(t)
	root := r29aCaredObject(t, s, led)
	at := s.started

	before := statusAt(t, s, at, true)
	var b struct {
		Objects []struct {
			Reserve int64 `json:"reserve"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(statusKey(t, before, "durability"), &b); err != nil {
		t.Fatal(err)
	}

	// Fund the escrow through the SAME loop the handler reads, then drive one mutating
	// request through guard so the invalidation hook runs.
	funder := ports.NodeID{0xAB}
	s.onLoop(func() {
		led.RecordServe(funder, ports.NodeID{0xFF}, ports.ChunkID{0xFF}, 9_000)
		if err := led.FundEscrow(root, funder, 5_000); err != nil {
			t.Errorf("FundEscrow: %v", err)
		}
	})
	mut := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/library/add", strings.NewReader(""))
	mut.Header.Set("Authorization", "Bearer "+s.token)
	s.guard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(httptest.NewRecorder(), mut)

	// Same instant, well inside one interval: the write must be visible anyway.
	after := statusAt(t, s, at, true)
	var a struct {
		Objects []struct {
			Reserve int64 `json:"reserve"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(statusKey(t, after, "durability"), &a); err != nil {
		t.Fatal(err)
	}
	if len(a.Objects) != 1 || a.Objects[0].Reserve != b.Objects[0].Reserve+5_000 {
		t.Fatalf("the operator's own write is invisible inside the refresh interval: reserve %+v -> %+v, want +5000. A cache that hides the write the caller just made reads as a failed action", b.Objects, a.Objects)
	}
}

// --- red-team F2: the exact per-object leak ----------------------------------------

// r29aCaredObject gives the node one cared object with a real funded reserve, so the
// per-object block has something in it to withhold.
func r29aCaredObject(t *testing.T, s *uiServer, led *credit.Ledger) ports.Hash {
	t.Helper()
	root := ports.Hash{0xF2, 0x0B, 0x1E}
	fetcher := ports.NodeID{0xC3}
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root})
		// THIS node is the server, so the node-wide balance moves too: the aggregate
		// that stays open and the per-object entry that does not both come off the
		// same serve, which is the pair the gate has to tell apart.
		led.RecordServeToObject(s.nd.ID(), fetcher, root, ports.ChunkID{0x1}, 8192)
	})
	return root
}

// TestR29aF2StatusWithholdsPerObjectDetailWithoutAToken is the gate on the object half
// of who-fetched-what.
//
// THE LEAK, exactly. RecordServeToObject adds bytes*SkimNum/SkimDen to the object's
// funded reserve (core/credit/escrow.go) and the skim is one eighth, so `delta funded
// x 8` is the EXACT byte count served of a NAMED content root. It shipped with no flag
// and no token on every /api/status response, and Don't #3 is a bright line.
//
// WHY NOT LESS PRECISION: rounding a CUMULATIVE counter does not stop delta extraction,
// because an observer polling across the rounding boundary still recovers the
// increments, and the increments are the leak.
func TestR29aF2StatusWithholdsPerObjectDetailWithoutAToken(t *testing.T) {
	s, led := statusServer(t)
	root := r29aCaredObject(t, s, led)

	open := statusAt(t, s, s.started, false)
	var od struct {
		BountyOn       bool  `json:"bountyOn"`
		Balance        int64 `json:"balance"`
		DetailWithheld bool  `json:"detailWithheld"`
		Objects        *[]struct {
			Root   string `json:"root"`
			Funded int64  `json:"funded"`
			Paid   int64  `json:"paid"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(statusKey(t, open, "durability"), &od); err != nil {
		t.Fatalf("decode durability: %v (body %s)", err, open)
	}
	if od.Objects != nil {
		t.Fatalf("durability.objects is on the UNAUTHENTICATED wire: %+v. Every entry carries a content root with `funded`, and the skim is one eighth, so eight times the delta in funded is the exact byte count served of that named root — the object half of who-fetches-what, with no flag and no token (red-team F2)", *od.Objects)
	}
	if !od.DetailWithheld {
		t.Fatalf("objects withheld but detailWithheld is false — an absent array must be distinguishable from a node that caretakes nothing, or a reader silently misreads a withholding as an empty node")
	}
	if strings.Contains(open, root.String()) {
		t.Fatalf("the content root %s appears in the unauthenticated body — withholding the funded counter while publishing the root it belongs to closes nothing:\n%s", root.String(), open)
	}
	// The aggregates that name no root stay open: the operator's dashboard and the
	// observatory read them, and they carry no object identity for a join to key on.
	if od.Balance == 0 {
		t.Fatalf("durability.balance is 0 on the unauthenticated wire — the node-wide aggregate names no root and must stay readable; only the per-OBJECT decomposition is gated")
	}

	// THE HARD CONSTRAINT: the operator's own solvency view keeps working. The embedded
	// UI attaches this exact header to every same-origin /api/ call.
	tokened := statusAt(t, s, s.started.Add(2*statusSnapshotInterval), true)
	var td struct {
		DetailWithheld bool `json:"detailWithheld"`
		Objects        []struct {
			Root       string `json:"root"`
			Funded     int64  `json:"funded"`
			Reserve    int64  `json:"reserve"`
			HorizonSec int64  `json:"horizonSec"`
			Finite     bool   `json:"finite"`
			Cliff      bool   `json:"cliff"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(statusKey(t, tokened, "durability"), &td); err != nil {
		t.Fatalf("decode tokened durability: %v (body %s)", err, tokened)
	}
	if td.DetailWithheld {
		t.Fatalf("detailWithheld true for a caller presenting the API token")
	}
	if len(td.Objects) != 1 || td.Objects[0].Root != root.String() {
		t.Fatalf("the operator's own per-object view is broken: got %+v, want one entry for %s. The durability horizon and the cliff early-warning are a shipped feature and this change publishes less, it never counts less", td.Objects, root.String())
	}
	// 8192 bytes served, skim 1/8 => funded 1024. The stored value is untouched.
	if td.Objects[0].Funded != 1024 {
		t.Fatalf("funded = %d, want 1024 (8192 served, one-eighth skim) — this change alters what is PUBLISHED, never what is counted", td.Objects[0].Funded)
	}
	if td.Objects[0].Reserve != 1024 {
		t.Fatalf("reserve = %d, want 1024 — the solvency figure the horizon projects from", td.Objects[0].Reserve)
	}
}

// economySelfAt drives one GET /api/economy/self at a fixed wall instant, with or
// without the bearer token, and returns the raw body. Same shape as statusAt: the two
// documents come off one snapshot and are gated the same way.
func economySelfAt(t *testing.T, s *uiServer, at time.Time, tokened bool) string {
	t.Helper()
	s.now = func() time.Time { return at }
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/economy/self", nil)
	if tokened {
		r.Header.Set("Authorization", "Bearer "+s.token)
	}
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiEconomySelf)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("economy/self %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// r29aEconomySelfView is the subset of the SELF document the F2 gates decode. Pointers
// where absence is the assertion: a struct field that defaults to zero would read a
// withheld block as "skimIn 0", which is the misreading the wire is designed to prevent.
type r29aEconomySelfView struct {
	DetailWithheld bool `json:"detailWithheld"`
	Objects        *[]struct {
		Root   string `json:"root"`
		SkimIn int64  `json:"skimIn"`
	} `json:"objects"`
	SelfFunding *struct {
		SkimIn    int64 `json:"skimIn"`
		BountyOut int64 `json:"bountyOut"`
		Net       int64 `json:"net"`
	} `json:"selfFunding"`
	Revenue struct {
		Balance     int64 `json:"balance"`
		ServedBytes int64 `json:"servedBytes"`
	} `json:"revenue"`
	TakenAt  int64 `json:"snapshotTakenAtUnix"`
	AgeSec   int64 `json:"snapshotAgeSec"`
	Interval int64 `json:"snapshotIntervalSec"`
}

func decodeEconomySelfView(t *testing.T, body string) r29aEconomySelfView {
	t.Helper()
	var v r29aEconomySelfView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode economy/self: %v (%s)", err, body)
	}
	return v
}

// TestR29aF2EconomySelfWithholdsPerObjectDetailWithoutAToken closes the twin surface.
// /api/economy/self republishes the same per-root skimIn/bountyOut as Panel 1, so
// gating /api/status alone would close nothing.
//
// THE POOLED SUM IS WITHHELD TOO. The first version of this gate asserted the opposite:
// that selfFunding.skimIn == 1024 must SURVIVE unauthenticated, "because it names no
// root". In this fixture — one cared object, which is every node from its first
// published object until its second — 1024 is the same number the assertion above it
// says must be withheld, and /api/roots names the root with no token. The blind PE
// review measured the withheld objects[0].funded and the open selfFunding.skimIn as the
// same number on a live daemon and recovered 131,104 bytes of a named root from a
// 16,388 step, at 330 ms. The gate did not miss the leak; it pinned it in place. Now it
// asserts the property that matters: withheld untokened, present tokened.
func TestR29aF2EconomySelfWithholdsPerObjectDetailWithoutAToken(t *testing.T) {
	s, led := statusServer(t)
	root := r29aCaredObject(t, s, led)

	body := economySelfAt(t, s, s.started, false)
	out := decodeEconomySelfView(t, body)
	if out.Objects != nil {
		t.Fatalf("economy/self publishes per-root skimIn unauthenticated: %+v — it is the same eighth-of-the-bytes counter /api/status withholds, so leaving it open closes nothing (red-team F2)", *out.Objects)
	}
	if out.SelfFunding != nil {
		t.Fatalf("economy/self publishes the POOLED selfFunding block unauthenticated: %+v. skimIn is the sum of objects[].funded; with one cared object the sum IS the withheld counter, and /api/roots names the root. Withholding the array and publishing its one-term sum closes nothing", *out.SelfFunding)
	}
	if !out.DetailWithheld {
		t.Fatalf("detail withheld but detailWithheld is false — withheld and empty must stay different objects")
	}
	if strings.Contains(body, root.String()) {
		t.Fatalf("the content root %s appears in the unauthenticated economy/self body:\n%s", root.String(), body)
	}
	// The node-wide aggregate stays open on this endpoint as on /api/status: it is
	// the same number durability.balance carries, and it is the observatory's read.
	if out.Revenue.Balance == 0 {
		t.Fatalf("revenue.balance is 0 on the unauthenticated wire — the node-wide aggregate must stay readable; only the per-object decomposition and its pooled sum are gated")
	}

	// THE HARD CONSTRAINT: the operator's own Panel 1 (my-solvency) and Panel 3
	// (is-durability-self-funding) keep working, with the numbers, for the token holder.
	full := decodeEconomySelfView(t, economySelfAt(t, s, s.started, true))
	if full.DetailWithheld || full.Objects == nil || len(*full.Objects) != 1 || (*full.Objects)[0].Root != root.String() {
		t.Fatalf("Panel 1 (my-solvency) is broken for the token holder: withheld=%v objects=%+v", full.DetailWithheld, full.Objects)
	}
	if full.SelfFunding == nil || full.SelfFunding.SkimIn != 1024 || full.SelfFunding.Net != 1024 {
		t.Fatalf("Panel 3 (is-durability-self-funding) is broken for the token holder: selfFunding=%+v, want skimIn 1024 (8192 served, one-eighth skim) — this change alters what is PUBLISHED, never what is counted", full.SelfFunding)
	}
}

// --- the whole surface, not one endpoint ---------------------------------------------

// r29aWholeSurfaceBytes is chosen so that funded (= bytes/8) is a value nothing else on
// the surface holds by coincidence: not a bucket edge, not a capacity, not a port.
const r29aWholeSurfaceBytes = 8 * 1_234_567

// jsonNumbers returns every number in a JSON document, at any depth. It refuses a body
// that is not JSON: a route that answered in some other shape would otherwise pass the
// scan by hiding from it.
func jsonNumbers(t *testing.T, body string) []int64 {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("not a JSON body (%v): %s", err, body)
	}
	var out []int64
	var walk func(any)
	walk = func(x any) {
		switch x := x.(type) {
		case json.Number:
			if n, err := x.Int64(); err == nil {
				out = append(out, n)
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

// TestR29aF2NoUnauthenticatedResponseOnTheWholeSurfaceCarriesTheWithheldCounter is the
// gate the PE review asked for in place of the per-endpoint ones: for a node with ONE
// cared object, no unauthenticated response on ANY route may carry objects[0].funded,
// under any name. It walks apiRoutes — the list serve registers — so a route added
// later is examined by construction, and it asks "what reconstructs the quantity", not
// "where is the field called funded": the scan is over every number in every body.
//
// WHAT IT PROVES AND WHAT IT DOES NOT. It proves the exact-equality leak is closed on
// the whole surface: no open number IS the withheld counter (funded, and at paid == 0
// its aliases reserve, net and per-object skimIn). It does NOT close
// R-BB-SIBLING-AGGREGATES: revenue.servedBytes is 8x funded and balance is 7x funded
// here, and on a node holding one root those node-wide totals are that root's. Those
// stay open because the cross-origin observatory reads stats.BytesServed with no token
// by design; gating them is the owner's trade, surfaced in the decisions correction.
// The gate logs the open aliases so the residual is measured, not assumed.
func TestR29aF2NoUnauthenticatedResponseOnTheWholeSurfaceCarriesTheWithheldCounter(t *testing.T) {
	s, led := statusServer(t)
	root := ports.Hash{0xF2, 0x0B, 0x1E}
	fetcher := ports.NodeID{0xC3}
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root})
		led.RecordServeToObject(s.nd.ID(), fetcher, root, ports.ChunkID{0x1}, r29aWholeSurfaceBytes)
	})
	const funded = int64(r29aWholeSurfaceBytes * credit.SkimNum / credit.SkimDen)
	at := s.started

	// POSITIVE CONTROL FIRST. The number must be on the TOKENED wire of both gated
	// endpoints, or a clean untokened walk below is vacuous: it would pass on a fixture
	// that served nothing.
	var td struct {
		Objects []struct {
			Funded int64 `json:"funded"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(statusKey(t, statusAt(t, s, at, true), "durability"), &td); err != nil {
		t.Fatal(err)
	}
	if len(td.Objects) != 1 || td.Objects[0].Funded != funded {
		t.Fatalf("fixture is vacuous: tokened durability.objects = %+v, want one entry with funded %d", td.Objects, funded)
	}
	full := decodeEconomySelfView(t, economySelfAt(t, s, at, true))
	if full.SelfFunding == nil || full.SelfFunding.SkimIn != funded || full.Objects == nil || len(*full.Objects) != 1 || (*full.Objects)[0].SkimIn != funded {
		t.Fatalf("fixture is vacuous: tokened economy/self selfFunding=%+v objects=%+v, want %d on both", full.SelfFunding, full.Objects, funded)
	}

	walked := 0
	for pattern, h := range s.apiRoutes() {
		method, path, _ := strings.Cut(pattern, " ")
		if method != http.MethodGet {
			continue // the mutating routes are refused before their handler runs (guard step 4)
		}
		walked++
		s.now = func() time.Time { return at }
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		w := httptest.NewRecorder()
		s.guard(h).ServeHTTP(w, r) // NO Authorization header
		body := w.Body.String()
		for _, n := range jsonNumbers(t, body) {
			if n == funded {
				t.Fatalf("%s carries %d on the UNAUTHENTICATED wire. That is objects[0].funded — one eighth of every byte served of a root /api/roots names — under whatever key this route calls it. Gating durability.objects on one route while a sibling republishes the same quantity closes nothing (red-team F2, PE ruling 2026-09-05):\n%s", pattern, n, body)
			}
		}
		if path != "/api/roots" && strings.Contains(body, root.String()) {
			t.Fatalf("%s names the cared root %s unauthenticated. /api/roots is the ONE route that publishes held roots (the observatory's shard-spread view); every other route must not supply the name half of the join:\n%s", pattern, root.String(), body)
		}
	}
	// EXACT, on purpose: adding a GET route must fail here until the route has been
	// examined for what it republishes and this count raised. That is the whole point
	// of walking the real table.
	const want = 7
	if walked != want {
		t.Fatalf("walked %d GET routes, expected exactly %d — a route was added or removed; examine it against this gate's property, then update the count", walked, want)
	}
	t.Logf("F2 whole-surface: %d GET routes carry no %d unauthenticated. OPEN by decision, not by omission (R-BB-SIBLING-AGGREGATES): revenue.servedBytes = %d (8x) and balance = %d (7x) on a node whose one held root /api/roots would name",
		walked, funded, full.Revenue.ServedBytes, full.Revenue.Balance)
}

// --- one cache, two views -------------------------------------------------------------

// TestR29aOneCacheTwoViewsAnAnonymousReadDoesNotStripTheOperatorsView. The cache holds
// ONE document and serves it at two privilege levels, so the withholding must be
// applied to a COPY. The PE review replaced apiStatus's `out.Durability = …` with
// `doc.Durability = …` — the one-token change a future "avoid the copy" optimisation
// takes — and the whole suite stayed green in both builds: on a live daemon one
// anonymous GET then stripped the operator's own solvency panel for the rest of the
// interval, intermittently, with no error. Silent-loss shape (Don't #4), reintroduced
// on a new axis and ungated. This is the gate: tokened, untokened, tokened again, all
// INSIDE one interval, and the third read must be byte-identical to the first and
// still carry the objects. Both documents, because both are served off that one cache.
func TestR29aOneCacheTwoViewsAnAnonymousReadDoesNotStripTheOperatorsView(t *testing.T) {
	s, led := statusServer(t)
	root := r29aCaredObject(t, s, led)
	at := s.started // one instant: every read is inside one interval and the age is 0 on each

	first := statusAt(t, s, at, true)
	_ = statusAt(t, s, at, false) // the anonymous read that must not poison the cache
	third := statusAt(t, s, at, true)
	if third != first {
		t.Fatalf("an unauthenticated read inside the interval changed what the OPERATOR is served afterwards — the withholding was applied to the cached document rather than to a copy, so one anonymous GET strips the operator's solvency panel for the rest of the interval.\nbefore: %s\nafter:  %s", first, third)
	}
	var d struct {
		DetailWithheld bool `json:"detailWithheld"`
		Objects        []struct {
			Root string `json:"root"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(statusKey(t, third, "durability"), &d); err != nil {
		t.Fatal(err)
	}
	if d.DetailWithheld || len(d.Objects) != 1 || d.Objects[0].Root != root.String() {
		t.Fatalf("the operator's tokened read after an anonymous one lost its objects: %+v", d)
	}

	e1 := economySelfAt(t, s, at, true)
	_ = economySelfAt(t, s, at, false)
	e3 := economySelfAt(t, s, at, true)
	if e3 != e1 {
		t.Fatalf("an unauthenticated /api/economy/self read inside the interval changed the operator's next read.\nbefore: %s\nafter:  %s", e1, e3)
	}
	if v := decodeEconomySelfView(t, e3); v.DetailWithheld || v.SelfFunding == nil || v.Objects == nil {
		t.Fatalf("the operator's tokened economy/self read after an anonymous one lost its detail: %s", e3)
	}
}

// --- the sibling is served from the SAME snapshot ---------------------------------------

// TestR29aEconomySelfIsServedFromTheStatusSnapshot. The floor(uptime/T) bound the
// ratified D-STATUS-SNAPSHOT-INTERVAL claims is only true if every endpoint that
// republishes the ledger aggregates reads the same snapshot. As first shipped
// /api/economy/self recomputed per request, and the PE review extracted the
// selfFunding step at 250 ms polling. This gate: a serve inside the interval is
// invisible on economy/self until the interval turns over, the two documents carry the
// SAME taken-at stamp, and the age moves so a reader can tell the cached value from a
// live one.
func TestR29aEconomySelfIsServedFromTheStatusSnapshot(t *testing.T) {
	s, led := statusServer(t)
	root := r29aCaredObject(t, s, led)
	base := s.started

	first := decodeEconomySelfView(t, economySelfAt(t, s, base, true))
	if first.SelfFunding == nil || first.SelfFunding.SkimIn != 1024 {
		t.Fatalf("fixture: skimIn = %+v, want 1024", first.SelfFunding)
	}
	var st struct {
		TakenAt int64 `json:"snapshotTakenAtUnix"`
	}
	if err := json.Unmarshal([]byte(statusAt(t, s, base, true)), &st); err != nil {
		t.Fatal(err)
	}
	if first.TakenAt != st.TakenAt || first.Interval != int64(statusSnapshotInterval/time.Second) {
		t.Fatalf("economy/self stamps takenAt %d interval %d; /api/status stamps takenAt %d — the two documents must come off ONE snapshot, or an observer diffs them against each other", first.TakenAt, first.Interval, st.TakenAt)
	}

	// The interleaved serve: exactly the observation the delta extraction needs.
	s.onLoop(func() {
		led.RecordServeToObject(s.nd.ID(), ports.NodeID{0xC4}, root, ports.ChunkID{0x2}, 16384)
	})
	second := decodeEconomySelfView(t, economySelfAt(t, s, base.Add(statusSnapshotInterval-time.Millisecond), true))
	if second.SelfFunding.SkimIn != 1024 {
		t.Fatalf("a serve inside the refresh interval is visible on /api/economy/self (skimIn 1024 -> %d) — the endpoint is recomputed per request, so the disclosed floor(uptime/T) bound is false and the extraction rate is the reader's own poll rate", second.SelfFunding.SkimIn)
	}
	if second.TakenAt != first.TakenAt || second.AgeSec != 4 {
		t.Fatalf("inside one interval takenAt must hold and the age must move: takenAt %d -> %d, age %d (want 4)", first.TakenAt, second.TakenAt, second.AgeSec)
	}
	third := decodeEconomySelfView(t, economySelfAt(t, s, base.Add(statusSnapshotInterval+time.Second), true))
	if third.SelfFunding.SkimIn != 1024+2048 {
		t.Fatalf("past the interval the serve is still invisible (skimIn %d, want 3072) — that is a freeze, not a snapshot", third.SelfFunding.SkimIn)
	}
}
