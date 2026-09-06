package sim

// TestDemandReceiptFlowBanksWitnessedDemand is the D-DEMAND wiring at the sim tier, on
// the SESSION lane (B-9 retired the flat receipt): the fetcher's blind-withdrawn token is
// the session anchor, spent at open under the SERVER's own committed key (issuer ==
// server, the bilateral shape); a cumulative-count receipt banks witnessed increments; a
// replayed count banks nothing more; a token from an impostor issuer opens nothing.

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

func TestDemandReceiptFlowBanksWitnessedDemand(t *testing.T) {
	const seed = 20260809
	const fee = int64(50_000)
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	server, serverSigner := identityNode(cl, 2026090202)
	fetcher, fetcherSigner := identityNode(cl, 2026090203)
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

	data := make([]byte, 32<<10)
	cl.rng.Read(data)
	object := ports.HashBytes(data)

	tok := acquireDemandToken(t, cl, fetcher, server.ID())
	handle, m, oerr := openSession(t, cl, fetcher, server, tok)
	if oerr != nil {
		t.Fatalf("honest open refused: %v", oerr)
	}
	if settled, serr := settleSession(t, cl, fetcher, server, handle, m, object, 1); serr != nil || settled != 1 {
		t.Fatalf("honest receipt was not banked (settled %d, err %v)", settled, serr)
	}
	if got := server.WitnessedIncrements(object); got != 1 {
		t.Fatalf("witnessed increments = %d, want 1", got)
	}

	// Replay: the SAME cumulative count settles nothing more (settle-monotone).
	if settled, serr := settleSession(t, cl, fetcher, server, handle, m, object, 1); serr != nil || settled != 0 {
		t.Fatalf("a replayed count settled %d (err %v), want 0", settled, serr)
	}
	if got := server.WitnessedIncrements(object); got != 1 {
		t.Fatalf("replay inflated witnessed increments to %d, want 1", got)
	}

	// The same TOKEN cannot open a second session: it was spent into the guard at open.
	if _, _, err := openSession(t, cl, fetcher, server, tok); err == nil {
		t.Fatal("a spent anchor opened a second session")
	}

	// Forged issuer: a token blind-signed by a DIFFERENT key opens nothing.
	impostor, _ := rsa.GenerateKey(rand.Reader, 2048)
	serial := make([]byte, 32)
	rand.Read(serial)
	blinded, secret, _ := demand.Withdraw(rand.Reader, &impostor.PublicKey, 0, serial)
	forged, ferr := demand.Unblind(&impostor.PublicKey, 0, serial, demand.SignWithdrawal(rand.Reader, impostor, blinded), secret)
	if ferr != nil {
		t.Fatalf("the impostor's own signature must unblind under its own key: %v", ferr)
	}
	other, otherSigner := identityNode(cl, 2026090204)
	other.EnableChain(sc, otherSigner)
	if _, _, err := openSession(t, cl, other, server, forged); err == nil {
		t.Fatal("a token from an impostor issuer opened a session")
	}
	if got := server.WitnessedIncrements(object); got != 1 {
		t.Fatalf("forged token changed witnessed increments to %d, want 1", got)
	}
}
