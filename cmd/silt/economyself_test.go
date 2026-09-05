package main

// Boulder 2, R2.1 economy observability slice 6a — the four local-exact SELF
// panels served by GET /api/economy/self, extending the /api/status durability
// block. These tests drive a real node+loop+ledger through the same handler the
// daemon serves and assert each panel's contract:
//   Panel 1 my-solvency: per-object horizon + cliff flag.
//   Panel 2 am-I-profitable: revenue split (serve vs bounty) + operator-cost margin.
//   Panel 3 is-durability-self-funding: pooled skim-in vs bounty-out.
//   Panel 4 wash self-check: serve/fetch symmetry SHAPE, "suspected" never "detected".
// Deliberation: docs/thinking/2026-09-01-economy-observability-design.md (§2, §5-6a).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// emptyRegistry is a no-op ports.Registry: every lookup misses. It lets a test
// Care for a root without wiring a real registry — Care's async lookup returns
// not-found immediately (no stall, no panic on a nil interface), and the
// durability accounting the panel reads is independent of the registry anyway.
type emptyRegistry struct{}

func (emptyRegistry) Publish(context.Context, ports.Entry) error { return nil }
func (emptyRegistry) Lookup(context.Context, ports.Hash) (ports.Entry, bool, error) {
	return ports.Entry{}, false, nil
}
func (emptyRegistry) All(context.Context) ([]ports.Entry, error) { return nil, nil }

// economyServer builds a uiServer over a real node+loop+ledger and returns the
// server, the guarded handler, the node's own ID, and the ledger (so a test can
// drive serve/fetch/bounty accounting directly).
func economyServer(t *testing.T, grant int64) (*uiServer, http.Handler, ports.NodeID, *credit.Ledger) {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	id := identity.FromSeed(9900)
	tr, err := tcpnet.New(loop, id, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	nd := node.New(id.NodeID(), node.DefaultConfig(), walltime.New(loop), tr, memstore.New())
	led := credit.New(0, grant)
	nd.SetLedger(led)
	// peerCount is a func field the daemon always wires. Both documents are now served
	// off the one status snapshot, so /api/economy/self reaches computeStatus too and a
	// nil here would segfault before any assertion.
	s := &uiServer{loop: loop, nd: nd, token: "tok", started: time.Now(), peerCount: func() int { return 0 }}
	return s, s.guard(http.HandlerFunc(s.apiEconomySelf)), id.NodeID(), led
}

func getEconomySelf(t *testing.T, h http.Handler, query string) economySelf {
	t.Helper()
	url := "http://127.0.0.1:8080/api/economy/self"
	if query != "" {
		url += "?" + query
	}
	r := httptest.NewRequest("GET", url, nil)
	// The OPERATOR's read. Panel 1 (my-solvency) is per-object and therefore
	// token-gated (red-team F2: delta skimIn x 8 is the exact byte count served of a
	// NAMED root), and the operator's own browser already attaches this header to every
	// same-origin /api/ call. TestR29aF2EconomySelfWithholdsPerObjectDetailWithoutAToken
	// covers the untokened read.
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("economy/self: status %d, body %s", w.Code, w.Body.String())
	}
	var out economySelf
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode economy/self: %v (body %s)", err, w.Body.String())
	}
	return out
}

// A read endpoint moves nothing, so it must be reachable WITHOUT the bearer token
// (the #89 read-only-localhost ergonomics). Every field is stamped local-exact. This
// is a real untokened request: getEconomySelf presents the token, and an earlier
// version of this test called it while its comment said "no Authorization header".
func TestEconomySelfIsReadOnlyAndLocalExact(t *testing.T) {
	_, h, _, _ := economyServer(t, 5_000)
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/economy/self", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r) // NO Authorization header
	if w.Code != 200 {
		t.Fatalf("untokened economy/self: status %d, body %s", w.Code, w.Body.String())
	}
	var out economySelf
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Tier != "local-exact" {
		t.Fatalf("tier=%q, want local-exact", out.Tier)
	}
	if out.Revenue.Balance != 5_000 {
		t.Fatalf("balance=%d, want the 5000 grant", out.Revenue.Balance)
	}
}

