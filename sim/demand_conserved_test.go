package sim

// TestDeliveryCreditConservedOverWire is conservation on the SESSION lane, end to end in
// one ledger view (B-9 retired the flat receipt): the fetcher's withdrawal burns one face
// on the server's ledger; the token opens a session at the server; a cumulative-count
// receipt settles count·p — count·p − skim to the server, skim to the object's escrow —
// and the unsettled remainder is a pending DEPOSIT (released at anchor expiry). Nothing is
// minted: Σ(server + fetcher + escrow + pending) is exactly unchanged; standing never moves.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

func TestDeliveryCreditConservedOverWire(t *testing.T) {
	const seed = 20260826
	const fee = int64(1000)
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	server, serverSigner := identityNode(cl, 2026090201)
	fetcher, fetcherSigner := identityNode(cl, 2026090208)
	ledger := credit.New(fee, 100*fee)
	server.SetLedger(ledger)
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	server.EnableTokenIssuer(rand.Reader, issuerKey)
	sc := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	if gerr := sc.AppendGenesis(issuerKeyGenesis(t, serverSigner, issuerKey)); gerr != nil {
		t.Fatalf("issuer-key genesis: %v", gerr)
	}
	wireDemandLane(t, cl, server, server, serverSigner, issuerKey, sc, serverSigner, demandFetcher{fetcher, fetcherSigner})
	server.EnableDeliverySessions(10 * ports.Second)
	ledger.Register(server.ID())
	ledger.Register(fetcher.ID())
	serverStanding := ledger.Reputation(server.ID())
	serverStart := ledger.Balance(server.ID())
	fetcherStart := ledger.Balance(fetcher.ID())
	object := ports.HashBytes([]byte("conserved-object"))

	tok := acquireDemandToken(t, cl, fetcher, server.ID())
	if got := fetcherStart - ledger.Balance(fetcher.ID()); got != fee {
		t.Fatalf("withdrawal charged %d, want the fee %d", got, fee)
	}
	handle, m, oerr := openSession(t, cl, fetcher, server, tok)
	if oerr != nil {
		t.Fatalf("open: %v", oerr)
	}
	const count = int64(8) // 8 increments: value 8, skim 1
	if settled, serr := settleSession(t, cl, fetcher, server, handle, m, object, uint64(count)); serr != nil || settled != count*credit.DeliveryIncrementCredit {
		t.Fatalf("settle: (%d, %v)", settled, serr)
	}
	value := count * credit.DeliveryIncrementCredit
	skim := value * credit.SkimNum / credit.SkimDen
	if got := ledger.Balance(server.ID()) - serverStart; got != value-skim {
		t.Fatalf("server was paid %d, want count·p − skim = %d", got, value-skim)
	}
	if got := ledger.EscrowBalance(object); got != skim {
		t.Fatalf("object escrow holds %d, want the skim %d", got, skim)
	}
	// The remainder (fee − value) becomes the session's DEPOSIT when the idle sweep closes
	// it; drive the sweep and count it in the conserved total.
	cl.Sched.RunUntil(cl.Sched.Now().Add(11 * ports.Second))
	server.SweepDeliverySessions()
	st := ledger.DeliverySettlementStats()
	if st.PendingRefundCredits != fee-value {
		t.Fatalf("pending deposit %d, want fee − value = %d", st.PendingRefundCredits, fee-value)
	}
	total := (ledger.Balance(server.ID()) - serverStart) + (ledger.Balance(fetcher.ID()) - fetcherStart) + ledger.EscrowBalance(object) + st.PendingRefundCredits
	if total != 0 {
		t.Fatalf("Σ(server + fetcher + escrow + pending) moved by %d, want 0: a receipt moves value, never mints it", total)
	}
	if got := ledger.Reputation(server.ID()); got != serverStanding {
		t.Fatalf("delivery credit moved server standing %d → %d — never standing", serverStanding, got)
	}
}
