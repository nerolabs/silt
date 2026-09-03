package sim

// R0.4b sim scaffolding: wiring the demand lane's per-epoch issuer keyset.
//
// Since R0.4b a redeemer will not hold key_E unless its fingerprint matches the
// CONSENSUS-ATTESTED commitment for (issuer, epoch). That is the anti-fingerprinting
// binding the certification makes mandatory, and it has a consequence for every
// demand-lane test: the server needs a CHAIN carrying the binding, and the issuer's
// identity must be the one the binding names.
//
// In production NodeID = sha256(identity pubkey), so "the peer that served the key"
// and "the identity the binding names" are the same value. NewCluster gives its nodes
// RANDOM NodeIDs unrelated to any signer, so these fixtures build the issuer node from
// an IDENTITY instead — which is what production does, and what keeps the one-NodeID
// API honest (two parameters that must always be equal are a footgun).

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// identityNode adds a node to cl whose NodeID is DERIVED from its signing identity
// (NodeID = sha256(pubkey)), the production shape. It joins the cluster's routing
// tables like any other peer.
func identityNode(cl *Cluster, seed int64) (*node.Node, ed25519.PrivateKey) {
	ident := identity.FromSeed(seed)
	id := ident.NodeID()
	nd := node.New(id, node.DefaultConfig(), cl.Sched, cl.Net.Endpoint(id), memstore.New())
	nd.SetSigner(ident.Signer())
	var seeds []ports.NodeID
	for i, peer := range cl.Nodes {
		if i >= 3 {
			break
		}
		seeds = append(seeds, peer.ID())
	}
	nd.Bootstrap(seeds, func() {})
	cl.Nodes = append(cl.Nodes, nd)
	cl.Sched.Run()
	return nd, ident.Signer()
}

// issuerKeyGenesis returns a v5 genesis block committing issuerSigner's demand key
// fingerprint for epoch 0, plus any extra registrations the caller wants folded in.
// Epochs stay OFF in these fixtures, so the consensus epoch is 0 throughout and one
// binding covers the whole run.
func issuerKeyGenesis(t *testing.T, issuerSigner ed25519.PrivateKey, issuerKey *rsa.PrivateKey) chain.Block {
	t.Helper()
	g := chain.Block{
		Version: chain.BlockVersionWitnessable,
		Height:  0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("demand-binding-genesis"))}},
		IssuerKeys: []chain.IssuerKeyReg{
			chain.SignIssuerKeyReg(issuerSigner, 0, demand.KeyFingerprint(&issuerKey.PublicKey)),
		},
	}
	chain.Sign(&g, issuerSigner)
	return g
}

// wireDemandLane is the standard R0.4b demand-lane setup: the issuer installs its
// per-epoch key, the server holds a chain committing that key's binding and banks
// against the issuer's identity, and every fetcher pins the issuer's served keyset.
//
// It ASSERTS the pin succeeded. A silent failure here would make every downstream
// receipt rejection look like a demand-logic bug instead of a missing binding.

// demandFetcher pairs a fetcher node with its RECEIPT-SIGNING key. The pair is
// required, not a convenience: EnableChain overwrites n.signer, so handing it the
// server's key would silently re-key every receipt the fetcher signs — which
// collapses distinct bonded fetchers onto one credential slot and makes a P3b test
// fail for a reason that has nothing to do with P3b.
type demandFetcher struct {
	nd     *node.Node
	signer ed25519.PrivateKey
}

func wireDemandLane(t *testing.T, cl *Cluster, issuer, server *node.Node,
	issuerSigner ed25519.PrivateKey, issuerKey *rsa.PrivateKey, serverChain *chain.Chain,
	serverSigner ed25519.PrivateKey, fetchers ...demandFetcher) {
	t.Helper()

	issuer.SetDemandIssuerKey(rand.Reader, 0, issuerKey)
	server.EnableChain(serverChain, serverSigner)
	server.EnableDemandBank(issuer.ID())

	// The server resolves the issuer's keyset against its own committed chain.
	if issuer.ID() != server.ID() {
		server.FetchDemandIssuerKeys(issuer.ID(), func(int, error) {})
		cl.Sched.Run()
	}
	if ks := server.DemandIssuerKeyset(issuer.ID()); ks == nil || ks.Key(0) == nil {
		t.Fatal("wireDemandLane: the server did not pin the issuer's committed key_0 — " +
			"the demand bank would reject every receipt")
	}

	// Each fetcher needs the keyset too: it blinds against the PINNED key so it can
	// never withdraw under a key the network does not agree on.
	for _, f := range fetchers {
		f.nd.EnableChain(serverChain, f.signer) // its OWN key, never the server's
		f.nd.FetchDemandIssuerKeys(issuer.ID(), func(int, error) {})
	}
	cl.Sched.Run()
	for _, f := range fetchers {
		if ks := f.nd.DemandIssuerKeyset(issuer.ID()); ks == nil || ks.Key(0) == nil {
			t.Fatalf("wireDemandLane: fetcher %s did not pin the issuer's committed key_0", f.nd.ID())
		}
	}
}

// acquireDemandToken withdraws one token on the R0.4b demand lane and returns it.
func acquireDemandToken(t *testing.T, cl *Cluster, f *node.Node, issuer ports.NodeID) demand.Token {
	t.Helper()
	var tok demand.Token
	var aerr error
	f.AcquireDemandTokenInWindow(rand.Reader, issuer, func(tk demand.Token, _ uint64, err error) {
		tok, aerr = tk, err
	})
	cl.Sched.Run()
	if aerr != nil {
		t.Fatalf("acquire demand token: %v", aerr)
	}
	return tok
}
