package sim

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

// bondingGenesis builds a minimal objective chain whose genesis DECLARES a bond for
// each key in `bonded` (its NodeID = sha256(pubkey) enters the committed bond ledger
// at minBond). Genesis state is trusted — AppendGenesis applies bonds without
// re-verifying the space-time proof — so this seeds exactly the committed bond ledger
// the P3b gate (chain.IsBonded) consults, without standing up consensus rounds. Each
// bond gets a distinct Root so the per-root dedup (F1) counts them as distinct
// identities.
func bondingGenesis(t *testing.T, proposer ed25519.PrivateKey, issuerReg chain.IssuerKeyReg,
	bonded ...ed25519.PublicKey) *chain.Chain {
	t.Helper()
	const minBond = int64(1) << 20
	propPub := proposer.Public().(ed25519.PublicKey)
	cfg := chain.Config{
		Quorum:           1,
		MinBond:          minBond,
		Anchors:          map[ports.NodeID]bool{ports.HashBytes(propPub): true},
		AnchorQuorum:     1,
		MatureValidators: 99, // never matures here; irrelevant to IsBonded
	}
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })

	reg := func(pub ed25519.PublicKey) chain.BondReg {
		return chain.BondReg{
			Validator: append([]byte(nil), pub...),
			Root:      ports.HashBytes(pub), // distinct per identity (dodges F1 dedup)
			Size:      minBond,
		}
	}
	regs := []chain.BondReg{reg(propPub)}
	for _, pub := range bonded {
		regs = append(regs, reg(pub))
	}
	// R0.4b: v5 genesis so it can commit the demand-issuer key binding (the era-3 leaf
	// set does not carry that keyspace, so a pre-v5 block carrying one is rejected).
	g := &chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{synthEntry("bonded-demand genesis")},
		BondRegs:   regs,
		IssuerKeys: []chain.IssuerKeyReg{issuerReg},
	}
	chain.Sign(g, proposer)
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("bonding genesis: %v", err)
	}
	return ch
}

// TestDemandBondedFetcherCapsWash is the P3b self-dealing red-team over the REAL node
// wire (AcquireDemandToken → SubmitDeliveryReceipt → the server's demand bank), the
// companion to TestDemandWashCostsRealFees's fee-burn lever. With the bonded-fetcher
// credential on, a washer running ONE bonded identity mints N genuine, verifiable
// delivery receipts (a self-fetch IS a real paid delivery — Douceur is unbeaten), yet
// witnessed demand rises by exactly 1. An UNBONDED fetcher's equally-valid delivery
// counts zero, and a SECOND distinct bonded identity adds exactly 1 — so fake demand
// is priced onto the storage-bond supply (one bond per unit), while real plural demand
// is untouched. Demand stays neutral throughout.
func TestDemandBondedFetcherCapsWash(t *testing.T) {
	const seed = 20260807
	cl := NewCluster(seed, 8, simnet.DefaultConfig(), node.DefaultConfig())
	server := cl.Nodes[1]
	washer, other, unbonded := cl.Nodes[2], cl.Nodes[3], cl.Nodes[4]
	// R0.4b: the issuer's NodeID must be its committed identity (the production shape).
	issuer, issuerSigner := identityNode(cl, 2026090204)

	// The issuer signs retrieval tokens (a funded ledger so withdrawals succeed; the
	// fee is not what this test asserts — TestDemandWashCostsRealFees owns that).
	issuerKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ledger := credit.New(1000, 100_000) // grant funds each fetcher's withdrawals
	issuer.SetLedger(ledger)
	issuer.EnableTokenIssuer(rand.Reader, issuerKey)

	// Each fetcher signs receipts with its own identity key; bond only the washer's
	// and `other`'s keys on the server's committed chain, NOT the unbonded fetcher's.
	sign := func(n *node.Node) (ed25519.PublicKey, ed25519.PrivateKey) {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		n.SetSigner(priv)
		return pub, priv
	}
	washerPub, washerKey := sign(washer)
	otherPub, otherKey := sign(other)
	unbondedPub, unbondedKey := sign(unbonded)
	_ = unbondedPub // deliberately left unbonded

	_, serverSigner, _ := ed25519.GenerateKey(rand.Reader)
	issuerReg := chain.SignIssuerKeyReg(issuerSigner, 0, demand.KeyFingerprint(&issuerKey.PublicKey))
	sc := bondingGenesis(t, serverSigner, issuerReg, washerPub, otherPub)
	wireDemandLane(t, cl, issuer, server, issuerSigner, issuerKey, sc, serverSigner,
		demandFetcher{washer, washerKey}, demandFetcher{other, otherKey},
		demandFetcher{unbonded, unbondedKey})
	server.RequireBondedFetchers()

	data := make([]byte, 16<<10)
	cl.rng.Read(data)
	object := ports.HashBytes(data)

	deliver := func(f *node.Node) bool {
		tok := acquireDemandToken(t, cl, f, issuer.ID())
		credited := false
		f.SubmitDeliveryReceipt(server.ID(), tok, object, func(c bool, err error) {
			credited = err == nil && c
		})
		cl.Sched.Run()
		return credited
	}

	// The washer mints N valid receipts from its single bonded identity.
	const N = 5
	firstCredited := false
	for i := 0; i < N; i++ {
		if deliver(washer) {
			firstCredited = true
		}
	}
	if !firstCredited {
		t.Fatal("the washer's first bonded delivery must credit demand")
	}
	if got := server.WitnessedDemand(object); got != 1 {
		t.Fatalf("one bonded identity washed %d receipts to demand %d, want 1 (cost-to-wash = one bond per unit)", N, got)
	}

	// An unbonded fetcher's equally-valid delivery counts nothing.
	if deliver(unbonded) {
		t.Fatal("an unbonded fetcher must not credit demand")
	}
	if got := server.WitnessedDemand(object); got != 1 {
		t.Fatalf("unbonded delivery moved demand to %d, want 1", got)
	}

	// A second, genuinely distinct bonded identity adds exactly one unit — real
	// plural demand is not penalized by the cap.
	if !deliver(other) {
		t.Fatal("a distinct bonded fetcher's delivery must credit demand")
	}
	if got := server.WitnessedDemand(object); got != 2 {
		t.Fatalf("distinct bonded fetcher gave demand %d, want 2", got)
	}
}