// Panel 2 (am-I-profitable): revenue is exact from the ledger; serve revenue and
// bounty revenue are SPLIT so an operator sees where credit came from; the margin
// is balance − operator-supplied cost, and absent a cost it is flagged not-given.
func TestEconomySelfMarginAndRevenueSplit(t *testing.T) {
	s, h, self, led := economyServer(t, 0)
	other := ports.NodeID{0xC3}
	root := ports.Hash{0x0A}

	// Serve 1000 bytes to `other` (earns balance, less the 1/8 skim → 875 net) and
	// earn a repair bounty of 500 on an escrow funded by `other`.
	led.Register(other)
	s.onLoop(func() {
		led.RecordServeToObject(self, other, root, ports.ChunkID{0x1}, 1000)
	})
	// Fund the escrow from `other` and pay `self` a repair bounty.
	if err := driveFund(s, led, root, other, 5_000); err != nil {
		t.Fatal(err)
	}
	s.onLoop(func() { led.PayBounty(root, self, 500) })

	out := getEconomySelf(t, h, "cost=200")
	// balance = 875 (serve net) + 500 (bounty) = 1375.
	if out.Revenue.Balance != 1375 {
		t.Fatalf("balance=%d, want 1375 (875 serve net + 500 bounty)", out.Revenue.Balance)
	}
	if out.Revenue.BountyEarned != 500 {
		t.Fatalf("bountyEarned=%d, want 500", out.Revenue.BountyEarned)
	}
	if out.Revenue.RepairsDone != 1 {
		t.Fatalf("repairsDone=%d, want 1", out.Revenue.RepairsDone)
	}
	if out.Revenue.ServeRevenue != 875 {
		t.Fatalf("serveRevenue=%d, want 875 (balance 1375 − bounty 500)", out.Revenue.ServeRevenue)
	}
	if !out.Margin.CostGiven || out.Margin.Cost != 200 {
		t.Fatalf("margin cost not carried: given=%v cost=%d", out.Margin.CostGiven, out.Margin.Cost)
	}
	if out.Margin.Margin != 1175 {
		t.Fatalf("margin=%d, want 1175 (1375 − 200)", out.Margin.Margin)
	}

	// Absent a cost, the margin is flagged not-given (never asserted as fact).
	out2 := getEconomySelf(t, h, "")
	if out2.Margin.CostGiven {
		t.Fatalf("no cost supplied but CostGiven=true")
	}
}

// Panel 3 (is-durability-self-funding): pooled skim-in vs bounty-out, with the
// drain signal (net<0) visible. bountyOn reports whether repair disburses.
func TestEconomySelfSelfFunding(t *testing.T) {
	s, h, self, led := economyServer(t, 0)
	other := ports.NodeID{0xC3}
	root := ports.Hash{0x0B}
	led.Register(other)

	// The self-funding panel reports THIS node's CARED objects, so care for the
	// root first (nil registry: no announce/lookup wiring needed for the accounting).
	s.onLoop(func() { s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root}) })

	// Serving skims into the escrow (skim-in); a bounty pays out (bounty-out). Make
	// bounty-out exceed skim-in to exercise the drain signal.
	s.onLoop(func() {
		led.RecordServeToObject(self, other, root, ports.ChunkID{0x1}, 800) // skim 100 in
	})
	if err := driveFund(s, led, root, other, 5_000); err != nil {
		t.Fatal(err)
	}
	s.onLoop(func() { led.PayBounty(root, self, 300) }) // 300 out

	out := getEconomySelf(t, h, "")
	// skim-in = 100 (from the 800-byte serve) + 5000 (the prepay) = 5100.
	if out.SelfFunding.SkimIn != 5100 {
		t.Fatalf("skimIn=%d, want 5100 (100 skim + 5000 prepay)", out.SelfFunding.SkimIn)
	}
	if out.SelfFunding.BountyOut != 300 {
		t.Fatalf("bountyOut=%d, want 300", out.SelfFunding.BountyOut)
	}
	if out.SelfFunding.Net != 4800 {
		t.Fatalf("net=%d, want 4800 (5100 − 300)", out.SelfFunding.Net)
	}
}

