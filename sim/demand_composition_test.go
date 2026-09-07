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
// RSA withdrawal over the wire; the delivery goes through the real MsgDeliveryOpen and
// MsgDeliverySettle handlers (B-9); and the assertion is POSITIVE SETTLED CREDIT, which is what the e2e
// `credit=` non-zero assertion measured.
//
// ABLATION that must redden it: drop the FetchDemandIssuerKeys call in step 1 (the
// fetcher then holds no pinned key and AcquireDemandTokenInWindow refuses with
// node.ErrNoIssuerKey), or make the second delivery reuse the first token (the guard
// refuses it and the second settlement is 0).

import (
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
	// R2.9: the session open is signed by the fetcher's DURABLE signer and the server
	// checks sha256(Fetcher) == the authenticated sender, so the fetcher must be a node
	// whose ID IS its signer's key hash (identityNode), not a cluster node with a random
	// signer bolted on.
	fetcher, fetcherSigner := identityNode(cl, 2026090302)

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
	server.EnableDeliverySessions(10 * ports.Second)

	ledger.Register(server.ID())
	ledger.Register(fetcher.ID())

	// R2.9: the delivery is a SESSION. The four node calls, in the order
	// `silt swarm receipt` makes them: pin the keys, withdraw the token (= the anchor),
	// open the session with it (the token is spent at OPEN), then settle one or more
	// cumulative-count receipts on it — here for TWO objects on ONE session (C7).
	var pinned int
	var keyErr error
	fetcher.FetchDemandIssuerKeys(server.ID(), func(n int, err error) { pinned, keyErr = n, err })
	cl.Sched.Run()
	if keyErr != nil || pinned == 0 {
		t.Fatalf("call 1 FetchDemandIssuerKeys: pinned %d err %v — the fetcher would refuse to withdraw and the lane is dark", pinned, keyErr)
	}
	var tok demand.Token
	var tokErr error
	fetcher.AcquireDemandTokenInWindow(rand.Reader, server.ID(), func(tk demand.Token, _ uint64, err error) { tok, tokErr = tk, err })
	cl.Sched.Run()
	if tokErr != nil {
		t.Fatalf("call 2 AcquireDemandTokenInWindow: %v", tokErr)
	}
	if got := ledger.Balance(fetcher.ID()); got != 100*fee-fee {
		t.Fatalf("the withdrawal charged %d, want one fee", 100*fee-got)
	}
	var handle uint64
	var commitment []byte
	var openErr error
	fetcher.OpenDeliverySessionRemote(server.ID(), []demand.Token{tok}, func(h uint64, m []byte, err error) { handle, commitment, openErr = h, m, err })
	cl.Sched.Run()
	if openErr != nil || handle == 0 {
		t.Fatalf("call 3 OpenDeliverySessionRemote: handle %d err %v", handle, openErr)
	}

	settle := func(t *testing.T, arm string, object ports.Hash, count uint64) int64 {
		t.Helper()
		before := ledger.Balance(server.ID())
		var settled int64
		var subErr error
		done := false
		fetcher.SubmitDeliverySettle(server.ID(), handle, commitment, object, count, func(s int64, err error) { settled, subErr, done = s, err, true })
		cl.Sched.Run()
		if !done || subErr != nil {
			t.Fatalf("%s: call 4 SubmitDeliverySettle: done=%v err=%v", arm, done, subErr)
		}
		_ = settled
		return ledger.Balance(server.ID()) - before
	}

	// ARM 1 — the first object's receipt must SETTLE POSITIVE CREDIT: 16 increments pay
	// 16·p − skim = 14. A banked receipt that pays 0 is the silent no-op the e2e
	// `credit=` assertion existed to catch.
	const j = 16
	want := int64(j)*credit.DeliveryIncrementCredit - int64(j)*credit.DeliveryIncrementCredit*credit.SkimNum/credit.SkimDen
	if first := settle(t, "first object", ports.HashBytes([]byte("composition-object-1")), j); first != want {
		t.Fatalf("the first object settled %d, want count·p − skim = %d", first, want)
	}
	// ARM 2 — a SECOND object on the SAME session (the session spans objects, C7): the
	// cumulative count advances by another 16 and pays again. One delivery is not a lane.
	if second := settle(t, "second object", ports.HashBytes([]byte("composition-object-2")), 2*j); second != want {
		t.Fatalf("the second object on the same session settled %d, want %d — the session did not span objects", second, want)
	}
	// ARM 3 — a replayed count on either object pays nothing: settle-monotone.
	if replay := settle(t, "replay", ports.HashBytes([]byte("composition-object-1")), 2*j); replay != 0 {
		t.Fatalf("a replayed cumulative count paid %d", replay)
	}
}
