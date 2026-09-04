package node

// R2.14 — shared node-tier fixtures for the ANCHORED relay lane (Builder,
// 2026-09-04). Every relay session now opens against verified prepayment anchors
// under the relay's own chain-committed key, so every pre-R2.14 test that opened a
// session needs three things it did not before: a relay with a committed key_0 and
// a self keyset, real ephemeral keypairs (sha256(Fetcher) == from), and anchors.
// These helpers give the existing tests those three things with a one-line change
// per open, so the properties they pin (the M0 guards, the #644 clamp, the #645
// sweep, the reaper, the resolver seam, single-settle) stay pinned unchanged.
//
// RSA keys are CACHED per index: a 2048-bit keygen is ~100 ms and the anchor tests
// need dozens of relays; nothing in any test depends on two clusters holding
// different keys, only on two relays IN one cluster holding different keys.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"sync"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

var (
	relayTestKeysMu sync.Mutex
	relayTestKeys   = map[int]*rsa.PrivateKey{}
	// relayKeyOf remembers the committed key each fixture relay was built with, so
	// a test holding only the *Node can still mint anchors under it.
	relayKeyOf sync.Map // ports.NodeID → *rsa.PrivateKey
)

// cachedRSAKey returns the i-th cached 2048-bit test key, generating it once.
func cachedRSAKey(t *testing.T, i int) *rsa.PrivateKey {
	t.Helper()
	relayTestKeysMu.Lock()
	defer relayTestKeysMu.Unlock()
	if k, ok := relayTestKeys[i]; ok {
		return k
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("test rsa key %d: %v", i, err)
	}
	relayTestKeys[i] = k
	return k
}

// commitSelfDemandKey gives n a chain whose genesis commits key as n's demand key_0
// (a v5 IssuerKeyReg), installs the key, and remembers it for mintAnchorsFor. After
// this n.DemandIssuerKeyset(n.id) resolves key_0 — the precondition for the anchor
// lane to be anything but dark.
func commitSelfDemandKey(t *testing.T, n *Node, ident *identity.Identity, key *rsa.PrivateKey) *chain.Chain {
	t.Helper()
	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version:    chain.BlockVersionWitnessable,
		Height:     0,
		Entries:    []ports.Entry{{Root: ports.HashBytes([]byte("r214-fixture-genesis"))}},
		IssuerKeys: []chain.IssuerKeyReg{chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&key.PublicKey))},
	}
	chain.Sign(&g, ident.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis with the issuer-key binding: %v", err)
	}
	n.SetSigner(ident.Signer())
	n.EnableChain(c, ident.Signer())
	n.SetDemandIssuerKey(rand.Reader, 0, key)
	if ks := n.DemandIssuerKeyset(n.id); ks == nil || ks.Key(0) == nil {
		t.Fatal("fixture: the node cannot resolve its OWN committed key_0")
	}
	relayKeyOf.Store(n.id, key)
	return c
}

// mintAnchorUnder is the issuer side of a blind withdrawal under key for issue
// epoch e, done directly and statelessly (goroutine-safe; the burn is exercised
// separately over the wire by TestRelayAnchorsAreBoughtOnTheRelaysOwnLedger).
func mintAnchorUnder(t *testing.T, key *rsa.PrivateKey, e uint64) relaypay.Anchor {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := &key.PublicKey
	blinded, secret, err := blindtoken.BlindRelayAnchor(rand.Reader, pub, e, serial)
	if err != nil {
		t.Fatalf("BlindRelayAnchor: %v", err)
	}
	blindSig, err := blindtoken.SignBlinded(rand.Reader, key, blinded)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := blindtoken.UnblindRelayAnchor(pub, e, serial, blindSig, secret)
	if err != nil {
		t.Fatalf("UnblindRelayAnchor: %v", err)
	}
	return relaypay.Anchor{Serial: serial, Sig: sig}
}

// mintAnchorsFor mints k anchors under the committed key of fixture relay n.
func mintAnchorsFor(t *testing.T, n *Node, e uint64, k int) []relaypay.Anchor {
	t.Helper()
	v, ok := relayKeyOf.Load(n.id)
	if !ok {
		t.Fatal("fixture: relay has no committed key (build it with commitSelfDemandKey)")
	}
	out := make([]relaypay.Anchor, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, mintAnchorUnder(t, v.(*rsa.PrivateKey), e))
	}
	return out
}

// openAnchoredWith presents a session to relay n from the ephemeral derived from
// ephSeed, with a freshly minted epoch-0 anchor, signing M for n. It returns the
// ephemeral's NodeID with the result so a test can name the session's owner.
func openAnchoredWith(t *testing.T, n *Node, ephSeed int64, root []byte, S int, funding FundingSource) (ports.NodeID, *RelaySession, error) {
	t.Helper()
	e := newEphemeral(ephSeed)
	anchors := mintAnchorsFor(t, n, 0, 1)
	sig := ed25519.Sign(e.priv, relayOpenCommitment(n.id, root, S, anchors))
	sess, err := n.OpenRelaySession(e.id, root, S, funding, anchors, e.pub, sig)
	return e.id, sess, err
}

// openAnchored is openAnchoredWith for the accepted (ephemeral-blind) funding form.
func openAnchored(t *testing.T, n *Node, ephSeed int64, root []byte, S int) (*RelaySession, error) {
	t.Helper()
	_, sess, err := openAnchoredWith(t, n, ephSeed, root, S, FundingEphemeralBlind)
	return sess, err
}
