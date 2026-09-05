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

// statusServer is the /api/status fixture, and it exists because economyServer leaves
// peerCount a nil FUNC FIELD: that fixture only drives the SELF panel, so any test that
// drives /api/status off it segfaults before it asserts anything.
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

// TestR29aF2EconomySelfWithholdsPerObjectDetailWithoutAToken closes the twin surface.
// /api/economy/self republishes the same per-root skimIn/bountyOut as Panel 1, so
// gating /api/status alone would close nothing.
func TestR29aF2EconomySelfWithholdsPerObjectDetailWithoutAToken(t *testing.T) {
	s, h, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 }
	root := r29aCaredObject(t, s, led)

	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/economy/self", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r) // NO Authorization header
	if w.Code != 200 {
		t.Fatalf("economy/self: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	var out struct {
		ObjectsWithheld bool `json:"objectsWithheld"`
		Objects         *[]struct {
			Root   string `json:"root"`
			SkimIn int64  `json:"skimIn"`
		} `json:"objects"`
		SelfFunding struct {
			SkimIn int64 `json:"skimIn"`
		} `json:"selfFunding"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if out.Objects != nil {
		t.Fatalf("economy/self publishes per-root skimIn unauthenticated: %+v — it is the same eighth-of-the-bytes counter /api/status withholds, so leaving it open closes nothing (red-team F2)", *out.Objects)
	}
	if !out.ObjectsWithheld {
		t.Fatalf("objects withheld but objectsWithheld is false — withheld and empty must stay different objects")
	}
	if strings.Contains(body, root.String()) {
		t.Fatalf("the content root %s appears in the unauthenticated economy/self body:\n%s", root.String(), body)
	}
	// The POOLED figure names no root and stays open: it is the aggregate that already
	// shipped, and Panel 3 (is-durability-self-funding) is read from it.
	if out.SelfFunding.SkimIn != 1024 {
		t.Fatalf("pooled selfFunding.skimIn = %d, want 1024 — only the per-OBJECT decomposition is gated; the pooled drain signal names no root and must survive", out.SelfFunding.SkimIn)
	}

	// The operator's read still carries Panel 1.
	full := getEconomySelf(t, h, "")
	if full.ObjectsWithheld || len(full.Objects) != 1 || full.Objects[0].Root != root.String() {
		t.Fatalf("Panel 1 (my-solvency) is broken for the token holder: withheld=%v objects=%+v", full.ObjectsWithheld, full.Objects)
	}
}
