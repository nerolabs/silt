package sim

// R0.4b C3 final round — the composition gate the e2e re-scope owes.
//
// WHY THIS EXISTS. e2e TestDeliveryReceiptBankedOverTCP used to drive the paid
// delivery lane's POSITIVE arm through two real OS processes. It cannot any more: a
// withdrawal is now only sound against a key that resolves to a committed E->key_E
// binding, and that binding needs an era-4/v5 chain, which the e2e fixture's
// -objective=false topology can never produce (G-8 convergence §1, 2026-09-03). That
// test was re-scoped to the certified refusal and renamed
// TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding.
//
// The convergence priced that re-scope at exactly TWO uncovered arms (§3), and this
// test is the payment for both:
//
//  1. THE THREE-CALL COMPOSITION on the SUCCESS arm. No tier ran
//     FetchDemandIssuerKeys -> AcquireDemandTokenInWindow -> SubmitDeliveryReceipt in
//     the order cmd/silt/swarm.go makes them, on a lane that pays. core/node
//     TestRTC3_RestartDoesNotRePayTheSameWireReceipt mints its token in-process and
//     calls the wire handler directly; TestRTC3_DegenerateCommittedKeyIsRefusedByThePinAndTheLane
//     drives call 2 on the REFUSAL arm only. Here the fetcher makes all three calls
//     itself, in order, over simnet, with no in-process shortcut.
//
//  2. THE SECOND GENUINE DELIVERY. The e2e test asserted that a fresh token on the
//     same lane banks again — one delivery is not a lane. Nothing below e2e drove it.
//
// The chain is a REAL v5 chain with a REAL committed IssuerKeyReg (issuerKeyGenesis),
// so the pin resolves against genuine consensus state; the withdrawal is a real blind
// RSA withdrawal over the wire; the receipt goes through the real MsgDeliveryReceipt
// handler; and the assertion is POSITIVE SETTLED CREDIT, which is what the e2e
// `credit=` non-zero assertion measured.
//
// ABLATION that must redden it: drop the FetchDemandIssuerKeys call in step 1 (the
// fetcher then holds no pinned key and AcquireDemandTokenInWindow refuses with
// node.ErrNoIssuerKey), or make the second delivery reuse the first token (the guard
// refuses it and the second settlement is 0).

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

func TestPaidDeliveryLaneThreeCallComposition(t *testing.T) {
	const seed = 20260903
	const fee = int64(50_000) // the shipped daemon's fee (cmd/silt/daemon.go)
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	fetcher := cl.Nodes[1]

	// The bilateral issuer==server shape the certification's settlement answer covers,
	// and the shape the e2e daemon ran: one node issues the tokens it later banks.
	server, serverSigner := identityNode(cl, 2026090301)
	ledger := credit.New(fee, 100*fee)
	server.SetLedger(ledger)
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	server.EnableTokenIssuer(rand.Reader, issuerKey)

	_, fetcherSigner, _ := ed25519.GenerateKey(rand.Reader)
	fetcher.SetSigner(fetcherSigner)

	// A REAL v5 chain committing this issuer's key_0 binding. Both sides read it; the
	// fetcher's pin has something genuine to resolve against.
	sc := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	if gerr := sc.AppendGenesis(issuerKeyGenesis(t, serverSigner, issuerKey)); gerr != nil {
		t.Fatalf("issuer-key genesis: %v", gerr)
	}
	server.SetDemandIssuerKey(rand.Reader, 0, issuerKey)
	server.EnableChain(sc, serverSigner)
	server.EnableDemandBank(server.ID())
	fetcher.EnableChain(sc, fetcherSigner)

	ledger.Register(server.ID())
	ledger.Register(fetcher.ID())

	// One delivery, exactly as `silt swarm receipt` performs it: three node calls, in
	// order, each completing before the next begins.
	deliver := func(t *testing.T, arm string, object ports.Hash) int64 {
		t.Helper()

		// CALL 1 — resolve the issuer's per-epoch keys against the committed binding.
		var pinned int
		var keyErr error
		fetcher.FetchDemandIssuerKeys(server.ID(), func(n int, err error) { pinned, keyErr = n, err })
		cl.Sched.Run()
		if keyErr != nil {
			t.Fatalf("%s: call 1 FetchDemandIssuerKeys: %v", arm, keyErr)
		}
		if pinned == 0 {
			t.Fatalf("%s: call 1 pinned no key against a chain that COMMITS the binding — "+
				"the client would refuse to withdraw and the lane is dark", arm)
		}

		// CALL 2 — a real blind withdrawal against the pinned key, naming the epoch.
		var tok demand.Token
		var tokErr error
		fetcher.AcquireDemandTokenInWindow(rand.Reader, server.ID(), func(tk demand.Token, _ uint64, err error) {
			tok, tokErr = tk, err
		})
		cl.Sched.Run()
		if tokErr != nil {
			t.Fatalf("%s: call 2 AcquireDemandTokenInWindow: %v", arm, tokErr)
		}

		// CALL 3 — sign and submit the receipt into the real wire handler.
		before := ledger.Balance(server.ID())
		var credited, done bool
		var subErr error
		fetcher.SubmitDeliveryReceipt(server.ID(), tok, object, func(c bool, err error) {
			credited, subErr, done = c, err, true
		})
		cl.Sched.Run()
		if subErr != nil {
			t.Fatalf("%s: call 3 SubmitDeliveryReceipt: %v", arm, subErr)
		}
		if !done || !credited {
			t.Fatalf("%s: the server did not bank the receipt (done=%v credited=%v)", arm, done, credited)
		}
		return ledger.Balance(server.ID()) - before
	}

	// ARM 1 — the first genuine delivery must SETTLE POSITIVE CREDIT. A banked receipt
	// that pays 0 is the silent no-op the e2e `credit=` assertion existed to catch.
	skim := fee * credit.SkimNum / credit.SkimDen
	first := deliver(t, "first delivery", ports.HashBytes([]byte("composition-object-1")))
	if first != fee-skim {
		t.Fatalf("the first delivery settled %d, want fee−skim = %d — the lane banked a "+
			"neutral observable and paid nothing", first, fee-skim)
	}

	// ARM 2 — a SECOND genuine delivery on the same lane, with a FRESH token, must also
	// bank and settle. One delivery is not a lane: the guard has to refuse a replayed
	// serial without refusing the next honest customer.
	second := deliver(t, "second delivery", ports.HashBytes([]byte("composition-object-2")))
	if second != fee-skim {
		t.Fatalf("the second genuine delivery settled %d, want fee−skim = %d — a fresh "+
			"token on the same lane must pay, or the double-spend guard is refusing "+
			"honest deliveries", second, fee-skim)
	}
}
