package sim

// R2.9 — the v3 twin of the retired v2 cost-to-wash sim (the certification
// R2.9-witnessed-demand-observable-under-sessions-2026-09-06 §4.3 parity, owed alongside
// G-DEM-1…8). On the session lane the washer — one operator running the server AND a
// bonded fetcher — cannot prove or disprove demand authenticity any more than before;
// what the mechanism guarantees is PARITY: every unit of witnessed demand it registers
// is one settled increment (one credit gross), the same registered unit at the same
// price an honest fetcher pays, and the face it bought is consumed by exactly those
// units. Cost-to-wash per claimed unit is therefore p, never less (rule (a)'s 1/⌈B/U⌉
// discount is what this pins shut); the LEVEL (a full delivery claim of B bytes costs
// ⌈B/U⌉ credits, 195× less than the flat fee at 64 MiB) is R-DEMAND-PRICE-LEVEL, set by
// U/p and P3b, not by the counter. And nothing here moves standing.
//
// ABLATION that must redden it: count one unit per receipt (`b.increments[object]++`) —
// the washer then registers N units for N/… credits.

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

func TestSessionDemandWashPaysThePricePerIncrement(t *testing.T) {
	for _, step := range []uint64{8, 7, 1} {
		t.Run(fmt.Sprintf("step-%d", step), func(t *testing.T) { sessionWashAtStep(t, step) })
	}
}

// TestWashPayerPaysTheSkimAtEveryGranularity is G-SKIM-4's name for the same property; it
// runs the three arms above.
func TestWashPayerPaysTheSkimAtEveryGranularity(t *testing.T) {
	for _, step := range []uint64{7, 1} {
		t.Run(fmt.Sprintf("step-%d", step), func(t *testing.T) { sessionWashAtStep(t, step) })
	}
}

func sessionWashAtStep(t *testing.T, washStep uint64) {
	const seed = 20260906
	const fee = int64(50_000)
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	server, serverSigner := identityNode(cl, 2026090601)
	fetcher, fetcherSigner := identityNode(cl, 2026090602)
	ledger := credit.New(fee, 100*fee)
	server.SetLedger(ledger)
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server.EnableTokenIssuer(rand.Reader, issuerKey)
	sc := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	if gerr := sc.AppendGenesis(issuerKeyGenesis(t, serverSigner, issuerKey)); gerr != nil {
		t.Fatalf("issuer-key genesis: %v", gerr)
	}
	server.SetDemandIssuerKey(rand.Reader, 0, issuerKey)
	server.EnableChain(sc, serverSigner)
	server.EnableDemandBank(server.ID())
	server.EnableDeliverySessions(10 * ports.Second)
	fetcher.EnableChain(sc, fetcherSigner)
	ledger.Register(server.ID())
	ledger.Register(fetcher.ID())
	// The washer's fetcher is BONDED so a spurious standing motion would show.
	ledger.RecordBondChallenge(fetcher.ID(), fetcher.ID(), 64<<20, true, 1)
	standing := ledger.Reputation(fetcher.ID())
	fStart, sStart := ledger.Balance(fetcher.ID()), ledger.Balance(server.ID())
	total := func() int64 {
		var sum int64
		for _, b := range ledger.Balances() {
			sum += b
		}
		return sum
	}
	object := ports.HashBytes([]byte("washed-object"))
	totalStart := total() + ledger.EscrowBalance(object)

	var pinned int
	fetcher.FetchDemandIssuerKeys(server.ID(), func(n int, _ error) { pinned = n })
	cl.Sched.Run()
	if pinned == 0 {
		t.Fatal("no key pinned")
	}
	var tok demand.Token
	fetcher.AcquireDemandTokenInWindow(rand.Reader, server.ID(), func(tk demand.Token, _ uint64, _ error) { tok = tk })
	cl.Sched.Run()
	var handle uint64
	var commitment []byte
	fetcher.OpenDeliverySessionRemote(server.ID(), []demand.Token{tok}, func(h uint64, m []byte, _ error) { handle, commitment = h, m })
	cl.Sched.Run()
	if handle == 0 {
		t.Fatal("open refused")
	}

	// Wash N units of demand on a self-dealt object (a self-fetch IS a real paid delivery —
	// no receipt can tell otherwise) at the payer's chosen GRANULARITY: the skim below must
	// hold at every step size, not only at the divisor's (G-SKIM-4; the per-settlement
	// floor passed at 8 by coincidence and paid zero skim at 7 and at 1).
	const N = uint64(40)
	var settledSum int64
	for c := uint64(washStep); c <= N; c += washStep {
		fetcher.SubmitDeliverySettle(server.ID(), handle, commitment, object, c, func(s int64, _ error) { settledSum += s })
		cl.Sched.Run()
	}
	if N%washStep != 0 { // the tail
		fetcher.SubmitDeliverySettle(server.ID(), handle, commitment, object, N, func(s int64, _ error) { settledSum += s })
		cl.Sched.Run()
	}
	// PARITY: the registered units are exactly the settled increments — one credit gross
	// each, the honest fetcher's price for the same unit; the v2 counter is untouched.
	if server.WitnessedIncrements(object) != int64(N) || settledSum != int64(N)*credit.DeliveryIncrementCredit {
		t.Fatalf("witnessed increments %d / settled %d, want %d / %d", server.WitnessedIncrements(object), settledSum, N, N*credit.DeliveryIncrementCredit)
	}
	// The face was BURNED at withdrawal; the washer's server side got back N − skim, so
	// the washer's net cost for N units is fee − (N − skim) — the whole face consumed
	// only by units, and at least the skim per full face is gone for good (into the
	// object's escrow, the A4 residual).
	skim := int64(N) * credit.DeliveryIncrementCredit * credit.SkimNum / credit.SkimDen
	if ledger.Balance(fetcher.ID()) != fStart-fee || ledger.Balance(server.ID()) != sStart+int64(N)-skim {
		t.Fatalf("fetcher %d→%d server %d→%d, want −fee and +N−skim", fStart, ledger.Balance(fetcher.ID()), sStart, ledger.Balance(server.ID()))
	}
	if d := total() + ledger.EscrowBalance(object) - totalStart; d != int64(N)-fee {
		t.Fatalf("Σ_L moved by %d, want settled − face = %d", d, int64(N)-fee)
	}
	// Demand is neutral: nothing that matters to consensus moved.
	if ledger.Reputation(fetcher.ID()) != standing {
		t.Fatalf("washing moved standing %d → %d", standing, ledger.Reputation(fetcher.ID()))
	}
}