// Panel 4 (wash self-check): the SHAPE only. A node with near-symmetric serve:fetch
// AND a non-positive balance matches the wash shape → "suspected". Authenticity is
// never knowable and never asserted.
func TestEconomySelfWashSelfCheckIsShapeNotDetection(t *testing.T) {
	s, h, self, led := economyServer(t, 0)
	partner := ports.NodeID{0xD4}
	led.Register(partner)

	// Ping-pong equal bytes both ways so serve ≈ fetch (the wash shape). RecordServe
	// credits the SERVER and debits nothing, so to get a non-positive balance we
	// spend the earned credit back out via a publish-style charge is unavailable
	// here; instead use FundEscrow to move the earned balance into a reserve,
	// leaving the node at zero — the churn-nets-to-nothing signature.
	s.onLoop(func() {
		led.RecordServe(self, partner, ports.ChunkID{0x1}, 1000) // self serves 1000
		led.RecordServe(partner, self, ports.ChunkID{0x2}, 1000) // self fetches 1000
	})
	// self earned 1000 from serving; move it ALL into an escrow so balance returns
	// to 0 (the churn-nets-to-nothing signature). Fund from self's OWN earned
	// balance — no faucet — or the balance would not return to zero.
	root := ports.Hash{0x0C}
	var fundErr error
	s.onLoop(func() { fundErr = led.FundEscrow(root, self, 1000) })
	if fundErr != nil {
		t.Fatal(fundErr)
	}

	out := getEconomySelf(t, h, "")
	if out.Wash.Symmetry < 0.99 {
		t.Fatalf("symmetry=%.3f, want ~1.0 for equal serve/fetch", out.Wash.Symmetry)
	}
	if !out.Wash.BalanceNonPositive {
		t.Fatalf("balance should be 0 after moving all earnings to escrow")
	}
	if !out.Wash.Suspected {
		t.Fatalf("symmetric flow + non-positive balance should flag the wash SHAPE as suspected")
	}
	// The honesty contract: authenticity is NEVER knowable from one node.
	if out.Wash.AuthenticityKnowable {
		t.Fatalf("authenticity must never be claimed knowable (Douceur)")
	}

	// Contrast 1: an honest hot server serves far more than it fetches → low symmetry
	// → NOT suspected, even with a low balance. Prove the symmetry clause narrows it.
	s2, h2, self2, led2 := economyServer(t, 0)
	buyer := ports.NodeID{0xE5}
	led2.Register(buyer)
	s2.onLoop(func() {
		led2.RecordServe(self2, buyer, ports.ChunkID{0x1}, 10_000) // serves a lot
		led2.RecordServe(buyer, self2, ports.ChunkID{0x2}, 10)     // fetches almost nothing
	})
	out2 := getEconomySelf(t, h2, "")
	if out2.Wash.Suspected {
		t.Fatalf("an asymmetric hot server must NOT be wash-suspected: symmetry=%.4f", out2.Wash.Symmetry)
	}

	// Contrast 2: SYMMETRIC flow but a POSITIVE balance must NOT be suspected — this
	// isolates the BALANCE clause of the conjunction (a wash pair nets to nothing; a
	// symmetric-but-profitable node is not the shape). Dropping the balance<=0
	// requirement from the endpoint would wrongly flag this node, so this case is the
	// ablation that gives the conjunction its teeth.
	s3, h3, self3, led3 := economyServer(t, 0)
	peer := ports.NodeID{0xF6}
	led3.Register(peer)
	s3.onLoop(func() {
		led3.RecordServe(self3, peer, ports.ChunkID{0x1}, 1000) // serves 1000 (earns, balance +1000)
		led3.RecordServe(peer, self3, ports.ChunkID{0x2}, 1000) // fetches 1000 (symmetric)
	})
	out3 := getEconomySelf(t, h3, "")
	if out3.Wash.Symmetry < 0.99 {
		t.Fatalf("contrast-2 should be symmetric: symmetry=%.3f", out3.Wash.Symmetry)
	}
	if out3.Wash.BalanceNonPositive {
		t.Fatalf("contrast-2 kept its +1000 earnings, balance must be positive")
	}
	if out3.Wash.Suspected {
		t.Fatalf("symmetric flow with a POSITIVE balance must NOT be wash-suspected (the balance clause)")
	}
}

