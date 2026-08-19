package main

// Phase 2, Slice 3 — the durability endowment endpoint (POST /api/fund). A
// publisher/operator prepays an object's repair reserve from THIS daemon's own
// EARNED credit balance, so content outlives churn before it self-funds via the
// serve auto-skim. Standing is untouched (Invariant A). These tests cover the
// new parsing (link OR bare hash) and the handler's status-code contract
// (200 endow / 402 insufficient credit / 400 bad input).
// Deliberation: docs/thinking/2026-08-19-phase2-economy-on-deliberation.md.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

func TestParseRootArg(t *testing.T) {
	var want ports.Hash
	for i := range want {
		want[i] = byte(i + 1)
	}
	h := link.Handle{Root: want, Key: want} // Key reused as a 32-byte value; only Root matters
	care := link.CareHandle{Root: want, LayoutKey: want}

	cases := []struct {
		name, in string
		ok       bool
	}{
		{"full-link", h.String(), true},
		{"care-link", care.String(), true},
		{"bare-hash", want.String(), true},
		{"garbage", "not-a-link", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		got, err := parseRootArg(c.in)
		if c.ok {
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", c.name, err)
			}
			if got != want {
				t.Fatalf("%s: root=%s, want %s", c.name, got, want)
			}
		} else if err == nil {
			t.Fatalf("%s: expected an error for %q", c.name, c.in)
		}
	}
}

// fundServer builds a uiServer over a real node+loop+ledger with a known grant.
func fundServer(t *testing.T, grant int64) (*uiServer, http.Handler) {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	id := identity.FromSeed(9800)
	tr, err := tcpnet.New(loop, id, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	nd := node.New(id.NodeID(), node.DefaultConfig(), walltime.New(loop), tr, memstore.New())
	nd.SetLedger(credit.New(0, grant)) // fee 0, `grant` starter credits per account
	s := &uiServer{loop: loop, nd: nd, token: "tok", started: time.Now()}
	return s, s.guard(http.HandlerFunc(s.apiFund))
}

func postFund(h http.Handler, root, amount string) *httptest.ResponseRecorder {
	form := url.Values{"root": {root}, "amount": {amount}}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/fund", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestApiFundEndowsReserveFromBalance(t *testing.T) {
	s, h := fundServer(t, 10_000)
	root := ports.Hash{0xE1, 0xD0}

	w := postFund(h, root.String(), "3000")
	if w.Code != 200 {
		t.Fatalf("fund: status %d, body %s", w.Code, w.Body.String())
	}
	// The escrow now holds the endowment and the node balance is debited.
	var reserve, balance int64
	s.onLoop(func() {
		reserve = s.nd.DurabilityReserve(root)
		balance = s.nd.CreditBalance()
	})
	if reserve != 3000 {
		t.Fatalf("reserve=%d, want 3000", reserve)
	}
	if balance != 7000 {
		t.Fatalf("balance=%d, want 7000 (10000 grant - 3000 endowment)", balance)
	}
}

func TestApiFundInsufficientCreditIs402(t *testing.T) {
	_, h := fundServer(t, 1_000)
	w := postFund(h, ports.Hash{0x01}.String(), "5000") // more than the 1000 grant
	if w.Code != 402 {
		t.Fatalf("over-balance fund: status %d (want 402), body %s", w.Code, w.Body.String())
	}
}

func TestApiFundBadInputIs400(t *testing.T) {
	_, h := fundServer(t, 10_000)
	if w := postFund(h, "not-a-root", "100"); w.Code != 400 {
		t.Fatalf("bad root: status %d, want 400", w.Code)
	}
	if w := postFund(h, ports.Hash{0x02}.String(), "0"); w.Code != 400 {
		t.Fatalf("zero amount: status %d, want 400", w.Code)
	}
	if w := postFund(h, ports.Hash{0x02}.String(), "-5"); w.Code != 400 {
		t.Fatalf("negative amount: status %d, want 400", w.Code)
	}
}

// TestApiFundNeedsToken: funding spends credits, so it is a mutating call and
// must be refused without the bearer token (the #89 gate).
func TestApiFundNeedsToken(t *testing.T) {
	_, h := fundServer(t, 10_000)
	form := url.Values{"root": {ports.Hash{0x03}.String()}, "amount": {"100"}}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/fund", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// no Authorization header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated fund: status %d, want 401", w.Code)
	}
}
