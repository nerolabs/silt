package sim

// TestDemandBondedFetcherCapsWash is D-DEMAND P3b at the sim tier, on the SESSION lane
// (B-9 retired the flat receipt): with the bonded-fetcher credential ON, witnessed demand
// counts DISTINCT bonded fetchers per object on its own surface (DistinctBondedFetchers),
// while the increment surface counts what was settled — two surfaces, never one field
// (certification R2.9-witnessed-demand-observable-under-sessions-2026-09-06 §3.3). One
// bonded identity settling N times is ONE distinct fetcher; an unbonded fetcher's equally
// valid deliveries are PAID (G-DEM-4: settlement is never gated on the observable) but
// contribute to neither surface; a second genuinely distinct bonded identity adds one.

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

func TestDemandBondedFetcherCapsWash(t *testing.T) {
	const seed = 20260807
	const fee = int64(1000)
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	server, serverSigner := identityNode(cl, 2026090204)
	washer, washerKey := identityNode(cl, 2026090205)
	other, otherKey := identityNode(cl, 2026090206)
	unbonded, unbondedKey := identityNode(cl, 2026090207)

	issuerKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ledger := credit.New(fee, 100*fee) // the grant funds each fetcher's withdrawals
	server.SetLedger(ledger)
	server.EnableTokenIssuer(rand.Reader, issuerKey)

	// Bond only the washer's and `other`'s identities on the server's committed chain.
	washerPub := washerKey.Public().(ed25519.PublicKey)
	otherPub := otherKey.Public().(ed25519.PublicKey)
	issuerReg := chain.SignIssuerKeyReg(serverSigner, 0, demand.KeyFingerprint(&issuerKey.PublicKey))
	sc := bondingGenesis(t, serverSigner, issuerReg, washerPub, otherPub)
	wireDemandLane(t, cl, server, server, serverSigner, issuerKey, sc, serverSigner,
		demandFetcher{washer, washerKey}, demandFetcher{other, otherKey}, demandFetcher{unbonded, unbondedKey})
	server.RequireBondedFetchers()
	server.EnableDeliverySessions(10 * ports.Second)
	for _, n := range []*node.Node{server, washer, other, unbonded} {
		ledger.Register(n.ID())
	}

	data := make([]byte, 16<<10)
	cl.rng.Read(data)
	object := ports.HashBytes(data)

	// open opens one session for f with a freshly withdrawn token and returns its handle.
	open := func(f *node.Node) (uint64, []byte) {
		tok := acquireDemandToken(t, cl, f, server.ID())
		h, m, err := openSession(t, cl, f, server, tok)
		if err != nil {
			t.Fatalf("%s: open refused: %v", f.ID(), err)
		}
		return h, m
	}

	// The washer settles N receipts from its single bonded identity, one increment each.
	const N = 5
	wh, wm := open(washer)
	for i := 1; i <= N; i++ {
		if settled, err := settleSession(t, cl, washer, server, wh, wm, object, uint64(i)); err != nil || settled != 1 {
			t.Fatalf("washer receipt %d: settled %d err %v", i, settled, err)
		}
	}
	if d, inc := server.DistinctBondedFetchers(object), server.WitnessedIncrements(object); d != 1 || inc != N {
		t.Fatalf("one bonded identity washed %d receipts: distinct %d (want 1 — cost-to-wash = one bond per unit), increments %d (want %d)", N, d, inc, N)
	}

	// An unbonded fetcher's equally-valid delivery is PAID and counts on neither surface.
	uh, um := open(unbonded)
	before := ledger.Balance(server.ID())
	if settled, err := settleSession(t, cl, unbonded, server, uh, um, object, 1); err != nil || settled != 1 {
		t.Fatalf("the unbonded fetcher's settlement was refused (%d, %v) — P3b gates the OBSERVABLE, never the payment (G-DEM-4)", settled, err)
	}
	if ledger.Balance(server.ID()) != before+1 {
		t.Fatal("the unbonded fetcher's settlement did not pay the server")
	}
	if d, inc := server.DistinctBondedFetchers(object), server.WitnessedIncrements(object); d != 1 || inc != N {
		t.Fatalf("an unbonded delivery moved a surface: distinct %d (want 1), increments %d (want %d)", d, inc, N)
	}

	// A second, genuinely distinct bonded identity adds exactly one distinct unit.
	oh, om := open(other)
	if settled, err := settleSession(t, cl, other, server, oh, om, object, 1); err != nil || settled != 1 {
		t.Fatalf("other: (%d, %v)", settled, err)
	}
	if d, inc := server.DistinctBondedFetchers(object), server.WitnessedIncrements(object); d != 2 || inc != N+1 {
		t.Fatalf("distinct bonded fetcher gave distinct %d (want 2), increments %d (want %d)", d, inc, N+1)
	}
}