// Panel 1 (my-solvency): per cared object, the funded horizon and the cliff flag.
// An object with an OBSERVED burn (paid>0) has a finite horizon; when that horizon
// is inside the warning window it is a cliff (RED). An object with NO observed burn
// (paid==0) is "not yet measurable" — finite=false, and NEVER a cliff (an unmeasured
// burn is not a proven-safe one, so it is never rendered green/perpetual).
func TestEconomySelfSolvencyCliff(t *testing.T) {
	s, h, self, led := economyServer(t, 0)
	other := ports.NodeID{0xC3}
	led.Register(other)

	cliffRoot := ports.Hash{0xC1} // small reserve + observed burn → finite, near expiry
	quietRoot := ports.Hash{0xC2} // funded but no burn observed → not yet measurable
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: cliffRoot})
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: quietRoot})
	})
	if err := driveFund(s, led, cliffRoot, other, 1_000); err != nil {
		t.Fatal(err)
	}
	if err := driveFund(s, led, quietRoot, other, 1_000); err != nil {
		t.Fatal(err)
	}
	// A repair on cliffRoot spends most of its reserve, so the observed burn leaves a
	// tiny remaining horizon (< the 30-day warning window at the test's ms uptime).
	s.onLoop(func() { led.PayBounty(cliffRoot, self, 900) })

	out := getEconomySelf(t, h, "")
	byRoot := map[string]economyObject{}
	for _, o := range out.Objects {
		byRoot[o.Root] = o
	}
	cliff := byRoot[cliffRoot.String()]
	quiet := byRoot[quietRoot.String()]

	if !cliff.Finite {
		t.Fatalf("cliffRoot has an observed burn (paid=900), horizon must be finite")
	}
	if !cliff.Cliff {
		t.Fatalf("cliffRoot's tiny remaining horizon must flag a cliff, horizonSec=%d", cliff.HorizonSec)
	}
	if quiet.Finite {
		t.Fatalf("quietRoot funded no repairs (paid=0): horizon must be NOT measurable (finite=false)")
	}
	if quiet.Cliff {
		t.Fatalf("a not-yet-measurable object must NEVER be a cliff (never faked as green OR red)")
	}
	if quiet.HorizonSec != -1 {
		t.Fatalf("not-yet-measurable renders horizonSec=-1, got %d", quiet.HorizonSec)
	}
}

// driveFund funds an escrow from funder's balance on the event loop (all ledger
// mutations in these tests go through the loop to stay race-free). The funder has
// no grant, so it is first credited via a serve-earn faucet (RecordServe against a
// sink node), then FundEscrow moves that into the reserve.
func driveFund(s *uiServer, led *credit.Ledger, root ports.Hash, funder ports.NodeID, amount int64) error {
	var err error
	s.onLoop(func() {
		led.RecordServe(funder, ports.NodeID{0xFF}, ports.ChunkID{0xFF}, amount) // faucet
		err = led.FundEscrow(root, funder, amount)
	})
	return err
}
