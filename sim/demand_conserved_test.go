package sim

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// TestDeliveryCreditConservedOverWire is the PoD neutral lane at the
// integration tier (docs/design/pod.md §3, certified 2026-08-26), in the
// certified BILATERAL shape: one node is both token issuer and server (the
// tit-for-tat pair the per-node-ledger settlement answer covers — cert Q5).
// The whole conservation story closes inside its one ledger view:
//
//	withdraw:  fetcher pays the fee (ChargePublish via tokenChargeFor)
//	redeem:    server is paid fee − skim; the skim funds the object's escrow
//	net:       the pair as a whole is down exactly the skim — a transfer,
//	           never a mint — and standing never moves (the §7.1 firewall).
func TestDeliveryCreditConservedOverWire(t *testing.T) {
	const seed = 20260826
	const fee = int64(1000)
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	fetcher := cl.Nodes[1]
	// R0.4b: the server is also the issuer, and its NodeID must be its committed
	// identity so the E->key_E binding names it (the production shape).
	server, serverSigner := identityNode(cl, 2026090201)

	// One node, both roles: issues the tokens it will later accept receipts
	// for, and holds the one ledger the bilateral loop settles in.
	ledger := credit.New(fee, 100*fee)
	server.SetLedger(ledger)
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	server.EnableTokenIssuer(rand.Reader, issuerKey)

	// The fetcher signs receipts with its NODE identity key, so the receipt's
	// Fetcher hashes to its NodeID — that is what routes the supersede lane.
	_, fetcherSigner, _ := ed25519.GenerateKey(rand.Reader)
	fetcher.SetSigner(fetcherSigner)

	// R0.4b: commit this issuer's key_0 binding, then pin it on both sides.
	sc := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	if gerr := sc.AppendGenesis(issuerKeyGenesis(t, serverSigner, issuerKey)); gerr != nil {
		t.Fatalf("issuer-key genesis: %v", gerr)
	}
	wireDemandLane(t, cl, server, server, serverSigner, issuerKey, sc, serverSigner, demandFetcher{fetcher, fetcherSigner})

	ledger.Register(server.ID())
	ledger.Register(fetcher.ID())
	serverStanding := ledger.Reputation(server.ID())
	serverStart := ledger.Balance(server.ID())
	fetcherStart := ledger.Balance(fetcher.ID())

	object := ports.HashBytes([]byte("conserved-object"))

	// Withdraw: the fee leaves the fetcher on the server's ledger.
	tok := acquireDemandToken(t, cl, fetcher, server.ID())
	if got := fetcherStart - ledger.Balance(fetcher.ID()); got != fee {
		t.Fatalf("withdrawal charged %d, want the fee %d", got, fee)
	}

	// Deliver + redeem over the wire.
	var credited bool
	fetcher.SubmitDeliveryReceipt(server.ID(), tok, object, func(c bool, err error) {
		if err != nil {
			t.Fatalf("submit receipt: %v", err)
		}
		credited = c
	})
	cl.Sched.Run()
	if !credited {
		t.Fatal("honest receipt was not banked")
	}

	// Conservation, end to end in the one ledger view.
	skim := fee * credit.SkimNum / credit.SkimDen
	if got := ledger.Balance(server.ID()) - serverStart; got != fee-skim {
		t.Fatalf("server was paid %d, want fee−skim = %d", got, fee-skim)
	}
	if got := ledger.EscrowBalance(object); got != skim {
		t.Fatalf("object escrow holds %d, want the skim %d", got, skim)
	}
	pairNet := (ledger.Balance(server.ID()) - serverStart) + (ledger.Balance(fetcher.ID()) - fetcherStart)
	if pairNet != -skim {
		t.Fatalf("the pair's net is %d, want exactly −skim (%d): a receipt must move value, never mint it", pairNet, -skim)
	}
	// The §7.1 firewall over the wire: delivery credit bought zero standing.
	if got := ledger.Reputation(server.ID()); got != serverStanding {
		t.Fatalf("delivery credit moved server standing %d → %d — never standing", serverStanding, got)
	}
}
